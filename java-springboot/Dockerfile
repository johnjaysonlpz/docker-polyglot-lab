# ==============================
# Stage 1: Build the application
# ==============================
FROM maven:3.9-eclipse-temurin-21-alpine AS builder

WORKDIR /app

# Build-time arguments (CI/Compose contract):
# - RUN_TESTS: optional unit/integration tests during image build
ARG RUN_TESTS=true

# Dependency caching:
# Copy pom.xml first so dependency download stays cached unless deps change.
COPY pom.xml ./

# BuildKit cache mount:
# Persists Maven repo between builds on the same runner/machine.
RUN --mount=type=cache,target=/root/.m2 \
    mvn -B -q dependency:go-offline

# Copy the source code after dependencies are cached.
COPY src ./src

# Build the jar (optionally running tests).
RUN --mount=type=cache,target=/root/.m2 \
    if [ "$RUN_TESTS" = "true" ]; then \
      mvn -B clean package; \
    else \
      mvn -B -DskipTests clean package; \
    fi

# Produce a stable artifact name for the runtime stage (/app/app.jar).
# Spring Boot builds can leave extra jars like:
# - *.jar.original (pre-repackage)
# - *-sources.jar / *-javadoc.jar
# We pick the runnable jar and copy it to a stable path used by runtime COPY.
RUN set -eu; \
    JAR="$(ls -1 target/*.jar \
      | grep -Ev '(original|sources|javadoc|tests)' \
      | head -n 1)"; \
    test -n "$JAR"; \
    echo "Selected jar: $JAR"; \
    cp "$JAR" /app/app.jar


# ==============================
# Stage 2: Runtime image
# ==============================
FROM eclipse-temurin:21-jre-alpine

# Runtime packages:
# - curl: used by container HEALTHCHECK
# - ca-certificates: enables outbound TLS trust store
RUN apk add --no-cache curl ca-certificates

# Build-time args re-declared in runtime stage (used for labels + ENV defaults).
ARG SERVICE_NAME=java-springboot-app
ARG VERSION=dev
ARG BUILD_TIME=local
ARG OCI_IMAGE_SOURCE=""
ARG OCI_IMAGE_REVISION=""

# OCI labels (traceability):
LABEL org.opencontainers.image.title="${SERVICE_NAME}" \
      org.opencontainers.image.description="Polyglot lab - Java + Spring Boot HTTP service" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.created="${BUILD_TIME}" \
      org.opencontainers.image.url="https://github.com/johnjaysonlpz/docker-polyglot-lab/tree/main/java-springboot" \
      org.opencontainers.image.source="${OCI_IMAGE_SOURCE}" \
      org.opencontainers.image.revision="${OCI_IMAGE_REVISION}" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.authors="John Jayson Lopez"

# Runtime env defaults (overridable via Compose):
# - JAVA_TOOL_OPTIONS is respected by the JVM automatically (no wrapper script).
# - Container-friendly memory tuning + fail-fast on OOM.
# - PORT + build metadata are exposed for /info + metrics and runtime debugging parity.
ENV JAVA_TOOL_OPTIONS="-XX:MaxRAMPercentage=75.0 -XX:InitialRAMPercentage=50.0 -XX:+ExitOnOutOfMemoryError" \
    PORT=8080 \
    SERVICE_NAME=${SERVICE_NAME} \
    VERSION=${VERSION} \
    BUILD_TIME=${BUILD_TIME}

# Non-root runtime user (security posture):
RUN addgroup -g 10001 appgroup \
    && adduser -D -u 10001 -G appgroup appuser

WORKDIR /app

# Copy the packaged jar from builder using the stable name.
# --chown avoids permission issues when running as non-root.
COPY --chown=appuser:appgroup --from=builder /app/app.jar ./app.jar

USER appuser

EXPOSE 8080

# Container liveness:
# Use 127.0.0.1 so healthcheck doesn't depend on DNS/networking.
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
  CMD curl -fsS http://127.0.0.1:${PORT}/health || exit 1

# Entrypoint (exec form):
# No shell; SIGTERM is delivered directly to the JVM process.
ENTRYPOINT ["java", "-jar", "/app/app.jar"]
