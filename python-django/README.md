# python-django-app

A small, production-style HTTP service built with **Python 3.12**, **Django**, and **Gunicorn**, designed as the Django component of the `docker-polyglot-lab` project.

This service focuses on **operational concerns** rather than business logic:

- Health & readiness endpoints
- Structured JSON logging via `python-json-logger`
- Prometheus metrics (`/metrics`) via `prometheus_client`
- Graceful shutdown with Gunicorn
- Configuration via environment variables
- Multi-stage Docker build, non-root runtime

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
cd docker-polyglot-lab/python-django

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

Example logs:

```bash
[2025-12-16 09:49:42 +0000] [1] [INFO] Starting gunicorn 23.0.0
[2025-12-16 09:49:42 +0000] [1] [INFO] Listening at: http://0.0.0.0:8080 (1)
{"asctime": "2025-12-16 09:49:42,645", "levelname": "INFO", "name": "infra", "message": "infra_app_ready"}
{"asctime": "2025-12-16 09:50:08,624", "levelname": "INFO", "name": "http", "message": "http_request service=python-django-app version=1.0.0 method=GET path=/ rawPath=/ status=200 ip=172.17.0.1 latencyMs=0.257 userAgent='Mozilla/5.0 ...'"}
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

### Structured logging

Logging is handled by Django’s logging configuration with `python-json-logger`:

- `django_app.settings.LOGGING` configures a JSON console handler
- Root and app-specific loggers write structured JSON to `stdout`
- HTTP access logs use the dedicated logger `"http"` (configured in `HttpLoggingAndMetricsMiddleware`)
- Infra endpoints (`/health`, `/ready`, `/metrics`) are not logged to keep noise low, but they are still instrumented for metrics

Example HTTP log (from `HttpLoggingAndMetricsMiddleware`):

```json
{
  "asctime": "2025-12-16 09:50:17,645",
  "levelname": "INFO",
  "name": "http",
  "message": "http_request service=python-django-app version=1.0.0 method=GET path=/info rawPath=/info status=200 ip=172.17.0.1 latencyMs=0.143 userAgent='Mozilla/5.0 ...'"
}
```

Startup logs from the `infra` app:

```json
{
  "asctime": "2025-12-16 09:49:42,645",
  "levelname": "INFO",
  "name": "infra",
  "message": "infra_app_ready"
}
```

### Prometheus metrics

Metrics are implemented via `prometheus_client` and exposed directly on /metrics`:
- `infra.metrics` defines a dedicated registry and metric objects
- Metrics scraped via `metrics_view` → `scrape_metrics()`

Key metrics:
- `http_requests_total{service,method,path,status}` (counter)
- `http_request_duration_seconds{service,method,path,status}` (histogram)
- `build_info{service,version,build_time}` (gauge with value `1`)

All HTTP requests are instrumented via `HttpLoggingAndMetricsMiddleware`, using:
- `method` (HTTP verb)
- `path` (Django route pattern when resolvable, e.g. `/info`)
- `status` (HTTP status code)
- Request duration in seconds

Example check:

```bash
curl -s http://localhost:8083/metrics | grep http_requests_total
```

---

## Configuration

Configuration is driven by environment variables, parsed and validated in `django_app.settings` via small helper functions.

In production, HTTP process-level timeouts (`GUNICORN_TIMEOUT`, `GUNICORN_KEEPALIVE`) are configured via environment variables and should generally be chosen in line with `READ_TIMEOUT` and `IDLE_TIMEOUT`.

### Core env vars

| Variable            | Default               | Description                                                  |
| ------------------- | --------------------- | ------------------------------------------------------------ |
| `HOST`              | `0.0.0.0`             | Listen address (used for metadata; Gunicorn binds `0.0.0.0`) |
| `PORT`              | `8080`                | HTTP port (1–65535)                                          |
| `LOG_LEVEL`         | `INFO`                | Log level (`DEBUG`, `INFO`, `WARNING`, `ERROR`, etc.)        |
| `READ_TIMEOUT`      | `5s`                  | Read timeout (seconds, parsed as simple duration)            |
| `IDLE_TIMEOUT`      | `120s`                | Idle/keep-alive timeout (seconds)                            |
| `SHUTDOWN_TIMEOUT`  | `5s`                  | Graceful shutdown timeout (seconds)                          |
| `DEBUG`             | `false`               | Django debug mode (`true`/`false`)                           |
| `DJANGO_SECRET_KEY` | `insecure-dev-secret` | Django secret key                                            |

Timeouts are parsed via `env_duration_seconds` and must be strictly > 0.

### Build metadata

Build metadata is provided via env vars and surfaced through `/info` and `build_info`:

| Variable       | Default             | Description          |
| -------------- | ------------------- | -------------------- |
| `SERVICE_NAME` | `python-django-app` | Logical service name |
| `VERSION`      | `0.0.0-dev`         | Build version        |
| `BUILD_TIME`   | `unknown`           | Build timestamp      |

These map directly to:

```python
SERVICE_NAME = env_str("SERVICE_NAME", "python-django-app")
VERSION = env_str("VERSION", "0.0.0-dev")
BUILD_TIME = env_str("BUILD_TIME", "unknown")
```

The Dockerfile injects these values at build time:

```dockerfile
ARG SERVICE_NAME=python-django-app
ARG VERSION=dev
ARG BUILD_TIME=local

ENV SERVICE_NAME=${SERVICE_NAME} \
    VERSION=${VERSION} \
    BUILD_TIME=${BUILD_TIME}
```

You can verify them via:

```bash
curl http://localhost:8083/info
# => {"service":"python-django-app","version":"1.0.0","buildTime":"2025-12-16T09:49:42Z"}
```

### Validation behavior

Helpers in `django_app.settings` enforce:

- `env_int` validates port ranges (`min_val=1`, `max_val=65535`)
- `env_str` can be marked `required=True` (raises `ImproperlyConfigured` if missing/empty)
- `env_duration_seconds` enforces durations > 0 and accepts values like `5`, `5s`, `120s`

On invalid settings, Django will fail to start with an explicit error.

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
```

Example Docker run using `.env.int`:

```bash
docker run -d \
  --name python-django-app \
  --restart unless-stopped \
  --env-file .env.int \
  -p 8083:8080 \
  python-django-app:1.0.0
```

---

## Testing

Tests live in `infra/tests.py` and cover:

- ### Infra endpoints
  - `InfraEndpointsTest`:
    - `/` returns the banner text `"python-django-app is running (Python + Django)\n"`
    - `/info` returns JSON with `service`, `version`, `buildTime`
    - `/health` and `/ready` return `200` by default
    - `/ready` returns `503` when readiness is set to not accepting traffic
    - `/metrics` contains `http_requests_total` after traffic hits `/`

- ### HTTP logging & metrics middleware
  - `LoggingAndMetricsMiddlewareTest`:
    - Infra endpoints (`/health`, `/metrics`) still record metrics
    - Application endpoints (like `/`) produce `"http_request"` logs on the `"http"` logger

Run tests:

```bash
cd python-django/app
export DJANGO_SETTINGS_MODULE=django_app.settings
python manage.py test
```

The Docker build also supports toggling tests:

```bash
# Run full test suite during image build
docker build --build-arg RUN_TESTS=true -t python-django-app:test .

# Skip tests (e.g. if already run in CI)
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
        ├── middleware.py             # HTTP logging + metrics middleware
        ├── models.py
        ├── readiness.py              # ReadinessState holder
        ├── tests.py                  # Infra + middleware tests
        └── views.py                  # Implements /, /info, /health, /ready, /metrics
```

---

## Notes

- This service intentionally has **minimal business logic** and **strong operational patterns**:
  - health/readiness probes
  - Prometheus metrics
  - JSON structured logging
  - Gunicorn-based runtime
  - Docker best practices (multi-stage build, non-root, healthcheck)
- In the full `docker-polyglot-lab` project, there are equivalent services in:
  - Go + Gin (`golang-gin-app`)
  - Java + Spring Boot (`java-springboot-app`)

You can use this Django service as a template for new Python microservices with production-friendly defaults that align with the Go and Java implementations.
