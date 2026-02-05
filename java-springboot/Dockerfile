# syntax=docker/dockerfile:1.7

ARG MAVEN_VERSION=3.9.12
ARG TEMURIN_VERSION=25

# ==============================
# Stage 1: Build the application
# ==============================

FROM maven:${MAVEN_VERSION}-eclipse-temurin-${TEMURIN_VERSION}-noble AS builder
WORKDIR /app

ARG RUN_TESTS=true

COPY pom.xml ./

RUN --mount=type=cache,target=/root/.m2,sharing=locked \
    mvn -B -ntp -q dependency:go-offline

COPY . .

RUN --mount=type=cache,target=/root/.m2,sharing=locked \
    set -eu; \
    if [ -x "./mvnw" ]; then MVN="./mvnw"; else MVN="mvn"; fi; \
    if [ "${RUN_TESTS}" = "true" ]; then \
      ${MVN} -B -ntp clean package; \
    else \
      ${MVN} -B -ntp -DskipTests clean package; \
    fi

RUN set -eu; \
    JAR="/app/target/app.jar"; \
    test -f "${JAR}"; \
    mkdir -p /app/extracted; \
    java -Djarmode=layertools -jar "${JAR}" extract --destination /app/extracted

# ==============================
# Stage 2: Runtime image
# ==============================

FROM eclipse-temurin:${TEMURIN_VERSION}-jre-noble
WORKDIR /app

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*

ARG SERVICE_NAME=java-springboot-app
ARG VERSION=dev
ARG BUILD_TIME=local
ARG OCI_IMAGE_SOURCE=""
ARG OCI_IMAGE_REVISION=""

ARG OTEL_JAVA_AGENT_VERSION=2.24.0
ARG OTEL_JAVA_AGENT_SHA256=5c48cd48f9907f824214e22f14a28375c92613a94b0cf98381997e0a678220ce

RUN set -eu; \
    apt-get update; \
    apt-get install -y --no-install-recommends curl; \
    rm -rf /var/lib/apt/lists/*; \
    mkdir -p /otel; \
    curl -fsSL -o /otel/opentelemetry-javaagent.jar \
      "https://repo1.maven.org/maven2/io/opentelemetry/javaagent/opentelemetry-javaagent/${OTEL_JAVA_AGENT_VERSION}/opentelemetry-javaagent-${OTEL_JAVA_AGENT_VERSION}.jar"; \
    echo "${OTEL_JAVA_AGENT_SHA256}  /otel/opentelemetry-javaagent.jar" | sha256sum -c -; \
    chmod 0444 /otel/opentelemetry-javaagent.jar; \
    apt-get purge -y curl; \
    apt-get autoremove -y; \
    rm -rf /var/lib/apt/lists/*

LABEL org.opencontainers.image.title="${SERVICE_NAME}" \
      org.opencontainers.image.description="Polyglot lab - Java + Spring Boot HTTP service" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.created="${BUILD_TIME}" \
      org.opencontainers.image.url="https://github.com/johnjaysonlpz/docker-polyglot-lab/tree/main/java-springboot" \
      org.opencontainers.image.source="${OCI_IMAGE_SOURCE}" \
      org.opencontainers.image.revision="${OCI_IMAGE_REVISION}" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.authors="John Jayson Lopez" \
      org.opencontainers.image.base.name="eclipse-temurin:${TEMURIN_VERSION}-jre-noble"

ENV PORT=8080 \
    SERVICE_NAME=${SERVICE_NAME} \
    VERSION=${VERSION} \
    BUILD_TIME=${BUILD_TIME} \
    OTEL_JAVAAGENT_ENABLED=true \
    OTEL_LOGS_EXPORTER=none \
    OTEL_METRICS_EXPORTER=none \
    OTEL_TRACES_EXPORTER=true

ENV JAVA_TOOL_OPTIONS_BASE="-XX:MaxRAMPercentage=75.0 -XX:InitialRAMPercentage=50.0 -XX:+ExitOnOutOfMemoryError"

RUN groupadd -g 10001 appgroup \
    && useradd -u 10001 -g 10001 -m -s /usr/sbin/nologin appuser

COPY --chown=appuser:appgroup --from=builder /app/extracted/dependencies/ ./
COPY --chown=appuser:appgroup --from=builder /app/extracted/spring-boot-loader/ ./
COPY --chown=appuser:appgroup --from=builder /app/extracted/snapshot-dependencies/ ./
COPY --chown=appuser:appgroup --from=builder /app/extracted/application/ ./

COPY --chown=appuser:appgroup entrypoint.sh /app/entrypoint.sh
RUN chmod 0555 /app/entrypoint.sh

EXPOSE 8080
USER appuser
STOPSIGNAL SIGTERM

ENTRYPOINT ["/app/entrypoint.sh"]
CMD ["java", "org.springframework.boot.loader.launch.JarLauncher"]
