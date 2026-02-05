# syntax=docker/dockerfile:1.7

ARG PYTHON_VERSION=3.12.12
ARG DEBIAN_SUITE=bookworm

# ==============================
# Stage 1: Build the application
# ==============================
FROM python:${PYTHON_VERSION}-slim-${DEBIAN_SUITE} AS builder
WORKDIR /app

ARG APT_BUILD_DEPS=""
ARG REQUIREMENTS_FILE=requirements.lock

ENV PIP_DISABLE_PIP_VERSION_CHECK=1 \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1

RUN set -eu; \
    if [ -n "${APT_BUILD_DEPS}" ]; then \
      apt-get update; \
      apt-get install -y --no-install-recommends ${APT_BUILD_DEPS}; \
      rm -rf /var/lib/apt/lists/*; \
    fi; \
    python -m venv /venv; \
    /venv/bin/pip install -U pip setuptools wheel

COPY requirements.txt requirements.lock* ./

RUN --mount=type=cache,target=/root/.cache/pip,sharing=locked \
    set -eu; \
    echo "Using ${REQUIREMENTS_FILE}"; \
    test -f "${REQUIREMENTS_FILE}"; \
    /venv/bin/pip wheel --wheel-dir=/wheels -r "${REQUIREMENTS_FILE}"; \
    /venv/bin/pip install --no-index --find-links=/wheels -r "${REQUIREMENTS_FILE}"; \
    rm -rf /wheels

COPY app ./app


# ==============================
# Stage 2: Test (separate venv)
# ==============================
FROM python:${PYTHON_VERSION}-slim-${DEBIAN_SUITE} AS test
WORKDIR /app

ARG RUN_TESTS=true
ARG REQUIREMENTS_FILE=requirements.lock

ENV PIP_DISABLE_PIP_VERSION_CHECK=1 \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    DJANGO_SETTINGS_MODULE=django_app.settings

RUN set -eu; \
    python -m venv /venv-test; \
    /venv-test/bin/pip install -U pip setuptools wheel

COPY requirements.txt requirements.lock* ./
RUN --mount=type=cache,target=/root/.cache/pip,sharing=locked \
    set -eu; \
    echo "Using ${REQUIREMENTS_FILE} (test venv)"; \
    test -f "${REQUIREMENTS_FILE}"; \
    /venv-test/bin/pip wheel --wheel-dir=/wheels-test -r "${REQUIREMENTS_FILE}"; \
    /venv-test/bin/pip install --no-index --find-links=/wheels-test -r "${REQUIREMENTS_FILE}"; \
    rm -rf /wheels-test

COPY requirements.test.txt ./
RUN --mount=type=cache,target=/root/.cache/pip,sharing=locked \
    set -eu; \
    /venv-test/bin/pip install -r requirements.test.txt

COPY app ./app
COPY pytest.ini ./pytest.ini
COPY tests ./tests

WORKDIR /app/app
RUN set -eu; \
    if [ "${RUN_TESTS}" = "true" ]; then \
      /venv-test/bin/pytest; \
    fi


# ==============================
# Stage 3: Runtime image
# ==============================
FROM python:${PYTHON_VERSION}-slim-${DEBIAN_SUITE} AS runtime
WORKDIR /app

ARG SERVICE_NAME=python-django-app
ARG VERSION=dev
ARG BUILD_TIME=local
ARG OCI_IMAGE_SOURCE=""
ARG OCI_IMAGE_REVISION=""

LABEL org.opencontainers.image.title="${SERVICE_NAME}" \
      org.opencontainers.image.description="Polyglot lab - Python + Django HTTP service" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.created="${BUILD_TIME}" \
      org.opencontainers.image.url="https://github.com/johnjaysonlpz/docker-polyglot-lab/tree/main/python-django" \
      org.opencontainers.image.source="${OCI_IMAGE_SOURCE}" \
      org.opencontainers.image.revision="${OCI_IMAGE_REVISION}" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.authors="John Jayson Lopez" \
      org.opencontainers.image.base.name="python:${PYTHON_VERSION}-slim-${DEBIAN_SUITE}"

ENV PIP_DISABLE_PIP_VERSION_CHECK=1 \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    DJANGO_SETTINGS_MODULE=django_app.settings \
    DJANGO_WSGI_MODULE=django_app.wsgi:application \
    PORT=8080 \
    SERVICE_NAME=${SERVICE_NAME} \
    VERSION=${VERSION} \
    BUILD_TIME=${BUILD_TIME} \
    PATH="/venv/bin:${PATH}"

RUN set -eu; \
    apt-get update; \
    apt-get install -y --no-install-recommends ca-certificates; \
    rm -rf /var/lib/apt/lists/*; \
    addgroup --gid 10001 appgroup; \
    adduser --disabled-password --gecos "" --uid 10001 --gid 10001 appuser

COPY --from=builder /venv /venv
COPY --chown=appuser:appgroup --from=builder /app/app /app/app

COPY --chown=appuser:appgroup gunicorn.conf.py /app/gunicorn.conf.py
COPY --chown=appuser:appgroup entrypoint.sh /app/entrypoint.sh
RUN chmod 0555 /app/entrypoint.sh

EXPOSE 8080
USER appuser
WORKDIR /app/app
STOPSIGNAL SIGTERM

ENTRYPOINT ["/app/entrypoint.sh"]
