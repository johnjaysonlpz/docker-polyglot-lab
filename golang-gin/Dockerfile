# ==============================
# Stage 1: Build the application
# ==============================
FROM golang:1.25.4-alpine AS builder

WORKDIR /app

# Build settings: produce a static-ish Linux/amd64 binary
ENV CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64

# Build-time arguments:
# - RUN_TESTS: whether to run `go test ./...` during the image build
# - SERVICE_NAME, VERSION, BUILD_TIME: injected into the binary via ldflags
ARG RUN_TESTS=true
ARG SERVICE_NAME=golang-gin-app
ARG VERSION=dev
ARG BUILD_TIME=local

# Copy go.mod/go.sum first to leverage Docker layer caching for dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the source code
COPY . .

# Optionally run unit tests as part of the build
RUN if [ "$RUN_TESTS" = "true" ]; then go test ./...; fi

# Build the binary with stripped symbols and embedded build metadata
RUN go build \
    -ldflags="-s -w \
      -X main.ServiceName=${SERVICE_NAME} \
      -X main.Version=${VERSION} \
      -X main.BuildTime=${BUILD_TIME}" \
    -o server ./cmd/server

# ==============================
# Stage 2: Runtime image
# ==============================
FROM alpine:3.20

# Install only what the app needs at runtime (HTTP healthcheck + TLS)
RUN apk add --no-cache curl ca-certificates

# Re-declare build-time args for use in labels
ARG SERVICE_NAME=golang-gin-app
ARG VERSION=dev
ARG BUILD_TIME=local
ARG OCI_IMAGE_SOURCE=""
ARG OCI_IMAGE_REVISION=""

# OCI image labels
LABEL org.opencontainers.image.title="${SERVICE_NAME}" \
      org.opencontainers.image.description="Polyglot lab - Go + Gin HTTP service" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.created="${BUILD_TIME}" \
      org.opencontainers.image.url="https://github.com/johnjaysonlpz/docker-polyglot-lab/tree/main/golang-gin" \
      org.opencontainers.image.source="${OCI_IMAGE_SOURCE}" \
      org.opencontainers.image.revision="${OCI_IMAGE_REVISION}" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.authors="John Jayson Lopez"

# Default runtime configuration (overridable via env vars)
ENV GIN_MODE=release \
    PORT=8080

# Create a dedicated non-root user & group with fixed IDs
RUN addgroup -g 10001 appgroup \
    && adduser -D -u 10001 -G appgroup appuser

WORKDIR /app

# Copy the compiled binary from the builder image
COPY --chown=appuser:appgroup --from=builder /app/server ./server

# Document the container port (actual mapping is done at runtime)
EXPOSE 8080

# Run as non-root for better security
USER appuser

# Container-level liveness check: hit the HTTP /health endpoint
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
  CMD curl -fsS http://127.0.0.1:${PORT}/health || exit 1

# Start the Go HTTP server
ENTRYPOINT ["./server"]
