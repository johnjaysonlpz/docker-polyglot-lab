# /golang-gin — **Go + Gin** HTTP service (**metrics + traces + structured logs**)

A small, production-shaped HTTP service written in **Go 1.25.7** using **Gin 1.11.0**, designed to run standalone or as part of the repo’s Docker Compose lab stack.

**Signals (repo stack):** **traces → Alloy → Tempo**, **metrics → Prometheus**, **logs → Loki**.

> [!TIP]
> For the repo overview and contracts, see [`../README.md`](../README.md). For Compose stacks and operator commands, see [`../docker/README.md`](../docker/README.md).

---

## Contents

- [TL;DR](#tldr)
- [What this service provides](#what-this-service-provides)
- [HTTP API](#http-api)
- [Configuration](#configuration)
- [Request processing pipeline](#request-processing-pipeline)
- [Payload validation & limits](#payload-validation-limits)
- [Observability](#observability)
- [Error utilities](#error-utilities)
- [Operational knobs](#operational-knobs)
- [Testing & linting](#testing-linting)
- [Local CI](#local-ci)
- [Container image details](#container-image-details)
- [Implementation map (where to look)](#implementation-map-where-to-look)
- [Interactions with other modules](#interactions-with-other-modules)

## TL;DR

> [!TIP]
> **Recommended path:** run the full stack (apps + observability) via the canonical Compose entrypoints in [`../docker/README.md#tldr—canonical-entrypoints`](../docker/README.md#tldr--canonical-entrypoints), then use Grafana Explore to validate metrics/logs/traces.

### Service contract (quick reference)

| Contract | Value |
|---|---|
| **Internal listen** | `0.0.0.0:8080` |
| **Compose host port** | `127.0.0.1:8081` |
| **Endpoints** | `/` · `/info` · `/health` · `/ready` · `/metrics` |
| **Request ID** | `X-Request-ID` (accepted if valid, always returned) |
| **Env templates** | `.env.development` · `.env.integration` · `.env.staging` |

### Running options (pick one)

<details>
<summary><strong>Run via the repo Compose stacks (recommended)</strong></summary>

Use the canonical stack entrypoints from [`../docker/README.md#tldr—canonical-entrypoints`](../docker/README.md#tldr--canonical-entrypoints).

**Option A (recommended): run from the repo root** (keeps paths consistent):

```bash
docker compose --project-directory docker \
  -f docker/compose.development.yaml \
  up --build

# or full stack with observability
docker compose --project-directory docker \
  -f docker/compose.integration.nosecrets.yaml \
  up --build
```

**Option B: run from the `/docker` directory** (works because includes are relative):

```bash
cd docker

docker compose \
  -f compose.development.yaml \
  up --build

# or full stack with observability
docker compose \
  -f compose.integration.nosecrets.yaml \
  up --build
```

Compose wiring for this service:
- service name: **`golang-gin-app`**
- host port: **`127.0.0.1:8081`**
- healthcheck: `GET http://127.0.0.1:8080/ready`
- env file selection: `../golang-gin/.env.${APP_ENV:-integration}`

</details>

<details>
<summary><strong>Run locally (without Docker)</strong></summary>

Requires **Go 1.25.7**.

```bash
cd golang-gin
set -a; source .env.development; set +a
go run ./cmd/server
```

Smoke test:
```bash
curl -i http://127.0.0.1:8080/health
curl -i http://127.0.0.1:8080/ready
curl -i http://127.0.0.1:8080/info
curl -i http://127.0.0.1:8080/metrics
```

</details>

<details>
<summary><strong>Build + run container image (Docker)</strong></summary>

#### Build the image
The Dockerfile builds the binary, optionally runs tests, and injects build metadata:

```bash
cd golang-gin

docker build \
  --build-arg RUN_TESTS=true \
  --build-arg SERVICE_NAME=golang-gin-app \
  --build-arg VERSION="${VERSION:-integration}" \
  --build-arg BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -t golang-gin-app:${VERSION:-integration} .
```

#### Run (using `.env.integration`)
```bash
docker run -d \
  --name golang-gin-app \
  --restart unless-stopped \
  --env-file .env.integration \
  -p 8081:8080 \
  golang-gin-app:${VERSION:-integration}
```

#### Check status
```bash
docker ps
docker logs -f golang-gin-app
```

</details>

---

## What this service provides

- **Infra endpoints**: `GET /`, `GET /health`, `GET /ready`, `GET /info`
- **Prometheus metrics**: `GET /metrics` (registry-scoped + custom meters + Go/process collectors)
- **OpenTelemetry tracing**: Gin server spans via `otelgin`, exporting via OTLP (**http/protobuf** or **gRPC**)
- **Structured JSON logs**: `log/slog` with per-request context
- **Request correlation**: strict **`X-Request-ID`** policy, echoed in responses and attached to spans
- **Payload size limiting**: `MAX_BODY_BYTES` with correct **413** behavior (fast-fail + `http.MaxBytesReader` mapping)
- **Graceful shutdown**: SIGINT/SIGTERM with configurable shutdown timeout
- Hardened container build (non-root, read-only-friendly runtime)

---

## HTTP API

### Network
- Binds to: `${HOST}:${PORT}` (defaults: **`0.0.0.0:8080`**)
- Container port: **`8080`**
- Repo compose port mapping: **`127.0.0.1:8081 → 8080`** (service: **`golang-gin-app`**)

### Endpoints
| Method | Path | Purpose |
|---|---|---|
| GET | `/` | Plain-text “service is running” message |
| GET | `/health` | Liveness probe (**200**) |
| GET | `/ready` | Readiness probe (**200**) |
| GET | `/info` | Build/service metadata (JSON) |
| GET | `/metrics` | Prometheus scrape endpoint |

<details>
<summary><strong>Sample: `/info` response</strong></summary>

```json
{
  "service": "golang-gin-app",
  "version": "<version>",
  "build_time": "2026-02-03T07:22:30Z"
}
```

</details>

<details>
<summary><strong>Sample: `/info` log event (JSON)</strong></summary>

```json
{
  "time": "2026-02-03T07:28:22.479749092Z",
  "level": "INFO",
  "msg": "http_request",
  "service": "golang-gin-app",
  "version": "<version>",
  "build_time": "2026-02-03T07:22:30Z",
  "request_id": "b7ca7802-9ac5-4ec2-af06-2dde73a31ed1",
  "trace_id": "ec08a7b15e8612cfb600897ac85d2cac",
  "span_id": "7b76bd13e91fd487",
  "method": "GET",
  "path": "/info",
  "raw_path": "/info",
  "status": 200,
  "ip": "172.18.0.1",
  "latency_ms": 0.022,
  "bytes_written": 82,
  "user_agent": "curl/7.81.0"
}
```

</details>

### Error response format
All errors are emitted as:
<details>
<summary><strong>Sample: error response JSON</strong></summary>

```json
{
  "error": "…",
  "code": "…",
  "request_id": "…"
}
```

</details>

Common codes (explicitly mapped):
- **404** → `not_found` (`NoRoute`)
- **405** → `method_not_allowed` (`HandleMethodNotAllowed=true` + `NoMethod` handler)
- **413** → `payload_too_large`
- **500** → `internal_server_error` (panic recovery / fallback)

---

## Configuration

All configuration is via environment variables (`internal/server/config.go`).

| Env var | Default | Notes |
|---|---:|---|
| `GIN_MODE` | `release` | `debug`, `release`, `test`. **Case-insensitive**. Typically set before router creation; also reflected by Gin’s internal mode handling. |
| `HOST` | `0.0.0.0` | bind address (e.g. `127.0.0.1` for local-only) |
| `PORT` | `8080` | TCP listen port |
| `LOG_LEVEL` | `info` | app logger level (recommended: accept case-insensitive `debug/info/warn/error`). Gin’s own mode is still controlled by `GIN_MODE`. |
| `READ_TIMEOUT` | `5s` | HTTP server `ReadTimeout` |
| `WRITE_TIMEOUT` | `10s` | HTTP server `WriteTimeout` |
| `READ_HEADER_TIMEOUT` | `2s` | HTTP server `ReadHeaderTimeout` |
| `IDLE_TIMEOUT` | `120s` | HTTP server `IdleTimeout` |
| `SHUTDOWN_TIMEOUT` | `5s` | graceful shutdown deadline |
| `MAX_BODY_BYTES` | `1048576` | max request body bytes. `<= 0` disables limit. Typically enforced via `http.MaxBytesReader` (or equivalent middleware) to trigger 413. |
| `TRUSTED_PROXIES` | empty | CSV list consumed by Gin `SetTrustedProxies`. Prefer CIDRs (e.g. `10.0.0.0/8`) or explicit IPs. When set, Gin can safely trust `X-Forwarded-For`/`X-Forwarded-Proto` from those sources. |

Build metadata variables are available as runtime defaults (and are commonly set at image build time):

| Variable | Default | Where it shows up |
|---|---:|---|
| `SERVICE_NAME` | `golang-gin-app` | `/info` JSON, logs, (and labels where applicable) |
| `VERSION` | `0.0.0-dev` | `/info` JSON, logs, `build_info` metric label |
| `BUILD_TIME` | `unknown` | `/info` JSON, logs, `build_info` metric label |

### OpenTelemetry env vars (from `.env.*`)

> [!NOTE]
> This repo uses a **hybrid model**: Prometheus scrapes `/metrics`, while traces are exported via OTLP to Alloy.

<details>
<summary><strong>Click to expand OTel env var examples (development vs integration/staging)</strong></summary>

The repo env files configure:

#### `.env.development`
Tracing is disabled explicitly:
- `OTEL_TRACES_EXPORTER=none`
- `OTEL_METRICS_EXPORTER=none`
- `OTEL_LOGS_EXPORTER=none`

#### `.env.integration` / `.env.staging`
Traces are exported to Alloy (OTLP/HTTP protobuf), logs/metrics exporters remain disabled:
- `OTEL_SERVICE_NAME=golang-gin-app`
- `OTEL_TRACES_EXPORTER=otlp`
- `OTEL_METRICS_EXPORTER=none`
- `OTEL_LOGS_EXPORTER=none`
- `OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf`
- `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT=http://alloy:4318/v1/traces`
- `OTEL_TRACES_SAMPLER=always_on`

### Provided env templates
This module includes:
- `.env.development`
- `.env.integration`
- `.env.staging`

---

</details>

## Request processing pipeline

### Server lifecycle
`cmd/server/main.go`:
- Loads config and warnings (`LoadConfig`).
- Initializes slog logging.
- Initializes tracing (`internal/telemetry/tracing.go`).
- Builds Gin router + middleware chain.
- Starts `http.Server` with timeouts and **`MaxHeaderBytes = 1 MiB`**.
- Gracefully shuts down on SIGINT/SIGTERM using `SHUTDOWN_TIMEOUT`.

<details>
<summary><strong>Router + middleware chain</strong></summary>

`internal/server/router.go` configures Gin with:
- `gin.New()` and `HandleMethodNotAllowed = true`
- Trusted proxies via `TRUSTED_PROXIES` (invalid values disable proxies and log a warning)
- Explicit `NoRoute` and `NoMethod` handlers for stable JSON errors

Middleware order (outer → inner), wired in `middleware_chain.go`:
1. Panic recovery (`GinRecoveryWithSlog`)
2. Tracing (`otelgin.Middleware(serviceName)`)
3. Request id (`RequestIDMiddleware`)
4. Request-id as span attribute (`RequestIDSpanAttrMiddleware`)
5. Request-scoped logger injection (`InjectRequestLogger`)
6. Access logs + metrics (`GinSlogMiddleware`)
7. Error finalizer (`ErrorFinalizer`)
8. Payload guard (`MaxBodyBytesMiddleware`)
9. MaxBytes error mapping (`MaxBytesErrorMiddleware`)

---

</details>

---

## Payload validation & limits

### `MAX_BODY_BYTES` (default **1 MiB**)
- If `MAX_BODY_BYTES <= 0`: limit is disabled.
- Fast fail:
  - if `Content-Length > limit` → reject before handler runs with **413** (`payload_too_large`).
- Streaming enforcement:
  - wraps `Request.Body` with `http.MaxBytesReader`
  - `MaxBytesErrorMiddleware` maps `*http.MaxBytesError` to **413** even if thrown during binding/reads.

413 response shape:
```json
{ "error": "payload too large", "code": "payload_too_large", "request_id": "…" }
```

---

## Observability

**Signal flow (repo stack):**

- **Metrics:** Prometheus scrapes `GET /metrics`
- **Traces:** OTLP → **Alloy** → **Tempo**
- **Logs:** container stdout/stderr → Docker → **Alloy** → **Loki**

| Signal | Boundary | Collected by |
|---|---|---|
| **Metrics** | `/metrics` | Prometheus scrape model |
| **Traces** | OTLP (`4317`/`4318`) | Alloy → Tempo |
| **Logs** | JSON to stdout/stderr | Alloy → Loki |

<details>
<summary><strong>Metrics (Prometheus)</strong></summary>

`/metrics` uses a **module-scoped registry** and includes:
- Go runtime collector
- process collector

Custom metrics:
- **`http_requests_total{method,path,status}`**
- **`http_request_duration_seconds{method,path,status}`**
- **`build_info{version,build_time}`** (gauge set to 1)

Label behavior:
- `path` uses Gin route template (`c.FullPath()`)
- Unmatched routes use `__unmatched__`

Access log suppression:
- successful `/health`, `/ready`, `/metrics` are **not logged** (but are still counted in metrics).

</details>

<details>
<summary><strong>Tracing (OpenTelemetry)</strong></summary>

- Server spans via `otelgin.Middleware`.
- Request id attached to span as attribute `request_id`.

Exporter is configured via standard OTel env vars; repo env files typically set:
- `OTEL_TRACES_EXPORTER=otlp`
- `OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf`
- `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT=http://alloy:4318/v1/traces`
- `OTEL_TRACES_SAMPLER=always_on`

</details>

<details>
<summary><strong>Logs</strong></summary>

Access logs are JSON and include request + correlation fields (request id and trace/span ids when available).
In the repo stack, container logs are shipped via Docker → Alloy → Loki.

---

</details>

---

## Error utilities

The module centralizes error JSON creation and finalization:
- `WriteError` (`errors.go`) produces the stable error shape and sets `X-Request-ID`.
- `ErrorFinalizer` converts Gin errors into normalized HTTP error responses (without leaking internal error types).

---

## Operational knobs

This service uses `net/http` and exposes operational knobs primarily via env-configured server timeouts:

- `READ_TIMEOUT`, `WRITE_TIMEOUT`, `READ_HEADER_TIMEOUT`, `IDLE_TIMEOUT` (HTTP server timeouts)
- `SHUTDOWN_TIMEOUT` (graceful shutdown window)
- `TRUSTED_PROXIES` (client IP attribution via Gin)

---

## Testing & linting

Unit + integration tests:
```bash
cd golang-gin
go test ./...
```

---

## Local CI

This repo provides a **local parity runner** at the repo root: `./.ci-local.sh` (mirrors `.github/workflows/ci.yaml`).

> [!NOTE]
> **Tool versions are pinned in `./.ci-tool-versions.sh`**, which is the **single source of truth** for:
> - `./.ci-local.sh`
> - `.github/workflows/ci.yaml`

### Prerequisites
- bash
- git (for `git diff --exit-code` checks)
- **Go 1.25.7**
- `python3.12` (used by the CI script to enforce coverage thresholds)
- `gcc` (required for the Go race detector)
- Network access (to `go install` tools the first time)

### Run the Go module CI checks
From repo root:

```bash
source ./.ci-tool-versions.sh
./.ci-local.sh go
```

What it runs for this module (in order):
- `go mod tidy` + `git diff --exit-code -- go.mod go.sum`
- formatting/import checks: `gofmt -l` and `goimports -l -local "$GO_MODULE"`
- lint: `golangci-lint config verify --config=.golangci.yaml` and `golangci-lint run --config=.golangci.yaml ./...`
- `go vet ./...`
- tests: `CGO_ENABLED=1 go test ./... -race -shuffle=on -count=1 -coverprofile=coverage.out`
- coverage gate: **100% statements** (fails if < 100%)
- (push/tags only) security: `govulncheck -test ./...` (skipped when `CI_EVENT_NAME=pull_request`)

Useful options:
```bash
./.ci-local.sh doctor go --summary
LOG_LEVEL=debug ./.ci-local.sh go
CI_EVENT_NAME=pull_request ./.ci-local.sh go
```

---

## Container image details

### Dockerfile highlights
- Multi-stage build: Go builder → minimal Alpine runtime
- `CGO_ENABLED=0` build; embeds build metadata via ldflags (`SERVICE_NAME`, `VERSION`, `BUILD_TIME`)
- Optional tests in builder stage (controlled by `RUN_TESTS`)
- Non-root runtime user (**uid=10001**), binary `0555`, `STOPSIGNAL SIGTERM`

---

### Build arguments
| Arg | Default | Purpose |
|---|---:|---|
| `GO_VERSION` | `1.25.7` | Go toolchain version for the builder stage. |
| `ALPINE_VERSION` | `3.23.3` | Alpine version for the builder/runtime base images. |
| `BUILDPLATFORM` | (auto; empty if not provided by BuildKit) | The platform doing the build (the builder machine), e.g. `linux/amd64`. Used to decide whether it’s safe to run tests with `-race` (only when build == target). |
| `TARGETPLATFORM` | (auto; empty if not provided by BuildKit) | The **intended output image platform**, e.g. `linux/arm64`. Used to compare against `BUILDPLATFORM` to gate tests. |
| `TARGETOS` | (auto; empty if not provided by BuildKit) | Target OS part of the platform (e.g. `linux`). Used to set `GOOS=${TARGETOS:-linux}` for cross-compiling. |
| `TARGETARCH` | (auto; empty if not provided by BuildKit) | Target CPU architecture part of the platform (e.g. `amd64`, `arm64`). Used to set `GOARCH=${TARGETARCH:-amd64}` for cross-compiling. |
| `RUN_TESTS` | `true` | If `true`, runs `go test` during the build (race only when build platform == target platform). |
| `TEST_COUNT` | `1` | Passed to `go test -count` to control test repetition. |
| `SERVICE_NAME` | `golang-gin-app` | Default runtime env + OCI labels (and used by `/info`). |
| `VERSION` | `dev` | Default runtime env + OCI labels (and used by `/info`). |
| `BUILD_TIME` | `local` | Default runtime env + OCI labels (and used by `/info`). |
| `OCI_IMAGE_SOURCE` | "" | Sets OCI label `org.opencontainers.image.source` (typically the repository URL; often populated in CI via `docker/build-push-action` `labels:` or `docker/metadata-action`). |
| `OCI_IMAGE_REVISION` | "" | Sets OCI label `org.opencontainers.image.revision` (typically the Git commit SHA, e.g. GitHub Actions `GITHUB_SHA`; often populated in CI via `docker/build-push-action` `labels:` or `docker/metadata-action`). |

> Note: BuildKit also provides implicit platform args (`BUILDPLATFORM`, `TARGETPLATFORM`, `TARGETOS`, `TARGETARCH`). The Go Dockerfile uses these to decide when it is safe to run `go test -race` during image builds.

## Implementation map (where to look)

| Change you want | Where to look |
|---|---|
| **`cmd/server/main.go`** | entrypoint + graceful shutdown |
| **`internal/server/router.go`** | router + NoRoute/NoMethod |
| **`internal/server/middleware_chain.go`** | middleware wiring |
| **`internal/server/http_middleware.go`** | request id, logging, payload limits, metrics |
| **`internal/server/routes.go`** | endpoints |
| **`internal/server/metrics.go`** | Prometheus registry + collectors |
| **`internal/telemetry/tracing.go`** | tracing setup |
| **`internal/server/error_finalizer.go`** | error normalization |
| **`internal/server/recovery.go`** | panic recovery |

> [!TIP]
> Prefer starting with the **request pipeline** (middleware/filter chain), then jump to metrics/logging/tracing wiring.

---

## Interactions with other modules

- `/docker` runs this service as `golang-gin-app`, maps it to `127.0.0.1:8081`, scrapes `/metrics`, and routes OTLP traces via Alloy → Tempo.
- `/java-springboot` and `/python-django` follow the same **health/ready/metrics/info + request-id + OTLP** conventions so dashboards/alerts can be shared.
