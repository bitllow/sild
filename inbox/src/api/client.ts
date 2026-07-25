// Thin fetch wrapper for the Sild admin API (§4.3). Requests go to /v1/* which
// Next proxies to the Go backend (see next.config.mjs), so they're same-origin
// and carry the HttpOnly admin session cookie automatically.

export class ApiError extends Error {
  status: number;
  code?: string;
  constructor(status: number, message: string, code?: string) {
    super(message);
    this.status = status;
    this.code = code;
  }
  get isUnauthorized() {
    return this.status === 401;
  }
}

type Json = Record<string, unknown> | unknown[];

async function request<T>(method: string, path: string, body?: Json): Promise<T> {
  let res: Response;
  try {
    res = await fetch(`/v1${path}`, {
      method,
      credentials: "include",
      headers: body !== undefined ? { "Content-Type": "application/json" } : undefined,
      body: body !== undefined ? JSON.stringify(body) : undefined,
    });
  } catch {
    throw new ApiError(0, "Network error — is the backend running on :8080?");
  }

  if (res.status === 204) return undefined as T;

  let payload: unknown = null;
  const text = await res.text();
  if (text) {
    try {
      payload = JSON.parse(text);
    } catch {
      payload = text;
    }
  }

  if (!res.ok) {
    const err = (payload as { error?: { code?: string; message?: string } })?.error;
    throw new ApiError(res.status, err?.message || res.statusText || "Request failed", err?.code);
  }
  return payload as T;
}

export const api = {
  get: <T>(path: string) => request<T>("GET", path),
  post: <T>(path: string, body?: Json) => request<T>("POST", path, body ?? {}),
  put: <T>(path: string, body?: Json) => request<T>("PUT", path, body ?? {}),
  patch: <T>(path: string, body?: Json) => request<T>("PATCH", path, body ?? {}),
  del: <T>(path: string, body?: Json) => request<T>("DELETE", path, body),
};

/**
 * Drain every page of a paginated collection.
 *
 * Bounded settings lists and a contact's history are rendered whole, with no
 * scroll UI to continue from — reading only the first page would silently hide
 * the rest. `pages` caps the walk so a runaway cursor cannot loop forever.
 */
export async function collectAll<T>(
  fetchPage: (cursor: string | null) => Promise<{ items: T[]; next_cursor: string | null; has_more: boolean }>,
  pages = 50
): Promise<T[]> {
  const out: T[] = [];
  let cursor: string | null = null;
  for (let i = 0; i < pages; i++) {
    const page = await fetchPage(cursor);
    out.push(...page.items);
    if (!page.has_more || !page.next_cursor) return out;
    cursor = page.next_cursor;
  }
  return out;
}
