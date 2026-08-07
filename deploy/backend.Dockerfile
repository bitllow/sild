# syntax=docker/dockerfile:1
# Sild backend image — every binary in one distroless image; each k8s Deployment,
# compose service or Cloud Run revision picks which one to run via `command`
# (ARCHITECTURE §3a). sild-standalone runs them all in one process.
#
# Built from the REPO ROOT (not ./backend) so the web drop-in bundle is compiled
# and embedded in the same build (§9), instead of relying on a stale checked-in
# copy of internal/webasset/widget.js.
#
#   docker buildx build -f deploy/backend.Dockerfile -t dmitri896/sild:backend .

# 1. Build the web drop-in and hand its bundle to the Go stage. Runs on the
# BUILD platform (native, fast) — the esbuild output is portable JS.
FROM --platform=$BUILDPLATFORM node:24-slim AS web
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
# build.mjs also syncs the bundle into ../backend/internal/webasset/dist; that
# path doesn't exist in this isolated context, so create it to keep the copy happy.
RUN mkdir -p /backend/internal/webasset/dist && npm run build

# 2. Compile the Go binaries with the freshly built widget embedded. Runs
# natively on the BUILD platform and cross-compiles to the target arch — CGO is
# off, so this is fast and needs no emulation.
FROM --platform=$BUILDPLATFORM golang:1.25 AS build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
COPY --from=web /web/dist/widget.js   ./internal/webasset/dist/widget.js
COPY --from=web /web/public/demo.html ./internal/webasset/demo.html
ENV CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH
# One invocation over ./cmd/...: the package graph loads once, the mains link in
# parallel, and no list of binaries needs maintaining as new ones land.
RUN --mount=type=cache,target=/root/.cache/go-build \
    go build -o /out/ ./cmd/...

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/ /usr/local/bin/
USER nonroot:nonroot
# 8080 REST, 8081 WS, 2525 SMTP ingest
EXPOSE 8080 8081 2525
ENTRYPOINT ["/usr/local/bin/sild-api"]
