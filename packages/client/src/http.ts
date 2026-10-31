import type { ErrorBody } from './wire.ts';

export type Credential =
  | { kind: 'bearer'; token: string }
  | { kind: 'bearer'; getToken: () => string | Promise<string> }
  // Same-origin session cookie (credentials: "include").
  | { kind: 'cookie' }
  // No cookie and no Authorization header.
  | { kind: 'anonymous' };

export type HttpOptions = {
  // Origin the paths are appended to; '' sends same-origin relative requests.
  baseUrl?: string;
  // A function is read on every request, so the credential can change at runtime.
  credential: Credential | (() => Credential);
  fetch?: typeof fetch;
  timeoutMs?: number;
  // Runs on a 401 to a credentialed request; resolve true to retry it once.
  onUnauthorized?: (err: ApiError) => boolean | void | Promise<boolean | void>;
};

export type Query = Record<string, string | number | boolean | null | undefined>;

export type RequestOptions = {
  body?: unknown;
  query?: Query;
  headers?: Record<string, string>;
  // Send this call without the credential: it precedes or establishes one.
  anonymous?: boolean;
  signal?: AbortSignal;
  // Let the request outlive the page that sent it (pagehide, reload).
  keepalive?: boolean;
};

// 204 and 304 carry no body: data is undefined.
export type HttpResponse<T> = { status: number; data: T; etag: string | null };

export type Http = {
  readonly baseUrl: string;
  url(path: string, query?: Query): string;
  exchange<T = unknown>(method: string, path: string, opts?: RequestOptions): Promise<HttpResponse<T>>;
  request<T = unknown>(method: string, path: string, opts?: RequestOptions): Promise<T>;
  get<T = unknown>(path: string, opts?: RequestOptions): Promise<T>;
  post<T = unknown>(path: string, body?: unknown, opts?: RequestOptions): Promise<T>;
  put<T = unknown>(path: string, body?: unknown, opts?: RequestOptions): Promise<T>;
  patch<T = unknown>(path: string, body?: unknown, opts?: RequestOptions): Promise<T>;
  del<T = unknown>(path: string, body?: unknown, opts?: RequestOptions): Promise<T>;
};

// Any failed call. status 0 means no HTTP response arrived (network, timeout, abort).
export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly fields?: Record<string, string>;
  readonly requestId?: string;
  readonly method: string;
  readonly path: string;

  constructor(
    status: number,
    code: string,
    message: string,
    extra: { fields?: Record<string, string>; requestId?: string; method?: string; path?: string; cause?: unknown } = {},
  ) {
    super(message, extra.cause === undefined ? undefined : { cause: extra.cause });
    this.name = 'ApiError';
    this.status = status;
    this.code = code;
    this.fields = extra.fields;
    this.requestId = extra.requestId;
    this.method = extra.method ?? '';
    this.path = extra.path ?? '';
  }

  get isUnauthorized(): boolean {
    return this.status === 401;
  }
}

export function createHttp(opts: HttpOptions): Http {
  const baseUrl = (opts.baseUrl ?? '').replace(/\/+$/, '');

  const url = (path: string, query?: Query): string => {
    let out = baseUrl + path;
    if (query) {
      const q = new URLSearchParams();
      for (const [k, v] of Object.entries(query)) if (v !== undefined && v !== null && v !== '') q.set(k, String(v));
      const s = q.toString();
      if (s) out += (path.includes('?') ? '&' : '?') + s;
    }
    return out;
  };

  const credential = (): Credential => (typeof opts.credential === 'function' ? opts.credential() : opts.credential);

  const send = async (method: string, path: string, o: RequestOptions): Promise<Response> => {
    const cred: Credential = o.anonymous ? { kind: 'anonymous' } : credential();
    const headers: Record<string, string> = { accept: 'application/json', ...o.headers };
    if (cred.kind === 'bearer') {
      headers.authorization = `Bearer ${'token' in cred ? cred.token : await cred.getToken()}`;
    }
    if (o.body !== undefined) headers['content-type'] = 'application/json';
    let signal = o.signal;
    // A controller, not AbortSignal.timeout(): the widget bundle targets browsers without it.
    // The timer also covers reading the body, so it is not cleared when fetch resolves.
    if (!signal && opts.timeoutMs) {
      const ctl = new AbortController();
      const timer = setTimeout(() => ctl.abort(), opts.timeoutMs) as { unref?: () => void };
      timer.unref?.();
      signal = ctl.signal;
    }
    try {
      return await (opts.fetch ?? fetch)(url(path, o.query), {
        method,
        headers,
        credentials: cred.kind === 'cookie' ? 'include' : 'omit',
        body: o.body === undefined ? undefined : JSON.stringify(o.body),
        signal,
        keepalive: o.keepalive,
      });
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e);
      throw new ApiError(0, 'network_error', msg, { method, path: path.split('?')[0], cause: e });
    }
  };

  const exchange = async <T>(method: string, path: string, o: RequestOptions = {}): Promise<HttpResponse<T>> => {
    let res = await send(method, path, o);
    if (res.status === 401 && !o.anonymous && opts.onUnauthorized && credential().kind !== 'anonymous') {
      const err = await toError(res, method, path);
      if (!(await opts.onUnauthorized(err))) throw err;
      res = await send(method, path, o);
    }
    if (!res.ok && res.status !== 304) throw await toError(res, method, path);
    const etag = res.headers.get('etag');
    if (res.status === 204 || res.status === 304) return { status: res.status, data: undefined as T, etag };
    return { status: res.status, data: parse(await res.text()) as T, etag };
  };

  const request = async <T>(method: string, path: string, o?: RequestOptions): Promise<T> =>
    (await exchange<T>(method, path, o)).data;

  return {
    baseUrl,
    url,
    exchange,
    request,
    get: (path, o) => request('GET', path, o),
    post: (path, body, o) => request('POST', path, { ...o, body }),
    put: (path, body, o) => request('PUT', path, { ...o, body }),
    patch: (path, body, o) => request('PATCH', path, { ...o, body }),
    del: (path, body, o) => request('DELETE', path, { ...o, body }),
  };
}

function parse(text: string): unknown {
  if (!text) return undefined;
  try {
    return JSON.parse(text);
  } catch {
    return text;
  }
}

async function toError(res: Response, method: string, path: string): Promise<ApiError> {
  const body = parse(await res.text().catch(() => '')) as Partial<ErrorBody> | string | undefined;
  const err = typeof body === 'object' && body !== null ? body.error : undefined;
  return new ApiError(res.status, err?.code || 'http_error', err?.message || res.statusText || `HTTP ${res.status}`, {
    fields: err?.fields,
    requestId: err?.request_id,
    method,
    path: path.split('?')[0],
  });
}
