package storage

import "context"

// FaultPoint identifies a boundary in an atomic posting operation where a
// contract test may inject a failure. It is deliberately adapter-neutral: no
// driver, transaction handle or table name is exposed.
type FaultPoint string

const (
	// FaultBeforeIdempotencyClaim fires before the durable key is reserved.
	FaultBeforeIdempotencyClaim FaultPoint = "before_idempotency_claim"
	// FaultBeforeCommit fires after all writes are staged but before any of
	// them become visible. Nothing may remain persisted when it fires.
	FaultBeforeCommit FaultPoint = "before_commit"
	// FaultBeforeAudit fires before the required audit effect is staged.
	FaultBeforeAudit FaultPoint = "before_audit"
	// FaultAfterCommit fires after the atomic unit is durable but before the
	// caller receives a response. The client may observe an unknown outcome;
	// replay by idempotency key must return the stored result.
	FaultAfterCommit FaultPoint = "after_commit"
)

// FaultInjector returns an error to abort at a fault point, or nil to continue.
type FaultInjector func(FaultPoint) error

type faultInjectorContextKey struct{}

// WithFaultInjector attaches a test-only injector to a context. Production
// callers never pass one, so the zero-cost lookup simply returns nil.
func WithFaultInjector(ctx context.Context, injector FaultInjector) context.Context {
	if ctx == nil || injector == nil {
		return ctx
	}
	return context.WithValue(ctx, faultInjectorContextKey{}, injector)
}

// InjectFault reports the injected error, if any, for point.
func InjectFault(ctx context.Context, point FaultPoint) error {
	if ctx == nil {
		return nil
	}
	injector, ok := ctx.Value(faultInjectorContextKey{}).(FaultInjector)
	if !ok || injector == nil {
		return nil
	}
	return injector(point)
}
