//go:build tools

// Package tools pins the four essential direct dependencies of the embeddable
// accounting kernel until the first production packages land in P1.
//
// The file is excluded from normal builds by the "tools" build tag, so it adds
// no runtime code to the library. It exists so that `go mod tidy` is idempotent
// at this bootstrap stage: without any imports in the module, tidy would drop
// every require directive from go.mod.
//
// Dependency scope (AE-PLAN-STEP-P0 / P0-01):
//   - github.com/google/uuid      : identifier generation
//   - github.com/shopspring/decimal: exact decimal money arithmetic (no floats)
//   - github.com/jackc/pgx/v5     : PostgreSQL adapter only (storage/postgres)
//   - golang.org/x/text           : ISO 4217 / international text codes
//
// Deliberately absent: google.golang.org/grpc, google.golang.org/protobuf,
// github.com/aws/aws-sdk-go-v2, and any HTTP server stack.
package tools

import (
	_ "github.com/google/uuid"
	_ "github.com/jackc/pgx/v5"
	_ "github.com/shopspring/decimal"
	_ "golang.org/x/text/currency"
)
