# ==============================
# Stage 1: Build the application
# ==============================
FROM maven:3.9-eclipse-temurin-21-alpine AS builder

WORKDIR /app

# Build-time argument:
# - RUN_TESTS: whether to run the full test suite during the image build
ARG RUN_TESTS=true

# Copy pom.xml first to leverage Docker layer caching for dependencies
COPY pom.xml ./
RUN mvn -B -q dependency:go-offline

# Copy the rest of the source code
COPY src ./src

# Run tests and build the fat jar (or skip tests if requested)
RUN if [ "$RUN_TESTS" = "true" ]; then \
      mvn -B clean package; \
    else \
      mvn -B -DskipTests clean package; \
    fi

# ==============================
# Stage 2: Runtime image
# ==============================
FROM eclipse-temurin:21-jre-alpine

# Install only what the app needs at runtime (HTTP healthcheck + TLS)
RUN apk add --no-cache curl ca-certificates

# Build-time arguments:
# - SERVICE_NAME, VERSION, BUILD_TIME: exposed to the app via env vars
ARG SERVICE_NAME=java-springboot-app
ARG VERSION=dev
ARG BUILD_TIME=local
ARG OCI_IMAGE_SOURCE=""
ARG OCI_IMAGE_REVISION=""

# OCI image labels
LABEL org.opencontainers.image.title="${SERVICE_NAME}" \
      org.opencontainers.image.description="Polyglot lab - Java + Spring Boot HTTP service" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.created="${BUILD_TIME}" \
      org.opencontainers.image.url="https://github.com/johnjaysonlpz/docker-polyglot-lab/tree/main/java-springboot" \
      org.opencontainers.image.source="${OCI_IMAGE_SOURCE}" \
      org.opencontainers.image.revision="${OCI_IMAGE_REVISION}" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.authors="John Jayson Lopez"

# Default runtime configuration (overridable via env vars),
# including build metadata consumed by the app.
ENV JAVA_OPTS="" \
    PORT=8080 \
    SERVICE_NAME=${SERVICE_NAME} \
    VERSION=${VERSION} \
    BUILD_TIME=${BUILD_TIME}

# Create a dedicated non-root user & group with fixed IDs
RUN addgroup -g 10001 appgroup \
    && adduser -D -u 10001 -G appgroup appuser

WORKDIR /app

# Path to the built jar inside the builder image
ARG JAR_FILE=target/java-springboot-1.0.0.jar

# Copy the packaged jar from the builder image
COPY --chown=appuser:appgroup --from=builder /app/${JAR_FILE} ./app.jar

# Run as non-root for better security
USER appuser

# Document the container port (actual mapping is done at runtime)
EXPOSE 8080

# Container-level liveness check: hit the HTTP /health endpoint
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
  CMD curl -fsS http://127.0.0.1:${PORT}/health || exit 1

# Start the Spring Boot application
ENTRYPOINT ["sh", "-c", "exec java $JAVA_OPTS -jar /app/app.jar"]
