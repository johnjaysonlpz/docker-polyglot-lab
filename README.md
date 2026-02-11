# Docker Polyglot Lab

[![CI](https://github.com/johnjaysonlpz/docker-polyglot-lab/actions/workflows/ci.yaml/badge.svg)](https://github.com/johnjaysonlpz/docker-polyglot-lab/actions/workflows/ci.yaml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Release](https://img.shields.io/github/v/release/johnjaysonlpz/docker-polyglot-lab)](https://github.com/johnjaysonlpz/docker-polyglot-lab/releases)
[![Last Commit](https://img.shields.io/github/last-commit/johnjaysonlpz/docker-polyglot-lab)](https://github.com/johnjaysonlpz/docker-polyglot-lab/commits)

![Go](https://img.shields.io/badge/Go-1.25.7-informational)
![Gin](https://img.shields.io/badge/Gin-v1.11.0-informational)
![Java](https://img.shields.io/badge/Java-25-informational)
![Spring%20Boot](https://img.shields.io/badge/Spring%20Boot-4.0.2-informational)
![Python](https://img.shields.io/badge/Python-3.12-informational)
![Django](https://img.shields.io/badge/Django-6.0.2-informational)
![Docker](https://img.shields.io/badge/Docker-Compose-informational)
![Observability](https://img.shields.io/badge/Observability-Alloy%20%7C%20Prometheus%20%7C%20Loki%20%7C%20Tempo%20%7C%20Grafana-informational)

[![Docker Pulls](https://img.shields.io/docker/pulls/johnjaysonlopez/golang-gin-app)](https://hub.docker.com/r/johnjaysonlopez/golang-gin-app)
[![Docker Pulls](https://img.shields.io/docker/pulls/johnjaysonlopez/java-springboot-app)](https://hub.docker.com/r/johnjaysonlopez/java-springboot-app)
[![Docker Pulls](https://img.shields.io/docker/pulls/johnjaysonlopez/python-django-app)](https://hub.docker.com/r/johnjaysonlopez/python-django-app)

A **polyglot microservices + observability** lab designed to showcase **modern container and operational best practices** and a complete **metrics / logs / traces** pipeline you can run locally — with a structure that maps cleanly to **cloud production patterns**.

### Services (polyglot)

- **Go / Gin** → [`golang-gin/`](golang-gin/)
- **Java / Spring Boot** → [`java-springboot/`](java-springboot/)
- **Python / Django** → [`python-django/`](python-django/)

### Observability stack

- **Alloy** (OpenTelemetry ingest + Docker log shipping)
- **Prometheus + Alertmanager** (metrics + alerting)
- **Loki** (logs)
- **Tempo** (traces + metrics-generator)
- **Grafana** (dashboards + Explore)

> [!TIP]
> The canonical “run everything” entrypoints are the **Docker Compose stacks** under [`docker/README.md#tldr—canonical-entrypoints`](docker/README.md#tldr--canonical-entrypoints).

> [!IMPORTANT]
> **Where to start (fast navigation)**
> - **Want to run the stack?** Start with [`docker/README.md`](docker/README.md) → **TL;DR — canonical entrypoints**.
> - **Want to understand the services?** Read the module docs: [`golang-gin/README.md`](golang-gin/README.md), [`java-springboot/README.md`](java-springboot/README.md), [`python-django/README.md`](python-django/README.md).
> - **Want pinned tooling + local checks?** Jump to [`#ci`](#ci) — `.ci-tool-versions.sh` is the **single source of truth** for tool pins.

---

## Contents

- [What this repo demonstrates](#what-this-repo-demonstrates)
- [Why a hybrid observability approach](#why-a-hybrid-observability-approach)
- [Production template for cloud](#production-template-for-cloud)
- [Quick start](#quick-start)
- [Endpoints](#endpoints)
- [Traffic generator](#traffic-generator)
- [Common operations](#common-operations)
- [System architecture](#system-architecture)
- [Service contract](#service-contract)
- [Environments](#environments)
- [Secrets bootstrap](#secrets-bootstrap)
- [CI](#ci)
- [Repository structure](#repository-structure)
- [Versions](#versions)
- [Troubleshooting](#troubleshooting)
- [Status](#status)
- [License](#license)

---

## What this repo demonstrates

This project is intentionally built as a **production-minded reference lab** (not just a demo wiring exercise). It demonstrates:

- **Production containerization patterns**: multi-stage Dockerfiles, slim runtime stages, **non-root execution**, and build metadata injection (e.g., `SERVICE_NAME`, `VERSION`, `BUILD_TIME`).
- **Operator-facing hardening behaviors**: explicit timeouts + graceful shutdown semantics (`SIGTERM`), request/payload limits (**clear `413` behavior**), trusted proxy controls for correct `X-Forwarded-*` handling, and health/readiness checks enforced via **Compose**.
- **Composable environment “shapes” with strong local parity**: **development** (apps only), **integration** (apps + observability), and **staging** (registry pulls / “build once, deploy many”) using modular Compose building blocks.
- **Observability you can rely on**: **Prometheus scrape** for metrics, **JSON logs** shipped via **Alloy → Loki**, and **OTLP traces** via **Alloy → Tempo** — correlated in **Grafana**.
- **High-signal CI with pinned tooling and local confidence checks**: `.ci-tool-versions.sh` is the **single source of truth** for pinned toolchains; it’s consumed by both `.ci-local.sh` and `.github/workflows/ci.yaml` to reduce drift. `.ci-local.sh` runs a fixed set of gates locally (not an event simulator).
> [!NOTE]
> For the implementation details of each item (Dockerfiles, Compose hardening, CI gates, and service-specific behaviors), see the per-module README.md docs.

---

## Why a hybrid observability approach

This repo intentionally uses the **best model per signal** and correlates everything in **Grafana**:

- **Metrics (Prometheus scrape)**: `/metrics` scraping is simple, reliable, and well-suited for RED/USE-style service signals without forcing a push pipeline.
- **Traces (OTLP push)**: traces are naturally push-based; services export OTLP to **Alloy**, which batches/routes and forwards to **Tempo**.
- **Logs (agent-based tailing)**: services emit structured JSON logs to stdout/stderr; **Alloy** tails container logs and ships them to **Loki** without per-app log agents.

**Goal:** *simple where possible, centralized where it pays off*, with clean correlation (request/trace/span IDs).

---

## Production template for cloud

The repo is shaped so it can serve as a **production reference template**:

- **Contracts map cleanly to orchestrators**: `/health` and `/ready` become liveness/readiness probes; `/metrics` remains the Prometheus boundary.
- **Signal boundaries remain stable**: **OTLP** for traces, **JSON logs** for platform logging collection, and **Prometheus scraping** (or managed Prometheus equivalents).
- **Staging mirrors real deployments**: **build once, deploy many** via **registry pulls**.

It also bakes in reusable “production-shaped” service and runtime behaviors:

- **Graceful shutdown + timeouts**: clean `SIGTERM` handling and explicit shutdown budgets.
- **Safety limits by default**: request/payload limits with clear `413` behavior.
- **Trusted proxy controls**: correct `X-Forwarded-*` handling without spoofable client IP headers.
- **Hardened containers**: non-root runtime plus Compose-level hardening (healthchecks, cap drops, read-only/tmpfs patterns where applicable).
- **Build provenance**: `/info` + OCI labels to expose version/build metadata, and repeatable dev → staging workflows.

You can swap Docker Compose for **Kubernetes / ECS / Nomad** while keeping the same operational shape and observability boundaries.

---

## Quick start

> [!IMPORTANT]
> This repo contains multiple Compose entrypoints. For the full matrix (dev vs. integration vs. staging, secrets vs. no-secrets), read [`docker/README.md#tldr—canonical-entrypoints`](docker/README.md#tldr--canonical-entrypoints).

### Prerequisites

| Type | Requirements |
|---|---|
| **Runtime** | Docker Engine / Docker Desktop (recent) + **Docker Compose v2** |
| **Host ports** | **8081–8083**, **3000**, **9090**, **9093**, **3100**, **3200**, **4317**, **4318**, **12345** |
| **Recommended** | **4+ CPU**, **6–8GB RAM**, **10GB+ disk** (volumes + images) |

> [!NOTE]
> All Compose stacks are designed to bind host ports to **`127.0.0.1`** (localhost-only) by default.

### Run the full stack (**pull from Docker Hub Registry**, no secrets)

This is the fastest on-ramp and mirrors **“build once, deploy many.”**

#### Step 1 — create `docker/.env` (optional but recommended)

Because Compose is invoked with `--project-directory docker`, the natural place to keep env vars is `docker/.env` (**do not commit**).

```bash
cat > docker/.env <<'EOF'
APP_ENV=staging
APP_VERSION=2.0.2
REGISTRY=docker.io/johnjaysonlopez
EOF
```

#### Step 2 — start the stack

```bash
docker compose --project-directory docker \
  -p polyglot-lab-staging \
  -f docker/compose.staging.nosecrets.yaml \
  up --pull always --remove-orphans
```

> [!TIP]
> Run detached: add `-d` to the `up` command.

#### Step 3 — verify the services are alive

```bash
curl -fsS http://127.0.0.1:8081/health >/dev/null && echo "svc-a OK"
curl -fsS http://127.0.0.1:8082/health >/dev/null && echo "svc-b OK"
curl -fsS http://127.0.0.1:8083/health >/dev/null && echo "svc-c OK"
```

#### Step 4 — open Grafana (and check scraping)

- **Grafana**: `http://127.0.0.1:3000`
- **Prometheus targets**: `http://127.0.0.1:9090/targets`

**Grafana credentials** depend on which Compose entrypoint you use:

- **No-secrets stacks**: Grafana typically uses upstream defaults **`admin` / `admin`** *unless* overridden in Compose.
- **Secrets-enabled stacks**: credentials come from Docker secrets created by [`.bootstrap-local.sh`](#secrets-bootstrap).

> [!TIP]
> If you can’t sign in, check [`docker/README.md#operational-knobs`](docker/README.md#operational-knobs) and/or inspect the running Grafana container configuration.

---

## Endpoints

### Services

| Component | URL |
|---|---|
| **Go (Gin)** | `http://127.0.0.1:8081` |
| **Java (Spring Boot)** | `http://127.0.0.1:8082` |
| **Python (Django)** | `http://127.0.0.1:8083` |

### Observability

| Component | URL |
|---|---|
| **Grafana** | `http://127.0.0.1:3000` |
| **Prometheus** | `http://127.0.0.1:9090` |
| **Alertmanager** | `http://127.0.0.1:9093` |
| **Tempo (query/frontend)** | `http://127.0.0.1:3200` |
| **Loki** | `http://127.0.0.1:3100` |
| **Alloy (OTLP/gRPC ingest)** | `http://127.0.0.1:4317` |
| **Alloy (OTLP/HTTP ingest)** | `http://127.0.0.1:4318` |
| **Alloy UI/status** | `http://127.0.0.1:12345` |

---

## Traffic generator

If the system is idle, generate traffic so **metrics / logs / traces** populate quickly.

> [!TIP]
> After running this, check **Prometheus targets** and **Grafana Explore** (Prometheus / Loki / Tempo).

<details>
<summary><strong>Click to expand traffic generator script</strong></summary>

```bash
TARGETS=(
  "svc-a root (200)|http://127.0.0.1:8081/"
  "svc-b root (200)|http://127.0.0.1:8082/"
  "svc-c root (200)|http://127.0.0.1:8083/"

  "svc-a WRONG /nope (404)|http://127.0.0.1:8081/nope"
  "svc-b WRONG /nope (404)|http://127.0.0.1:8082/nope"
  "svc-c WRONG /nope (404)|http://127.0.0.1:8083/nope"

  "svc-a info (200)|http://127.0.0.1:8081/info"
  "svc-b info (200)|http://127.0.0.1:8082/info"
  "svc-c info (200)|http://127.0.0.1:8083/info"

  "svc-a WRONG /infoo (404)|http://127.0.0.1:8081/asd"
  "svc-b WRONG /infoo (404)|http://127.0.0.1:8082/asd"
  "svc-c WRONG /infoo (404)|http://127.0.0.1:8083/asd"
)

REQUESTS_PER_TARGET=25

for entry in "${TARGETS[@]}"; do
  IFS='|' read -r label url <<< "$entry"
  echo "Hitting: $label -> $url x$REQUESTS_PER_TARGET"
  for ((i=1; i<=REQUESTS_PER_TARGET; i++)); do
    code="$(curl -sS -o /dev/null -w '%{http_code}' "$url" || echo '000')"
    echo "  [$i/$REQUESTS_PER_TARGET] HTTP $code"
  done
done
```

</details>

Then validate:

- **Prometheus targets:** `http://127.0.0.1:9090/targets`
- **Grafana:** `http://127.0.0.1:3000` → **Dashboards** / **Explore** (Prometheus / Loki / Tempo)

---

## Common operations

> [!TIP]
> For the full operator/runbook commands (including `ps`, `logs`, `restart`, and `config` rendering), see [`docker/README.md#common-operator-commands`](docker/README.md#common-operator-commands).

<details>
<summary><strong>Click to expand quick common operations (root convenience)</strong></summary>

> [!TIP]
> The examples below use the **staging / no-secrets** entrypoint. Keep `-p` and `-f` consistent per run, so you’re operating on the same project.

### Stop the stack

```bash
docker compose --project-directory docker \
  -p polyglot-lab-staging \
  -f docker/compose.staging.nosecrets.yaml \
  down --remove-orphans
```

### Full reset (removes volumes; destructive)

Use this if you want a clean slate for Prometheus/Loki/Tempo/Grafana state.

```bash
docker compose --project-directory docker \
  -p polyglot-lab-staging \
  -f docker/compose.staging.nosecrets.yaml \
  down -v --remove-orphans
```

### See what’s running

```bash
docker compose --project-directory docker \
  -p polyglot-lab-staging \
  -f docker/compose.staging.nosecrets.yaml \
  ps
```

### Tail logs

```bash
docker compose --project-directory docker \
  -p polyglot-lab-staging \
  -f docker/compose.staging.nosecrets.yaml \
  logs -f --tail=200
```

> [!NOTE]
> If you forget a service name, `docker compose ... ps` is the fastest way to discover it.

---

</details>

## System architecture

> [!NOTE]
> Scope: this diagram represents **integration/staging** runs where observability is enabled. The **apps-only** development stack does not start Prometheus/Grafana/Loki/Tempo/Alertmanager/Alloy.

<details>
<summary><strong>Click to expand architecture diagram</strong></summary>

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

</details>

### Init helpers (run inside the stack)

- `toolbox-init` — stages BusyBox into the shared `toolbox` volume for healthchecks
- `loki-init` — prepares Loki runtime directories/permissions for the volume-backed store
- `tempo-init` — prepares Tempo runtime directories/permissions for the volume-backed store

For deeper breakdowns (compose entrypoints, overlays, healthchecks, and secrets workflow), see [`docker/README.md#compose-layout`](docker/README.md#compose-layout) and [`docker/README.md#implementation-map-where-to-look`](docker/README.md#implementation-map-where-to-look).

---

## Service contract

All three services expose the same HTTP surface area:

| Endpoint | Meaning | Used by |
|---|---|---|
| `GET /` | Banner (“service is running”) | Humans / smoke checks |
| `GET /info` | Build/service metadata | Humans / dashboards |
| `GET /health` | **Liveness** probe | Orchestrator liveness checks |
| `GET /ready` | **Readiness** probe | Compose healthchecks / orchestrator readiness |
| `GET /metrics` | Prometheus metrics (text format) | Prometheus scraping / alerting |

> [!NOTE]
> This consistency is intentional: it keeps health checks, scraping, dashboards, and alerts predictable in a polyglot stack.

---

## Environments

This repo is deliberately structured around environment “shapes” that mirror real deployments:

- **development** — apps only; fastest inner-loop
- **integration** — apps + full observability (builds locally; typically no registry pulls)
- **staging (prod-like)** — pulls prebuilt images from a registry; mirrors production **“build once, deploy many”**

All authoritative run commands (including secrets-enabled stacks and overlays) live in:
- [`docker/README.md#tldr—canonical-entrypoints`](docker/README.md#tldr--canonical-entrypoints)
- [`docker/README.md#common-operator-commands`](docker/README.md#common-operator-commands)

Common entrypoints:

- **Apps-only (local builds):** `docker/compose.development.yaml`
- **Apps + observability (no secrets overlays):** `docker/compose.integration.nosecrets.yaml`
- **Apps + observability (with secrets overlays):** `docker/compose.integration.yaml`
- **Staging pulls (no secrets / with secrets):** `docker/compose.staging*.yaml`

---

## Secrets bootstrap

### `.bootstrap-local.sh` (secrets + permissions)

If you use the **secrets-enabled** Compose entrypoints:

- `docker/compose.integration.yaml`
- `docker/compose.staging.yaml`

…run the bootstrap script once from the repo root. It creates the expected Docker secrets files in `docker/secrets/` and sets ownership/permissions so **Grafana/Alertmanager** can read them safely.

#### Required env vars

- **`GRAFANA_ADMIN_USER`**
- **`GRAFANA_ADMIN_PASSWORD`**
- **`TELEGRAM_BOT_TOKEN`**
- **`TELEGRAM_CHAT_ID`**

#### Usage (from repo root)

```bash
./.bootstrap-local.sh --help  # or: -h

export GRAFANA_ADMIN_USER=admin
export GRAFANA_ADMIN_PASSWORD='supersecret'
export TELEGRAM_BOT_TOKEN='...'
export TELEGRAM_CHAT_ID='...'

./.bootstrap-local.sh
```

> [!TIP]
> For the overlay mechanics and the exact secret file wiring, see [`docker/README.md#compose-layout`](docker/README.md#compose-layout)

---

## CI

### GitHub Actions workflow

The workflow runs on pushes, PRs, and tags; it enforces formatting, linting, tests, coverage expectations, and security scanning. It also builds app images and pushes them to a registry on release tags.

See: [`.github/workflows/ci.yaml`](.github/workflows/ci.yaml) (workflow) and [`.github/actions/`](.github/actions/) (shared composite steps).

> [!NOTE]
> GitHub Actions only discovers workflows from **`.github/workflows/`**. If you’re viewing this project from an archive that strips dot-directories, double-check the repo still contains `.github/workflows` when pushed to GitHub.

### Local checks: `.ci-local.sh`

> [!NOTE]
> `.ci-local.sh` is a **local checker** that runs a **fixed set of high-signal gates** (format/lint/test/coverage/security).
> It **does not simulate CI events** (push/PR/schedule) and does not attempt to match GitHub Actions runtime details
> (caching, SARIF upload, dependency review, release/tag behavior, etc.).  
> The GitHub workflow (`.github/workflows/ci.yaml`) remains the source of truth for CI-only behavior.

```bash
./.ci-local.sh --help                       # or: -h

./.ci-local.sh                              # default: trivy_repo + go + java + python
./.ci-local.sh trivy_repo                   # repo scan only
./.ci-local.sh docker                       # docker builds + image scans only
./.ci-local.sh doctor all --summary         # preflight checks (recommended)
```

Tool pins live in: [`.ci-tool-versions.sh`](.ci-tool-versions.sh) — the **single source of truth** consumed by **`.ci-local.sh`** and the **GitHub Actions workflow** (via **`.github/actions/load-tool-versions`**).

---

## Repository structure

| Path | Purpose |
|---|---|
| [`docker/`](docker/README.md) | Compose stacks (apps-only + full observability), configs for **Alloy/Prometheus/Loki/Tempo/Grafana/Alertmanager**, secret overlays, staging pulls |
| [`golang-gin/`](golang-gin/README.md) | Go + Gin service |
| [`java-springboot/`](java-springboot/README.md) | Spring Boot service |
| [`python-django/`](python-django/README.md) | Django service |
| [`.github/workflows/ci.yaml`](.github/workflows/ci.yaml) | CI workflow |
| [`.github/actions/load-tool-versions/`](.github/actions/load-tool-versions/) | Composite action: loads `.ci-tool-versions.sh` into `GITHUB_ENV` |
| [`.bootstrap-local.sh`](.bootstrap-local.sh) | Local bootstrap for secrets + permissions |
| [`.ci-local.sh`](.ci-local.sh) | Local CI runner |
| [`.ci-tool-versions.sh`](.ci-tool-versions.sh) | Tool/version pins (**single source of truth**) consumed by **`.ci-local.sh`** and **`.github/workflows/ci.yaml`** |
| [`.gitignore`](.gitignore) | Secret hygiene + build artifact ignores |

---

## Versions

### App runtimes/frameworks

| Runtime / framework | Version |
|---|---|
| **Go** | `1.25.7` |
| **Gin** | `v1.11.0` |
| **Java** | `25` |
| **Spring Boot** | `4.0.2` |
| **Python** | `3.12` |
| **Django** | `6.0.2` |

### Observability images (from `docker/compose._observability.yaml`)

| Component | Image |
|---|---|
| **Alloy** | `grafana/alloy:v1.12.2` |
| **Prometheus** | `prom/prometheus:v3.9.1` |
| **Alertmanager** | `prom/alertmanager:v0.31.0` |
| **Loki** | `grafana/loki:3.6.4` |
| **Tempo** | `grafana/tempo:2.10.0` |
| **Grafana** | `grafana/grafana:12.3.2` |

---

## Troubleshooting

### Ports are already allocated

**Symptom:** Compose fails with `ports are already allocated`.

**Fix:** stop the conflicting process, or change the host port mapping(s) in the relevant Compose file.

### Grafana is up but I don’t see data

1. Generate traffic (see [`#traffic-generator`](#traffic-generator)).
2. Confirm Prometheus is scraping: `http://127.0.0.1:9090/targets`
3. In Grafana, verify datasources are reachable (you can also inspect container logs).

### Healthchecks keep failing / services restart

Common causes:

- insufficient CPU/RAM assigned to Docker Desktop
- old Docker / Compose versions
- stale volumes from a previous run (try a **full reset** via `down -v`)

### Secrets-enabled stack won’t start

Run the bootstrap step and ensure the required env vars are set. 

See: [`#secrets-bootstrap`](#secrets-bootstrap)

---

## Status

This is a personal lab project. There are **no stability or backwards-compatibility guarantees**:
directory layout, APIs, and Docker tags may change at any time.

Use it as a reference or template at your own risk.

---

## License

MIT — see [`LICENSE`](LICENSE).
