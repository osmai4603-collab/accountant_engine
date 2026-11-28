package sdk_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/journal"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/osmai4603-collab/accounting_engine/pkg/sdk"
	publicapplication "github.com/osmai4603-collab/accounting_engine/pkg/spi/application"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/masterdata"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/storage"
	"github.com/osmai4603-collab/accounting_engine/storage/memory"
	"github.com/shopspring/decimal"
)

type observerSignal struct {
	kind          string
	companyID     string
	correlationID string
	duration      time.Duration
	err           error
}

type recordingObserver struct {
	mu      sync.Mutex
	signals []observerSignal
}

func (o *recordingObserver) ObservePosting(ctx context.Context, companyID string, duration time.Duration, err error) {
	o.record("post", ctx, companyID, duration, err)
}

func (o *recordingObserver) ObserveReversal(ctx context.Context, companyID string, duration time.Duration, err error) {
	o.record("reversal", ctx, companyID, duration, err)
}

func (o *recordingObserver) ObserveStorageTx(ctx context.Context, opName string, duration time.Duration, err error) {
	o.record(opName, ctx, "", duration, err)
}

func (o *recordingObserver) record(kind string, ctx context.Context, companyID string, duration time.Duration, err error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.signals = append(o.signals, observerSignal{
		kind:          kind,
		companyID:     companyID,
		correlationID: publicapplication.ObserverCorrelationID(ctx),
		duration:      duration,
		err:           err,
	})
}

func (o *recordingObserver) snapshot() []observerSignal {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]observerSignal(nil), o.signals...)
}

type failingPostingWriter struct{ err error }

func (w failingPostingWriter) CommitPosting(context.Context, storage.PostingCommit) (journal.JournalEntry, error) {
	return journal.JournalEntry{}, w.err
}

func observerTestFixture(t *testing.T) (*memory.Adapter, storage.Ports, journal.JournalEntry, types.CompanyID) {
	t.Helper()
	ctx := context.Background()
	adapter := memory.New()
	ports := adapter.Ports()
	companyID := types.NewCompanyID()
	journalID := masterdata.NewJournalID()
	cashID := types.NewAccountID()
	revenueID := types.NewAccountID()
	fyID := types.FiscalYearID(uuid.New())
	periodID := types.FiscalPeriodID(uuid.New())
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 12, 31, 23, 59, 59, 0, time.UTC)

	must(t, adapter.MasterDataStore().CreateCompany(ctx, masterdata.Company{
		ID: companyID, Code: "OBS-1", LegalName: "Observer Co", FunctionalCurrency: "USD", PresentationCurrency: "USD", Active: true,
	}))
	must(t, adapter.MasterDataStore().CreateJournal(ctx, masterdata.Journal{ID: journalID, CompanyID: companyID, Code: "GEN", Name: "General"}))
	must(t, adapter.MasterDataStore().CreateAccount(ctx, masterdata.Account{ID: cashID, CompanyID: companyID, Code: "1000", Name: "Cash", Type: masterdata.AccountTypeAsset, NormalBalance: masterdata.NormalBalanceDebit, Currency: "USD", IsActive: true}))
	must(t, adapter.MasterDataStore().CreateAccount(ctx, masterdata.Account{ID: revenueID, CompanyID: companyID, Code: "4000", Name: "Revenue", Type: masterdata.AccountTypeIncome, NormalBalance: masterdata.NormalBalanceCredit, Currency: "USD", IsActive: true}))
	must(t, adapter.MasterDataStore().CreateFiscalYear(ctx, masterdata.FiscalYear{ID: fyID, CompanyID: companyID, Name: "FY26", StartDate: start, EndDate: end, State: masterdata.FiscalYearOpen}))
	must(t, adapter.MasterDataStore().CreateFiscalPeriod(ctx, masterdata.FiscalPeriod{ID: periodID, CompanyID: companyID, FiscalYearID: fyID, Number: 1, Name: "P1", StartDate: start, EndDate: end, State: masterdata.PeriodOpen, Type: masterdata.FiscalPeriodTypeNormal}))

	amount, err := types.NewMoneyFromString("100.00", "USD")
	if err != nil {
		t.Fatalf("amount: %v", err)
	}
	zero, err := types.NewMoneyFromString("0", "USD")
	if err != nil {
		t.Fatalf("zero: %v", err)
	}
	entry := journal.JournalEntry{
		ID:                  types.NewJournalEntryID(),
		CompanyID:           companyID,
		JournalID:           journalID,
		FiscalPeriodID:      periodID,
		EffectiveDate:       start.AddDate(0, 0, 2),
		PostingDate:         start.AddDate(0, 0, 2),
		TransactionCurrency: "USD",
		ExchangeRate:        decimal.NewFromInt(1),
		Lines: []journal.EntryLine{
			{ID: types.NewJournalEntryLineID(), AccountID: cashID, TransactionAmount: amount, TransactionCurrency: "USD", ExchangeRate: decimal.NewFromInt(1), FunctionalDebit: amount, FunctionalCredit: zero},
			{ID: types.NewJournalEntryLineID(), AccountID: revenueID, TransactionAmount: amount, TransactionCurrency: "USD", ExchangeRate: decimal.NewFromInt(1), FunctionalDebit: zero, FunctionalCredit: amount},
		},
	}
	return adapter, ports, entry, companyID
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("provisioning: %v", err)
	}
}

func TestObserverReceivesPostingReversalAndFailures(t *testing.T) {
	ctx := context.Background()
	_, ports, entry, companyID := observerTestFixture(t)
	observer := &recordingObserver{}

	engine, err := sdk.NewBuilder(sdk.WithStoragePorts(ports), sdk.WithObserver(observer)).Build(ctx)
	if err != nil {
		t.Fatalf("build engine: %v", err)
	}
	defer engine.Close()

	postCtx := publicapplication.WithRequestMetadata(ctx, publicapplication.RequestMetadata{
		IdempotencyKey: "obs-post-1",
		CorrelationID:  "corr-obs-post",
	})
	posted, err := engine.Ledger().PostEntry(postCtx, entry)
	if err != nil {
		t.Fatalf("PostEntry: %v", err)
	}
	reverseCtx := publicapplication.WithRequestMetadata(ctx, publicapplication.RequestMetadata{
		IdempotencyKey: "obs-reverse-1",
		CorrelationID:  "corr-obs-reverse",
	})
	if _, err := engine.Ledger().ReverseEntry(reverseCtx, companyID, posted.ID, "observer test"); err != nil {
		t.Fatalf("ReverseEntry: %v", err)
	}

	signals := observer.snapshot()
	if len(signals) != 2 {
		t.Fatalf("signals = %d, want 2: %+v", len(signals), signals)
	}
	post, reversal := signals[0], signals[1]
	if post.kind != "post" || post.companyID != companyID.String() || post.correlationID != "corr-obs-post" {
		t.Fatalf("posting signal = %+v", post)
	}
	if post.err != nil || post.duration <= 0 {
		t.Fatalf("posting signal outcome = %+v", post)
	}
	if reversal.kind != "reversal" || reversal.companyID != companyID.String() || reversal.correlationID != "corr-obs-reverse" {
		t.Fatalf("reversal signal = %+v", reversal)
	}
	if reversal.err != nil || reversal.duration <= 0 {
		t.Fatalf("reversal signal outcome = %+v", reversal)
	}

	// A storage failure must be reported with a non-nil error.
	failure := errors.New("commit rejected")
	failingPorts := ports
	failingPorts.Posting = failingPostingWriter{err: failure}
	failingEngine, err := sdk.NewBuilder(sdk.WithStoragePorts(failingPorts), sdk.WithObserver(observer)).Build(ctx)
	if err != nil {
		t.Fatalf("build failing engine: %v", err)
	}
	defer failingEngine.Close()
	failureCtx := publicapplication.WithRequestMetadata(ctx, publicapplication.RequestMetadata{
		IdempotencyKey: "obs-post-failure",
		CorrelationID:  "corr-obs-failure",
	})
	failedEntry := entry
	failedEntry.ID = types.NewJournalEntryID()
	if _, err := failingEngine.Ledger().PostEntry(failureCtx, failedEntry); err == nil {
		t.Fatal("expected posting failure")
	}

	signals = observer.snapshot()
	if len(signals) != 3 {
		t.Fatalf("signals after failure = %d, want 3: %+v", len(signals), signals)
	}
	if signals[2].kind != "post" || signals[2].correlationID != "corr-obs-failure" || !errors.Is(signals[2].err, failure) {
		t.Fatalf("failed posting signal = %+v", signals[2])
	}
}

func TestWithObserverRejectsNil(t *testing.T) {
	if _, err := sdk.NewBuilder(sdk.WithObserver(nil)).Build(context.Background()); err == nil {
		t.Fatal("expected nil observer error")
	}
	var typedNil *recordingObserver
	if _, err := sdk.NewBuilder(sdk.WithObserver(typedNil)).Build(context.Background()); err == nil {
		t.Fatal("expected typed-nil observer error")
	}
}
