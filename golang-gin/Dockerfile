# ==============================
# Stage 1: Build the application
# ==============================
FROM golang:1.25.4-alpine AS builder

WORKDIR /app

# Build strategy:
# - CGO_ENABLED=0 produces a "static-ish" binary (no libc dependency at runtime),
#   which plays nicely with small Alpine runtime images.
# - If you later need cgo (e.g., sqlite extensions), switch this back on.
ENV CGO_ENABLED=0

# Multi-arch support:
# BuildKit/Buildx populate these automatically. If not using Buildx, they may be empty.
ARG TARGETOS
ARG TARGETARCH

# Build-time arguments (CI/Compose contract):
# - RUN_TESTS: optional unit tests during image build
# - SERVICE_NAME / VERSION / BUILD_TIME: build metadata (embedded into the binary)
ARG RUN_TESTS=true
ARG SERVICE_NAME=golang-gin-app
ARG VERSION=dev
ARG BUILD_TIME=local

# Dependency caching:
# Copy go.mod/go.sum first so module download stays cached unless deps change.
COPY go.mod go.sum ./

# BuildKit cache mount:
# Persists Go module cache between builds on the same runner/machine.
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Copy the remainder of the source after dependencies are cached.
COPY . .

# Optional unit tests (defense-in-depth gate).
# Cache mounts speed up repeated test/build cycles.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    if [ "$RUN_TESTS" = "true" ]; then go test ./...; fi

# Build the binary into a stable output location (/out/server).
# - GOOS/GOARCH enable cross-compilation when Buildx passes TARGETOS/TARGETARCH.
# - -s -w strips debug symbols to reduce size.
# - -X injects build metadata into variables used by /info + metrics.
RUN --mount=type=cache,target=/root/.cache/go-build \
    GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} go build \
      -ldflags="-s -w \
        -X main.ServiceName=${SERVICE_NAME} \
        -X main.Version=${VERSION} \
        -X main.BuildTime=${BUILD_TIME}" \
      -o /out/server ./cmd/server


# ==============================
# Stage 2: Runtime image
# ==============================
FROM alpine:3.20

# Runtime packages:
# - curl: used by container HEALTHCHECK (simple and reliable)
# - ca-certificates: enables outbound TLS and avoids x509 surprises
RUN apk add --no-cache curl ca-certificates

# Build-time args re-declared in runtime stage (used for labels + ENV defaults).
ARG SERVICE_NAME=golang-gin-app
ARG VERSION=dev
ARG BUILD_TIME=local
ARG OCI_IMAGE_SOURCE=""
ARG OCI_IMAGE_REVISION=""

# OCI labels (traceability):
# Visible in registries and `docker inspect`, helps map an image back to source+commit.
LABEL org.opencontainers.image.title="${SERVICE_NAME}" \
      org.opencontainers.image.description="Polyglot lab - Go + Gin HTTP service" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.created="${BUILD_TIME}" \
      org.opencontainers.image.url="https://github.com/johnjaysonlpz/docker-polyglot-lab/tree/main/golang-gin" \
      org.opencontainers.image.source="${OCI_IMAGE_SOURCE}" \
      org.opencontainers.image.revision="${OCI_IMAGE_REVISION}" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.authors="John Jayson Lopez"

# Runtime env defaults (overridable via Compose):
# - GIN_MODE=release disables Gin debug logging.
# - PORT is used by the app and healthcheck.
# - SERVICE_NAME/VERSION/BUILD_TIME are exported for runtime parity/debugging
#   even though Go also embeds them into the binary via ldflags.
ENV GIN_MODE=release \
    PORT=8080 \
    SERVICE_NAME=${SERVICE_NAME} \
    VERSION=${VERSION} \
    BUILD_TIME=${BUILD_TIME}

# Non-root runtime user (security posture):
# Fixed UID/GID improves consistency across hosts/volumes.
RUN addgroup -g 10001 appgroup \
    && adduser -D -u 10001 -G appgroup appuser

WORKDIR /app

# Copy the compiled binary from builder output.
# --chown avoids permission issues when running as non-root.
COPY --chown=appuser:appgroup --from=builder /out/server ./server

# Document container port (host mapping is done by Compose).
EXPOSE 8080

USER appuser

# Container liveness:
# Use 127.0.0.1 so healthcheck doesn't depend on DNS/networking.
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
  CMD curl -fsS http://127.0.0.1:${PORT}/health || exit 1

# Entrypoint (exec form):
# No shell; SIGTERM is delivered directly to the process.
ENTRYPOINT ["./server"]
