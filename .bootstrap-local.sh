#!/usr/bin/env bash
# ------------------------------------------------------------------------------
# .bootstrap-local.sh - one-time local setup (secrets, permissions, provisioning)
# ------------------------------------------------------------------------------

set -Eeuo pipefail
IFS=$'\n\t'
umask 077

# ------------------------------------------------------------------------------
# Logging (LOG_LEVEL: quiet|info|debug) - default: info
# ------------------------------------------------------------------------------

LOG_LEVEL="${LOG_LEVEL:-info}"

say() {
  [[ "$LOG_LEVEL" == "quiet" ]] && return 0
  printf "\n\033[1m==> %s\033[0m\n" "$*"
}
debug() {
  [[ "$LOG_LEVEL" == "debug" ]] || return 0
  printf "DEBUG: %s\n" "$*" >&2
}
warn() { printf "WARN: %s\n" "$*" >&2; }
die()  { printf "ERROR: %s\n" "$*" >&2; exit "${2:-1}"; }

# ------------------------------------------------------------------------------
# Repo root detection
# ------------------------------------------------------------------------------

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
debug "SCRIPT_DIR=${SCRIPT_DIR}"

if [[ -d "${SCRIPT_DIR}/docker" ]]; then
  ROOT_DIR="${SCRIPT_DIR}"
elif [[ -d "${SCRIPT_DIR}/../docker" ]]; then
  ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
else
  die "could not locate repo root (expected docker/ next to this script or one level up). Current: ${SCRIPT_DIR}" 2
fi

readonly ROOT_DIR
cd "$ROOT_DIR"
debug "ROOT_DIR=${ROOT_DIR}"

# ------------------------------------------------------------------------------
# Helpers
# ------------------------------------------------------------------------------

need_cmd() { command -v "$1" >/dev/null 2>&1 || die "missing required command: $1" 127; }

ensure_file() {
  local path="$1"
  [[ -f "$path" ]] || die "missing required file: $path" 2
}

DRY_RUN=0

run() {
  if [[ "$DRY_RUN" -eq 1 ]]; then
    printf "DRY-RUN: "
    printf "%q " "$@"
    printf "\n"
    return 0
  fi
  "$@"
}

ensure_dir() {
  local path="$1"
  if [[ "$DRY_RUN" -eq 1 ]]; then
    [[ -d "$path" ]] || printf "DRY-RUN: mkdir -p %q\n" "$path"
    return 0
  fi
  [[ -d "$path" ]] || mkdir -p "$path"
}

root_run() {
  if [[ "$DRY_RUN" -eq 1 ]]; then
    if [[ "$(id -u)" -eq 0 ]]; then
      run "$@"
    else
      run sudo "$@"
    fi
    return 0
  fi

  if [[ "$(id -u)" -eq 0 ]]; then
    "$@"
  else
    need_cmd sudo
    sudo "$@"
  fi
}

# ------------------------------------------------------------------------------
# Error trap
# ------------------------------------------------------------------------------

on_err() {
  local exit_code="$1"
  local line_no="$2"
  local cmd="${3:-unknown}"
  printf "ERROR: %s failed (exit=%s) at line %s: %s\n" \
    "${BASH_SOURCE[0]}" "$exit_code" "$line_no" "$cmd" >&2
  exit "$exit_code"
}
trap 'on_err "$?" "$LINENO" "${BASH_COMMAND:-}"' ERR

# ------------------------------------------------------------------------------
# CLI flags
# ------------------------------------------------------------------------------

usage() {
  cat <<'TXT'
bootstrap-local: one-time local setup

USAGE
  ./.bootstrap-local.sh [--dry-run] [--help]

ENV
  LOG_LEVEL=quiet|info|debug  (default: info)

  Required:
    GRAFANA_ADMIN_USER
    GRAFANA_ADMIN_PASSWORD
    TELEGRAM_BOT_TOKEN
    TELEGRAM_CHAT_ID

  Optional:
    GRAFANA_UID=<uid>         (default: 472)
    GRAFANA_GID=<gid>         (default: 0)
    ALERTMANAGER_UID=<uid>    (default: 65534)
    ALERTMANAGER_GID=<gid>    (default: 65534)

CHANGES
  - docker/secrets permissions + safety checks (no symlinks, not git-tracked)
  - Grafana + Alertmanager secret files (0400, service UID/GID)
  - Prometheus entrypoint executable bit
  - Grafana provisioning directories + perms

EXAMPLES
  ./.bootstrap-local.sh
  ./.bootstrap-local.sh --dry-run
  LOG_LEVEL=debug ./.bootstrap-local.sh --dry-run
TXT
}

while [[ "${1:-}" != "" ]]; do
  case "$1" in
    --help|-h) usage; exit 0 ;;
    --dry-run) DRY_RUN=1; shift ;;
    *) die "unknown argument: $1 (use --help)" 2 ;;
  esac
done

# ------------------------------------------------------------------------------
# Prereqs
# ------------------------------------------------------------------------------

need_cmd id
need_cmd mkdir
need_cmd chmod
need_cmd chown
need_cmd find
need_cmd install
need_cmd cat
if [[ "$(id -u)" -ne 0 ]]; then
  need_cmd sudo
fi

# ------------------------------------------------------------------------------
# Env requirements
# ------------------------------------------------------------------------------

require_env() {
  local name="$1"
  if [[ -z "${!name:-}" ]]; then
    warn "missing required env var: ${name}"
    return 1
  fi
  return 0
}

missing=0
require_env GRAFANA_ADMIN_USER     || missing=1
require_env GRAFANA_ADMIN_PASSWORD || missing=1
require_env TELEGRAM_BOT_TOKEN     || missing=1
require_env TELEGRAM_CHAT_ID       || missing=1

if [[ "$missing" -ne 0 ]]; then
  say "Set the missing env vars and re-run."
  printf "Examples (export then run):\n"
  printf "  export GRAFANA_ADMIN_USER=admin\n"
  printf "  export GRAFANA_ADMIN_PASSWORD=secret\n"
  printf "  export TELEGRAM_BOT_TOKEN=...\n"
  printf "  export TELEGRAM_CHAT_ID=...\n"
  printf "  ./.bootstrap-local.sh\n"
  die "bootstrap aborted: missing required env vars" 2
fi

SECRETS_DIR="docker/secrets"
GRAFANA_PROV_DIR="docker/grafana/provisioning"
PROM_ENTRYPOINT="docker/prometheus/entrypoint.sh"

SECRET_FILES=(
  "${SECRETS_DIR}/grafana_admin_user"
  "${SECRETS_DIR}/grafana_admin_password"
  "${SECRETS_DIR}/telegram_bot_token"
  "${SECRETS_DIR}/telegram_chat_id"
)

GRAFANA_UID="${GRAFANA_UID:-472}"
GRAFANA_GID="${GRAFANA_GID:-0}"
ALERTMANAGER_UID="${ALERTMANAGER_UID:-65534}"
ALERTMANAGER_GID="${ALERTMANAGER_GID:-65534}"

# ------------------------------------------------------------------------------
# Secrets
# ------------------------------------------------------------------------------

say "Secrets - create dir"
root_run mkdir -p "$SECRETS_DIR"

root_run chown root:root "$SECRETS_DIR"
root_run chmod 0711 "$SECRETS_DIR"

say "Secrets - guards (no symlinks, not tracked by git)"
if [[ -L "$SECRETS_DIR" ]]; then
  die "refusing: ${SECRETS_DIR} is a symlink" 2
fi

for f in "${SECRET_FILES[@]}"; do
  if [[ -L "$f" ]]; then
    die "refusing to write secret to symlink: $f" 2
  fi
done

if command -v git >/dev/null 2>&1 && git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  mapfile -t tracked < <(git ls-files -- "$SECRETS_DIR" 2>/dev/null || true)
  if ((${#tracked[@]} > 0)); then
    printf "ERROR: refusing: secrets appear tracked by git under %s:\n" "$SECRETS_DIR" >&2
    printf "  %s\n" "${tracked[@]}" >&2
    die "untrack and gitignore secrets before running bootstrap" 2
  fi
fi

say "Secrets - precreate files (0400, owned by service users)"
if [[ "$DRY_RUN" -eq 1 ]]; then
  printf "DRY-RUN: install -o %s -g %s -m 0400 /dev/null %s\n" "$GRAFANA_UID" "$GRAFANA_GID" "${SECRET_FILES[0]}"
  printf "DRY-RUN: install -o %s -g %s -m 0400 /dev/null %s\n" "$GRAFANA_UID" "$GRAFANA_GID" "${SECRET_FILES[1]}"
  printf "DRY-RUN: install -o %s -g %s -m 0400 /dev/null %s\n" "$ALERTMANAGER_UID" "$ALERTMANAGER_GID" "${SECRET_FILES[2]}"
  printf "DRY-RUN: install -o %s -g %s -m 0400 /dev/null %s\n" "$ALERTMANAGER_UID" "$ALERTMANAGER_GID" "${SECRET_FILES[3]}"
else
  root_run install -o "$GRAFANA_UID" -g "$GRAFANA_GID" -m 0400 /dev/null "${SECRET_FILES[0]}"
  root_run install -o "$GRAFANA_UID" -g "$GRAFANA_GID" -m 0400 /dev/null "${SECRET_FILES[1]}"
  root_run install -o "$ALERTMANAGER_UID" -g "$ALERTMANAGER_GID" -m 0400 /dev/null "${SECRET_FILES[2]}"
  root_run install -o "$ALERTMANAGER_UID" -g "$ALERTMANAGER_GID" -m 0400 /dev/null "${SECRET_FILES[3]}"
fi

write_secret() {
  local file="$1"
  local val="$2"

  if [[ "$DRY_RUN" -eq 1 ]]; then
    printf "DRY-RUN: write %s\n" "$file"
    return 0
  fi

  printf '%s' "$val" | root_run bash -c '
    set -Eeuo pipefail
    f="$1"
    [[ -f "$f" && ! -L "$f" ]] || {
      echo "ERROR: refusing to write secret (not a precreated regular file): $f" >&2
      exit 2
    }
    cat > "$f"
  ' _ "$file"
}

say "Secrets - write files"
write_secret "${SECRET_FILES[0]}" "$GRAFANA_ADMIN_USER"
write_secret "${SECRET_FILES[1]}" "$GRAFANA_ADMIN_PASSWORD"
write_secret "${SECRET_FILES[2]}" "$TELEGRAM_BOT_TOKEN"
write_secret "${SECRET_FILES[3]}" "$TELEGRAM_CHAT_ID"

say "Secrets - re-enforce owner/perms (idempotent)"
if [[ "$DRY_RUN" -eq 1 ]]; then
  printf "DRY-RUN: chown %s:%s %s %s\n" "$GRAFANA_UID" "$GRAFANA_GID" "${SECRET_FILES[0]}" "${SECRET_FILES[1]}"
  printf "DRY-RUN: chmod 0400 %s %s\n" "${SECRET_FILES[0]}" "${SECRET_FILES[1]}"
  printf "DRY-RUN: chown %s:%s %s %s\n" "$ALERTMANAGER_UID" "$ALERTMANAGER_GID" "${SECRET_FILES[2]}" "${SECRET_FILES[3]}"
  printf "DRY-RUN: chmod 0400 %s %s\n" "${SECRET_FILES[2]}" "${SECRET_FILES[3]}"
else
  root_run chown "${GRAFANA_UID}:${GRAFANA_GID}" "${SECRET_FILES[0]}" "${SECRET_FILES[1]}"
  root_run chmod 0400 "${SECRET_FILES[0]}" "${SECRET_FILES[1]}"
  root_run chown "${ALERTMANAGER_UID}:${ALERTMANAGER_GID}" "${SECRET_FILES[2]}" "${SECRET_FILES[3]}"
  root_run chmod 0400 "${SECRET_FILES[2]}" "${SECRET_FILES[3]}"
fi

# ------------------------------------------------------------------------------
# Prometheus
# ------------------------------------------------------------------------------

say "Prometheus - entrypoint permissions"
ensure_file "$PROM_ENTRYPOINT"
run chmod +x "$PROM_ENTRYPOINT"

# ------------------------------------------------------------------------------
# Grafana provisioning
# ------------------------------------------------------------------------------

say "Grafana - provisioning dirs"
ensure_dir "$GRAFANA_PROV_DIR/alerting"
ensure_dir "$GRAFANA_PROV_DIR/plugins"

say "Grafana - provisioning perms"
run chmod 755 "$GRAFANA_PROV_DIR"
run find "$GRAFANA_PROV_DIR" -type d -exec chmod 755 {} +
run find "$GRAFANA_PROV_DIR" -type f -exec chmod 644 {} +

say "Done."

# ------------------------------------------------------------------------------
# Optional note
# ------------------------------------------------------------------------------

if [[ -z "${NVD_API_KEY:-}" ]]; then
  say "Note: If running OWASP Dependency-Check locally, export NVD_API_KEY in your shell:"
  printf "  export NVD_API_KEY='...'\n"
else
  debug "NVD_API_KEY is set (not printing value)."
fi
