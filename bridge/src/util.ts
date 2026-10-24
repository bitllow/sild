import { ApiError } from '@sild/client';

// The adapter kit owns truncateUtf8 so providers can take kit/ on its own.
export { truncateUtf8 } from './adapter/kit/server.ts';

export function errText(e: unknown): string {
  if (e instanceof ApiError && e.method) return `${e.method} ${e.path} → ${e.status} ${e.code}: ${e.message}`;
  return e instanceof Error ? e.message : String(e);
}
