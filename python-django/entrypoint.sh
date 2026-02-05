#!/usr/bin/env sh
set -eu

is_true() {
  case "${1:-}" in
    1|true|TRUE|yes|YES|on|ON) return 0 ;;
    *) return 1 ;;
  esac
}

: "${PORT:=8080}"
: "${DJANGO_WSGI_MODULE:=django_app.wsgi:application}"
: "${GUNICORN_CONFIG:=/app/gunicorn.conf.py}"

OTEL_PYTHON_ENABLED="${OTEL_PYTHON_ENABLED:-true}"

if is_true "${DJANGO_MIGRATE_ON_STARTUP:-false}"; then
  python manage.py migrate --noinput
fi

if is_true "${DJANGO_COLLECTSTATIC_ON_STARTUP:-false}"; then
  python manage.py collectstatic --noinput
fi

workers="${GUNICORN_WORKERS:-1}"
worker_class="${GUNICORN_WORKER_CLASS:-gthread}"
threads="${GUNICORN_THREADS:-4}"
timeout="${GUNICORN_TIMEOUT:-30}"
keepalive="${GUNICORN_KEEPALIVE:-5}"
graceful_timeout="${GUNICORN_GRACEFUL_TIMEOUT:-5}"

set -- gunicorn "${DJANGO_WSGI_MODULE}" \
  --config "${GUNICORN_CONFIG}" \
  --bind "0.0.0.0:${PORT}" \
  --workers "${workers}" \
  --worker-class "${worker_class}" \
  --threads "${threads}" \
  --timeout "${timeout}" \
  --keep-alive "${keepalive}" \
  --graceful-timeout "${graceful_timeout}"

if is_true "${OTEL_PYTHON_ENABLED}"; then
  if command -v opentelemetry-instrument >/dev/null 2>&1; then
    set -- opentelemetry-instrument "$@"
  else
    echo "Entrypoint: OTEL_PYTHON_ENABLED=true but opentelemetry-instrument not found; starting without instrumentation" >&2
  fi
fi

exec "$@"
