#!/bin/sh
set -eu

entrypoint_log() {
  case "${ENTRYPOINT_LOG_LEVEL:-}" in
    debug|DEBUG) printf '%s\n' "$*" >&2 ;;
    *) : ;;
  esac
}

TEMPLATE="/etc/prometheus/prometheus.yml.tpl"
OUT="/tmp/prometheus.yml"

: "${APP_ENV:=integration}"

if command -v envsubst >/dev/null 2>&1; then
  envsubst < "$TEMPLATE" > "$OUT"
else
  sed "s/\${APP_ENV:-integration}/${APP_ENV}/g" "$TEMPLATE" > "$OUT"
fi

entrypoint_log "Entrypoint: rendered Prometheus config to ${OUT} with APP_ENV=${APP_ENV}"

exec /bin/prometheus "$@"
