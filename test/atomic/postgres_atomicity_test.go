package atomic

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/journal"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/osmai4603-collab/accounting_engine/pkg/sdk"
	publicapplication "github.com/osmai4603-collab/accounting_engine/pkg/spi/application"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/storage"
	parity "github.com/osmai4603-collab/accounting_engine/test/contract_parity"
	"github.com/shopspring/decimal"
)

// payloadHashIdempotency adapts the public idempotency port for PostgreSQL,
// whose storage contract requires a non-empty payload hash. The SDK currently
// passes an empty hash; this test-side adapter keeps the atomicity scenario
// executable without changing production code. The gap is recorded in the P7
// evidence log as a known integration limitation.
type payloadHashIdempotency struct {
	storage.IdempotencyStore
}

func (payloadHashIdempotency) hash(key string) string { return "p7-atomic:" + key }

func (a payloadHashIdempotency) Claim(ctx context.Context, record storage.IdempotencyRecord) (storage.IdempotencyRecord, bool, error) {
	if record.PayloadHash == "" {
		record.PayloadHash = a.hash(record.Key)
	}
	return a.IdempotencyStore.Claim(ctx, record)
}

func (a payloadHashIdempotency) Complete(ctx context.Context, companyID types.CompanyID, key, payloadHash string, entryID types.JournalEntryID, response []byte) error {
	if payloadHash == "" {
		payloadHash = a.hash(key)
	}
	return a.IdempotencyStore.Complete(ctx, companyID, key, payloadHash, entryID, response)
}

func (a payloadHashIdempotency) Fail(ctx context.Context, companyID types.CompanyID, key, payloadHash, status string) error {
	if payloadHash == "" {
		payloadHash = a.hash(key)
	}
	return a.IdempotencyStore.Fail(ctx, companyID, key, payloadHash, status)
}

// TestAtomicPosting_Postgres_NoPartialWrite runs the atomicity scenario against
// the production adapter in an isolated schema. The schema is created and
// dropped by the shared parity harness; no production-code change is needed.
func TestAtomicPosting_Postgres_NoPartialWrite(t *testing.T) {
	ctx := context.Background()
	fixture := parity.NewUnifiedFixture()
	// Distinct presentation currency avoids the auto-provisioned policy's
	// duplicate-currency validation defect (recorded as P7-04 integration gap).
	fixture.Company.PresentationCurrency = "EUR"
	harness := parity.NewPostgresStorageHarness(t, fixture)
	if harness == nil {
		t.Skip("postgres not available")
	}
	t.Cleanup(harness.Cleanup)

	fault := &faultPorts{sequences: harness.Ports.Sequences, posting: harness.Ports.Posting}
	ports := harness.Ports
	ports.Sequences = fault
	ports.Posting = fault
	ports.Idempotency = payloadHashIdempotency{IdempotencyStore: ports.Idempotency}
	engine, err := sdk.NewBuilder(sdk.WithStoragePorts(ports)).Build(ctx)
	if err != nil {
		t.Fatalf("build engine: %v", err)
	}
	t.Cleanup(func() { _ = engine.Close() })

	amount, err := types.NewMoneyFromString("25.00", "USD")
	if err != nil {
		t.Fatalf("amount: %v", err)
	}
	zero, err := types.NewMoneyFromString("0", "USD")
	if err != nil {
		t.Fatalf("zero: %v", err)
	}
	date := time.Date(2026, time.January, 15, 0, 0, 0, 0, time.UTC)
	entry := journal.JournalEntry{
		ID: types.NewJournalEntryID(), CompanyID: fixture.Company.ID,
		JournalID: types.JournalID(fixture.Journal.ID), FiscalPeriodID: fixture.OpenPeriod.ID,
		Status: journal.EntryDraft, Classification: journal.EntryClassRegular,
		EffectiveDate: date, PostingDate: date,
		Description: "postgres atomic posting", SourceModule: "p7-atomic",
		TransactionCurrency: "USD", ExchangeRate: decimal.NewFromInt(1),
		CreatedBy: "p7-atomic-test",
		Lines: []journal.EntryLine{
			{
				ID: types.NewJournalEntryLineID(), AccountID: fixture.CashAccount.ID,
				TransactionAmount: amount, TransactionCurrency: "USD",
				ExchangeRate: decimal.NewFromInt(1), FunctionalDebit: amount, FunctionalCredit: zero,
			},
			{
				ID: types.NewJournalEntryLineID(), AccountID: fixture.RevenueAccount.ID,
				TransactionAmount: amount, TransactionCurrency: "USD",
				ExchangeRate: decimal.NewFromInt(1), FunctionalDebit: zero, FunctionalCredit: amount,
			},
		},
	}
	postCtx := publicapplication.WithRequestMetadata(ctx, publicapplication.RequestMetadata{
		CorrelationID:  "p7-atomic-postgres",
		IdempotencyKey: "p7-atomic-postgres-" + entry.ID.String(),
		SourceSystem:   "p7-atomic-test",
	})

	fault.failAllocation.Store(true)
	if _, err := engine.Ledger().PostEntry(postCtx, entry); !errors.Is(err, storage.ErrSerializationFailure) {
		t.Fatalf("faulted PostEntry error = %v", err)
	}
	stored, err := engine.Ledger().GetEntry(ctx, fixture.Company.ID, entry.ID)
	if err != nil {
		t.Fatalf("draft not preserved after rollback: %v", err)
	}
	if stored.Status != journal.EntryDraft {
		t.Fatalf("status after rollback = %q, want draft", stored.Status)
	}

	fault.failAllocation.Store(false)
	postCtx = publicapplication.WithRequestMetadata(ctx, publicapplication.RequestMetadata{
		CorrelationID:  "p7-atomic-postgres",
		IdempotencyKey: "p7-atomic-postgres-retry-" + entry.ID.String(),
		SourceSystem:   "p7-atomic-test",
	})
	posted, err := engine.Ledger().PostEntry(postCtx, entry)
	if err != nil {
		t.Fatalf("retry after rollback: %v", err)
	}
	if posted.Status != journal.EntryPosted || posted.SequenceNumber != 1 {
		t.Fatalf("posted = %s sequence=%d, want posted/1", posted.Status, posted.SequenceNumber)
	}
}
