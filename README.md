# docker-polyglot-lab

[![CI](https://github.com/johnjaysonlpz/docker-polyglot-lab/actions/workflows/cicd.yml/badge.svg)](https://github.com/johnjaysonlpz/docker-polyglot-lab/actions/workflows/cicd.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

A **polyglot microservice lab** focused on *operational excellence* rather than business logic.

Three minimal HTTP services (**same endpoints, same observability contract**), each in a different stack:

- **Go 1.25.4 + Gin 1.11.0** (`golang-gin/`)
- **Java 21 + Spring Boot 3.5.8** (`java-springboot/`)
- **Python 3.12 + Django 5.2.9 + Gunicorn 23.0.0** (`python-django/`)

All three expose the same HTTP API:

- `/`        – banner
- `/info`    – build/service metadata
- `/health`  – liveness probe
- `/ready`   – readiness probe
- `/metrics` – Prometheus metrics (text format)

Everything is wired to run **locally with Docker Compose** in **dev / integration / prod-like** environments, with **Prometheus** scraping all services and **Grafana** provisioned for dashboards.

This repo is designed to showcase:

- Multi-stage Dockerfiles, non-root containers, healthchecks
- Environment-specific Docker Compose (dev / int / prod)
- Structured JSON logging (true JSON fields, not string-parsed messages)
- Request correlation via `X-Request-ID` (returned in responses + attached to logs)
- Prometheus metrics across Go, Java, and Python
- Stable metrics labels:
  - 404s use `path="__unmatched__"` to avoid cardinality blowups
  - “path” labels represent route templates (raw paths stay in logs)
- Prometheus + Grafana wiring + dashboard provisioning
- Config driven by env vars with validation
- Tests integrated into Docker builds (integration stack)

---

## Components

### 1) Go + Gin — `golang-gin/`

- Lightweight HTTP service using `gin-gonic/gin`
- Structured JSON logging (stdout)
- Prometheus metrics via `prometheus/client_golang`
- Env-driven config with validation
- Multi-stage Docker build → small runtime, non-root user

See: [`golang-gin/README.md`](./golang-gin/README.md)

---

### 2) Java + Spring Boot — `java-springboot/`

- Spring Boot 3.5.x, Java 21
- Structured JSON logging (stdout)
- Micrometer + Prometheus registry (`/metrics`)
- `@ConfigurationProperties` + validation for config
- Graceful shutdown + runtime tuning
- Multi-stage Docker build (builder → runtime), non-root user

See: [`java-springboot/README.md`](./java-springboot/README.md)

---

### 3) Python + Django — `python-django/`

- Django (minimal settings) + Gunicorn runtime
- Structured JSON logging with `python-json-logger` (real fields)
- Request correlation (`X-Request-ID`) stored as `request_id` on log records
- Prometheus metrics via `prometheus_client` + custom registry
- Env-driven config with validation helpers
- Multi-stage Docker build with builder + runtime, dedicated venv, non-root user

See: [`python-django/README.md`](./python-django/README.md)

---

### 4) Docker / Compose / Prometheus / Grafana — `docker/`

This directory contains:

- `compose.dev.yml` – dev: three services only
- `compose.int.yml` – integration: three services + Prometheus + Grafana, builds with tests
- `compose.prod.yml` – prod-like: pre-built images + Prometheus + Grafana
- `prometheus/prometheus.yml` – scrape config (**adds consistent `service` label at scrape-time**)
- `grafana/provisioning/` – provisioning layout:
  - `dashboards/` (provider + dashboard JSON)
  - `datasources/` (Prometheus datasource)

See: [`docker/README.md`](./docker/README.md)

---

## Quick start

### Prerequisites

- Docker (Docker Desktop or Docker Engine)
- Optional for running without Docker:
  - Go **1.25+**
  - Java **21** + Maven **3.9+**
  - Python **3.12** + `venv`

Clone:

```bash
git clone https://github.com/johnjaysonlpz/docker-polyglot-lab.git
cd docker-polyglot-lab
```

---

## Run everything with Docker Compose

All Compose stacks bind ports to 127.0.0.1 (localhost-only) by default.

Dev environment (apps only)

```bash
docker compose -f compose.dev.yml up --build
```

Endpoints:

- Gin: `http://localhost:8081`
- Spring Boot: `http://localhost:8082`
- Django: `http://localhost:8083`

Stop:

```bash
docker compose -f compose.dev.yml down
```

---

## Integration environment (apps + Prometheus + Grafana; tests on build)

Builds images, runs tests during build, then starts the full stack:

```bash
BUILD_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ) \
  docker compose -f compose.int.yml up --build
```

Endpoints:
- Gin: `http://localhost:8081`
- Spring Boot: `http://localhost:8082`
- Django: `http://localhost:8083`
- Prometheus: `http://localhost:9090`
- Grafana: `http://localhost:3000`

Grafana credentials (defaults):
- user: `admin`
- pass: `admin`

Override safely (if your Compose supports env substitution for these):

```bash
GRAFANA_ADMIN_USER=admin \
GRAFANA_ADMIN_PASSWORD=change-me \
docker compose -f compose.int.yml up --build
```

Stop (keeps volumes by default):

```bash
docker compose -f compose.int.yml down
```

---

## Prod-like environment (pre-built images + Prometheus + Grafana)

Assumes images are already available in a registry (e.g. Docker Hub / GHCR):

```bash
docker compose -f compose.prod.yml up -d
```

Stop:

```bash
docker compose -f compose.prod.yml down
```

---

## Observability contract (shared across all services)

### Request correlation (`X-Request-ID`)
- If the client sends `X-Request-ID`, the service reuses it.
- Otherwise, the service generates one.
- The value is returned in the response header `X-Request-ID`.
- Logs include the correlated field `request_id` for easy tracing.

Example:

```bash
curl -i -H "X-Request-ID: demo-123" http://localhost:8083/info
```

### Metrics (Prometheus)

All services export Prometheus metrics on `/metrics`, including:
- `http_requests_total{service,method,path,status,...}`
- `http_request_duration_seconds{service,method,path,status,...}`
- `build_info{service,version,build_time}`

Prometheus adds a consistent `service` label at scrape-time so dashboards don’t depend
on language-specific instrumentation to supply `service`.

**Important**: `path` is kept stable:
- matched routes use a route template label
- 404s use `path="__unmatched__"` to prevent cardinality blowups
- raw paths (debuggable) live in logs, not metric labels

### Logging (structured JSON)

- Logs go to stdout as JSON with true fields (queryable in Loki/ELK)
- HTTP access logs are unified as an `"http_request"` event (or equivalent)
- Infra endpoints (`/health`, `/ready`, `/metrics`) are not access-logged to reduce noise
(but are still counted in metrics)

---

```text
docker-polyglot-lab/
├── .github/
│   └── workflows/
│       └── cicd.yml              # CI/CD pipeline (tests + Docker builds + registry publish)
├── docker/                       # Docker Compose + Prometheus + Grafana
├── golang-gin/                   # Go + Gin service
├── java-springboot/              # Java + Spring Boot service
├── python-django/                # Python + Django + Gunicorn service
├── LICENSE                       # MIT license
└── README.md                     # This file
```

---

## CI / Docker images

GitHub Actions runs on each push and pull request:
- Go / Gin tests
- Java / Spring Boot tests
- Python / Django tests
- A smoke Docker build for all three services

When you push a git tag like `v1.0.0`, CI can build production images for all services,
inject build metadata (version, build time, VCS revision, OCI labels), and push images
to a registry with tags like:
- `golang-gin-app:1.0.0`
- `java-springboot-app:1.0.0`
- `python-django-app:1.0.0`

The `docker/compose.prod.yml` file is designed to consume those pre-built images.

---

## Why this repo exists

This lab is intentionally **light on business logic** and **heavy on operational patterns**:
- health/readiness probes
- consistent logging and metrics across three stacks
- Docker images with build metadata + healthchecks + non-root runtime
- Compose environments (dev/int/prod-like) with networks and persistent volumes
- Prometheus wired to every service
- Grafana provisioned with datasource + dashboards

It can be used as:
- a **portfolio** piece to demonstrate Docker + observability skills
- a **template** for new microservices in Go, Java, or Python with production-style defaults

---

## Status

This is a personal lab project. There are **no stability or backwards-compatibility guarantees**:
directory layout, APIs, and Docker tags may change at any time.

Use it as a reference or template at your own risk.

## Contributing

Issues and pull requests are welcome, but this repo is primarily a personal learning lab.

If you open a PR, try to keep the three stacks aligned:
- same endpoints (`/`, `/info`, `/health`, `/ready`, `/metrics`)
- similar logging + metrics story
- tests for any behavior you change
- consistent Docker/Compose conventions across services

## License
MIT — see [LICENSE](./LICENSE).
