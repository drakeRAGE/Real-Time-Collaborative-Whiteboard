import { execFile } from 'node:child_process';
import { createHmac } from 'node:crypto';
import { mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { readFile } from 'node:fs/promises';
import { dirname, join } from 'node:path';
import { promisify } from 'node:util';
import type { Page } from '@playwright/test';

const exec = promisify(execFile);

export const localPassword = 'local-e2e-password';
export const authFile = join(process.cwd(), '.auth', 'users.json');

export interface SeedUser {
  role: 'admin' | 'guest';
  email: string;
  userId: string;
  token: string;
  refresh: string;
}

const users: { role: 'admin' | 'guest'; email: string; userId: string }[] = [
  { role: 'admin', email: 'e2e-admin@example.test', userId: '00000000-0000-4000-8000-00000000e2e1' },
  { role: 'guest', email: 'e2e-guest@example.test', userId: '00000000-0000-4000-8000-00000000e2e2' },
];

function b64url(raw: Buffer | string): string {
  return Buffer.from(raw).toString('base64url');
}

function mint(secret: string, issuer: string, audience: string, user: { email: string; userId: string }): string {
  const now = Math.floor(Date.now() / 1000);
  const header = b64url('{"alg":"HS256","typ":"JWT"}');
  const payload = b64url(JSON.stringify({
    iss: issuer,
    aud: audience,
    sub: user.userId,
    email: user.email,
    iat: now,
    exp: now + 2 * 60 * 60,
  }));
  const sig = createHmac('sha256', secret).update(`${header}.${payload}`).digest('base64url');
  return `${header}.${payload}.${sig}`;
}

function composeValue(text: string, key: string, fallback: string): string {
  const match = text.match(new RegExp(`^\\s*${key}:\\s*(\\S+)\\s*$`, 'm'));
  if (!match?.[1]) return fallback;
  return match[1].replace(/^["']|["']$/g, '');
}

/** Inserts two local users the way load/harness does, and writes HS256 sessions the gateway accepts. */
export async function seedLocalUsers(root: string): Promise<void> {
  const composePath = join(root, 'deploy', 'compose', 'docker-compose.yml');
  const compose = await readFile(composePath, 'utf8');
  const secret = composeValue(compose, 'SUPABASE_JWT_SECRET', '');
  if (!secret) {
    console.log('global-setup: SUPABASE_JWT_SECRET is unset. Authenticated scenarios will skip.');
    return;
  }
  const issuer = composeValue(compose, 'SUPABASE_JWT_ISSUER', 'http://127.0.0.1:54321/auth/v1');
  const audience = composeValue(compose, 'SUPABASE_JWT_AUDIENCE', 'authenticated');
  const sql = users.map((user) =>
    `INSERT INTO auth.users (id, email) VALUES ('${user.userId}', '${user.email}') ON CONFLICT (id) DO NOTHING;`,
  ).join(' ');
  try {
    await exec('docker', ['exec', 'whiteboard-postgres-1', 'psql', '-U', 'postgres', '-d', 'whiteboard', '-v', 'ON_ERROR_STOP=1', '-c', sql]);
    const check = await exec('docker', ['exec', 'whiteboard-postgres-1', 'psql', '-U', 'postgres', '-d', 'whiteboard', '-tA', '-c',
      `SELECT count(*) FROM profiles WHERE id IN ('${users[0].userId}', '${users[1].userId}');`]);
    if (String(check.stdout).trim() !== '2') {
      console.log('global-setup: profiles were not created for the local users. Authenticated scenarios will skip.');
      return;
    }
  } catch (err) {
    const message = err instanceof Error ? err.message.split('\n')[0] : 'seed failed';
    console.log(`global-setup: local user seed failed (${message}). Authenticated scenarios will skip.`);
    return;
  }
  const seeded: SeedUser[] = users.map((user) => ({
    ...user,
    token: mint(secret, issuer, audience, user),
    refresh: `e2e-${user.userId}`,
  }));
  mkdirSync(dirname(authFile), { recursive: true });
  writeFileSync(authFile, JSON.stringify(seeded), { mode: 0o600 });
  console.log('global-setup: seeded 2 local users against compose postgres.');
}

export function loadSeed(): SeedUser[] {
  try {
    const parsed = JSON.parse(readFileSync(authFile, 'utf8')) as SeedUser[];
    if (!Array.isArray(parsed) || parsed.length < 2) return [];
    return parsed;
  } catch {
    return [];
  }
}

function tokenSub(token: string): string {
  const part = token.split('.')[1];
  if (!part) return '';
  try {
    const payload = JSON.parse(Buffer.from(part, 'base64url').toString()) as { sub?: unknown };
    return typeof payload.sub === 'string' ? payload.sub : '';
  } catch {
    return '';
  }
}

export async function installLocalAuth(page: Page, seeded: SeedUser[]): Promise<void> {
  await page.route('**/auth/v1/**', async (route) => {
    let email = '';
    let refresh = '';
    try {
      const body = route.request().postDataJSON() as { email?: string; refresh_token?: string } | null;
      if (body && typeof body.email === 'string') email = body.email;
      if (body && typeof body.refresh_token === 'string') refresh = body.refresh_token;
    } catch {
      email = '';
    }
    const header = route.request().headers().authorization ?? '';
    const bearer = header.toLowerCase().startsWith('bearer ') ? header.slice(7).trim() : '';
    const sub = tokenSub(bearer);
    const user = seeded.find((item) => item.email === email)
      ?? seeded.find((item) => refresh !== '' && item.refresh === refresh)
      ?? seeded.find((item) => sub !== '' && item.userId === sub);
    if (!user) {
      await route.fulfill({ status: 401, contentType: 'application/json', body: '{"message":"no local user"}' });
      return;
    }
    const now = Math.floor(Date.now() / 1000);
    const record = {
      id: user.userId,
      aud: 'authenticated',
      role: 'authenticated',
      email: user.email,
      email_confirmed_at: new Date().toISOString(),
      app_metadata: { provider: 'email', providers: ['email'] },
      user_metadata: {},
      created_at: new Date().toISOString(),
      updated_at: new Date().toISOString(),
    };
    const session = {
      access_token: user.token,
      token_type: 'bearer',
      expires_in: 3600,
      expires_at: now + 3600,
      refresh_token: user.refresh,
      user: record,
    };
    const url = route.request().url();
    const body = url.includes('/token') ? JSON.stringify(session) : JSON.stringify(record);
    await route.fulfill({ status: 200, contentType: 'application/json', body });
  });
}
