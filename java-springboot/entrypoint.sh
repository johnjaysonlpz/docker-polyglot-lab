#!/usr/bin/env sh
set -eu

is_true() {
  case "${1:-}" in
    1|true|TRUE|yes|YES|on|ON) return 0 ;;
    *) return 1 ;;
  esac
}

JAVA_TOOL_OPTIONS="${JAVA_TOOL_OPTIONS_BASE:-}"

if is_true "${OTEL_JAVAAGENT_ENABLED:-true}"; then
  if [ -r /otel/opentelemetry-javaagent.jar ]; then
    JAVA_TOOL_OPTIONS="${JAVA_TOOL_OPTIONS} -javaagent:/otel/opentelemetry-javaagent.jar"
  else
    echo "Entrypoint: OTEL_JAVAAGENT_ENABLED=true but /otel/opentelemetry-javaagent.jar not found; starting without agent" >&2
  fi
fi

export JAVA_TOOL_OPTIONS

exec "$@"
