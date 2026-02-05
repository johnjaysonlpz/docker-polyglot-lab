# Docker Polyglot Lab

[![CI](https://github.com/johnjaysonlpz/docker-polyglot-lab/actions/workflows/cicd.yaml/badge.svg)](https://github.com/johnjaysonlpz/docker-polyglot-lab/actions/workflows/cicd.yaml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Release](https://img.shields.io/github/v/release/johnjaysonlpz/docker-polyglot-lab)](https://github.com/johnjaysonlpz/docker-polyglot-lab/releases)
[![Last Commit](https://img.shields.io/github/last-commit/johnjaysonlpz/docker-polyglot-lab)](https://github.com/johnjaysonlpz/docker-polyglot-lab/commits)

![Go](https://img.shields.io/badge/Go-1.25.6-informational)
![Gin](https://img.shields.io/badge/Gin-v1.11.0-informational)
![Java](https://img.shields.io/badge/Java-25.0.2-informational)
![Spring%20Boot](https://img.shields.io/badge/Spring%20Boot-4.0.2-informational)
![Python](https://img.shields.io/badge/Python-3.12-informational)
![Django](https://img.shields.io/badge/Django-6.0.2-informational)
![Docker](https://img.shields.io/badge/Docker-Compose-informational)
![Observability](https://img.shields.io/badge/Observability-Alloy%20%7C%20Prometheus%20%7C%20Loki%20%7C%20Tempo%20%7C%20Grafana-informational)

[![Docker Pulls](https://img.shields.io/docker/pulls/johnjaysonlopez/golang-gin-app)](https://hub.docker.com/r/johnjaysonlopez/golang-gin-app)
[![Docker Pulls](https://img.shields.io/docker/pulls/johnjaysonlopez/java-springboot-app)](https://hub.docker.com/r/johnjaysonlopez/java-springboot-app)
[![Docker Pulls](https://img.shields.io/docker/pulls/johnjaysonlopez/python-django-app)](https://hub.docker.com/r/johnjaysonlopez/python-django-app)


A **polyglot microservices + observability** lab built to showcase **modern microservice best practices** and a complete **metrics / logs / traces** pipeline you can run locally — with a structure that also maps cleanly to **cloud production patterns**.

It includes three production-minded HTTP services:
- **Go / Gin** (`golang-gin/`)
- **Java / Spring Boot** (`java-springboot/`)
- **Python / Django** (`python-django/`)

…and a full observability stack:
- **Alloy** (OpenTelemetry ingest + Docker log shipping)
- **Prometheus + Alertmanager** (metrics + alerting)
- **Loki** (logs)
- **Tempo** (traces + metrics-generator)
- **Grafana** (dashboards + Explore)

The canonical “run everything” entrypoint is the **Docker Compose stacks** in [`docker/`](docker/README.md).

---

## TL;DR

### Run the full stack (apps + observability, no secrets overlays)

```bash
BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
APP_ENV=integration APP_VERSION=1.0.0 \
docker compose --project-directory docker \
  -p polyglot-lab-integration -f docker/compose.integration.nosecrets.yaml \
  up --build --remove-orphans
```

Open:
- Go (Gin): `http://127.0.0.1:8081`
- Java (Spring Boot): `http://127.0.0.1:8082`
- Python (Django): `http://127.0.0.1:8083`
- Grafana: `http://127.0.0.1:3000`

### Generate traffic (to light up dashboards)

If the system is idle, generate a little traffic so **metrics/logs/traces** populate quickly:

```bash
URLS=(
  "http://127.0.0.1:8081/"
  "http://127.0.0.1:8082/"
  "http://127.0.0.1:8083/"
  "http://127.0.0.1:8081/info"
  "http://127.0.0.1:8082/info"
  "http://127.0.0.1:8083/info"
)

for url in "${URLS[@]}"; do
  for i in {1..25}; do
    curl -fsS "$url" >/dev/null || true
  done
done
```

Then check:
- **Prometheus** targets: `http://127.0.0.1:9090/targets`
- **Grafana**: `http://127.0.0.1:3000` → Dashboards/Explore (Prometheus/Loki/Tempo)

> For **staging pulls**, **secrets-enabled** stacks, and the full operational guide, see [`docker/README.md`](docker/README.md).

---

## Highlights

### Consistent service contract across Go/Gin, Java/Spring Boot, Python/Django

All three services expose the same HTTP surface area:
- `GET /`        — banner (“service is running”)
- `GET /info`    — build/service metadata
- `GET /health`  — liveness probe
- `GET /ready`   — readiness probe (used by **Docker Compose** healthchecks)
- `GET /metrics` — Prometheus metrics (text format)

This consistency is intentional: it keeps health checks, scraping, dashboards, and alerts predictable in a polyglot stack.

### What this repo is designed to demonstrate

- **Production-minded containerization (real-world patterns)**
  - multi-stage Dockerfiles
  - build metadata injection (`SERVICE_NAME`, `VERSION`, `BUILD_TIME`)
  - apps run as **non-root** users (unprivileged UID/GID)
  - slim runtime stages to reduce attack surface

- **Concrete production-hardening behaviors (operator-facing)**
  - **Timeouts + graceful shutdown semantics** are explicitly configured and validated per service (server timeouts, shutdown timeouts, clean termination on `SIGTERM`).
  - **Payload limits** are enforced at the edge of each service (request body sizing / upload limits) with clear 413 behavior.
  - **Trusted proxy controls** (`TRUSTED_PROXIES` / Tomcat RemoteIpValve settings) prevent spoofed client IPs and ensure correct `X-Forwarded-*` handling.
  - **Non-root runtime**: app containers run as unprivileged UID/GID; hardening is reinforced via Compose (cap drops, tmpfs/read-only patterns where applicable).
  - **Build provenance**: images embed build metadata (`SERVICE_NAME`, `VERSION`, `BUILD_TIME`) surfaced via `GET /info` and exported via metrics labels where applicable.
  - **CI gates + reproducibility**: local CI parity runner (`.ci-local.sh`) + pinned toolchain versions (`.ci-tool-versions.sh`) to minimize “works on my machine” drift.
  - **Security scanning** is part of the normal workflow (language-specific vuln/dependency scanning + CI enforcement).

- **Composable environments with strong local parity**
  - **development** (apps only; fastest loop)
  - **integration** (apps + full observability)
  - **staging (prod-like)** stacks that support **registry pulls** (build once, deploy many)
  - modular Compose using `include:` building blocks

- **Operational correctness and hardening where it belongs**
  - Runtime hardening and healthchecks are enforced via **Compose** (cap drops, tmpfs/read-only patterns where applicable; /ready healthcheck), not embedded in the Dockerfile.
  - a shared `toolbox-init` pattern stages a tiny BusyBox helper for healthchecks without bloating app images
  - secrets-enabled stacks use overlays + a safe bootstrap workflow

- **Observability you can trust**
  - **metrics**: Prometheus scraping across all services
  - **logs**: JSON logs to stdout/stderr, shipped via Alloy → Loki
  - **traces**: OTLP → Alloy → Tempo, queryable in Grafana
  - stable metrics labels to avoid cardinality blowups (e.g., `**path="__unmatched__"**` for 404s; template paths for labels)

- **High-signal CI/CD with local parity**
  - `.ci-local.sh` mirrors the CI intent locally
  - `.ci-tool-versions.sh` pins tool versions for reproducibility
  - security scanning + dependency checks are integrated
  - strong test + coverage expectations (designed to detect drift aggressively)

---

## System architecture (full stack)

> Scope: this diagram represents the **full stack** (integration/staging) runs where observability is enabled. The **apps-only** development stack does not start Prometheus/Grafana/Loki/Tempo/Alertmanager/Alloy.

```text
User/Operator
   |
   |  HTTP (host ports; bound to 127.0.0.1)
   v
Host Ports
   |---- 127.0.0.1:8081 --------> golang-gin-app (container :8080)
   |---- 127.0.0.1:8082 --------> java-springboot-app (container :8080)
   |---- 127.0.0.1:8083 --------> python-django-app (container :8080)
   |
   |---- 127.0.0.1:3000 --------> Grafana
   |---- 127.0.0.1:9090 --------> Prometheus
   |---- 127.0.0.1:9093 --------> Alertmanager
   |---- 127.0.0.1:3200 --------> Tempo query/frontend
   |---- 127.0.0.1:3100 --------> Loki
   |---- 127.0.0.1:4317 --------> Alloy (OTLP/gRPC ingest)
   |---- 127.0.0.1:4318 --------> Alloy (OTLP/HTTP ingest; POST /v1/traces)
   |---- 127.0.0.1:12345 -------> Alloy UI/status

METRICS:
Prometheus ---scrape /metrics---> Go/Gin app
Prometheus ---scrape /metrics---> Java/Spring Boot app
Prometheus ---scrape /metrics---> Python/Django app
Tempo --metrics-generator remote_write--> Prometheus

TRACES:
Go/Gin app ------------ OTLP ----\
Java/Spring Boot app -- OTLP -----+--> Alloy --> Tempo (trace store) <-- Tempo query/frontend
Python/Django app------ OTLP ----/

LOGS:
Docker daemon (container logs) --> Alloy --> Loki

DASHBOARDS:
Grafana --> Prometheus (metrics)
Grafana --> Loki (logs)
Grafana --> Tempo query/frontend (traces)

ALERTING:
Prometheus --> Alertmanager --> (optional) Telegram / other receivers

HEALTHCHECKS:
toolbox-init (busybox to shared volume) --> healthchecks --> services (apps, Prom, AM, TempoQ, Grafana)
```

Init helpers (run inside the stack):
- `toolbox-init` — stages BusyBox into the shared `toolbox` volume for healthchecks
- `loki-init` — prepares Loki runtime directories/permissions for the volume-backed store
- `tempo-init` — prepares Tempo runtime directories/permissions for the volume-backed store

For a deeper breakdown (compose entrypoints, overlays, healthchecks, and secrets workflow), see [`docker/README.md`](docker/README.md).

---

## Module documentation

- [`docker/README.md`](docker/README.md) — Compose stacks, overlays, staging pulls, and observability wiring
- [`golang-gin/README.md`](golang-gin/README.md) — Go service API + observability contract + container image details
- [`java-springboot/README.md`](java-springboot/README.md) — Java service API + observability contract + operational knobs + container image details
- [`python-django/README.md`](python-django/README.md) — Django service API + observability contract + operational knobs + container image details

---

## Why a hybrid observability approach

This repo intentionally uses the **best model per signal**, then correlates everything in Grafana:

- **Metrics: Prometheus scrape model**
  - `/metrics` scraping is simple, reliable, and scales well for RED/USE signals.
  - it avoids pushing metrics through a collector unless you actually need that architecture.

- **Traces: OTLP push model**
  - traces are naturally push-based.
  - services export OTLP to **Alloy**, which batches/routes, then forwards to **Tempo**.

- **Logs: agent-based tailing**
  - logs are emitted as JSON to stdout/stderr.
  - **Alloy** tails container logs (via Docker) and ships them to **Loki** without per-app log agents.

The goal is pragmatic: **simple where possible, centralized where it pays off**, with a clean correlation story (request ID + trace/span IDs).

---

## Production template for cloud

This repo is structured so it can serve as a **production reference template**, not just a demo:

- Replace Compose with **Kubernetes** (or ECS/Nomad) while keeping the same contracts:
  - readiness/liveness probes map directly to `/ready` and `/health`
  - Prometheus scrapes services (or use managed Prometheus + scrape configs)
  - Alloy becomes a DaemonSet/sidecar/agent (or a managed collector)
  - Tempo/Loki/Grafana can be self-hosted or swapped for managed equivalents

- Keep the same “shape”:
  - `/metrics` remains the metrics boundary
  - OTLP remains the trace boundary
  - JSON logs remain the log boundary (collected by platform logging)

- **Staging “pull images”** already exists here (registry images + prod-like stack), mirroring how production deployments operate: **build once, deploy many**.

---

## Running the stack

All authoritative run commands (including repo-root staging pulls and secrets-enabled stacks) are documented in:
- [`docker/README.md`](docker/README.md)

Common entrypoints:
- **apps-only (local builds):** `docker/compose.development.yaml`
- **apps + observability (no secrets overlays):** `docker/compose.integration.nosecrets.yaml`
- **apps + observability (with secrets overlays):** `docker/compose.integration.yaml`
- **staging pulls (no secrets / with secrets):** `docker/compose.staging*.yaml`

---

## Secrets and local bootstrap

### `.bootstrap-local.sh` (secrets + permissions)

If you use the **secrets-enabled** Compose entrypoints:
- `docker/compose.integration.yaml`
- `docker/compose.staging.yaml`

…run the bootstrap script once from the repo root. It creates the expected Docker secrets files in `docker/secrets/` and sets ownership/permissions so Grafana/Alertmanager can read them safely.

Required env vars:
- `GRAFANA_ADMIN_USER`
- `GRAFANA_ADMIN_PASSWORD`
- `TELEGRAM_BOT_TOKEN`
- `TELEGRAM_CHAT_ID`

Usage (from repo root):
```bash
export GRAFANA_ADMIN_USER=admin
export GRAFANA_ADMIN_PASSWORD='supersecret'
export TELEGRAM_BOT_TOKEN='...'
export TELEGRAM_CHAT_ID='...'

./.bootstrap-local.sh
```

> For the overlay mechanics and the exact secret file wiring, see [`docker/README.md`](docker/README.md).

---

## CI/CD

### GitHub Actions workflow

The workflow runs on pushes, PRs, and tags; it enforces formatting, linting, tests, coverage expectations, and security scanning. It also builds app images and pushes them to a registry on release tags.

See: [`.github/workflows/cicd.yaml`](.github/workflows/cicd.yaml)

### Local CI parity: `.ci-local.sh`

Usage (from repo root):

```bash
./.ci-local.sh               # run all: go + java + python
./.ci-local.sh go            # Go only
./.ci-local.sh java          # Java only
./.ci-local.sh python        # Python only
./.ci-local.sh doctor all    # preflight checks (recommended)
```

Tool pins live in: `.ci-tool-versions.sh`

---

## Repository structure

| Path | Purpose |
|---|---|
| [`docker/`](docker/README.md) | Compose stacks (apps-only + full observability), configs for Alloy/Prometheus/Loki/Tempo/Grafana/Alertmanager, secret overlays, staging pulls |
| [`golang-gin/`](golang-gin/README.md) | Go + Gin service |
| [`java-springboot/`](java-springboot/README.md) | Spring Boot service |
| [`python-django/`](python-django/README.md) | Django service |
| [`.github/workflows/cicd.yaml`](.github/workflows/cicd.yaml) | CI/CD workflow |
| [`.bootstrap-local.sh`](.bootstrap-local.sh) | Local bootstrap for secrets + permissions |
| [`.ci-local.sh`](.ci-local.sh) | Local CI runner |
| [`.ci-tool-versions.sh`](.ci-tool-versions.sh) | Tool/version pins used by local CI + CI |
| [`.gitignore`](.gitignore) | Secret hygiene + build artifact ignores |

---

## Languages, frameworks, and stack versions

### App runtimes/frameworks
- **Go:** `1.25.6`
- **Gin:** `v1.11.0`
- **Java:** `25.0.2`
- **Spring Boot:** `4.0.2`
- **Python:** `3.12`
- **Django:** `6.0.2`

### Observability images (from `docker/compose._observability.yaml`)
- **Alloy:** `grafana/alloy:v1.12.2`
- **Prometheus:** `prom/prometheus:v3.9.1`
- **Alertmanager:** `prom/alertmanager:v0.31.0`
- **Loki:** `grafana/loki:3.6.4`
- **Tempo:** `grafana/tempo:2.10.0`
- **Grafana:** `grafana/grafana:12.3.2`

---

## `.gitignore` policy

This repo is designed to be safe-by-default for local development:
- real `.env` files and secret outputs should **never** be committed
- build artifacts and generated security reports should remain out of git

See [`.gitignore`](.gitignore) `for the exact ignore/allowlist rules.`

---

## Status

This is a personal lab project. There are **no stability or backwards-compatibility guarantees**:
directory layout, APIs, and Docker tags may change at any time.

Use it as a reference or template at your own risk.

---

## License

MIT — see [`LICENSE`](LICENSE).
