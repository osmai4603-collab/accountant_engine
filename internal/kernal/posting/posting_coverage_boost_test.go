package posting

import (
	"errors"
	"testing"
	"time"

	"github.com/osmai4603-collab/accounting_engine/internal/kernal/journal"
	"github.com/osmai4603-collab/accounting_engine/internal/kernal/types"
	apiTypes "github.com/osmai4603-collab/accounting_engine/internal/kernal/types"
	"github.com/shopspring/decimal"
)

func TestPostingValidationLineAndDoubleEntryBranches(t *testing.T) {
	accID1 := types.NewAccountID()
	accID2 := types.NewAccountID()
	lineID1 := types.NewJournalEntryLineID()
	lineID2 := types.NewJournalEntryLineID()
	now := time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC)
	companyID := types.NewCompanyID()
	journalID := types.NewJournalID()

	m100USD, _ := types.NewMoney(decimal.NewFromInt(100), "USD")
	m50USD, _ := types.NewMoney(decimal.NewFromInt(50), "USD")
	zeroUSD, _ := types.Zero("USD")

	// 1. ValidateEntryLine: zero accountID
	if err := ValidateEntryLine(journal.EntryLine{ID: lineID1}, nil); !errors.Is(err, ErrInvalidEntry) {
		t.Fatalf("zero accountID error = %v, want ErrInvalidEntry", err)
	}

	// 2. ValidateEntryLine: missing transaction currency
	lineMissingTxCurr := journal.EntryLine{
		ID:                lineID1,
		AccountID:         accID1,
		TransactionAmount: m100USD,
	}
	if err := ValidateEntryLine(lineMissingTxCurr, nil); !errors.Is(err, ErrMissingTransactionCurrency) {
		t.Fatalf("missing tx curr error = %v, want ErrMissingTransactionCurrency", err)
	}

	// 3. ValidateEntryLine: missing functional currency
	lineMissingFuncCurr := journal.EntryLine{
		ID:                  lineID1,
		AccountID:           accID1,
		TransactionCurrency: "USD",
		TransactionAmount:   m100USD,
		FunctionalDebit:     types.Money{},
		FunctionalCredit:    zeroUSD,
	}
	if err := ValidateEntryLine(lineMissingFuncCurr, nil); !errors.Is(err, ErrMissingFunctionalCurrency) {
		t.Fatalf("missing func curr error = %v, want ErrMissingFunctionalCurrency", err)
	}

	// 4. ValidateEntryLine: FunctionalDebit != FunctionalCredit currency
	m100EUR, _ := types.NewMoney(decimal.NewFromInt(100), "EUR")
	lineMismatchedSides := journal.EntryLine{
		ID:                  lineID1,
		AccountID:           accID1,
		TransactionCurrency: "USD",
		TransactionAmount:   m100USD,
		FunctionalDebit:     m100USD,
		FunctionalCredit:    m100EUR,
	}
	if err := ValidateEntryLine(lineMismatchedSides, nil); !errors.Is(err, apiTypes.ErrCurrencyMismatch) {
		t.Fatalf("mismatched debit/credit curr error = %v, want ErrCurrencyMismatch", err)
	}

	// 5. ValidateEntryLine: amount mismatch between calculated and actual
	lineAmtMismatch := journal.EntryLine{
		ID:                  lineID1,
		AccountID:           accID1,
		TransactionCurrency: "USD",
		TransactionAmount:   m100USD,
		ExchangeRate:        decimal.NewFromInt(1),
		FunctionalDebit:     m50USD, // expected 100
		FunctionalCredit:    zeroUSD,
	}
	if err := ValidateEntryLine(lineAmtMismatch, nil); !errors.Is(err, apiTypes.ErrInvalidAmount) {
		t.Fatalf("amount mismatch error = %v, want ErrInvalidAmount", err)
	}

	// 6. ValidateEntryDoubleEntry: nil entry
	if err := ValidateEntryDoubleEntry(nil, nil); !errors.Is(err, ErrInvalidEntry) {
		t.Fatalf("nil entry error = %v, want ErrInvalidEntry", err)
	}

	// 7. ValidateEntryDoubleEntry: missing JournalID
	entryMissingJournal := &journal.JournalEntry{
		ID:            types.NewJournalEntryID(),
		CompanyID:     companyID,
		EffectiveDate: now,
		Lines: []journal.EntryLine{
			{ID: lineID1, AccountID: accID1, TransactionCurrency: "USD", TransactionAmount: m100USD, ExchangeRate: decimal.NewFromInt(1), FunctionalDebit: m100USD, FunctionalCredit: zeroUSD},
			{ID: lineID2, AccountID: accID2, TransactionCurrency: "USD", TransactionAmount: m100USD, ExchangeRate: decimal.NewFromInt(1), FunctionalDebit: zeroUSD, FunctionalCredit: m100USD},
		},
	}
	if err := ValidateEntryDoubleEntry(entryMissingJournal, nil); !errors.Is(err, ErrMissingJournalID) {
		t.Fatalf("missing journalID error = %v, want ErrMissingJournalID", err)
	}

	// 8. ValidateEntryDoubleEntry: zero EffectiveDate and PostingDate
	entryZeroDates := &journal.JournalEntry{
		ID:        types.NewJournalEntryID(),
		CompanyID: companyID,
		JournalID: journalID,
		Lines: []journal.EntryLine{
			{ID: lineID1, AccountID: accID1, TransactionCurrency: "USD", TransactionAmount: m100USD, ExchangeRate: decimal.NewFromInt(1), FunctionalDebit: m100USD, FunctionalCredit: zeroUSD},
			{ID: lineID2, AccountID: accID2, TransactionCurrency: "USD", TransactionAmount: m100USD, ExchangeRate: decimal.NewFromInt(1), FunctionalDebit: zeroUSD, FunctionalCredit: m100USD},
		},
	}
	if err := ValidateEntryDoubleEntry(entryZeroDates, nil); !errors.Is(err, ErrZeroEntryDate) {
		t.Fatalf("zero dates error = %v, want ErrZeroEntryDate", err)
	}

	// 9. ValidateEntryDoubleEntry: all debits no credits (hasDebit && !hasCredit)
	entryNoCredits := &journal.JournalEntry{
		ID:             types.NewJournalEntryID(),
		CompanyID:      companyID,
		JournalID:      journalID,
		Classification: journal.EntryClassRegular,
		EffectiveDate:  now,
		Lines: []journal.EntryLine{
			{ID: lineID1, AccountID: accID1, TransactionCurrency: "USD", TransactionAmount: m100USD, ExchangeRate: decimal.NewFromInt(1), FunctionalDebit: m100USD, FunctionalCredit: zeroUSD},
			{ID: lineID2, AccountID: accID2, TransactionCurrency: "USD", TransactionAmount: m100USD, ExchangeRate: decimal.NewFromInt(1), FunctionalDebit: m100USD, FunctionalCredit: zeroUSD},
		},
	}
	if err := ValidateEntryDoubleEntry(entryNoCredits, nil); !errors.Is(err, ErrMinimumLines) {
		t.Fatalf("no credits error = %v, want ErrMinimumLines", err)
	}

	// 10. NewJournalEntryAt with missing journalID or zero date
	if _, err := NewJournalEntryAt(companyID, types.JournalID{}, now, "desc", nil, now); !errors.Is(err, ErrMissingJournalID) {
		t.Fatalf("missing journalID error = %v, want ErrMissingJournalID", err)
	}
	if _, err := NewJournalEntryAt(companyID, journalID, time.Time{}, "desc", nil, now); !errors.Is(err, ErrZeroEntryDate) {
		t.Fatalf("zero effective date error = %v, want ErrZeroEntryDate", err)
	}

	entryNew, err := NewJournalEntryAt(companyID, journalID, now, "desc", []journal.EntryLine{
		{ID: lineID1, AccountID: accID1, TransactionCurrency: "USD", TransactionAmount: m100USD, ExchangeRate: decimal.NewFromInt(1), FunctionalDebit: m100USD, FunctionalCredit: zeroUSD},
		{ID: lineID2, AccountID: accID2, TransactionCurrency: "USD", TransactionAmount: m100USD, ExchangeRate: decimal.NewFromInt(1), FunctionalDebit: zeroUSD, FunctionalCredit: m100USD},
	}, now)
	if err != nil {
		t.Fatalf("NewJournalEntryAt: %v", err)
	}
	if entryNew.CreatedAt.IsZero() {
		t.Fatal("expected non-zero CreatedAt in NewJournalEntryAt")
	}

	// 11. BuildReversalEntryAt with invalid status
	origDraft := entryNew
	origDraft.Status = journal.EntryDraft
	if _, err := BuildReversalEntryAt(origDraft, now, "actor", "reason", now); !errors.Is(err, ErrEntryNotPosted) {
		t.Fatalf("draft reversal error = %v, want ErrEntryNotPosted", err)
	}

	// 12. BuildReversalEntryAt with empty actor or reason
	origPosted := entryNew
	origPosted.Status = journal.EntryPosted
	if _, err := BuildReversalEntryAt(origPosted, now, "", "reason", now); !errors.Is(err, ErrReversalActorRequired) {
		t.Fatalf("empty actor reversal error = %v, want ErrReversalActorRequired", err)
	}
	if _, err := BuildReversalEntryAt(origPosted, now, "actor", "", now); !errors.Is(err, ErrReversalReasonRequired) {
		t.Fatalf("empty reason reversal error = %v, want ErrReversalReasonRequired", err)
	}
}
