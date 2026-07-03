/** @type {import('next').NextConfig} */

// The inbox talks to the Go backend (default :8080). We proxy /v1/* through Next
// so the browser stays same-origin — the admin session cookie is first-party and
// no CORS config is needed. Override the target with SILD_API_URL.
const API = process.env.SILD_API_URL || "http://localhost:8080";

const nextConfig = {
  reactStrictMode: true,
  // The dev-tools indicator renders a portal in the bottom-left corner, on top
  // of the inbox's sign-out control. Disable it only when explicitly requested
  // (e.g. e2e runs) so it can't intercept clicks; normal `npm run dev` keeps it.
  ...(process.env.SILD_DISABLE_DEV_INDICATOR ? { devIndicators: false } : {}),
  // Emit a self-contained server bundle (server.js + trimmed node_modules) so
  // the Docker runtime image stays small — see deploy/inbox.Dockerfile.
  output: "standalone",
  async rewrites() {
    return [
      { source: "/v1/:path*", destination: `${API}/v1/:path*` },
      // The Appearance live preview loads the real drop-in bundle from the
      // backend and renders it with the draft config (preview === production).
      { source: "/widget.js", destination: `${API}/widget.js` },
    ];
  },
};

export default nextConfig;
