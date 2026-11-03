const ENCODING = '0123456789ABCDEFGHJKMNPQRSTVWXYZ';

/** Crockford ULID. The first character stays in 0–7 for current timestamps. */
export function ulid(now = Date.now(), random: () => number = Math.random): string {
  let time = now;
  const chars = new Array<string>(26);
  for (let i = 9; i >= 0; i--) {
    chars[i] = ENCODING[time % 32] ?? '0';
    time = Math.floor(time / 32);
  }
  for (let i = 10; i < 26; i++) {
    const index = Math.min(31, Math.floor(random() * 32));
    chars[i] = ENCODING[index] ?? '0';
  }
  return chars.join('');
}
