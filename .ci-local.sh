#!/usr/bin/env bash
# ------------------------------------------------------------------------------
# .ci-local.sh
# Runs a fixed set of checks that mirror CI intent, without CI event emulation.
# ------------------------------------------------------------------------------

set -Eeuo pipefail
IFS=$'\n\t'

if ((BASH_VERSINFO[0] < 4)); then
  echo "ERROR: bash >= 4 is required (detected: ${BASH_VERSION})" >&2
  exit 2
fi

# ------------------------------------------------------------------------------
# Logging (LOG_LEVEL: quiet|info|debug) - default: info
# ------------------------------------------------------------------------------

LOG_LEVEL="${LOG_LEVEL:-info}"

say() {
  [[ "${LOG_LEVEL}" == "quiet" ]] && return 0
  printf "\n\033[1m==> %s\033[0m\n" "$*"
}
debug() {
  [[ "${LOG_LEVEL}" == "debug" ]] || return 0
  printf "DEBUG: %s\n" "$*" >&2
}
warn() { printf "WARN: %s\n" "$*" >&2; }
die()  { printf "ERROR: %s\n" "$*" >&2; exit "${2:-1}"; }

# ------------------------------------------------------------------------------
# Fixed defaults (no CI knobs)
# ------------------------------------------------------------------------------

TRIVY_SCANNERS_DEFAULT="vuln,misconfig,secret"
TRIVY_SEVERITY_DEFAULT="HIGH,CRITICAL"
TRIVY_TIMEOUT_DEFAULT="10m0s"
TRIVY_SCAN_RETRIES_DEFAULT=2

DOCKER_USE_BUILDX_DEFAULT=1
DOCKER_PULL_BASE_DEFAULT=1
DOCKER_BUILD_RETRIES_DEFAULT=2

PY_CLEAN_VENV_DEFAULT=1

# OWASP Dependency-Check defaults (fixed; only NVD_API_KEY is optional)
ODC_STRICT_DEFAULT=0
ODC_RETRIES_DEFAULT=2
ODC_PURGE_ON_FAIL_DEFAULT=1
ODC_NVD_API_DELAY_MS_DEFAULT=3500
ODC_NVD_MAX_RETRY_COUNT_DEFAULT=10
ODC_NVD_VALID_FOR_HOURS_DEFAULT=24

GO_COVERAGE_MIN="${GO_COVERAGE_MIN:-100}"
PY_COVERAGE_MIN="${PY_COVERAGE_MIN:-100}"
JAVA_COVERAGE_MIN_RATIO="${JAVA_COVERAGE_MIN_RATIO:-1.0}"

# ------------------------------------------------------------------------------
# Repo root + global paths
# ------------------------------------------------------------------------------

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly ROOT_DIR
debug "ROOT_DIR=${ROOT_DIR}"

SUMMARY_ENABLED=0
SUMMARY_DIR="${ROOT_DIR}/.cache/ci-local"
SUMMARY_FILE="${SUMMARY_DIR}/summary.tsv"

ARTIFACTS_DIR="${SUMMARY_DIR}/artifacts"
ARTIFACTS_GO_DIR="${ARTIFACTS_DIR}/go-gin"
ARTIFACTS_JAVA_DIR="${ARTIFACTS_DIR}/java-springboot"
ARTIFACTS_PY_DIR="${ARTIFACTS_DIR}/python-django"
ARTIFACTS_TRIVY_DIR="${ARTIFACTS_DIR}/trivy"

readonly SUMMARY_DIR SUMMARY_FILE
readonly ARTIFACTS_DIR ARTIFACTS_GO_DIR ARTIFACTS_JAVA_DIR ARTIFACTS_PY_DIR ARTIFACTS_TRIVY_DIR

CURRENT_MODULE=""

# ------------------------------------------------------------------------------
# Core helpers
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

copy_artifact() {
  local src="$1"
  local dest_dir="$2"
  local dest_name="${3:-}"

  [[ -f "$src" ]] || return 0
  ensure_dir "$dest_dir"
  if [[ -n "$dest_name" ]]; then
    cp -f "$src" "${dest_dir}/${dest_name}"
  else
    cp -f "$src" "$dest_dir/"
  fi
}

retry() {
  local attempts="$1"; shift
  local n=1 rc=0

  (( attempts >= 1 )) || die "retry: attempts must be >= 1 (got: $attempts)" 2

  local had_errexit=0
  [[ $- == *e* ]] && had_errexit=1

  local err_trap
  err_trap="$(trap -p ERR || true)"
  trap - ERR

  while (( n <= attempts )); do
    (( had_errexit )) && set +e
    "$@"
    rc=$?
    (( had_errexit )) && set -e

    if (( rc == 0 )); then
      eval "$err_trap"
      return 0
    fi
    if (( n == attempts )); then
      eval "$err_trap"
      return "$rc"
    fi

    warn "retry: attempt ${n}/${attempts} failed (rc=${rc})"

    local delay=$((n * 2))
    (( delay > 30 )) && delay=30
    sleep "$delay"
    n=$((n + 1))
  done

  eval "$err_trap"
  return "$rc"
}

# ------------------------------------------------------------------------------
# Summary + deterministic artifacts
# ------------------------------------------------------------------------------

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
    printf "%-10s %-8s %-20s %-31s\n" "Module" "Result" "Coverage" "Security"
    printf "%-10s %-8s %-20s %-31s\n" "------" "------" "--------------------" "-------------------------------"

    for mod in trivy_repo go java python docker; do
      printf "%-10s %-8s %-20s %-31s\n" \
        "$mod" \
        "${status[$mod]:-SKIPPED}" \
        "${coverage[$mod]:--}" \
        "${security[$mod]:--}"
    done

    printf "\nArtifacts:\n  %s\n" "$ARTIFACTS_DIR"
  fi

  exit "$rc"
}

init_summary() {
  ensure_dir "$SUMMARY_DIR"
  ensure_dir "$ARTIFACTS_DIR"
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
# Tool versions
# ------------------------------------------------------------------------------

VERSIONS_FILE="${ROOT_DIR}/.ci-tool-versions.sh"
ensure_file "$VERSIONS_FILE"
 
load_tool_versions() {
   local file="$1"
 
   local allowed_keys_re='^(TRIVY_VERSION|GOIMPORTS_VERSION|GOVULNCHECK_VERSION|GOLANGCI_LINT_VERSION|GO_MODULE|DEPENDENCY_CHECK_MAVEN_VERSION|DEPENDENCY_CHECK_FAIL_CVSS|PIP_AUDIT_VERSION|PIP_TOOLS_VERSION|PIP_MAX_VERSION)$'
   local base_value_re='[A-Za-z0-9][A-Za-z0-9._+/:@-]*'
 
   local unquoted_line_re="^([A-Z][A-Z0-9_]*)=(${base_value_re})$"
   local quoted_line_re="^([A-Z][A-Z0-9_]*)=\"(${base_value_re})\"$"
 
   local semverish_re='^v?[0-9]+(\.[0-9]+){0,3}([.-][0-9A-Za-z][0-9A-Za-z.-]*)?(\+[0-9A-Za-z][0-9A-Za-z.-]*)?$'
   local numeric_version_re='^[0-9]+(\.[0-9]+){0,3}$'
   local cvss_re='^(10(\.0)?|[0-9](\.[0-9])?)$'
   local go_module_re='^[A-Za-z0-9.-]+(/[A-Za-z0-9._-]+)+$'
 
   declare -A key_value_re=(
     ["TRIVY_VERSION"]="$semverish_re"
     ["GOIMPORTS_VERSION"]="$semverish_re"
     ["GOVULNCHECK_VERSION"]="$semverish_re"
     ["GOLANGCI_LINT_VERSION"]="$semverish_re"
     ["DEPENDENCY_CHECK_MAVEN_VERSION"]="$semverish_re"
     ["PIP_AUDIT_VERSION"]="$semverish_re"
     ["PIP_TOOLS_VERSION"]="$semverish_re"
     ["PIP_MAX_VERSION"]="$numeric_version_re"
     ["DEPENDENCY_CHECK_FAIL_CVSS"]="$cvss_re"
     ["GO_MODULE"]="$go_module_re"
   )
 
   _trim_ws() {
     local s="$1"
     s="${s#"${s%%[![:space:]]*}"}"
     s="${s%"${s##*[![:space:]]}"}"
     printf '%s' "$s"
   }
 
   while IFS= read -r raw || [[ -n "$raw" ]]; do
     local line key value expected_re
     line="${raw%$'\r'}"
     line="$(_trim_ws "$line")"
 
     [[ -z "$line" ]] && continue
     [[ "$line" =~ ^# ]] && continue
 
     key=""
     value=""
 
     if [[ "$line" =~ $unquoted_line_re ]]; then
       key="${BASH_REMATCH[1]}"
       value="${BASH_REMATCH[2]}"
     elif [[ "$line" =~ $quoted_line_re ]]; then
       key="${BASH_REMATCH[1]}"
       value="${BASH_REMATCH[2]}"
     else
       die "invalid line in ${file}: must be KEY=value or KEY=\"value\"; no spaces; KEY=[A-Z][A-Z0-9_]*; value charset=${base_value_re}" 2
     fi
 
     if [[ ! "$key" =~ $allowed_keys_re ]]; then
       die "disallowed key in ${file}: ${key} (allowed: ${allowed_keys_re})" 2
     fi
 
     expected_re="${key_value_re[$key]:-}"
     [[ -n "$expected_re" ]] || die "internal config missing per-key validator for: ${key}" 2
 
     if [[ ! "$value" =~ $expected_re ]]; then
       die "invalid value for ${key} in ${file} (expected: ${expected_re}; got: ${value})" 2
     fi
 
     printf -v "$key" '%s' "$value"
     export "$key"
   done < "$file"
 }
 
 load_tool_versions "$VERSIONS_FILE"

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

# ------------------------------------------------------------------------------
# Shared language helpers
# ------------------------------------------------------------------------------

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
      print(f"doctor: non-ASCII in {p}:{line}:{col} ({code} '{ch}')", file=sys.stderr)
      bad = True
      break

sys.exit(1 if bad else 0)
PY
}

doctor_check_go() {
  local issues=0

  doctor_require_dir "${ROOT_DIR}/golang-gin" || issues=$((issues+1))
  doctor_require_file "${ROOT_DIR}/golang-gin/go.mod" || issues=$((issues+1))
  doctor_require_file "${ROOT_DIR}/golang-gin/go.sum" || issues=$((issues+1))
  doctor_require_file "${ROOT_DIR}/golang-gin/.golangci.yaml" || issues=$((issues+1))

  doctor_require_env GOIMPORTS_VERSION || issues=$((issues+1))
  doctor_require_env GOVULNCHECK_VERSION || issues=$((issues+1))
  doctor_require_env GOLANGCI_LINT_VERSION || issues=$((issues+1))

  doctor_need_cmd go || issues=$((issues+1))
  doctor_need_cmd curl || issues=$((issues+1))
  doctor_need_cmd tar || issues=$((issues+1))
  doctor_need_cmd sha256sum || issues=$((issues+1))
  doctor_need_cmd install || issues=$((issues+1))
  doctor_need_cmd gcc || issues=$((issues+1))

  doctor_scan_non_ascii \
    "${ROOT_DIR}/golang-gin/go.mod" \
    "${ROOT_DIR}/golang-gin/go.sum" \
    "${ROOT_DIR}/golang-gin/.golangci.yaml" \
    >/dev/null || issues=$((issues+1))

  printf "%s\n" "$issues"
}

doctor_check_java() {
  local issues=0

  doctor_require_dir "${ROOT_DIR}/java-springboot" || issues=$((issues+1))
  doctor_require_file "${ROOT_DIR}/java-springboot/pom.xml" || issues=$((issues+1))

  doctor_require_env DEPENDENCY_CHECK_MAVEN_VERSION || issues=$((issues+1))
  doctor_require_env DEPENDENCY_CHECK_FAIL_CVSS || issues=$((issues+1))

  doctor_need_cmd mvn || issues=$((issues+1))
  doctor_need_cmd java || issues=$((issues+1))

  doctor_scan_non_ascii "${ROOT_DIR}/java-springboot/pom.xml" >/dev/null || issues=$((issues+1))
  printf "%s\n" "$issues"
}

doctor_check_python() {
  local issues=0

  doctor_require_dir "${ROOT_DIR}/python-django" || issues=$((issues+1))
  doctor_require_file "${ROOT_DIR}/python-django/requirements.txt" || issues=$((issues+1))
  doctor_require_file "${ROOT_DIR}/python-django/requirements.test.txt" || issues=$((issues+1))
  doctor_require_file "${ROOT_DIR}/python-django/requirements.lock" || issues=$((issues+1))
  doctor_require_file "${ROOT_DIR}/python-django/requirements.test.lock" || issues=$((issues+1))
  doctor_require_dir "${ROOT_DIR}/python-django/app" || issues=$((issues+1))

  doctor_require_env PIP_MAX_VERSION || issues=$((issues+1))
  doctor_require_env PIP_TOOLS_VERSION || issues=$((issues+1))
  doctor_require_env PIP_AUDIT_VERSION || issues=$((issues+1))

  doctor_need_cmd python3.12 || issues=$((issues+1))

  doctor_scan_non_ascii \
    "${ROOT_DIR}/python-django/requirements.txt" \
    "${ROOT_DIR}/python-django/requirements.test.txt" \
    "${ROOT_DIR}/python-django/requirements.lock" \
    "${ROOT_DIR}/python-django/requirements.test.lock" \
    >/dev/null || issues=$((issues+1))

  printf "%s\n" "$issues"
}

doctor_check_docker() {
  local issues=0

  doctor_require_env TRIVY_VERSION || issues=$((issues+1))
  doctor_need_cmd docker || issues=$((issues+1))

  if command -v docker >/dev/null 2>&1; then
    if ! docker info >/dev/null 2>&1; then
      warn "doctor: docker daemon not reachable (start Docker Desktop / ensure WSL integration)."
      issues=$((issues+1))
    fi
  fi

  printf "%s\n" "$issues"
}

doctor_print_mini_summary() {
  printf "\n\033[1m==> Doctor Summary\033[0m\n"
  printf "%-10s %-8s %-6s\n" "Module" "Result" "Issues"
  printf "%-10s %-8s %-6s\n" "------" "------" "------"
  for mod in trivy_repo go java python docker; do
    printf "%-10s %-8s %-6s\n" \
      "$mod" \
      "${mod_status[$mod]:-SKIPPED}" \
      "${mod_issues[$mod]:--}"
  done
}

doctor() {
  local scope="all"
  local doctor_summary=0

  for arg in "$@"; do
    case "$arg" in
      "" ) ;;
      all|go|java|python|docker) scope="$arg" ;;
      --summary|-s) doctor_summary=1 ;;
      *) die "Unknown doctor arg: $arg (use: doctor [all|go|java|python|docker] [--summary])" 2 ;;
    esac
  done

  say "Doctor - preflight checks (${scope})"

  if [[ -n "$(git status --porcelain 2>/dev/null || true)" ]]; then
    warn "doctor: git working tree is not clean (CI runs on a clean checkout)."
  fi

  doctor_require_file "$VERSIONS_FILE" || true
  doctor_scan_non_ascii "${BASH_SOURCE[0]}" "$VERSIONS_FILE" >/dev/null || true

  declare -gA mod_status mod_issues
  mod_status[trivy_repo]="SKIPPED"; mod_issues[trivy_repo]="-"
  mod_status[go]="SKIPPED";     mod_issues[go]="-"
  mod_status[java]="SKIPPED";   mod_issues[java]="-"
  mod_status[python]="SKIPPED"; mod_issues[python]="-"
  mod_status[docker]="SKIPPED"; mod_issues[docker]="-"

  local total_issues=0

  if [[ "$scope" == "all" || "$scope" == "go" ]]; then
    local rc; rc="$(doctor_check_go)"
    mod_issues[go]="$rc"
    mod_status[go]="$([[ "$rc" -eq 0 ]] && echo OK || echo FAIL)"
    total_issues=$((total_issues + rc))
  fi
  if [[ "$scope" == "all" || "$scope" == "java" ]]; then
    local rc; rc="$(doctor_check_java)"
    mod_issues[java]="$rc"
    mod_status[java]="$([[ "$rc" -eq 0 ]] && echo OK || echo FAIL)"
    total_issues=$((total_issues + rc))
  fi
  if [[ "$scope" == "all" || "$scope" == "python" ]]; then
    local rc; rc="$(doctor_check_python)"
    mod_issues[python]="$rc"
    mod_status[python]="$([[ "$rc" -eq 0 ]] && echo OK || echo FAIL)"
    total_issues=$((total_issues + rc))
  fi
  if [[ "$scope" == "all" || "$scope" == "docker" ]]; then
    local rc; rc="$(doctor_check_docker)"
    mod_issues[docker]="$rc"
    mod_status[docker]="$([[ "$rc" -eq 0 ]] && echo OK || echo FAIL)"
    total_issues=$((total_issues + rc))
  fi

  if [[ "$doctor_summary" -eq 1 ]]; then
    doctor_print_mini_summary
  fi

  if [[ "$total_issues" -ne 0 ]]; then
    die "Doctor failed with ${total_issues} issue(s). Fix warnings above before running checks." 2
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
  record_summary go security "govulncheck=on"

  say "Go / Gin - quality, tests, security"

  require_env GOIMPORTS_VERSION
  require_env GOVULNCHECK_VERSION
  require_env GOLANGCI_LINT_VERSION

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
        if [[ -n "${GO_MODULE:-}" ]]; then
          goimports -l -local "${GO_MODULE}" "$d"
        else
          goimports -l "$d"
        fi
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

    say "Setup - system deps for race detector (CGO)"
    need_cmd gcc

    say "Test - unit tests (race, shuffle) + coverage profile"
    CGO_ENABLED=1 go test ./... -race -shuffle=on -count=1 \
      -covermode=atomic -coverprofile=coverage.out

    say "Coverage - enforce minimum statements (Go) (min=${GO_COVERAGE_MIN}%)"
    python3.12 - <<'PY'
import os, re, subprocess, sys
out = subprocess.check_output(["go", "tool", "cover", "-func=coverage.out"], text=True)
m = re.search(r"total:\s*\(statements\)\s*([\d.]+)%", out)
if not m:
    print("Could not parse total coverage from go tool cover output.", file=sys.stderr)
    sys.exit(2)
pct = float(m.group(1))
min_pct = float(os.environ.get("GO_COVERAGE_MIN", "100"))
print(f"Go total coverage: {pct:.1f}% (min {min_pct:.1f}%)")
sys.exit(0 if pct >= min_pct else 1)
PY

    local go_cov
    go_cov="$(go tool cover -func=coverage.out | awk '/^total:/{print $NF; exit}')"
    [[ -n "$go_cov" ]] || die "could not parse Go coverage percent" 2
    record_summary go coverage "$go_cov"

    say "Artifact - coverage profile (coverage.out)"
    test -f coverage.out
    copy_artifact "coverage.out" "$ARTIFACTS_GO_DIR" "coverage.out"

    say "Security - vulnerability scan (govulncheck)"
    set -o pipefail
    govulncheck -test ./... | tee govulncheck.txt

    say "Artifact - govulncheck report"
    [[ -f govulncheck.txt ]] || warn "expected govulncheck.txt but it was not created."
    copy_artifact "govulncheck.txt" "$ARTIFACTS_GO_DIR" "govulncheck.txt"

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
  record_summary java security "dependency-check=on"

  say "Java / Spring Boot - Spotless, SpotBugs, tests, JaCoCo, security"

  require_env DEPENDENCY_CHECK_MAVEN_VERSION
  require_env DEPENDENCY_CHECK_FAIL_CVSS
  need_cmd mvn

  (
    cd "$ROOT_DIR/java-springboot"
    export MAVEN_OPTS="${MAVEN_OPTS:---sun-misc-unsafe-memory-access=allow}"

    say "Build - verify (Spotless, SpotBugs, tests, JaCoCo) (jacoco.min.ratio=${JAVA_COVERAGE_MIN_RATIO})"
    mvn -B -ntp verify -Djacoco.min.ratio="${JAVA_COVERAGE_MIN_RATIO}"

    say "Artifact - JaCoCo report"
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

    copy_artifact "target/site/jacoco/jacoco.xml" "$ARTIFACTS_JAVA_DIR" "jacoco.xml"

    say "Cache - OWASP Dependency-Check data (local)"
    local odc_data_dir
    odc_data_dir="${ROOT_DIR}/.cache/dependency-check"
    ensure_dir "$odc_data_dir"

    local odc_strict="${ODC_STRICT_DEFAULT}"
    local odc_retries="${ODC_RETRIES_DEFAULT}"
    local purge_on_fail="${ODC_PURGE_ON_FAIL_DEFAULT}"

    local nvd_delay_ms="${ODC_NVD_API_DELAY_MS_DEFAULT}"
    local nvd_max_retry="${ODC_NVD_MAX_RETRY_COUNT_DEFAULT}"
    local nvd_valid_hours="${ODC_NVD_VALID_FOR_HOURS_DEFAULT}"

    local have_key=0
    [[ -n "${NVD_API_KEY:-}" ]] && have_key=1

    local have_db=0
    if ls "${odc_data_dir}"/*.mv.db >/dev/null 2>&1 || ls "${odc_data_dir}"/*.db >/dev/null 2>&1; then
      have_db=1
    fi

    local -a odc_cmd=(
      mvn -B -ntp "org.owasp:dependency-check-maven:${DEPENDENCY_CHECK_MAVEN_VERSION}:check"
      -DfailBuildOnCVSS="${DEPENDENCY_CHECK_FAIL_CVSS}"
      -Dformats=HTML,JSON
      -DdataDirectory="${odc_data_dir}"
      -DnvdApiKeyEnvironmentVariable=NVD_API_KEY
      -DnvdApiDelay="${nvd_delay_ms}"
      -DnvdMaxRetryCount="${nvd_max_retry}"
      -DnvdValidForHours="${nvd_valid_hours}"
    )

    local odc_skip=0
    if [[ "$have_key" -eq 0 ]]; then
      if [[ "$have_db" -eq 1 ]]; then
        warn "NVD_API_KEY is not set; running Dependency-Check offline using cached data (autoUpdate=false, failOnError=false)."
        odc_cmd+=(-DautoUpdate=false -DfailOnError=false)
      else
        warn "NVD_API_KEY is not set and no cached Dependency-Check DB found at ${odc_data_dir}; skipping Dependency-Check. Export NVD_API_KEY to enable."
        record_summary java security "dependency-check=skipped(no-key)"
        odc_skip=1
      fi
    else
      odc_cmd+=(-DfailOnError=true)
    fi

    if [[ "$odc_skip" -eq 0 ]]; then
      say "Security - OWASP Dependency-Check scan"

      set +e
      retry "$odc_retries" "${odc_cmd[@]}"
      local odc_rc=$?
      set -e

      if [[ "$odc_rc" -ne 0 && "$purge_on_fail" == "1" ]]; then
        warn "Dependency-Check failed (rc=${odc_rc}). Purging local data dir and retrying once..."
        rm -rf "$odc_data_dir" >/dev/null 2>&1 || true
        ensure_dir "$odc_data_dir"

        set +e
        retry "$odc_retries" "${odc_cmd[@]}"
        odc_rc=$?
        set -e
      fi

      if [[ "$odc_rc" -ne 0 ]]; then
        if [[ "$odc_strict" == "1" ]]; then
          die "Dependency-Check failed (rc=${odc_rc}). Hint: export NVD_API_KEY to avoid NVD rate limits (HTTP 429)." 2
        fi
        warn "Dependency-Check failed (rc=${odc_rc}) (non-gating locally). Hint: export NVD_API_KEY to avoid NVD rate limits (HTTP 429)."
        record_summary java security "dependency-check=warn"
      else
        record_summary java security "dependency-check=on"
      fi

      say "Artifact - Dependency-Check reports"
      [[ -f target/dependency-check-report.html ]] || warn "target/dependency-check-report.html not found."
      [[ -f target/dependency-check-report.json ]] || warn "target/dependency-check-report.json not found."
      copy_artifact "target/dependency-check-report.html" "$ARTIFACTS_JAVA_DIR" "dependency-check-report.html"
      copy_artifact "target/dependency-check-report.json" "$ARTIFACTS_JAVA_DIR" "dependency-check-report.json"
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
  record_summary python security "pip-audit=on"

  say "Python / Django - quality, tests, coverage, security"

  require_env PIP_MAX_VERSION
  require_env PIP_TOOLS_VERSION
  require_env PIP_AUDIT_VERSION

  need_cmd python3.12

  (
    cd "$ROOT_DIR/python-django"

    local venv_dir=".venv-ci"
    local cleanup_venv="${PY_CLEAN_VENV_DEFAULT}"

    say "Setup - create isolated venv + install pip-tools"
    rm -rf "$venv_dir"
    python3.12 -m venv "$venv_dir"

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
    python -m pip install -U "pip<${PIP_MAX_VERSION}"
    python -m pip install --require-hashes -r requirements.lock
    python -m pip install --require-hashes -r requirements.test.lock

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

    say "Quality - typecheck (mypy)"
    mypy app

    say "Test - unit tests + coverage (pytest, enforce min, write XML) (min=${PY_COVERAGE_MIN}%)"
    DJANGO_SETTINGS_MODULE=django_app.settings \
      pytest --cov --cov-report=xml --cov-fail-under="${PY_COVERAGE_MIN}"

    say "Artifact - coverage report (coverage.xml)"
    test -f coverage.xml
    copy_artifact "coverage.xml" "$ARTIFACTS_PY_DIR" "coverage.xml"

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

    say "Security - vulnerability scan (pip-audit)"
    python -m pip install "pip-audit==${PIP_AUDIT_VERSION}"
    pip-audit -r requirements.lock --format=json --output=pip-audit.json
    pip-audit -r requirements.lock

    say "Artifact - pip-audit report"
    [[ -f pip-audit.json ]] || warn "pip-audit.json not found."
    copy_artifact "pip-audit.json" "$ARTIFACTS_PY_DIR" "pip-audit.json"

    say "Python / Django - OK"
  )

  record_summary python status PASS
  CURRENT_MODULE=""
}

# ------------------------------------------------------------------------------
# Docker + Trivy
# ------------------------------------------------------------------------------

docker_has_buildx() { docker buildx version >/dev/null 2>&1; }

ensure_buildx_builder() {
  local name="ci-local"
  if ! docker buildx inspect "$name" >/dev/null 2>&1; then
    docker buildx create --name "$name" --driver docker-container --use >/dev/null
  else
    docker buildx use "$name" >/dev/null
  fi
  docker buildx inspect --bootstrap >/dev/null
}

docker_build_image() {
  local image_ref="$1"
  local context="$2"
  shift 2

  local use_buildx="${DOCKER_USE_BUILDX_DEFAULT}"
  local pull="${DOCKER_PULL_BASE_DEFAULT}"

  local -a base_args=()
  [[ "$pull" == "1" ]] && base_args+=(--pull)

  if [[ "$use_buildx" == "1" ]] && docker_has_buildx; then
    ensure_buildx_builder
    DOCKER_BUILDKIT=1 docker buildx build --load -t "$image_ref" "${base_args[@]}" "$@" "$context"
  else
    DOCKER_BUILDKIT=1 docker build -t "$image_ref" "${base_args[@]}" "$@" "$context"
  fi
}

trivy_fs_scan_table_gate() {
  local trivy_img="$1"
  local cache_dir="$2"
  local cache_mount="$3"
  local scanners="$4"
  local severity="$5"
  local timeout="$6"

  local out="/workspace/.cache/ci-local/artifacts/trivy/trivy-repo.txt"
  ensure_dir "$ARTIFACTS_TRIVY_DIR"

  docker run --rm \
    -v "${ROOT_DIR}:/workspace" \
    -w /workspace \
    -v "${cache_dir}:${cache_mount}" \
    "$trivy_img" \
    fs \
    --cache-dir "$cache_mount" \
    --timeout "$timeout" \
    --scanners "$scanners" \
    --severity "$severity" \
    --ignore-unfixed \
    --exit-code 1 \
    --no-progress \
    --skip-dirs .git,.cache \
    --format table \
    --output "$out" \
    .
}

trivy_fs_scan_sarif() {
  local trivy_img="$1"
  local cache_dir="$2"
  local cache_mount="$3"
  local scanners="$4"
  local severity="$5"
  local timeout="$6"

  local out="/workspace/.cache/ci-local/artifacts/trivy/trivy-repo.sarif"
  ensure_dir "$ARTIFACTS_TRIVY_DIR"

  docker run --rm \
    -v "${ROOT_DIR}:/workspace" \
    -w /workspace \
    -v "${cache_dir}:${cache_mount}" \
    "$trivy_img" \
    fs \
    --cache-dir "$cache_mount" \
    --timeout "$timeout" \
    --scanners "$scanners" \
    --severity "$severity" \
    --ignore-unfixed \
    --exit-code 0 \
    --no-progress \
    --skip-dirs .git,.cache \
    --format sarif \
    --output "$out" \
    .
}

trivy_image_scan_table_gate() {
  local trivy_img="$1"
  local cache_dir="$2"
  local cache_mount="$3"
  local scanners="$4"
  local severity="$5"
  local timeout="$6"
  local image_ref="$7"
  local out_txt_host="$8"

  local rel="${out_txt_host#${ROOT_DIR}/}"
  local out="/workspace/${rel}"
  ensure_dir "$ARTIFACTS_TRIVY_DIR"

  docker run --rm \
    -v /var/run/docker.sock:/var/run/docker.sock \
    -v "${ROOT_DIR}:/workspace" \
    -w /workspace \
    -v "${cache_dir}:${cache_mount}" \
    "$trivy_img" \
    image \
    --cache-dir "$cache_mount" \
    --timeout "$timeout" \
    --scanners "$scanners" \
    --vuln-type os,library \
    --severity "$severity" \
    --ignore-unfixed \
    --exit-code 1 \
    --no-progress \
    --format table \
    --output "$out" \
    "$image_ref"
}

trivy_image_scan_sarif() {
  local trivy_img="$1"
  local cache_dir="$2"
  local cache_mount="$3"
  local scanners="$4"
  local severity="$5"
  local timeout="$6"
  local image_ref="$7"
  local out_sarif_host="$8"

  local rel="${out_sarif_host#${ROOT_DIR}/}"
  local out="/workspace/${rel}"
  ensure_dir "$ARTIFACTS_TRIVY_DIR"

  docker run --rm \
    -v /var/run/docker.sock:/var/run/docker.sock \
    -v "${ROOT_DIR}:/workspace" \
    -w /workspace \
    -v "${cache_dir}:${cache_mount}" \
    "$trivy_img" \
    image \
    --cache-dir "$cache_mount" \
    --timeout "$timeout" \
    --scanners "$scanners" \
    --vuln-type os,library \
    --severity "$severity" \
    --ignore-unfixed \
    --exit-code 0 \
    --no-progress \
    --format sarif \
    --output "$out" \
    "$image_ref"
}

run_trivy_repo() {
  CURRENT_MODULE="trivy_repo"
  record_summary trivy_repo status RUNNING
  record_summary trivy_repo coverage "-"
  record_summary trivy_repo security "trivy_repo=on"

  say "Security - Trivy (repo) + SARIF"

  require_env TRIVY_VERSION
  need_cmd docker
  need_cmd date

  if ! docker info >/dev/null 2>&1; then
    record_summary trivy_repo status FAIL
    record_summary trivy_repo coverage "FAIL(docker)"
    die "docker daemon not reachable (is Docker running?)" 2
  fi

  ensure_dir "$ARTIFACTS_TRIVY_DIR"

  local scanners="${TRIVY_SCANNERS_DEFAULT}"
  local severity="${TRIVY_SEVERITY_DEFAULT}"
  local timeout="${TRIVY_TIMEOUT_DEFAULT}"
  local scan_retries="${TRIVY_SCAN_RETRIES_DEFAULT}"

  local sev_label
  sev_label="$(printf '%s' "$severity" | tr ',' '/')"

  local trivy_cache_dir="${ROOT_DIR}/.cache/trivy"
  ensure_dir "$trivy_cache_dir"

  local trivy_cache_mount="/trivycache"
  local trivy_img="aquasec/trivy:${TRIVY_VERSION#v}"

  if ! docker image inspect "$trivy_img" >/dev/null 2>&1; then
    say "Setup - pull Trivy scanner image (${trivy_img})"
    retry 2 docker pull "$trivy_img" >/dev/null || {
      record_summary trivy_repo status FAIL
      record_summary trivy_repo coverage "FAIL(pull)"
      die "failed to pull trivy image: $trivy_img" 2
    }
  fi

  say "Security - Trivy scan (fs) (table, gate HIGH/CRITICAL)"
  if ! retry "$scan_retries" trivy_fs_scan_table_gate \
      "$trivy_img" "$trivy_cache_dir" "$trivy_cache_mount" \
      "$scanners" "$severity" "$timeout"; then
    record_summary trivy_repo coverage "FAIL(${sev_label})"
    record_summary trivy_repo status FAIL
    die "trivy fs scan found ${severity} issues (gating)" 1
  fi

  record_summary trivy_repo coverage "PASS(${sev_label})"

  say "Security - Trivy scan (fs) -> SARIF"
  if ! retry "$scan_retries" trivy_fs_scan_sarif \
      "$trivy_img" "$trivy_cache_dir" "$trivy_cache_mount" \
      "$scanners" "$severity" "$timeout"; then
    warn "trivy fs sarif generation failed (non-gating)."
  fi

  record_summary trivy_repo status PASS
  CURRENT_MODULE=""
}

run_docker() {
  CURRENT_MODULE="docker"
  record_summary docker status RUNNING
  record_summary docker coverage "-"
  record_summary docker security "trivy_image=on"

  say "Docker - build + Trivy image scan + SARIF"

  require_env TRIVY_VERSION
  need_cmd docker
  need_cmd date
  need_cmd git

  if ! docker info >/dev/null 2>&1; then
    die "docker daemon not reachable (is Docker running?)" 2
  fi

  ensure_dir "$ARTIFACTS_TRIVY_DIR"

  local run_tests="false"
  local rev
  rev="$(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
  local tag="${rev}"
  local version_arg="${rev}"

  local build_time
  build_time="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

  local rev_full
  rev_full="$(git rev-parse --short HEAD 2>/dev/null || echo unknown)"

  local scanners="${TRIVY_SCANNERS_DEFAULT}"
  local severity="${TRIVY_SEVERITY_DEFAULT}"
  local timeout="${TRIVY_TIMEOUT_DEFAULT}"

  local sev_label
  sev_label="$(printf '%s' "$severity" | tr ',' '/')"

  local build_retries="${DOCKER_BUILD_RETRIES_DEFAULT}"
  local scan_retries="${TRIVY_SCAN_RETRIES_DEFAULT}"

  local trivy_cache_dir="${ROOT_DIR}/.cache/trivy"
  ensure_dir "$trivy_cache_dir"

  local trivy_cache_mount="/trivycache"
  local trivy_img="aquasec/trivy:${TRIVY_VERSION#v}"

  if ! docker image inspect "$trivy_img" >/dev/null 2>&1; then
    say "Setup - pull Trivy scanner image (${trivy_img})"
    retry 2 docker pull "$trivy_img" >/dev/null || die "failed to pull trivy image: $trivy_img" 2
  fi

  build_and_scan() {
    local image="$1"
    local context="$2"
    local image_ref="${image}:${tag}"

    say "Build - ${image_ref}"
    retry "$build_retries" docker_build_image "$image_ref" "$context" \
      --build-arg "RUN_TESTS=${run_tests}" \
      --build-arg "SERVICE_NAME=${image}" \
      --build-arg "VERSION=${version_arg}" \
      --build-arg "BUILD_TIME=${build_time}" \
      --build-arg "OCI_IMAGE_SOURCE=local" \
      --build-arg "OCI_IMAGE_REVISION=${rev_full}" \
      || die "docker build failed for ${image_ref}" 2

    say "Scan - Trivy image (${image_ref}) (table, gate HIGH/CRITICAL)"
    local out_txt="${ARTIFACTS_TRIVY_DIR}/trivy-image-${image}.txt"
    local out_sarif="${ARTIFACTS_TRIVY_DIR}/trivy-image-${image}.sarif"

    if ! retry "$scan_retries" trivy_image_scan_table_gate "$trivy_img" "$trivy_cache_dir" "$trivy_cache_mount" "$scanners" "$severity" "$timeout" "$image_ref" "$out_txt"; then
      die "trivy image scan found ${severity} issues (gating) for ${image_ref}" 1
    fi

    say "Trivy - scan image -> SARIF"
    if ! retry "$scan_retries" trivy_image_scan_sarif "$trivy_img" "$trivy_cache_dir" "$trivy_cache_mount" "$scanners" "$severity" "$timeout" "$image_ref" "$out_sarif"; then
      warn "trivy image sarif generation failed for ${image_ref} (non-gating)."
    fi
  }

  build_and_scan golang-gin-app      "${ROOT_DIR}/golang-gin"
  build_and_scan java-springboot-app "${ROOT_DIR}/java-springboot"
  build_and_scan python-django-app   "${ROOT_DIR}/python-django"

  record_summary docker coverage "PASS(${sev_label})"
  record_summary docker status PASS
  CURRENT_MODULE=""
}

# ------------------------------------------------------------------------------
# CLI flags
# ------------------------------------------------------------------------------

usage() {
  cat <<'TXT'
ci-local: run local checks

USAGE
  ./.ci-local.sh [command] [--help]

COMMANDS
  all         Run the default flow: trivy_repo + go + java + python
  trivy_repo  Scan the repository filesystem with Trivy (table gate + SARIF).
  go          Run Go (golang-gin) checks only.
  java        Run Java (java-springboot) checks only.
  python      Run Python (python-django) checks only.
  docker      Build images + Trivy image scans (table gate + SARIF).
  doctor      Preflight checks (doctor [all|go|java|python|docker] [--summary]).

ENV
  LOG_LEVEL=quiet|info|debug  (default: info)
  GO_COVERAGE_MIN             (default: 100) percent
  PY_COVERAGE_MIN             (default: 100) percent
  JAVA_COVERAGE_MIN_RATIO     (default: 1.0) ratio (0.0-1.0)
  NVD_API_KEY                 (optional; enables live OWASP Dependency-Check updates)

ARTIFACT OUTPUT
  .cache/ci-local/artifacts/

EXAMPLES
  ./.ci-local.sh
  ./.ci-local.sh all
  ./.ci-local.sh trivy_repo
  ./.ci-local.sh go
  ./.ci-local.sh doctor all --summary
  LOG_LEVEL=debug ./.ci-local.sh
TXT
}

main() {
  local target="${1:-all}"
  shift || true

  case "$target" in
    all)
      init_summary
      run_trivy_repo
      run_go
      run_java
      run_python
      ;;
    trivy_repo)
      init_summary
      run_trivy_repo
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
    docker)
      init_summary
      run_docker
      ;;
    doctor)
      doctor "$@"
      ;;
    -h|--help|help)
      usage
      ;;
    *)
      echo "Unknown command: $target" >&2
      usage
      exit 2
      ;;
  esac
}

main "$@"
