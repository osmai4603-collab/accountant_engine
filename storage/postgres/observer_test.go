package postgres_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	publicapplication "github.com/osmai4603-collab/accounting_engine/pkg/spi/application"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/storage"
)

type txObservation struct {
	opName   string
	duration time.Duration
	err      error
}

type txObserver struct {
	mu           sync.Mutex
	observations []txObservation
}

func (o *txObserver) ObservePosting(context.Context, string, time.Duration, error)  {}
func (o *txObserver) ObserveReversal(context.Context, string, time.Duration, error) {}
func (o *txObserver) ObserveStorageTx(_ context.Context, opName string, duration time.Duration, err error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.observations = append(o.observations, txObservation{opName: opName, duration: duration, err: err})
}

func (o *txObserver) snapshot() []txObservation {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]txObservation(nil), o.observations...)
}

func TestDriverObserverReportsTransactionOutcomes(t *testing.T) {
	ctx := context.Background()
	driver, err := newIsolatedPostgresDriver(t, ctx)
	if err != nil {
		t.Skipf("PostgreSQL not available for observer test: %v", err)
	}
	observer := &txObserver{}
	driver.SetObserver(observer)
	if driver.Observer() != publicapplication.Observer(observer) {
		t.Fatal("SetObserver did not install the observer")
	}

	companyCtx := storage.WithCompanyContext(ctx, types.NewCompanyID())
	if err := driver.Execute(companyCtx, func(context.Context, storage.UnitOfWork) error {
		return nil
	}); err != nil {
		t.Fatalf("successful transaction: %v", err)
	}
	rejected := errors.New("reject transaction")
	if err := driver.Execute(companyCtx, func(context.Context, storage.UnitOfWork) error {
		return rejected
	}); !errors.Is(err, rejected) {
		t.Fatalf("failed transaction error = %v, want %v", err, rejected)
	}

	observations := observer.snapshot()
	if len(observations) != 2 {
		t.Fatalf("observations = %d, want 2: %+v", len(observations), observations)
	}
	for i, observation := range observations {
		if observation.opName != "postgres.unit_of_work" {
			t.Fatalf("observation %d opName = %q", i, observation.opName)
		}
		if observation.duration <= 0 {
			t.Fatalf("observation %d duration = %v", i, observation.duration)
		}
	}
	if observations[0].err != nil {
		t.Fatalf("successful transaction reported error: %v", observations[0].err)
	}
	if !errors.Is(observations[1].err, rejected) {
		t.Fatalf("failed transaction error = %v, want %v", observations[1].err, rejected)
	}

	driver.SetObserver(nil)
	if _, ok := driver.Observer().(publicapplication.NoopObserver); !ok {
		t.Fatalf("nil observer did not restore no-op, got %T", driver.Observer())
	}
}
