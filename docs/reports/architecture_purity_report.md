# Architecture Purity Report — `P7-01`

- **Generated (UTC):** 2026-11-21T02:02:11Z
- **Module:** `github.com/osmai4603-collab/accounting_engine`
- **Toolchain:** `go version go1.26.3 linux/amd64`
- **Command:** `./scripts/verify_architecture.sh`
- **Result:** `PASS - clean architectural base` (0 violations, 11 rules checked)
- **Decision record:** [P7_01_public_surface_decision.md](../architecture/KERNEL_ARCHITECTURE_DECISION_RECORDS.md#adr-p7-01)

## Rule results

```text
Architecture verification (module: github.com/osmai4603-collab/accounting_engine)
-------------------------------------------------------------
  [OK]   R1: internal/* free of net/http, grpc and protobuf
  [OK]   R2a: pkg/spi/* free of internal/* imports
  [OK]   R2b: pkg/api/* internal imports limited to internal/kernal/*
  [OK]   R2c: exported API free of unbridged internal/* type exposure
  [OK]   R3a: internal/kernal/* free of internal/orbitals/* imports
  [OK]   R3b: internal/kernal/* free of pkg/* imports
  [OK]   R3c: internal/kernal/* free of concrete storage/* imports
  [OK]   R3d: internal/kernal/* free of database/sql and pgx
  [OK]   R4: no cmd/ or proto/ directories
  [OK]   R5: go.mod free of grpc, protobuf, aws-sdk and net/http
  [OK]   R6: package graph resolves without import cycles
-------------------------------------------------------------
RESULT: PASS - clean architectural base
```

## Policy applied

| Rule | Enforcement |
| :--- | :--- |
| `R2a` | `grep` over `pkg/spi/**/*.go` for any `internal/...` import path. Zero hits. |
| `R2b` | `go list -f '{{range .Imports}}...'` over `./pkg/api/...`; only `internal/kernal/*` accepted. Zero other hits. |
| `R2c` | `go run ./scripts/internalapi -bridge=<go list ./pkg/api/v1/...> <go list ./pkg/sdk/... ./pkg/spi/... ./storage/...>` → 0 exposures. |

`R2c` initially reported 299 exposures because the `source` importer resolves bridge aliases such as `types.Account = kernalMasterdata.Account` into their internal named objects before inspection. The checker therefore builds an allow-list from the exported names of every `pkg/api/v1/...` package and exempts only those named objects. All 299 were attributable to the documented bridge after the refactor of `pkg/spi/masterdata/masterdata.go` to `pkg/api/v1/types`; no raw `internal/*` type remains in a non-bridge exported signature.

## Dependency evidence

`go list -deps ./...` (raw output intentionally summarized):

| Metric | Value |
| :--- | ---: |
| Total dependency packages | 309 |
| Packages whose path contains `/internal/` | 81 |
| Import cycles | 0 (`R6`) |

`go mod graph` (raw output intentionally summarized):

| Metric | Value |
| :--- | ---: |
| Module edges | 43 |
| Direct requirements | 4 (`google/uuid`, `jackc/pgx/v5`, `shopspring/decimal`, `golang.org/x/text`) |
| Indirect requirements | 4 |
| Forbidden deps (`grpc`, `protobuf`, `aws-sdk`, `net/http`) | 0 (`R5`) |

Direct dependency lines from `go mod graph`:

```text
github.com/osmai4603-collab/accounting_engine github.com/google/uuid@v1.6.0
github.com/osmai4603-collab/accounting_engine github.com/jackc/pgpassfile@v1.0.0
github.com/osmai4603-collab/accounting_engine github.com/jackc/pgservicefile@v0.0.0-20240606120523-5a60cdf6a761
github.com/osmai4603-collab/accounting_engine github.com/jackc/pgx/v5@v5.12.0
github.com/osmai4603-collab/accounting_engine github.com/jackc/puddle/v2@v2.2.3
github.com/osmai4603-collab/accounting_engine github.com/shopspring/decimal@v1.6.0
github.com/osmai4603-collab/accounting_engine go@1.26.0
github.com/osmai4603-collab/accounting_engine golang.org/x/sync@v0.24.0
github.com/osmai4603-collab/accounting_engine golang.org/x/text@v0.43.0
github.com/osmai4603-collab/accounting_engine toolchain@go1.26.3
```

## Scope note

This report covers architecture only. Test execution (`go test`, `-race`, benchmarks) is deliberately deferred to `P7-04` per the mandatory ordering in the phase plan. The fix of gap `F-12` (`pkg/sdk/consumer_contract_test.go`) belongs to the owning task and is recorded here only because it was required for the working tree to build consistently; it is not part of the `P7-01` gate evidence.

- **Signed:** Architecture gate owner — `P7-01` — 2026-11-21T02:02:11Z
