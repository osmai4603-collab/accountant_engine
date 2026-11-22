#!/usr/bin/env bash
# Full Fuzz Testing Runner for Accounting Kernel (AE-FUZZ-RUNNER)
# Runs all 16 Fuzz targets across the 5 kernel packages with documented fuzztime.
#
# Usage: FUZZTIME=3s ./scripts/run_fuzz_all.sh [logfile]
set -uo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

logfile="${1:-/tmp/fuzz_evidence.log}"
mkdir -p "$(dirname "$logfile")"
: >"$logfile"

fuzztime="${FUZZTIME:-3s}"

echo "=== Accounting Kernel Full Fuzz Suite ===" | tee -a "$logfile"
echo "repo_root=${repo_root}" | tee -a "$logfile"
echo "fuzztime_per_target=${fuzztime}" | tee -a "$logfile"
echo "started_utc=$(date -u +%Y-%m-%dT%H:%M:%SZ)" | tee -a "$logfile"

targets=(
  "types:FuzzMoneyFromStringNeverPanics"
  "types:FuzzMoneyJSONNeverPanics"
  "types:FuzzMinorUnitsExactNeverDropsFraction"
  "types:FuzzMoneyOperationsPreserveCurrencyAndDeterminism"
  "types:FuzzMoneyConstructorPrecisionAndRoundTrip"
  "types:FuzzBankersRoundingIsDeterministicAndBounded"
  "masterdata:FuzzGenerateFiscalYearMaintainsContinuityAndContainment"
  "masterdata:FuzzEffectiveRangeValidationNeverPanics"
  "posting:FuzzReversalNetImpactIsZeroPerAccount"
  "posting:FuzzValidateEntryDoubleEntryNeverPanics"
  "posting:FuzzEntryLineRejectsCurrencyAndExchangeRateMismatch"
  "posting:FuzzBalancedEntryProperties"
  "posting:FuzzRoundingResidualRequiresExplicitSettlement"
  "governance:FuzzAuditSnapshotDetectsPayloadTampering"
  "valuation:FuzzFXConversionNeverPanics"
  "valuation:FuzzRateRegistryEffectiveSelectionNeverPanics"
)

passed=0
failed=0

for spec in "${targets[@]}"; do
  pkg="${spec%%:*}"
  fn="${spec##*:}"
  echo "" | tee -a "$logfile"
  echo "--- Running Fuzz: ${pkg}/${fn} (fuzztime=${fuzztime}) ---" | tee -a "$logfile"
  
  start_ts=$(date +%s)
  out=$(go test "./internal/kernal/${pkg}" -run='^$' -fuzz="^${fn}$" -fuzztime="${fuzztime}" -count=1 2>&1)
  rc=$?
  end_ts=$(date +%s)
  duration=$((end_ts - start_ts))
  
  echo "$out" >>"$logfile"
  exec_summary=$(echo "$out" | grep -E "fuzz: elapsed:" | tail -1 || true)
  if [[ -n "$exec_summary" ]]; then
    echo "summary: ${exec_summary}" | tee -a "$logfile"
  fi
  
  if [[ $rc -eq 0 ]]; then
    passed=$((passed + 1))
    echo "result: PASS (0 panics, duration=${duration}s)" | tee -a "$logfile"
  else
    failed=$((failed + 1))
    echo "result: FAIL (exit=${rc}, duration=${duration}s)" | tee -a "$logfile"
  fi
done

echo "" | tee -a "$logfile"
echo "=== Fuzz Testing Summary ===" | tee -a "$logfile"
echo "Targets Run:    $((passed + failed))" | tee -a "$logfile"
echo "Targets Passed: ${passed}" | tee -a "$logfile"
echo "Targets Failed: ${failed}" | tee -a "$logfile"

if [[ $failed -eq 0 ]]; then
  echo "RESULT: PASS (all 16 Fuzz targets passed without panics)" | tee -a "$logfile"
  exit 0
else
  echo "RESULT: FAIL (${failed} targets failed)" | tee -a "$logfile"
  exit 1
fi

