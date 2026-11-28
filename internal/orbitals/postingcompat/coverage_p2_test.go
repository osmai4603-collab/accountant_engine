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

type coverageAttachment struct {
	validateErr error
	executeErr  error
	validated   bool
	executed    bool
}

func (a *coverageAttachment) AttachmentKind() string { return "coverage" }
func (a *coverageAttachment) ValidateAttachment(context.Context, JournalEntry) error {
	a.validated = true
	return a.validateErr
}
func (a *coverageAttachment) ExecuteAtomicMutation(context.Context, AtomicTransaction, JournalEntry) error {
	a.executed = true
	return a.executeErr
}

func coverageValidEntry(t *testing.T, companyID uuid.UUID) JournalEntry {
	t.Helper()
	entry, err := NewJournalEntry(uuid.New(), time.Now().UTC(), "tester", validCompatLines(t, companyID))
	if err != nil {
		t.Fatalf("NewJournalEntry: %v", err)
	}
	return entry
}

func coverageSingleLine(t *testing.T, companyID uuid.UUID, amount, currency string, debit bool) JournalEntryLine {
	t.Helper()
	money := mustCompatMoney(t, amount, currency)
	zero := mustCompatMoney(t, "0.00", currency)
	line := JournalEntryLine{
		CompanyID:           companyID,
		ID:                  uuid.New(),
		AccountID:           uuid.New(),
		TransactionAmount:   money,
		TransactionCurrency: currency,
		ExchangeRate:        decimal.NewFromInt(1),
	}
	if debit {
		line.FunctionalDebit = money
		line.FunctionalCredit = zero
	} else {
		line.FunctionalDebit = zero
		line.FunctionalCredit = money
	}
	return line
}

func TestCoveragePostingCompatEntryValidationBranches(t *testing.T) {
	companyID := uuid.New()
	base := coverageValidEntry(t, companyID)

	missingID := base
	missingID.ID = uuid.Nil
	if err := missingID.Validate(); !errors.Is(err, ErrInvalidEntry) {
		t.Fatalf("missing ID error = %v", err)
	}

	invalidLine := base
	invalidLine.Lines = append([]JournalEntryLine(nil), base.Lines...)
	invalidLine.Lines[0].AccountID = uuid.Nil
	if err := invalidLine.Validate(); !errors.Is(err, ErrInvalidEntry) {
		t.Fatalf("invalid line error = %v", err)
	}

	wrongCompany := base
	wrongCompany.Lines = append([]JournalEntryLine(nil), base.Lines...)
	wrongCompany.Lines[0].CompanyID = uuid.New()
	if err := wrongCompany.Validate(); !errors.Is(err, ErrInvalidEntry) {
		t.Fatalf("company mismatch error = %v", err)
	}

	negative := base
	negative.Lines = append([]JournalEntryLine(nil), base.Lines...)
	negative.Lines[0].FunctionalDebit = mustCompatMoney(t, "-1.00", "USD")
	if err := negative.Validate(); !errors.Is(err, ErrInvalidEntry) {
		t.Fatalf("negative amount error = %v", err)
	}

	twoSides := base
	twoSides.Lines = append([]JournalEntryLine(nil), base.Lines...)
	twoSides.Lines[0].FunctionalCredit = mustCompatMoney(t, "10.00", "USD")
	if err := twoSides.Validate(); !errors.Is(err, ErrLineMustHaveOneSide) {
		t.Fatalf("two-sided line error = %v", err)
	}

	noAmount := base
	noAmount.Lines = append([]JournalEntryLine(nil), base.Lines...)
	noAmount.Lines[0].FunctionalDebit = financialtypes.Money{}
	noAmount.Lines[0].FunctionalCredit = financialtypes.Money{}
	if err := noAmount.Validate(); !errors.Is(err, ErrInvalidEntry) {
		t.Fatalf("zero-value line error = %v", err)
	}

	mismatch := base
	mismatch.Lines = []JournalEntryLine{
		coverageSingleLine(t, companyID, "100.00", "USD", true),
		coverageSingleLine(t, companyID, "100.00", "EUR", false),
	}
	if err := mismatch.Validate(); !errors.Is(err, ErrInvalidEntry) {
		t.Fatalf("currency mismatch error = %v", err)
	}

	noCurrency := base
	noCurrency.Lines = []JournalEntryLine{
		coverageSingleLine(t, companyID, "100.00", "USD", true),
		coverageSingleLine(t, companyID, "100.00", "USD", false),
	}
	for i := range noCurrency.Lines {
		noCurrency.Lines[i].TransactionCurrency = "USD"
		noCurrency.Lines[i].TransactionAmount = mustCompatMoney(t, "100.00", "USD")
		noCurrency.Lines[i].FunctionalDebit = financialtypes.Money{}
		noCurrency.Lines[i].FunctionalCredit = financialtypes.Money{}
		noCurrency.Lines[i].Debit = financialtypes.Money{}
		noCurrency.Lines[i].Credit = financialtypes.Money{}
	}
	if err := noCurrency.Validate(); err == nil {
		t.Fatal("entry without functional currency must fail")
	}

	// Deprecated Debit/Credit aliases are used when functional amounts are absent.
	legacyLine := JournalEntryLine{
		CompanyID:           companyID,
		ID:                  uuid.New(),
		AccountID:           uuid.New(),
		Debit:               mustCompatMoney(t, "75.00", "EUR"),
		Credit:              mustCompatMoney(t, "0.00", "EUR"),
		TransactionCurrency: "EUR",
	}
	legacyCreditLine := JournalEntryLine{
		CompanyID:           companyID,
		ID:                  uuid.New(),
		AccountID:           uuid.New(),
		Debit:               mustCompatMoney(t, "0.00", "EUR"),
		Credit:              mustCompatMoney(t, "75.00", "EUR"),
		TransactionCurrency: "EUR",
	}
	legacy := base
	legacy.Lines = []JournalEntryLine{legacyLine, legacyCreditLine}
	if err := legacy.Validate(); err != nil {
		t.Fatalf("legacy debit/credit entry: %v", err)
	}
	if debit, credit, currency := LineFunctionalAmounts(legacyCreditLine); currency != "EUR" ||
		!debit.IsZero() || !credit.Equal(decimal.RequireFromString("75")) {
		t.Fatalf("LineFunctionalAmounts legacy = %s %s %s", debit, credit, currency)
	}

	invalidNewEntry := base
	invalidNewEntry.Lines = []JournalEntryLine{base.Lines[0]}
	if _, err := NewJournalEntry(uuid.New(), time.Now().UTC(), "tester", invalidNewEntry.Lines); !errors.Is(err, ErrMinimumLines) {
		t.Fatalf("NewJournalEntry validation error = %v", err)
	}
}

func TestCoveragePostingCompatNormalizationBranches(t *testing.T) {
	amount := mustCompatMoney(t, "25.00", "GBP")
	normalized, err := NormalizeLineCurrencies(JournalEntryLine{Debit: amount})
	if err != nil || normalized.TransactionCurrency != "GBP" || !normalized.ExchangeRate.Equal(decimal.NewFromInt(1)) {
		t.Fatalf("debit normalization = %+v, %v", normalized, err)
	}

	withPresetCurrency, err := NormalizeLineCurrencies(JournalEntryLine{
		TransactionCurrency: "GBP",
		AmountCurrency:      amount,
	})
	if err != nil || !withPresetCurrency.TransactionAmount.Equal(amount) {
		t.Fatalf("preset currency normalization = %+v, %v", withPresetCurrency, err)
	}

	credit := mustCompatMoney(t, "25.00", "CHF")
	withCredit, err := NormalizeLineCurrencies(JournalEntryLine{Credit: credit})
	if err != nil || !withCredit.FunctionalCredit.Equal(credit) {
		t.Fatalf("credit normalization = %+v, %v", withCredit, err)
	}

	withDebit, err := NormalizeLineCurrencies(JournalEntryLine{Debit: amount})
	if err != nil || !withDebit.FunctionalDebit.Equal(amount) {
		t.Fatalf("functional debit normalization = %+v, %v", withDebit, err)
	}
}

func TestCoveragePostingCompatEngineBranches(t *testing.T) {
	ctx := context.Background()
	companyID := uuid.New()
	entry := coverageValidEntry(t, companyID)

	engine := NewEngine()
	if engine == nil || engine.store == nil {
		t.Fatal("NewEngine did not build an in-memory store")
	}
	blank := entry
	blank.Status = ""
	blank.ID = uuid.New()
	if err := engine.CreateDraftContext(nil, blank); err != nil {
		t.Fatalf("CreateDraftContext with nil ctx: %v", err)
	}
	stored, err := engine.GetEntry(ctx, companyID, blank.ID)
	if err != nil || stored.Status != EntryDraft {
		t.Fatalf("stored blank-status entry = %+v, %v", stored, err)
	}

	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := engine.CreateDraftContext(cancelled, entry); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled CreateDraftContext = %v", err)
	}
	if _, err := engine.GetEntry(cancelled, companyID, entry.ID); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled GetEntry = %v", err)
	}

	noStore := &Engine{}
	if err := noStore.CreateDraftContext(ctx, entry); err == nil {
		t.Fatal("engine without store must fail")
	}
	if err := noStore.PostEntryWithAttachmentContext(ctx, &entry, "poster"); !errors.Is(err, ErrInvalidEntry) {
		t.Fatalf("engine without store posting = %v", err)
	}
	if _, err := noStore.GetEntry(ctx, companyID, entry.ID); !errors.Is(err, ErrEntryNotFound) {
		t.Fatalf("engine without store lookup = %v", err)
	}

	withStore := NewEngineWithStore(nil, &mockOpenLineStore{})
	if withStore.store == nil {
		t.Fatal("nil store should be replaced by memory store")
	}

	postable := coverageValidEntry(t, companyID)
	if err := engine.PostEntryWithAttachmentContext(ctx, nil, "poster"); !errors.Is(err, ErrInvalidEntry) {
		t.Fatalf("nil entry error = %v", err)
	}
	if err := engine.PostEntryWithAttachmentContext(nil, &postable, "poster"); err != nil {
		t.Fatalf("nil context posting = %v", err)
	}
	if err := engine.PostEntryWithAttachmentContext(cancelled, &postable, "poster"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled posting = %v", err)
	}

	invalid := coverageValidEntry(t, companyID)
	invalid.Lines = nil
	if err := engine.PostEntryWithAttachmentContext(ctx, &invalid, "poster"); !errors.Is(err, ErrMinimumLines) {
		t.Fatalf("invalid entry posting = %v", err)
	}

	validationFailure := errors.New("invalid attachment")
	attachment := &coverageAttachment{validateErr: validationFailure}
	rejected := coverageValidEntry(t, companyID)
	if err := engine.PostEntryWithAttachmentContext(ctx, &rejected, "poster", nil, attachment); !errors.Is(err, validationFailure) {
		t.Fatalf("attachment validation error = %v", err)
	}
	if !attachment.validated || attachment.executed {
		t.Fatalf("attachment lifecycle = %+v", attachment)
	}

	mutationFailure := errors.New("mutation failed")
	mutating := &coverageAttachment{executeErr: mutationFailure}
	toPost := coverageValidEntry(t, companyID)
	if err := engine.PostEntryWithAttachmentContext(ctx, &toPost, "poster", nil, mutating); !errors.Is(err, mutationFailure) {
		t.Fatalf("attachment mutation error = %v", err)
	}
	if !mutating.executed || toPost.Status != EntryDraft {
		t.Fatalf("failed mutation changed entry: %+v %+v", toPost, mutating)
	}

	success := coverageValidEntry(t, companyID)
	successful := &coverageAttachment{}
	if err := engine.PostEntryWithAttachmentContext(ctx, &success, "poster", successful, nil); err != nil {
		t.Fatalf("posting with nil attachment: %v", err)
	}
	if !successful.executed || success.Status != EntryPosted {
		t.Fatalf("posting result = %+v, attachment = %+v", success, successful)
	}
}

func TestCoveragePostingCompatOpenLineAttachmentBranches(t *testing.T) {
	ctx := context.Background()
	companyID := uuid.New()
	entry := coverageValidEntry(t, companyID)
	attachment := &OpenLineLedgerAttachment{Attachment: OpenLineAttachment{
		ID:               uuid.New(),
		CompanyID:        companyID,
		JournalEntryID:   entry.ID,
		PartnerID:        uuid.New(),
		ControlAccountID: uuid.New(),
	}}
	if attachment.AttachmentKind() != "open_line" {
		t.Fatalf("AttachmentKind = %q", attachment.AttachmentKind())
	}
	if err := attachment.ValidateAttachment(ctx, entry); err != nil {
		t.Fatalf("valid open-line attachment: %v", err)
	}
	invalidEntry := entry
	invalidEntry.CompanyID = uuid.Nil
	if err := attachment.ValidateAttachment(ctx, invalidEntry); !errors.Is(err, ErrInvalidEntry) {
		t.Fatalf("invalid open-line attachment = %v", err)
	}
	tx := atomicTx{state: struct{}{}}
	if err := attachment.ExecuteAtomicMutation(ctx, tx, entry); !errors.Is(err, ErrUnsupportedMutationBackend) {
		t.Fatalf("unsupported backend error = %v", err)
	}

	mem := newMemoryStore()
	if err := mem.SaveDraft(cancelledContext(), entry); !errors.Is(err, context.Canceled) {
		t.Fatalf("memory store save error = %v", err)
	}
	if _, err := mem.GetEntry(cancelledContext(), companyID, entry.ID); !errors.Is(err, context.Canceled) {
		t.Fatalf("memory store get error = %v", err)
	}
}

func cancelledContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}
