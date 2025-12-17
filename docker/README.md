# Docker – Compose & Prometheus for docker-polyglot-lab

This directory contains **Docker Compose** definitions and **Prometheus** configuration
for running the polyglot lab in different environments:

- `compose.dev.yml` – local dev (apps only)
- `compose.int.yml` – integration / prod-like (apps + Prometheus, build with tests)
- `compose.prod.yml` – production-like (pre-built images + Prometheus)
- `prometheus/prometheus.yml` – Prometheus scrape config for all services

All setups use a shared user-defined network:

- `polyglot-net` – allows containers to reach each other via DNS names:
  - `golang-gin-app`
  - `java-springboot-app`
  - `python-django-app`
  - `prometheus`

---

## Dev environment

Build and run all three apps in dev mode (no Prometheus):

```bash
cd docker

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

If you want Prometheus available in dev as well, you can copy the prometheus service
from compose.int.yml into compose.dev.yml. The current setup keeps dev minimal and
uses the integration environment for full observability.

## Integration environment (apps + Prometheus, tests on build)

Integration runs:
- All three services
- Prometheus with a persistent data volume
- Builds images and runs tests as part of the image build

```bash
cd docker

BUILD_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ) \
  docker compose -f compose.int.yml up --build
```

Services:
- Gin: `http://localhost:8081`
- Spring Boot: `http://localhost:8082`
- Django: `http://localhost:8083`
- Prometheus UI: `http://localhost:9090`

Prometheus scrapes:
- `prometheus:9090`
- `golang-gin-app:8080`
- `java-springboot-app:8080`
- `python-django-app:8080`

Once Prometheus is up (integration or prod), you can try some example queries in the UI:
- `http_requests_total` – see per-service request counts
- `http_request_duration_seconds_bucket` – latency histogram
- `build_info` – one sample per service with `service`, `version`, `build_time`

`compose.int.yml` also demonstrates:

- `depends_on` with `condition: service_healthy` for Prometheus → it waits until all app containers are healthy.
- `deploy.resources.limits` to document intended CPU and memory limits.
  - These limits are enforced in Docker Swarm; Compose (non-Swarm) ignores them, but they still serve as useful documentation.

Stop:

```bash
docker compose -f compose.int.yml down
```

The named volume `polyglot-lab-int_prometheus-data` is kept, so metrics persist.

---

## Production-like environment

`compose.prod.yml` simulates a production deployment where images come from a registry.

Current example:

```yaml
golang-gin-app:
  # In real prod, this would be a registry image, e.g.:
  # image: ghcr.io/your-username/golang-gin-app:1.0.0
  image: golang-gin-app:1.0.0
```

In a real setup, you would:

1. Build and push images to a registry (e.g. GHCR/ECR).
2. Replace the `image`: lines with registry references.
3. Run:

```bash
cd docker
docker compose -f compose.prod.yml up -d
```

Prometheus is configured identically to the integration environment.

Stop:

```bash
docker compose -f compose.prod.yml down
```

---

## Prometheus configuration

`prometheus/prometheus.yml`:

```yaml
global:
  scrape_interval: 15s
  evaluation_interval: 15s

scrape_configs:
  - job_name: "prometheus"
    static_configs:
      - targets: ["prometheus:9090"]

  - job_name: "golang-gin-app"
    static_configs:
      - targets: ["golang-gin-app:8080"]

  - job_name: "java-springboot-app"
    static_configs:
      - targets: ["java-springboot-app:8080"]

  - job_name: "python-django-app"
    static_configs:
      - targets: ["python-django-app:8080"]
```

Example query in the Prometheus UI:
- `http_requests_total`
- `http_request_duration_seconds_bucket`
- `build_info`

---

## Networks and volumes

When you bring up integration:

```bash
docker compose -f compose.int.yml up --build
```

Docker will create:
- Network: `polyglot-lab-int_polyglot-net`
- Volume: `polyglot-lab-int_prometheus-data`

Inspect:

```bash
docker network ls
docker volume ls
```

Clean up explicitly if needed:

```bash
docker network rm polyglot-lab-int_polyglot-net
docker volume rm polyglot-lab-int_prometheus-data
```

Names are prefixed by the project name (`polyglot-lab-dev`, `polyglot-lab-int`, `polyglot-lab-prod`), configured via `name:` in each Compose file.

---

## Notes

- All app containers expose HTTP on port 8080 internally and get mapped to unique host ports:
  - Gin: `8081`
  - Spring Boot: `8082`
  - Django: `8083`
- All app images are built as:
  - multi-stage
  - non-root (user `appuser`)
  - with Docker `HEALTHCHECK` hitting `/health`

This directory focuses purely on **runtime topology** (networks, volumes, metrics) and assumes each service manages its own configuration and tests.
