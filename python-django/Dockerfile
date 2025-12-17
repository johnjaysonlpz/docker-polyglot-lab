# ==============================
# Stage 1: Build the application
# ==============================
FROM python:3.12-alpine AS builder

WORKDIR /app

# Build-time argument:
# - RUN_TESTS: whether to run the Django test suite during the image build
ARG RUN_TESTS=true

# System deps for building Python wheels (if needed by requirements.txt)
RUN apk add --no-cache gcc musl-dev libffi-dev

# Copy requirements.txt first to leverage Docker layer caching for dependencies
COPY requirements.txt ./
RUN pip install --upgrade pip && pip install -r requirements.txt

# Copy the rest of the application source code
COPY app ./app

# Set Django settings module for test execution
ENV DJANGO_SETTINGS_MODULE=django_app.settings

WORKDIR /app/app

# Optionally run the Django test suite as part of the image build
RUN if [ "$RUN_TESTS" = "true" ]; then \
      python manage.py test; \
    fi

# ==============================
# Stage 2: Runtime image
# ==============================
FROM python:3.12-alpine

# Install only what the app needs at runtime (HTTP healthcheck + TLS)
RUN apk add --no-cache curl ca-certificates

WORKDIR /app

# Copy requirements and install into a dedicated virtualenv
COPY requirements.txt ./
RUN python -m venv /venv \
    && /venv/bin/pip install --upgrade pip \
    && /venv/bin/pip install -r requirements.txt

# Copy the Django application source from the builder image
COPY --from=builder /app/app ./app

# Build-time arguments:
# - SERVICE_NAME, VERSION, BUILD_TIME: exposed to the app via env vars
ARG SERVICE_NAME=python-django-app
ARG VERSION=dev
ARG BUILD_TIME=local
ARG VCS_REF=local

# OCI image labels
LABEL org.opencontainers.image.title="${SERVICE_NAME}" \
      org.opencontainers.image.description="Polyglot lab - Python + Django HTTP service" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.created="${BUILD_TIME}" \
      org.opencontainers.image.url="https://github.com/johnjaysonlpz/docker-polyglot-lab/tree/main/python-django" \
      org.opencontainers.image.source="https://github.com/johnjaysonlpz/docker-polyglot-lab" \
      org.opencontainers.image.revision="${VCS_REF}" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.authors="John Jayson Lopez"

# App-level metadata + timeouts (used by Django/settings).
# Gunicorn process-level timeouts are configured separately via GUNICORN_* envs.
ENV PATH="/venv/bin:$PATH" \
    DJANGO_SETTINGS_MODULE=django_app.settings \
    PORT=8080 \
    HOST=0.0.0.0 \
    APP_SERVICE_NAME=${SERVICE_NAME} \
    APP_VERSION=${VERSION} \
    APP_BUILD_TIME=${BUILD_TIME} \
    READ_TIMEOUT=2s \
    IDLE_TIMEOUT=30s \
    SHUTDOWN_TIMEOUT=5s \
    GUNICORN_WORKERS=2 \
    GUNICORN_TIMEOUT=30 \
    GUNICORN_KEEPALIVE=5

# Create a dedicated non-root user & group with fixed IDs
RUN addgroup -g 10001 appgroup \
    && adduser -D -u 10001 -G appgroup appuser

# Run as non-root for better security
USER appuser

# Document the container port (actual mapping is done at runtime)
EXPOSE 8080

# Container-level liveness check: hit the HTTP /health endpoint
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
  CMD curl -fsS http://127.0.0.1:${PORT}/health || exit 1

WORKDIR /app/app

# Gunicorn process configuration:
# - GUNICORN_WORKERS: number of worker processes
# - GUNICORN_TIMEOUT: max seconds before a worker is considered stuck and killed
# - GUNICORN_KEEPALIVE: seconds to keep idle HTTP connections open
# These can be overridden at runtime via env vars.
ENTRYPOINT ["sh", "-c", "exec gunicorn django_app.wsgi:application \
    --bind 0.0.0.0:${PORT} \
    --workers ${GUNICORN_WORKERS} \
    --timeout ${GUNICORN_TIMEOUT} \
    --keep-alive ${GUNICORN_KEEPALIVE} \
"]
