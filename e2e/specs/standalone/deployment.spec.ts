import { test, expect } from "@playwright/test";
import { BACKEND_URL, uid } from "../../support/env";
import { BOOTSTRAP, createTenant, runAdmin } from "../../support/standalone";

// Covers the DEPLOYMENT, not the product: one process on one port, a production
// posture, a way in on a fresh database, and a CLI that reaches the same store.
// The product behaviour on top is covered by the inbox/widget/cross projects.

test.describe("one listener", () => {
  test("serves REST, health and the realtime transport on the same port", async ({ request }) => {
    await expect((await request.get(`${BACKEND_URL}/healthz`)).status()).toBe(200);
    await expect((await request.get(`${BACKEND_URL}/readyz`)).status()).toBe(200);

    // The property that makes one container enough: /v1/ws and /v1/ws/sse are
    // mounted on the REST listener rather than on a second one. A Centrifuge
    // transport rejects a bare GET, so anything but 404 proves the route exists.
    for (const path of ["/v1/ws", "/v1/ws/sse"]) {
      const res = await request.get(`${BACKEND_URL}${path}`);
      expect(res.status(), `${path} is mounted on this listener`).not.toBe(404);
    }

    // The drop-in bundle is served from the same origin too (§9), so a host page
    // needs no second deployment.
    const widget = await request.get(`${BACKEND_URL}/widget.js`);
    expect(widget.status()).toBe(200);
    expect(widget.headers()["content-type"]).toContain("javascript");
  });
});

test.describe("production posture", () => {
  test("serves none of the dev surfaces", async ({ request }) => {
    // Each is a real hole if it ships: a login needing no credentials, a token
    // mint needing no API key, tenant ids for the asking.
    for (const path of [
      "/sild-demo",
      "/v1/dev/app-id",
      "/v1/dev/widget-token",
      "/v1/dev/peer-conversation",
      "/v1/admin/auth/google/dev",
      "/docs",
    ]) {
      const res = await request.get(`${BACKEND_URL}${path}`, { maxRedirects: 0 });
      expect(res.status(), `${path} must not be served`).toBe(404);
    }
  });
});

test.describe("bootstrap", () => {
  test("left a tenant whose owner can sign in and use the API", async ({ request }) => {
    const login = await request.post(`${BACKEND_URL}/v1/admin/auth/password`, {
      data: { email: BOOTSTRAP.email, password: BOOTSTRAP.password },
    });
    expect(login.ok(), "bootstrap owner can sign in").toBeTruthy();

    // Secure means the cookie never travels over plain HTTP, which is why the
    // request below carries it explicitly — this suite runs without TLS.
    const setCookie = login.headers()["set-cookie"] ?? "";
    expect(setCookie, "session cookie is issued").toContain("sild_admin=");
    expect(setCookie).toContain("HttpOnly");
    expect(setCookie).toContain("Secure");
    const session = /sild_admin=([^;]+)/.exec(setCookie)?.[1] as string;

    // The session is real, not just a 200: it reaches a tenant-scoped surface.
    const me = await request.get(`${BACKEND_URL}/v1/principal`, {
      headers: { Cookie: `sild_admin=${session}` },
    });
    expect(me.ok(), "session reaches an authenticated route").toBeTruthy();
    const body = (await me.json()) as {
      kind?: string;
      tenant_id?: string;
      subject?: { email?: string; roles?: string[] };
    };
    expect(body.kind).toBe("admin");
    expect(body.tenant_id, "the session is scoped to a tenant").toBeTruthy();
    expect(body.subject?.email).toBe(BOOTSTRAP.email);
    expect(body.subject?.roles, "the bootstrapped operator owns the tenant").toContain("owner");
  });
});

test.describe("sild-admin", () => {
  // A separate process against the same database as the serving one, so this also
  // proves the two agree on the schema sild-migrate gave them.
  test("creates a tenant whose API key works against the running server", async ({ request }) => {
    const name = uid("Acme");
    const { tenantId, apiKey, forwardingAddress } = createTenant(name, `${name}@acme.test`);
    expect(tenantId, "printed the tenant id").toBeTruthy();
    expect(apiKey, "printed the API key once").toBeTruthy();
    expect(forwardingAddress, "printed the forwarding address").toContain("@");

    // The key the CLI printed authenticates against the process already running.
    const created = await request.post(`${BACKEND_URL}/v1/conversations`, {
      headers: { Authorization: `Bearer ${apiKey}` },
      data: { reference: uid("ref"), members: [{ user_id: uid("u"), conv_role: "client" }] },
    });
    expect(created.ok(), `create conversation with the new key: ${created.status()}`).toBeTruthy();
    const conv = (await created.json()) as { id: string };

    const own = await request.get(`${BACKEND_URL}/v1/conversations/${conv.id}`, {
      headers: { Authorization: `Bearer ${apiKey}` },
    });
    expect(own.ok(), "the tenant reads its own conversation").toBeTruthy();

    // A CLI-minted key is a tenant key: another tenant's must not reach this one.
    const other = uid("Other");
    const { apiKey: otherKey } = createTenant(other, `${other}@acme.test`);
    expect(otherKey, "second tenant has its own key").toBeTruthy();
    expect(otherKey).not.toBe(apiKey);

    const leak = await request.get(`${BACKEND_URL}/v1/conversations/${conv.id}`, {
      headers: { Authorization: `Bearer ${otherKey}` },
    });
    expect(leak.ok(), `tenant ${other} reached tenant ${tenantId}'s conversation`).toBeFalsy();
    expect([403, 404]).toContain(leak.status());
  });

  test("sets a password from stdin and refuses one from the command line", async ({ request }) => {
    const name = uid("Pw");
    const email = `${name}@acme.test`;
    const tenantId = createTenant(name, email).tenantId as string;

    runAdmin(["agent", "set-password", "--tenant", tenantId, "--email", email, "--password", "-"], "from-stdin-pw\n");

    // The password the CLI read from stdin is the one that signs in.
    const login = await request.post(`${BACKEND_URL}/v1/admin/auth/password`, {
      data: { email, password: "from-stdin-pw" },
    });
    expect(login.ok(), "sign in with the stdin-set password").toBeTruthy();

    // A password in argv lands in shell history and in the process list, so the
    // only accepted value for --password is "-".
    let stderr = "";
    try {
      runAdmin(["agent", "set-password", "--tenant", tenantId, "--email", email, "--password", "hunter2"]);
      throw new Error("the CLI accepted a password on the command line");
    } catch (e) {
      stderr = String((e as { stderr?: string }).stderr ?? (e as Error).message);
    }
    expect(stderr).toContain("process list");
  });
});
