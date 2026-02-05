#!/usr/bin/env bash
# ------------------------------------------------------------------------------
# .ci-local.sh - Local parity runner for .github/workflows/cicd.yaml
# ------------------------------------------------------------------------------

set -Eeuo pipefail
IFS=$'\n\t'

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

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly ROOT_DIR
debug "ROOT_DIR=${ROOT_DIR}"

# ------------------------------------------------------------------------------
# Helpers
# ------------------------------------------------------------------------------

need_cmd() { command -v "$1" >/dev/null 2>&1 || die "missing required command: $1" 127; }

require_env() {
  local name="$1"
  [[ -n "${!name:-}" ]] || die "required env var is empty/unset: ${name}" 2
}

ensure_file() {
  local path="$1"
  [[ -f "$path" ]] || die "missing required file: $path" 2
}

ensure_dir() {
  local path="$1"
  [[ -d "$path" ]] || mkdir -p "$path"
}

# ------------------------------------------------------------------------------
# Summary (lazy-init so help/doctor don't create files)
# ------------------------------------------------------------------------------

SUMMARY_ENABLED=0
SUMMARY_DIR="${ROOT_DIR}/.cache/ci-local"
SUMMARY_FILE="${SUMMARY_DIR}/summary.tsv"

CURRENT_MODULE=""

record_summary() {
  [[ "$SUMMARY_ENABLED" -eq 1 ]] || return 0
  printf "%s\t%s\t%s\n" "$1" "$2" "$3" >> "$SUMMARY_FILE"
}

print_summary() {
  local rc=$?
  trap - EXIT

  if [[ "$SUMMARY_ENABLED" -eq 1 && -f "$SUMMARY_FILE" ]]; then
    declare -A status coverage security

    while IFS=$'\t' read -r mod key val; do
      case "$key" in
        status)   status["$mod"]="$val" ;;
        coverage) coverage["$mod"]="$val" ;;
        security) security["$mod"]="$val" ;;
      esac
    done < "$SUMMARY_FILE"

    printf "\n\033[1m==> Summary\033[0m\n"
    printf "%-8s %-8s %-10s %-22s\n" "Module" "Result" "Coverage" "Security"
    printf "%-8s %-8s %-10s %-22s\n" "------" "------" "--------" "--------"

    for mod in go java python; do
      printf "%-8s %-8s %-10s %-22s\n" \
        "$mod" \
        "${status[$mod]:-SKIPPED}" \
        "${coverage[$mod]:--}" \
        "${security[$mod]:--}"
    done
  fi

  exit "$rc"
}

init_summary() {
  ensure_dir "$SUMMARY_DIR"
  : > "$SUMMARY_FILE"
  SUMMARY_ENABLED=1
  trap 'print_summary' EXIT
}

# ------------------------------------------------------------------------------
# Error trap
# ------------------------------------------------------------------------------

on_err() {
  local exit_code="$1"
  local line_no="$2"
  local cmd="${3:-unknown}"
  local func="${4:-main}"

  if [[ -n "${CURRENT_MODULE:-}" ]]; then
    record_summary "$CURRENT_MODULE" status FAIL
  fi

  printf "ERROR: %s failed (exit=%s) at %s:%s in %s: %s\n" \
    "${BASH_SOURCE[0]}" "$exit_code" "${BASH_SOURCE[0]}" "$line_no" "$func" "$cmd" >&2
  exit "$exit_code"
}
trap 'on_err "$?" "$LINENO" "${BASH_COMMAND:-}" "${FUNCNAME[0]:-main}"' ERR

# ------------------------------------------------------------------------------
# Net helpers
# ------------------------------------------------------------------------------

curl_supports_retry_all_errors() {
  { curl --help all 2>/dev/null || curl --help 2>/dev/null; } | grep -q -- '--retry-all-errors'
}

curl_fetch() {
  local url="$1"
  local out="$2"

  need_cmd curl

  local -a args=(
    -fsSL
    --retry 5
    --retry-delay 1
    --connect-timeout 10
    --max-time 300
  )
  if curl_supports_retry_all_errors; then
    args+=(--retry-all-errors)
  fi

  curl "${args[@]}" -o "$out" "$url"
}

validate_ci_event_name() {
  local v="${CI_EVENT_NAME:-push}"
  case "$v" in
    push|pull_request) ;;
    *) die "CI_EVENT_NAME must be 'push' or 'pull_request' (got: $v)" 2 ;;
  esac
}

go_bin_dir() {
  local gobin
  gobin="$(go env GOBIN)"
  if [[ -n "$gobin" ]]; then
    printf "%s\n" "$gobin"
    return 0
  fi

  local gopath
  gopath="$(go env GOPATH)"
  gopath="${gopath%%:*}"
  printf "%s\n" "${gopath}/bin"
}

# ------------------------------------------------------------------------------
# Tool versions
# ------------------------------------------------------------------------------

VERSIONS_FILE="${ROOT_DIR}/.ci-tool-versions.sh"
ensure_file "$VERSIONS_FILE"

# shellcheck disable=SC1090
source "$VERSIONS_FILE"

# ------------------------------------------------------------------------------
# CI parity controls
# ------------------------------------------------------------------------------

CI_EVENT_NAME="${CI_EVENT_NAME:-push}"
export CI_EVENT_NAME
validate_ci_event_name
debug "CI_EVENT_NAME=${CI_EVENT_NAME}"

# ------------------------------------------------------------------------------
# Global prereqs
# ------------------------------------------------------------------------------

need_cmd git
need_cmd python3.12

# ------------------------------------------------------------------------------
# Doctor mode
# ------------------------------------------------------------------------------

doctor_need_cmd() {
  local cmd="$1"
  if ! command -v "$cmd" >/dev/null 2>&1; then
    warn "doctor: missing command: $cmd"
    return 1
  fi
  return 0
}

doctor_require_env() {
  local name="$1"
  if [[ -z "${!name:-}" ]]; then
    warn "doctor: required env var empty/unset: $name (check .ci-tool-versions.sh)"
    return 1
  fi
  return 0
}

doctor_require_file() {
  local path="$1"
  if [[ ! -f "$path" ]]; then
    warn "doctor: missing required file: $path"
    return 1
  fi
  return 0
}

doctor_require_dir() {
  local path="$1"
  if [[ ! -d "$path" ]]; then
    warn "doctor: missing required directory: $path"
    return 1
  fi
  return 0
}

doctor_scan_non_ascii() {
  python3.12 - "$@" <<'PY'
import sys, pathlib

bad = False
for p in sys.argv[1:]:
  path = pathlib.Path(p)
  if not path.exists():
    continue
  try:
    data = path.read_bytes()
  except Exception as e:
    print(f"doctor: could not read {p}: {e}", file=sys.stderr)
    bad = True
    continue

  try:
    text = data.decode("utf-8")
  except UnicodeDecodeError as e:
    print(f"doctor: {p}: not valid UTF-8 ({e})", file=sys.stderr)
    bad = True
    continue

  for i, ch in enumerate(text):
    if ord(ch) > 127:
      prefix = text[:i]
      line = prefix.count("\n") + 1
      col = i - prefix.rfind("\n")
      code = f"U+{ord(ch):04X}"
      snippet = ch
      print(f"doctor: non-ASCII in {p}:{line}:{col} ({code} '{snippet}')", file=sys.stderr)
      bad = True
      break

sys.exit(1 if bad else 0)
PY
}

doctor_check_go() {
  local errs=0

  doctor_require_dir "${ROOT_DIR}/golang-gin" || errs=$((errs+1))
  doctor_require_file "${ROOT_DIR}/golang-gin/go.mod" || errs=$((errs+1))
  doctor_require_file "${ROOT_DIR}/golang-gin/go.sum" || errs=$((errs+1))
  doctor_require_file "${ROOT_DIR}/golang-gin/.golangci.yaml" || errs=$((errs+1))

  doctor_require_env GOIMPORTS_VERSION || errs=$((errs+1))
  doctor_require_env GOVULNCHECK_VERSION || errs=$((errs+1))
  doctor_require_env GOLANGCI_LINT_VERSION || errs=$((errs+1))
  doctor_require_env GO_MODULE || errs=$((errs+1))

  doctor_need_cmd go || errs=$((errs+1))
  doctor_need_cmd curl || errs=$((errs+1))
  doctor_need_cmd tar || errs=$((errs+1))
  doctor_need_cmd sha256sum || errs=$((errs+1))
  doctor_need_cmd install || errs=$((errs+1))
  doctor_need_cmd gcc || errs=$((errs+1))

  doctor_scan_non_ascii \
    "${ROOT_DIR}/golang-gin/go.mod" \
    "${ROOT_DIR}/golang-gin/go.sum" \
    "${ROOT_DIR}/golang-gin/.golangci.yaml" \
    >/dev/null || errs=$((errs+1))

  return "$errs"
}

doctor_check_java() {
  local errs=0

  doctor_require_dir "${ROOT_DIR}/java-springboot" || errs=$((errs+1))
  doctor_require_file "${ROOT_DIR}/java-springboot/pom.xml" || errs=$((errs+1))

  doctor_require_env DEPENDENCY_CHECK_MAVEN_VERSION || errs=$((errs+1))
  doctor_require_env DEPENDENCY_CHECK_FAIL_CVSS || errs=$((errs+1))

  doctor_need_cmd mvn || errs=$((errs+1))
  doctor_need_cmd java || errs=$((errs+1))

  doctor_scan_non_ascii \
    "${ROOT_DIR}/java-springboot/pom.xml" \
    >/dev/null || errs=$((errs+1))

  return "$errs"
}

doctor_check_python() {
  local errs=0

  doctor_require_dir "${ROOT_DIR}/python-django" || errs=$((errs+1))
  doctor_require_file "${ROOT_DIR}/python-django/requirements.txt" || errs=$((errs+1))
  doctor_require_file "${ROOT_DIR}/python-django/requirements.test.txt" || errs=$((errs+1))
  doctor_require_file "${ROOT_DIR}/python-django/requirements.lock" || errs=$((errs+1))
  doctor_require_dir "${ROOT_DIR}/python-django/app" || errs=$((errs+1))

  doctor_require_env PIP_MAX_VERSION || errs=$((errs+1))
  doctor_require_env PIP_TOOLS_VERSION || errs=$((errs+1))
  doctor_require_env PIP_AUDIT_VERSION || errs=$((errs+1))

  doctor_need_cmd python3.12 || errs=$((errs+1))

  doctor_scan_non_ascii \
    "${ROOT_DIR}/python-django/requirements.txt" \
    "${ROOT_DIR}/python-django/requirements.test.txt" \
    "${ROOT_DIR}/python-django/requirements.lock" \
    >/dev/null || errs=$((errs+1))

  return "$errs"
}

doctor_print_mini_summary() {
  # expects associative arrays: mod_status, mod_issues
  printf "\n\033[1m==> Doctor Summary\033[0m\n"
  printf "%-8s %-8s %-6s\n" "Module" "Result" "Issues"
  printf "%-8s %-8s %-6s\n" "------" "------" "------"
  for mod in go java python; do
    printf "%-8s %-8s %-6s\n" \
      "$mod" \
      "${mod_status[$mod]:-SKIPPED}" \
      "${mod_issues[$mod]:--}"
  done
}

doctor() {
  local scope="all"
  local doctor_summary=0
  local errs=0

  for arg in "$@"; do
    case "$arg" in
      "" ) ;;
      all|go|java|python) scope="$arg" ;;
      --summary|-s) doctor_summary=1 ;;
      *)
        die "Unknown doctor arg: $arg (use: doctor [all|go|java|python] [--summary])" 2
        ;;
    esac
  done

  say "Doctor - preflight checks (${scope})"

  if [[ -n "$(git status --porcelain 2>/dev/null || true)" ]]; then
    warn "doctor: git working tree is not clean (CI runs on a clean checkout)."
  fi

  local workflow_file="${ROOT_DIR}/.github/workflows/cicd.yaml"
  doctor_require_file "$VERSIONS_FILE" || errs=$((errs+1))
  if [[ -f "$workflow_file" ]]; then
    : # OK
  else
    warn "doctor: workflow not found at ${workflow_file} (skip scan, but parity checks expect it there)."
  fi

  doctor_scan_non_ascii \
    "${BASH_SOURCE[0]}" \
    "$VERSIONS_FILE" \
    >/dev/null || errs=$((errs+1))

  if [[ -f "$workflow_file" ]]; then
    if ! doctor_scan_non_ascii "$workflow_file" >/dev/null 2>/dev/null; then
      warn "doctor: non-ASCII found in workflow YAML (usually OK for GitHub Actions)."
    fi
  fi

  declare -A mod_status mod_issues
  mod_status[go]="SKIPPED";    mod_issues[go]="-"
  mod_status[java]="SKIPPED";  mod_issues[java]="-"
  mod_status[python]="SKIPPED";mod_issues[python]="-"

  if [[ "$scope" == "all" || "$scope" == "go" ]]; then
    doctor_check_go; local rc=$?
    mod_issues[go]="$rc"
    mod_status[go]="$([[ "$rc" -eq 0 ]] && echo OK || echo FAIL)"
    errs=$((errs+rc))
  fi

  if [[ "$scope" == "all" || "$scope" == "java" ]]; then
    doctor_check_java; local rc=$?
    mod_issues[java]="$rc"
    mod_status[java]="$([[ "$rc" -eq 0 ]] && echo OK || echo FAIL)"
    errs=$((errs+rc))
  fi

  if [[ "$scope" == "all" || "$scope" == "python" ]]; then
    doctor_check_python; local rc=$?
    mod_issues[python]="$rc"
    mod_status[python]="$([[ "$rc" -eq 0 ]] && echo OK || echo FAIL)"
    errs=$((errs+rc))
  fi

  if [[ "$doctor_summary" -eq 1 ]]; then
    doctor_print_mini_summary
  fi

  if [[ "$errs" -ne 0 ]]; then
    die "Doctor failed with ${errs} issue(s). Fix warnings above before running CI steps." 2
  fi

  say "Doctor OK"
}

# ------------------------------------------------------------------------------
# Go / Gin
# ------------------------------------------------------------------------------

install_golangci_lint_cached() {
  require_env GOLANGCI_LINT_VERSION

  need_cmd tar
  need_cmd sha256sum
  need_cmd install
  need_cmd uname

  local os arch ver_no_v tarball checksums base_url
  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  arch="$(uname -m)"
  case "$arch" in
    x86_64|amd64) arch="amd64" ;;
    aarch64|arm64) arch="arm64" ;;
    *) die "unsupported arch for golangci-lint: $arch" 2 ;;
  esac

  ver_no_v="${GOLANGCI_LINT_VERSION#v}"
  tarball="golangci-lint-${ver_no_v}-${os}-${arch}.tar.gz"
  checksums="golangci-lint-${ver_no_v}-checksums.txt"
  base_url="https://github.com/golangci/golangci-lint/releases/download/${GOLANGCI_LINT_VERSION}"

  local cache_root cache_dir bin_path
  cache_root="${ROOT_DIR}/.cache/ci-tools/golangci-lint"
  cache_dir="${cache_root}/${GOLANGCI_LINT_VERSION}/${os}-${arch}"
  bin_path="${cache_dir}/golangci-lint"
  ensure_dir "$cache_dir"

  if [[ -x "$bin_path" ]]; then
    printf "%s\n" "$bin_path"
    return 0
  fi

  local tmpdir
  tmpdir="$(mktemp -d)"

  (
    trap 'rm -rf "$tmpdir"' EXIT

    curl_fetch "${base_url}/${tarball}" "${tmpdir}/${tarball}"
    curl_fetch "${base_url}/${checksums}" "${tmpdir}/${checksums}"

    grep -F " ${tarball}" "${tmpdir}/${checksums}" >/dev/null || {
      echo "ERROR: checksum entry not found for ${tarball}" >&2
      exit 2
    }

    (
      cd "$tmpdir" || exit 2
      grep " ${tarball}\$" "$checksums" | sha256sum -c - >/dev/null
    )

    tar -C "$tmpdir" -xzf "${tmpdir}/${tarball}"

    install -m 0755 \
      "${tmpdir}/golangci-lint-${ver_no_v}-${os}-${arch}/golangci-lint" \
      "$bin_path"
  ) || return $?

  printf "%s\n" "$bin_path"
}

run_go() {
  CURRENT_MODULE="go"
  record_summary go status RUNNING
  record_summary go security "govulncheck=$( [[ "$CI_EVENT_NAME" == "push" ]] && echo on || echo off )"

  say "Go / Gin - quality, tests, security"

  require_env GOIMPORTS_VERSION
  require_env GOVULNCHECK_VERSION
  require_env GOLANGCI_LINT_VERSION
  require_env GO_MODULE

  need_cmd go

  (
    cd "$ROOT_DIR/golang-gin"

    say "Validate - go.mod/go.sum are tidy"
    go mod tidy
    git diff --exit-code -- go.mod go.sum

    say "Setup - install Go dev tools (goimports, govulncheck)"
    go install "golang.org/x/tools/cmd/goimports@${GOIMPORTS_VERSION}"
    go install "golang.org/x/vuln/cmd/govulncheck@${GOVULNCHECK_VERSION}"

    local gobin
    gobin="$(go_bin_dir)"
    ensure_dir "$gobin"
    export PATH="$gobin:$PATH"

    need_cmd goimports
    need_cmd govulncheck

    say "Quality - format/import checks (gofmt, goimports)"
    local go_dirs
    go_dirs="$(go list -f '{{.Dir}}' ./...)"

    if [[ -n "$go_dirs" ]]; then
      while IFS= read -r d; do
        gofmt -l "$d"
      done <<<"$go_dirs" | grep -q . && exit 1

      while IFS= read -r d; do
        goimports -l -local "${GO_MODULE}" "$d"
      done <<<"$go_dirs" | grep -q . && exit 1
    fi

    say "Quality - lint (golangci-lint)"
    local lint_bin
    lint_bin="$(install_golangci_lint_cached | tail -n 1)"
    install -m 0755 "$lint_bin" "${gobin}/golangci-lint"

    golangci-lint config verify --config=.golangci.yaml
    golangci-lint run --config=.golangci.yaml ./...

    say "Quality - static analysis (go vet)"
    go vet ./...

    say "Setup - system deps for race detector (CGO) [local prereq: gcc]"
    need_cmd gcc

    say "Test - unit tests (race, shuffle) + coverage profile"
    CGO_ENABLED=1 go test ./... -race -shuffle=on -count=1 \
      -covermode=atomic -coverprofile=coverage.out

    say "Coverage - enforce 100% statements (Go)"
    python3.12 - <<'PY'
import re, subprocess, sys
out = subprocess.check_output(["go", "tool", "cover", "-func=coverage.out"], text=True)
m = re.search(r"total:\s*\(statements\)\s*([\d.]+)%", out)
if not m:
    print("Could not parse total coverage from go tool cover output.", file=sys.stderr)
    sys.exit(2)
pct = float(m.group(1))
print(f"Go total coverage: {pct:.1f}%")
sys.exit(0 if pct >= 100.0 else 1)
PY

    local go_cov
    go_cov="$(go tool cover -func=coverage.out | awk '/^total:/{print $NF; exit}')"
    [[ -n "$go_cov" ]] || die "could not parse Go coverage percent" 2
    record_summary go coverage "$go_cov"

    say "Artifact - upload coverage profile (coverage.out) [local check]"
    test -f coverage.out

    if [[ "$CI_EVENT_NAME" == "push" ]]; then
      say "Security - vulnerability scan (govulncheck) (push/tags only)"
      set -o pipefail
      govulncheck -test ./... | tee govulncheck.txt

      say "Artifact - upload govulncheck report (push/tags only) [local check]"
      [[ -f govulncheck.txt ]] || warn "expected govulncheck.txt but it was not created."
    else
      say "Security - vulnerability scan (govulncheck) (push/tags only) [skipped: CI_EVENT_NAME=$CI_EVENT_NAME]"
    fi

    say "Go / Gin - OK"
  )

  record_summary go status PASS
  CURRENT_MODULE=""
}

# ------------------------------------------------------------------------------
# Java / Spring Boot
# ------------------------------------------------------------------------------

run_java() {
  CURRENT_MODULE="java"
  record_summary java status RUNNING
  record_summary java security "dependency-check=$( [[ "$CI_EVENT_NAME" == "push" ]] && echo on || echo off )"

  say "Java / Spring Boot - Spotless, SpotBugs, tests, JaCoCo, security"

  require_env DEPENDENCY_CHECK_MAVEN_VERSION
  require_env DEPENDENCY_CHECK_FAIL_CVSS

  need_cmd mvn

  (
    cd "$ROOT_DIR/java-springboot"

    export MAVEN_OPTS="${MAVEN_OPTS:---sun-misc-unsafe-memory-access=allow}"

    say "Build - verify (Spotless, SpotBugs, tests, JaCoCo)"
    mvn -B -ntp verify

    say "Artifact - upload JaCoCo report directory [local check]"
    [[ -d target/site/jacoco ]] || die "target/site/jacoco/ not found (JaCoCo expected from mvn verify)." 2
    [[ -f target/site/jacoco/jacoco.xml ]] || die "target/site/jacoco/jacoco.xml not found (needed for summary coverage)." 2

    local java_cov
    java_cov="$(python3.12 - <<'PY'
import xml.etree.ElementTree as ET
p = "target/site/jacoco/jacoco.xml"
root = ET.parse(p).getroot()
missed = covered = 0
for c in root.iter("counter"):
    if c.attrib.get("type") == "LINE":
        missed += int(c.attrib.get("missed", "0"))
        covered += int(c.attrib.get("covered", "0"))
total = missed + covered
pct = (covered / total * 100.0) if total else 0.0
print(f"{pct:.1f}%")
PY
)"
    [[ -n "$java_cov" ]] || die "could not parse JaCoCo coverage percent" 2
    record_summary java coverage "$java_cov"

    if [[ "$CI_EVENT_NAME" == "push" ]]; then
      say "Cache - OWASP Dependency-Check data (push/tags only) [local cache dir]"
      local odc_data_dir
      odc_data_dir="${ROOT_DIR}/.cache/dependency-check"
      ensure_dir "$odc_data_dir"

      say "Security - OWASP Dependency-Check scan (push/tags only)"
      mvn -B -ntp "org.owasp:dependency-check-maven:${DEPENDENCY_CHECK_MAVEN_VERSION}:check" \
        -DfailBuildOnCVSS="${DEPENDENCY_CHECK_FAIL_CVSS}" \
        -Dformats=HTML,JSON \
        -DdataDirectory="${odc_data_dir}" \
        -DnvdApiKeyEnvironmentVariable=NVD_API_KEY

      say "Artifact - upload Dependency-Check reports (push/tags only) [local check]"
      [[ -f target/dependency-check-report.html ]] || warn "target/dependency-check-report.html not found."
      [[ -f target/dependency-check-report.json ]] || warn "target/dependency-check-report.json not found."
    else
      say "Security - OWASP Dependency-Check scan (push/tags only) [skipped: CI_EVENT_NAME=$CI_EVENT_NAME]"
    fi

    say "Java / Spring Boot - OK"
  )

  record_summary java status PASS
  CURRENT_MODULE=""
}

# ------------------------------------------------------------------------------
# Python / Django
# ------------------------------------------------------------------------------

run_python() {
  CURRENT_MODULE="python"
  record_summary python status RUNNING
  record_summary python security "pip-audit=$( [[ "$CI_EVENT_NAME" == "push" ]] && echo on || echo off )"

  say "Python / Django - quality, tests, coverage, security"

  require_env PIP_MAX_VERSION
  require_env PIP_TOOLS_VERSION
  require_env PIP_AUDIT_VERSION

  need_cmd python3.12

  (
    cd "$ROOT_DIR/python-django"

    local venv_dir=".venv-ci"
    local cleanup_venv="${CI_LOCAL_CLEAN_VENV:-1}"

    say "Setup - create isolated venv for CI tools (pip-tools)"
    rm -rf "$venv_dir"
    python3.12 -m venv "$venv_dir"

    # shellcheck disable=SC1091
    . "${venv_dir}/bin/activate"

    python_cleanup() {
      deactivate >/dev/null 2>&1 || true
      [[ "$cleanup_venv" == "1" ]] && rm -rf "$venv_dir" || true
    }
    trap python_cleanup EXIT

    python -m pip install -U "pip<${PIP_MAX_VERSION}" "pip-tools==${PIP_TOOLS_VERSION}"
    need_cmd pip-compile

    say "Validate - requirements.lock matches requirements.txt"
    pip-compile --no-strip-extras --generate-hashes --output-file=requirements.lock requirements.txt
    git diff --exit-code -- requirements.lock

    say "Install - runtime + test dependencies (locked)"
    python -m pip install --upgrade pip
    python -m pip install --require-hashes -r requirements.lock
    python -m pip install -r requirements.test.txt

    need_cmd ruff
    need_cmd mypy
    need_cmd pytest

    say "Validate - installed packages are consistent (pip check, compileall)"
    python -m pip check
    python -m compileall -q app

    say "Quality - format check (ruff format)"
    ruff format --check .

    say "Quality - lint (ruff)"
    ruff check .

    if [[ "$CI_EVENT_NAME" == "pull_request" ]]; then
      say "Quality - typecheck (mypy) (PR only, non-blocking)"
      set +e
      mypy app
      local mypy_rc=$?
      set -e
      if [[ $mypy_rc -ne 0 ]]; then
        echo "NOTE: mypy failed but is non-blocking for pull_request (matches CI)."
      fi
    else
      say "Quality - typecheck (mypy) (push/tags only, blocking)"
      mypy app
    fi

    say "Test - unit tests + coverage (pytest, enforce 100%, write XML)"
    DJANGO_SETTINGS_MODULE=django_app.settings \
      pytest --cov --cov-report=xml --cov-fail-under=100

    say "Artifact - upload coverage report (coverage.xml) [local check]"
    test -f coverage.xml

    local py_cov
    py_cov="$(python3.12 - <<'PY'
import xml.etree.ElementTree as ET
root = ET.parse("coverage.xml").getroot()
rate = float(root.attrib.get("line-rate", "0"))
print(f"{rate*100:.1f}%")
PY
)"
    [[ -n "$py_cov" ]] || die "could not parse Python coverage percent" 2
    record_summary python coverage "$py_cov"

    if [[ "$CI_EVENT_NAME" == "push" ]]; then
      say "Security - vulnerability scan (pip-audit) (push/tags only)"
      python -m pip install "pip-audit==${PIP_AUDIT_VERSION}"
      pip-audit -r requirements.lock --format=json --output=pip-audit.json
      pip-audit -r requirements.lock

      say "Artifact - upload pip-audit report (push/tags only) [local check]"
      [[ -f pip-audit.json ]] || warn "pip-audit.json not found."
    else
      say "Security - vulnerability scan (pip-audit) (push/tags only) [skipped: CI_EVENT_NAME=$CI_EVENT_NAME]"
    fi

    say "Python / Django - OK"
  )

  record_summary python status PASS
  CURRENT_MODULE=""
}

# ------------------------------------------------------------------------------
# CLI flags
# ------------------------------------------------------------------------------

usage() {
  cat <<'TXT'
Usage:
  ./.ci-local.sh [COMMAND] [--help]

Commands:
  all        Run Go + Java + Python checks (default).
  go         Run Go checks only.
  java       Run Java checks only.
  python     Run Python checks only.
  doctor     Preflight checks (doctor [all|go|java|python] [--summary]).

Options:
  --help, -h   Show this help.

Environment:
  LOG_LEVEL=quiet|info|debug      (default: info)
  CI_EVENT_NAME=push|pull_request (default: push)
  CI_LOCAL_CLEAN_VENV=1|0         (default: 1; python only)
  NVD_API_KEY                     (needed for OWASP Dependency-Check when CI_EVENT_NAME=push)

Examples:
  ./.ci-local.sh
  ./.ci-local.sh go
  ./.ci-local.sh doctor all
  ./.ci-local.sh doctor --summary
  ./.ci-local.sh doctor java --summary
  CI_EVENT_NAME=pull_request ./.ci-local.sh
  LOG_LEVEL=debug ./.ci-local.sh java
TXT
}

main() {
  local target="${1:-all}"
  shift || true

  case "$target" in
    all)
      init_summary
      run_go; run_java; run_python
      ;;
    go)
      init_summary
      run_go
      ;;
    java)
      init_summary
      run_java
      ;;
    python)
      init_summary
      run_python
      ;;
    doctor)
      doctor "$@"
      ;;
    -h|--help|help)
      usage
      ;;
    *)
      echo "Unknown target: $target" >&2
      usage
      exit 2
      ;;
  esac
}

main "$@"
