import { test as setup, expect } from "@playwright/test";
import { ADMIN, INBOX_URL } from "../support/env";
import fs from "node:fs";
import path from "node:path";

const AUTH_FILE = path.resolve(__dirname, "../.auth/agent.json");

// Log the seeded admin in ONCE and persist the session cookie. Logging in
// through the inbox origin (:3000, which proxies /v1/* to the backend) is
// essential: the HttpOnly sild_admin cookie must be first-party to :3000, since
// that is the only origin the browser talks to.
setup("authenticate admin", async ({ page }) => {
  const res = await page.request.post(`${INBOX_URL}/v1/admin/auth/password`, {
    data: { email: ADMIN.email, password: ADMIN.password },
  });
  expect(res.ok(), "admin password login").toBeTruthy();

  fs.mkdirSync(path.dirname(AUTH_FILE), { recursive: true });
  await page.context().storageState({ path: AUTH_FILE });

  // Sanity: the saved state must carry the session cookie or every inbox spec
  // would silently fall back to the login screen.
  const state = JSON.parse(fs.readFileSync(AUTH_FILE, "utf8")) as {
    cookies: { name: string }[];
  };
  expect(state.cookies.some((c) => c.name === "sild_admin"), "sild_admin cookie present").toBeTruthy();
});
