import { expect, test, type BrowserContext, type Page } from '@playwright/test';
import { installLocalAuth, loadSeed, localPassword } from './local-auth';
import { ackErrorCode, undoEnvelope } from './proto';

const skipReason = 'No local seed and E2E_EMAIL/E2E_PASSWORD are unset, so live login, draw, chat, and undo were not run.';
const guestSkip = 'No local guest seed and E2E_GUEST_EMAIL/E2E_GUEST_PASSWORD are unset, so the non-admin undo rejection was not run.';

function credentials(): { email: string; password: string } | null {
  const email = process.env.E2E_EMAIL?.trim();
  const password = process.env.E2E_PASSWORD?.trim();
  if (email && password) return { email, password };
  const seeded = loadSeed().find((user) => user.role === 'admin');
  if (!seeded) return null;
  return { email: seeded.email, password: localPassword };
}

function guestCredentials(): { email: string; password: string } | null {
  const email = process.env.E2E_GUEST_EMAIL?.trim();
  const password = process.env.E2E_GUEST_PASSWORD?.trim();
  if (email && password) return { email, password };
  const seeded = loadSeed().find((user) => user.role === 'guest');
  if (!seeded) return null;
  return { email: seeded.email, password: localPassword };
}

async function login(page: Page, email: string, password: string): Promise<void> {
  const seeded = loadSeed();
  if (seeded.length > 0 && !process.env.E2E_EMAIL?.trim()) {
    await installLocalAuth(page, seeded);
  }
  await page.goto('/auth');
  await page.getByLabel('Email Address').fill(email);
  await page.getByLabel('Password').fill(password);
  await page.getByRole('button', { name: 'Sign In' }).click();
  await expect(page.getByRole('heading', { name: 'Collaborative Whiteboard' })).toBeVisible();
}

async function createRoom(page: Page): Promise<string> {
  await page.getByRole('button', { name: 'Create New Room' }).click();
  await page.waitForURL(/\/canva\/[0-9a-f-]{36}/i);
  const match = page.url().match(/\/canva\/([0-9a-f-]{36})/i);
  if (!match?.[1]) throw new Error('room id missing from the canvas URL');
  return match[1];
}

async function ink(page: Page): Promise<number> {
  return page.locator('canvas').evaluate((canvas: HTMLCanvasElement) => {
    const ctx = canvas.getContext('2d');
    if (!ctx || canvas.width === 0 || canvas.height === 0) return 0;
    const { data } = ctx.getImageData(0, 0, canvas.width, canvas.height);
    let n = 0;
    for (let i = 0; i < data.length; i += 16) {
      const r = data[i] ?? 255;
      const g = data[i + 1] ?? 255;
      const b = data[i + 2] ?? 255;
      if (r < 250 || g < 250 || b < 250) n += 1;
    }
    return n;
  });
}

async function drawStroke(page: Page, index: number): Promise<void> {
  const box = await page.locator('canvas').boundingBox();
  if (!box) throw new Error('canvas has no box');
  const y = box.y + 30 + index * 8;
  await page.mouse.move(box.x + 24, y);
  await page.mouse.down();
  await page.mouse.move(box.x + Math.min(box.width - 10, 220), y + 6, { steps: 6 });
  await page.mouse.up();
}

async function openChat(page: Page): Promise<void> {
  const open = page.getByRole('button', { name: 'Open sidebar' });
  if (await open.isVisible()) await open.click();
  await page.getByRole('button', { name: 'Chat' }).click();
}

/** Room state has arrived. The sidebar is closed so later strokes hit the canvas. */
async function waitUntilJoined(page: Page): Promise<void> {
  const open = page.getByRole('button', { name: 'Open sidebar' });
  if (await open.isVisible()) await open.click();
  await expect(page.getByText('No users online')).toHaveCount(0);
  const close = page.getByRole('button', { name: 'Close sidebar' });
  if (await close.isVisible()) await close.click();
}

async function waitForStoredOps(roomId: string, token: string): Promise<void> {
  await expect.poll(async () => {
    const res = await fetch(`http://localhost:8080/api/v1/rooms/${roomId}/board/history`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    if (!res.ok) return 0;
    const body = await res.json() as { ops?: unknown[] };
    return body.ops?.length ?? 0;
  }, { timeout: 10_000 }).toBeGreaterThan(0);
}

test.describe('authenticated collaboration', () => {
  test('login, create room, draw, reload, strokes persist', async ({ page }) => {
    const creds = credentials();
    test.skip(!creds, skipReason);
    if (!creds) return;
    await login(page, creds.email, creds.password);
    const roomId = await createRoom(page);
    await expect(page.locator('canvas')).toBeVisible();
    await waitUntilJoined(page);
    for (let i = 0; i < 10; i++) await drawStroke(page, i);
    await expect.poll(() => ink(page), { timeout: 10_000 }).toBeGreaterThan(0);
    const token = loadSeed().find((user) => user.role === 'admin')?.token ?? '';
    await waitForStoredOps(roomId, token);
    const before = await ink(page);
    await page.reload();
    await expect(page.locator('canvas')).toBeVisible();
    await expect.poll(() => ink(page), { timeout: 15_000 }).toBeGreaterThan(before * 0.5);
  });

  test('two contexts sync a stroke and a chat message within 2s', async ({ browser }) => {
    const creds = credentials();
    test.skip(!creds, skipReason);
    if (!creds) return;
    const aCtx = await browser.newContext();
    const bCtx = await browser.newContext();
    const a = await aCtx.newPage();
    const b = await bCtx.newPage();
    try {
      await login(a, creds.email, creds.password);
      const roomId = await createRoom(a);
      await login(b, creds.email, creds.password);
      await b.goto(`/canva/${roomId}`);
      await expect(b.locator('canvas')).toBeVisible();
      await waitUntilJoined(a);
      await waitUntilJoined(b);
      const before = await ink(b);
      await drawStroke(a, 0);
      await expect.poll(() => ink(b), { timeout: 2_000 }).toBeGreaterThan(before);
      await openChat(a);
      await openChat(b);
      const body = `sync-${Date.now()}`;
      await b.getByRole('textbox', { name: 'Message' }).fill(body);
      await b.getByRole('button', { name: 'Send message' }).click();
      await expect(a.getByText(body)).toBeVisible({ timeout: 2_000 });
    } finally {
      await aCtx.close();
      await bCtx.close();
    }
  });

  test('admin undo removes the stroke on both contexts', async ({ browser }) => {
    const creds = credentials();
    test.skip(!creds, skipReason);
    if (!creds) return;
    const aCtx = await browser.newContext();
    const bCtx = await browser.newContext();
    const a = await aCtx.newPage();
    const b = await bCtx.newPage();
    try {
      await login(a, creds.email, creds.password);
      const roomId = await createRoom(a);
      await login(b, creds.email, creds.password);
      await b.goto(`/canva/${roomId}`);
      await waitUntilJoined(a);
      await waitUntilJoined(b);
      await expect(a.getByRole('button', { name: 'Undo' })).toBeVisible();
      await drawStroke(a, 2);
      await expect.poll(() => ink(b), { timeout: 2_000 }).toBeGreaterThan(0);
      const drawn = await ink(a);
      await a.getByRole('button', { name: 'Undo' }).click();
      await expect.poll(() => ink(a), { timeout: 2_000 }).toBeLessThan(drawn);
      await expect.poll(() => ink(b), { timeout: 2_000 }).toBeLessThan(drawn);
    } finally {
      await aCtx.close();
      await bCtx.close();
    }
  });

  test('non-admin has no undo button and a forged undo is rejected', async ({ browser }) => {
    const admin = credentials();
    const guest = guestCredentials();
    test.skip(!admin, skipReason);
    test.skip(!guest, guestSkip);
    if (!admin || !guest) return;
    const guestCtx: BrowserContext = await browser.newContext();
    await guestCtx.addInitScript(() => {
      const Native = window.WebSocket;
      const frames: Uint8Array[] = [];
      const sockets: WebSocket[] = [];
      const Wrapped = class extends Native {
        constructor(url: string | URL, protocols?: string | string[]) {
          super(url, protocols);
          this.binaryType = 'arraybuffer';
          sockets.push(this);
          this.addEventListener('message', (ev) => {
            if (ev.data instanceof ArrayBuffer) frames.push(new Uint8Array(ev.data));
            else if (ev.data instanceof Blob) {
              void ev.data.arrayBuffer().then((buf) => frames.push(new Uint8Array(buf)));
            }
          });
        }
      };
      Object.defineProperty(window, 'WebSocket', { configurable: true, writable: true, value: Wrapped });
      (window as unknown as { __wbFrames: Uint8Array[] }).__wbFrames = frames;
      (window as unknown as { __wbSockets: WebSocket[] }).__wbSockets = sockets;
    });
    const adminCtx = await browser.newContext();
    const adminPage = await adminCtx.newPage();
    const guestPage = await guestCtx.newPage();
    try {
      await login(adminPage, admin.email, admin.password);
      const roomId = await createRoom(adminPage);
      await login(guestPage, guest.email, guest.password);
      await guestPage.goto(`/canva/${roomId}`);
      await guestPage.getByRole('button', { name: 'Open sidebar' }).click();
      await expect(guestPage.getByText('Member', { exact: true })).toBeVisible();
      await expect(guestPage.getByRole('button', { name: 'Undo' })).toHaveCount(0);
      const payload = Array.from(undoEnvelope(roomId));
      const rejected = await guestPage.evaluate(async (bytes) => {
        const win = window as unknown as { __wbSockets: WebSocket[]; __wbFrames: Uint8Array[] };
        const open = win.__wbSockets.filter((item) => item.readyState === WebSocket.OPEN);
        if (open.length === 0) {
          return { error: 'no open socket', sockets: win.__wbSockets.map((item) => item.readyState) };
        }
        const before = win.__wbFrames.length;
        const bytesOut = new Uint8Array(bytes);
        for (const socket of open) socket.send(bytesOut);
        const deadline = Date.now() + 5_000;
        while (Date.now() < deadline) {
          if (win.__wbFrames.length > before) {
            return { frames: win.__wbFrames.slice(before).map((frame) => Array.from(frame)) };
          }
          await new Promise((resolve) => setTimeout(resolve, 50));
        }
        return {
          error: 'no server frame',
          before,
          after: win.__wbFrames.length,
          sockets: open.map((item) => ({ ready: item.readyState, binary: item.binaryType, buffered: item.bufferedAmount })),
        };
      }, payload);
      expect(rejected.error, JSON.stringify(rejected)).toBeUndefined();
      const frames = (rejected.frames ?? []).map((frame) => Uint8Array.from(frame));
      const codes = frames.map((frame) => ackErrorCode(frame)).filter((code) => code !== null);
      expect(codes).toContain(2);
    } finally {
      await adminCtx.close();
      await guestCtx.close();
    }
  });
});
