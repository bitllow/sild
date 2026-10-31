// The one HTTP client for the Sild API. /v1/* is proxied by Next, so requests
// carry the HttpOnly session cookie, or the in-memory bearer when embedded.

import { createHttp } from "@sild/client";

export { ApiError } from "@sild/client";

let bearer: string | null = null;
let onBearerRejected: (() => void) | null = null;

/** Switch to bearer auth (embedded inbox). Held in memory only, never stored. */
export function setBearer(token: string | null, onRejected?: () => void) {
  bearer = token;
  onBearerRejected = onRejected ?? null;
}

export function isEmbedded(): boolean {
  return bearer !== null;
}

const NETWORK_ERROR = "Network error — is the backend running on :8080?";

export const http = createHttp({
  baseUrl: "",
  credential: () => (bearer ? { kind: "bearer", token: bearer } : { kind: "cookie" }),
  onUnauthorized: () => {
    if (bearer) onBearerRejected?.();
  },
  // The message of a status-0 ApiError, which most screens render as is.
  fetch: (input, init) =>
    fetch(input, init).catch((e: unknown) => {
      throw new Error(NETWORK_ERROR, { cause: e });
    }),
});

// The backend's message is the useful part — it says which field was wrong.
export function errorText(e: unknown): string {
  return e instanceof Error ? e.message : "Something went wrong.";
}

// The panels render whole profiles, so the blob is asked for by name; a page
// carries one contacts block instead of a fetch per row.
export const WITH_PROFILES = "contacts.metadata";
