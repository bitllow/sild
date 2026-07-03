import { test, expect } from "../../fixtures";
import { uid } from "../../support/env";
import { gotoInbox, searchInput, rowByText, allRows } from "../../support/inbox";

// Exhaustive search coverage (§4.3). Each field gets its OWN unique token that
// appears nowhere else, so a hit proves that specific field matched. We assert
// both the target is found (partial + exact) and a distractor is excluded.
test.describe("inbox search", () => {
  test("finds by message keyword and participant metadata (partial + exact)", async ({ page, app }) => {
    // Distinct, non-overlapping tokens per field.
    const bodyTok = `zorderref${uid("x").replace(/[^a-z0-9]/g, "")}`;
    const nameTok = `wonderland${uid("x").replace(/[^a-z0-9]/g, "")}`;
    const emailTok = `qmailbox${uid("x").replace(/[^a-z0-9]/g, "")}`;
    const phoneTok = `9${Date.now().toString().slice(-8)}`; // 9 digits, unique

    const target = await app.conversation({
      body: `hello ${bodyTok} please help`,
      metadata: {
        name: `Agent ${nameTok}`,
        email: `${emailTok}@example.com`,
        phone: `+372 ${phoneTok}`,
      },
    });
    // A distractor conversation that must NOT match the target's tokens.
    const distractor = await app.conversation({ body: `unrelated chatter ${uid("d")}` });

    await gotoInbox(page);

    const search = searchInput(page);
    // Helper: run a query and assert target present, distractor absent.
    const expectFinds = async (query: string) => {
      await search.fill(query);
      await expect(rowByText(page, target.body)).toBeVisible();
      await expect(rowByText(page, distractor.body)).toHaveCount(0);
    };

    // 1) Keyword in the message body — exact then partial.
    await expectFinds(bodyTok);
    await expectFinds(bodyTok.slice(2, 9));

    // 2) Participant name — exact then partial.
    await expectFinds(nameTok);
    await expectFinds(nameTok.slice(0, 8));

    // 3) Participant email — exact then partial.
    await expectFinds(emailTok);
    await expectFinds(emailTok.slice(1, 7));

    // 4) Participant phone — exact then partial (last 5 digits).
    await expectFinds(phoneTok);
    await expectFinds(phoneTok.slice(-5));
  });

  test("clearing the search restores the queue", async ({ page, app }) => {
    const c = await app.conversation({ body: `needle ${uid("n")}` });
    await gotoInbox(page);
    await searchInput(page).fill(c.body);
    await expect(rowByText(page, c.body)).toBeVisible();

    await page.getByRole("button", { name: "Clear search" }).click();
    await expect(searchInput(page)).toHaveValue("");
    await expect(allRows(page).first()).toBeVisible();
  });

  test("no matches shows an empty state", async ({ page }) => {
    await gotoInbox(page);
    await searchInput(page).fill("zzz-no-such-conversation-xyz");
    await expect(page.getByText(/No matches for/)).toBeVisible();
  });
});
