import { join } from 'node:path';
import type { FullConfig } from '@playwright/test';
import { seedLocalUsers } from './local-auth';

/**
 * Authenticated scenarios read a local session seeded like load/harness:
 * a row in auth.users plus an HS256 JWT the compose gateway already trusts.
 * There is no Supabase project. If the seed cannot run, those scenarios skip.
 */
export default async function globalSetup(_config: FullConfig): Promise<void> {
  if (process.env.E2E_EMAIL?.trim() && process.env.E2E_PASSWORD?.trim()) {
    console.log('global-setup: using E2E_EMAIL from the environment. Not seeding.');
    return;
  }
  const root = join(process.cwd(), '..');
  await seedLocalUsers(root);
}
