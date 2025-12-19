# python-django-app

A small, production-style HTTP service built with **Python 3.12**, **Django**, and **Gunicorn**, designed as the Django component of the `docker-polyglot-lab` project.

This service focuses on **operational concerns** rather than business logic:

- Health & readiness endpoints
- Structured JSON logging via `python-json-logger` (true fields, not string-parsed)
- Request correlation via `X-Request-ID` (stored as `request_id` for all logs)
- Prometheus metrics (`/metrics`) via `prometheus_client`
- Stable metrics labels for 404s (`path="__unmatched__"`)
- Real client IP handling behind reverse proxies (configurable trusted proxies)
- Graceful shutdown with Gunicorn (`--graceful-timeout`)
- Configuration via environment variables (validated in settings)
- Multi-stage Docker build, non-root runtime
- No shell entrypoint (`ENTRYPOINT ["python","-u","entrypoint.py"]`)

---

## Quick start

### Prerequisites

- Python **3.12**
- `pip` and `virtualenv` (or the built-in `venv` module)
- Docker (optional, for containerized runs)

### Clone and enter the service directory

```bash
git clone https://github.com/johnjaysonlpz/docker-polyglot-lab.git
cd docker-polyglot-lab/python-django
```

### Install dependencies (local)

Create and activate a virtualenv, then install requirements:

```bash
python -m venv .venv
source .venv/bin/activate      # On Windows: .venv\Scripts\activate
pip install --upgrade pip
pip install -r requirements.txt
```

### Run tests

From the `app` directory:

```bash
cd app
export DJANGO_SETTINGS_MODULE=django_app.settings
python manage.py test
```

### Run locally (without Docker)

Using Django’s development server:

```bash
cd python-django/app

export DJANGO_SETTINGS_MODULE=django_app.settings
export DEBUG=true
export HOST=0.0.0.0
export PORT=8080
export LOG_LEVEL=DEBUG
export DJANGO_SECRET_KEY=dev-secret-key

python manage.py runserver 0.0.0.0:8080
```

Then hit:

```bash
curl http://localhost:8080/
curl http://localhost:8080/info
curl http://localhost:8080/health
curl http://localhost:8080/ready
curl http://localhost:8080/metrics
```

---

## Running with Docker

### Build the image

The Dockerfile uses a multi-stage build (test stage → runtime) and optionally runs tests:

```bash
docker build \
  --build-arg RUN_TESTS=true \
  --build-arg SERVICE_NAME=python-django-app \
  --build-arg VERSION=1.0.0 \
  --build-arg BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -t python-django-app:1.0.0 .
```

### Run (prod-like, using `.env.int`)

```bash
docker run -d \
  --name python-django-app \
  --restart unless-stopped \
  --env-file .env.int \
  -p 8083:8080 \
  python-django-app:1.0.0
```

Check status:

```bash
docker ps
docker logs -f python-django-app
```

Example logs (shape):

```bash
[2025-12-19 09:18:41 +0000] [1] [INFO] Starting gunicorn 23.0.0
[2025-12-19 09:18:41 +0000] [1] [INFO] Listening at: http://0.0.0.0:8080 (1)
```

Infra startup log (note `process`/`threadName` and request id placeholder):

```json
{
  "asctime": "2025-12-19 09:18:41,872",
  "levelname": "INFO",
  "name": "infra",
  "process": 7,
  "threadName": "MainThread",
  "message": "infra_app_ready",
  "request_id": "-"
}
```

The resulting image:

- Uses `python:3.12-alpine` as runtime
- Includes only curl + CA certs and a dedicated virtualenv
- Runs as non-root user `appuser` (UID 10001)
- Has a Docker `HEALTHCHECK` hitting `/health`

---

## HTTP API

All endpoints are HTTP GET.

| Path       | Description                                                            | Status codes                                |
| ---------- | ---------------------------------------------------------------------- | ------------------------------------------- |
| `/`        | Simple banner: `"python-django-app is running (Python + Django)\n"`    | `200`                                       |
| `/info`    | Service metadata (`service`, `version`, `buildTime`) as JSON           | `200`                                       |
| `/health`  | Liveness probe (always `200` in this template)                         | `200`                                       |
| `/ready`   | Readiness probe (based on in-memory readiness state)                   | `200` if accepting traffic, `503` otherwise |
| `/metrics` | Prometheus metrics (text exposition format, Prometheus client library) | `200`                                       |

In a real system, `/ready` would incorporate dependency checks (DB, downstream services, etc.).

Examples (Docker, mapped to host port `8083`):

```bash
curl http://localhost:8083/
curl http://localhost:8083/info
curl http://localhost:8083/health
curl http://localhost:8083/ready
curl http://localhost:8083/metrics
```

The readiness behavior is controlled by `infra.readiness.state`, which can be toggled in code or tests.

---

## Observability

### Request correlation (`X-Request-ID`)

- If the client sends `X-Request-ID`, the service reuses it
- Otherwise, the service generates one
- The value is always returned in the response header `X-Request-ID`
- It is also stored as `request_id` and attached to all log records (MDC-like)

Example:

```bash
curl -i -H "X-Request-ID: demo-123" http://localhost:8083/info
```

### Structured logging

Logging is handled by Django’s logging configuration with `python-json-logger`:
- Logs go to `stdout` as JSON
- A request-id logging filter attaches `request_id` to every log record (ContextVar)
- HTTP access logs use the dedicated logger `"http"` (middleware)
- Infra endpoints (`/health`, `/ready`, `/metrics`) are not logged to keep noise low (but still counted in metrics)
- Duplicate Django 404 logs are suppressed (middleware is the single source of HTTP access logs)

HTTP access logs:
- Message: `http_request`
- Fields include:
  - `service`, `version`, `buildTime`
  - `request_id`
  - `status`, `method`
  - `path` (stable route label)
  - `rawPath` (actual raw path)
  - `query`
  - `ip` (real client IP when trusted proxies configured)
  - `latencyMs`
  - `userAgent`

Severity:
- `2xx/3xx` → `INFO`
- `4xx` → `WARNING`
- `5xx` → `ERROR`

Example log (shape):

```json
{
  "asctime": "2025-12-19 09:19:45,026",
  "levelname": "INFO",
  "name": "http",
  "process": 7,
  "threadName": "Thread-1",
  "message": "http_request",
  "request_id": "demo-123",
  "service": "python-django-app",
  "version": "1.0.0",
  "buildTime": "2025-12-19T09:18:13Z",
  "status": 200,
  "method": "GET",
  "path": "/info",
  "rawPath": "/info",
  "query": "",
  "ip": "172.17.0.1",
  "latencyMs": 0.198,
  "userAgent": "Mozilla/5.0 ..."
}
```

---

### Prometheus metrics

Metrics are implemented via `prometheus_client` and exposed directly on `/metrics`:
- `infra.metrics` defines a dedicated registry and metric objects
- Metrics scraped via `metrics_view` → `scrape_metrics()`

Key metrics:
- `http_requests_total{service,method,path,status}` (counter)
- `http_request_duration_seconds{service,method,path,status}` (histogram)
- `build_info{service,version,build_time}` (gauge with value `1`)

**Important**: `path` is a stable label.

For matched routes it is the Django route pattern (e.g. `/info`)

For unmatched routes (404s) it is `__unmatched__` to prevent label cardinality blowups

Raw paths are still available in logs via `rawPath`

Example check:

```bash
curl -s http://localhost:8083/metrics | grep http_requests_total
```

---

## Configuration

Configuration is driven by environment variables, parsed and validated in `django_app.settings` via small helper functions.

In production, Gunicorn process-level timeouts are configured via environment variables and should generally be chosen in line with `READ_TIMEOUT` and `IDLE_TIMEOUT`.

### Core env vars

| Variable            | Default               | Description                                                   |
| ------------------- | --------------------- | ------------------------------------------------------------- |
| `HOST`              | `0.0.0.0`             | Listen address (used for metadata; Gunicorn binds `0.0.0.0`)  |
| `PORT`              | `8080`                | HTTP port (1–65535)                                           |
| `LOG_LEVEL`         | `INFO`                | Log level (`DEBUG`, `INFO`, `WARNING`, `ERROR`, etc.)         |
| `READ_TIMEOUT`      | `5s`                  | Read timeout (seconds, parsed as simple duration)             |
| `IDLE_TIMEOUT`      | `120s`                | Idle/keep-alive timeout (seconds)                             |
| `SHUTDOWN_TIMEOUT`  | `5s`                  | App-level shutdown intent (seconds)                           |
| `DEBUG`             | `false`               | Django debug mode (`true`/`false`)                            |
| `DJANGO_SECRET_KEY` | `insecure-dev-secret` | Django secret key                                             |
| `TRUSTED_PROXIES`   | (empty)               | Comma-separated trusted proxy CIDRs/IPs for `X-Forwarded-For` |

Timeouts are parsed via `env_duration_seconds` and must be strictly > 0.

### Trusted proxies

By default, the service does not trust forwarded headers.
If you run behind a reverse proxy (ALB/Nginx), set `TRUSTED_PROXIES` so client IP is resolved correctly.

Example:

```bash
export TRUSTED_PROXIES="10.0.0.0/8,172.16.0.0/12"
```

### Gunicorn env vars

| Variable                    | Default   | Description                        |
| --------------------------- | --------- | ---------------------------------- |
| `GUNICORN_WORKERS`          | `2`       | Worker processes                   |
| `GUNICORN_WORKER_CLASS`     | `gthread` | Worker type                        |
| `GUNICORN_THREADS`          | `4`       | Threads per worker                 |
| `GUNICORN_TIMEOUT`          | `30`      | Worker timeout (seconds)           |
| `GUNICORN_KEEPALIVE`        | `5`       | Keep-alive seconds                 |
| `GUNICORN_GRACEFUL_TIMEOUT` | `5`       | Graceful shutdown window (seconds) |

### Build metadata

Build metadata is provided via env vars and surfaced through `/info` and `build_info`:

| Variable       | Default             | Description          |
| -------------- | ------------------- | -------------------- |
| `SERVICE_NAME` | `python-django-app` | Logical service name |
| `VERSION`      | `0.0.0-dev`         | Build version        |
| `BUILD_TIME`   | `unknown`           | Build timestamp      |

Verify:

```bash
curl http://localhost:8083/info
# => {"service":"python-django-app","version":"1.0.0","buildTime":"2025-12-19T09:18:13Z"}
```

---

## `.env` profiles

The project includes environment files for convenience:

- `.env.dev` – local development (DEBUG, short timeouts)
- `.env.int` – integration / prod-like
- `.env.prod` – production baseline

Example `.env.int`:

```env
DEBUG=false
HOST=0.0.0.0
PORT=8080

LOG_LEVEL=INFO

READ_TIMEOUT=2s
IDLE_TIMEOUT=30s
SHUTDOWN_TIMEOUT=5s

DJANGO_SECRET_KEY=dev-secret-key

# Optional: only if running behind proxies you trust
# TRUSTED_PROXIES=10.0.0.0/8,172.16.0.0/12

# Gunicorn
GUNICORN_WORKERS=2
GUNICORN_WORKER_CLASS=gthread
GUNICORN_THREADS=4
GUNICORN_TIMEOUT=30
GUNICORN_KEEPALIVE=5
GUNICORN_GRACEFUL_TIMEOUT=5
```

---

## Testing

Tests live in `infra/tests.py` and cover:

- Infra endpoints:
  - `/`, `/info`, `/health`, `/ready`, `/metrics`
  - Readiness state behavior (`/ready` → `503` when not accepting)
  - `X-Request-ID` is always present in responses (generated if missing)
  - Incoming `X-Request-ID` is echoed back

- HTTP logging & metrics middleware:

  - Infra endpoints record metrics but do not log access lines
  - Application endpoints produce `http_request` logs with structured fields
  - 404s use stable metrics label: `path="__unmatched__"`

Run tests:

```bash
cd python-django/app
export DJANGO_SETTINGS_MODULE=django_app.settings
python manage.py test
```

Docker build can run tests too:

```bash
docker build --build-arg RUN_TESTS=true -t python-django-app:test .
docker build --build-arg RUN_TESTS=false -t python-django-app:fast .
```

---

## Project Structure

```text
python-django/
├── Dockerfile
├── .dockerignore
├── requirements.txt
├── .env.dev
├── .env.int
├── .env.prod
└── app/
    ├── manage.py                     # Django entrypoint
    ├── entrypoint.py                 # Gunicorn exec launcher (no shell)
    ├── django_app/
    │   ├── __init__.py
    │   ├── asgi.py
    │   ├── settings.py               # Env-driven config + logging
    │   ├── urls.py                   # Routes: /, /info, /health, /ready, /metrics
    │   └── wsgi.py                   # WSGI entrypoint for Gunicorn
    └── infra/
        ├── __init__.py
        ├── admin.py
        ├── apps.py                   # Logs infra_app_ready on startup
        ├── metrics.py                # Prometheus registry + metrics
        ├── request_id.py             # request_id contextvar + log filter
        ├── middleware.py             # HTTP logging + metrics + request id
        ├── models.py
        ├── readiness.py              # ReadinessState holder
        ├── tests.py                  # Infra + middleware tests
        └── views.py                  # Implements /, /info, /health, /ready, /metrics
```

---

## Notes

- This service intentionally has **minimal business logic** and **strong operational patterns**:
  - health/readiness probes
  - Prometheus metrics with stable labels
  - JSON structured logging with request correlation
  - Gunicorn-based runtime with graceful shutdown
  - Docker best practices (multi-stage build, non-root, healthcheck, no shell entrypoint)
- In the full `docker-polyglot-lab` project, there are equivalent services in:
  - Go + Gin (`golang-gin`)
  - Java + Spring Boot (`java-springboot`)

You can use this Django service as a template for new Python microservices with production-friendly defaults that align with the Go and Java implementations.
