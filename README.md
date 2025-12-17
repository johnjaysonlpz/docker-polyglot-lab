# docker-polyglot-lab

[![CI](https://github.com/johnjaysonlpz/docker-polyglot-lab/actions/workflows/ci.yml/badge.svg)](https://github.com/johnjaysonlpz/docker-polyglot-lab/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

A **polyglot microservice lab** focused on *operational excellence* rather than business logic.

Three minimal HTTP services, each in a different stack:

- **Go 1.25.4 + Gin** (`golang-gin/`)
- **Java 21 + Spring Boot 3.5** (`java-springboot/`)
- **Python 3.12 + Django + Gunicorn** (`python-django/`)

All three expose the same basic HTTP API:

- `/`      – banner
- `/info`  – build/service metadata
- `/health` – liveness probe
- `/ready`  – readiness probe
- `/metrics` – Prometheus metrics (text format)

Everything is wired to run **locally with Docker**, and in **dev / integration / prod-like** Compose environments with **Prometheus** scraping all services and **Grafana** available for dashboards.

The goal of this repo is to showcase:

- Multi-stage Dockerfiles, non-root containers, healthchecks
- Environment-specific Docker Compose (dev / int / prod)
- Structured JSON logging (slog / Logback / python-json-logger)
- Prometheus metrics across Go, Java, and Python
- Grafana wired to Prometheus for visualization
- Config driven by env vars with validation
- Tests integrated into Docker builds (for int)

---

## Components

### 1. Go + Gin – `golang-gin/`

- Lightweight HTTP service using `gin-gonic/gin`
- JSON logging via `slog` (structured logs to stdout)
- Prometheus metrics via `prometheus/client_golang`
- Config via env vars, validated at startup
- Multi-stage Docker build → small Alpine runtime, non-root user

See: [`golang-gin/README.md`](./golang-gin/README.md)

---

### 2. Java + Spring Boot – `java-springboot/`

- Spring Boot 3.5.x, Java 21
- JSON logging via Logback + `logstash-logback-encoder`
- Micrometer + Prometheus registry, `/metrics` endpoint
- `@ConfigurationProperties` + Bean Validation for config
- Tomcat tuning + graceful shutdown
- Multi-stage Docker build (Maven builder → JRE runtime), non-root user

See: [`java-springboot/README.md`](./java-springboot/README.md)

---

### 3. Python + Django – `python-django/`

- Django (minimal settings) + Gunicorn runtime
- JSON logging with `python-json-logger`
- Prometheus metrics via `prometheus_client` + custom registry
- Env-driven config with validation helpers
- Multi-stage Docker build with builder + runtime, dedicated virtualenv, non-root user

See: [`python-django/README.md`](./python-django/README.md)

---

### 4. Docker / Compose / Prometheus / Grafana – `docker/`

This directory contains:

- `compose.dev.yml` – dev: three services only
- `compose.int.yml` – integration: three services + Prometheus + Grafana, builds with tests
- `compose.prod.yml` – prod-like: three pre-built images + Prometheus + Grafana
- `prometheus/prometheus.yml` – scrape config for all services

All Compose files use a shared network:

- `polyglot-net` – containers discover each other by service name:
  - `golang-gin-app`
  - `java-springboot-app`
  - `python-django-app`
  - `prometheus`
  - `grafana`

Grafana is pre-wired to use Prometheus as its primary datasource.

See: [`docker/README.md`](./docker/README.md)

---

## Quick start

### Prerequisites

- Docker (Docker Desktop or Docker Engine)
- Optional for local-only runs:
  - Go **1.25+**
  - Java **21** + Maven **3.9+**
  - Python **3.12** + `venv`

Clone the repo:

```bash
git clone https://github.com/johnjaysonlpz/docker-polyglot-lab.git
cd docker-polyglot-lab
```

---

## Running everything with Docker Compose

### Dev environment (apps only)

Build and run all services in dev mode:

```bash
cd docker
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

### Integration environment (apps + Prometheus, tests on build)

Builds images, runs tests during build, and starts Prometheus:

```bash
cd docker

BUILD_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ) \
  docker compose -f compose.int.yml up --build
```

Endpoints:

- Gin: `http://localhost:8081`
- Spring Boot: `http://localhost:8082`
- Django: `http://localhost:8083`
- Prometheus: `http://localhost:9090`
- Grafana: `http://localhost:3000`

Example Prometheus query:

- `http_requests_total`
- `http_request_duration_seconds_bucket`
- `build_info`

In Grafana, you can add dashboards using the Prometheus datasource that is already configured in the Compose stack.

Stop (keeps Prometheus data volume, and any Grafana state you mapped to a volume):

```bash
docker compose -f compose.int.yml down
```

---

### Prod-like environment (pre-built images + Prometheus + Grafana)

Assumes images are already built and (in a real setup) pushed to a registry:

```bash
cd docker
docker compose -f compose.prod.yml up -d
```

Update `image:` tags in `compose.prod.yml` to point to your registry if needed, e.g.:

```yaml
image: ghcr.io/your-username/golang-gin-app:1.0.0
```

Endpoints (example):

- Gin: `http://localhost:8081`
- Spring Boot: `http://localhost:8082`
- Django: `http://localhost:8083`
- Prometheus: `http://localhost:9090`
- Grafana: `http://localhost:3000`

Stop:

```bash
docker compose -f compose.prod.yml down
```

---

## Running stacks individually (without Docker)

Each service can also be run standalone.

- Go: see [`golang-gin/README.md`](./golang-gin/README.md)
- Java: see [`java-springboot/README.md`](./java-springboot/README.md)
- Python: see [`python-django/README.md`](./python-django/README.md)

---

## Observability

All three stacks share the same operational story:

- Endpoints
  - `/health` – liveness
  - `/ready` – readiness
  - `/metrics` – Prometheus metrics

- Metrics
  - `http_requests_total{service,method,path,status,...}`
  - `http_request_duration_seconds{service,method,path,status,...}`
  - `build_info{service,version,build_time}`

- Logging
  - JSON logs to stdout
  - Dedicated HTTP access logs:
    - `"http_request"` message (or equivalent)
    - includes `service`, `version`, `method`, `path`, `status`, `latency`, `userAgent`
  - Infra endpoints (`/health`, `/ready`, `/metrics`) are not logged to reduce noise, but are still counted in metrics.

- Visualization
  - Prometheus UI at `:9090` for raw queries
  - Grafana at `:3000` (integration / prod Compose) for dashboards built on top of the same Prometheus metrics

---

## Project Structure

```text
docker-polyglot-lab/
├── docker/              # Docker Compose, Prometheus, Grafana setup
├── golang-gin/          # Go 1.25.4 + Gin service (infra-focused microservice)
├── java-springboot/     # Java 21 + Spring Boot 3.5 service
├── python-django/       # Python 3.12 + Django + Gunicorn service
└── README.md            # This file
```

---

## Why this repo exists

This lab is intentionally **light on business logic** and **heavy on operational patterns**:
- health/readiness probes
- consistent logging and metrics across three stacks
- Docker images with build metadata and healthchecks
- Compose networks, volumes, and environment-specific configs
- Prometheus wired to every service
- Grafana wired to Prometheus for easy dashboards

It can be used as:
- A **portfolio** piece to demonstrate Docker + observability skills.
- A **template** for new microservices in Go, Java, or Python with production-style defaults.

---

## Status

This is a personal lab project. There are **no stability or backwards-compatibility guarantees**:
directory layout, APIs, and Docker tags may change at any time.

Use it as a reference or template at your own risk.

## Contributing

Issues and pull requests are welcome, but this repo is primarily a personal learning lab.

If you open a PR, try to:

- Keep the three stacks (Go, Java, Python) conceptually aligned:
  - same endpoints (`/`, `/info`, `/health`, `/ready`, `/metrics`)
  - similar logging and metrics story
- Add or update tests for any behavior you change
- Keep Dockerfiles and Compose files consistent across services
