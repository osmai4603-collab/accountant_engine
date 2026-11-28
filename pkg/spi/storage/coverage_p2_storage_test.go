package storage_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/journal"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/storage"
)

func TestP2PortCatalogAndCapabilities(t *testing.T) {
	catalog := storage.PortCatalog()
	if len(catalog) == 0 {
		t.Fatal("expected a non-empty port catalog")
	}
	for i := 1; i < len(catalog); i++ {
		if catalog[i-1].Name >= catalog[i].Name {
			t.Fatalf("catalog not sorted at %d: %q >= %q", i, catalog[i-1].Name, catalog[i].Name)
		}
	}
	catalog[0].Name = "mutated"
	catalog[0].Required = false
	catalog[0].Description = "mutated"
	if again := storage.PortCatalog(); again[0].Name == "mutated" || again[0].Description == "mutated" {
		t.Fatal("PortCatalog did not return a defensive copy")
	}
	requiredCount := 0
	for _, definition := range storage.PortCatalog() {
		if definition.Required {
			requiredCount++
		}
	}
	if requiredCount != 5 {
		t.Fatalf("required ports = %d, want 5", requiredCount)
	}

	if err := storage.ValidateContractVersion(storage.StorageContractVersion); err != nil {
		t.Fatalf("valid contract version rejected: %v", err)
	}
	if err := storage.ValidateContractVersion("storage.spi/v999"); err == nil {
		t.Fatal("expected invalid contract version error")
	}
	validCaps := storage.AdapterCapabilities{
		AdapterName:     "memory",
		AdapterVersion:  "1.0.0",
		ContractVersion: storage.StorageContractVersion,
	}
	if err := storage.ValidateAdapterCapabilities(validCaps); err != nil {
		t.Fatalf("valid capabilities rejected: %v", err)
	}
	for name, caps := range map[string]storage.AdapterCapabilities{
		"version": {AdapterName: "memory", AdapterVersion: "1", ContractVersion: "unknown"},
		"name":    {AdapterVersion: "1", ContractVersion: storage.StorageContractVersion},
		"version field": {
			AdapterName:     "memory",
			ContractVersion: storage.StorageContractVersion,
		},
	} {
		if err := storage.ValidateAdapterCapabilities(caps); err == nil {
			t.Fatalf("%s: expected validation error", name)
		}
	}
}

func TestP2ContextHelpers(t *testing.T) {
	if got, ok := storage.LedgerFromContext(nil); ok || got != storage.DefaultLedgerID {
		t.Fatalf("nil ledger context = %q, %v", got, ok)
	}
	if got, ok := storage.LedgerFromContext(context.Background()); ok || got != storage.DefaultLedgerID {
		t.Fatalf("empty ledger context = %q, %v", got, ok)
	}
	ctx := storage.WithLedgerContext(nil, "secondary")
	if got, ok := storage.LedgerFromContext(ctx); !ok || got != "secondary" {
		t.Fatalf("ledger context = %q, %v", got, ok)
	}
	ctx = storage.WithLedgerContext(context.Background(), "")
	if got, ok := storage.LedgerFromContext(ctx); ok || got != storage.DefaultLedgerID {
		t.Fatalf("empty ledger value = %q, %v", got, ok)
	}
	if got, ok := storage.LedgerFromContext(storage.WithLedgerContext(context.Background(), "books")); !ok || got != "books" {
		t.Fatalf("ledger value = %q, %v", got, ok)
	}

	if got, ok := storage.CompanyFromContext(nil); ok || got != (types.CompanyID{}) {
		t.Fatalf("nil company context = %s, %v", got, ok)
	}
	companyID := types.NewCompanyID()
	if got, ok := storage.CompanyFromContext(storage.WithCompanyContext(context.Background(), companyID)); !ok || got != companyID {
		t.Fatalf("company context = %s, %v", got, ok)
	}
	if got, ok := storage.ActorFromContext(nil); ok || got != "" {
		t.Fatalf("nil actor context = %q, %v", got, ok)
	}
	if got, ok := storage.ActorFromContext(storage.WithActorContext(context.Background(), "clerk")); !ok || got != "clerk" {
		t.Fatalf("actor context = %q, %v", got, ok)
	}
}

func TestP2FaultInjectorHelpers(t *testing.T) {
	if err := storage.InjectFault(nil, storage.FaultBeforeCommit); err != nil {
		t.Fatalf("nil context fault = %v", err)
	}
	if err := storage.InjectFault(context.Background(), storage.FaultBeforeCommit); err != nil {
		t.Fatalf("missing injector fault = %v", err)
	}
	if got := storage.WithFaultInjector(nil, nil); got != nil {
		t.Fatal("WithFaultInjector(nil, nil) must return nil")
	}
	base := context.Background()
	if got := storage.WithFaultInjector(base, nil); got != base {
		t.Fatal("nil injector must not wrap the context")
	}
	want := errors.New("injected")
	ctx := storage.WithFaultInjector(base, func(point storage.FaultPoint) error {
		if point != storage.FaultAfterCommit {
			t.Fatalf("fault point = %q, want %q", point, storage.FaultAfterCommit)
		}
		return want
	})
	if err := storage.InjectFault(ctx, storage.FaultAfterCommit); !errors.Is(err, want) {
		t.Fatalf("injected error = %v, want %v", err, want)
	}
}

func TestP2LockHelpersEdgeCases(t *testing.T) {
	if got := storage.PostingLockTargets(storage.PostingCommit{}); got != nil {
		t.Fatalf("empty company lock targets = %v, want nil", got)
	}
	companyID := types.NewCompanyID()
	targets := storage.PostingLockTargets(storage.PostingCommit{
		Entry: journal.JournalEntry{CompanyID: companyID},
		Idempotency: &storage.PostingIdempotency{
			Key: "   ", Scope: "", PayloadHash: "hash",
		},
	})
	if len(targets) != 0 {
		t.Fatalf("targets for blank idempotency key = %v, want none", targets)
	}

	if got := storage.WithLockObserver(nil, &recordingLockObserver{}); got != nil {
		t.Fatal("nil context must be returned unchanged")
	}
	base := context.Background()
	if got := storage.WithLockObserver(base, nil); got != base {
		t.Fatal("nil observer must not wrap the context")
	}

	target := storage.LockTarget{Kind: storage.LockKindAccount, CompanyID: companyID, ID: "acc"}
	if err := storage.WithOrderedLocks(nil, []storage.LockTarget{target},
		func(context.Context, storage.LockTarget) (func(), error) { return nil, nil },
		func() error { return nil },
	); err == nil {
		t.Fatal("expected error for nil lock context")
	}
	if err := storage.WithOrderedLocks(base, []storage.LockTarget{target}, nil, func() error { return nil }); err == nil {
		t.Fatal("expected error for nil acquirer")
	}
	acquire := func(context.Context, storage.LockTarget) (func(), error) { return func() {}, nil }
	if err := storage.WithOrderedLocks(base, []storage.LockTarget{target}, acquire, nil); err == nil {
		t.Fatal("expected error for nil callback")
	}
	canceled, cancel := context.WithCancel(base)
	cancel()
	if err := storage.WithOrderedLocks(canceled, []storage.LockTarget{target}, acquire, func() error { return nil }); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled context error = %v, want context.Canceled", err)
	}
	invalid := storage.LockTarget{Kind: storage.LockKind("bogus"), CompanyID: companyID, ID: "x"}
	if err := storage.WithOrderedLocks(base, []storage.LockTarget{invalid}, acquire, func() error { return nil }); !errors.Is(err, storage.ErrInvalidLockTarget) {
		t.Fatalf("invalid target error = %v, want ErrInvalidLockTarget", err)
	}
}

func TestP2PolicyAndIdempotencyValidation(t *testing.T) {
	effectiveFrom := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	effectiveTo := effectiveFrom.Add(24 * time.Hour)
	valid := storage.PolicyVersion{
		CompanyID:     types.NewCompanyID(),
		PolicyID:      uuid.New(),
		Kind:          storage.PolicyKindPosting,
		Version:       1,
		EffectiveFrom: effectiveFrom,
		EffectiveTo:   &effectiveTo,
		Payload:       []byte(`{"max_backdating_days":0}`),
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid policy version rejected: %v", err)
	}
	badCompany := valid
	badCompany.CompanyID = types.CompanyID{}
	badKind := valid
	badKind.Kind = "unknown"
	badVersion := valid
	badVersion.Version = 0
	badEffectiveTo := valid
	sameEffectiveTo := effectiveFrom
	badEffectiveTo.EffectiveTo = &sameEffectiveTo
	badPayload := valid
	badPayload.Payload = []byte(`not-json`)
	for name, version := range map[string]storage.PolicyVersion{
		"company":      badCompany,
		"policy id":    func() storage.PolicyVersion { v := valid; v.PolicyID = uuid.Nil; return v }(),
		"kind":         badKind,
		"version":      badVersion,
		"effective to": badEffectiveTo,
		"payload":      badPayload,
	} {
		if err := version.Validate(); err == nil {
			t.Fatalf("%s: expected validation error", name)
		}
	}
	currency := valid
	currency.Kind = storage.PolicyKindCurrency
	if err := currency.Validate(); err != nil {
		t.Fatalf("currency policy rejected: %v", err)
	}

	if err := (storage.PostingIdempotency{Key: "k", Scope: "s", PayloadHash: "h"}).Validate(); err != nil {
		t.Fatalf("valid idempotency rejected: %v", err)
	}
	for _, idem := range []storage.PostingIdempotency{
		{Scope: "s", PayloadHash: "h"},
		{Key: "k", PayloadHash: "h"},
		{Key: "k", Scope: "s"},
	} {
		if err := idem.Validate(); err == nil {
			t.Fatalf("invalid idempotency accepted: %+v", idem)
		}
	}
}

type p2RetryableError struct{ retryable bool }

func (e p2RetryableError) Error() string   { return "p2 retryable" }
func (e p2RetryableError) Retryable() bool { return e.retryable }

type p2UnitOfWorkRunner struct {
	errs   []error
	calls  int
	values []int
}

func (r *p2UnitOfWorkRunner) Execute(context.Context, func(context.Context, storage.UnitOfWork) error) error {
	return nil
}

func (r *p2UnitOfWorkRunner) ExecuteWithOptions(_ context.Context, _ storage.TxOptions, fn func(context.Context, storage.UnitOfWork) error) error {
	r.calls++
	r.values = append(r.values, r.calls)
	if fn != nil {
		_ = fn(context.Background(), nil)
	}
	if r.calls <= len(r.errs) {
		return r.errs[r.calls-1]
	}
	return nil
}

func TestP2RetryPolicyAndExecuteWithRetry(t *testing.T) {
	for name, err := range map[string]error{
		"nil":        nil,
		"foreign":    errors.New("other"),
		"terminal":   storage.ErrUniqueConstraint,
		"nonretry":   p2RetryableError{retryable: false},
		"serial":     storage.ErrSerializationFailure,
		"deadlock":   storage.ErrDeadlockDetected,
		"inprogress": storage.ErrIdempotencyInProgress,
		"concurrent": storage.ErrConcurrentModification,
		"custom":     p2RetryableError{retryable: true},
	} {
		want := false
		switch name {
		case "serial", "deadlock", "inprogress", "concurrent", "custom":
			want = true
		}
		if got := storage.IsRetryable(err); got != want {
			t.Fatalf("IsRetryable(%s) = %v, want %v", name, got, want)
		}
	}

	if err := storage.ExecuteWithRetry(context.Background(), nil, storage.TxOptions{}, storage.DefaultRetryPolicy(), nil); err == nil {
		t.Fatal("expected error for nil runner")
	}
	runner := &p2UnitOfWorkRunner{}
	if err := storage.ExecuteWithRetry(context.Background(), runner, storage.TxOptions{}, storage.RetryPolicy{MaxRetries: -1}, nil); err == nil {
		t.Fatal("expected error for invalid policy")
	}

	runner = &p2UnitOfWorkRunner{errs: []error{storage.ErrSerializationFailure, nil}}
	called := 0
	err := storage.ExecuteWithRetry(context.Background(), runner, storage.TxOptions{Isolation: storage.IsolationSerializable}, storage.RetryPolicy{
		MaxRetries: 2, InitialBackoff: time.Millisecond, MaxBackoff: 2 * time.Millisecond,
	}, func(context.Context, storage.UnitOfWork) error {
		called++
		return nil
	})
	if err != nil || runner.calls != 2 || called != 2 {
		t.Fatalf("retry-then-success: err=%v calls=%d callbacks=%d", err, runner.calls, called)
	}

	terminal := errors.New("terminal failure")
	runner = &p2UnitOfWorkRunner{errs: []error{terminal}}
	if err := storage.ExecuteWithRetry(context.Background(), runner, storage.TxOptions{}, storage.DefaultRetryPolicy(), nil); !errors.Is(err, terminal) {
		t.Fatalf("terminal error = %v, want %v", err, terminal)
	}

	runner = &p2UnitOfWorkRunner{errs: []error{storage.ErrDeadlockDetected, storage.ErrDeadlockDetected}}
	retryable := errors.New("still failing")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := storage.ExecuteWithRetry(ctx, runner, storage.TxOptions{}, storage.RetryPolicy{
		MaxRetries: 2, InitialBackoff: time.Millisecond, MaxBackoff: time.Millisecond,
	}, func(context.Context, storage.UnitOfWork) error { return retryable }); !errors.Is(err, storage.ErrTransactionAborted) {
		t.Fatalf("canceled retry error = %v, want ErrTransactionAborted", err)
	}

	runner = &p2UnitOfWorkRunner{errs: []error{
		storage.ErrConcurrentModification, storage.ErrConcurrentModification, storage.ErrConcurrentModification,
	}}
	err = storage.ExecuteWithRetry(context.Background(), runner, storage.TxOptions{}, storage.RetryPolicy{
		MaxRetries: 2, InitialBackoff: time.Millisecond, MaxBackoff: time.Millisecond,
	}, nil)
	if err == nil || !errors.Is(err, storage.ErrConcurrentModification) {
		t.Fatalf("retry limit error = %v, want wrapped ErrConcurrentModification", err)
	}
}
