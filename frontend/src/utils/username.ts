export function usernameFromEmail(email: string | null | undefined): string {
  if (!email) return 'User';
  const name = email.split('@')[0] ?? 'User';
  return name.replace(/\.(com|net|org|xyz|io|co)$/i, '') || 'User';
}
