# golang-gin-app

A small, production-style HTTP service built with **Go** and **Gin**, designed as the Go/Gin component of the `docker-polyglot-lab` project.

This service focuses on **operational concerns** rather than business logic:

- Health & readiness endpoints
- Structured JSON logging with `slog`
- Prometheus metrics (`/metrics`)
- Graceful shutdown on SIGINT/SIGTERM
- Configuration via environment variables
- Small, non-root Docker image using a multi-stage build

---

## Quick start

### Prerequisites

- Go **1.25+**
- Docker (optional, for containerized runs)

### Clone and enter the service directory

```bash
git clone https://github.com/johnjaysonlpz/docker-polyglot-lab.git
cd docker-polyglot-lab/golang-gin
```

### Run tests
```bash
go test ./...
```

### Run locally (without Docker)

```bash
export GIN_MODE=debug
export HOST=0.0.0.0
export PORT=8080
export LOG_LEVEL=debug

go run ./cmd/server
```

Then hit:

```bash
curl http://localhost:8080/
curl http://localhost:8080/info
curl http://localhost:8080/metrics
```

---

## Running with Docker

### Build the image

The Dockerfile builds the binary, runs tests (optional), and injects build metadata:

```bash
docker build \
  --build-arg RUN_TESTS=true \
  --build-arg SERVICE_NAME=golang-gin-app \
  --build-arg VERSION=1.0.0 \
  --build-arg BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -t golang-gin-app:1.0.0 .
```

### Run (prod-like, using `.env.int`)

```bash
docker run -d \
  --name golang-gin-app \
  --restart unless-stopped \
  --env-file .env.int \
  -p 8081:8080 \
  golang-gin-app:1.0.0
```

### Check status:

```bash
docker ps
docker logs -f golang-gin-app
```

### Example logs:

```json
{"time":"2025-12-07T21:09:12.075915367Z","level":"INFO","msg":"starting_server","service":"golang-gin-app","version":"1.0.0","buildTime":"2025-12-07T21:08:28Z","addr":"0.0.0.0:8080","gin_mode":"release"}
{"time":"2025-12-07T21:09:43.592252617Z","level":"INFO","msg":"http_request","service":"golang-gin-app","version":"1.0.0","buildTime":"2025-12-07T21:08:28Z","status":200,"method":"GET","path":"/","rawPath":"/","query":"","ip":"172.17.0.1","latency":"55.918µs","userAgent":"..."}
```

The resulting image is small and non-root:
- ~43MB Alpine-based runtime
- Runs as user appuser (UID 10001)

---

## HTTP API

All endpoints are HTTP GET.

| Path       | Description                                                  | Status codes |
| ---------- | ------------------------------------------------------------ | ------------ |
| `/`        | Simple banner: `"golang-gin-app is running (Go + Gin)\n"`    | `200`        |
| `/info`    | Service metadata (`service`, `version`, `buildTime`) as JSON | `200`        |
| `/health`  | Liveness probe (always `200` in this template)               | `200`        |
| `/ready`   | Readiness probe (currently always `200` in this template)    | `200`        |
| `/metrics` | Prometheus metrics (text exposition format)                  | `200`        |

In a real system, `/ready` would incorporate dependency checks (DB, downstream services, etc.).

Examples:

```bash
curl http://localhost:8081/
curl http://localhost:8081/info
curl http://localhost:8081/health
curl http://localhost:8081/ready
curl http://localhost:8081/metrics
```

---

## Observability

### Structured logging

Logging is done via `log/slog` with a JSON handler:

- All application logs go to `stdout` as structured JSON
- HTTP access logs use the `"http_request"` message
- Infra endpoints (`/health`, `/ready`, `/metrics`) are not logged, to keep noise low

Example HTTP log record:

```json
{
  "time":"2025-12-07T21:09:51.515428321Z",
  "level":"INFO",
  "msg":"http_request",
  "service":"golang-gin-app",
  "version":"1.0.0",
  "buildTime":"2025-12-07T21:08:28Z",
  "status":200,
  "method":"GET",
  "path":"/info",
  "rawPath":"/info",
  "query":"",
  "ip":"172.17.0.1",
  "latency":"65.714µs",
  "userAgent":"Mozilla/5.0 ..."
}
```

Logging behavior is implemented in:

- `internal/server/logger.go`
- `internal/server/middleware.go`
- `internal/server/recovery.go`

### Prometheus Metrics

Metrics are provided via `github.com/prometheus/client_golang` and exposed on `/metrics`:

- `http_requests_total{service,method,path,status}`
- `http_request_duration_seconds{service,method,path,status}`
- `build_info{service,version,build_time}` (value always `1`)
- Standard `go_*` and `process_*` metrics

All HTTP requests are instrumented via GinSlogMiddleware, with labels based on:

- `service` (from config)
- `method`
- `path` (resolved Gin route, e.g. `/info`)
- `status` (HTTP status code)

---

## Configuration

Configuration is driven by environment variables and validated on startup (`Config.Validate()`).

### Core env vars

| Variable              | Default   | Description                                               |
| --------------------- | --------- | --------------------------------------------------------- |
| `GIN_MODE`            | `release` | Gin mode: `debug`, `release`, or `test`                   |
| `HOST`                | `0.0.0.0` | Listen address                                            |
| `PORT`                | `8080`    | Listen port (1–65535)                                     |
| `LOG_LEVEL`           | `info`    | `debug`, `info`, `warn`, `warning`, `error`               |
| `READ_TIMEOUT`        | `5s`      | `http.Server.ReadTimeout` (Go duration string)            |
| `WRITE_TIMEOUT`       | `10s`     | `http.Server.WriteTimeout`                                |
| `READ_HEADER_TIMEOUT` | `2s`      | `http.Server.ReadHeaderTimeout`                           |
| `IDLE_TIMEOUT`        | `120s`    | `http.Server.IdleTimeout`                                 |
| `SHUTDOWN_TIMEOUT`    | `5s`      | Graceful shutdown timeout when SIGINT/SIGTERM is received |

Validation rules include:

- `GIN_MODE` must be one of `debug`, `release`, `test`
- `HOST` must not be empty
- `PORT` must be a valid TCP port (1–65535)
- All timeouts must be `> 0`

### Build metadata

Build metadata is passed at link time and surfaced via `/info` and log records:

- `ServiceName` (default: `golang-gin-app`)
- `Version` (default: `0.0.0-dev`)
- `BuildTime` (default: `unknown`)

The Dockerfile injects these values:

```dockerfile
RUN go build \
    -ldflags="-s -w \
      -X main.ServiceName=${SERVICE_NAME} \
      -X main.Version=${VERSION} \
      -X main.BuildTime=${BUILD_TIME}" \
    -o server ./cmd/server
```

You can verify them via:

```bash
curl http://localhost:8081/info
# => {"service":"golang-gin-app","version":"1.0.0","buildTime":"2025-12-07T21:08:28Z"}
```

---

## Testing

Unit and integration-style tests live in `internal/server/server_test.go` and cover:
- Config parsing and validation (`parseLogLevel`, `parseDurationEnv`, `Config.Validate`, `LoadConfig`)
- Router and endpoints (`/`, `/info`, `/health`, `/ready`, `/metrics`)
- Logging middleware behavior: 
    - infra endpoints increment metrics but do not log
    - application endpoints produce a `http_request` log
- Recovery middleware behavior:
    - panics are caught and result in `500` with a `panic_recovered` log

Run all tests:

```bash
go test ./...
```

The Docker build also supports running tests as part of the image build:

```bash
docker build --build-arg RUN_TESTS=true -t golang-gin-app:dev .
```

Set `RUN_TESTS=false` to skip tests in CI scenarios where you’ve already run them.

---

## Project Structure

```text
golang-gin/
├── cmd/
│   └── server/
│       └── main.go          # Entry point, wiring config/logging/HTTP server
├── internal/
│   └── server/
│       ├── config.go        # Env-driven config + validation
│       ├── logger.go        # slog JSON logger setup
│       ├── metrics.go       # Prometheus registry + metrics
│       ├── middleware.go    # Logging + metrics middleware for Gin
│       ├── recovery.go      # Panic recovery middleware
│       ├── router.go        # Gin router & HTTP endpoints
│       └── server_test.go   # Tests
├── Dockerfile               # Multi-stage build, non-root runtime
├── .dockerignore
├── .env.dev                 # Dev env config
├── .env.int                 # Integration env config
└── .env.prod                # Production env config
```

---

## Notes

- This service is intentionally minimal on business logic and heavy on **operational patterns**:
    - health/readiness
    - metrics
    - structured logs
    - graceful shutdown
    - Docker best practices
- In the full `docker-polyglot-lab` project, there are equivalent services in:
    - Java + Spring Boot
    - Python + Django

This Go service can be used as a template for new Gin-based microservices with production-friendly defaults.
