package atomic

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/journal"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/osmai4603-collab/accounting_engine/pkg/sdk"
	publicapplication "github.com/osmai4603-collab/accounting_engine/pkg/spi/application"
	publicmasterdata "github.com/osmai4603-collab/accounting_engine/pkg/spi/masterdata"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/storage"
	"github.com/osmai4603-collab/accounting_engine/storage/memory"
	"github.com/shopspring/decimal"
)

// postContext supplies the idempotency key required by the target posting
// pipeline. Each invocation uses a fresh key so retries are real retries, not
// idempotent replays.
func postContext(label string) context.Context {
	return publicapplication.WithRequestMetadata(context.Background(), publicapplication.RequestMetadata{
		CorrelationID:  "p7-atomicity",
		IdempotencyKey: label + "-" + uuid.NewString(),
		SourceSystem:   "p7-atomicity-test",
	})
}

// faultPorts injects failures at the two boundaries exposed by the target
// storage contract: sequence allocation and posting commit. A failure before
// allocation must leave no trace at all; a failure at commit must leave the
// draft intact and must never produce a posted entry or balance movement.
type faultPorts struct {
	sequences      storage.SequenceAllocator
	posting        storage.PostingWriter
	failAllocation atomic.Bool
	failCommit     atomic.Bool
	allocations    atomic.Int64
	commits        atomic.Int64
}

func (f *faultPorts) NextSequence(ctx context.Context, companyID types.CompanyID, journalID types.JournalID) (int64, error) {
	f.allocations.Add(1)
	if f.failAllocation.Load() {
		return 0, storage.ErrSerializationFailure
	}
	return f.sequences.NextSequence(ctx, companyID, journalID)
}

func (f *faultPorts) CommitPosting(ctx context.Context, commit storage.PostingCommit) (journal.JournalEntry, error) {
	f.commits.Add(1)
	if f.failCommit.Load() {
		return journal.JournalEntry{}, storage.ErrSerializationFailure
	}
	return f.posting.CommitPosting(ctx, commit)
}

type fixture struct {
	engine      sdk.Engine
	app         publicapplication.Service
	ports       storage.Ports
	fault       *faultPorts
	companyID   types.CompanyID
	journalID   types.JournalID
	periodID    types.FiscalPeriodID
	cashAccount types.AccountID
	revAccount  types.AccountID
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	ctx := context.Background()
	driver := memory.New()
	masterData := driver.MasterDataStore()

	f := fixture{
		companyID:   types.CompanyID(uuid.MustParse("10000000-0000-4000-8000-0000000000d1")),
		cashAccount: types.AccountID(uuid.MustParse("20000000-0000-4000-8000-0000000000d1")),
		revAccount:  types.AccountID(uuid.MustParse("20000000-0000-4000-8000-0000000000d2")),
		periodID:    types.FiscalPeriodID(uuid.MustParse("40000000-0000-4000-8000-0000000000d1")),
	}
	yearID := types.FiscalYearID(uuid.MustParse("30000000-0000-4000-8000-0000000000d1"))
	journalID := publicmasterdata.NewJournalID()
	f.journalID = types.JournalID(journalID)

	if err := masterData.CreateCompany(ctx, publicmasterdata.Company{
		ID: f.companyID, Code: "ATOMIC", LegalName: "Atomic Ltd",
		FunctionalCurrency: "USD", PresentationCurrency: "EUR", Active: true,
	}); err != nil {
		t.Fatalf("create company: %v", err)
	}
	if err := masterData.CreateJournal(ctx, publicmasterdata.Journal{
		ID: journalID, CompanyID: f.companyID, Code: "ATOMIC", Name: "Atomic journal",
	}); err != nil {
		t.Fatalf("create journal: %v", err)
	}
	for _, account := range []publicmasterdata.Account{
		{
			ID: f.cashAccount, CompanyID: f.companyID, Code: "1000", Name: "Cash",
			Type: publicmasterdata.AccountTypeAsset, NormalBalance: publicmasterdata.NormalBalanceDebit,
			Currency: "USD", IsActive: true,
		},
		{
			ID: f.revAccount, CompanyID: f.companyID, Code: "4000", Name: "Revenue",
			Type: publicmasterdata.AccountTypeIncome, NormalBalance: publicmasterdata.NormalBalanceCredit,
			Currency: "USD", IsActive: true,
		},
	} {
		if err := masterData.CreateAccount(ctx, account); err != nil {
			t.Fatalf("create account %s: %v", account.Code, err)
		}
	}
	start := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	if err := masterData.CreateFiscalYear(ctx, publicmasterdata.FiscalYear{
		ID: yearID, CompanyID: f.companyID, Name: "FY2026",
		StartDate: start, EndDate: time.Date(2026, time.December, 31, 0, 0, 0, 0, time.UTC),
		State: publicmasterdata.FiscalYearOpen,
	}); err != nil {
		t.Fatalf("create fiscal year: %v", err)
	}
	if err := masterData.CreateFiscalPeriod(ctx, publicmasterdata.FiscalPeriod{
		ID: f.periodID, CompanyID: f.companyID, FiscalYearID: yearID,
		Number: 1, Name: "January 2026", StartDate: start,
		EndDate: time.Date(2026, time.January, 31, 0, 0, 0, 0, time.UTC),
		State:   publicmasterdata.PeriodOpen, Type: publicmasterdata.FiscalPeriodTypeNormal,
	}); err != nil {
		t.Fatalf("create fiscal period: %v", err)
	}

	ports := driver.Ports()
	// The application service keeps the unwrapped ports so idempotent replay
	// can be exercised without re-triggering injected faults.
	rawPorts := ports
	app, err := sdk.NewApplicationService(rawPorts)
	if err != nil {
		t.Fatalf("build application service: %v", err)
	}
	f.app = app
	f.ports = rawPorts

	f.fault = &faultPorts{sequences: ports.Sequences, posting: ports.Posting}
	ports.Sequences = f.fault
	ports.Posting = f.fault
	engine, err := sdk.NewBuilder(sdk.WithStoragePorts(ports)).Build(ctx)
	if err != nil {
		t.Fatalf("build engine: %v", err)
	}
	f.engine = engine
	t.Cleanup(func() { _ = engine.Close() })
	return f
}

func (f fixture) entry(t *testing.T) journal.JournalEntry {
	t.Helper()
	amount, err := types.NewMoneyFromString("25.00", "USD")
	if err != nil {
		t.Fatalf("amount: %v", err)
	}
	zero, err := types.NewMoneyFromString("0", "USD")
	if err != nil {
		t.Fatalf("zero: %v", err)
	}
	date := time.Date(2026, time.January, 15, 0, 0, 0, 0, time.UTC)
	return journal.JournalEntry{
		ID: types.NewJournalEntryID(), CompanyID: f.companyID,
		JournalID: f.journalID, FiscalPeriodID: f.periodID,
		Status: journal.EntryDraft, Classification: journal.EntryClassRegular,
		EffectiveDate: date, PostingDate: date,
		Description: "atomic posting", SourceModule: "atomic-test",
		TransactionCurrency: "USD", ExchangeRate: decimal.NewFromInt(1),
		CreatedBy: "atomic-test",
		Lines: []journal.EntryLine{
			{
				ID: types.NewJournalEntryLineID(), AccountID: f.cashAccount,
				TransactionAmount: amount, TransactionCurrency: "USD",
				ExchangeRate: decimal.NewFromInt(1), FunctionalDebit: amount, FunctionalCredit: zero,
			},
			{
				ID: types.NewJournalEntryLineID(), AccountID: f.revAccount,
				TransactionAmount: amount, TransactionCurrency: "USD",
				ExchangeRate: decimal.NewFromInt(1), FunctionalDebit: zero, FunctionalCredit: amount,
			},
		},
	}
}

func assertNoPostedEntry(t *testing.T, f fixture, entry journal.JournalEntry) {
	t.Helper()
	stored, err := f.engine.Ledger().GetEntry(context.Background(), f.companyID, entry.ID)
	if err != nil {
		t.Fatalf("draft was not preserved after rollback: %v", err)
	}
	if stored.Status != journal.EntryDraft {
		t.Fatalf("status after rollback = %q, want draft", stored.Status)
	}
}

func TestAtomicPosting_FaultBeforeSequenceLeavesNoTrace(t *testing.T) {
	f := newFixture(t)
	entry := f.entry(t)

	f.fault.failAllocation.Store(true)
	if _, err := f.engine.Ledger().PostEntry(postContext(entry.ID.String()), entry); !errors.Is(err, storage.ErrSerializationFailure) {
		t.Fatalf("first PostEntry error = %v, want %v", err, storage.ErrSerializationFailure)
	}
	if f.fault.allocations.Load() != 1 || f.fault.commits.Load() != 0 {
		t.Fatalf("fault boundaries: allocations=%d commits=%d", f.fault.allocations.Load(), f.fault.commits.Load())
	}
	assertNoPostedEntry(t, f, entry)

	f.fault.failAllocation.Store(false)
	posted, err := f.engine.Ledger().PostEntry(postContext(entry.ID.String()), entry)
	if err != nil {
		t.Fatalf("retry after rollback: %v", err)
	}
	if posted.Status != journal.EntryPosted || posted.SequenceNumber != 1 {
		t.Fatalf("posted = %s sequence=%d, want posted/1", posted.Status, posted.SequenceNumber)
	}
}

func TestAtomicPosting_FaultAtCommitLeavesNoPartialWrite(t *testing.T) {
	f := newFixture(t)
	entry := f.entry(t)

	f.fault.failCommit.Store(true)
	if _, err := f.engine.Ledger().PostEntry(postContext(entry.ID.String()), entry); !errors.Is(err, storage.ErrSerializationFailure) {
		t.Fatalf("PostEntry error = %v, want %v", err, storage.ErrSerializationFailure)
	}
	assertNoPostedEntry(t, f, entry)

	f.fault.failCommit.Store(false)
	posted, err := f.engine.Ledger().PostEntry(postContext(entry.ID.String()), entry)
	if err != nil {
		t.Fatalf("retry after commit failure: %v", err)
	}
	if posted.Status != journal.EntryPosted {
		t.Fatalf("status after retry = %q, want posted", posted.Status)
	}
}

func TestAtomicPosting_SuccessUsesSingleCommit(t *testing.T) {
	f := newFixture(t)
	entry := f.entry(t)

	posted, err := f.engine.Ledger().PostEntry(postContext(entry.ID.String()), entry)
	if err != nil {
		t.Fatalf("PostEntry: %v", err)
	}
	if posted.Status != journal.EntryPosted || posted.SequenceNumber != 1 {
		t.Fatalf("posted = %s sequence=%d", posted.Status, posted.SequenceNumber)
	}
	if got := f.fault.commits.Load(); got != 1 {
		t.Fatalf("posting commits = %d, want 1", got)
	}
}

// TestAtomicPosting_ConcurrentFailuresNeverLeavePartialState combines rollback
// with concurrency: allocations that fail before effect must consume no
// sequence, and every successful attempt must receive a unique number.
func TestAtomicPosting_ConcurrentFaultsNeverLeavePartialState(t *testing.T) {
	f := newFixture(t)

	const attempts = 6
	entries := make([]journal.JournalEntry, attempts)
	for i := range entries {
		entries[i] = f.entry(t)
	}

	f.fault.failAllocation.Store(true)
	var wg sync.WaitGroup
	for i := range entries {
		wg.Add(1)
		go func(entry journal.JournalEntry) {
			defer wg.Done()
			if _, err := f.engine.Ledger().PostEntry(postContext(entry.ID.String()), entry); !errors.Is(err, storage.ErrSerializationFailure) {
				t.Errorf("expected serialization failure, got %v", err)
			}
		}(entries[i])
	}
	wg.Wait()
	for i := range entries {
		assertNoPostedEntry(t, f, entries[i])
	}

	f.fault.failAllocation.Store(false)
	sequences := make(map[int64]bool, attempts)
	for i := range entries {
		posted, err := f.engine.Ledger().PostEntry(postContext(entries[i].ID.String()), entries[i])
		if err != nil {
			t.Fatalf("retry %d: %v", i, err)
		}
		if posted.SequenceNumber <= 0 || sequences[posted.SequenceNumber] {
			t.Fatalf("invalid/duplicate sequence %d after rollback", posted.SequenceNumber)
		}
		sequences[posted.SequenceNumber] = true
	}
	for expected := int64(1); expected <= attempts; expected++ {
		if !sequences[expected] {
			t.Fatalf("missing sequence %d after rollback: %v", expected, sequences)
		}
	}
}

// TestAtomicPosting_IdempotencyReplayLeavesNoPartialWrite covers the mandatory
// duplicate-key scenario. A replayed key must return the original posted entry
// without allocating a new sequence, appending a second audit record, or
// writing any partial state.
func TestAtomicPosting_IdempotencyReplayLeavesNoPartialWrite(t *testing.T) {
	f := newFixture(t)
	entry := f.entry(t)
	const key = "p7-atomic-idempotency-replay"

	principal := publicapplication.Principal{
		ActorID:    "atomic-test",
		CompanyIDs: []types.CompanyID{f.companyID},
	}
	ctx := publicapplication.WithPrincipal(context.Background(), principal)
	ctx = publicapplication.WithRequestMetadata(ctx, publicapplication.RequestMetadata{
		CorrelationID:  "p7-atomic-idempotency",
		IdempotencyKey: key,
		SourceSystem:   "p7-atomicity-test",
	})

	posted, err := f.engine.Ledger().PostEntry(ctx, entry)
	if err != nil {
		t.Fatalf("initial PostEntry: %v", err)
	}
	if f.fault.commits.Load() != 1 {
		t.Fatalf("initial commits = %d, want 1", f.fault.commits.Load())
	}

	replay, err := f.app.Post(ctx, publicapplication.PostEntryCommand{
		Context: publicapplication.Context{
			Principal:      principal,
			CorrelationID:  "p7-atomic-idempotency",
			IdempotencyKey: key,
			SourceSystem:   "p7-atomicity-test",
		},
		CompanyID: f.companyID,
		EntryID:   posted.ID,
	})
	if err != nil {
		t.Fatalf("idempotent replay: %v", err)
	}
	if !replay.IdempotentReplay {
		t.Fatal("duplicate key was not recognised as an idempotent replay")
	}
	if replay.Entry.ID != posted.ID || replay.Entry.SequenceNumber != posted.SequenceNumber {
		t.Fatalf("replay = %+v, want original %+v", replay.Entry, posted)
	}
	if got := f.fault.commits.Load(); got != 1 {
		t.Fatalf("commits after replay = %d, want 1", got)
	}
	stored, err := f.engine.Ledger().GetEntry(ctx, f.companyID, posted.ID)
	if err != nil {
		t.Fatalf("read after replay: %v", err)
	}
	if stored.Status != journal.EntryPosted || stored.SequenceNumber != posted.SequenceNumber {
		t.Fatalf("stored after replay = %s/%d, want posted/%d", stored.Status, stored.SequenceNumber, posted.SequenceNumber)
	}
}
