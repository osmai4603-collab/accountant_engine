package application

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestNoopObserverDiscardsSignals(t *testing.T) {
	observer := NoopObserver{}
	ctx := WithObserverCorrelation(context.Background(), "corr-noop")
	observer.ObservePosting(ctx, "company", time.Millisecond, nil)
	observer.ObserveReversal(ctx, "company", time.Millisecond, errors.New("failed"))
	observer.ObserveStorageTx(ctx, "op", time.Millisecond, nil)
	if got := ObserverCorrelationID(ctx); got != "corr-noop" {
		t.Fatalf("correlation id = %q, want corr-noop", got)
	}
}

func TestObserverCorrelationContext(t *testing.T) {
	if got := ObserverCorrelationID(context.Background()); got != "" {
		t.Fatalf("empty correlation id = %q", got)
	}
	if got := ObserverCorrelationID(nil); got != "" {
		t.Fatalf("nil context correlation id = %q", got)
	}
	if got := ObserverCorrelationID(WithObserverCorrelation(context.Background(), "")); got != "" {
		t.Fatalf("blank correlation id = %q", got)
	}
	var nilContext context.Context
	if got := WithObserverCorrelation(nilContext, "corr"); got != nil {
		t.Fatalf("nil context was replaced: %v", got)
	}
}
