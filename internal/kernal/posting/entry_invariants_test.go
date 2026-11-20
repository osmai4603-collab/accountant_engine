package posting

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/osmai4603-collab/accounting_engine/internal/kernal/journal"
	"github.com/osmai4603-collab/accounting_engine/internal/kernal/masterdata"
	"github.com/osmai4603-collab/accounting_engine/internal/kernal/types"
	"github.com/shopspring/decimal"
)

func makeMoney(t *testing.T, amount string, currency string) types.Money {
	t.Helper()
	m, err := types.NewMoneyFromString(amount, currency)
	if err != nil {
		t.Fatalf("failed to create money: %v", err)
	}
	return m
}

func TestDoubleEntryMathematicalValidation(t *testing.T) {
	compID := types.NewCompanyID()
	journalID := types.NewJournalID()
	acc1 := types.NewAccountID()
	acc2 := types.NewAccountID()

	// 1. Unbalanced entry
	unbalanced := journal.JournalEntry{
		CompanyID:     compID,
		JournalID:     journalID,
		EffectiveDate: time.Now().UTC(),
		Lines: []journal.EntryLine{
			{
				AccountID:           acc1,
				TransactionCurrency: "USD",
				TransactionAmount:   makeMoney(t, "100.00", "USD"),
				ExchangeRate:        decimal.NewFromInt(1),
				FunctionalDebit:     makeMoney(t, "100.00", "USD"),
				FunctionalCredit:    makeMoney(t, "0.00", "USD"),
			},
			{
				AccountID:           acc2,
				TransactionCurrency: "USD",
				TransactionAmount:   makeMoney(t, "90.00", "USD"),
				ExchangeRate:        decimal.NewFromInt(1),
				FunctionalDebit:     makeMoney(t, "0.00", "USD"),
				FunctionalCredit:    makeMoney(t, "90.00", "USD"),
			},
		},
	}

	err := ValidateEntryDoubleEntry(&unbalanced, nil)
	if !errors.Is(err, ErrUnbalancedEntry) {
		t.Fatalf("expected ErrUnbalancedEntry, got %v", err)
	}

	// 2. Balanced entry
	balanced := unbalanced
	balanced.Lines[1].TransactionAmount = makeMoney(t, "100.00", "USD")
	balanced.Lines[1].FunctionalCredit = makeMoney(t, "100.00", "USD")

	if err := ValidateEntryDoubleEntry(&balanced, nil); err != nil {
		t.Fatalf("expected balanced entry to pass, got %v", err)
	}

	// 3. Zero amount line
	zeroLine := balanced
	zeroLine.Lines[0].TransactionAmount = makeMoney(t, "0.00", "USD")
	zeroLine.Lines[0].FunctionalDebit = makeMoney(t, "0.00", "USD")
	if err := ValidateEntryDoubleEntry(&zeroLine, nil); !errors.Is(err, types.ErrZeroAmountLine) {
		t.Fatalf("expected ErrZeroAmountLine, got %v", err)
	}

	// 4. Line with both debit and credit
	bothSides := journal.JournalEntry{
		CompanyID:     compID,
		JournalID:     journalID,
		EffectiveDate: time.Now().UTC(),
		Lines: []journal.EntryLine{
			{
				AccountID:           acc1,
				TransactionCurrency: "USD",
				TransactionAmount:   makeMoney(t, "100.00", "USD"),
				ExchangeRate:        decimal.NewFromInt(1),
				FunctionalDebit:     makeMoney(t, "100.00", "USD"),
				FunctionalCredit:    makeMoney(t, "50.00", "USD"),
			},
			{
				AccountID:           acc2,
				TransactionCurrency: "USD",
				TransactionAmount:   makeMoney(t, "50.00", "USD"),
				ExchangeRate:        decimal.NewFromInt(1),
				FunctionalDebit:     makeMoney(t, "0.00", "USD"),
				FunctionalCredit:    makeMoney(t, "50.00", "USD"),
			},
		},
	}
	if err := ValidateEntryDoubleEntry(&bothSides, nil); !errors.Is(err, ErrLineMustHaveOneSide) {
		t.Fatalf("expected ErrLineMustHaveOneSide, got %v", err)
	}

	// 5. Negative amounts rejected
	negMoney, _ := types.NewMoneyFromString("-10.00", "USD")
	negEntry := balanced
	negEntry.Lines[0].FunctionalDebit = negMoney
	if err := ValidateEntryDoubleEntry(&negEntry, nil); !errors.Is(err, types.ErrNegativeAmountProhibited) {
		t.Fatalf("expected ErrNegativeAmountProhibited, got %v", err)
	}
}

func TestReversalLifecycle(t *testing.T) {
	compID := types.NewCompanyID()
	journalID := types.NewJournalID()
	acc1 := types.NewAccountID()
	acc2 := types.NewAccountID()

	origDate := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	entry := journal.JournalEntry{
		ID:            types.NewJournalEntryID(),
		CompanyID:     compID,
		JournalID:     journalID,
		EffectiveDate: origDate,
		PostingDate:   origDate,
		Status:        journal.EntryPosted,
		Lines: []journal.EntryLine{
			{
				AccountID:           acc1,
				TransactionCurrency: "USD",
				TransactionAmount:   makeMoney(t, "150.00", "USD"),
				ExchangeRate:        decimal.NewFromInt(1),
				FunctionalDebit:     makeMoney(t, "150.00", "USD"),
				FunctionalCredit:    makeMoney(t, "0.00", "USD"),
			},
			{
				AccountID:           acc2,
				TransactionCurrency: "USD",
				TransactionAmount:   makeMoney(t, "150.00", "USD"),
				ExchangeRate:        decimal.NewFromInt(1),
				FunctionalDebit:     makeMoney(t, "0.00", "USD"),
				FunctionalCredit:    makeMoney(t, "150.00", "USD"),
			},
		},
	}

	// 1. Reversal on later date
	revDate := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	reversal, err := BuildReversalEntry(entry, revDate, "maker-1", "Correction")
	if err != nil {
		t.Fatalf("BuildReversalEntry failed: %v", err)
	}

	if reversal.Classification != journal.EntryClassReversing {
		t.Fatalf("expected classification reversing, got %s", reversal.Classification)
	}
	if *reversal.ReversalOfID != entry.ID {
		t.Fatalf("reversal not pointing to original")
	}
	// Check swapped debit/credit
	if !reversal.Lines[0].FunctionalCredit.Equal(entry.Lines[0].FunctionalDebit) {
		t.Fatalf("expected line 0 debit to become credit")
	}

	// 2. Reversal date before original date
	earlierDate := time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC)
	_, err = BuildReversalEntry(entry, earlierDate, "maker-1", "Correction")
	if !errors.Is(err, ErrReversalDateBeforeOriginal) {
		t.Fatalf("expected ErrReversalDateBeforeOriginal, got %v", err)
	}

	// 3. Reversing non-posted draft entry
	draftEntry := entry
	draftEntry.Status = journal.EntryDraft
	_, err = BuildReversalEntry(draftEntry, revDate, "maker-1", "Correction")
	if !errors.Is(err, ErrEntryNotPosted) {
		t.Fatalf("expected ErrEntryNotPosted, got %v", err)
	}

	// 4. Reversing already reversed entry
	revID := types.NewJournalEntryID()
	alreadyRev := entry
	alreadyRev.ReversedByID = &revID
	_, err = BuildReversalEntry(alreadyRev, revDate, "maker-1", "Correction")
	if !errors.Is(err, ErrEntryAlreadyReversed) {
		t.Fatalf("expected ErrEntryAlreadyReversed, got %v", err)
	}
}

type mockMasterReaderForPosting struct {
	compActive bool
	periodOpen bool
	isHeader   bool
}

func (m *mockMasterReaderForPosting) GetCompany(ctx context.Context, id types.CompanyID) (masterdata.Company, error) {
	return masterdata.Company{ID: id, Active: m.compActive}, nil
}
func (m *mockMasterReaderForPosting) GetAccount(ctx context.Context, cid types.CompanyID, aid types.AccountID) (masterdata.Account, error) {
	return masterdata.Account{ID: aid, CompanyID: cid, IsActive: true}, nil
}
func (m *mockMasterReaderForPosting) GetAccountByCode(ctx context.Context, cid types.CompanyID, code string) (masterdata.Account, error) {
	return masterdata.Account{}, nil
}
func (m *mockMasterReaderForPosting) GetJournalByCode(ctx context.Context, cid types.CompanyID, code string) (masterdata.Journal, error) {
	return masterdata.Journal{}, nil
}
func (m *mockMasterReaderForPosting) HasAccountChildren(ctx context.Context, cid types.CompanyID, aid types.AccountID) (bool, error) {
	return m.isHeader, nil
}
func (m *mockMasterReaderForPosting) GetFiscalYear(ctx context.Context, cid types.CompanyID, fyid types.FiscalYearID) (masterdata.FiscalYear, error) {
	return masterdata.FiscalYear{}, nil
}
func (m *mockMasterReaderForPosting) GetPeriodForDate(ctx context.Context, cid types.CompanyID, date time.Time) (masterdata.FiscalPeriod, error) {
	st := masterdata.PeriodOpen
	if !m.periodOpen {
		st = masterdata.PeriodHardClosed
	}
	return masterdata.FiscalPeriod{ID: types.NewFiscalPeriodID(), CompanyID: cid, State: st}, nil
}

func TestPostingPipelineEndToEnd(t *testing.T) {
	ctx := context.Background()
	reader := &mockMasterReaderForPosting{compActive: true, periodOpen: true, isHeader: false}
	pipeline := NewPostingPipeline(reader, func(ctx context.Context, entry journal.JournalEntry, opts PostOptions) (journal.JournalEntry, error) {
		entry.SequenceNumber = 1
		return entry, nil
	})

	compID := types.NewCompanyID()
	journalID := types.NewJournalID()
	acc1 := types.NewAccountID()
	acc2 := types.NewAccountID()

	entry, err := NewJournalEntry(compID, journalID, time.Now().UTC(), "maker-1", []journal.EntryLine{
		{
			AccountID:           acc1,
			TransactionCurrency: "USD",
			TransactionAmount:   makeMoney(t, "100.00", "USD"),
			ExchangeRate:        decimal.NewFromInt(1),
			FunctionalDebit:     makeMoney(t, "100.00", "USD"),
			FunctionalCredit:    makeMoney(t, "0.00", "USD"),
		},
		{
			AccountID:           acc2,
			TransactionCurrency: "USD",
			TransactionAmount:   makeMoney(t, "100.00", "USD"),
			ExchangeRate:        decimal.NewFromInt(1),
			FunctionalDebit:     makeMoney(t, "0.00", "USD"),
			FunctionalCredit:    makeMoney(t, "100.00", "USD"),
		},
	})
	if err != nil {
		t.Fatalf("NewJournalEntry failed: %v", err)
	}

	posted, err := pipeline.PostEntry(ctx, entry, PostOptions{ActorID: "poster-1", Reason: "Regular Posting"})
	if err != nil {
		t.Fatalf("pipeline.PostEntry failed: %v", err)
	}

	if posted.Status != journal.EntryPosted {
		t.Fatalf("expected status posted, got %s", posted.Status)
	}
	if posted.SequenceNumber != 1 {
		t.Fatalf("expected sequence 1, got %d", posted.SequenceNumber)
	}
}

func TestAdjustingEntriesAndImmutabilityCoverage(t *testing.T) {
	// EnsureImmutable tests
	if err := EnsureImmutable(journal.EntryPosted); !errors.Is(err, types.ErrCannotModifyPostedMove) {
		t.Fatalf("expected ErrCannotModifyPostedMove, got %v", err)
	}
	if err := EnsureImmutable(journal.EntryReversed); !errors.Is(err, types.ErrCannotModifyPostedMove) {
		t.Fatalf("expected ErrCannotModifyPostedMove, got %v", err)
	}
	if err := EnsureImmutable(journal.EntryDraft); err != nil {
		t.Fatalf("draft should be mutable, got %v", err)
	}

	// Adjusting summary tests
	now := time.Now().UTC()
	entries := []journal.JournalEntry{
		{
			ID:                          types.NewJournalEntryID(),
			EntryNumber:                 "JE-001",
			Classification:              journal.EntryClassAdjusting,
			AdjustingEntryType:          journal.AdjustingEntryAccrual,
			AdjustmentReason:            "Month end accrual",
			AdjustmentApprovalReference: "APP-01",
			AdjustmentAuthorized:        true,
			CreatedBy:                   "user1",
			CreatedAt:                   now,
			ApprovedBy:                  "manager1",
			ApprovedAt:                  &now,
			Status:                      journal.EntryPosted,
		},
		{
			ID:             types.NewJournalEntryID(),
			Classification: journal.EntryClassAdjusting,
			Status:         journal.EntryDraft, // missing metadata + draft
		},
		{
			ID:             types.NewJournalEntryID(),
			Classification: journal.EntryClassRegular,
		},
	}

	summary := SummarizeAdjustingEntriesForPreClose(entries)
	if summary.TotalAdjustingEntries != 2 {
		t.Fatalf("expected 2 adjusting entries, got %d", summary.TotalAdjustingEntries)
	}
	if summary.PendingDrafts != 1 {
		t.Fatalf("expected 1 draft, got %d", summary.PendingDrafts)
	}
	if summary.MissingAuditMetadata != 1 {
		t.Fatalf("expected 1 missing audit, got %d", summary.MissingAuditMetadata)
	}
	if summary.Ready() {
		t.Fatalf("summary should not be ready")
	}

	readySummary := SummarizeAdjustingEntriesForPreClose([]journal.JournalEntry{entries[0]})
	if !readySummary.Ready() {
		t.Fatalf("single approved adjusting entry should be ready")
	}
}
