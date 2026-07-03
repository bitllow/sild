import { test, expect } from "../../fixtures";
import { ADMIN } from "../../support/env";

// The login UI must be exercised logged-OUT, so override the project's stored
// admin session for this file.
test.use({ storageState: { cookies: [], origins: [] } });

test.describe("inbox auth", () => {
  test("unauthenticated visit shows the login screen", async ({ page }) => {
    await page.goto("/");
    await expect(page.getByRole("heading", { name: "Sild support inbox" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Sign in" })).toBeVisible();
  });

  test("password sign-in lands on the inbox shell", async ({ page }) => {
    await page.goto("/");
    // Email is prefilled with the seeded admin; fill the password and submit.
    await expect(page.getByPlaceholder("you@company.com")).toHaveValue(ADMIN.email);
    await page.getByPlaceholder("Password").fill(ADMIN.password);
    await page.getByRole("button", { name: "Sign in" }).click();

    // Shell = nav rail with Inbox/Settings/Sign out.
    await expect(page.getByRole("button", { name: "Inbox" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Sign out" })).toBeVisible();
  });

  test("wrong password shows an error and stays on login", async ({ page }) => {
    await page.goto("/");
    await page.getByPlaceholder("Password").fill("definitely-wrong");
    await page.getByRole("button", { name: "Sign in" }).click();

    await expect(page.getByText("Invalid email or password.")).toBeVisible();
    await expect(page.getByRole("heading", { name: "Sild support inbox" })).toBeVisible();
  });

  test("Google sign-in is offered as an alternative", async ({ page }) => {
    await page.goto("/");
    await expect(page.getByRole("button", { name: /Continue with Google/ })).toBeVisible();
  });

  test("sign-out returns to the login screen", async ({ page }) => {
    // Log in first (this file is logged-out by default).
    await page.goto("/");
    await page.getByPlaceholder("Password").fill(ADMIN.password);
    await page.getByRole("button", { name: "Sign in" }).click();
    await expect(page.getByRole("button", { name: "Sign out" })).toBeVisible();

    await page.getByRole("button", { name: "Sign out" }).click();
    await expect(page.getByRole("heading", { name: "Sild support inbox" })).toBeVisible();
  });
});
