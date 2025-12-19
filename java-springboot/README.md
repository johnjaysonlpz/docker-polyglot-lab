# java-springboot-app

A small, production-style HTTP service built with **Java 21** and **Spring Boot 3.5.x**, designed as the Spring Boot component of the `docker-polyglot-lab` project.

This service focuses on **operational concerns** rather than business logic:

- Health & readiness endpoints
- Structured JSON logging (Logback + logstash encoder)
- Request correlation via `X-Request-ID` (stored in MDC as `request_id`)
- Prometheus metrics (`/metrics`) via Micrometer + Prometheus registry
- Graceful shutdown with Spring Boot + Tomcat tuning
- Correct client IP handling behind proxies (Forwarded headers)
- Configuration via `application.yaml` + environment variables
- Multi-stage Docker build, non-root runtime

---

## Quick start

### Prerequisites

- Java **21**
- Maven **3.9+**
- Docker (optional, for containerized runs)

### Clone and enter the service directory

```bash
git clone https://github.com/johnjaysonlpz/docker-polyglot-lab.git
cd docker-polyglot-lab/java-springboot
```

### Run tests and build (local)

```bash
mvn clean package
```

The packaged jar will be at:

```text
target/java-springboot-1.0.0.jar
```

### Run locally (without Docker)

Using Maven:

```bash
SPRING_PROFILES_ACTIVE=dev \
HOST=0.0.0.0 \
PORT=8080 \
LOG_LEVEL=DEBUG \
mvn spring-boot:run
```

Or using the built jar:

```bash
java -jar \
  -Dspring.profiles.active=dev \
  target/java-springboot-1.0.0.jar
```

Then hit:

```bash
curl http://localhost:8080/
curl http://localhost:8080/info
curl http://localhost:8080/health
curl http://localhost:8080/ready
curl http://localhost:8080/metrics
```

---

## Running with Docker

### Build the image

The Dockerfile uses a multi-stage build (Maven builder → JRE runtime) and optionally runs tests.
It also uses **BuildKit cache mounts** for faster Maven builds.

```bash
docker build \
  --build-arg RUN_TESTS=true \
  --build-arg SERVICE_NAME=java-springboot-app \
  --build-arg VERSION=1.0.0 \
  --build-arg BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -t java-springboot-app:1.0.0 .
```

### Run (prod-like, using `.env.int`)

```bash
docker run -d \
  --name java-springboot-app \
  --restart unless-stopped \
  --env-file .env.int \
  -p 8082:8080 \
  java-springboot-app:1.0.0
```

Check status:

```bash
docker ps
docker logs -f java-springboot-app
```

Example logs (JSON to stdout):

```bash
{"@timestamp":"2025-12-19T07:47:35.109264374Z","message":"Tomcat initialized with port 8080 (http)","logger_name":"org.springframework.boot.web.embedded.tomcat.TomcatWebServer","thread_name":"main","level":"INFO"}
```

The resulting image:
- Uses `eclipse-temurin:21-jre-alpine` as runtime
- Includes only `curl` + CA certs
- Runs as non-root user `appuser` (UID `10001`)
- Has a Docker `HEALTHCHECK` hitting `/health`
- Uses `JAVA_TOOL_OPTIONS` for container-safe JVM defaults
- Uses a direct `ENTRYPOINT ["java", "-jar", ...]` (no `sh -c`)

---

## HTTP API

All endpoints are HTTP GET.

| Path       | Description                                               | Status codes                                |
| ---------- | --------------------------------------------------------- | ------------------------------------------- |
| `/`        | `"java-springboot-app is running (Java + Spring Boot)\n"` | `200`                                       |
| `/info`    | Metadata (`service`, `version`, `buildTime`)              | `200`                                       |
| `/health`  | Liveness probe (based on Spring `LivenessState`)          | `200` if healthy, `500` otherwise           |
| `/ready`   | Readiness probe (based on Spring `ReadinessState`)        | `200` if accepting traffic, `503` otherwise |
| `/metrics` | Prometheus metrics (text exposition format)               | `200`                                       |

Examples (Docker, mapped to host port 8082):

```bash
curl http://localhost:8082/
curl http://localhost:8082/info
curl http://localhost:8082/health
curl http://localhost:8082/ready
curl http://localhost:8082/metrics
```

Actuator endpoints are exposed under `/actuator` for `health` and `info`, but the primary “infra” endpoints in this app are the top-level paths above.

---

## Observability

### Request correlation (`X-Request-ID`)

Request correlation is implemented in `RequestIdFilter`:
- If the client sends `X-Request-ID`, the service reuses it
- Otherwise, the service generates one (UUID)
- The value is always returned in the response header `X-Request-ID`
- It is also stored in MDC as `request_id` and included in logs

```bash
curl -i -H "X-Request-ID: demo-123" http://localhost:8082/info
```

### Structured logging

Logging is configured for consistent JSON output:

- Logback emits JSON to stdout
- `LOG_LEVEL` is honored (root level is not hardcoded)
- Spring banner is disabled for cleaner logs
- HTTP access logs are emitted by the logger `"http"` and include real JSON fields using structured arguments
- Severity is based on status code:
    - `2xx/3xx` -> `INFO`
    - `4xx` -> `WARN`
    - `5xx` -> `ERROR`
- Infra endpoints (`/health`, `/ready`, `/metrics`,` /actuator/**`) are not logged (but still counted in metrics)

Example HTTP log (shape):

```bash
{
  "@timestamp":"...",
  "level":"WARN",
  "logger_name":"http",
  "message":"http_request",
  "service":"java-springboot-app",
  "version":"1.0.0",
  "request_id":"demo-123",
  "method":"GET",
  "path":"__unmatched__",
  "rawPath":"/favicon.ico",
  "status":404,
  "ip":"...",
  "latencyMs":3,
  "userAgent":"..."
}

```

### Client IP correctness behind proxies

`application.yaml` enables:

```yaml
server:
  forward-headers-strategy: framework
```

The HTTP logging filter also prefers:
- `X-Forwarded-For` (first IP)
- then `X-Real-IP`
- then falls back to `request.getRemoteAddr()`

This maps cleanly to AWS ALB / reverse proxy deployments.

### Prometheus metrics (Micrometer)

Metrics are implemented via Micrometer + Prometheus:

- `io.micrometer:micrometer-registry-prometheus`
- `PrometheusMeterRegistry` wired in `MetricsConfiguration`
- Metrics scraped via `/metrics` (see `MetricsController`)

Key metrics:

- `http_requests_total{service,method,path,status}` (counter)
- `http_request_duration_seconds{service,method,path,status}` (timer + histogram)
- `build_info{service,version,build_time}` (gauge with value `1`)

For unknown routes, metrics use a stable label:

- `path="__unmatched__"` (raw paths are never used in metrics)

Note: Spring may provide a handler pattern like `/**` for some 404s; the filter normalizes those to `__unmatched__` for metrics and logs.

---

## Configuration

Configuration uses Spring Boot’s externalized configuration with:

- `@ConfigurationProperties(prefix = "app")` via `ServiceProperties`
- Defaults defined in `ServiceProperties` and `application.yaml`
- Validation via Jakarta Bean Validation (`@NotBlank`, `@Min`, `@Max`, `@DurationMin`)

### `app.*` properties

Backed by `ServiceProperties`:

| Property               | Default               | Description                                    |
| ---------------------- | --------------------- | ---------------------------------------------- |
| `app.service-name`     | `java-springboot-app` | Logical service name                           |
| `app.version`          | `0.0.0-dev`           | Build version                                  |
| `app.build-time`       | `unknown`             | Build timestamp                                |
| `app.host`             | `0.0.0.0`             | Bind address                                   |
| `app.port`             | `8080`                | HTTP port (1–65535)                            |
| `app.read-timeout`     | `5s`                  | Connection/read timeout                        |
| `app.idle-timeout`     | `120s`                | Keep-alive/idle timeout                        |
| `app.shutdown-timeout` | `5s`                  | Graceful shutdown timeout (used via lifecycle) |

Environment variables map using Spring’s relaxed binding, e.g.:

- `SERVICE_NAME` → `app.service-name`
- `VERSION` → `app.version`
- `BUILD_TIME` → `app.build-time`
- `HOST` → `app.host`
- `PORT` → `app.port`
- `READ_TIMEOUT` → `app.read-timeout`
- `IDLE_TIMEOUT` → `app.idle-timeout`
- `SHUTDOWN_TIMEOUT` → `app.shutdown-timeout`

---

## `.env` profiles

The project includes environment files for convenience:

- `.env.dev` – local development (DEBUG, short timeouts)
- `.env.int` – integration / prod-like
- `.env.prod` – production baseline

Example `.env.int`:

```env
SPRING_PROFILES_ACTIVE=int
HOST=0.0.0.0
PORT=8080
LOG_LEVEL=INFO

READ_TIMEOUT=2s
IDLE_TIMEOUT=30s
SHUTDOWN_TIMEOUT=5s

MANAGEMENT_ENDPOINT_HEALTH_PROBES_ENABLED=true
MANAGEMENT_HEALTH_LIVENESSSTATE_ENABLED=true
MANAGEMENT_HEALTH_READINESSSTATE_ENABLED=true
```

---

## Testing

Run all tests:

```bash
mvn test
```

Coverage includes:
- Configuration binding + validation (`ServicePropertiesTest`, `InvalidConfigStartupTest`)
- Infra endpoints (`InfraControllerTest`, `MetricsControllerTest`)
- Request ID behavior (`HttpLoggingFilterTest` asserts `X-Request-ID` exists and is reused)
- Stable 404 metrics label (`HttpMetricsUnmatchedTest` asserts `path="__unmatched__"`)

Docker build can run or skip tests:
```bash
docker build --build-arg RUN_TESTS=true -t java-springboot-app:test .
docker build --build-arg RUN_TESTS=false -t java-springboot-app:fast .
```

---

## Project structure

```text
java-springboot/
├── pom.xml
├── src/
│   ├── main/
│   │   ├── java/com/github/johnjaysonlpz/dockerpolyglotlab/javaspringboot/
│   │   │   ├── JavaSpringbootApplication.java        # Bootstraps Spring Boot app
│   │   │   ├── config/
│   │   │   │   ├── ServiceProperties.java            # app.* config + validation
│   │   │   │   ├── ServerConfiguration.java          # Tomcat host/port/timeouts
│   │   │   │   ├── MetricsConfiguration.java         # Micrometer + Prometheus + build_info
│   │   │   │   └── HttpServerMetrics.java            # Cached meters for http_* metrics
│   │   │   └── web/
│   │   │       ├── InfraController.java              # /, /info, /health, /ready
│   │   │       ├── MetricsController.java            # /metrics endpoint
│   │   │       ├── RequestIdFilter.java              # X-Request-ID + MDC request_id
│   │   │       └── HttpLoggingFilter.java            # Logging + metrics per HTTP
│   │   └── resources/
│   │       ├── application.yaml                      # Core config & management
│   │       └── logback-spring.xml                    # JSON logging config
│   └── test/
│       ├── .../JavaSpringbootApplicationTests.java
│       ├── .../config/ServicePropertiesTest.java
│       ├── .../config/InvalidConfigStartupTest.java
│       ├── .../web/HttpLoggingFilterTest.java
│       ├── .../web/HttpMetricsUnmatchedTest.java
│       ├── .../web/InfraControllerTest.java
│       └── .../web/MetricsControllerTest.java
├── Dockerfile
├── .dockerignore
├── .env.dev
├── .env.int
└── .env.prod
```

---

## Notes

- This service intentionally has **minimal business logic** and **strong operational patterns**:
  - health/readiness probes
  - Micrometer + Prometheus metrics
  - JSON structured logging + request correlation (MDC)
  - severity-based HTTP logs
  - forward headers strategy for proxy correctness
  - graceful shutdown & Tomcat tuning
  - Docker best practices (multi-stage build, non-root, health check)
- In the full `docker-polyglot-lab` project, there are equivalent services in:
    - Go + Gin (`/golang-gin `)
    - Python + Django (`/python-django`)

You can use this Spring Boot service as a template for new Java microservices with production-friendly defaults.
