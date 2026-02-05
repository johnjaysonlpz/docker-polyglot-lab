# /docker — Docker Compose stacks (**apps + observability**)

This module is the **runtime glue** for the whole repo. It provides Docker Compose stacks that run:

- the three app services (`/golang-gin`, `/java-springboot`, `/python-django`), and
- a full local observability stack: **Alloy, Prometheus, Loki, Tempo, Grafana**.

**Signals (repo stack):** apps export **traces → Alloy → Tempo**, **metrics → Prometheus**, **logs → Loki**, with Grafana pre-provisioned.

> Compose implementation note: these stacks use Docker Compose **`include:`** (Compose v2 feature). Secrets-enabled stacks also use YAML tags like `!override` inside the secrets overlays.

---

## TL;DR

The commands below are written to be **runnable from the repository root** (supported workflow). They use `--project-directory docker` so Compose `include:` paths resolve correctly.

### A) Development (apps only, local builds)

#### up
```bash
BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
APP_ENV=development APP_VERSION=development \
docker compose --project-directory docker \
  -p polyglot-lab-development -f docker/compose.development.yaml \
  up --build --remove-orphans
```

#### down (also remove volumes)
```bash
docker compose --project-directory docker \
  -p polyglot-lab-development -f docker/compose.development.yaml \
  down -v --remove-orphans
```

### B) Integration (apps + observability, NO secrets overlays)

#### up
```bash
BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
APP_ENV=integration APP_VERSION=2.0.0 \
docker compose --project-directory docker \
  -p polyglot-lab-integration -f docker/compose.integration.nosecrets.yaml \
  up --build --remove-orphans
```

#### down (also remove volumes)
```bash
docker compose --project-directory docker \
  -p polyglot-lab-integration -f docker/compose.integration.nosecrets.yaml \
  down -v --remove-orphans
```

### C) Integration (apps + observability, WITH secrets overlays)

#### up
```bash
BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
APP_ENV=integration APP_VERSION=2.0.0 \
docker compose --project-directory docker \
  -p polyglot-lab-integration -f docker/compose.integration.yaml \
  up --build --remove-orphans

```

#### down (also remove volumes)
```bash
docker compose --project-directory docker \
  -p polyglot-lab-integration -f docker/compose.integration.yaml \
  down -v --remove-orphans
```

### D) Staging (pull images)

#### up (NO secrets)
```bash
APP_ENV=staging APP_VERSION=2.0.0 \
REGISTRY=docker.io/johnjaysonlopez \
docker compose --project-directory docker \
  -p polyglot-lab-staging -f docker/compose.staging.nosecrets.yaml \
  up --pull always --remove-orphans
```

#### down (NO secrets; also remove volumes)
```bash
APP_ENV=staging APP_VERSION=2.0.0 \
REGISTRY=docker.io/johnjaysonlopez \
docker compose --project-directory docker \
  -p polyglot-lab-staging -f docker/compose.staging.nosecrets.yaml \
  down -v --remove-orphans
```

#### up (WITH secrets)
```bash
BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
APP_ENV=staging APP_VERSION=2.0.0 \
REGISTRY=docker.io/johnjaysonlopez \
docker compose --project-directory docker \
  -p polyglot-lab-staging -f docker/compose.staging.yaml \
  up --pull always --remove-orphans
```

#### down (WITH secrets; also remove volumes)
```bash
BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
APP_ENV=staging APP_VERSION=2.0.0 \
REGISTRY=docker.io/johnjaysonlopez \
docker compose --project-directory docker \
  -p polyglot-lab-staging -f docker/compose.staging.yaml \
  down -v --remove-orphans
```

---

## Operator verification checklist (after `up`)

Once the stack is running, these quick checks confirm the “happy path” end-to-end:

### Generate traffic (to light up dashboards)

If the system is idle, generate a little traffic so **metrics/logs/traces** populate quickly:

```bash
TARGETS=(
  "svc-a root (200)|http://127.0.0.1:8081/"
  "svc-b root (200)|http://127.0.0.1:8082/"
  "svc-c root (200)|http://127.0.0.1:8083/"

  "svc-a info (200)|http://127.0.0.1:8081/info"
  "svc-b info (200)|http://127.0.0.1:8082/info"
  "svc-c info (200)|http://127.0.0.1:8083/info"

  "svc-a WRONG /nope (404)|http://127.0.0.1:8081/nope"
  "svc-b WRONG /nope (404)|http://127.0.0.1:8082/nope"
  "svc-c WRONG /nope (404)|http://127.0.0.1:8083/nope"

  "svc-a WRONG /infoo (404)|http://127.0.0.1:8081/infoo"
  "svc-b WRONG /infoo (404)|http://127.0.0.1:8082/infoo"
  "svc-c WRONG /infoo (404)|http://127.0.0.1:8083/infoo"
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

1. **Apps are ready**
   - `curl -fsS http://127.0.0.1:8081/ready`
   - `curl -fsS http://127.0.0.1:8082/ready`
   - `curl -fsS http://127.0.0.1:8083/ready`

2. **Prometheus sees targets and is scraping**
   - Open `http://127.0.0.1:9090/targets` and confirm all app targets are **UP**.

3. **Grafana is up (and provisioned)**
   - Open `http://127.0.0.1:3000` (defaults: `admin/admin` in no-secrets stacks).
   - Confirm datasources exist for **Prometheus**, **Loki**, and **Tempo**.
   - Open the provisioned dashboard: `polyglot-http.json` (via Dashboards).

4. **Traces are flowing**
   - In Grafana Explore, select the **Tempo** datasource and query for traces (or generate traffic against the apps and then search by service name).

5. **Logs are flowing**
   - In Grafana Explore, select the **Loki** datasource and query logs (e.g., by service/container labels).

6. **Alerting pipeline is reachable**
   - Open `http://127.0.0.1:9093` (Alertmanager UI).
   - Optional (secrets stacks): validate Telegram receiver wiring via your Alertmanager config and secrets.

### What to click in Grafana (fast validation)

After the stack is up, you can validate each signal in under a minute:

- **Dashboards →** open `polyglot-http` (provisioned from `grafana/provisioning/dashboards/json/polyglot-http.json`).
- **Explore → Prometheus →** run a quick query like `http_requests_total` (or any service-specific metric).
- **Explore → Loki →** filter logs by service/container (generate a few requests first if the system is idle).
- **Explore → Tempo →** search traces by **service name**; when available, pivot from logs to traces using shared correlation fields.

---

## Running

Use the **TL;DR** commands above as the canonical entrypoints (dev / integration / staging). They are written to be runnable **from the repo root** using `--project-directory docker`.

Most operators use:
- **Integration (no secrets):** `docker/compose.integration.nosecrets.yaml`
- **Staging (pull images):** `docker/compose.staging.nosecrets.yaml` (or `docker/compose.staging.yaml` with secrets)

## What this module provides

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

## Services and local ports (host → container)

Apps:
- Go (Gin): `http://127.0.0.1:8081` → `golang-gin-app:8080`
- Java (Spring Boot): `http://127.0.0.1:8082` → `java-springboot-app:8080`
- Python (Django): `http://127.0.0.1:8083` → `python-django-app:8080`

Observability UIs/APIs:
- Grafana: `http://127.0.0.1:3000` → `grafana:3000`
- Prometheus: `http://127.0.0.1:9090` → `prometheus:9090`
- Alertmanager: `http://127.0.0.1:9093` → `alertmanager:9093`
- Tempo (query/frontend): `http://127.0.0.1:3200` → `tempo:3200`
- Alloy:
  - OTLP/gRPC: `127.0.0.1:4317` → `alloy:4317`
  - OTLP/HTTP: `http://127.0.0.1:4318` → `alloy:4318`
  - UI/status: `http://127.0.0.1:12345` → `alloy:12345`

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

> In the secrets-enabled stacks (`compose.integration.yaml`, `compose.staging.yaml`), Grafana admin credentials and Alertmanager Telegram secrets are provided via Docker secrets overlays (`compose._secrets.*.yaml`).

---

## Request processing pipeline

### Observability: how signals flow

When running the full stack:

1. **Apps emit traces** using OTLP (**HTTP** or **gRPC**) to **Alloy** (`alloy:4318`/`4317`).
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

- Alloy (OTel collector + log shipping)
- Prometheus (scraping + alert rule evaluation)
- Alertmanager (routes alerts; optional Telegram integration via secrets overlay)
- Loki (log store)
- Tempo (trace store + metrics-generator)
- Grafana (dashboards + Explore)

### Dashboards & alerts (what you get out of the box)

#### Grafana provisioning (datasources + dashboards)

Grafana is **pre-provisioned** so it comes up already wired for metrics/logs/traces:

- Datasources: `grafana/provisioning/datasources/datasources.yaml`
- Dashboards provider: `grafana/provisioning/dashboards/provider.yaml`
- Dashboard JSON: `grafana/provisioning/dashboards/json/polyglot-http.json`

#### Prometheus alert rules

`prometheus/alerts.yaml` contains real alert rules that match the service contract and local port wiring:

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

> Secrets overlays use `!override` to replace Grafana’s env/entrypoint and to wire secrets into Alertmanager safely.

### Local bootstrap for secrets and permissions (`/.bootstrap-local.sh`)

If you use the **secrets-enabled** stacks:

- `docker/compose.integration.yaml`
- `docker/compose.staging.yaml`

…you should run the repo’s bootstrap script once to create the expected **Docker secrets files** and to ensure the required **permissions** for Grafana/Alertmanager and provisioning directories.

#### What it does (idempotent)

`./.bootstrap-local.sh` (from the repo root):

- requires these env vars:
  - `GRAFANA_ADMIN_USER`
  - `GRAFANA_ADMIN_PASSWORD`
  - `TELEGRAM_BOT_TOKEN`
  - `TELEGRAM_CHAT_ID`
- creates `docker/secrets/` with strict permissions (`0711`, owned by `root:root`)
- writes the secrets files (regular files only; refuses symlinks; refuses if secrets are tracked by git):
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

> Advanced: you can override the expected service UIDs via `GRAFANA_UID`, `GRAFANA_GID`, `ALERTMANAGER_UID`, `ALERTMANAGER_GID` if your environment requires it.

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

- `compose.development.yaml` — apps only entrypoint
- `compose.integration.nosecrets.yaml` — full stack entrypoint (no secrets overlays)
- `compose.integration.yaml` — full stack entrypoint (with secrets overlays)
- `compose.staging*.yaml` — staging entrypoints (pull app images)
- `compose._apps.yaml` / `compose._apps.staging.yaml` — app service wiring
- `compose._observability.yaml` — observability stack wiring (+ init services)
- `alloy/config.alloy` — OTLP ingest + Docker log shipping + forwarding
- `prometheus/prometheus.yaml.tpl` + `prometheus/entrypoint.sh` — scrape config rendering
- `prometheus/alerts.yaml` — alert rules
- `tempo/tempo.yaml` — Tempo storage + metrics-generator
- `loki/loki.yaml` — Loki config
- `grafana/provisioning/**` — datasources + dashboards
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
