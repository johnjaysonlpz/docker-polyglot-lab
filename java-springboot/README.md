# /java-springboot — **Java + Spring Boot** HTTP service (**metrics + traces + structured logs**)

A small, production-shaped HTTP service built with **Java 25** and **Spring Boot 4.0.2** (WebMVC), designed to run standalone or as part of the repo’s Docker Compose lab stack.

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
| **Compose host port** | `127.0.0.1:8082` |
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

Compose wiring for this service:
- service name: **`java-springboot-app`**
- host port: **`127.0.0.1:8082`**
- healthcheck: `GET http://127.0.0.1:8080/ready`
- env file selection: `../java-springboot/.env.${APP_ENV:-integration}`

</details>

<details>
<summary><strong>Run locally (without Docker)</strong></summary>

Requires **Java 25 (Temurin)**. Maven wrapper is provided (`./mvnw`).

> [!NOTE]
> The Dockerfile uses `eclipse-temurin-25` base images (major-version pinned). If you need patch-level reproducibility locally, align your JDK patch version with the image tag you build from.

```bash
cd java-springboot
set -a; source .env.development; set +a
./mvnw spring-boot:run
```

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
The Dockerfile builds the jar, optionally runs tests, extracts Spring Boot layers, downloads the OTel agent, and injects build metadata:

```bash
cd java-springboot

docker build \
  --build-arg RUN_TESTS=true \
  --build-arg SERVICE_NAME=java-springboot-app \
  --build-arg VERSION=2.0.0 \
  --build-arg BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -t java-springboot-app:2.0.0 .
```

#### Run (using `.env.integration`)
```bash
docker run -d \
  --name java-springboot-app \
  --restart unless-stopped \
  --env-file .env.integration \
  -p 8082:8080 \
  java-springboot-app:2.0.0
```

#### Check status
```bash
docker ps
docker logs -f java-springboot-app
```

</details>

---

## What this service provides

- **Infra endpoints**: `GET /`, `GET /health`, `GET /ready`, `GET /info`
- **Prometheus metrics**: `GET /metrics` (Micrometer Prometheus registry; **not** actuator)
- **Consistent JSON errors** with stable `code` + `request_id`
- **Request correlation** via strict **`X-Request-ID`** policy (MDC + response header)
- **Structured JSON access logs** (Logback + logstash encoder) with optional `trace_id` / `span_id`
- **OpenTelemetry tracing** via the **OpenTelemetry Java Agent** (downloaded + SHA256-verified in the image; injected by `entrypoint.sh`)
- **Payload size limiting** (`MAX_BODY_BYTES`) with correct **413** behavior (Content-Length pre-check + streaming enforcement)
- **Virtual threads enabled** (`spring.threads.virtual.enabled: true`)
- **Graceful shutdown**: SIGINT/SIGTERM with configurable shutdown timeout
- Hardened container build (non-root, layer-extracted Spring Boot jar)

---

## HTTP API

### Network
- Binds to: `${HOST}:${PORT}` (defaults: **`0.0.0.0:8080`**)
- Container port: **`8080`**
- Repo compose port mapping: **`127.0.0.1:8082 → 8080`** (service: **`java-springboot-app`**)

### Endpoints
| Method | Path | Purpose |
|---|---|---|
| GET | `/` | Plain-text “service is running” message |
| GET | `/health` | Liveness probe (**200** if `LivenessState.CORRECT`, else **500**) |
| GET | `/ready` | Readiness probe (**200** if `ReadinessState.ACCEPTING_TRAFFIC`, else **503**) |
| GET | `/info` | Build/service metadata (JSON) |
| GET | `/metrics` | Prometheus scrape endpoint (`text/plain; version=0.0.4; charset=utf-8`) |

<details>
<summary><strong>Sample: `/info` response</strong></summary>

```json
{
  "service": "java-springboot-app",
  "version": "2.0.0",
  "build_time": "2026-02-03T07:22:30Z"
}
```

</details>

<details>
<summary><strong>Sample: `/info` log event (JSON)</strong></summary>

```json
{
  "time": "2026-02-03T07:28:22.55809845Z",
  "level": "INFO",
  "msg": "http_request",
  "service": "java-springboot-app",
  "version": "2.0.0",
  "build_time": "2026-02-03T07:22:30Z",
  "request_id": "d7c19038-da50-4112-b93f-085b8eff987d",
  "trace_id": "31d5a46c4d5481247b84c616919e372c",
  "span_id": "f56fc292a3846988",
  "method": "GET",
  "path": "/info",
  "raw_path": "/info",
  "status": 200,
  "ip": "172.18.0.1",
  "latency_ms": 21.338,
  "bytes_written": 87,
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
- **400** → `bad_request` (validation/type mismatch)
- **404** → `not_found` (`NoHandlerFoundException`)
- **405** → `method_not_allowed`
- **413** → `payload_too_large`
- **500** → `internal_server_error`

---

## Configuration

This module reads configuration primarily from environment variables:

- **Spring profile selection** via `SPRING_PROFILES_ACTIVE`
- **Application/server settings** bound to `app.*` (`ServiceProperties`) and applied by `ServerConfiguration`
- **Health probe/availability flags** via Spring Boot management env vars (`MANAGEMENT_*`)
- **Tracing/export settings** via OpenTelemetry env vars (`OTEL_*`) when the Java agent is enabled in Docker

### Application & server env vars (`app.*`)

| Env var | Default | Notes |
|---|---:|---|
| `SPRING_PROFILES_ACTIVE` | `development` | selects `application-<profile>.yaml` (repo provides `development`, `integration`, `staging`) |
| `HOST` | `0.0.0.0` | bind address (`app.host`) |
| `PORT` | `8080` | listen port (`app.port`) |
| `LOG_LEVEL` | `INFO` | root log level (treat as case-insensitive input; mapped to framework levels) |
| `SHUTDOWN_TIMEOUT` | `5s` | graceful shutdown timeout (`spring.lifecycle.timeout-per-shutdown-phase`) |
| `MAX_BODY_BYTES` | `1048576` | max request body bytes; `<= 0` **disables limit** |
| `TOMCAT_CONNECTION_TIMEOUT` | `2s` | Tomcat connector connection timeout |
| `TOMCAT_KEEP_ALIVE_TIMEOUT` | `30s` | keep-alive idle timeout |
| `TOMCAT_MAX_CONNECTIONS` | `8192` | max concurrent connections |
| `TOMCAT_ACCEPT_COUNT` | `100` | accept backlog when busy |
| `TOMCAT_MAX_KEEP_ALIVE_REQUESTS` | `100` | max requests per keep-alive connection |
| `TOMCAT_MAX_THREADS` | `200` | connector max threads |
| `TOMCAT_MIN_SPARE_THREADS` | `10` | connector min spare threads |
| `TOMCAT_INTERNAL_PROXIES` | *(LAN regex)* | trusted proxy allowlist for `RemoteIpValve` (`X-Forwarded-*`) |

Build metadata variables are available as runtime defaults (and are commonly set at image build time):

| Variable | Default | Where it shows up |
|---|---:|---|
| `SERVICE_NAME` | `java-springboot-app` | `/info` JSON, logs, (and labels where applicable) |
| `VERSION` | `0.0.0-dev` | `/info` JSON, logs, `build_info` metric label |
| `BUILD_TIME` | `unknown` | `/info` JSON, logs, `build_info` metric label |

### Health / availability env vars (from `.env.*`)

The repo’s `.env.*` files explicitly enable Spring Boot liveness/readiness probes:

| Env var | `.env.development` | `.env.integration` | `.env.staging` |
|---|---:|---:|---:|
| `MANAGEMENT_ENDPOINT_HEALTH_PROBES_ENABLED` | `true` | `true` | `true` |
| `MANAGEMENT_HEALTH_LIVENESSSTATE_ENABLED` | `true` | `true` | `true` |
| `MANAGEMENT_HEALTH_READINESSSTATE_ENABLED` | `true` | `true` | `true` |

> Note: the service’s **primary** probe endpoints are the application routes `GET /health` and `GET /ready` (those use Spring’s `ApplicationAvailability` states). Actuator health/info are still available under `/actuator/*` with limited exposure.

### OpenTelemetry env vars (from `.env.*`)

> [!NOTE]
> This repo uses a **hybrid model**: Prometheus scrapes `/metrics`, while traces are exported via OTLP to Alloy.

<details>
<summary><strong>Click to expand OTel env var examples (development vs integration/staging)</strong></summary>

The OTel Java agent is injected by `entrypoint.sh` in Docker images. The repo env files configure:

#### `.env.development`
Tracing is disabled explicitly:
- `OTEL_TRACES_EXPORTER=none`
- `OTEL_METRICS_EXPORTER=none`
- `OTEL_LOGS_EXPORTER=none`

#### `.env.integration` / `.env.staging`
Traces are exported to Alloy (OTLP/HTTP protobuf), logs/metrics exporters remain disabled:
- `OTEL_SERVICE_NAME=java-springboot-app`
- `OTEL_TRACES_EXPORTER=otlp`
- `OTEL_METRICS_EXPORTER=none`
- `OTEL_LOGS_EXPORTER=none`
- `OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf`
- `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT=http://alloy:4318/v1/traces`
- `OTEL_TRACES_SAMPLER=always_on`
- log/trace correlation:
  - `OTEL_INSTRUMENTATION_LOGBACK_MDC_ENABLED=true`
  - `OTEL_INSTRUMENTATION_HTTP_CAPTURE_HEADERS_SERVER_REQUEST=X-Request-ID`

### Provided env templates
This module includes:
- `.env.development`
- `.env.integration`
- `.env.staging`

---

</details>

## Request processing pipeline

### Spring MVC behavior
Configured in `application.yaml`:
- `spring.mvc.throw-exception-if-no-handler-found: true` → consistent JSON 404 (no HTML “whitelabel”)
- `spring.web.resources.add-mappings: false` → disables static resources
- `spring.threads.virtual.enabled: true` → virtual threads
- `management.observations.enable.http.server.requests: false` → disables default Micrometer HTTP server observations (this module uses custom metrics)
- graceful shutdown:
  - `server.shutdown: graceful`
  - `spring.lifecycle.timeout-per-shutdown-phase: ${SHUTDOWN_TIMEOUT:5s}`

<details>
<summary><strong>Filter order (outer → inner)</strong></summary>

All filters are `OncePerRequestFilter` with explicit `@Order`:

1. **RequestIdFilter**
   - strict `X-Request-ID` validation (trim, length ≤ 128, safe charset)
   - sets MDC `request_id` and always echoes `X-Request-ID`

2. **HttpLoggingFilter**
   - records custom metrics
   - structured access logs to logger name `http` (`msg="http_request"`)
   - counts response bytes
   - suppresses logs for successful infra paths: `/health`, `/ready`, `/metrics`, `/actuator/**`
   - log levels: `INFO` (<400), `WARN` (4xx), `ERROR` (5xx/exception)

3. **MaxBodyBytesFilter**
   - enforces `MAX_BODY_BYTES`
   - converts limit violations into a consistent 413 JSON response

---

</details>

---

## Payload validation & limits

### `MAX_BODY_BYTES` (default **1 MiB**)
- `MAX_BODY_BYTES == 0` → limit disabled.
- Fast fail:
  - if `Content-Length > limit` → reject before handler runs with **413** (`payload_too_large`).
- Streaming enforcement:
  - wraps the `ServletInputStream` and counts bytes read
  - throws `PayloadTooLargeException(limit)` when exceeded
  - filter converts to **413** if the response isn’t committed

413 response shape:
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

`/metrics` returns the scrape output of a `PrometheusMeterRegistry`.

Custom meters (created on-demand per `{method,path,status}` tag set):
- **`http_requests_total`** — Counter
- **`http_request_duration`** — Timer with SLO boundaries:
  - 5ms, 10ms, 25ms, 50ms, 100ms, 250ms, 500ms, 1s, 2.5s, 5s, 10s
- **`build.info{version,build_time}`** — Gauge always set to `1.0`

Path labeling (low cardinality):
- 404 → `__unmatched__`
- else, uses `HandlerMapping.BEST_MATCHING_PATTERN_ATTRIBUTE` (e.g., `/users/{id}`), when available and not `/**`
- infra paths (`/health`, `/ready`, `/metrics`, `/actuator/**`) → raw path
- otherwise → `__unmatched__`

Access log suppression:
- successful infra paths are **not logged** (but are still counted in metrics).

</details>

<details>
<summary><strong>Tracing (OpenTelemetry Java Agent)</strong></summary>

Tracing is handled by the OTel Java agent shipped in the Docker image.

Agent injection behavior:
- `entrypoint.sh` injects `-javaagent:/otel/opentelemetry-javaagent.jar` into `JAVA_TOOL_OPTIONS` when:
  - `OTEL_JAVAAGENT_ENABLED` is truthy (`1|true|yes|on`, case-insensitive; default `true`), and
  - the agent jar exists at `/otel/opentelemetry-javaagent.jar`.
- `JAVA_TOOL_OPTIONS` starts from `JAVA_TOOL_OPTIONS_BASE` (default includes `-XX:MaxRAMPercentage=75.0` and `-XX:+ExitOnOutOfMemoryError`).

Repo env files typically configure:
- `OTEL_TRACES_EXPORTER=otlp`
- `OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf`
- `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT=http://alloy:4318/v1/traces`
- `OTEL_TRACES_SAMPLER=always_on`
- `OTEL_SERVICE_NAME=java-springboot-app`

Log/trace correlation options commonly set in env:
- `OTEL_INSTRUMENTATION_LOGBACK_MDC_ENABLED=true`
- `OTEL_INSTRUMENTATION_HTTP_CAPTURE_HEADERS_SERVER_REQUEST=X-Request-ID`

</details>

<details>
<summary><strong>Logs</strong></summary>

- JSON logs via `logback-spring.xml`.
- Access logs are emitted with message `"http_request"` to logger name `http`.
- Correlation fields:
  - `request_id` (MDC, always present)
  - `trace_id` / `span_id` when present in MDC (OTel agent + MDC instrumentation)

---

</details>

---

## Error utilities

Centralized error normalization (`ErrorResponse` + `GlobalExceptionHandler`):
- Ensures stable JSON for 400/404/405/413/500.
- Stores handled exceptions under `HANDLED_EXCEPTION_ATTR` so `HttpLoggingFilter` can log root causes even when the exception does not escape the filter chain.

---

## Operational knobs

### Tomcat connector tuning
Configured via env vars and applied in `ServerConfiguration`:
- connection timeout, keep-alive timeout
- max connections + accept count
- thread counts
- max keep-alive requests

### Forwarded headers / client scheme and IP
Configured via Tomcat `RemoteIpValve`:
- `server.forward-headers-strategy: native`
- headers: `x-forwarded-for`, `x-forwarded-proto`
- trust boundary controlled by `TOMCAT_INTERNAL_PROXIES` (only widen if you control the proxy layer)

---

## Testing & linting

Unit/integration verification (this runs formatter + SpotBugs + tests + JaCoCo):
```bash
cd java-springboot
./mvnw verify
```

---

## Local CI

This repo provides a **local parity runner** at the repo root: `./.ci-local.sh` (mirrors `.github/workflows/cicd.yaml`).

> [!NOTE]
> **Tool versions are pinned in `./.ci-tool-versions.sh`**, which is the **single source of truth** for:
> - `./.ci-local.sh`
> - `.github/workflows/cicd.yaml`

### Prerequisites
- bash
- git
- **Java 25** (Temurin)
- Maven (**the CI runner calls `mvn`**, not `./mvnw`)
- `python3.12` (used by the CI script to parse JaCoCo coverage)

> If you only have the Maven wrapper, install Maven or provide a `mvn` shim in your PATH that invokes `./java-springboot/mvnw`.

### Run the Java module CI checks
From repo root:

```bash
source ./.ci-tool-versions.sh
./.ci-local.sh java
```

What it runs for this module (in order):
- `mvn -B -ntp verify` (Spotless, SpotBugs, tests, JaCoCo)
- parses `target/site/jacoco/jacoco.xml` for LINE coverage summary
- coverage gate: **100% LINE coverage** (fails if < 100%)
- (push/tags only) OWASP Dependency-Check Maven plugin:
  - requires `NVD_API_KEY`
  - controlled by `DEPENDENCY_CHECK_MAVEN_VERSION` and `DEPENDENCY_CHECK_FAIL_CVSS`

Useful options:
```bash
./.ci-local.sh doctor java --summary
LOG_LEVEL=debug ./.ci-local.sh java
CI_EVENT_NAME=pull_request ./.ci-local.sh java   # skips Dependency-Check step
```

---

## Container image details

### Dockerfile highlights
- Multi-stage build:
  - builder: Maven + Temurin JDK (`maven:${MAVEN_VERSION}-eclipse-temurin-${TEMURIN_VERSION}-noble`)
  - runtime: Temurin JRE (`eclipse-temurin:${TEMURIN_VERSION}-jre-noble`)
- Extracts Spring Boot layers (layertools) for better Docker caching.
- Downloads OTel agent and verifies SHA256 (`OTEL_JAVA_AGENT_SHA256`).
- Non-root runtime user (**uid=10001**), `STOPSIGNAL SIGTERM`.
- Entrypoint injects the agent conditionally.

---

### Build arguments
| Arg | Default | Purpose |
|---|---:|---|
| `MAVEN_VERSION` | `3.9.12` | Maven builder image version. |
| `TEMURIN_VERSION` | `25` | Java/JRE (Temurin) version for builder/runtime images. |
| `RUN_TESTS` | `true` | If `true`, runs `mvn test` during the Docker build. |
| `SERVICE_NAME` | `java-springboot-app` | Default runtime env + OCI labels (and used by `/info`). |
| `VERSION` | `dev` | Default runtime env + OCI labels (and used by `/info`). |
| `BUILD_TIME` | `local` | Default runtime env + OCI labels (and used by `/info`). |
| `OCI_IMAGE_SOURCE` | "" | Sets OCI label `org.opencontainers.image.source` (typically the repository URL; often populated in CI via `docker/build-push-action` `labels:` or `docker/metadata-action`). |
| `OCI_IMAGE_REVISION` | "" | Sets OCI label `org.opencontainers.image.revision` (typically the Git commit SHA, e.g. GitHub Actions `GITHUB_SHA`; often populated in CI via `docker/build-push-action` `labels:` or `docker/metadata-action`). |
| `OTEL_JAVA_AGENT_VERSION` | `2.24.0` | OpenTelemetry Java agent version to download at build time. |
| `OTEL_JAVA_AGENT_SHA256` | `5c48cd48f9907f824214e22f14a28375c92613a94b0cf98381997e0a678220ce` | Expected SHA256 of the agent JAR (integrity pin). |

## Implementation map (where to look)

| Change you want | Where to look |
|---|---|
| **`pom.xml`** | dependencies + build plugins (Spotless/SpotBugs/JaCoCo) |
| **`mvnw` / `mvnw.cmd`** | Maven wrapper scripts |
| **`Application.java`** | bootstrapping |
| **`InfraController.java`** | `/`, `/info`, `/health`, `/ready` |
| **`MetricsController.java`** | `/metrics` |
| **`RequestIdFilter.java`** | `X-Request-ID` validation + MDC + response header |
| **`HttpLoggingFilter.java`** | access logs + metrics recording + path labeling |
| **`MaxBodyBytesFilter.java` / `PayloadTooLargeException.java`** | 413 enforcement |
| **`GlobalExceptionHandler.java` / `ErrorResponse.java`** | stable JSON errors |
| **`MetricsConfiguration.java` / `HttpServerMetrics.java`** | registry + meters + SLOs |
| **`ServerConfiguration.java` / `ServiceProperties.java`** | bind + Tomcat tuning + validation |
| **`src/main/resources/application.yaml`** | defaults and actuator exposure |
| **`src/main/resources/logback-spring.xml`** | JSON logging schema |
| **`entrypoint.sh`** | OTel agent injection |
| **`.mvn/jvm.config`** | JVM flag `--sun-misc-unsafe-memory-access=allow` |

> [!TIP]
> Prefer starting with the **request pipeline** (middleware/filter chain), then jump to metrics/logging/tracing wiring.

---

## Interactions with other modules

- `/docker` runs this service as `java-springboot-app`, maps it to `127.0.0.1:8082`, scrapes `/metrics`, and routes OTLP traces via Alloy → Tempo.
- `/golang-gin` and `/python-django` follow the same **health/ready/metrics/info + request-id + OTLP** conventions so dashboards/alerts can be shared.
