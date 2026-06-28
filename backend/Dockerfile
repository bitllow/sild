# Single cgo-free image containing all four Sild binaries; compose picks which
# one each service runs (ARCHITECTURE §3a).
FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ENV CGO_ENABLED=0
RUN go build -o /out/sild-api      ./cmd/sild-api     && \
    go build -o /out/sild-ws       ./cmd/sild-ws      && \
    go build -o /out/sild-worker   ./cmd/sild-worker  && \
    go build -o /out/sild-migrate  ./cmd/sild-migrate

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/ /usr/local/bin/
USER nonroot:nonroot
EXPOSE 8080 8081
ENTRYPOINT ["/usr/local/bin/sild-api"]
