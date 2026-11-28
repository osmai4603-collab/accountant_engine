package posting

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/osmai4603-collab/accounting_engine/internal/kernel/journal"
	"github.com/osmai4603-collab/accounting_engine/internal/kernel/masterdata"
	"github.com/osmai4603-collab/accounting_engine/internal/kernel/types"
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
		ID:             types.NewJournalEntryID(),
		CompanyID:      compID,
		JournalID:      journalID,
		Classification: journal.EntryClassRegular,
		EffectiveDate:  time.Now().UTC(),
		Lines: []journal.EntryLine{
			{
				ID:                  types.NewJournalEntryLineID(),
				AccountID:           acc1,
				TransactionCurrency: "USD",
				TransactionAmount:   makeMoney(t, "100.00", "USD"),
				ExchangeRate:        decimal.NewFromInt(1),
				FunctionalDebit:     makeMoney(t, "100.00", "USD"),
				FunctionalCredit:    makeMoney(t, "0.00", "USD"),
			},
			{
				ID:                  types.NewJournalEntryLineID(),
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
	if err := ValidateEntryDoubleEntry(&zeroLine, nil); !errors.Is(err, ErrZeroAmountLine) {
		t.Fatalf("expected ErrZeroAmountLine, got %v", err)
	}

	// 4. Line with both debit and credit
	bothSides := journal.JournalEntry{
		ID:             types.NewJournalEntryID(),
		CompanyID:      compID,
		JournalID:      journalID,
		Classification: journal.EntryClassRegular,
		EffectiveDate:  time.Now().UTC(),
		Lines: []journal.EntryLine{
			{
				ID:                  types.NewJournalEntryLineID(),
				AccountID:           acc1,
				TransactionCurrency: "USD",
				TransactionAmount:   makeMoney(t, "100.00", "USD"),
				ExchangeRate:        decimal.NewFromInt(1),
				FunctionalDebit:     makeMoney(t, "100.00", "USD"),
				FunctionalCredit:    makeMoney(t, "50.00", "USD"),
			},
			{
				ID:                  types.NewJournalEntryLineID(),
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

func TestEntryIdentityValidationRequiresUniqueLineIDs(t *testing.T) {
	entry := newPostingTestEntry(t, "creator")
	missingEntryID := entry
	missingEntryID.ID = types.JournalEntryID{}
	if err := ValidateEntryIdentity(missingEntryID); !errors.Is(err, ErrMissingEntryID) {
		t.Fatalf("missing entry id error = %v, want missing entry id", err)
	}

	missingLineID := entry
	missingLineID.Lines = append([]journal.EntryLine(nil), entry.Lines...)
	missingLineID.Lines[0].ID = types.JournalEntryLineID{}
	if err := ValidateEntryIdentity(missingLineID); !errors.Is(err, ErrMissingLineID) {
		t.Fatalf("missing line id error = %v, want missing line id", err)
	}

	duplicateLineID := entry
	duplicateLineID.Lines = append([]journal.EntryLine(nil), entry.Lines...)
	duplicateLineID.Lines[1].ID = duplicateLineID.Lines[0].ID
	if err := ValidateEntryIdentity(duplicateLineID); !errors.Is(err, ErrDuplicateLineID) {
		t.Fatalf("duplicate line id error = %v, want duplicate line id", err)
	}
}

func TestAdjustingEntriesRequireMetadataAtValidation(t *testing.T) {
	entry := newPostingTestEntry(t, "creator")
	entry.Classification = ""
	if err := ValidateEntryClassificationMetadata(entry); !errors.Is(err, ErrInvalidEntry) {
		t.Fatalf("entry without classification error = %v, want invalid entry", err)
	}

	entry.Classification = journal.EntryClassAdjusting
	if err := ValidateEntryDoubleEntry(&entry, nil); !errors.Is(err, ErrAdjustingEntryMetadataRequired) {
		t.Fatalf("adjusting entry without metadata error = %v, want required metadata", err)
	}

	entry.AdjustingEntryType = journal.AdjustingEntryAccrual
	entry.AdjustmentReason = "Month-end accrual"
	if err := ValidateEntryDoubleEntry(&entry, nil); err != nil {
		t.Fatalf("adjusting entry with metadata: %v", err)
	}

	entry.Classification = journal.EntryClassClosing
	entry.AdjustingEntryType = ""
	entry.AdjustmentReason = ""
	if err := ValidateEntryDoubleEntry(&entry, nil); err != nil {
		t.Fatalf("closing entry without adjusting metadata: %v", err)
	}
}

func TestReversalClassificationAndReferenceMustMatch(t *testing.T) {
	entry := newPostingTestEntry(t, "creator")
	originalID := types.NewJournalEntryID()
	entry.ReversalOfID = &originalID
	if err := ValidateEntryDoubleEntry(&entry, nil); !errors.Is(err, ErrInvalidReversal) {
		t.Fatalf("regular entry with reversal reference error = %v, want invalid reversal", err)
	}

	entry.Classification = journal.EntryClassReversing
	entry.ReversalOfID = nil
	if err := ValidateEntryDoubleEntry(&entry, nil); !errors.Is(err, ErrInvalidReversal) {
		t.Fatalf("reversing entry without reference error = %v, want invalid reversal", err)
	}

	entry.ReversalOfID = &originalID
	entry.ReversalReason = "Correction"
	if err := ValidateEntryDoubleEntry(&entry, nil); err != nil {
		t.Fatalf("reversing entry with valid reference: %v", err)
	}

	entry.ReversalReason = ""
	if err := ValidateEntryDoubleEntry(&entry, nil); !errors.Is(err, ErrReversalReasonRequired) {
		t.Fatalf("reversing entry without reason error = %v, want reversal reason required", err)
	}

	entry.ReversalReason = "   "
	if err := ValidateEntryDoubleEntry(&entry, nil); !errors.Is(err, ErrReversalReasonRequired) {
		t.Fatalf("reversing entry with blank reason error = %v, want reversal reason required", err)
	}

	entry.ReversalReason = "Correction"
	entry.ReversalOfID = &entry.ID
	if err := ValidateEntryDoubleEntry(&entry, nil); !errors.Is(err, ErrInvalidReversal) {
		t.Fatalf("self-reversal reference error = %v, want invalid reversal", err)
	}
	entry.ReversalOfID = &originalID
	entry.Classification = journal.EntryClassRegular
	if err := ValidateEntryDoubleEntry(&entry, nil); !errors.Is(err, ErrInvalidReversal) {
		t.Fatalf("regular entry with orphan reversal reason error = %v, want invalid reversal", err)
	}
}

func TestReversalLifecycle(t *testing.T) {
	compID := types.NewCompanyID()
	journalID := types.NewJournalID()
	acc1 := types.NewAccountID()
	acc2 := types.NewAccountID()

	origDate := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	entry := journal.JournalEntry{
		ID:             types.NewJournalEntryID(),
		CompanyID:      compID,
		JournalID:      journalID,
		Classification: journal.EntryClassRegular,
		EffectiveDate:  origDate,
		PostingDate:    origDate,
		Status:         journal.EntryPosted,
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
	if _, err := BuildReversalEntry(entry, revDate, "creator-1", ""); !errors.Is(err, ErrReversalReasonRequired) {
		t.Fatalf("empty reversal reason error = %v, want reversal reason required", err)
	}
	if _, err := BuildReversalEntry(entry, revDate, "creator-1", " \t "); !errors.Is(err, ErrReversalReasonRequired) {
		t.Fatalf("blank reversal reason error = %v, want reversal reason required", err)
	}
	reversal, err := BuildReversalEntry(entry, revDate, "creator-1", "Correction")
	if err != nil {
		t.Fatalf("BuildReversalEntry failed: %v", err)
	}
	if reversal.ReversalReason != "Correction" {
		t.Fatalf("reversal reason = %q, want trimmed reason", reversal.ReversalReason)
	}

	if reversal.Classification != journal.EntryClassReversing {
		t.Fatalf("expected classification reversing, got %s", reversal.Classification)
	}
	if reversal.Status != journal.EntryDraft || reversal.FiscalPeriodID != (types.FiscalPeriodID{}) {
		t.Fatalf("reversal must be a draft with no inherited fiscal period")
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
	_, err = BuildReversalEntry(entry, earlierDate, "creator-1", "Correction")
	if !errors.Is(err, ErrReversalDateBeforeOriginal) {
		t.Fatalf("expected ErrReversalDateBeforeOriginal, got %v", err)
	}

	// 3. Reversing non-posted draft entry
	draftEntry := entry
	draftEntry.Status = journal.EntryDraft
	_, err = BuildReversalEntry(draftEntry, revDate, "creator-1", "Correction")
	if !errors.Is(err, ErrEntryNotPosted) {
		t.Fatalf("expected ErrEntryNotPosted, got %v", err)
	}

	// 4. Reversing already reversed entry
	revID := types.NewJournalEntryID()
	alreadyRev := entry
	alreadyRev.ReversedByID = &revID
	_, err = BuildReversalEntry(alreadyRev, revDate, "creator-1", "Correction")
	if !errors.Is(err, ErrEntryAlreadyReversed) {
		t.Fatalf("expected ErrEntryAlreadyReversed, got %v", err)
	}

	// 5. Reversing a reversing entry is forbidden (PST-008 / RULE-ACC-026)
	reversingPosted := reversal
	reversingPosted.Status = journal.EntryPosted
	_, err = BuildReversalEntry(reversingPosted, revDate, "creator-1", "Reverse reversal")
	if !errors.Is(err, ErrInvalidReversal) {
		t.Fatalf("expected ErrInvalidReversal when reversing a reversing entry, got %v", err)
	}

	mismatchedReversal := entry
	mismatchedReversal.Classification = journal.EntryClassRegular
	origID := types.NewJournalEntryID()
	mismatchedReversal.ReversalOfID = &origID
	_, err = BuildReversalEntry(mismatchedReversal, revDate, "creator-1", "Reverse with ReversalOfID")
	if !errors.Is(err, ErrInvalidReversal) {
		t.Fatalf("expected ErrInvalidReversal when reversing entry with ReversalOfID, got %v", err)
	}
}

func TestValidateEntryLinePST004AndExchangeRatePrecision(t *testing.T) {
	acc1 := types.NewAccountID()
	lineID := types.NewJournalEntryLineID()

	// Exceeding 0.5 minor-unit tolerance (PST-004) must fail closed.
	divergent := journal.EntryLine{
		ID:                  lineID,
		AccountID:           acc1,
		TransactionCurrency: "EUR",
		TransactionAmount:   makeMoney(t, "10.005", "EUR"),
		ExchangeRate:        decimal.RequireFromString("3.75"),
		FunctionalDebit:     makeMoney(t, "37.50", "SAR"),
		FunctionalCredit:    makeMoney(t, "0.00", "SAR"),
	}
	if err := ValidateEntryLine(divergent, nil); !errors.Is(err, types.ErrInvalidAmount) {
		t.Fatalf("divergent functional amount error = %v, want ErrInvalidAmount", err)
	}

	// Both round to 12.04 USD under banker's rounding, but diff > 0.005 must fail PST-004.
	sneakyDivergent := journal.EntryLine{
		ID:                  lineID,
		AccountID:           acc1,
		TransactionCurrency: "EUR",
		TransactionAmount:   makeMoney(t, "10.00", "EUR"),
		ExchangeRate:        decimal.RequireFromString("1.2045"),
		FunctionalDebit:     makeMoney(t, "12.036", "USD"),
		FunctionalCredit:    makeMoney(t, "0.00", "USD"),
	}
	if err := ValidateEntryLine(sneakyDivergent, nil); !errors.Is(err, types.ErrInvalidAmount) {
		t.Fatalf("sub-cent >0.5 minor-unit divergence error = %v, want ErrInvalidAmount", err)
	}

	// MONEY-025: exchange rate with 13 fractional digits must fail closed.
	overScaledRate := journal.EntryLine{
		ID:                  lineID,
		AccountID:           acc1,
		TransactionCurrency: "EUR",
		TransactionAmount:   makeMoney(t, "10.00", "EUR"),
		ExchangeRate:        decimal.RequireFromString("1.1234567890123"),
		FunctionalDebit:     makeMoney(t, "11.23", "USD"),
		FunctionalCredit:    makeMoney(t, "0.00", "USD"),
	}
	if err := ValidateEntryLine(overScaledRate, nil); !errors.Is(err, ErrInvalidExchangeRate) {
		t.Fatalf("over-scaled exchange rate error = %v, want ErrInvalidExchangeRate", err)
	}

	// MONEY-026: exchange rate exceeding 28 integer/significant digits must fail.
	hugeRate := overScaledRate
	hugeRate.ExchangeRate = decimal.RequireFromString("1e28")
	hugeRate.FunctionalDebit = makeMoney(t, "1e27", "USD")
	if err := ValidateEntryLine(hugeRate, nil); !errors.Is(err, ErrInvalidExchangeRate) {
		t.Fatalf("oversized exchange rate error = %v, want ErrInvalidExchangeRate", err)
	}
}

func TestRoundEntryWithRoundingPolicyHonorsExplicitHalfUp(t *testing.T) {
	halfUp := func(m types.Money) (types.Money, error) {
		c, ok := types.LookupCurrency(m.Currency())
		if !ok {
			return types.Money{}, types.ErrInvalidCurrency
		}
		rounded := m.Amount().Round(c.MinorUnit)
		return types.NewMoney(rounded, m.Currency())
	}
	zeroUSD := makeMoney(t, "0.00", "USD")
	entry := journal.JournalEntry{
		ID:             types.NewJournalEntryID(),
		CompanyID:      types.NewCompanyID(),
		JournalID:      types.NewJournalID(),
		Classification: journal.EntryClassRegular,
		EffectiveDate:  time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		Lines: []journal.EntryLine{
			{
				ID:                  types.NewJournalEntryLineID(),
				AccountID:           types.NewAccountID(),
				TransactionCurrency: "USD",
				TransactionAmount:   makeMoney(t, "10.005", "USD"),
				ExchangeRate:        decimal.NewFromInt(1),
				FunctionalDebit:     makeMoney(t, "10.01", "USD"),
				FunctionalCredit:    zeroUSD,
			},
			{
				ID:                  types.NewJournalEntryLineID(),
				AccountID:           types.NewAccountID(),
				TransactionCurrency: "USD",
				TransactionAmount:   makeMoney(t, "10.005", "USD"),
				ExchangeRate:        decimal.NewFromInt(1),
				FunctionalDebit:     zeroUSD,
				FunctionalCredit:    makeMoney(t, "10.01", "USD"),
			},
		},
	}
	if err := RoundEntryWithRoundingPolicy(&entry, types.AccountID{}, halfUp); err != nil {
		t.Fatalf("RoundEntryWithRoundingPolicy(halfUp) failed: %v", err)
	}
	if entry.Lines[0].FunctionalDebit.Amount().StringFixed(2) != "10.01" {
		t.Fatalf("FunctionalDebit = %s, want 10.01 under HalfUp", entry.Lines[0].FunctionalDebit.ExactString())
	}
}

func FuzzReversalNetImpactIsZeroPerAccount(f *testing.F) {
	for _, amount := range []string{"0.01", "150.00", "99999999999999999999.99", "0", "-1.00", "invalid"} {
		f.Add(amount)
	}

	f.Fuzz(func(t *testing.T, rawAmount string) {
		amount, err := types.NewMoneyFromString(rawAmount, "USD")
		if err != nil || !amount.Amount().IsPositive() {
			return
		}
		zero := makeMoney(t, "0", "USD")
		accountA, accountB := types.NewAccountID(), types.NewAccountID()
		originalDate := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
		original := journal.JournalEntry{
			ID:             types.NewJournalEntryID(),
			CompanyID:      types.NewCompanyID(),
			JournalID:      types.NewJournalID(),
			Classification: journal.EntryClassRegular,
			EffectiveDate:  originalDate,
			Status:         journal.EntryPosted,
			Lines: []journal.EntryLine{
				{
					ID:                  types.NewJournalEntryLineID(),
					AccountID:           accountA,
					TransactionCurrency: "USD",
					TransactionAmount:   amount,
					ExchangeRate:        decimal.NewFromInt(1),
					FunctionalDebit:     amount,
					FunctionalCredit:    zero,
				},
				{
					ID:                  types.NewJournalEntryLineID(),
					AccountID:           accountB,
					TransactionCurrency: "USD",
					TransactionAmount:   amount,
					ExchangeRate:        decimal.NewFromInt(1),
					FunctionalDebit:     zero,
					FunctionalCredit:    amount,
				},
			},
		}
		reversal, err := BuildReversalEntry(
			original, originalDate.AddDate(0, 0, 1), "fuzz-actor", "fuzz reversal",
		)
		if err != nil {
			t.Fatalf("build reversal for %s USD: %v", rawAmount, err)
		}

		netByAccount := make(map[types.AccountID]decimal.Decimal)
		for _, entry := range []journal.JournalEntry{original, reversal} {
			for _, line := range entry.Lines {
				netByAccount[line.AccountID] = netByAccount[line.AccountID].
					Add(line.FunctionalDebit.Amount()).
					Sub(line.FunctionalCredit.Amount())
			}
		}
		for accountID, net := range netByAccount {
			if !net.IsZero() {
				t.Fatalf("combined reversal impact for account %s = %s, want zero", accountID, net)
			}
		}
	})
}

type mockMasterReaderForPosting struct {
	compActive         bool
	periodOpen         bool
	periodState        masterdata.PeriodState
	isHeader           bool
	functionalCurrency string
	journal            masterdata.Journal
	foreignAccount     bool
	controlAccount     bool
}

func (m *mockMasterReaderForPosting) GetCompany(ctx context.Context, id types.CompanyID) (masterdata.Company, error) {
	fc := m.functionalCurrency
	if fc == "" {
		fc = "USD"
	}
	return masterdata.Company{ID: id, Active: m.compActive, FunctionalCurrency: fc}, nil
}
func (m *mockMasterReaderForPosting) GetAccount(ctx context.Context, cid types.CompanyID, aid types.AccountID) (masterdata.Account, error) {
	accountCompany := cid
	if m.foreignAccount {
		accountCompany = types.NewCompanyID()
	}
	return masterdata.Account{
		ID: aid, CompanyID: accountCompany, IsActive: true,
		IsControlAccount: m.controlAccount, Type: masterdata.AccountTypeAsset,
	}, nil
}
func (m *mockMasterReaderForPosting) GetAccountByCode(ctx context.Context, cid types.CompanyID, code string) (masterdata.Account, error) {
	return masterdata.Account{}, nil
}
func (m *mockMasterReaderForPosting) GetJournalByCode(ctx context.Context, cid types.CompanyID, code string) (masterdata.Journal, error) {
	return masterdata.Journal{}, nil
}
func (m *mockMasterReaderForPosting) GetJournalByID(_ context.Context, cid types.CompanyID, journalID types.JournalID) (masterdata.Journal, error) {
	if m.journal.ID != (types.JournalID{}) {
		return m.journal, nil
	}
	return masterdata.Journal{ID: journalID, CompanyID: cid, Code: "GEN", Name: "General"}, nil
}
func (m *mockMasterReaderForPosting) HasAccountChildren(ctx context.Context, cid types.CompanyID, aid types.AccountID) (bool, error) {
	return m.isHeader, nil
}
func (m *mockMasterReaderForPosting) GetFiscalYear(ctx context.Context, cid types.CompanyID, fyid types.FiscalYearID) (masterdata.FiscalYear, error) {
	return masterdata.FiscalYear{
		ID: fyid, CompanyID: cid, State: masterdata.FiscalYearOpen,
		StartDate: time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2999, time.December, 31, 23, 59, 59, 999999999, time.UTC),
	}, nil
}
func (m *mockMasterReaderForPosting) GetPeriodForDate(ctx context.Context, cid types.CompanyID, date time.Time) (masterdata.FiscalPeriod, error) {
	st := m.periodState
	if st == "" {
		st = masterdata.PeriodOpen
		if !m.periodOpen {
			st = masterdata.PeriodHardClosed
		}
	}
	date = date.UTC()
	start := time.Date(date.Year(), date.Month(), 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0).Add(-time.Nanosecond)
	return masterdata.FiscalPeriod{
		ID: types.NewFiscalPeriodID(), CompanyID: cid, FiscalYearID: types.NewFiscalYearID(),
		Number: int(date.Month()), State: st, Type: masterdata.FiscalPeriodTypeNormal,
		StartDate: start, EndDate: end,
	}, nil
}

func TestPostingPipelineEndToEnd(t *testing.T) {
	ctx := context.Background()
	reader := &mockMasterReaderForPosting{compActive: true, periodOpen: true, isHeader: false}

	compID := types.NewCompanyID()
	journalID := types.NewJournalID()
	acc1 := types.NewAccountID()
	acc2 := types.NewAccountID()

	entry, err := NewJournalEntry(compID, journalID, time.Now().UTC(), "creator-1", []journal.EntryLine{
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
	pipeline := NewPostingPipeline(reader, func(ctx context.Context, entry journal.JournalEntry, opts PostOptions) (journal.JournalEntry, error) {
		entry.SequenceNumber = 1
		return entry, nil
	}, WithPolicyEngine(testPostingPolicyEngine(t, compID, "USD")))

	posted, err := pipeline.PostEntry(ctx, entry, PostOptions{ActorID: "poster-1", Reason: "Regular Posting", IdempotencyKey: "idem-key-1"})
	if err != nil {
		t.Fatalf("pipeline.PostEntry failed: %v", err)
	}

	if posted.Status != journal.EntryPosted {
		t.Fatalf("expected status posted, got %s", posted.Status)
	}
	if posted.SequenceNumber != 1 {
		t.Fatalf("expected sequence 1, got %d", posted.SequenceNumber)
	}

	// 1. Rejection on non-draft status
	postedEntry := entry
	postedEntry.Status = journal.EntryPosted
	_, err = pipeline.PostEntry(ctx, postedEntry, PostOptions{ActorID: "poster-1", IdempotencyKey: "idem-key-1"})
	if !errors.Is(err, ErrInvalidEntryState) {
		t.Fatalf("expected ErrInvalidEntryState for non-draft, got %v", err)
	}

	// 2. Rejection on empty ActorID
	_, err = pipeline.PostEntry(ctx, entry, PostOptions{ActorID: "", IdempotencyKey: "idem-key-1"})
	if !errors.Is(err, ErrPosterIDRequired) {
		t.Fatalf("expected ErrPosterIDRequired, got %v", err)
	}

	// 2b. Rejection on empty IdempotencyKey
	_, err = pipeline.PostEntry(ctx, entry, PostOptions{ActorID: "poster-1", IdempotencyKey: ""})
	if !errors.Is(err, ErrIdempotencyKeyRequired) {
		t.Fatalf("expected ErrIdempotencyKeyRequired for empty key, got %v", err)
	}
	_, err = pipeline.PostEntry(ctx, entry, PostOptions{ActorID: "poster-1", IdempotencyKey: "   "})
	if !errors.Is(err, ErrIdempotencyKeyRequired) {
		t.Fatalf("expected ErrIdempotencyKeyRequired for whitespace key, got %v", err)
	}

	// 3. Rejection on functional currency mismatch with company
	eurCompID := types.NewCompanyID()
	eurReader := &mockMasterReaderForPosting{compActive: true, periodOpen: true}
	eurPipeline := NewPostingPipeline(eurReader, func(ctx context.Context, entry journal.JournalEntry, opts PostOptions) (journal.JournalEntry, error) {
		return entry, nil
	}, WithPolicyEngine(testPostingPolicyEngine(t, eurCompID, "USD")))
	eurEntry := entry
	eurEntry.CompanyID = eurCompID
	eurEntry.Lines = make([]journal.EntryLine, len(entry.Lines))
	copy(eurEntry.Lines, entry.Lines)
	eurEntry.Lines[0].FunctionalDebit = makeMoney(t, "100.00", "EUR")
	eurEntry.Lines[0].FunctionalCredit = makeMoney(t, "0.00", "EUR")
	eurEntry.Lines[1].FunctionalDebit = makeMoney(t, "0.00", "EUR")
	eurEntry.Lines[1].FunctionalCredit = makeMoney(t, "100.00", "EUR")
	_, err = eurPipeline.PostEntry(ctx, eurEntry, PostOptions{ActorID: "poster-1", IdempotencyKey: "idem-key-1"})
	if !errors.Is(err, ErrFunctionalCurrencyMismatch) {
		t.Fatalf("expected ErrFunctionalCurrencyMismatch, got %v", err)
	}

}

func TestAdjustingEntriesAndImmutabilityCoverage(t *testing.T) {
	// EnsureImmutable tests
	if err := EnsureImmutable(journal.EntryPosted); !errors.Is(err, ErrCannotModifyPostedMove) {
		t.Fatalf("expected ErrCannotModifyPostedMove, got %v", err)
	}
	if err := EnsureImmutable(journal.EntryReversed); !errors.Is(err, ErrCannotModifyPostedMove) {
		t.Fatalf("expected ErrCannotModifyPostedMove, got %v", err)
	}
	if err := EnsureImmutable(journal.EntryCancelled); !errors.Is(err, ErrInvalidEntryState) {
		t.Fatalf("expected ErrInvalidEntryState for cancelled entry, got %v", err)
	}
	if err := EnsureImmutable(journal.EntryDraft); err != nil {
		t.Fatalf("draft should be mutable, got %v", err)
	}

	// Adjusting summary tests
	now := time.Now().UTC()
	entries := []journal.JournalEntry{
		{
			ID:                 types.NewJournalEntryID(),
			EntryNumber:        "JE-001",
			Classification:     journal.EntryClassAdjusting,
			AdjustingEntryType: journal.AdjustingEntryAccrual,
			AdjustmentReason:   "Month end accrual",
			CreatedBy:          "user1",
			CreatedAt:          now,
			Status:             journal.EntryPosted,
		},
		{
			ID:             types.NewJournalEntryID(),
			Classification: journal.EntryClassAdjusting,
			Status:         journal.EntryDraft, // missing metadata + draft
		},
		{
			ID:                 types.NewJournalEntryID(),
			EntryNumber:        "JE-004",
			Classification:     journal.EntryClassAdjusting,
			AdjustingEntryType: journal.AdjustingEntryAudit,
			AdjustmentReason:   "External audit correction",
			TargetPeriodNumber: 13,
			CreatedBy:          "user1",
			CreatedAt:          now,
			Status:             journal.EntryPosted,
		},
		{
			ID:             types.NewJournalEntryID(),
			Classification: journal.EntryClassRegular,
		},
	}

	summary := SummarizeAdjustingEntriesForPreClose(entries)
	if summary.TotalAdjustingEntries != 3 {
		t.Fatalf("expected 3 adjusting entries, got %d", summary.TotalAdjustingEntries)
	}
	if summary.PendingDrafts != 1 {
		t.Fatalf("expected 1 draft, got %d", summary.PendingDrafts)
	}
	if summary.MissingAuditMetadata != 1 {
		t.Fatalf("expected 1 missing audit, got %d", summary.MissingAuditMetadata)
	}
	if summary.InvalidClassificationPeriod != 1 {
		t.Fatalf("invalid classification period count = %d, want 1", summary.InvalidClassificationPeriod)
	}
	foundPeriodEvidence := false
	for _, evidence := range summary.Evidence {
		if strings.Contains(evidence, "target period 13") {
			foundPeriodEvidence = true
			break
		}
	}
	if !foundPeriodEvidence {
		t.Fatalf("summary evidence does not explain the invalid target period: %v", summary.Evidence)
	}
	if summary.Ready() {
		t.Fatalf("summary should not be ready")
	}

	readySummary := SummarizeAdjustingEntriesForPreClose([]journal.JournalEntry{entries[0]})
	if !readySummary.Ready() {
		t.Fatalf("single adjusting entry with valid audit metadata should be ready")
	}

	auditEntry := entries[3]
	auditEntry.TargetPeriodNumber = 14
	readyAuditSummary := SummarizeAdjustingEntriesForPreClose([]journal.JournalEntry{auditEntry})
	if !readyAuditSummary.Ready() {
		t.Fatalf("audit adjustment in period 14 should be ready: %+v", readyAuditSummary)
	}
}
