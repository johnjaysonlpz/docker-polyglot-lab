# Docker, Docker Compose, Prometheus & Grafana for docker-polyglot-lab

This directory contains **Docker Compose** definitions and **Prometheus / Grafana** configuration
for running the polyglot lab in different environments:

- `compose.dev.yml` – local dev (apps only)
- `compose.int.yml` – integration / prod-like (apps + Prometheus + Grafana, builds with tests)
- `compose.prod.yml` – production-like (pre-built images + Prometheus + Grafana)
- `prometheus/prometheus.yml` – Prometheus scrape config for all services
- `grafana/provisioning/` – Grafana provisioning (dashboards + datasources)

All setups use a shared user-defined network:

- `polyglot-net` – allows containers to reach each other via DNS service names:
  - `golang-gin-app`
  - `java-springboot-app`
  - `python-django-app`
  - `prometheus`
  - `grafana`

---

## Grafana provisioning layout (important)

Grafana provisioning must follow this structure:

```text
docker/grafana/provisioning/
  dashboards/
    provider.yml
    dashboard.json
  datasources/
    prometheus.yml
```

- Dashboards providers live under `provisioning/dashboards/`
- Datasources must live under `provisioning/datasources/`

Compose mounts provisioning read-only:

```yaml
- ./grafana/provisioning:/etc/grafana/provisioning:ro
```

---

## Dev environment (apps only)

Dev environment (apps only)

```bash
docker compose -f compose.dev.yml up --build
```

Services:

- Go + Gin: `http://localhost:8081`
- Spring Boot: `http://localhost:8082`
- Django: `http://localhost:8083`

Stop:

```bash
docker compose -f compose.dev.yml down
```

---

## Integration environment (apps + Prometheus + Grafana, tests on build)

Integration runs:

- Integration runs:
- Prometheus with a persistent data volume
- Grafana provisioned with a Prometheus datasource and a dashboard
- Builds images and runs tests as part of the image build

```bash
BUILD_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ) \
  docker compose -f compose.int.yml up --build
```

Services:

- Gin: `http://localhost:8081`
- Spring Boot: `http://localhost:8082`
- Django: `http://localhost:8083`
- Prometheus UI: `http://localhost:9090`
- Grafana UI: `http://localhost:3000`

Grafana credentials (defaults):

- user: `admin`
- pass: `admin`

Override safely:

```bash
GRAFANA_ADMIN_USER=admin \
GRAFANA_ADMIN_PASSWORD=change-me \
docker compose -f compose.int.yml up
```

Stop:

```bash
docker compose -f compose.int.yml down
```

Volumes are kept by default:

- `prometheus-data` persists Prometheus TSDB
- `grafana-data` persists Grafana state

---

## Production-like environment

`compose.prod.yml` simulates a production deployment where images come from a registry.

Run:

```bash
docker compose -f compose.prod.yml up -d
```

Stop:

```bash
docker compose -f compose.prod.yml down
```

---

## Prometheus configuration

`prometheus/prometheus.yml` scrapes all services and adds a consistent service label
at scrape-time (so dashboards don’t depend on language-specific instrumentation).

Key idea:

```yaml
- job_name: "golang-gin-app"
  metrics_path: /metrics
  static_configs:
    - targets: ["golang-gin-app:8080"]
      labels:
        service: golang-gin-app
```

Prometheus scrapes:

- `golang-gin-app:8080` (`/metrics`)
- `java-springboot-app:8080` (`/metrics`)
- `python-django-app:8080` (/`metrics`)
- `prometheus:9090`
- `grafana:3000` (`/metrics`)

Example queries in Prometheus UI:

- `http_requests_total`
- `http_request_duration_seconds_bucket`
- `build_info`

---

## Networks and volumes

When you bring up integration:

```bash
docker compose -f compose.int.yml up --build
```

Docker will create (names are prefixed by the Compose project name):

- Network: `polyglot-net`
- Volumes: `prometheus-data`, `grafana-data`

Inspect:

```bash
docker network ls
docker volume ls
```

Remove explicitly if needed:

```bash
docker volume rm polyglot-lab-int_prometheus-data polyglot-lab-int_grafana-data
```

---

## Notes

- All app containers expose HTTP on port 8080 internally and get mapped to unique host ports:
  - Gin: `8081`
  - Spring Boot: `8082`
  - Django: `8083`
- Prometheus is exposed on `9090`
- Grafana is exposed on `3000`
- All app images are:
  - multi-stage
  - non-root (user `appuser`)
  - with Docker `HEALTHCHECK` hitting `/health`

This directory focuses purely on **runtime topology** (networks, volumes, metrics, dashboards) and assumes each service manages its own configuration and tests.
