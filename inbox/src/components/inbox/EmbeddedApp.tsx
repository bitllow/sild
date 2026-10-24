"use client";

import * as React from "react";
import { observer } from "mobx-react-lite";
import { useStore } from "@/store/StoreProvider";
import { adminApi, type ApiEmbeddedSession } from "@/api/admin";
import { ApiError, setBearer } from "@/api/client";
import { setHostOrigin } from "@/api/host";
import { CheckIcon, Spinner } from "@/components/ds";
import { Shell } from "./Shell";

// The inbox framed by a host app. The host posts its own user token; only the
// tenant's allowed origins are listened to, and the session bearer that token
// buys lives in memory for the life of the frame.

interface Denied {
  reason: string;
  sub: string;
  message: string;
}

type Phase =
  | { kind: "checking"; step: number; sub?: string; tokenHint?: string }
  | { kind: "ready" }
  | { kind: "denied"; denied: Denied }
  | { kind: "error"; message: string };

class DeniedError extends Error {
  constructor(readonly denied: Denied) {
    super(denied.message);
  }
}

// Ask the host for a fresh token this long before the session ends.
const REFRESH_LEAD_MS = 60_000;
const ASK_INTERVAL_MS = 5_000;
const RETRY_MS = 15_000;
// A replaced bearer stays valid this long so requests already sent with it can finish.
const RETIRE_MS = 30_000;

// No answer, or the validator or Sild failing; a 401 or 403 is final.
function transient(e: unknown): boolean {
  return e instanceof ApiError && (e.status === 0 || e.status === 429 || e.status >= 500);
}

// Ends a session the frame is done with instead of leaving it valid until expiry.
function endSession(token: string | null) {
  if (token) adminApi.endEmbedded(token).catch(() => {});
}

async function exchange(embedKey: string, token: string): Promise<ApiEmbeddedSession> {
  try {
    return await adminApi.exchangeEmbedded(embedKey, token);
  } catch (e) {
    if (e instanceof ApiError && e.status === 403) {
      throw new DeniedError({ reason: e.fields?.reason ?? "access_denied", sub: e.fields?.sub ?? "", message: e.message });
    }
    throw e;
  }
}

async function allowedOrigins(embedKey: string): Promise<string[]> {
  try {
    return (await adminApi.embedOrigins(embedKey)).allowed_origins;
  } catch (e) {
    throw e instanceof ApiError && e.status > 0 ? new Error("This support inbox is not set up for embedding.") : e;
  }
}

export const EmbeddedApp = observer(function EmbeddedApp() {
  const store = useStore();
  const [phase, setPhase] = React.useState<Phase>({ kind: "checking", step: 0 });
  const [host, setHost] = React.useState("your app");

  React.useEffect(() => {
    const embedKey = new URLSearchParams(window.location.search).get("key") ?? "";
    if (!embedKey || window.parent === window) {
      setPhase({ kind: "error", message: "Open support from inside your app." });
      return;
    }
    let origins: string[] = [];
    let parentOrigin: string | null = null;
    let adminId: string | null = null;
    let bearer: string | null = null;
    let expiresAt = 0;
    // A throttle, not a latch: a host that missed one request is asked again.
    let askedAt = 0;
    let refresh: ReturnType<typeof setTimeout> | null = null;
    let disposed = false;
    // Only the newest token's exchange counts: validators may answer out of order.
    let latest = 0;
    const retiring = new Map<string, ReturnType<typeof setTimeout>>();
    const retire = (token: string | null) => {
      if (!token) return;
      retiring.set(token, setTimeout(() => {
        retiring.delete(token);
        endSession(token);
      }, RETIRE_MS));
    };
    const endRetiring = () => {
      for (const [token, timer] of retiring) {
        clearTimeout(timer);
        endSession(token);
      }
      retiring.clear();
    };

    const askForToken = () => {
      if (!parentOrigin || Date.now() - askedAt < ASK_INTERVAL_MS) return;
      askedAt = Date.now();
      window.parent.postMessage({ type: "sild:token-request" }, parentOrigin);
    };

    // The next token starts over: the store was disposed, so a refresh alone cannot restore it.
    const fail = (next: Phase) => {
      endSession(bearer);
      bearer = null;
      setBearer(null);
      store.dispose();
      adminId = null;
      if (refresh) clearTimeout(refresh);
      refresh = null;
      setPhase(next);
    };

    const signIn = async (token: string, origin: string) => {
      parentOrigin = origin;
      askedAt = 0;
      const first = adminId === null;
      if (first) setPhase({ kind: "checking", step: 1, tokenHint: token.slice(0, 12) });
      const seq = ++latest;
      let exchanged = false;
      try {
        const s = await exchange(embedKey, token);
        exchanged = true;
        if (disposed || seq !== latest) {
          endSession(s.token);
          return;
        }
        // Another person signed in on the host: start from a clean store.
        if (adminId !== null && s.admin_id !== adminId) {
          endSession(bearer);
          endSession(s.token);
          bearer = null;
          window.location.reload();
          return;
        }
        if (bearer !== s.token) retire(bearer);
        bearer = s.token;
        expiresAt = new Date(s.expires_at).getTime();
        setBearer(s.token, askForToken);
        setHostOrigin(origin);
        if (refresh) clearTimeout(refresh);
        const wait = Math.max(5_000, expiresAt - Date.now() - REFRESH_LEAD_MS);
        refresh = setTimeout(askForToken, wait);
        if (!first) return;
        adminId = s.admin_id;
        setPhase({ kind: "checking", step: 2, sub: s.external_sub });
        await store.bootstrap();
        if (disposed) return;
        if (store.session !== "authed") {
          fail({ kind: "error", message: "Your inbox could not be loaded. Try again shortly." });
          return;
        }
        setPhase({ kind: "ready" });
      } catch (e) {
        if (disposed || (!exchanged && seq !== latest)) return;
        // A refresh that hit a hiccup keeps the bearer it has and asks again.
        if (!first && transient(e) && Date.now() < expiresAt) {
          if (refresh) clearTimeout(refresh);
          refresh = setTimeout(askForToken, RETRY_MS);
          return;
        }
        if (e instanceof DeniedError) fail({ kind: "denied", denied: e.denied });
        else fail({ kind: "error", message: e instanceof Error ? e.message : "Sign-in failed." });
      }
    };

    const onMessage = (e: MessageEvent) => {
      if (e.source !== window.parent || !origins.includes(e.origin)) return;
      const d = e.data as { type?: unknown; token?: unknown; host?: unknown } | null;
      if (!d || d.type !== "sild:token" || typeof d.token !== "string" || !d.token) return;
      if (typeof d.host === "string" && d.host) setHost(d.host.slice(0, 40));
      void signIn(d.token, e.origin);
    };
    window.addEventListener("message", onMessage);
    const onPageHide = () => {
      endRetiring();
      endSession(bearer);
    };
    window.addEventListener("pagehide", onPageHide);

    allowedOrigins(embedKey)
      .then((list) => {
        if (disposed) return;
        origins = list;
        // Exact targets only: a message to an origin that is not the parent's
        // is dropped by the browser, so the host learns nothing it isn't owed.
        for (const o of origins) window.parent.postMessage({ type: "sild:ready" }, o);
      })
      .catch((e: Error) => !disposed && setPhase({ kind: "error", message: e.message }));

    return () => {
      disposed = true;
      setHostOrigin(null);
      window.removeEventListener("message", onMessage);
      window.removeEventListener("pagehide", onPageHide);
      if (refresh) clearTimeout(refresh);
      endRetiring();
      endSession(bearer);
      setBearer(null);
    };
  }, [store]);

  return (
    <div
      style={{
        display: "flex",
        flexDirection: "column",
        height: "100vh",
        width: "100%",
        overflow: "hidden",
        background: "var(--surface-page)",
      }}
    >
      {phase.kind === "ready" ? (
        <Shell />
      ) : phase.kind === "denied" ? (
        <DeniedScreen denied={phase.denied} host={host} />
      ) : phase.kind === "error" ? (
        <Centered>
          <h2 style={{ fontSize: 16, margin: 0 }}>Support is unavailable</h2>
          <p style={{ fontSize: 14, color: "var(--text-secondary)", margin: 0 }}>{phase.message}</p>
        </Centered>
      ) : (
        <Checking step={phase.step} host={host} sub={phase.sub} tokenHint={phase.tokenHint} />
      )}
    </div>
  );
});

function Centered({ children, width = 380 }: { children: React.ReactNode; width?: number }) {
  return (
    <div style={{ flex: 1, display: "flex", alignItems: "center", justifyContent: "center", padding: 24 }}>
      <div style={{ width: "100%", maxWidth: width, display: "flex", flexDirection: "column", gap: 14 }}>
        {children}
      </div>
    </div>
  );
}

function Checking({ step, host, sub, tokenHint }: { step: number; host: string; sub?: string; tokenHint?: string }) {
  const steps: [string, string][] = [
    [`Reading ${host} access token`, tokenHint ? `Bearer ${tokenHint}…` : "waiting…"],
    [`Validating token with ${host}`, "POST /v1/admin/auth/embedded"],
    ["Loading your grants", sub ? `sub=${sub}` : ""],
  ];
  return (
    <Centered>
      <h2 style={{ fontSize: 16, margin: 0 }}>Connecting to support</h2>
      <div
        role="status"
        aria-live="polite"
        style={{
          display: "flex",
          flexDirection: "column",
          gap: 10,
          background: "var(--white)",
          border: "1px solid var(--border-default)",
          borderRadius: 12,
          padding: "14px 16px",
          boxShadow: "var(--shadow-sm)",
        }}
      >
        {steps.map(([label, mono], n) => (
          <div key={label} style={{ display: "flex", alignItems: "center", gap: 10, minHeight: 22 }}>
            <span style={{ width: 18, height: 18, flex: "none", display: "flex", alignItems: "center", justifyContent: "center" }}>
              {step > n ? (
                <CheckIcon size={16} stroke="var(--green-600)" aria-label="done" />
              ) : step === n ? (
                <Spinner />
              ) : (
                <span style={{ width: 6, height: 6, borderRadius: "50%", background: "var(--slate-300)" }} />
              )}
            </span>
            <span style={{ flex: 1, fontSize: 14 }}>{label}</span>
            <span style={{ fontFamily: "var(--font-mono)", fontSize: 11, color: "var(--text-tertiary)" }}>{mono}</span>
          </div>
        ))}
      </div>
    </Centered>
  );
}

const DENIED_TEXT: Record<string, (host: string) => string> = {
  no_grants: () => "Your sign-in is valid, but you have no grants in this tenant. Ask an admin to add you in Settings → Team.",
  user_not_provisioned: () =>
    "Your sign-in is valid, but no support team member is linked to it. Ask an admin to add you in Settings → Team.",
  role_not_allowed: (host) => `Your sign-in is valid, but your role in ${host} does not include support.`,
};

function DeniedScreen({ denied, host }: { denied: Denied; host: string }) {
  const text = DENIED_TEXT[denied.reason]?.(host) ?? denied.message;
  return (
    <Centered width={460}>
      {/* eslint-disable-next-line @next/next/no-img-element */}
      <img src="/assets/sild-mark-tile.png" width={40} alt="" style={{ borderRadius: 11 }} />
      <div>
        <h1 style={{ fontSize: 22, letterSpacing: "-0.02em", margin: 0 }}>You don&apos;t have access to support</h1>
        <p style={{ fontSize: 14, lineHeight: 1.55, color: "var(--text-secondary)", marginTop: 6, textWrap: "pretty" }}>
          {text}
        </p>
      </div>
      <div
        data-testid="embed-denied"
        style={{
          display: "grid",
          gridTemplateColumns: "auto 1fr",
          gap: "6px 16px",
          padding: "12px 14px",
          background: "var(--white)",
          border: "1px solid var(--border-default)",
          borderRadius: 10,
          fontFamily: "var(--font-mono)",
          fontSize: 12,
        }}
      >
        <span style={{ color: "var(--text-tertiary)" }}>status</span>
        <span style={{ color: "var(--red-600)" }}>403 access_denied</span>
        <span style={{ color: "var(--text-tertiary)" }}>reason</span>
        <span>{denied.reason}</span>
        <span style={{ color: "var(--text-tertiary)" }}>sub</span>
        <span style={{ wordBreak: "break-all" }}>{denied.sub}</span>
      </div>
    </Centered>
  );
}
