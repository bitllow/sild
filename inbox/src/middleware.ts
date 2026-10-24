import { NextResponse, type NextRequest } from "next/server";

// The embed route may be framed only by the origins its tenant allows; any
// failure to learn them leaves it unframeable.
const API = process.env.SILD_API_URL || "http://localhost:8080";
const ORIGIN = /^https?:\/\/[a-z0-9.-]+(:\d{1,5})?$/i;

export async function middleware(req: NextRequest) {
  const res = NextResponse.next();
  res.headers.set("Content-Security-Policy", `frame-ancestors ${await frameAncestors(req.nextUrl.searchParams.get("key") ?? "")}`);
  return res;
}

export const config = { matcher: "/embed" };

async function frameAncestors(key: string): Promise<string> {
  if (!/^[A-Za-z0-9_-]{1,64}$/.test(key)) return "'none'";
  try {
    const r = await fetch(`${API}/v1/admin/auth/embedded/${key}`, { cache: "no-store" });
    if (!r.ok) return "'none'";
    const origins = ((await r.json()) as { allowed_origins?: unknown }).allowed_origins;
    const valid = Array.isArray(origins) ? origins.filter((o): o is string => typeof o === "string" && ORIGIN.test(o)) : [];
    return valid.length ? valid.join(" ") : "'none'";
  } catch {
    return "'none'";
  }
}
