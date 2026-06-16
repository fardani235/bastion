// Tiny client-side id generator for tab keys (not security-sensitive).
export function nanoid(): string {
  return Math.random().toString(36).slice(2) + Date.now().toString(36)
}
