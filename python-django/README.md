# /python-django — **Python + Django** HTTP service (**metrics + traces + structured logs**)

A small, production-shaped HTTP service built with **Python 3.12** and **Django 6.0.2**, designed to run standalone or as part of the repo’s Docker Compose lab stack.

It runs behind **Gunicorn 25.0.1** and exports the standard repo signals:

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
| **Compose host port** | `127.0.0.1:8083` |
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

Compose wiring for this service (from `docker/compose._apps.yaml`):
- service name: **`python-django-app`**
- host port: **`127.0.0.1:8083`**
- healthcheck: `GET http://127.0.0.1:8080/ready`
- env file selection: `../python-django/.env.${APP_ENV:-integration}`

</details>

<details>
<summary><strong>Run locally (without Docker)</strong></summary>

Requires:
- **Python 3.12**
- `pip` (venv recommended)

```bash
cd python-django
python3.12 -m venv .venv
. .venv/bin/activate

python -m pip install --upgrade pip
python -m pip install --require-hashes -r requirements.lock
python -m pip install -r requirements.test.txt

set -a; source .env.development; set +a

python app/manage.py runserver 0.0.0.0:8080
```

> [!NOTE]
> `runserver` is a development server. For production-parity behavior (Gunicorn, graceful shutdown, OTel instrumentation wrapper, and the same startup flags used in Compose), run the container or use `entrypoint.sh`.

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
The Dockerfile builds a locked venv, optionally runs tests in a separate stage, and injects build metadata:

```bash
cd python-django

docker build \
  --build-arg RUN_TESTS=true \
  --build-arg SERVICE_NAME=python-django-app \
  --build-arg VERSION=${VERSION:-integration} \
  --build-arg BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -t python-django-app:${VERSION:-integration} .
```

#### Run (using `.env.integration`)
```bash
docker run -d \
  --name python-django-app \
  --restart unless-stopped \
  --env-file .env.integration \
  -p 8083:8080 \
  python-django-app:${VERSION:-integration}
```

#### Check status
```bash
docker ps
docker logs -f python-django-app
```

</details>

---

## What this service provides

- **Infra endpoints**: `GET /`, `GET /health`, `GET /ready`, `GET /info`
- **Prometheus metrics**: `GET /metrics` (custom registry + process/platform/gc collectors)
- **OpenTelemetry tracing**: WSGI + Django instrumentation via `opentelemetry-instrument` (enabled by default in Docker)
- **Structured JSON logs**: canonical field ordering via `python-json-logger`
- **Request correlation**: strict **`X-Request-ID`** policy (contextvars + response header + span attribute)
- **Payload size limiting**: `MAX_BODY_BYTES` enforced at multiple layers (fast-fail + Django upload limits + `RequestDataTooBig` mapping)
- **Readiness state**: in-process toggle (`infra.readiness.state`), used by `GET /ready`
- **Graceful shutdown**: SIGINT/SIGTERM with configurable shutdown timeout
- Hardened container build (non-root runtime; multi-stage build; optional tests)

---

## HTTP API

### Network
- Binds to: `${HOST}:${PORT}` (defaults: **`0.0.0.0:8080`**)
- Container port: **`8080`**
- Repo compose port mapping: **`127.0.0.1:8083 → 8080`** (service: **`python-django-app`**)

### Endpoints
| Method | Path | Purpose |
|---|---|---|
| GET | `/` | Plain-text “service is running” message |
| GET | `/health` | Liveness probe (**200**) |
| GET | `/ready` | Readiness probe (**200** if accepting traffic, else **503**) |
| GET | `/info` | Build/service metadata (JSON) |
| GET | `/metrics` | Prometheus scrape endpoint |

<details>
<summary><strong>Sample: `/info` response</strong></summary>

```json
{
  "service": "python-django-app",
  "version": "<version>",
  "build_time": "2026-02-03T07:22:30Z"
}
```

</details>

<details>
<summary><strong>Sample: `/info` log event (JSON)</strong></summary>

```json
{
  "time": "2026-02-03T07:28:22.661249Z",
  "level": "INFO",
  "msg": "http_request",
  "service": "python-django-app",
  "version": "<version>",
  "build_time": "2026-02-03T07:22:30Z",
  "request_id": "7fe0f262-9c9b-4d8a-b3e1-4ae5d79f62ca",
  "trace_id": "24b55ac4c6fc4506215b56998c8c6827",
  "span_id": "6b45f5ebcfe79e7f",
  "method": "GET",
  "path": "/info",
  "raw_path": "/info",
  "status": 200,
  "ip": "172.18.0.1",
  "latency_ms": 0.137,
  "bytes_written": 90,
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
- **400** → `bad_request`
- **403** → `forbidden`
- **404** → `not_found`
- **405** → `method_not_allowed` (also sets `Allow: GET` on infra routes)
- **413** → `payload_too_large`
- **500** → `internal_server_error`

---

## Configuration

Configuration is read from environment variables and applied in `django_app/settings.py` and the Docker entrypoint.

### Core app env vars (aligned with Go/Java)

These env vars intentionally mirror the other services where it makes sense (**HOST/PORT/timeouts/body limits/trusted proxies**), while adding Django-specific settings.

| Env var | Default | Notes |
|---|---:|---|
| `DJANGO_SECRET_KEY` | `development-secret-key` | required for real deployments |
| `DEBUG` | `false` | parsed from `true/false` |
| `HOST` | `0.0.0.0` | bind address used by `runserver`; in Docker Gunicorn binds `0.0.0.0:${PORT}` |
| `PORT` | `8080` | validated **1..65535** |
| `LOG_LEVEL` | `INFO` | root log level (applies to Django + app loggers) |
| `READ_TIMEOUT` | `5s` | parsed as seconds (supports `5s` or `5`) |
| `WRITE_TIMEOUT` | `10s` | parsed as seconds (supports `10s` or `10`) |
| `READ_HEADER_TIMEOUT` | `2s` | parsed as seconds (supports `2s` or `2`) |
| `IDLE_TIMEOUT` | `120s` | parsed as seconds (supports `120s` or `120`) |
| `SHUTDOWN_TIMEOUT` | `5s` | parsed as seconds (supports `5s` or `5`) |
| `MAX_BODY_BYTES` | `1048576` | validated **1..50 MiB**; used for Django upload limits and 413 behavior |
| `TRUSTED_PROXIES` | empty | optional CSV of CIDR/IP used for `X-Forwarded-For` parsing |

> Difference vs Go/Java: the middleware supports “disable limit when <= 0”, but `settings.py` validates `MAX_BODY_BYTES` as **>= 1**, so disabling requires a code change (intentionally strict).

Build metadata variables are available as runtime defaults (and are commonly set at image build time):

| Variable | Default | Where it shows up |
|---|---:|---|
| `SERVICE_NAME` | `python-django-app` | `/info` JSON, logs, (and labels where applicable) |
| `VERSION` | `0.0.0-dev` | `/info` JSON, logs, `build_info` metric label |
| `BUILD_TIME` | `unknown` | `/info` JSON, logs, `build_info` metric label |

### Gunicorn / runtime env vars (entrypoint)

`entrypoint.sh` starts Gunicorn with these knobs:

| Env var | Default | Used for |
|---|---:|---|
| `DJANGO_WSGI_MODULE` | `django_app.wsgi:application` | Gunicorn app module |
| `GUNICORN_CONFIG` | `/app/gunicorn.conf.py` | config file |
| `GUNICORN_WORKERS` | `1` | worker processes |
| `GUNICORN_WORKER_CLASS` | `gthread` | worker type |
| `GUNICORN_THREADS` | `4` | threads per worker |
| `GUNICORN_TIMEOUT` | `30` | seconds |
| `GUNICORN_KEEPALIVE` | `5` | seconds |
| `GUNICORN_GRACEFUL_TIMEOUT` | `5` | seconds |
| `DJANGO_MIGRATE_ON_STARTUP` | `false` | if true: `manage.py migrate --noinput` |
| `DJANGO_COLLECTSTATIC_ON_STARTUP` | `false` | if true: `manage.py collectstatic --noinput` |
| `OTEL_PYTHON_ENABLED` | `true` | if true and `opentelemetry-instrument` exists, wraps Gunicorn with OTel instrumentation |

### OpenTelemetry env vars (from `.env.*`)

> [!NOTE]
> This repo uses a **hybrid model**: Prometheus scrapes `/metrics`, while traces are exported via OTLP to Alloy.

<details>
<summary><strong>Click to expand OTel env var examples (development vs integration/staging)</strong></summary>

The repo env files configure tracing consistently with other modules:

#### `.env.development`
Tracing is disabled explicitly:
- `OTEL_TRACES_EXPORTER=none`
- `OTEL_METRICS_EXPORTER=none`
- `OTEL_LOGS_EXPORTER=none`

#### `.env.integration` / `.env.staging`
Traces are exported to Alloy (OTLP/HTTP protobuf), logs/metrics exporters remain disabled:
- `OTEL_SERVICE_NAME=python-django-app`
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

### URL routing
`django_app/urls.py` defines:
- routes: `/`, `/info`, `/health`, `/ready`, `/metrics`
- error handlers: `handler400/403/404/500` mapped to `infra.errors.*`

<details>
<summary><strong>Middleware order (outer → inner)</strong></summary>

Configured in `django_app/settings.py`:

1. **`infra.middleware.RequestIdMiddleware`**
   - validates inbound `X-Request-ID`:
     - trim; non-empty; length ≤ 128
     - rejects `\r`, `\n`, `\t`
     - allowed chars: `^[A-Za-z0-9._\-:]+$`
   - if invalid/missing: generates UUID
   - stores id on request (`request.request_id`) and in `contextvars`
   - echoes `X-Request-ID` in the response
   - attaches `request_id` attribute to the active span (best-effort)

2. **`django.middleware.security.SecurityMiddleware`** (standard)
3. **`django.middleware.common.CommonMiddleware`** (standard)

4. **`infra.middleware.HttpLoggingAndMetricsMiddleware`**
   - records Prometheus metrics per request
   - emits structured access logs to logger name `http` with message `http_request`
   - determines client IP:
     - if `REMOTE_ADDR` is in `TRUSTED_PROXIES`, uses `X-Forwarded-For` chain to pick the first untrusted hop
     - otherwise uses `REMOTE_ADDR` directly
   - log suppression for successful infra paths: `/health`, `/ready`, `/metrics`
   - log levels: `INFO` (<400), `WARNING` (4xx), `ERROR` (5xx/exception)
   - maps Django `RequestDataTooBig` to a stable 413 response

5. **`infra.middleware.MaxBodyBytesMiddleware`**
   - fast-fails based on `Content-Length` if `> MAX_BODY_BYTES` → 413

---

</details>

---

## Payload validation & limits

### `MAX_BODY_BYTES` (default **1 MiB**)
Enforced in **three places**:

1. **Django request upload limits**
   - `DATA_UPLOAD_MAX_MEMORY_SIZE = MAX_BODY_BYTES`
   - `FILE_UPLOAD_MAX_MEMORY_SIZE = MAX_BODY_BYTES`
   - if exceeded during parsing: Django raises `RequestDataTooBig`, which is mapped to **413**.

2. **Fast fail middleware**
   - `MaxBodyBytesMiddleware` checks `Content-Length` early and returns 413 when `> MAX_BODY_BYTES`.

3. **Stable 413 JSON**
   - `infra.errors.payload_too_large` returns:
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

`/metrics` uses **prometheus-client 0.24.1** with a module-scoped `CollectorRegistry` and collectors:
- process
- platform
- gc

Custom metrics:
- **`http_requests_total{method,path,status}`**
- **`http_request_duration_seconds{method,path,status}`** (buckets: 5ms..10s)
- **`build_info{version,build_time}`** (gauge set to 1)

Path labeling (low cardinality):
- 404 → `__unmatched__`
- else uses Django’s resolver route template (`request.resolver_match.route`)
  - empty route maps to `/`
- infra paths keep raw path
- otherwise falls back to `__unmatched__`

Access log suppression:
- successful `/health`, `/ready`, `/metrics` are **not logged** (but are still counted in metrics).

</details>

<details>
<summary><strong>Tracing (OpenTelemetry)</strong></summary>

- Docker entrypoint wraps Gunicorn with `opentelemetry-instrument` when `OTEL_PYTHON_ENABLED=true`.
- The request middleware attaches `request_id` to the active span attribute `request_id` (best-effort).

Repo env files typically set:
- `OTEL_TRACES_EXPORTER=otlp`
- `OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf`
- `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT=http://alloy:4318/v1/traces`
- `OTEL_TRACES_SAMPLER=always_on`

</details>

<details>
<summary><strong>Logs</strong></summary>

Structured logs are configured in `infra/logging_config.py` and used by both Django and Gunicorn:

- canonical JSON schema with stable field ordering
- service metadata fields injected automatically:
  - `service`, `version`, `build_time`
- request correlation via `request_id` (contextvars)
- trace correlation via live span context:
  - `trace_id`, `span_id` when a valid span exists

Gunicorn access logs are disabled (`accesslog = os.devnull`) because the app emits its own access logs.

---

</details>

---

## Error utilities

Centralized JSON error utilities in `infra/errors.py`:
- stable `error` + `code` + `request_id`
- handler functions wired at the URLconf level: `handler400/403/404/500`
- `method_not_allowed` returns 405 and sets `Allow` when provided

---

## Operational knobs

This service’s “knobs” are primarily Gunicorn settings and request limits:

- concurrency: `GUNICORN_WORKERS`, `GUNICORN_THREADS`, `GUNICORN_WORKER_CLASS`
- timeouts: `GUNICORN_TIMEOUT`, `GUNICORN_KEEPALIVE`, `GUNICORN_GRACEFUL_TIMEOUT`
- request limits: `MAX_BODY_BYTES`, `STACK_TRACE_MAX_CHARS`
- proxy trust boundary: `TRUSTED_PROXIES` (affects client IP attribution)

---

## Testing & linting

Unit tests + coverage:
```bash
cd python-django
python3.12 -m venv .venv
. .venv/bin/activate

python -m pip install --require-hashes -r requirements.lock
python -m pip install -r requirements.test.txt

DJANGO_SETTINGS_MODULE=django_app.settings   pytest --cov --cov-report=xml --cov-fail-under=100
```

Formatting + lint + typecheck:
```bash
ruff format --check .
ruff check .
mypy app
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
- **python3.12**
- network access (to install CI tools into an isolated venv)

The CI runner uses tool versions from `./.ci-tool-versions.sh`:
- `PIP_MAX_VERSION=26`
- `PIP_TOOLS_VERSION=7.5.2`
- `PIP_AUDIT_VERSION=2.10.0`

### Run the Python module CI checks
From repo root:

```bash
source ./.ci-tool-versions.sh
./.ci-local.sh python
```

What it runs for this module (in order):
- creates an isolated venv `.venv-ci` (removable via `CI_LOCAL_CLEAN_VENV=1`)
- installs `pip<${PIP_MAX_VERSION}` and `pip-tools==${PIP_TOOLS_VERSION}`
- regenerates `requirements.lock` with hashes from `requirements.txt` and enforces no diff:
  - `pip-compile --no-strip-extras --generate-hashes --output-file=requirements.lock requirements.txt`
  - `git diff --exit-code -- requirements.lock`
- installs locked runtime deps and test deps
- validates package state: `pip check`, `python -m compileall -q app`
- formatting: `ruff format --check .`
- lint: `ruff check .`
- typecheck:
  - **pull_request**: `mypy app` is **non-blocking**
  - **push/tags**: `mypy app` is **blocking**
- tests + coverage:
  - `pytest --cov --cov-report=xml --cov-fail-under=100` (writes `coverage.xml`)
  - coverage gate: **100% statements** via `--cov-fail-under=100` (fails if < 100%)
- security (push/tags only):
  - `pip-audit -r requirements.lock` (+ JSON report `pip-audit.json`)

Useful options:
```bash
./.ci-local.sh doctor python --summary
LOG_LEVEL=debug ./.ci-local.sh python
CI_EVENT_NAME=pull_request ./.ci-local.sh python
CI_LOCAL_CLEAN_VENV=0 ./.ci-local.sh python   # keep .venv-ci for debugging
```

---

## Container image details

### Dockerfile highlights
- Multi-stage build:
  - **builder**: creates `/venv`, installs from locked requirements, copies app
  - **test**: separate venv `/venv-test`, installs test deps, runs pytest when `RUN_TESTS=true`
  - **runtime**: copies `/venv` and app into slim Python image
- Runs as non-root user (**uid=10001**), `STOPSIGNAL SIGTERM`
- Injects build metadata into runtime env: `SERVICE_NAME`, `VERSION`, `BUILD_TIME`

### Build arguments
| Arg | Default | Purpose |
|---|---:|---|
| `PYTHON_VERSION` | `3.12` | base runtime version |
| `DEBIAN_SUITE` | `bookworm` | base image variant |
| `APT_BUILD_DEPS` | empty | optional apt deps for building wheels |
| `REQUIREMENTS_FILE` | `requirements.lock` | allows choosing the lock file |
| `RUN_TESTS` | `true` | controls test stage execution |
| `SERVICE_NAME` | `python-django-app` | Default runtime env + OCI labels (and used by `/info`). |
| `VERSION` | `dev` | Default runtime env + OCI labels (and used by `/info`). |
| `BUILD_TIME` | `local` | Default runtime env + OCI labels (and used by `/info`). |
| `OCI_IMAGE_SOURCE` | "" | Sets OCI label `org.opencontainers.image.source` (typically the repository URL; often populated in CI via `docker/build-push-action` `labels:` or `docker/metadata-action`). |
| `OCI_IMAGE_REVISION` | "" | Sets OCI label `org.opencontainers.image.revision` (typically the Git commit SHA, e.g. GitHub Actions `GITHUB_SHA`; often populated in CI via `docker/build-push-action` `labels:` or `docker/metadata-action`). |

---

## Implementation map (where to look)

| Change you want | Where to look |
|---|---|
| **`pyproject.toml`** | Ruff configuration (line length, lint rules) |
| **`app/django_app/settings.py`** | env parsing, middleware wiring, payload limits |
| **`app/django_app/urls.py`** | routing + error handler wiring |
| **`app/infra/views.py`** | endpoints (`/`, `/info`, `/health`, `/ready`, `/metrics`) |
| **`app/infra/middleware.py`** | request id, client IP, access logs, metrics, payload fast-fail |
| **`app/infra/errors.py`** | stable JSON errors |
| **`app/infra/metrics.py`** | Prometheus registry + meters + buckets |
| **`app/infra/logging_config.py`** | JSON log schema + correlation filters |
| **`entrypoint.sh`** | Gunicorn startup + optional OTel instrumentation |
| **`gunicorn.conf.py`** | Gunicorn log config (app JSON logs; access log suppressed) |

> [!TIP]
> Prefer starting with the **request pipeline** (middleware/filter chain), then jump to metrics/logging/tracing wiring.

---

## Interactions with other modules

- `/docker` runs this service as `python-django-app`, maps it to `127.0.0.1:8083`, scrapes `/metrics`, and routes OTLP traces via Alloy → Tempo.
- `/golang-gin` and `/java-springboot` follow the same **health/ready/metrics/info + request-id + OTLP** conventions so dashboards/alerts can be shared.
