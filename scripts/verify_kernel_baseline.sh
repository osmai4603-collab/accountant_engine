#!/usr/bin/env bash
# Reproducible Phase 0 baseline run for the accounting kernel remediation plan.
#
# Runs the fixed evidence-commands from docs/KERNEL_REMEDIATION_BASELINE.md:
#   environment fingerprint, formatting, kernel unit tests, kernel race tests,
#   full repository tests, architecture gate, external-consumer module tests,
#   fuzz smoke tests (9 targets), and a fresh kernel coverage profile.
#
# The script is read-only with respect to tracked source files. All generated
# artifacts are written outside the repository (default: /tmp/opencode/phase0).
#
# Usage:
#   ./scripts/verify_kernel_baseline.sh [artifact_dir]
#   FUZZTIME=10s ./scripts/verify_kernel_baseline.sh
set -uo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

artifacts="${1:-/tmp/opencode/phase0}"
logs="${artifacts}/logs"
mkdir -p "$logs"

fuzztime="${FUZZTIME:-30s}"

run() {
  local label="$1" logfile="$2"
  shift 2
  printf '\n== %s ==\n' "$label"
  "$@" >"$logfile" 2>&1
  local rc=$?
  printf 'exit=%d log=%s\n' "$rc" "$logfile"
  return "$rc"
}

echo "Kernel baseline verification"
echo "repo_root=${repo_root}"
echo "artifacts=${artifacts}"
echo "fuzztime=${fuzztime}"

{
  echo "captured_utc=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "git_commit=$(git rev-parse HEAD)"
  echo "git_short=$(git rev-parse --short HEAD)"
  echo "git_branch=$(git branch --show-current)"
  echo "git_status_porcelain:"
  git status --porcelain=v1
  echo "go_version=$(go version)"
  echo "go_mod_line=$(awk '/^go /{print; exit}' go.mod)"
  echo "toolchain_line=$(awk '/^toolchain /{print; exit}' go.mod)"
  echo "go_env=$(go env GOVERSION GOOS GOARCH GOTOOLCHAIN | tr '\n' ' ')"
} >"${logs}/environment.txt" 2>&1
echo "== environment =="
cat "${logs}/environment.txt"

rc=0

run "gofmt -l internal/kernal/" "${logs}/gofmt_kernel.log" \
  bash -c 'gofmt -l internal/kernal/' || rc=1

run "go test kernel" "${logs}/kernel_unit.log" \
  go test ./internal/kernal/... -count=1 || rc=1

run "go test -race kernel" "${logs}/kernel_race.log" \
  go test -race ./internal/kernal/... -count=1 || rc=1

# The contract-parity suite rewrites these tracked reports with fresh timings.
# Snapshot and restore them verbatim so this baseline script never mutates the
# working tree, including any pre-existing local edits.
parity_reports=(
  docs/reports/contract_parity_report.md
  test/contract_parity/parity_report.md
)
for report in "${parity_reports[@]}"; do
  if [[ -f "$report" ]]; then
    cp "$report" "${artifacts}/$(basename "$report").original"
  fi
done
run "go test full repository" "${logs}/repo_full_test.log" \
  go test ./... -count=1 || rc=1
for report in "${parity_reports[@]}"; do
  original="${artifacts}/$(basename "$report").original"
  if [[ -f "$original" ]]; then
    cp "$original" "$report"
  fi
done

run "verify_architecture" "${logs}/architecture.log" \
  ./scripts/verify_architecture.sh || rc=1

if [[ -f test/external_consumer/go.mod ]]; then
  run "external consumer module" "${logs}/external_consumer_test.log" \
    bash -c 'cd test/external_consumer && go test ./... -count=1' || rc=1
fi

targets=(
  "types:FuzzMoneyFromStringNeverPanics"
  "types:FuzzMoneyJSONNeverPanics"
  "masterdata:FuzzGenerateFiscalYearMaintainsContinuityAndContainment"
  "posting:FuzzValidateEntryDoubleEntryNeverPanics"
  "posting:FuzzEntryLineRejectsCurrencyAndExchangeRateMismatch"
  "posting:FuzzBalancedEntryProperties"
  "posting:FuzzRoundingResidualRequiresExplicitSettlement"
  "posting:FuzzReversalNetImpactIsZeroPerAccount"
  "governance:FuzzAuditSnapshotDetectsPayloadTampering"
)
: >"${logs}/fuzz_smoke.log"
for spec in "${targets[@]}"; do
  pkg="${spec%%:*}"
  fn="${spec##*:}"
  echo "### ${pkg}/${fn} $(date -u +%FT%TZ)" >>"${logs}/fuzz_smoke.log"
  go test "./internal/kernal/${pkg}" -run='^$' -fuzz="^${fn}$" \
    -fuzztime="${fuzztime}" -count=1 >>"${logs}/fuzz_smoke.log" 2>&1
  echo "EXIT=$?" >>"${logs}/fuzz_smoke.log"
done
echo "== fuzz smoke =="
grep -E '^(###|EXIT=|ok  |FAIL|fuzz:)' "${logs}/fuzz_smoke.log"

run "kernel coverage profile" "${logs}/coverage_run.log" \
  go test ./internal/kernal/... -count=1 -coverprofile="${artifacts}/kernel.cover" || rc=1
go tool cover -func="${artifacts}/kernel.cover" >"${logs}/coverage_func.txt" 2>&1
echo "== coverage total =="
tail -1 "${logs}/coverage_func.txt"

printf '\nRESULT: %s\n' "$([[ $rc -eq 0 ]] && echo PASS || echo FAIL)"
exit "$rc"
