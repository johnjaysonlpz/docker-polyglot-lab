# ==============================
# Stage 1: Build the application
# ==============================
FROM python:3.12-alpine AS builder

WORKDIR /app

# Build-time arguments (CI/Compose contract):
# - RUN_TESTS: optional Django tests during image build
ARG RUN_TESTS=true

# Build dependencies (builder-only):
# Alpine uses musl; some packages may need compilation.
# Keep compilers out of runtime by installing them only in this builder stage.
RUN apk add --no-cache gcc musl-dev libffi-dev

# Dependency caching:
# Copy requirements first so wheel build stays cached unless deps change.
COPY requirements.txt ./

# Build wheels for all dependencies (future-proof for native deps on Alpine).
# Produces /wheels/*.whl which the runtime stage can install without compiling.
RUN python -m pip install --upgrade pip \
    && pip wheel --no-cache-dir --wheel-dir /wheels -r requirements.txt

# Create a virtualenv and install deps FROM wheels (mirrors runtime behavior),
# then run tests against the exact wheel set used for production.
RUN python -m venv /venv \
    && /venv/bin/pip install --upgrade pip \
    && /venv/bin/pip install --no-index --find-links=/wheels -r requirements.txt

# Copy app source (Django project).
COPY app ./app

# Optional tests (defense-in-depth gate).
ENV DJANGO_SETTINGS_MODULE=django_app.settings
WORKDIR /app/app
RUN if [ "$RUN_TESTS" = "true" ]; then /venv/bin/python manage.py test; fi


# ==============================
# Stage 2: Runtime image
# ==============================
FROM python:3.12-alpine

# Runtime packages:
# - curl: used by container HEALTHCHECK
# - ca-certificates: enables outbound TLS trust store
#
# NOTE:
# If you later add deps that require runtime shared libs, add them here
# (e.g., libffi, libpq, etc.). Keep runtime minimal by default.
RUN apk add --no-cache curl ca-certificates

WORKDIR /app

# Install runtime dependencies into an isolated venv from prebuilt wheels.
# IMPORTANT:
# - BuildKit mount accesses /wheels from the builder stage without copying wheels
#   into the final image layers (keeps runtime image smaller/cleaner).
COPY requirements.txt ./
RUN --mount=type=bind,from=builder,source=/wheels,target=/wheels \
    python -m venv /venv \
    && /venv/bin/pip install --upgrade pip \
    && /venv/bin/pip install --no-index --find-links=/wheels -r requirements.txt

# Copy app source from builder stage.
COPY --from=builder /app/app ./app

# Build-time args re-declared in runtime stage (used for labels + ENV defaults).
ARG SERVICE_NAME=python-django-app
ARG VERSION=dev
ARG BUILD_TIME=local
ARG OCI_IMAGE_SOURCE=""
ARG OCI_IMAGE_REVISION=""

# OCI labels (traceability):
LABEL org.opencontainers.image.title="${SERVICE_NAME}" \
      org.opencontainers.image.description="Polyglot lab - Python + Django HTTP service" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.created="${BUILD_TIME}" \
      org.opencontainers.image.url="https://github.com/johnjaysonlpz/docker-polyglot-lab/tree/main/python-django" \
      org.opencontainers.image.source="${OCI_IMAGE_SOURCE}" \
      org.opencontainers.image.revision="${OCI_IMAGE_REVISION}" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.authors="John Jayson Lopez"

# Runtime env defaults (overridable via Compose):
# - PATH points to venv so "python"/"gunicorn" resolve from /venv.
# - PYTHONUNBUFFERED=1 ensures logs flush immediately (important for containers).
# - PORT + build metadata are exposed for /info + metrics and runtime debugging parity.
# - Remaining knobs are consumed by the app and/or entrypoint.py (Gunicorn launcher).
ENV PATH="/venv/bin:$PATH" \
    PYTHONUNBUFFERED=1 \
    DJANGO_SETTINGS_MODULE=django_app.settings \
    HOST=0.0.0.0 \
    PORT=8080 \
    SERVICE_NAME=${SERVICE_NAME} \
    VERSION=${VERSION} \
    BUILD_TIME=${BUILD_TIME} \
    LOG_LEVEL=INFO \
    READ_TIMEOUT=2s \
    IDLE_TIMEOUT=30s \
    SHUTDOWN_TIMEOUT=5s \
    TRUSTED_PROXIES="" \
    GUNICORN_WORKERS=2 \
    GUNICORN_WORKER_CLASS=gthread \
    GUNICORN_THREADS=4 \
    GUNICORN_TIMEOUT=30 \
    GUNICORN_KEEPALIVE=5 \
    GUNICORN_GRACEFUL_TIMEOUT=5

# Non-root runtime user (security posture):
RUN addgroup -g 10001 appgroup \
    && adduser -D -u 10001 -G appgroup appuser

USER appuser

EXPOSE 8080

# Container liveness:
# Use 127.0.0.1 so healthcheck doesn't depend on DNS/networking.
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
  CMD curl -fsS http://127.0.0.1:${PORT}/health || exit 1

# Entrypoint:
# Exec form avoids shell; entrypoint.py reads env and execs Gunicorn.
WORKDIR /app/app
ENTRYPOINT ["python", "-u", "entrypoint.py"]
