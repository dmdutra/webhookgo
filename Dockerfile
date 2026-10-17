# syntax=docker/dockerfile:1

# The UI and the Go compiler run on the build machine's platform; only the
# binary is cross-compiled, so multi-arch images need no emulation.

# ---- 1. Web UI (Svelte + Vite) ------------------------------------------------
FROM --platform=$BUILDPLATFORM node:24-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

# ---- 2. Go binary with the UI embedded ------------------------------------------
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS build
WORKDIR /src
ENV CGO_ENABLED=0 GOTOOLCHAIN=local
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY web/embed.go ./web/
COPY --from=web /src/web/dist ./web/dist
ARG VERSION=dev TARGETOS TARGETARCH
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/webhookgo ./cmd/webhookgo

# ---- 3. Runtime: static binary on distroless, non-root ---------------------------
FROM gcr.io/distroless/static-debian13:nonroot
ARG VERSION=dev
LABEL org.opencontainers.image.title="webhookgo" \
      org.opencontainers.image.description="Inspect webhooks and HTTP requests in real time" \
      org.opencontainers.image.url="https://webhookgo.dev" \
      org.opencontainers.image.source="https://github.com/dbaio/webhookgo" \
      org.opencontainers.image.licenses="BSD-2-Clause" \
      org.opencontainers.image.version="${VERSION}"
COPY LICENSE /LICENSE
COPY --from=build /out/webhookgo /webhookgo
ENV LISTEN_ADDR=:8080
EXPOSE 8080
HEALTHCHECK --interval=15s --timeout=4s --start-period=5s --retries=3 CMD ["/webhookgo", "healthcheck"]
ENTRYPOINT ["/webhookgo"]
