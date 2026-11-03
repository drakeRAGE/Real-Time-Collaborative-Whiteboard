import type { FullConfig } from '@playwright/test';

/**
 * Authenticated scenarios read E2E_EMAIL / E2E_PASSWORD at test time.
 * Missing credentials are a skip, not a crash. A Supabase admin seed runs
 * only when both the project URL and a service-role key are already in the
 * environment. This file never invents either value.
 */
export default async function globalSetup(_config: FullConfig): Promise<void> {
  const email = process.env.E2E_EMAIL?.trim();
  const password = process.env.E2E_PASSWORD?.trim();
  if (!email || !password) {
    console.log('global-setup: E2E_EMAIL or E2E_PASSWORD is unset. Authenticated scenarios will skip.');
    return;
  }

  const url = process.env.E2E_SUPABASE_URL?.trim();
  const serviceRole = process.env.E2E_SUPABASE_SERVICE_ROLE_KEY?.trim();
  if (!url || !serviceRole) {
    console.log('global-setup: service-role credentials are unset. Not seeding a user.');
    return;
  }

  const endpoint = `${url.replace(/\/$/, '')}/auth/v1/admin/users`;
  const res = await fetch(endpoint, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${serviceRole}`,
      apikey: serviceRole,
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({ email, password, email_confirm: true }),
  });
  if (res.ok || res.status === 422) {
    console.log('global-setup: test user is present.');
    return;
  }
  const body = await res.text();
  console.log(`global-setup: seed returned ${res.status} (${body.slice(0, 180)}). Tests will try the given credentials anyway.`);
}
