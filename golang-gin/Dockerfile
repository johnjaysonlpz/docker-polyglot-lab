# syntax=docker/dockerfile:1.7

ARG GO_VERSION=1.25.7
ARG ALPINE_VERSION=3.23.3

# ==============================
# Stage 1: Build the application
# ==============================

FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-alpine AS builder
WORKDIR /app

ENV CGO_ENABLED=0 \
    GOFLAGS="-mod=readonly"

ARG BUILDPLATFORM
ARG TARGETPLATFORM
ARG TARGETOS
ARG TARGETARCH

ARG RUN_TESTS=true
ARG TEST_COUNT=1
ARG SERVICE_NAME=golang-gin-app
ARG VERSION=dev
ARG BUILD_TIME=local

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    go mod download

COPY . .

RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    --mount=type=cache,target=/root/.cache/go-build,sharing=locked \
    set -eu; \
    if [ "${RUN_TESTS}" = "true" ] && [ "${TARGETPLATFORM:-}" = "${BUILDPLATFORM:-}" ]; then \
      apk add --no-cache --virtual .test-deps gcc musl-dev && \
      CGO_ENABLED=1 go test -shuffle=on -race -p 1 -count="${TEST_COUNT}" ./... && \
      apk del .test-deps; \
    fi

RUN --mount=type=cache,target=/root/.cache/go-build,sharing=locked \
    set -eu; \
    GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build \
      -trimpath \
      -buildvcs=false \
      -tags=netgo,osusergo \
      -ldflags="-s -w \
        -X main.ServiceName=${SERVICE_NAME} \
        -X main.Version=${VERSION} \
        -X main.BuildTime=${BUILD_TIME}" \
      -o /out/server ./cmd/server

# ==============================
# Stage 2: Runtime image
# ==============================

FROM alpine:${ALPINE_VERSION}

RUN apk add --no-cache ca-certificates

ARG SERVICE_NAME=golang-gin-app
ARG VERSION=dev
ARG BUILD_TIME=local
ARG OCI_IMAGE_SOURCE=""
ARG OCI_IMAGE_REVISION=""

LABEL org.opencontainers.image.title="${SERVICE_NAME}" \
      org.opencontainers.image.description="Polyglot lab - Go + Gin HTTP service" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.created="${BUILD_TIME}" \
      org.opencontainers.image.url="https://github.com/johnjaysonlpz/docker-polyglot-lab/tree/main/golang-gin" \
      org.opencontainers.image.source="${OCI_IMAGE_SOURCE}" \
      org.opencontainers.image.revision="${OCI_IMAGE_REVISION}" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.authors="John Jayson Lopez" \
      org.opencontainers.image.base.name="alpine:${ALPINE_VERSION}"

ENV GIN_MODE=release \
    PORT=8080 \
    SERVICE_NAME=${SERVICE_NAME} \
    VERSION=${VERSION} \
    BUILD_TIME=${BUILD_TIME}

RUN addgroup -g 10001 appgroup \
    && adduser -D -u 10001 -G appgroup appuser

WORKDIR /app
COPY --chown=appuser:appgroup --from=builder /out/server ./server

RUN chmod 0555 ./server

EXPOSE 8080
USER appuser
STOPSIGNAL SIGTERM

ENTRYPOINT ["./server"]
