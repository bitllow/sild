import fs from "node:fs";
import { test, expect, type APIRequest, type APIRequestContext, type Browser, type Page } from "@playwright/test";
import { BACKEND_URL, INBOX_URL, uid } from "../../support/env";

// The tech-support triage loop with the assistant played over its service key,
// so the spec needs no model: the bridge's side is the API it would call.

const REQUESTER = { email: "marko@sild.local", password: "password123" } as const;

type Conv = { id: string; assignment?: { id: string; status: string } };

async function signedIn(browser: Browser, who: { email: string; password: string }): Promise<Page> {
  const context = await browser.newContext({ baseURL: INBOX_URL });
  const page = await context.newPage();
  const res = await page.request.post(`${INBOX_URL}/v1/admin/auth/password`, { data: who });
  expect(res.ok(), `${who.email} login`).toBeTruthy();
  return page;
}

async function assistant(request: APIRequest): Promise<APIRequestContext> {
  const file = process.env.SILD_E2E_TRIAGE_KEY_FILE!;
  await expect.poll(() => fs.existsSync(file) && fs.readFileSync(file, "utf8").trim().length > 0).toBeTruthy();
  const key = fs.readFileSync(file, "utf8").trim();
  return request.newContext({ baseURL: BACKEND_URL, extraHTTPHeaders: { Authorization: `Bearer ${key}` } });
}

async function queuedWith(bot: APIRequestContext, body: string): Promise<Conv> {
  let found: Conv | undefined;
  await expect
    .poll(async () => {
      const res = await bot.get("/v1/conversations?kind=support&queue=triage&assignment_status=queued&limit=100");
      for (const c of ((await res.json()) as { items: Conv[] }).items) {
        const msgs = (await (await bot.get(`/v1/conversations/${c.id}/messages`)).json()) as { items: { body: string }[] };
        if (msgs.items.some((m) => m.body === body)) found = c;
      }
      return !!found;
    }, { timeout: 15_000 })
    .toBeTruthy();
  return found!;
}

test.describe("tech support triage", () => {
  test("a requester gets a diagnosis and confirms the fix; an operator sees the data requests", async ({ browser, page, playwright }) => {
    const bot = await assistant(playwright.request);
    const marko = await signedIn(browser, REQUESTER);
    // A retry must not start behind the previous attempt's still-open request.
    const open = (await (await marko.request.get("/v1/conversations?kind=support&queue=triage&status=open")).json()) as { items: Conv[] };
    for (const c of open.items) await marko.request.post(`/v1/conversations/${c.id}/close`, { data: { reason: "closed" } });
    const problem = `Andres Kuusk gets no trips ${uid("t")}`;

    await marko.goto("/");
    await marko.getByTestId("triage-nav").click();
    await marko.getByTestId("triage-new").click();
    await marko.getByPlaceholder("e.g. A driver hasn't received new trips since 8:00").fill(problem);
    const log = `${uid("alerts")}.csv`;
    await marko.locator('input[type="file"]').setInputFiles({ name: log, mimeType: "text/csv", buffer: Buffer.from("ts,event\n08:01,push_token_invalid\n") });
    await expect(marko.getByText(log)).toBeVisible();
    await marko.getByTestId("triage-send").click();

    const conv = await queuedWith(bot, problem);
    expect((await bot.patch(`/v1/assignments/${conv.assignment!.id}`, { data: { assignee_actor_id: "me" } })).status()).toBe(200);
    const sent = (await (await bot.get(`/v1/conversations/${conv.id}/messages`)).json()) as { items: { attachments: { filename: string; url?: string }[] }[] };
    expect(sent.items.flatMap((m) => m.attachments).map((a) => [a.filename, !!a.url]), "the assistant can download the requester's file").toEqual([[log, true]]);

    const step = { label: "Checked his phone registration", internal: { source: "driver.devices", params: { driver_id: "drv_2207" } } };
    const steps = await bot.post(`/v1/conversations/${conv.id}/messages`, {
      data: { body: "Looking into it", parts: [{ type: "steps", steps: [{ ...step, state: "running" }] }] },
    });
    expect(steps.status()).toBe(201);
    await expect(marko.getByTestId("triage-steps")).toContainText("Checked his phone registration");

    const { id: stepsId } = (await steps.json()) as { id: string };
    const done = await bot.patch(`/v1/conversations/${conv.id}/messages/${stepsId}`, {
      data: { parts: [{ type: "steps", steps: [{ ...step, state: "done", result: "Token expired 07:42" }] }] },
    });
    expect(done.status()).toBe(200);
    await expect(marko.getByTestId("triage-steps")).toContainText("Token expired 07:42");
    await expect(marko.getByTestId("triage-requests-toggle"), "the requester never gets internal fields").toHaveCount(0);

    expect(
      (
        await bot.post(`/v1/conversations/${conv.id}/messages`, {
          data: {
            body: "His phone stopped accepting notifications.",
            parts: [{
              type: "diagnosis", cause: "The push token expired at 07:42 after app 5.12.0.", fix: ["Ask Andres to sign in again."],
              evidence: [{ finding: "His token stopped working at 07:42", source: "his phone registration", internal: { ref: "data/req_devices/1.json" } }],
            }],
          },
        })
      ).status(),
    ).toBe(201);
    await expect(marko.getByTestId("triage-diagnosis")).toContainText("Ask Andres to sign in again.");
    await marko.getByTestId("triage-evidence").locator("summary").click();
    await expect(marko.getByTestId("triage-evidence")).toContainText("His token stopped working at 07:42");
    await expect(marko.getByTestId("triage-evidence"), "the requester never gets evidence references").not.toContainText("req_devices");

    // The operator (the suite's signed-in owner) watches every chat and sees the data requests.
    await page.goto("/");
    await page.getByTestId("triage-nav").click();
    await page.locator(`[data-testid="triage-row"][data-conversation="${conv.id}"]`).click();
    await page.getByTestId("triage-requests-toggle").click();
    await expect(page.getByTestId("triage-steps")).toContainText("driver.devices");
    await page.getByTestId("triage-evidence").locator("summary").click();
    await expect(page.getByTestId("triage-evidence")).toContainText("data/req_devices/1.json");

    await marko.getByTestId("triage-resolve").click();
    await expect
      .poll(async () => ((await (await bot.get(`/v1/conversations/${conv.id}`)).json()) as { close_reason?: string }).close_reason)
      .toBe("resolved");
  });
});
