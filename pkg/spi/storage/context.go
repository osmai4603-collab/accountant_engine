package storage

import (
	"context"

	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
)

type contextKey string

const (
	companyContextKey contextKey = "storage.company_id"
	actorContextKey   contextKey = "storage.actor_id"
	ledgerContextKey  contextKey = "storage.ledger_id"
)

// DefaultLedgerID is the canonical primary statutory book used when no
// explicit multi-ledger book is attached to the context (ADR P12-01).
const DefaultLedgerID = "primary"

// WithLedgerContext attaches the active ledger/book ID to the context for
// multi-ledger isolation (ADR P12-01).
func WithLedgerContext(ctx context.Context, ledgerID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, ledgerContextKey, ledgerID)
}

// LedgerFromContext extracts the active ledger/book ID from the context,
// returning DefaultLedgerID when none was explicitly attached.
func LedgerFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return DefaultLedgerID, false
	}
	val, ok := ctx.Value(ledgerContextKey).(string)
	if !ok || val == "" {
		return DefaultLedgerID, false
	}
	return val, true
}

// WithCompanyContext attaches the active company ID to the context.
// Storage adapters must extract this value and enforce tenant boundaries
// (e.g. through PostgreSQL RLS or in-memory tenant isolation). Adapters must
// ensure tenant state is cleaned up prior to returning connections to any pool.
func WithCompanyContext(ctx context.Context, companyID types.CompanyID) context.Context {
	return context.WithValue(ctx, companyContextKey, companyID)
}

// CompanyFromContext extracts the active company ID from the context if present.
func CompanyFromContext(ctx context.Context) (types.CompanyID, bool) {
	if ctx == nil {
		return types.CompanyID{}, false
	}
	val, ok := ctx.Value(companyContextKey).(types.CompanyID)
	return val, ok
}

// WithActorContext attaches the authenticated actor ID to the context.
// Adapters may persist this actor ID in audit trails or transaction metadata.
func WithActorContext(ctx context.Context, actorID string) context.Context {
	return context.WithValue(ctx, actorContextKey, actorID)
}

// ActorFromContext extracts the active actor ID from the context if present.
func ActorFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	val, ok := ctx.Value(actorContextKey).(string)
	return val, ok
}
