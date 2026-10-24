import type { Page } from "@playwright/test";
import { test, expect } from "../../fixtures";
import { gotoInbox, gotoSettings } from "../../support/inbox";
import { uid } from "../../support/env";

test.describe.configure({ mode: "serial" });

// The dev seed (SILD_DEV_TRIAGE) wires embedded sign-in to tools/fake-integrator.
const VALIDATOR_URL = "http://localhost:4400/validate";

async function invite(page: Page, email: string) {
  await page.getByTestId("invite-member").click();
  await page.getByTestId("invite-email").fill(email);
  await page.getByTestId("invite-role-agent").click();
  await page.getByTestId("invite-send").click();
  await page.getByRole("button", { name: "Done" }).click();
  return page.getByTestId("team-row").filter({ hasText: email });
}

test.describe("settings · embedding", () => {
  test("the config saves whole and the validator token is never echoed", async ({ page }) => {
    await gotoInbox(page);
    await gotoSettings(page, "Embedding");

    await expect(page.getByTestId("embedding-status")).toHaveText("On");
    const key = (await page.getByTestId("embed-key").textContent()) ?? "";
    expect(key).toMatch(/^emb_/);
    await expect(page.getByTestId("embed-snippet")).toContainText(`/embed?key=${key}`);

    const origins = page.getByTestId("embedding-origins");
    const original = await origins.inputValue();
    const extra = "http://127.0.0.1:4499";
    await page.getByTestId("embedding-validator-url").fill(VALIDATOR_URL);
    await origins.fill(`${original}\n${extra}`);
    await expect(page.getByTestId("embedding-validator-token")).toHaveValue("");
    await expect(page.getByTestId("embedding-token-stored")).toBeVisible();
    await page.getByTestId("embedding-save").click();
    await expect(page.getByTestId("embedding-message")).toHaveText("Saved.");

    await page.reload();
    await gotoSettings(page, "Embedding");
    await expect.poll(() => page.getByTestId("embedding-origins").inputValue()).toContain(extra);
    await expect(page.getByTestId("embedding-validator-token")).toHaveValue("");
    await expect(page.getByTestId("embedding-token-stored")).toBeVisible();
    await expect(page.getByTestId("embed-key")).toHaveText(key);

    // A stored token goes only to the URL it was given for.
    await page.getByTestId("embedding-validator-url").fill("http://localhost:4400/elsewhere");
    await expect(page.getByTestId("embedding-token-stored")).toHaveCount(0);
    await page.getByTestId("embedding-save").click();
    await expect(page.getByTestId("embedding-message")).toContainText("validator_token");

    await page.reload();
    await gotoSettings(page, "Embedding");
    await page.getByTestId("embedding-origins").fill(original);
    await page.getByTestId("embedding-save").click();
    await expect(page.getByTestId("embedding-message")).toHaveText("Saved.");
  });

  test("a member is linked to their integrator identity, and unlinked", async ({ page }) => {
    await gotoInbox(page);
    await gotoSettings(page, "Team");

    const first = await invite(page, `${uid("linked")}@example.test`);
    const second = await invite(page, `${uid("other")}@example.test`);
    const sub = uid("acme");

    await expect(first.getByTestId("integrator-identity-value")).toHaveText("Not linked");
    await first.getByTestId("integrator-identity-edit").click();
    await page.getByTestId("integrator-identity-input").fill(sub);
    await page.getByTestId("integrator-identity-save").click();
    await expect(first.getByTestId("integrator-identity-value")).toHaveText(sub);

    // One identity signs in as one member.
    await second.getByTestId("integrator-identity-edit").click();
    await page.getByTestId("integrator-identity-input").fill(sub);
    await page.getByTestId("integrator-identity-save").click();
    await expect(page.getByRole("dialog")).toContainText("already linked");
    await page.getByRole("button", { name: "Cancel" }).click();
    await expect(second.getByTestId("integrator-identity-value")).toHaveText("Not linked");

    await page.reload();
    await gotoSettings(page, "Team");
    await expect(first.getByTestId("integrator-identity-value")).toHaveText(sub);
    await first.getByTestId("integrator-identity-edit").click();
    await page.getByTestId("integrator-identity-unlink").click();
    await expect(first.getByTestId("integrator-identity-value")).toHaveText("Not linked");
  });
});
