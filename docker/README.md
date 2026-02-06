# `/docker` — Docker Compose stacks (**apps + observability**)

This module is the **runtime glue** for the whole repo. It provides Docker Compose stacks that run:

- the three app services (**Go/Gin**, **Java/Spring Boot**, **Python/Django**), and
- a full local observability stack: **Alloy, Prometheus, Loki, Tempo, Grafana**.

**Signals (repo stack):** apps export **traces → Alloy → Tempo**, **metrics → Prometheus**, **logs → Alloy → Loki**, with **Grafana pre-provisioned** (datasources + dashboards).

> [!TIP]
> For the repo overview, architecture, service contract, and a shorter onboarding path, see [`../README.md`](../README.md).

> [!IMPORTANT]
> These stacks use Docker Compose **`include:`** (**Compose v2** feature). Secrets-enabled stacks also use YAML tags like **`!override`** inside the secrets overlays.

---

## Contents

- [Quick start](#quick-start)
- [TL;DR — canonical entrypoints](#tldr--canonical-entrypoints)
- [Common operator commands](#common-operator-commands)
- [Operator verification checklist](#operator-verification-checklist)
- [Services and local ports](#services-and-local-ports-host--container)
- [Compose layout](#compose-layout)
- [Configuration](#configuration)
- [Request processing pipeline](#request-processing-pipeline)
- [Observability](#observability)
- [Operational knobs](#operational-knobs)
- [Testing](#testing)
- [Container image details](#container-image-details)
- [Implementation map](#implementation-map-where-to-look)
- [Interactions with other modules](#interactions-with-other-modules)

---

## Quick start

### Prerequisites

| Type | Requirement |
|---|---|
| **Docker** | Docker Engine / Docker Desktop (recent) |
| **Compose** | **Docker Compose v2** (`include:` is required) |
| **Host ports** | `8081–8083`, `3000`, `9090`, `9093`, `3100`, `3200`, `4317`, `4318`, `12345` |
| **Recommended** | **4+ CPU**, **6–8GB RAM**, **10GB+ disk** (volumes + images) |

Quick version check:

```bash
docker compose version
```

> [!NOTE]
> The commands in this document are written to be **runnable from the repository root** and use `--project-directory docker` so Compose `include:` paths resolve correctly.

### Suggested local env file (`docker/.env`)

Most runs are easier (and less error-prone) if you keep common variables in **`docker/.env`** instead of repeating them inline.

Create/update `docker/.env` (do **not** commit it):

```bash
cat > docker/.env <<'EOF'
# Choose: development | integration | staging
APP_ENV=integration

# Used for staging pulls (and surfaced in /info where applicable)
APP_VERSION=${VERSION:-integration}

# Required only for staging pulls (ignored for local builds)
REGISTRY=docker.io/johnjaysonlopez

# Hardening defaults for app containers (override if needed)
APP_UID=10001
APP_GID=10001
EOF
```

At runtime you typically set `BUILD_TIME` at the shell (so it’s accurate per run):

```bash
export BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
```

---

## TL;DR — canonical entrypoints

The **Compose entrypoints** below are the supported “shapes”:

- **Development** → apps only (built locally)
- **Integration** → apps + observability (built locally), with an option to enable secrets overlays
- **Staging** → apps + observability (pulled from registry), with an option to enable secrets overlays

> [!TIP]
> Prefer **Integration (no secrets)** when you want the full stack locally with minimal setup.  
> Prefer **Staging (no secrets)** when you want prod-like “pull images” behavior (**build once, deploy many**).

### Entry points summary

| Environment | What runs | Compose file | Images |
|---|---|---|---|
| **Development** | apps only | `docker/compose.development.yaml` | **build locally** |
| **Integration (no secrets)** | apps + obs | `docker/compose.integration.nosecrets.yaml` | **build locally** |
| **Integration (with secrets)** | apps + obs | `docker/compose.integration.yaml` | **build locally** |
| **Staging (no secrets)** | apps + obs | `docker/compose.staging.nosecrets.yaml` | **pull from `${REGISTRY}`** |
| **Staging (with secrets)** | apps + obs | `docker/compose.staging.yaml` | **pull from `${REGISTRY}`** |

### Copy/paste commands (root-run)

> [!IMPORTANT]
> The examples below assume you’ve created `docker/.env` and exported `BUILD_TIME` (see [`#quick-start`](#quick-start)).  
> If you don’t use `docker/.env`, you can still inline `APP_ENV`, `APP_VERSION`, and `REGISTRY` the same way as the original doc.

#### Development (apps only; local builds)

<details>
<summary><strong>Commands: development up / down / reset</strong></summary>

**Up**

```bash
export BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

docker compose --project-directory docker \
  -p polyglot-lab-development \
  -f docker/compose.development.yaml \
  up --build --remove-orphans
```

**Down (non-destructive)**

```bash
docker compose --project-directory docker \
  -p polyglot-lab-development \
  -f docker/compose.development.yaml \
  down --remove-orphans
```

**Reset (destructive; removes volumes)**

```bash
docker compose --project-directory docker \
  -p polyglot-lab-development \
  -f docker/compose.development.yaml \
  down -v --remove-orphans
```

</details>

#### Integration (apps + observability; **no secrets overlays**)

<details>
<summary><strong>Commands: integration (no secrets) up / down / reset</strong></summary>

**Up**

```bash
export BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

docker compose --project-directory docker \
  -p polyglot-lab-integration \
  -f docker/compose.integration.nosecrets.yaml \
  up --build --remove-orphans
```

**Down (non-destructive)**

```bash
docker compose --project-directory docker \
  -p polyglot-lab-integration \
  -f docker/compose.integration.nosecrets.yaml \
  down --remove-orphans
```

**Reset (destructive; removes volumes)**

```bash
docker compose --project-directory docker \
  -p polyglot-lab-integration \
  -f docker/compose.integration.nosecrets.yaml \
  down -v --remove-orphans
```

</details>

#### Integration (apps + observability; **with secrets overlays**)

> [!IMPORTANT]
> Before running secrets-enabled stacks, run the repo bootstrap script once (from repo root):  
> see [`#local-bootstrap-for-secrets-and-permissions`](#local-bootstrap-for-secrets-and-permissions-bootstrap-localsh).

<details>
<summary><strong>Commands: integration (with secrets) up / down / reset</strong></summary>

**Up**

```bash
export BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

docker compose --project-directory docker \
  -p polyglot-lab-integration \
  -f docker/compose.integration.yaml \
  up --build --remove-orphans
```

**Down (non-destructive)**

```bash
docker compose --project-directory docker \
  -p polyglot-lab-integration \
  -f docker/compose.integration.yaml \
  down --remove-orphans
```

**Reset (destructive; removes volumes)**

```bash
docker compose --project-directory docker \
  -p polyglot-lab-integration \
  -f docker/compose.integration.yaml \
  down -v --remove-orphans
```

</details>

#### Staging (pull images; **no secrets overlays**)

<details>
<summary><strong>Commands: staging (no secrets) up / down / reset</strong></summary>

**Up**

```bash
docker compose --project-directory docker \
  -p polyglot-lab-staging \
  -f docker/compose.staging.nosecrets.yaml \
  up --pull always --remove-orphans
```

**Down (non-destructive)**

```bash
docker compose --project-directory docker \
  -p polyglot-lab-staging \
  -f docker/compose.staging.nosecrets.yaml \
  down --remove-orphans
```

**Reset (destructive; removes volumes)**

```bash
docker compose --project-directory docker \
  -p polyglot-lab-staging \
  -f docker/compose.staging.nosecrets.yaml \
  down -v --remove-orphans
```

</details>

#### Staging (pull images; **with secrets overlays**)

> [!IMPORTANT]
> Before running secrets-enabled stacks, run the repo bootstrap script once (from repo root):  
> see [`#local-bootstrap-for-secrets-and-permissions`](#local-bootstrap-for-secrets-and-permissions-bootstrap-localsh).

<details>
<summary><strong>Commands: staging (with secrets) up / down / reset</strong></summary>

**Up**

```bash
docker compose --project-directory docker \
  -p polyglot-lab-staging \
  -f docker/compose.staging.yaml \
  up --pull always --remove-orphans
```

**Down (non-destructive)**

```bash
docker compose --project-directory docker \
  -p polyglot-lab-staging \
  -f docker/compose.staging.yaml \
  down --remove-orphans
```

**Reset (destructive; removes volumes)**

```bash
docker compose --project-directory docker \
  -p polyglot-lab-staging \
  -f docker/compose.staging.yaml \
  down -v --remove-orphans
```

</details>

---

## Common operator commands

These are the “90% commands” you’ll use while iterating.

> [!TIP]
> Keep `-p` and `-f` consistent per run so you operate on the same Compose project.

### Show what’s running

```bash
docker compose --project-directory docker \
  -p polyglot-lab-integration \
  -f docker/compose.integration.nosecrets.yaml \
  ps
```

### Tail logs (all services)

```bash
docker compose --project-directory docker \
  -p polyglot-lab-integration \
  -f docker/compose.integration.nosecrets.yaml \
  logs -f --tail=200
```

### Tail logs (one service)

```bash
docker compose --project-directory docker \
  -p polyglot-lab-integration \
  -f docker/compose.integration.nosecrets.yaml \
  logs -f --tail=200 golang-gin-app
```

### Restart a service

```bash
docker compose --project-directory docker \
  -p polyglot-lab-integration \
  -f docker/compose.integration.nosecrets.yaml \
  restart golang-gin-app
```

### Render the merged Compose config (debug `include:` / overlays)

```bash
docker compose --project-directory docker \
  -p polyglot-lab-integration \
  -f docker/compose.integration.nosecrets.yaml \
  config
```

---

## Operator verification checklist

Once the stack is running, these checks confirm the “happy path” end-to-end.

### 0) Generate traffic (to light up dashboards)

If the system is idle, generate a little traffic so **metrics / logs / traces** populate quickly.

> [!TIP]
> **[`../README.md#traffic-generator`](../README.md#traffic-generator)**

### 1) Apps are ready

```bash
curl -fsS http://127.0.0.1:8081/ready
curl -fsS http://127.0.0.1:8082/ready
curl -fsS http://127.0.0.1:8083/ready
```

### 2) Prometheus sees targets and is scraping

Open: `http://127.0.0.1:9090/targets` and confirm all app targets are **UP**.

### 3) Grafana is up (and provisioned)

- Open: `http://127.0.0.1:3000`
- No-secrets stacks default to `admin/admin` (unless overridden in Compose).
- Confirm datasources exist for **Prometheus**, **Loki**, and **Tempo**.
- Open the provisioned dashboard: `polyglot-http.json` (via Dashboards).

### 4) Traces are flowing

- Grafana → **Explore** → **Tempo**
- Query for traces (or generate traffic and search by **service name**)

### 5) Logs are flowing

- Grafana → **Explore** → **Loki**
- Query logs by service/container labels (generate traffic first if idle)

> [!TIP]
> **Logs → traces:** in **Explore → Loki**, expand a log line and click **TraceID** to open the linked trace in **Tempo**.

### 6) Alerting pipeline is reachable

- Alertmanager UI: `http://127.0.0.1:9093`
- Optional (secrets stacks): validate Telegram receiver wiring via your Alertmanager config + secrets.

### What to click in Grafana (fast validation)

- **Dashboards →** open `polyglot-http` (provisioned from `grafana/provisioning/dashboards/json/polyglot-http.json`).
- **Explore → Prometheus →** run a quick query like `http_requests_total` (or any service-specific metric).
- **Explore → Loki →** filter logs by service/container (generate a few requests first if idle).
- **Explore → Tempo →** search traces by **service name**; when available, pivot from logs to traces using shared correlation fields.

---

## Services and local ports (host → container)

### Apps

| Service | Host URL | Container |
|---|---|---|
| **Go (Gin)** | `http://127.0.0.1:8081` | `golang-gin-app:8080` |
| **Java (Spring Boot)** | `http://127.0.0.1:8082` | `java-springboot-app:8080` |
| **Python (Django)** | `http://127.0.0.1:8083` | `python-django-app:8080` |

### Observability UIs/APIs

| Component | Host URL | Container |
|---|---|---|
| **Grafana** | `http://127.0.0.1:3000` | `grafana:3000` |
| **Prometheus** | `http://127.0.0.1:9090` | `prometheus:9090` |
| **Alertmanager** | `http://127.0.0.1:9093` | `alertmanager:9093` |
| **Tempo (query/frontend)** | `http://127.0.0.1:3200` | `tempo:3200` |
| **Loki** | `http://127.0.0.1:3100` | `loki:3100` |
| **Alloy OTLP/gRPC** | `127.0.0.1:4317` | `alloy:4317` |
| **Alloy OTLP/HTTP** | `http://127.0.0.1:4318` | `alloy:4318` |
| **Alloy UI/status** | `http://127.0.0.1:12345` | `alloy:12345` |

---

## Compose layout

### Compose entrypoints (top-level)

- `compose.development.yaml` — apps only (built locally)
- `compose.integration.nosecrets.yaml` — apps + observability (**no secrets overlays**)
- `compose.integration.yaml` — apps + observability (**includes secrets overlays**)
- `compose.staging.nosecrets.yaml` — apps + observability, staging-style images (pulled) (**no secrets overlays**)
- `compose.staging.yaml` — apps + observability, staging-style images (pulled) (**includes secrets overlays**)

### Included building blocks (via `include:`)

- `compose._network.yaml` — `polyglot-net` bridge network
- `compose._volumes.shared.yaml` — shared `toolbox` volume
- `compose._toolbox.yaml` — `toolbox-init` copies BusyBox into the shared volume (used by healthchecks)
- `compose._apps.yaml` — app services (ports, env files, builds, healthchecks, hardening)
- `compose._apps.staging.yaml` — switches app services from local builds → registry images
- `compose._volumes.observability.yaml` — persistent volumes for Alertmanager/Alloy/Grafana/Loki/Prometheus/Tempo
- `compose._observability.yaml` — Alertmanager/Alloy/Grafana/Loki/Prometheus/Tempo services (+ init helpers)
- `compose._app_prereqs.yaml` — apps depend on `alloy` being healthy + `toolbox-init`
- `compose._secrets.grafana.yaml` — Grafana admin user/pass via Docker secrets (**uses `!override`**)
- `compose._secrets.telegram.yaml` — Alertmanager Telegram secrets + env wiring (**uses `!override`**)

---

## Configuration

### Variables used by Compose/configs in this module

| Variable | Default | Used for |
|---|---:|---|
| `APP_ENV` | `integration` | selects `../<module>/.env.${APP_ENV}` for each app service |
| `APP_VERSION` | `1.0.0` | app image tag (staging stacks) |
| `REGISTRY` | *(required for staging pulls)* | image registry namespace (staging stacks) |
| `APP_UID` | `10001` | container user id for app services (hardening) |
| `APP_GID` | `10001` | container group id for app services (hardening) |
| `BUILD_TIME` | `unknown` | build metadata injected into app images (build args) |
| `PROMETHEUS_LOG_LEVEL` | `info` | Prometheus log level |
| `PROMETHEUS_EXTERNAL_URL` | `http://127.0.0.1:9090` | Prometheus external URL |
| `ALERTMANAGER_LOG_LEVEL` | `info` | Alertmanager log level |
| `GRAFANA_LOG_LEVEL` | `info` | Grafana log level |
| `GRAFANA_ADMIN_USER` | `admin` | Grafana admin username (no-secrets stacks) |
| `GRAFANA_ADMIN_PASSWORD` | `admin` | Grafana admin password (no-secrets stacks) |

> [!NOTE]
> In the secrets-enabled stacks (`compose.integration.yaml`, `compose.staging.yaml`), Grafana admin credentials and Alertmanager Telegram secrets are provided via Docker secrets overlays (`compose._secrets.*.yaml`).

---

## Request processing pipeline

### Observability: how signals flow

When running the full stack:

1. **Apps emit traces** using OTLP (**HTTP** or **gRPC**) to **Alloy** (`alloy:4318` / `alloy:4317`).
2. **Alloy forwards traces** to **Tempo**.
3. **Apps expose metrics** on `/metrics`; **Prometheus scrapes** them directly:
   - `golang-gin-app:8080/metrics`
   - `java-springboot-app:8080/metrics`
   - `python-django-app:8080/metrics`
4. **Tempo metrics-generator** remote-writes derived metrics to **Prometheus** (configured in `tempo/tempo.yaml`).
5. **Docker container logs** are tailed by **Alloy** (via Docker socket) and shipped to **Loki**.
6. **Grafana** is provisioned with datasources + dashboards pointing at Prometheus/Loki/Tempo.

Config entrypoints:

- Alloy: `alloy/config.alloy`
- Prometheus: `prometheus/prometheus.yaml.tpl` rendered by `prometheus/entrypoint.sh`
- Alert rules: `prometheus/alerts.yaml`
- Tempo: `tempo/tempo.yaml`
- Loki: `loki/loki.yaml`
- Grafana provisioning: `grafana/provisioning/**`

---

## Observability

### What runs

- **Alloy** (OTel collector + log shipping)
- **Prometheus** (scraping + alert rule evaluation)
- **Alertmanager** (routes alerts; optional Telegram integration via secrets overlay)
- **Loki** (log store)
- **Tempo** (trace store + metrics-generator)
- **Grafana** (dashboards + Explore)

### Dashboards & alerts (out of the box)

#### Grafana provisioning (datasources + dashboards)

Grafana is **pre-provisioned** so it comes up already wired for metrics/logs/traces:

- Datasources: `grafana/provisioning/datasources/datasources.yaml`
- Dashboards provider: `grafana/provisioning/dashboards/provider.yaml`
- Dashboard JSON: `grafana/provisioning/dashboards/json/polyglot-http.json`

#### Prometheus alert rules

`prometheus/alerts.yaml` contains alert rules that match the service contract and local wiring:

- `ServiceDown` — a target is not up / scraped
- `ReadinessFailing` — `/ready` probe indicates the service is not ready
- `ScrapeMetricsFailing` — Prometheus cannot scrape `/metrics`
- `High4xxRate` — elevated 4xx rate (client-side errors) over a window
- `High5xxRate` — elevated 5xx rate (server-side errors) over a window
- `HighLatencyP95` — p95 latency above threshold (RED-style signal)
- `NoTraffic` — no requests observed (helps catch “silent” failures)

#### Alertmanager routing + Telegram integration (optional)

Alertmanager is configured via templates and overlays:

- Config template: `alertmanager/alertmanager.yaml.tpl`
- Telegram message template: `alertmanager/templates/telegram.tmpl`
- Secrets overlay (enables Telegram wiring): `compose._secrets.telegram.yaml`
- Local secret bootstrap: `/.bootstrap-local.sh` (writes `docker/secrets/*` with safe perms/ownership)

---

## Operational knobs

### Secrets (Grafana admin + Alertmanager Telegram)

- Grafana secrets overlay: `compose._secrets.grafana.yaml`
- Alertmanager Telegram overlay: `compose._secrets.telegram.yaml`
- Alertmanager template: `alertmanager/templates/telegram.tmpl`

> [!NOTE]
> Secrets overlays use `!override` to replace Grafana’s env/entrypoint and to wire secrets into Alertmanager safely.

### Local bootstrap for secrets and permissions (`/.bootstrap-local.sh`)

If you use the **secrets-enabled** stacks:

- `docker/compose.integration.yaml`
- `docker/compose.staging.yaml`

…run the repo’s bootstrap script once to create the expected **Docker secrets files** and ensure the required **permissions** for Grafana/Alertmanager and provisioning directories.

#### What it does (idempotent)

`./.bootstrap-local.sh` (from the repo root):

- requires these env vars:
  - `GRAFANA_ADMIN_USER`
  - `GRAFANA_ADMIN_PASSWORD`
  - `TELEGRAM_BOT_TOKEN`
  - `TELEGRAM_CHAT_ID`
- creates `docker/secrets/` with strict permissions (`0711`, owned by `root:root`)
- writes secrets files (regular files only; refuses symlinks; refuses if secrets are tracked by git):
  - `docker/secrets/grafana_admin_user` *(mode `0400`, owned by Grafana UID `472` by default)*
  - `docker/secrets/grafana_admin_password` *(mode `0400`, owned by Grafana UID `472` by default)*
  - `docker/secrets/telegram_bot_token` *(mode `0400`, owned by Alertmanager UID `65534` by default)*
  - `docker/secrets/telegram_chat_id` *(mode `0400`, owned by Alertmanager UID `65534` by default)*
- ensures `docker/prometheus/entrypoint.sh` is executable
- ensures Grafana provisioning directories exist and have safe perms:
  - `docker/grafana/provisioning/alerting`
  - `docker/grafana/provisioning/plugins`

Optional:
- `LOG_LEVEL=quiet|info|debug` (default: `info`)
- `NVD_API_KEY` (only needed for OWASP Dependency-Check runs)

#### Usage

```bash
export GRAFANA_ADMIN_USER=admin
export GRAFANA_ADMIN_PASSWORD='supersecret'
export TELEGRAM_BOT_TOKEN='...'
export TELEGRAM_CHAT_ID='...'

./.bootstrap-local.sh
```

Dry-run (prints actions without writing files):

```bash
LOG_LEVEL=debug ./.bootstrap-local.sh --dry-run
```

> [!TIP]
> Advanced: you can override service UIDs via `GRAFANA_UID`, `GRAFANA_GID`, `ALERTMANAGER_UID`, `ALERTMANAGER_GID` if your environment requires it.

### Hardening / runtime constraints

Apps and observability containers use common hardening settings (cap drops, tmpfs, read-only where possible). See:

- app hardening anchors in `compose._apps.yaml`
- observability hardening anchors in `compose._observability.yaml`

---

## Testing

Recommended local sanity checks:

```bash
cd docker

# Compose parse/merge validation (renders final config)
docker compose -f compose.integration.nosecrets.yaml config

# Optional: secrets stack config render check
docker compose -f compose.integration.yaml config
```

> [!NOTE]
> If you’re debugging path resolution or prefer root-run parity, you can also run:
>
> ```bash
> docker compose --project-directory docker -f docker/compose.integration.nosecrets.yaml config
> ```

---

## Container image details

This module mostly **references upstream images** for observability services and **builds/pulls** the app images.

### App images (local build vs staging pull)

- Development/integration stacks build app images from `../<module>/Dockerfile` (see `compose._apps.yaml`).
- Staging stacks pull images from `${REGISTRY}` using `*-app:${APP_VERSION}` (see `compose._apps.staging.yaml`).

### Healthcheck toolbox

Healthchecks use a small BusyBox binary staged into a shared volume:

- `toolbox-init` copies BusyBox into the `toolbox` volume (`compose._toolbox.yaml`)
- healthchecks invoke `/tools/busybox wget …`

---

## Implementation map (where to look)

### Compose entrypoints

- `compose.development.yaml` — apps only entrypoint
- `compose.integration.nosecrets.yaml` — full stack entrypoint (no secrets overlays)
- `compose.integration.yaml` — full stack entrypoint (with secrets overlays)
- `compose.staging*.yaml` — staging entrypoints (pull app images)

### App and obs wiring

- `compose._apps.yaml` / `compose._apps.staging.yaml` — app service wiring
- `compose._observability.yaml` — observability stack wiring (+ init services)

### Signal configs

- `alloy/config.alloy` — OTLP ingest + Docker log shipping + forwarding
- `prometheus/prometheus.yaml.tpl` + `prometheus/entrypoint.sh` — scrape config rendering
- `prometheus/alerts.yaml` — alert rules
- `tempo/tempo.yaml` — Tempo storage + metrics-generator
- `loki/loki.yaml` — Loki config
- `grafana/provisioning/**` — datasources + dashboards

### Alerting configs

- `alertmanager/alertmanager.yaml.tpl` — Alertmanager config template
- `alertmanager/entrypoint.sh` — renders the template at startup (substitutes env/secrets)
- `alertmanager/templates/telegram.tmpl` — Telegram notification template

---

## Interactions with other modules

- Runs `/golang-gin`, `/java-springboot`, `/python-django` as `*-app` services with stable host ports (`8081/8082/8083`).
- Provides a consistent **observability contract** across apps:
  - Prometheus scrapes `/metrics`
  - logs flow through Alloy → Loki
  - traces flow through Alloy → Tempo
- Secrets overlays let you run “closer to production” behaviors locally without hardcoding sensitive values.
