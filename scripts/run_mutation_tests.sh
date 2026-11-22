#!/usr/bin/env bash
# Targeted Semantic Business Rule Mutation Testing Runner (AE-MUTATION-RUNNER)
#
# Reproducibly tests the 8 critical business rule mutations documented in
# internal/kernal/KERNEL_MUTATION_REPORT.md:
#   MUT-01: INV-004 (Currency Uniformity in types/money.go)
#   MUT-02: INV-002 (Precision Boundaries in types/money.go)
#   MUT-03: INV-008 (Double-Entry Balance in posting/entry_validation.go)
#   MUT-04: INV-017 (Calendar SoftClose in masterdata/fiscal_calendar.go)
#   MUT-05: INV-025 (Audit Snapshot Sequence in governance/audit_chain.go)
#   MUT-06: INV-012 (Entry Reversal Identity in posting/entry_validation.go)
#   MUT-07: INV-024 (Audit Tamper Detection in governance/audit_chain.go)
#   MUT-08: INV-016 (Period Date Containment in masterdata/fiscal_calendar.go)
#
# Usage: ./scripts/run_mutation_tests.sh [logfile]
set -uo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

logfile="${1:-/tmp/mutation_run.log}"
mkdir -p "$(dirname "$logfile")"
: >"$logfile"

echo "=== Targeted Mutation Testing Suite ===" | tee -a "$logfile"
echo "repo_root=${repo_root}" | tee -a "$logfile"
echo "logfile=${logfile}" | tee -a "$logfile"
echo "started_utc=$(date -u +%Y-%m-%dT%H:%M:%SZ)" | tee -a "$logfile"

# Clean up trap to ensure backups are always restored
restore_all() {
  for f in $(find internal/kernal -name "*.bak" 2>/dev/null); do
    orig="${f%.bak}"
    if [[ -f "$f" ]]; then
      mv -f "$f" "$orig"
    fi
  done
}
trap restore_all EXIT INT TERM

mutants_tested=0
mutants_killed=0
mutants_survived=0

run_mutant() {
  local id="$1"
  local rule="$2"
  local target_file="$3"
  local description="$4"
  local mutate_cmd="$5"
  local test_pkg="$6"
  local test_run="$7"

  mutants_tested=$((mutants_tested + 1))
  echo "" | tee -a "$logfile"
  echo "--- [${id}] ${rule}: ${description} ---" | tee -a "$logfile"
  echo "target: ${target_file}" | tee -a "$logfile"

  # Backup original file
  cp -p "$target_file" "${target_file}.bak"

  # Apply mutation
  eval "$mutate_cmd"

  # Run test
  local test_out
  local test_rc=0
  test_out=$(go test "$test_pkg" -run "$test_run" -count=1 2>&1) || test_rc=$?

  # Restore original file immediately
  mv -f "${target_file}.bak" "$target_file"

  echo "test exit code: ${test_rc}" >>"$logfile"
  echo "$test_out" >>"$logfile"

  if [[ $test_rc -ne 0 ]]; then
    mutants_killed=$((mutants_killed + 1))
    echo "result: KILLED (test failed as expected when invariant violated)" | tee -a "$logfile"
  else
    mutants_survived=$((mutants_survived + 1))
    echo "result: SURVIVED (CRITICAL: test passed despite invariant violation!)" | tee -a "$logfile"
  fi
}

# MUT-01: INV-004 Currency Uniformity in types/money.go
run_mutant \
  "MUT-01" \
  "INV-004" \
  "internal/kernal/types/money.go" \
  "Disable currency match check in ensureSameCurrency" \
  "sed -i 's/if m.currency != other.currency {/if false \&\& m.currency != other.currency {/' internal/kernal/types/money.go" \
  "./internal/kernal/types" \
  "TestMoneyOperationsPreserveCurrency"

# MUT-02: INV-002 Precision Boundaries in types/money.go
run_mutant \
  "MUT-02" \
  "INV-002" \
  "internal/kernal/types/money.go" \
  "Allow more than 28 integer digits in Money precision limits" \
  "sed -i 's/integerDigits(amount) > MaximumIntegerDigits/false/g' internal/kernal/types/money.go && sed -i 's/amount.Exponent() > MaximumIntegerDigits/false/g' internal/kernal/types/money.go" \
  "./internal/kernal/types" \
  "TestMoneyRejectsIntegerDigitOverflow"

# MUT-03: INV-008 Double-Entry Balance in posting/entry_validation.go
run_mutant \
  "MUT-03" \
  "INV-008" \
  "internal/kernal/posting/entry_validation.go" \
  "Disable double-entry net zero balance check" \
  "sed -i 's/if !net.IsZero() {/if false \&\& !net.IsZero() {/' internal/kernal/posting/entry_validation.go" \
  "./internal/kernal/posting" \
  "TestDoubleEntryMathematicalValidation"

# MUT-04: INV-017 Calendar SoftClose in masterdata/fiscal_calendar.go
run_mutant \
  "MUT-04" \
  "INV-017" \
  "internal/kernal/masterdata/fiscal_calendar.go" \
  "Allow posting in SoftClosed period without explicit policy permission" \
  "sed -i 's/year.State != FiscalYearOpen && !(allowSoftClosed && year.State == FiscalYearSoftClosed)/year.State == FiscalYearHardClosed/g' internal/kernal/masterdata/fiscal_calendar.go && sed -i 's/period.State != PeriodOpen && !(allowSoftClosed && period.State == PeriodSoftClosed)/period.State == PeriodHardClosed/g' internal/kernal/masterdata/fiscal_calendar.go" \
  "./internal/kernal/masterdata" \
  "TestPostingPeriodRequiresFiscalYearAndExplicitSoftClosePolicy"

# MUT-05: INV-025 Audit Snapshot Sequence in governance/audit_chain.go
run_mutant \
  "MUT-05" \
  "INV-025" \
  "internal/kernal/governance/audit_chain.go" \
  "Disable snapshot sequence matching against checkpoint" \
  "sed -i 's/if snapshot.SnapshotSequence != checkpoint.SequenceNumber {/if false \&\& snapshot.SnapshotSequence != checkpoint.SequenceNumber {/' internal/kernal/governance/audit_chain.go" \
  "./internal/kernal/governance" \
  "TestVerifyAuditSnapshotSequenceMismatchKillsMutant"

# MUT-06: INV-012 Entry Reversal Identity in posting/entry_validation.go
run_mutant \
  "MUT-06" \
  "INV-012" \
  "internal/kernal/posting/entry_validation.go" \
  "Allow reversal entry to reverse itself (Self-Reversal)" \
  "sed -i 's/\*entry.ReversalOfID == entry.ID/false/' internal/kernal/posting/entry_validation.go" \
  "./internal/kernal/posting" \
  "TestReversalClassificationAndReferenceMustMatch"

# MUT-07: INV-024 Audit Tamper Detection in governance/audit_chain.go
run_mutant \
  "MUT-07" \
  "INV-024" \
  "internal/kernal/governance/audit_chain.go" \
  "Disable hash recalculation during snapshot verification" \
  "sed -i 's/if computedHash := CalculateHash(log); computedHash != log.CurrentLogHash {/if false {/' internal/kernal/governance/audit_chain.go" \
  "./internal/kernal/governance" \
  "TestAuditSnapshotVerificationDetectsTampering"

# MUT-08: INV-016 Period Date Containment in masterdata/fiscal_calendar.go
run_mutant \
  "MUT-08" \
  "INV-016" \
  "internal/kernal/masterdata/fiscal_calendar.go" \
  "Disable transaction date containment check inside period" \
  "python3 -c 'path = \"internal/kernal/masterdata/fiscal_calendar.go\"; c = open(path).read(); open(path, \"w\").write(c.replace(\"!date.Before(p.StartDate.UTC()) && !date.After(p.EndDate.UTC())\", \"true\"))'" \
  "./internal/kernal/masterdata" \
  "TestMasterDataEdgeCases"

echo "" | tee -a "$logfile"
echo "=== Mutation Testing Summary ===" | tee -a "$logfile"
echo "Mutants Tested:   ${mutants_tested}" | tee -a "$logfile"
echo "Mutants Killed:   ${mutants_killed}" | tee -a "$logfile"
echo "Mutants Survived: ${mutants_survived}" | tee -a "$logfile"
score=$(awk -v k="$mutants_killed" -v t="$mutants_tested" 'BEGIN { printf "%.1f", (k / t) * 100 }')
echo "Mutation Score:   ${score}%" | tee -a "$logfile"

if [[ $mutants_survived -eq 0 && $mutants_tested -eq 8 ]]; then
  echo "RESULT: PASS (100% kill rate achieved on critical business rules)" | tee -a "$logfile"
  exit 0
else
  echo "RESULT: FAIL (${mutants_survived} mutants survived)" | tee -a "$logfile"
  exit 1
fi
