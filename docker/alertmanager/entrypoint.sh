#!/bin/sh
set -eu

entrypoint_log() {
  case "${ENTRYPOINT_LOG_LEVEL:-}" in
    debug|DEBUG) printf '%s\n' "$*" >&2 ;;
    *) : ;;
  esac
}

TEMPLATE="/etc/alertmanager/alertmanager.yaml.tpl"
OUT="/tmp/alertmanager.yaml"

: "${APP_ENV:=integration}"

SECRETS_DIR="${SECRETS_DIR:-/run/secrets}"
TOKEN_FILE="${TOKEN_FILE:-${SECRETS_DIR}/telegram_bot_token}"
CHAT_ID_FILE="${CHAT_ID_FILE:-${SECRETS_DIR}/telegram_chat_id}"

tmpdir="$(mktemp -d)"
cleanup() { rm -rf "$tmpdir"; }
trap cleanup EXIT

routes_block="${tmpdir}/routes.yaml"
receivers_block="${tmpdir}/receivers.yaml"
sed_script="${tmpdir}/render.sed"

enable_telegram="0"
CHAT_ID=""

if [ -r "$TOKEN_FILE" ] && [ -r "$CHAT_ID_FILE" ] && [ -s "$TOKEN_FILE" ] && [ -s "$CHAT_ID_FILE" ]; then
  CHAT_ID="$(tr -d '\r\n' <"$CHAT_ID_FILE")"
  case "$CHAT_ID" in
    -[0-9]*|[0-9]*) enable_telegram="1" ;;
    *)
      echo "Entrypoint: invalid telegram chat_id with APP_ENV=${APP_ENV} (must be int): $CHAT_ID" >&2
      exit 1
      ;;
  esac
fi

if [ "$enable_telegram" = "1" ]; then
  cat >"$routes_block" <<'EOF'
  routes:
    - receiver: telegram_critical
      matchers:
        - severity="critical"
      repeat_interval: 15m
EOF

  cat >"$receivers_block" <<EOF
  - name: telegram_critical
    telegram_configs:
      - bot_token_file: ${TOKEN_FILE}
        chat_id: ${CHAT_ID}
        send_resolved: true
        parse_mode: ""
        message: '{{ template "telegram.polyglot.message" . }}'
EOF

  cat >"$sed_script" <<EOF
/__ROUTES__/{
r $routes_block
d
}
/__RECEIVERS__/{
r $receivers_block
d
}
EOF

  sed -f "$sed_script" "$TEMPLATE" >"$OUT"
  entrypoint_log "Entrypoint: integration to telegram enabled with APP_ENV=${APP_ENV}"
else
  sed \
    -e 's/__ROUTES__/routes: []/g' \
    -e '/__RECEIVERS__/d' \
    "$TEMPLATE" >"$OUT"
  entrypoint_log "Entrypoint: integration to telegram disabled (missing/empty secrets)"
fi

exec /bin/alertmanager "$@"
