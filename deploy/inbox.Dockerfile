# syntax=docker/dockerfile:1
# Sild inbox (Phase 2 agent console, Next.js) — standalone output for a small
# runtime image. Built from the REPO ROOT; only ./inbox is used.
#
# NEXT_PUBLIC_* and the /v1 proxy target are baked at BUILD time: Next inlines
# NEXT_PUBLIC_* into the client bundle, and next.config rewrites() is resolved
# into the routes manifest during `next build`. Both are overridable via ARG.
#
#   docker buildx build -f deploy/inbox.Dockerfile -t dmitri896/sild:inbox .

# Build on the BUILD platform (native, fast) — Next output is portable JS.
FROM --platform=$BUILDPLATFORM node:24-slim AS build
WORKDIR /app
# Browser → wss://ws.sild.bitllow.com/v1/ws (baked into the client bundle).
ARG NEXT_PUBLIC_SILD_WS_URL=wss://ws.sild.bitllow.com/v1/ws
# Server-side /v1 proxy target — the in-cluster REST service (same-origin cookie).
ARG SILD_API_URL=http://sild-api:8080
ENV NEXT_PUBLIC_SILD_WS_URL=$NEXT_PUBLIC_SILD_WS_URL \
    SILD_API_URL=$SILD_API_URL \
    NEXT_TELEMETRY_DISABLED=1
COPY inbox/package.json inbox/package-lock.json ./
RUN npm ci
COPY inbox/ ./
RUN npm run build

FROM node:24-slim AS runtime
WORKDIR /app
ENV NODE_ENV=production \
    NEXT_TELEMETRY_DISABLED=1 \
    PORT=3000 \
    HOSTNAME=0.0.0.0
# Next "standalone" output: self-contained server + trimmed node_modules.
COPY --from=build /app/.next/standalone ./
COPY --from=build /app/.next/static     ./.next/static
COPY --from=build /app/public           ./public
EXPOSE 3000
USER node
CMD ["node", "server.js"]
