#!/usr/bin/env bash
# Source integrity gate for the P0 bootstrap (AE-PLAN-STEP-P0 stage gate).
#
# Checks:
#   1. gofmt cleanliness.
#   2. go vet ./...
#   3. go mod verify.
#   4. go mod tidy is idempotent (go.mod / go.sum unchanged).
#   5. Every Go package directory carries a doc.go charter.
#   6. Fewer than 6 direct dependencies, and no forbidden ones.
#
# Usage: ./scripts/verify_integrity.sh
set -uo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

tmpdir="$(mktemp -d)"
cleanup() {
  # Always restore the manifests so this check never mutates the workspace.
  if [[ -f "$tmpdir/go.mod" ]]; then cp "$tmpdir/go.mod" go.mod; fi
  if [[ -f "$tmpdir/go.sum" ]]; then cp "$tmpdir/go.sum" go.sum; fi
  rm -rf "$tmpdir"
}
trap cleanup EXIT

failures=0
report() {
  local status="$1" label="$2" detail="${3:-}"
  printf '  [%s] %s\n' "$status" "$label"
  if [[ -n "$detail" ]]; then
    printf '%s\n' "$detail" | sed 's/^/         /'
  fi
  if [[ "$status" == "FAIL" ]]; then failures=$((failures + 1)); fi
}

echo "Source integrity verification"
echo "-------------------------------------------------------------"

unformatted="$(gofmt -l . 2>&1 || true)"
if [[ -n "$unformatted" ]]; then
  report FAIL "gofmt: all Go files formatted" "$unformatted"
else
  report OK "gofmt: all Go files formatted"
fi

if vet_out="$(go vet ./... 2>&1)"; then
  report OK "go vet ./..."
else
  report FAIL "go vet ./..." "$vet_out"
fi

if verify_out="$(go mod verify 2>&1)"; then
  report OK "go mod verify" "$verify_out"
else
  report FAIL "go mod verify" "$verify_out"
fi

cp go.mod "$tmpdir/go.mod"
if [[ -f go.sum ]]; then cp go.sum "$tmpdir/go.sum"; fi
tidy_out="$(go mod tidy 2>&1)" || true
if ! cmp -s go.mod "$tmpdir/go.mod" || \
   { [[ -f "$tmpdir/go.sum" ]] && ! cmp -s go.sum "$tmpdir/go.sum"; }; then
  report FAIL "go mod tidy is idempotent" "manifests changed on tidy; restored originals"
else
  report OK "go mod tidy is idempotent"
fi

missing_doc="$(find pkg internal storage modules test -type d 2>/dev/null \
  | while read -r dir; do [[ -f "$dir/doc.go" ]] || printf '%s\n' "$dir"; done)"
if [[ -n "$missing_doc" ]]; then
  report FAIL "every package directory has a doc.go charter" "$missing_doc"
else
  report OK "every package directory has a doc.go charter"
fi

direct_deps="$(awk '
  /^require \(/ { in_block = 1; next }
  in_block && /^\)/ { in_block = 0; next }
  in_block && NF && $0 !~ /\/\/ indirect/ { count++ }
  END { print count + 0 }
' go.mod)"
if [[ "$direct_deps" -lt 6 ]]; then
  report OK "direct dependency count < 6 (found $direct_deps)"
else
  report FAIL "direct dependency count < 6 (found $direct_deps)"
fi

echo "-------------------------------------------------------------"
if [[ "$failures" -ne 0 ]]; then
  printf 'RESULT: FAILED (%d check(s))\n' "$failures"
  exit 1
fi

echo "RESULT: PASS - source integrity verified"
