// The page that frames the embedded inbox. A step-up (a passkey at the
// integrator) runs there, on the integrator's own origin.

let hostOrigin: string | null = null;

export function setHostOrigin(origin: string | null) {
  hostOrigin = origin;
}

export function canStepUp(): boolean {
  return hostOrigin !== null;
}

const STEP_UP_TIMEOUT_MS = 120_000;

/** Asks the host to run its step-up for this conversation; true once it reports success. */
export function stepUp(conversation: string, reason: string): Promise<boolean> {
  const origin = hostOrigin;
  if (!origin) return Promise.resolve(false);
  return new Promise((resolve) => {
    const done = (ok: boolean) => {
      window.removeEventListener("message", onMessage);
      clearTimeout(timer);
      resolve(ok);
    };
    const onMessage = (e: MessageEvent) => {
      if (e.source !== window.parent || e.origin !== origin) return;
      const d = e.data as { type?: unknown; conversation?: unknown; ok?: unknown } | null;
      if (d?.type === "sild:step-up-done" && d.conversation === conversation) done(d.ok === true);
    };
    const timer = setTimeout(() => done(false), STEP_UP_TIMEOUT_MS);
    window.addEventListener("message", onMessage);
    window.parent.postMessage({ type: "sild:step-up", conversation, reason }, origin);
  });
}
