// Fleetline: a dev-only host app that embeds the Sild inbox the way an
// integrator would. It signs tokens shaped like the reference integrator's
// (ES256; claims i = account id, n = namespace, r = roles; no sub/iss/aud),
// hands them to the iframe over postMessage, and answers Sild's validator calls
// for them (docs/embedded-sign-in.md). Never deploy it.

import { createServer, type IncomingMessage } from "node:http";
import { createHash, generateKeyPairSync, sign, verify } from "node:crypto";
import { challenge, check, credentialIds, register, type Ceremony } from "./passkeys.ts";

const PORT = Number(process.env.PORT ?? 4400);
// Unset: the inbox on port 3000 of whatever host this page was opened on, so a phone on the LAN works.
const INBOX_URL = process.env.INBOX_URL?.replace(/\/$/, "") || undefined;
const inboxUrl = (req: IncomingMessage) => INBOX_URL ?? `http://${new URL(`http://${req.headers.host ?? "localhost"}`).hostname}:3000`;
const EMBED_KEY = process.env.EMBED_KEY ?? "emb_dev_fleetline";
const TOKEN_TTL = Number(process.env.TOKEN_TTL ?? 3600);
// Matches devValidatorToken in backend/cmd/sild-dev/embed.go.
const VALIDATOR_TOKEN = process.env.FAKE_VALIDATOR_TOKEN ?? "fleetline-dev-validator-token";
const NAMESPACE = "fleetline";
const STAFF_ROLES = ["admin", "taxiCompanyAdmin", "dispatcher"];
const END_USER_NAMESPACES = ["driver", "client"];

interface Persona {
  label: string;
  name: string;
  title: string;
  i: number;
  r: string[];
}

// Linked by sild-dev's SILD_DEV_TRIAGE seed: fleetline.2 → admin@sild.local,
// fleetline.1 → liis@sild.local, fleetline.7 → marko@sild.local.
const PERSONAS: Record<string, Persona> = {
  admin: { label: "Admin", name: "Eva Marleen", title: "Operations admin", i: 2, r: ["taxiCompanyAdmin"] },
  agent: { label: "Support agent", name: "Liis Mägi", title: "Support agent", i: 1, r: ["admin"] },
  user: { label: "Standard user", name: "Marko Saar", title: "Dispatcher", i: 7, r: ["dispatcher"] },
  none: { label: "Not provisioned", name: "Peeter Ots", title: "Dispatcher", i: 40, r: ["dispatcher"] },
  driver: { label: "Driver", name: "Toomas Vaher", title: "Driver", i: 4411, r: ["driver"] },
};

// A fresh key each start: only this process signs and verifies its tokens.
const { privateKey, publicKey } = generateKeyPairSync("ec", { namedCurve: "P-256" });

const b64url = (v: string | Buffer) => Buffer.from(v).toString("base64url");

// Data access approvals, as a passkey step-up records them: "<sub>:<conversation>" → expiry (ms).
// The fake adapter asks GET /elevation, standing in for an adapter reading the integrator's own store.
const STEP_UP_TTL_MS = Number(process.env.STEP_UP_TTL ?? 900) * 1000;
const elevations = new Map<string, number>();

function mint(p: Persona): { token: string; claims: Record<string, unknown> } {
  const iat = Math.floor(Date.now() / 1000);
  const claims = { i: p.i, n: NAMESPACE, r: p.r, t: "access", iat, exp: iat + TOKEN_TTL };
  const input = `${b64url(JSON.stringify({ alg: "ES256", typ: "JWT" }))}.${b64url(JSON.stringify(claims))}`;
  const sig = sign("sha256", Buffer.from(input), { key: privateKey, dsaEncoding: "ieee-p1363" });
  return { token: `${input}.${b64url(sig)}`, claims };
}

interface Claims {
  i?: unknown;
  n?: unknown;
  r?: unknown;
  exp?: unknown;
}

// verifyToken returns the claims of a token this process signed, or null.
function verifyToken(token: string): Claims | null {
  const parts = token.split(".");
  if (parts.length !== 3) return null;
  try {
    const header = JSON.parse(Buffer.from(parts[0], "base64url").toString());
    if (header.alg !== "ES256") return null;
    const ok = verify(
      "sha256",
      Buffer.from(`${parts[0]}.${parts[1]}`),
      { key: publicKey, dsaEncoding: "ieee-p1363" },
      Buffer.from(parts[2], "base64url"),
    );
    return ok ? (JSON.parse(Buffer.from(parts[1], "base64url").toString()) as Claims) : null;
  } catch {
    return null;
  }
}

// validate answers Sild's validator call for one token (docs/embedded-sign-in.md).
function validate(token: string): Record<string, unknown> {
  const claims = verifyToken(token);
  if (!claims || typeof claims.n !== "string" || typeof claims.i !== "number") {
    return { active: false, reason: "invalid_token" };
  }
  const sub = `${claims.n}.${claims.i}`;
  const now = Math.floor(Date.now() / 1000);
  if (typeof claims.exp !== "number" || claims.exp <= now) return { active: false, reason: "token_expired", sub };
  const roles = Array.isArray(claims.r) ? claims.r : [];
  if (END_USER_NAMESPACES.includes(claims.n) || !roles.some((r) => STAFF_ROLES.includes(r))) {
    return { active: false, reason: "role_not_allowed", sub };
  }
  const persona = Object.values(PERSONAS).find((p) => p.i === claims.i);
  return { active: true, sub, exp: claims.exp, ...(persona && { name: persona.name }) };
}

async function readJSON(req: IncomingMessage): Promise<unknown> {
  let raw = "";
  for await (const chunk of req) {
    raw += chunk;
    if (raw.length > 64 * 1024) throw new Error("body too large");
  }
  return JSON.parse(raw);
}

createServer(async (req, res) => {
  const url = new URL(req.url ?? "/", `http://localhost:${PORT}`);
  const json = (status: number, body: unknown) => {
    res.writeHead(status, { "Content-Type": "application/json" });
    res.end(JSON.stringify(body, null, 2));
  };
  switch (url.pathname) {
    case "/":
      res.writeHead(200, { "Content-Type": "text/html; charset=utf-8" });
      res.end(page(inboxUrl(req)));
      return;
    case "/token": {
      const persona = PERSONAS[url.searchParams.get("user") ?? "admin"];
      if (!persona) return json(404, { error: `unknown user; one of ${Object.keys(PERSONAS).join(", ")}` });
      return json(200, { ...mint(persona), sub: `${NAMESPACE}.${persona.i}`, persona: persona.name });
    }
    case "/passkey/register-options":
    case "/passkey/register":
    case "/passkey/options":
    case "/step-up": {
      if (req.method !== "POST") return json(405, { error: "POST only" });
      const body = ((await readJSON(req).catch(() => null)) ?? {}) as Record<string, unknown>;
      const persona = typeof body.user === "string" ? PERSONAS[body.user] : undefined;
      if (!persona) return json(400, { error: "user is required" });
      const sub = `${NAMESPACE}.${persona.i}`;
      // Plain http is a secure context only on localhost, the one place passkeys work here.
      const host = new URL(`http://${req.headers.host ?? "localhost"}`);
      const c: Ceremony = { origin: host.origin, rpId: host.hostname };
      if (url.pathname === "/passkey/register-options") {
        return json(200, { challenge: challenge(`reg:${sub}`), userId: createHash("sha256").update(sub).digest("base64url"), userName: sub, displayName: persona.name });
      }
      if (url.pathname === "/passkey/register") {
        const why = register(sub, c, body);
        if (why) return json(400, { error: why });
        console.log(`passkey registered for ${sub}`);
        return json(200, { ok: true });
      }
      if (typeof body.conversation !== "string") return json(400, { error: "conversation is required" });
      const purpose = `auth:${sub}:${body.conversation}`;
      if (url.pathname === "/passkey/options") return json(200, { challenge: challenge(purpose), credentials: credentialIds(sub) });
      // Someone with a passkey must use it; without one the page falls back to a simulated prompt.
      if (credentialIds(sub).length) {
        const why = check(sub, purpose, c, body);
        if (why) return json(403, { error: why });
      }
      elevations.set(`${sub}:${body.conversation}`, Date.now() + STEP_UP_TTL_MS);
      console.log(`step-up → ${sub} approved data access in ${body.conversation} for ${STEP_UP_TTL_MS / 1000}s (${credentialIds(sub).length ? "passkey" : "simulated"})`);
      return json(200, { ok: true });
    }
    case "/elevation": {
      const key = `${url.searchParams.get("user")}:${url.searchParams.get("conv")}`;
      const until = elevations.get(key) ?? 0;
      return json(200, { elevated: until > Date.now(), until: Math.floor(until / 1000) });
    }
    case "/validate": {
      if (req.method !== "POST") return json(405, { error: "POST only" });
      if (req.headers.authorization !== `Bearer ${VALIDATOR_TOKEN}`) return json(401, { error: "bad validator token" });
      let body: unknown;
      try {
        body = await readJSON(req);
      } catch {
        return json(400, { error: "body must be JSON" });
      }
      const token = (body as { token?: unknown } | null)?.token;
      if (typeof token !== "string" || !token) return json(400, { error: "token is required" });
      const verdict = validate(token);
      console.log(`validate → ${JSON.stringify(verdict)}`);
      return json(200, verdict);
    }
  }
  json(404, { error: "not found" });
}).listen(PORT, () => {
  const origin = `http://localhost:${PORT}`;
  console.log(`Fleetline (fake integrator) on ${origin} — embeds ${INBOX_URL ?? "the inbox on this host's port 3000"}/embed?key=${EMBED_KEY}`);
  console.log(`Validator: POST ${origin}/validate, Authorization: Bearer ${VALIDATOR_TOKEN}`);
  console.log(`\nsild-dev seeds this validator, the embed key and the member links with SILD_DEV_TRIAGE=1.`);
  console.log(`Configure a tenant by hand instead (cd backend):`);
  console.log(
    `  go run ./cmd/sild-admin identity set --tenant <TENANT_ID> --validator-url ${origin}/validate \\\n` +
      `    --validator-token-file <(printf %s ${VALIDATOR_TOKEN}) --origins ${origin}`,
  );
  console.log(`  go run ./cmd/sild-admin agent link --tenant <TENANT_ID> --email marko@sild.local --sub ${NAMESPACE}.7`);
  console.log(`  (then restart this server with EMBED_KEY=<printed embed_key>)`);
});

function page(inbox: string): string {
  const personas = JSON.stringify(
    Object.fromEntries(Object.entries(PERSONAS).map(([id, p]) => [id, { ...p, sub: `${NAMESPACE}.${p.i}` }])),
  );
  const src = `${inbox}/embed?key=${encodeURIComponent(EMBED_KEY)}`;
  return `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Fleetline · Support</title>
<style>
  *{box-sizing:border-box} html,body{margin:0;height:100%}
  body{font-family:Inter,system-ui,-apple-system,sans-serif;color:#0f172a;background:#f8fafc;-webkit-font-smoothing:antialiased}
  .app{display:flex;flex-direction:column;height:100vh}
  .bar{flex:none;height:44px;display:flex;align-items:center;gap:12px;padding:0 16px;background:#0f172a;color:#cbd5e1;font-size:12px}
  .bar b{color:#fff}
  .switch{display:flex;gap:2px;background:rgba(255,255,255,.08);padding:3px;border-radius:8px}
  .switch button{height:28px;padding:0 10px;border:0;border-radius:6px;cursor:pointer;font:600 12px inherit;font-family:inherit;background:transparent;color:#cbd5e1}
  .switch button[aria-pressed=true]{background:#fff;color:#0f172a}
  .hint{margin-left:auto;font-family:ui-monospace,Menlo,monospace;color:#94a3b8}
  .body{flex:1;min-height:0;display:flex}
  aside{width:224px;flex:none;background:#fff;border-right:1px solid #e2e8f0;display:flex;flex-direction:column;padding:14px 10px}
  .brand{display:flex;align-items:center;gap:10px;padding:4px 8px 18px}
  .logo{width:28px;height:28px;border-radius:7px;background:#1e293b;color:#fff;display:flex;align-items:center;justify-content:center;font-weight:700;font-size:14px}
  nav a{display:flex;align-items:center;height:36px;padding:0 12px;border-radius:8px;font-size:14px;color:#475569;text-decoration:none}
  nav a.on{color:#0f172a;font-weight:600;background:#f1f5f9}
  .me{margin-top:auto;display:flex;align-items:center;gap:10px;padding:10px 8px;border-top:1px solid #f1f5f9}
  .avatar{width:32px;height:32px;border-radius:50%;background:#e0e7ff;color:#3730a3;display:flex;align-items:center;justify-content:center;font-weight:600;font-size:12px}
  main{flex:1;min-width:0;display:flex}
  iframe{flex:1;border:0;width:100%;height:100%}
  .veil{position:fixed;inset:0;background:rgba(15,23,42,.45);display:flex;align-items:center;justify-content:center}
  .sheet{width:360px;background:#fff;border-radius:14px;padding:20px;box-shadow:0 20px 50px rgba(0,0,0,.25);display:flex;flex-direction:column;gap:12px;font-size:14px}
  .sheet button{height:36px;border-radius:8px;border:1px solid #e2e8f0;background:#fff;font:600 13px inherit;font-family:inherit;cursor:pointer}
  .sheet button.go{background:#0f172a;color:#fff;border-color:#0f172a}
</style>
</head>
<body>
<div class="app">
  <div class="bar">
    <b>Prototype</b><span>Signed into Fleetline as</span>
    <div class="switch" id="switch"></div>
    <span class="hint" id="hint"></span>
  </div>
  <div class="body">
    <aside>
      <div class="brand"><span class="logo">F</span><b style="font-size:15px">Fleetline</b><span style="font-size:11px;color:#94a3b8;margin-left:auto">Admin</span></div>
      <nav>${["Dashboard", "Dispatch", "Drivers", "Vehicles", "Billing", "Support", "Settings"]
        .map((l) => `<a href="#" class="${l === "Support" ? "on" : ""}">${l}</a>`)
        .join("")}</nav>
      <div class="me"><span class="avatar" id="avatar"></span><div><div style="font-size:13px;font-weight:600" id="name"></div><div style="font-size:12px;color:#94a3b8" id="title"></div></div></div>
    </aside>
    <main><iframe id="sild" title="Support" src="${src}"></iframe></main>
  </div>
</div>
<script>
  const INBOX_ORIGIN = ${JSON.stringify(new URL(inbox).origin)};
  const SRC = ${JSON.stringify(src)};
  const PERSONAS = ${personas};
  const frame = document.getElementById("sild");
  let current = sessionStorage.getItem("fleetline_user") || "admin";

  function render() {
    const me = PERSONAS[current];
    document.getElementById("switch").innerHTML = Object.entries(PERSONAS)
      .map(([id, p]) => '<button data-id="' + id + '" aria-pressed="' + (id === current) + '">' + p.label + "</button>").join("");
    document.getElementById("hint").textContent = "sub=" + me.sub + " · r=" + me.r.join(",") + " · grants resolved by Sild";
    document.getElementById("name").textContent = me.name;
    document.getElementById("title").textContent = me.title;
    document.getElementById("avatar").textContent = me.name.split(" ").map((w) => w[0]).join("");
  }

  async function sendToken() {
    const r = await fetch("/token?user=" + current);
    const { token } = await r.json();
    // Exact target origin: the token never reaches a frame that navigated elsewhere.
    frame.contentWindow.postMessage({ type: "sild:token", token, host: "Fleetline" }, INBOX_ORIGIN);
  }

  window.addEventListener("message", (e) => {
    if (e.origin !== INBOX_ORIGIN || e.source !== frame.contentWindow) return;
    if (e.data && (e.data.type === "sild:ready" || e.data.type === "sild:token-request")) void sendToken();
    if (e.data && e.data.type === "sild:step-up" && typeof e.data.conversation === "string") stepUp(e.data);
  });

  // Passkeys need a secure context: https, or plain http on localhost.
  const PASSKEYS = window.isSecureContext && !!window.PublicKeyCredential;
  const b64u = (buf) => btoa(String.fromCharCode(...new Uint8Array(buf))).replaceAll("+", "-").replaceAll("/", "_").replaceAll("=", "");
  const fromB64u = (s) => Uint8Array.from(atob(s.replaceAll("-", "+").replaceAll("_", "/")), (ch) => ch.charCodeAt(0));
  const post = async (path, body) => (await fetch(path, { method: "POST", body: JSON.stringify({ user: current, ...body }) })).json();

  async function createPasskey() {
    const o = await post("/passkey/register-options", {});
    const cred = await navigator.credentials.create({ publicKey: {
      challenge: fromB64u(o.challenge), rp: { id: location.hostname, name: "Fleetline (dev)" },
      user: { id: fromB64u(o.userId), name: o.userName, displayName: o.displayName },
      pubKeyCredParams: [{ type: "public-key", alg: -7 }],
      authenticatorSelection: { residentKey: "preferred", userVerification: "required" }, attestation: "none", timeout: 60000,
    } });
    const r = cred.response;
    const done = await post("/passkey/register", {
      id: cred.id, alg: r.getPublicKeyAlgorithm(), publicKey: b64u(r.getPublicKey()),
      clientDataJSON: b64u(r.clientDataJSON), authenticatorData: b64u(r.getAuthenticatorData()),
    });
    if (!done.ok) throw new Error(done.error);
  }

  async function usePasskey(conversation) {
    let o = await post("/passkey/options", { conversation });
    if (!o.credentials.length) {
      await createPasskey();
      o = await post("/passkey/options", { conversation });
    }
    const a = await navigator.credentials.get({ publicKey: {
      challenge: fromB64u(o.challenge), rpId: location.hostname, userVerification: "required", timeout: 60000,
      allowCredentials: o.credentials.map((id) => ({ type: "public-key", id: fromB64u(id) })),
    } });
    const r = a.response;
    const done = await post("/step-up", {
      conversation, id: a.id, clientDataJSON: b64u(r.clientDataJSON), authenticatorData: b64u(r.authenticatorData), signature: b64u(r.signature),
    });
    if (!done.ok) throw new Error(done.error);
    return true;
  }

  function stepUp({ conversation, reason }) {
    const veil = document.createElement("div");
    veil.className = "veil";
    veil.innerHTML = '<div class="sheet"><b>Confirm with your passkey</b>' +
      '<span style="color:#475569">' + (PASSKEYS
        ? "Fleetline asks for your passkey (the first time, it creates one). The support assistant asks to check:"
        : "Simulated: passkeys need https or localhost. The support assistant asks to check:") + "</span>" +
      '<span id="why" style="font-size:13px"></span>' +
      '<div style="display:flex;gap:8px;justify-content:flex-end"><button id="no">Cancel</button><button class="go" id="yes">Use passkey</button></div></div>';
    veil.querySelector("#why").textContent = String(reason || "");
    document.body.appendChild(veil);
    const answer = async (ok) => {
      veil.remove();
      if (ok && PASSKEYS) ok = await usePasskey(conversation).catch((e) => (console.warn("passkey:", e), false));
      else if (ok) ok = (await post("/step-up", { conversation })).ok === true;
      frame.contentWindow.postMessage({ type: "sild:step-up-done", conversation, ok }, INBOX_ORIGIN);
    };
    veil.querySelector("#yes").addEventListener("click", () => void answer(true));
    veil.querySelector("#no").addEventListener("click", () => void answer(false));
  }

  document.getElementById("switch").addEventListener("click", (e) => {
    const id = e.target.closest("button")?.dataset.id;
    if (!id || id === current) return;
    current = id;
    sessionStorage.setItem("fleetline_user", id);
    render();
    frame.src = SRC; // a different person signs in from a clean frame
  });

  render();
</script>
</body>
</html>`;
}
