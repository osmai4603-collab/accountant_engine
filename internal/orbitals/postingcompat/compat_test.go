package posting

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	financialtypes "github.com/osmai4603-collab/accounting_engine/internal/kernel/types"
	"github.com/shopspring/decimal"
)

type mockOpenLineStore struct {
	saved []OpenLineAttachment
	err   error
}

func (m *mockOpenLineStore) SaveOpenLineFromAttachment(_ context.Context, attachment OpenLineAttachment, _ JournalEntry) error {
	if m.err != nil {
		return m.err
	}
	m.saved = append(m.saved, attachment)
	return nil
}

func mustCompatMoney(t *testing.T, amount, currency string) financialtypes.Money {
	t.Helper()
	d, err := decimal.NewFromString(amount)
	if err != nil {
		t.Fatalf("decimal %q: %v", amount, err)
	}
	m, err := financialtypes.NewMoney(d, currency)
	if err != nil {
		t.Fatalf("money %q %q: %v", amount, currency, err)
	}
	return m
}

func validCompatLines(t *testing.T, companyID uuid.UUID) []JournalEntryLine {
	t.Helper()
	amt := mustCompatMoney(t, "100.00", "USD")
	zero := mustCompatMoney(t, "0.00", "USD")
	return []JournalEntryLine{
		{
			CompanyID:           companyID,
			ID:                  uuid.New(),
			AccountID:           uuid.New(),
			TransactionAmount:   amt,
			TransactionCurrency: "USD",
			ExchangeRate:        decimal.NewFromInt(1),
			FunctionalDebit:     amt,
			FunctionalCredit:    zero,
		},
		{
			CompanyID:           companyID,
			ID:                  uuid.New(),
			AccountID:           uuid.New(),
			TransactionAmount:   amt,
			TransactionCurrency: "USD",
			ExchangeRate:        decimal.NewFromInt(1),
			FunctionalDebit:     zero,
			FunctionalCredit:    amt,
		},
	}
}

func TestNewJournalEntryAndValidation(t *testing.T) {
	companyID := uuid.New()
	journalID := uuid.New()
	now := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	lines := validCompatLines(t, companyID)

	if _, err := NewJournalEntry(uuid.Nil, now, "u1", lines); !errors.Is(err, ErrMissingJournalID) {
		t.Fatalf("expected ErrMissingJournalID, got %v", err)
	}
	if _, err := NewJournalEntry(journalID, time.Time{}, "u1", lines); !errors.Is(err, ErrZeroEntryDate) {
		t.Fatalf("expected ErrZeroEntryDate, got %v", err)
	}

	mismatchLines := append([]JournalEntryLine(nil), lines...)
	mismatchLines[1].CompanyID = uuid.New()
	if _, err := NewJournalEntry(journalID, now, "u1", mismatchLines); !errors.Is(err, ErrCompanyMismatch) {
		t.Fatalf("expected ErrCompanyMismatch, got %v", err)
	}

	entry, err := NewJournalEntry(journalID, now, "u1", lines)
	if err != nil {
		t.Fatalf("NewJournalEntry: %v", err)
	}
	if entry.Status != EntryDraft || entry.CompanyID != companyID {
		t.Fatalf("unexpected entry header: %+v", entry)
	}

	// Balance mismatch
	badBalance := entry
	badBalance.Lines = append([]JournalEntryLine(nil), lines...)
	badBalance.Lines[1].FunctionalCredit = mustCompatMoney(t, "50.00", "USD")
	if err := badBalance.Validate(); !errors.Is(err, ErrFunctionalBalanceMismatch) {
		t.Fatalf("expected ErrFunctionalBalanceMismatch, got %v", err)
	}

	// Adjusting metadata check
	adj := entry
	adj.Classification = EntryClassAdjusting
	if err := adj.Validate(); !errors.Is(err, ErrAdjustingEntryMetadataRequired) {
		t.Fatalf("expected ErrAdjustingEntryMetadataRequired, got %v", err)
	}

	// Less than 2 lines
	short := entry
	short.Lines = lines[:1]
	if err := short.Validate(); !errors.Is(err, ErrMinimumLines) {
		t.Fatalf("expected ErrMinimumLines, got %v", err)
	}
}

func TestLineHelpersAndNormalization(t *testing.T) {
	amt := mustCompatMoney(t, "42.00", "EUR")
	zero := mustCompatMoney(t, "0.00", "EUR")
	l1, err := NormalizeLineCurrencies(JournalEntryLine{
		ID:               uuid.New(),
		AccountID:        uuid.New(),
		AmountCurrency:   amt,
		Debit:            amt,
		Credit:           zero,
		FunctionalCredit: zero,
	})
	if err != nil {
		t.Fatalf("NormalizeLineCurrencies: %v", err)
	}
	if l1.TransactionCurrency != "EUR" || !l1.ExchangeRate.Equal(decimal.NewFromInt(1)) {
		t.Fatalf("unexpected normalized line: %+v", l1)
	}

	l2, _ := NormalizeLineCurrencies(JournalEntryLine{
		ID:        uuid.New(),
		AccountID: uuid.New(),
		Credit:    amt,
	})
	if l2.TransactionCurrency != "EUR" {
		t.Fatalf("expected EUR from Credit, got %q", l2.TransactionCurrency)
	}

	d, c, cur := LineFunctionalAmounts(JournalEntryLine{Debit: amt})
	if cur != "EUR" || !d.Equal(amt.Amount()) || !c.IsZero() {
		t.Fatalf("unexpected LineFunctionalAmounts: %s %s %s", d, c, cur)
	}

	p1 := uuid.New()
	p2 := p1
	l1.PartnerID = &p1
	l1Copy := l1
	l1Copy.PartnerID = &p2
	if !l1.Equal(l1Copy) {
		t.Fatal("expected Equal lines to return true")
	}
	l1Copy.PartnerID = nil
	if l1.Equal(l1Copy) {
		t.Fatal("expected non-equal PartnerID to return false")
	}

	badLine := JournalEntryLine{AccountID: uuid.New(), Debit: amt, Credit: amt, TransactionCurrency: "EUR"}
	if err := badLine.Validate(); !errors.Is(err, ErrLineMustHaveOneSide) {
		t.Fatalf("expected ErrLineMustHaveOneSide, got %v", err)
	}
	if err := (JournalEntryLine{}).Validate(); !errors.Is(err, ErrInvalidEntry) {
		t.Fatalf("expected ErrInvalidEntry, got %v", err)
	}
	if err := (JournalEntryLine{AccountID: uuid.New()}).Validate(); !errors.Is(err, ErrMissingTransactionCurrency) {
		t.Fatalf("expected ErrMissingTransactionCurrency, got %v", err)
	}
	if err := (JournalEntryLine{AccountID: uuid.New(), TransactionCurrency: "USD", ExchangeRate: decimal.NewFromInt(-1)}).Validate(); !errors.Is(err, ErrInvalidExchangeRate) {
		t.Fatalf("expected ErrInvalidExchangeRate, got %v", err)
	}
}

func TestEngineDraftPostingAndAttachments(t *testing.T) {
	ctx := context.Background()
	companyID := uuid.New()
	journalID := uuid.New()
	lines := validCompatLines(t, companyID)
	entry, err := NewJournalEntry(journalID, time.Now().UTC(), "tester", lines)
	if err != nil {
		t.Fatalf("NewJournalEntry: %v", err)
	}

	store := &mockOpenLineStore{}
	eng := NewEngineWithStore(nil, store)
	if err := eng.CreateDraftContext(ctx, entry); err != nil {
		t.Fatalf("CreateDraftContext: %v", err)
	}
	got, err := eng.GetEntry(ctx, companyID, entry.ID)
	if err != nil || got.ID != entry.ID {
		t.Fatalf("GetEntry: got=%+v err=%v", got, err)
	}
	if _, err := eng.GetEntry(ctx, uuid.New(), entry.ID); !errors.Is(err, ErrEntryNotFound) {
		t.Fatalf("expected ErrEntryNotFound for wrong company, got %v", err)
	}

	att := OpenLineAttachment{
		ID:               uuid.New(),
		CompanyID:        companyID,
		JournalEntryID:   entry.ID,
		PartnerID:        uuid.New(),
		ControlAccountID: lines[0].AccountID,
		Kind:             "AR",
		Amount:           lines[0].TransactionAmount,
		FunctionalAmount: lines[0].FunctionalDebit,
	}
	if err := eng.PostEntryWithOpenLineContext(ctx, &entry, att, "poster"); err != nil {
		t.Fatalf("PostEntryWithOpenLineContext: %v", err)
	}
	if entry.Status != EntryPosted || len(store.saved) != 1 {
		t.Fatalf("expected posted entry and 1 saved attachment, got status=%s saved=%d", entry.Status, len(store.saved))
	}
	if err := eng.PostEntryWithOpenLineContext(ctx, &entry, att, "poster"); !errors.Is(err, ErrInvalidEntryState) {
		t.Fatalf("expected ErrInvalidEntryState on re-post, got %v", err)
	}

	// Unsupported mutation backend & SQL helpers
	tx := atomicTx{state: store}
	if err := tx.Exec(ctx, "SELECT 1"); err == nil {
		t.Fatal("expected Exec error")
	}
	if err := tx.QueryRow(ctx, "SELECT 1").Scan(); err == nil {
		t.Fatal("expected QueryRow.Scan error")
	}
	if err := tx.CustomMutation(ctx, "sql", func(any) error { return nil }); !errors.Is(err, ErrUnsupportedMutationBackend) {
		t.Fatalf("expected ErrUnsupportedMutationBackend, got %v", err)
	}

	// Nil engine checks
	var nilEng *Engine
	if err := nilEng.CreateDraftContext(ctx, entry); err == nil {
		t.Fatal("expected error on nil engine CreateDraftContext")
	}
	if _, err := nilEng.GetEntry(ctx, companyID, entry.ID); !errors.Is(err, ErrEntryNotFound) {
		t.Fatalf("expected ErrEntryNotFound on nil engine, got %v", err)
	}
}
