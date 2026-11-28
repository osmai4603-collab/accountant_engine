package application

import (
	"context"
	"time"
)

// Observer is a zero-dependency operational hook. Hosts may bridge it to a
// metrics backend, a tracer, or a structured logger without adding any
// dependency to the engine module. Implementations must be safe for
// concurrent use; all hooks are optional and are invoked with a context that
// carries the request correlation id when one is available.
type Observer interface {
	// ObservePosting reports the outcome and latency of a journal posting.
	ObservePosting(ctx context.Context, companyID string, duration time.Duration, err error)
	// ObserveReversal reports the outcome and latency of a journal reversal.
	ObserveReversal(ctx context.Context, companyID string, duration time.Duration, err error)
	// ObserveStorageTx reports the outcome and latency of a storage transaction.
	ObserveStorageTx(ctx context.Context, opName string, duration time.Duration, err error)
}

// NoopObserver is the default Observer. It discards every signal and is safe
// to share between goroutines.
type NoopObserver struct{}

// ObservePosting implements Observer.
func (NoopObserver) ObservePosting(context.Context, string, time.Duration, error) {}

// ObserveReversal implements Observer.
func (NoopObserver) ObserveReversal(context.Context, string, time.Duration, error) {}

// ObserveStorageTx implements Observer.
func (NoopObserver) ObserveStorageTx(context.Context, string, time.Duration, error) {}

var _ Observer = NoopObserver{}

type observerCorrelationKey struct{}

// WithObserverCorrelation attaches a correlation id to the context passed to
// Observer hooks. An empty id leaves the context unchanged.
func WithObserverCorrelation(ctx context.Context, correlationID string) context.Context {
	if ctx == nil || correlationID == "" {
		return ctx
	}
	return context.WithValue(ctx, observerCorrelationKey{}, correlationID)
}

// ObserverCorrelationID returns the correlation id carried by ctx, if any.
func ObserverCorrelationID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	correlationID, _ := ctx.Value(observerCorrelationKey{}).(string)
	return correlationID
}
