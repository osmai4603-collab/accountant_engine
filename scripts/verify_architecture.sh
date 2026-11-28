#!/usr/bin/env bash
# Architecture guard for the embeddable accounting kernel.
#
# Enforces the import boundaries declared in AE-PLAN-STEP-P0 (P0-03),
# NEW_PROJECT_STRUCTURE.md section 5 and ARCHITECTURAL_PRINCIPLES_CHARTER.md:
#
#   1. internal/* never imports grpc, protobuf or net/http.
#   2. pkg/api/* and pkg/spi/* may depend on internal/kernel/* contracts.
#   3. internal/kernel/* never imports internal/orbitals/* or storage/*
#      (and never imports database/sql or pgx).
#   4. The repository contains no cmd/ or proto/ directories.
#   5. go.mod contains no network/cloud dependencies.
#   6. The package graph has no import cycles (go list -deps sanity check).
#
# Usage: ./scripts/verify_architecture.sh
# Exit code 0 = clean base, 1 = at least one violation.
set -uo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

module_path="$(awk '/^module /{print $2; exit}' go.mod)"
failures=0

report_failure() {
  printf '  [FAIL] %s\n' "$1"
  shift
  if [[ $# -gt 0 ]]; then
    printf '%s\n' "$1" | sed 's/^/         /'
  fi
  failures=$((failures + 1))
}

report_pass() {
  printf '  [OK]   %s\n' "$1"
}

# grep only real import lines: optional alias, optional blank identifier,
# then a quoted import path. Comments in doc.go are deliberately ignored.
import_pattern() {
  printf '^[[:space:]]*([[:alnum:]_]+[[:space:]]+)?(_[[:space:]]+)?"%s"' "$1"
}

scan_imports() {
  local label="$1" pattern="$2"
  shift 2
  local hits
  hits="$(grep -RInE --include='*.go' "$pattern" "$@" 2>/dev/null || true)"
  if [[ -n "$hits" ]]; then
    report_failure "$label" "$hits"
  else
    report_pass "$label"
  fi
}

echo "Architecture verification (module: ${module_path})"
echo "-------------------------------------------------------------"

# Rule 1: transport independence of every internal package.
network_pattern="$(import_pattern '(net/http(/[^"]*)?|google\.golang\.org/grpc(/[^"]*)?|google\.golang\.org/protobuf(/[^"]*)?)')"
scan_imports "R1: internal/* free of net/http, grpc and protobuf" "$network_pattern" internal/

# Rule 2: the sanctioned public bridge is pkg/api/v1 only (ADR P7-01).
# R2a: pkg/spi/* must never import internal/* directly.
internal_any_pattern="$(import_pattern '([^"]*/)?internal(/[^"]*)?')"
scan_imports "R2a: pkg/spi/* free of internal/* imports" "$internal_any_pattern" pkg/spi/

# R2b: pkg/api/* may import internal/kernel/* only, through the bridge.
non_kernel_imports="$(go list -f '{{range .Imports}}{{$.ImportPath}} {{.}}{{"\n"}}{{end}}' ./pkg/api/... 2>/dev/null | awk '$2 ~ /(^|\/)internal(\/|$)/ && $2 !~ /\/internal\/kernel(\/|$)/ {print}' || true)"
if [[ -n "$non_kernel_imports" ]]; then
  report_failure "R2b: pkg/api/* internal imports limited to internal/kernel/*" "$non_kernel_imports"
else
  report_pass "R2b: pkg/api/* internal imports limited to internal/kernel/*"
fi

# R2c: exported signatures must not expose internal types unless they arrive
# through the documented pkg/api/v1 re-export bridge (ADR P7-01). The bridge
# packages are checked by R2b; R2c verifies every other public package.
bridge_packages="$(go list ./pkg/api/v1/... 2>/dev/null | paste -sd, - || true)"
public_packages=()
while IFS= read -r package; do
  [[ -n "$package" ]] && public_packages+=("$package")
done < <(go list ./pkg/sdk/... ./pkg/spi/... ./storage/... 2>/dev/null || true)
internalapi_output="$(go run ./scripts/internalapi "-bridge=${bridge_packages}" "${public_packages[@]}" 2>&1)"
if [[ $? -eq 0 ]]; then
  report_pass "R2c: exported API free of unbridged internal/* type exposure"
else
  report_failure "R2c: exported API free of unbridged internal/* type exposure" "$internalapi_output"
fi

# Rule 3: the kernel is independent of public contracts, orbitals and storage.
orbit_pattern="$(import_pattern '([^"]*/)?internal/orbitals(/[^"]*)?')"
scan_imports "R3a: internal/kernel/* free of internal/orbitals/* imports" "$orbit_pattern" internal/kernel/

public_pattern="$(import_pattern '([^"]*/)?pkg(/[^\"]*)?')"
scan_imports "R3b: internal/kernel/* free of pkg/* imports" "$public_pattern" internal/kernel/

storage_adapter_pattern="$(import_pattern "${module_path}/storage(/[^\"]*)?")"
scan_imports "R3c: internal/kernel/* free of concrete storage/* imports" "$storage_adapter_pattern" internal/kernel/

sql_pattern="$(import_pattern '(database/sql|github\.com/jackc/pgx(/[^\"]*)?)')"
scan_imports "R3d: internal/kernel/* free of database/sql and pgx" "$sql_pattern" internal/kernel/

internal_pattern="$(import_pattern "${module_path}/internal(/[^\"]*)?")"
scan_imports "R3e: storage/* free of internal/* imports" "$internal_pattern" storage/

# Rule 4: pruned components must stay pruned.
forbidden_dirs="$(find . -type d \( -name cmd -o -name proto \) -print 2>/dev/null || true)"
if [[ -n "$forbidden_dirs" ]]; then
  report_failure "R4: no cmd/ or proto/ directories" "$forbidden_dirs"
else
  report_pass "R4: no cmd/ or proto/ directories"
fi

# Rule 5: lean go.mod (checked literally, not only in Go sources).
if grep -nE 'google\.golang\.org/(grpc|protobuf)|github\.com/aws/aws-sdk-go-v2|net/http' go.mod >/dev/null 2>&1; then
  report_failure "R5: go.mod free of grpc, protobuf, aws-sdk and net/http" \
    "$(grep -nE 'google\.golang\.org/(grpc|protobuf)|github\.com/aws/aws-sdk-go-v2|net/http' go.mod)"
else
  report_pass "R5: go.mod free of grpc, protobuf, aws-sdk and net/http"
fi

# Rule 6: go list resolves the whole module; import cycles surface as errors.
if go list -deps ./... >/dev/null 2>&1; then
  report_pass "R6: package graph resolves without import cycles"
else
  report_failure "R6: package graph resolves without import cycles" \
    "$(go list -deps ./... 2>&1)"
fi

echo "-------------------------------------------------------------"
if [[ "$failures" -ne 0 ]]; then
  printf 'RESULT: FAILED (%d violation(s))\n' "$failures"
  exit 1
fi

echo "RESULT: PASS - clean architectural base"
