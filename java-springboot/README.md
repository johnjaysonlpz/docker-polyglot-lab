# java-springboot-app

A small, production-style HTTP service built with **Java 21** and **Spring Boot 3.5.x**, designed as the Spring Boot component of the `docker-polyglot-lab` project.

This service focuses on **operational concerns** rather than business logic:

- Health & readiness endpoints
- Structured JSON logging via Logback + logstash encoder
- Prometheus metrics (`/metrics`) via Micrometer + Prometheus registry
- Graceful shutdown with Spring Boot + Tomcat tuning
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

The Dockerfile uses a multi-stage build (Maven builder → JRE runtime) and optionally runs tests:

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

Example logs:

```bash
19:15:44.328 [main] INFO com.github.johnjaysonlpz.dockerpolyglotlab.javaspringboot.JavaSpringbootApplication -- bootstrapping_application

{"@timestamp":"2025-12-15T19:15:47.441992266Z","level":"INFO","thread_name":"main","logger_name":"com.github.johnjaysonlpz.dockerpolyglotlab.javaspringboot.JavaSpringbootApplication","message":"starting_server service=java-springboot-app version=1.0.0 buildTime=2025-12-15T19:15:16Z addr=0.0.0.0:8080 profiles=int"}
{"@timestamp":"2025-12-15T19:17:10.943084248Z","level":"INFO","thread_name":"tomcat-handler-5","logger_name":"http","message":"http_request service=java-springboot-app version=1.0.0 method=GET path=/ rawPath=/ status=200 ip=172.17.0.1 latencyMs=9 userAgent=\"Mozilla/5.0 ...\""}
```

The resulting image:

- Uses eclipse-temurin:21-jre-alpine as runtime
- Includes only curl + CA certs
- Runs as non-root user appuser (UID 10001)
- Has a Docker HEALTHCHECK hitting /health

---

## HTTP API

All endpoints are HTTP GET.

| Path       | Description                                                              | Status codes                                |
| ---------- | ------------------------------------------------------------------------ | ------------------------------------------- |
| `/`        | Simple banner: `"java-springboot-app is running (Java + Spring Boot)\n"` | `200`                                       |
| `/info`    | Service metadata (`service`, `version`, `buildTime`) as JSON             | `200`                                       |
| `/health`  | Liveness probe (based on Spring `LivenessState`)                         | `200` if healthy, `500` otherwise           |
| `/ready`   | Readiness probe (based on `ReadinessStateHolder`)                        | `200` if accepting traffic, `503` otherwise |
| `/metrics` | Prometheus metrics (text exposition format)                              | `200`                                       |

In a real system, `/ready` would incorporate dependency checks (DB, downstream services, etc.).

Examples (Docker, mapped to host port `8082`):

```bash
curl http://localhost:8082/
curl http://localhost:8082/info
curl http://localhost:8082/health
curl http://localhost:8082/ready
curl http://localhost:8082/metrics
```

Spring Boot Actuator endpoints are also exposed under `/actuator` for `health` and `info`, but the primary “infra” endpoints in this app are the top-level paths above.

---

## Observability

### Structured logging

Logging is handled by Logback with `logstash-logback-encoder`:

- `src/main/resources/logback-spring.xml` configures a JSON console appender
- Root logger writes structured JSON to `stdout`
- HTTP access logs use the dedicated logger `"http"` (configured in `HttpLoggingFilter`)
- Infra endpoints (`/health`, `/ready`, `/metrics`, `/actuator/**`) are not logged to keep noise low

Example HTTP log (from `HttpLoggingFilter`):

```bash
{"@timestamp":"2025-12-15T19:17:19.550369602Z",
 "level":"INFO",
 "thread_name":"tomcat-handler-11",
 "logger_name":"http",
 "message":"http_request service=java-springboot-app version=1.0.0 method=GET path=/info rawPath=/info status=200 ip=172.17.0.1 latencyMs=24 userAgent=\"Mozilla/5.0 ...\""}
```

Application lifecycle logs:

- JavaSpringbootApplication logs bootstrapping_application before startup
- On ApplicationReadyEvent, logs starting_server ...
- On ContextClosedEvent, logs server_shutdown_complete ...

### Prometheus metrics (Micrometer)

Metrics are implemented via Micrometer + Prometheus:

- `io.micrometer:micrometer-registry-prometheus`
- `PrometheusMeterRegistry` wired in `MetricsConfiguration`
- Custom registry uses `PrometheusRegistry` internally
- Metrics scraped via `/metrics` (see `MetricsController`)

Key metrics:

- `http_requests_total{service,method,path,status}` (counter)
- `http_request_duration_seconds{service,method,path,status}` (timer + histogram)
- `build_info{service,version,build_time}` (gauge with value `1`)

Common tags:

- `service`
- `version`

are added via `MeterRegistryCustomizer` using `ServiceProperties`.

The HTTP filter (`HttpLoggingFilter`) records metrics for every request, using:

- `method` (HTTP verb)
- `path` (Spring handler mapping pattern, e.g. `/info`)
- `status` (HTTP status code)
- Request duration (for `http_request_duration_seconds`)

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

Defaults in `application.yaml`:

```yaml
app:
  service-name: ${SERVICE_NAME:java-springboot-app}
  version: ${VERSION:0.0.0-dev}
  build-time: ${BUILD_TIME:unknown}
  host: ${HOST:0.0.0.0}
  port: ${PORT:8080}
  read-timeout: ${READ_TIMEOUT:5s}
  idle-timeout: ${IDLE_TIMEOUT:120s}
  shutdown-timeout: ${SHUTDOWN_TIMEOUT:5s}
```

### Server & lifecycle config

From `application.yaml`:

```yaml
server:
  shutdown: graceful

spring:
  lifecycle:
    timeout-per-shutdown-phase: ${SHUTDOWN_TIMEOUT:5s}
  application:
    name: java-springboot-app
  threads:
    virtual:
      enabled: true
```

`ServerConfiguration` customizes Tomcat based on `ServiceProperties`:

Binds to `app.host` / `app.port`

Sets `connectionTimeout` and `keepAliveTimeout` from `readTimeout` and `idleTimeout`

### Health & probes

Actuator & health configuration:

```yaml
management:
  endpoints:
    web:
      exposure:
        include: health,info
      base-path: /actuator

  endpoint:
    health:
      probes:
        enabled: true

  health:
    livenessstate:
      enabled: true
    readinessstate:
      enabled: true
```

Environment variables like these are used in the `.env.*` files:

- `MANAGEMENT_ENDPOINT_HEALTH_PROBES_ENABLED`
- `MANAGEMENT_HEALTH_LIVENESSSTATE_ENABLED`
- `MANAGEMENT_HEALTH_READINESSSTATE_ENABLED`

### Logging level

From `application.yaml`:

```yaml
logging:
  level:
    root: ${LOG_LEVEL:INFO}
```

Use `LOG_LEVEL=DEBUG` or `LOG_LEVEL=INFO` in `.env.*` files.

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

Use with Docker:

```bash
docker run -d \
  --name java-springboot-app \
  --restart unless-stopped \
  --env-file .env.int \
  -p 8082:8080 \
  java-springboot-app:1.0.0
```

---

## Testing

Tests live under src/test/java/... and cover:

- ### Configuration properties & validation
    - `ServicePropertiesTest`:
        - Binds defaults when unset
        - Binds valid override values
        - Fails validation for invalid values (e.g. port out of range, blank host, zero timeouts)

    - `InvalidConfigStartupTest`:
        - Asserts that invalid config (`--app.port=70000`, `--app.host= `) causes startup failure

- ### Infra web layer
    - `InfraControllerTest`:
        - `/` returns banner
        - `/health` and `/ready` respond `200` under normal conditions
        - `/info` returns expected JSON metadata
        - `/ready` returns `503` when readiness state refuses traffic

    - `MetricsControllerTest`:
        - `/metrics` contains `http_requests_total` after hitting `/`

- ### HTTP logging filter
    - `HttpLoggingFilterTest`:

        - Verifies `/health`, `/ready`, `/metrics` produce no `"http_request"` logs
        - Verifies application routes (like `/`) do produce `"http_request"` logs

Run all tests:

```bash
mvn test
```

The Docker build also supports toggling tests:

```bash
# Run full test suite during image build
docker build --build-arg RUN_TESTS=true -t java-springboot-app:test .

# Skip tests (e.g. if already run in CI)
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
│   │   │   ├── JavaSpringbootApplication.java    # Bootstraps Spring Boot app
│   │   │   ├── config/
│   │   │   │   ├── ServiceProperties.java        # app.* config + validation
│   │   │   │   ├── ServerConfiguration.java      # Tomcat host/port/timeouts
│   │   │   │   ├── MetricsConfiguration.java     # Micrometer + Prometheus
│   │   │   │   └── ReadinessStateHolder.java     # Tracks ReadinessState
│   │   │   └── web/
│   │   │       ├── InfraController.java          # /, /info, /health, /ready
│   │   │       ├── MetricsController.java        # /metrics endpoint
│   │   │       └── HttpLoggingFilter.java        # Logging + metrics per HTTP
│   │   └── resources/
│   │       ├── application.yaml                  # Core config & management
│   │       └── logback-spring.xml                # JSON logging config
│   └── test/
│       ├── .../JavaSpringbootApplicationTests.java
│       ├── .../config/ServicePropertiesTest.java
│       ├── .../config/InvalidConfigStartupTest.java
│       ├── .../web/HttpLoggingFilterTest.java
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
    - JSON structured logging
    - graceful shutdown & Tomcat tuning
    - Docker best practices (multi-stage build, non-root, health check)
- In the full `docker-polyglot-lab` project, there are equivalent services in:
    - Go + Gin (`golang-gin-app`)
    - Python + Django (`python-django-app`)

You can use this Spring Boot service as a template for new Java microservices with production-friendly defaults.
