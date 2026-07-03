// Shared constants for the suite. Origins mirror playwright.config.ts.
export const INBOX_URL = process.env.SILD_E2E_INBOX_URL || "http://localhost:3000";
export const BACKEND_URL = process.env.SILD_E2E_BACKEND_URL || "http://localhost:8080";

// Seeded by the backend's first-run devSeed (backend/cmd/sild-dev/main.go).
export const ADMIN = { email: "admin@sild.local", password: "password123" } as const;

// A short unique suffix so conversations/users created by one test never collide
// with another on the shared backend DB. Not security-sensitive — just uniqueness.
export function uid(prefix = "e2e"): string {
  const rand = Math.random().toString(36).slice(2, 8);
  return `${prefix}_${Date.now().toString(36)}_${rand}`;
}
