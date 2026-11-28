package subledgers

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/osmai4603-collab/accounting_engine/internal/kernel/masterdata"
	financialtypes "github.com/osmai4603-collab/accounting_engine/internal/kernel/types"
	posting "github.com/osmai4603-collab/accounting_engine/internal/orbitals/postingcompat"
	ledger "github.com/osmai4603-collab/accounting_engine/pkg/api/v1/journal"
	apisubledger "github.com/osmai4603-collab/accounting_engine/pkg/api/v1/subledger"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	publicapplication "github.com/osmai4603-collab/accounting_engine/pkg/spi/application"
	publicsubledger "github.com/osmai4603-collab/accounting_engine/pkg/spi/subledger"
	"github.com/shopspring/decimal"
)

type covPostedRepo struct {
	*MemoryRepository
	account    masterdata.Account
	accountErr error
	entry      posting.JournalEntry
	entryErr   error
	saveErr    error
}

func (r *covPostedRepo) GetAccountByIDForCompany(
	context.Context, uuid.UUID, uuid.UUID,
) (masterdata.Account, error) {
	return r.account, r.accountErr
}

func (r *covPostedRepo) GetEntryByIDForCompany(
	context.Context, uuid.UUID, uuid.UUID,
) (posting.JournalEntry, error) {
	return r.entry, r.entryErr
}

func (r *covPostedRepo) SaveOpenLineForPostedEntry(context.Context, OpenLine) error {
	return r.saveErr
}

func covControlEntry(
	companyID, journalID, controlID, partnerID, counterpartID uuid.UUID,
	kind LineKind, status posting.EntryStatus,
) posting.JournalEntry {
	amount := covMoneyRaw("100", "USD")
	zero := covMoneyRaw("0", "USD")
	control := posting.JournalEntryLine{
		ID: uuid.New(), CompanyID: companyID, AccountID: controlID, PartnerID: &partnerID,
		Debit: amount, Credit: zero, FunctionalDebit: amount, FunctionalCredit: zero,
	}
	other := posting.JournalEntryLine{
		ID: uuid.New(), CompanyID: companyID, AccountID: counterpartID,
		Debit: zero, Credit: amount, FunctionalDebit: zero, FunctionalCredit: amount,
	}
	if kind == LinePayable {
		control.Debit, control.Credit = zero, amount
		control.FunctionalDebit, control.FunctionalCredit = zero, amount
		other.Debit, other.Credit = amount, zero
		other.FunctionalDebit, other.FunctionalCredit = amount, zero
	}
	return posting.JournalEntry{
		ID: journalID, CompanyID: companyID, Status: status,
		EntryDate: covDate(2), Date: covDate(2),
		Lines: []posting.JournalEntryLine{control, other},
	}
}

func covPostedOpenLine(
	companyID, controlID, journalID, partnerID uuid.UUID, kind LineKind,
) OpenLine {
	amount := covMoneyRaw("100", "USD")
	return OpenLine{
		CompanyID: companyID, ID: uuid.New(), JournalEntryID: journalID, PartnerID: &partnerID,
		ControlAccountID: controlID, Kind: kind,
		OriginalAmount: amount, ResidualAmount: amount,
		FunctionalOriginalAmount: amount, FunctionalResidualAmount: amount,
		PostingDate: covDate(2), DocumentDate: covDate(2), DueDate: covDate(3),
	}
}

func TestCovNewPostingOpenLineAttachment(t *testing.T) {
	companyID, lineID, journalID, partnerID, accountID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	valid := OpenLine{
		CompanyID: companyID, ID: lineID, JournalEntryID: journalID, PartnerID: &partnerID,
		ControlAccountID: accountID, Kind: LineReceivable,
		OriginalAmount: covMoney(t, "100", "USD"), ResidualAmount: covMoney(t, "100", "USD"),
		FunctionalOriginalAmount: covMoney(t, "100", "USD"),
		PostingDate:              covDate(2), DocumentDate: covDate(1), DueDate: covDate(3), Reference: "INV-1",
	}
	attachment, err := NewPostingOpenLineAttachment(valid)
	if err != nil {
		t.Fatalf("valid attachment: %v", err)
	}
	if attachment.ID != lineID || attachment.Kind != string(LineReceivable) || attachment.PartnerID != partnerID ||
		!attachment.Amount.Equal(valid.OriginalAmount) {
		t.Fatalf("unexpected attachment: %+v", attachment)
	}

	invalid := []struct {
		name   string
		mutate func(*OpenLine)
	}{
		{"nil line id", func(l *OpenLine) { l.ID = uuid.Nil }},
		{"nil company", func(l *OpenLine) { l.CompanyID = uuid.Nil }},
		{"nil journal", func(l *OpenLine) { l.JournalEntryID = uuid.Nil }},
		{"nil partner", func(l *OpenLine) { l.PartnerID = nil }},
		{"nil partner id", func(l *OpenLine) { l.PartnerID = covUUIDPtr(uuid.Nil) }},
		{"nil account", func(l *OpenLine) { l.ControlAccountID = uuid.Nil }},
		{"bad kind", func(l *OpenLine) { l.Kind = LineKind("other") }},
		{"empty currency", func(l *OpenLine) { l.OriginalAmount = financialtypes.Money{} }},
		{"negative", func(l *OpenLine) { l.OriginalAmount = covMoney(t, "-1", "USD") }},
		{"zero", func(l *OpenLine) { l.OriginalAmount = covMoney(t, "0", "USD") }},
		{"residual mismatch", func(l *OpenLine) { l.ResidualAmount = covMoney(t, "99", "USD") }},
	}
	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			candidate := valid
			tc.mutate(&candidate)
			if _, err := NewPostingOpenLineAttachment(candidate); !errors.Is(err, ErrInvalidOpenLine) {
				t.Fatalf("error = %v, want invalid open line", err)
			}
		})
	}
}

func TestCovValidatePostedOpenLine(t *testing.T) {
	companyID, controlID, journalID, partnerID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	account := covAccount(companyID, controlID)
	line := covPostedOpenLine(companyID, controlID, journalID, partnerID, LineReceivable)
	entry := covControlEntry(companyID, journalID, controlID, partnerID, uuid.New(), LineReceivable, posting.EntryPosted)

	if err := ValidatePostedOpenLine(line, account, entry); err != nil {
		t.Fatalf("valid posted line: %v", err)
	}
	if err := ValidatePostedOpenLine(line, account, covControlEntry(companyID, journalID, controlID, partnerID,
		uuid.New(), LineReceivable, posting.EntryDraft)); !errors.Is(err, posting.ErrEntryNotPosted) {
		t.Fatalf("draft entry error = %v", err)
	}

	notControl := account
	notControl.IsControlAccount = false
	if err := ValidatePostedOpenLine(line, notControl, entry); !errors.Is(err, ErrInvalidControlAccount) {
		t.Fatalf("non-control account error = %v", err)
	}
	notReconcilable := account
	notReconcilable.IsReconcilable = false
	if err := ValidatePostedOpenLine(line, notReconcilable, entry); !errors.Is(err, ErrInvalidControlAccount) {
		t.Fatalf("non-reconcilable account error = %v", err)
	}
	wrongType := account
	wrongType.Type = masterdata.AccountTypeLiability
	if err := ValidatePostedOpenLine(line, wrongType, entry); !errors.Is(err, ErrInvalidControlAccount) {
		t.Fatalf("wrong account type error = %v", err)
	}
	payableAccount := covAccount(companyID, controlID)
	payableAccount.Type = masterdata.AccountTypeLiability
	payableLine := covPostedOpenLine(companyID, controlID, journalID, partnerID, LinePayable)
	payableEntry := covControlEntry(companyID, journalID, controlID, partnerID, uuid.New(), LinePayable, posting.EntryPosted)
	if err := ValidatePostedOpenLine(payableLine, payableAccount, payableEntry); err != nil {
		t.Fatalf("valid payable line: %v", err)
	}
	if err := ValidatePostedOpenLine(payableLine, payableAccount, entry); !errors.Is(err, ErrInvalidOpenLine) {
		t.Fatalf("payable line with debit-only entry error = %v", err)
	}
	wrongPayableType := payableAccount
	wrongPayableType.Type = masterdata.AccountTypeAsset
	if err := ValidatePostedOpenLine(payableLine, wrongPayableType, payableEntry); !errors.Is(err, ErrInvalidControlAccount) {
		t.Fatalf("payable line with asset account error = %v", err)
	}

	cases := []struct {
		name   string
		mutate func(*OpenLine)
	}{
		{"nil company", func(l *OpenLine) { l.CompanyID = uuid.Nil }},
		{"nil id", func(l *OpenLine) { l.ID = uuid.Nil }},
		{"nil control account", func(l *OpenLine) { l.ControlAccountID = uuid.Nil }},
		{"nil journal", func(l *OpenLine) { l.JournalEntryID = uuid.Nil }},
		{"nil partner", func(l *OpenLine) { l.PartnerID = nil }},
		{"nil partner id", func(l *OpenLine) { l.PartnerID = covUUIDPtr(uuid.Nil) }},
		{"wrong account", func(l *OpenLine) { l.ControlAccountID = uuid.New() }},
		{"wrong journal", func(l *OpenLine) { l.JournalEntryID = uuid.New() }},
		{"bad kind", func(l *OpenLine) { l.Kind = LineKind("other") }},
		{"no currency", func(l *OpenLine) { l.OriginalAmount = financialtypes.Money{} }},
		{"negative amount", func(l *OpenLine) { l.OriginalAmount = covMoney(t, "-1", "USD") }},
		{"zero amount", func(l *OpenLine) { l.OriginalAmount = covMoney(t, "0", "USD") }},
		{"residual mismatch", func(l *OpenLine) { l.ResidualAmount = covMoney(t, "99", "USD") }},
		{"posted amount mismatch", func(l *OpenLine) { l.OriginalAmount = covMoney(t, "200", "USD"); l.ResidualAmount = l.OriginalAmount }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			candidate := line
			tc.mutate(&candidate)
			if err := ValidatePostedOpenLine(candidate, account, entry); !errors.Is(err, ErrInvalidOpenLine) {
				t.Fatalf("error = %v, want invalid open line", err)
			}
		})
	}

	mixed := entry
	mixed.Lines[0].AmountCurrency = covMoney(t, "100", "EUR")
	if err := ValidatePostedOpenLine(line, account, mixed); !errors.Is(err, ErrMixedCurrencyReconciliation) {
		t.Fatalf("mixed currency error = %v", err)
	}

	dateEntry := entry
	dateEntry.Date = covDate(9)
	dateEntry.EntryDate = time.Time{}
	line.PostingDate = dateEntry.Date
	if got := entryPostingDate(dateEntry); !got.Equal(dateEntry.Date) {
		t.Fatalf("posting date fallback = %v", got)
	}
}

func TestCovPostedControlAmounts(t *testing.T) {
	companyID, controlID, partnerID := uuid.New(), uuid.New(), uuid.New()
	line := covPostedOpenLine(companyID, controlID, uuid.New(), partnerID, LineReceivable)
	entry := covControlEntry(companyID, uuid.New(), controlID, partnerID, uuid.New(), LineReceivable, posting.EntryPosted)

	amount, err := postedControlAmount(entry, line)
	if err != nil || !amount.Equal(covMoney(t, "100", "USD")) {
		t.Fatalf("control amount = %v err=%v", amount, err)
	}
	functional, err := postedFunctionalControlAmount(entry, line)
	if err != nil || !functional.Equal(covMoney(t, "100", "USD")) {
		t.Fatalf("functional amount = %v err=%v", functional, err)
	}

	noFunctional := entry
	noFunctional.Lines[0].FunctionalDebit = financialtypes.Money{}
	noFunctional.Lines[0].FunctionalCredit = financialtypes.Money{}
	functional, err = postedFunctionalControlAmount(noFunctional, line)
	if err != nil || !functional.Equal(covMoney(t, "100", "USD")) {
		t.Fatalf("functional fallback = %v err=%v", functional, err)
	}

	payableLine := covPostedOpenLine(companyID, controlID, uuid.New(), partnerID, LinePayable)
	payableEntry := covControlEntry(companyID, uuid.New(), controlID, partnerID, uuid.New(), LinePayable, posting.EntryPosted)
	functional, err = postedFunctionalControlAmount(payableEntry, payableLine)
	if err != nil || !functional.Equal(covMoney(t, "100", "USD")) {
		t.Fatalf("payable functional amount = %v err=%v", functional, err)
	}
	amount, err = postedControlAmount(payableEntry, payableLine)
	if err != nil || !amount.Equal(covMoney(t, "100", "USD")) {
		t.Fatalf("payable control amount = %v err=%v", amount, err)
	}

	mixed := entry
	mixed.Lines = append(mixed.Lines, posting.JournalEntryLine{
		AccountID: controlID, PartnerID: &partnerID,
		FunctionalDebit: covMoney(t, "1", "EUR"),
	})
	if _, err := postedFunctionalControlAmount(mixed, line); !errors.Is(err, ErrControlBalanceCurrencyMismatch) {
		t.Fatalf("functional mixed currency error = %v", err)
	}
	if _, err := postedFunctionalControlAmount(posting.JournalEntry{}, line); !errors.Is(err, ErrInvalidOpenLine) {
		t.Fatalf("empty entry error = %v", err)
	}

	noPartner := line
	noPartner.PartnerID = nil
	withPartner := entry
	withPartner.Lines[0].PartnerID = nil
	if _, err := postedControlAmount(withPartner, noPartner); err != nil {
		t.Fatalf("nil line partner: %v", err)
	}
	otherPartner := uuid.New()
	filtered := entry
	filtered.Lines[0].PartnerID = &otherPartner
	if amount, err := postedControlAmount(filtered, line); err != nil || !amount.Amount().IsZero() {
		t.Fatalf("partner-filtered amount = %v err=%v", amount, err)
	}
}

func TestCovAddPostedInvoice(t *testing.T) {
	companyID, controlID, journalID, partnerID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	ctx := context.Background()
	entry := covControlEntry(companyID, journalID, controlID, partnerID, uuid.New(), LineReceivable, posting.EntryPosted)
	repo := &covPostedRepo{
		MemoryRepository: NewMemoryRepository(),
		account:          covAccount(companyID, controlID),
		entry:            entry,
	}
	line, err := addPostedInvoice(ctx, repo, companyID, partnerID, controlID, journalID,
		covMoney(t, "100", "USD"), covDate(1), covDate(3), "INV-1", LineReceivable)
	if err != nil || line.OriginalAmount.Amount().IsZero() || line.Kind != LineReceivable {
		t.Fatalf("add posted invoice: %+v err=%v", line, err)
	}
	if line.PostingDate.Equal(entry.EntryDate) == false {
		t.Fatalf("posting date not taken from entry: %v", line.PostingDate)
	}

	repo.accountErr = errors.New("account failed")
	if _, err := addPostedInvoice(ctx, repo, companyID, partnerID, controlID, journalID,
		covMoney(t, "100", "USD"), covDate(1), covDate(3), "INV-1", LineReceivable); err == nil {
		t.Fatal("expected account error")
	}
	repo.accountErr = nil
	repo.entryErr = errors.New("entry failed")
	if _, err := addPostedInvoice(ctx, repo, companyID, partnerID, controlID, journalID,
		covMoney(t, "100", "USD"), covDate(1), covDate(3), "INV-1", LineReceivable); err == nil {
		t.Fatal("expected entry error")
	}
	repo.entryErr = nil
	repo.saveErr = errors.New("save failed")
	if _, err := addPostedInvoice(ctx, repo, companyID, partnerID, controlID, journalID,
		covMoney(t, "100", "USD"), covDate(1), covDate(3), "INV-1", LineReceivable); err == nil {
		t.Fatal("expected save error")
	}
	repo.saveErr = nil

	invalidInputs := []struct {
		companyID, partnerID, controlID, journalID uuid.UUID
		amount                                     financialtypes.Money
	}{
		{uuid.Nil, partnerID, controlID, journalID, covMoney(t, "1", "USD")},
		{companyID, uuid.Nil, controlID, journalID, covMoney(t, "1", "USD")},
		{companyID, partnerID, uuid.Nil, journalID, covMoney(t, "1", "USD")},
		{companyID, partnerID, controlID, uuid.Nil, covMoney(t, "1", "USD")},
		{companyID, partnerID, controlID, journalID, financialtypes.Money{}},
		{companyID, partnerID, controlID, journalID, covMoney(t, "-1", "USD")},
		{companyID, partnerID, controlID, journalID, covMoney(t, "0", "USD")},
	}
	for _, input := range invalidInputs {
		if _, err := addPostedInvoice(ctx, repo, input.companyID, input.partnerID, input.controlID, input.journalID,
			input.amount, covDate(1), covDate(3), "INV", LineReceivable); !errors.Is(err, ErrInvalidOpenLine) {
			t.Fatalf("invalid input error = %v", err)
		}
	}
	if _, err := addPostedInvoice(nil, repo, companyID, partnerID, controlID, journalID,
		covMoney(t, "1", "USD"), covDate(1), covDate(3), "INV", LineReceivable); !errors.Is(err, ErrInvalidOpenLine) {
		t.Fatalf("nil context error = %v", err)
	}

	wrongEntry := repo.entry
	wrongEntry.Lines[0].Debit = covMoney(t, "50", "USD")
	wrongEntry.Lines[0].FunctionalDebit = covMoney(t, "50", "USD")
	repo.entry = wrongEntry
	if _, err := addPostedInvoice(ctx, repo, companyID, partnerID, controlID, journalID,
		covMoney(t, "100", "USD"), covDate(1), covDate(3), "INV", LineReceivable); err == nil {
		t.Fatal("expected posted amount mismatch")
	}
}

type covAttachmentState struct {
	lines []OpenLine
}

func (s *covAttachmentState) SaveOpenLineFromAttachment(
	_ context.Context, attachment posting.OpenLineAttachment, _ posting.JournalEntry,
) error {
	s.lines = append(s.lines, OpenLine{
		ID: attachment.ID, CompanyID: attachment.CompanyID, PartnerID: covUUIDPtr(attachment.PartnerID),
		ControlAccountID: attachment.ControlAccountID, JournalEntryID: attachment.JournalEntryID,
		Kind: LineKind(attachment.Kind), OriginalAmount: attachment.Amount,
		ResidualAmount: attachment.Amount,
	})
	return nil
}

func TestCovPostInvoiceEntryContext(t *testing.T) {
	companyID, journalID, partnerID, controlID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	amount := covMoney(t, "100", "USD")
	zero := covMoney(t, "0", "USD")
	entry := posting.JournalEntry{
		ID: uuid.New(), CompanyID: companyID, JournalID: journalID,
		Status: posting.EntryDraft, EntryDate: covDate(2), Date: covDate(2),
		Lines: []posting.JournalEntryLine{
			{ID: uuid.New(), CompanyID: companyID, AccountID: controlID, PartnerID: &partnerID,
				Debit: amount, Credit: zero, FunctionalDebit: amount, FunctionalCredit: zero},
			{ID: uuid.New(), CompanyID: companyID, AccountID: uuid.New(),
				Debit: zero, Credit: amount, FunctionalDebit: zero, FunctionalCredit: amount},
		},
	}
	ctx := context.Background()
	if _, err := postInvoiceEntryContext(nil, posting.NewEngine(), &entry, companyID, partnerID, controlID,
		LineReceivable, amount, covDate(1), covDate(3), "INV", "poster"); !errors.Is(err, ErrInvalidOpenLine) {
		t.Fatalf("nil context error = %v", err)
	}
	for name, args := range map[string]struct {
		engine *posting.Engine
		entry  *posting.JournalEntry
	}{
		"nil engine": {nil, &entry},
		"nil entry":  {posting.NewEngine(), nil},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := postInvoiceEntryContext(ctx, args.engine, args.entry, companyID, partnerID, controlID,
				LineReceivable, amount, covDate(1), covDate(3), "INV", "poster"); !errors.Is(err, ErrInvalidOpenLine) {
				t.Fatalf("error = %v", err)
			}
		})
	}
	if _, err := postInvoiceEntryContext(ctx, posting.NewEngine(), &entry, uuid.Nil, partnerID, controlID,
		LineReceivable, amount, covDate(1), covDate(3), "INV", "poster"); !errors.Is(err, ErrInvalidOpenLine) {
		t.Fatalf("nil company error = %v", err)
	}
	if _, err := postInvoiceEntryContext(ctx, posting.NewEngine(), &entry, companyID, uuid.Nil, controlID,
		LineReceivable, amount, covDate(1), covDate(3), "INV", "poster"); !errors.Is(err, ErrInvalidOpenLine) {
		t.Fatalf("nil partner error = %v", err)
	}

	state := &covAttachmentState{}
	engine := posting.NewEngineWithStore(nil, state)
	line, err := postInvoiceEntryContext(ctx, engine, &entry, companyID, partnerID, controlID,
		LineReceivable, amount, covDate(1), covDate(3), "INV", "poster")
	if err != nil {
		t.Fatalf("posting with attachment: %v", err)
	}
	if len(state.lines) != 1 || line.ID != state.lines[0].ID || line.PostingDate.Equal(covDate(2)) == false {
		t.Fatalf("attachment state mismatch: line=%+v state=%+v", line, state.lines)
	}

	wrongAmount := entry
	wrongAmount.Lines = append([]posting.JournalEntryLine(nil), entry.Lines...)
	wrongAmount.Lines[0].FunctionalDebit = covMoney(t, "90", "USD")
	if _, err := postInvoiceEntryContext(ctx, posting.NewEngine(), &wrongAmount, companyID, partnerID, controlID,
		LineReceivable, amount, covDate(1), covDate(3), "INV", "poster"); err == nil {
		t.Fatal("expected functional amount mismatch")
	}
}

func covWriteOffLine(t *testing.T, companyID, controlID uuid.UUID, kind LineKind) OpenLine {
	t.Helper()
	return OpenLine{
		CompanyID: companyID, ID: uuid.New(), PartnerID: covUUIDPtr(uuid.New()),
		ControlAccountID: controlID, JournalEntryID: uuid.New(), Kind: kind,
		OriginalAmount: covMoney(t, "100", "USD"), ResidualAmount: covMoney(t, "100", "USD"),
		FunctionalOriginalAmount: covMoney(t, "100", "USD"), FunctionalResidualAmount: covMoney(t, "100", "USD"),
		PostingDate: covDate(1), DocumentDate: covDate(1), DueDate: covDate(3),
	}
}

func covWriteOffParams() WriteOffParams {
	return WriteOffParams{
		JournalID: uuid.New(), AdjustmentAccountID: uuid.New(),
		Date: covDate(2), Description: "bad debt",
	}
}

func TestCovWriteOffAttachment(t *testing.T) {
	companyID, lineID := uuid.New(), uuid.New()
	attachment := &WriteOffAttachment{
		OpenLineID: lineID, CompanyID: companyID,
		WriteOffAmount: covMoney(t, "10", "USD"), AdjustmentAccountID: uuid.New(),
		UpdatedLine: OpenLine{ID: lineID, CompanyID: companyID},
	}
	if attachment.AttachmentKind() != "receivable_writeoff" {
		t.Fatalf("unexpected attachment kind: %s", attachment.AttachmentKind())
	}
	entry := posting.JournalEntry{ID: uuid.New(), CompanyID: companyID}
	if err := attachment.ValidateAttachment(context.Background(), entry); err != nil {
		t.Fatalf("valid attachment: %v", err)
	}
	badCompany := entry
	badCompany.CompanyID = uuid.New()
	if err := attachment.ValidateAttachment(context.Background(), badCompany); !errors.Is(err, ErrCompanyMismatch) {
		t.Fatalf("company mismatch error = %v", err)
	}
	invalid := *attachment
	invalid.OpenLineID = uuid.Nil
	if err := invalid.ValidateAttachment(context.Background(), entry); !errors.Is(err, ErrInvalidOpenLine) {
		t.Fatalf("nil line error = %v", err)
	}
	invalid = *attachment
	invalid.CompanyID = uuid.Nil
	if err := invalid.ValidateAttachment(context.Background(), entry); !errors.Is(err, ErrInvalidOpenLine) {
		t.Fatalf("nil company error = %v", err)
	}
}

type covAtomicTx struct {
	state       any
	execErr     error
	mutationErr error
	queryErr    error
	execCalls   int
	queryCalls  int
}

func (tx *covAtomicTx) Exec(context.Context, string, ...any) error {
	tx.execCalls++
	return tx.execErr
}

func (tx *covAtomicTx) QueryRow(context.Context, string, ...any) posting.Row {
	tx.queryCalls++
	return &covAtomicRow{err: tx.queryErr}
}

func (tx *covAtomicTx) CustomMutation(_ context.Context, target string, fn func(any) error) error {
	if target != "memory" {
		return posting.ErrUnsupportedMutationBackend
	}
	if tx.mutationErr != nil {
		return tx.mutationErr
	}
	return fn(tx.state)
}

type covAtomicRow struct {
	err error
}

func (row *covAtomicRow) Scan(...any) error { return row.err }

func TestCovWriteOffAtomicMutation(t *testing.T) {
	ctx := context.Background()
	companyID, lineID := uuid.New(), uuid.New()
	updated := OpenLine{CompanyID: companyID, ID: lineID, ResidualAmount: covMoney(t, "90", "USD")}
	attachment := &WriteOffAttachment{OpenLineID: lineID, CompanyID: companyID, UpdatedLine: updated}
	repo := NewMemoryRepository()
	if err := attachment.ExecuteAtomicMutation(ctx, &covAtomicTx{state: repo}, posting.JournalEntry{}); err != nil {
		t.Fatalf("memory mutation: %v", err)
	}
	stored, err := repo.GetOpenLine(ctx, companyID, lineID)
	if err != nil || !stored.ResidualAmount.Equal(covMoney(t, "90", "USD")) {
		t.Fatalf("stored line = %+v err=%v", stored, err)
	}

	sqlTx := &covAtomicTx{state: struct{}{}}
	if err := attachment.ExecuteAtomicMutation(ctx, sqlTx, posting.JournalEntry{}); err != nil || sqlTx.execCalls != 1 {
		t.Fatalf("SQL fallback: err=%v execCalls=%d", err, sqlTx.execCalls)
	}
	sqlTx.execErr = errors.New("sql failed")
	if err := attachment.ExecuteAtomicMutation(ctx, sqlTx, posting.JournalEntry{}); err == nil {
		t.Fatal("expected SQL failure")
	}
	mutErr := errors.New("mutation failed")
	if err := attachment.ExecuteAtomicMutation(ctx, &covAtomicTx{state: repo, mutationErr: mutErr}, posting.JournalEntry{}); !errors.Is(err, mutErr) {
		t.Fatalf("mutation error = %v", err)
	}

	revaluation := &RevaluationAttachment{AssetID: uuid.New(), CompanyID: companyID, AsOf: covDate(2)}
	_ = revaluation
}

func TestCovWriteOffWorkflows(t *testing.T) {
	companyID, controlID := uuid.New(), uuid.New()
	repo := NewMemoryRepository()
	line := covWriteOffLine(t, companyID, controlID, LineReceivable)
	if err := repo.SaveOpenLine(context.Background(), line); err != nil {
		t.Fatal(err)
	}
	params := covWriteOffParams()
	ledger := NewReceivableLedgerForCompany(repo, companyID)

	engine := posting.NewEngineWithStore(nil, repo)
	entry, err := ledger.WriteOffAtomic(context.Background(), engine, line.ID, covMoney(t, "40", "USD"), params, "poster")
	if err != nil || entry.Status != posting.EntryPosted {
		t.Fatalf("atomic write-off: %+v err=%v", entry, err)
	}
	updated, err := repo.GetOpenLine(context.Background(), companyID, line.ID)
	if err != nil || !updated.ResidualAmount.Equal(covMoney(t, "60", "USD")) ||
		!updated.WrittenOffAmount.Equal(covMoney(t, "40", "USD")) || updated.Closed {
		t.Fatalf("updated line: %+v err=%v", updated, err)
	}

	if _, err := ledger.WriteOffAtomic(context.Background(), posting.NewEngine(), line.ID,
		covMoney(t, "1", "USD"), params, "poster"); err == nil {
		t.Fatal("expected write-off amount exceeding residual to fail")
	}
	noStoreEngine := posting.NewEngine()
	prepareLine := covWriteOffLine(t, companyID, controlID, LineReceivable)
	if err := repo.SaveOpenLine(context.Background(), prepareLine); err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.WriteOffAtomic(context.Background(), noStoreEngine, prepareLine.ID,
		covMoney(t, "1", "USD"), params, "poster"); err == nil {
		t.Fatal("expected missing memory store to fail")
	}

	payableLine := covWriteOffLine(t, companyID, controlID, LinePayable)
	if err := repo.SaveOpenLine(context.Background(), payableLine); err != nil {
		t.Fatal(err)
	}
	payableLedger := NewPayableLedgerForCompany(repo, companyID)
	if _, err := payableLedger.WriteOffAtomic(context.Background(), engine, payableLine.ID,
		covMoney(t, "10", "USD"), params, "poster"); err != nil {
		t.Fatalf("payable atomic write-off: %v", err)
	}

	preparedLine := covWriteOffLine(t, companyID, controlID, LineReceivable)
	if err := repo.SaveOpenLine(context.Background(), preparedLine); err != nil {
		t.Fatal(err)
	}
	draft, err := ledger.PrepareWriteOff(preparedLine.ID, covMoney(t, "25", "USD"), params)
	if err != nil {
		t.Fatalf("prepare write-off: %v", err)
	}
	pending, err := repo.GetOpenLine(context.Background(), companyID, preparedLine.ID)
	if err != nil || pending.PendingWriteOffID != draft.ID {
		t.Fatalf("pending line: %+v err=%v", pending, err)
	}
	if _, err := ledger.PrepareWriteOff(preparedLine.ID, covMoney(t, "25", "USD"), params); !errors.Is(err, ErrWriteOffPending) {
		t.Fatalf("second prepare error = %v", err)
	}

	repo.mu.Lock()
	posted := draft
	posted.Status = posting.EntryPosted
	repo.drafts[posted.ID] = posted
	repo.mu.Unlock()
	if err := ledger.ConfirmWriteOff(preparedLine.ID, posted.ID); err != nil {
		t.Fatalf("confirm write-off: %v", err)
	}
	confirmed, err := repo.GetOpenLine(context.Background(), companyID, preparedLine.ID)
	if err != nil || confirmed.PendingWriteOffID != uuid.Nil || !confirmed.ResidualAmount.Equal(covMoney(t, "75", "USD")) {
		t.Fatalf("confirmed line: %+v err=%v", confirmed, err)
	}
	if err := ledger.ConfirmWriteOff(preparedLine.ID, posted.ID); !errors.Is(err, ErrNoPendingWriteOff) {
		t.Fatalf("second confirmation error = %v", err)
	}

	otherLine := covWriteOffLine(t, companyID, controlID, LinePayable)
	if err := repo.SaveOpenLine(context.Background(), otherLine); err != nil {
		t.Fatal(err)
	}
	if _, err := payableLedger.PrepareWriteOff(otherLine.ID, covMoney(t, "5", "USD"), params); err != nil {
		t.Fatalf("payable prepare: %v", err)
	}
	if _, err := PrepareWriteOffForCompany(repo, companyID, uuid.New(), covMoney(t, "1", "USD"), params); !errors.Is(err, ErrNotFound) {
		t.Fatalf("prepare unknown line error = %v", err)
	}
	if err := ConfirmWriteOffForCompany(repo, companyID, otherLine.ID, uuid.New()); err == nil {
		t.Fatal("expected confirmation with unknown entry to fail")
	}
}

func TestCovWriteOffValidationErrors(t *testing.T) {
	companyID, controlID := uuid.New(), uuid.New()
	repo := NewMemoryRepository()
	ledger := NewReceivableLedgerForCompany(repo, companyID)
	params := covWriteOffParams()

	if _, err := ledger.WriteOffAtomic(context.Background(), posting.NewEngine(), uuid.New(),
		covMoney(t, "1", "USD"), params, "poster"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown line error = %v", err)
	}

	line := covWriteOffLine(t, companyID, controlID, LineReceivable)
	if err := repo.SaveOpenLine(context.Background(), line); err != nil {
		t.Fatal(err)
	}
	wrongCurrency := covWriteOffLine(t, companyID, controlID, LineReceivable)
	wrongCurrency.ID = line.ID
	wrongCurrency.ResidualAmount = covMoney(t, "100", "EUR")
	if err := repo.SaveOpenLine(ctxOrBackground(), wrongCurrency); err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.WriteOffAtomic(context.Background(), posting.NewEngine(), line.ID,
		covMoney(t, "1", "USD"), params, "poster"); !errors.Is(err, ErrInvalidOpenLine) {
		t.Fatalf("currency mismatch error = %v", err)
	}

	closed := line
	closed.ResidualAmount = covMoney(t, "100", "USD")
	closed.Closed = true
	if err := repo.SaveOpenLine(ctxOrBackground(), closed); err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.WriteOffAtomic(context.Background(), posting.NewEngine(), line.ID,
		covMoney(t, "1", "USD"), params, "poster"); !errors.Is(err, ErrInvalidOpenLine) {
		t.Fatalf("closed line error = %v", err)
	}

	pending := line
	pending.Closed = false
	pending.PendingWriteOffID = uuid.New()
	if err := repo.SaveOpenLine(ctxOrBackground(), pending); err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.WriteOffAtomic(context.Background(), posting.NewEngine(), line.ID,
		covMoney(t, "1", "USD"), params, "poster"); !errors.Is(err, ErrWriteOffPending) {
		t.Fatalf("pending write-off error = %v", err)
	}
	if _, err := ledger.PrepareWriteOff(line.ID, covMoney(t, "1", "USD"), params); !errors.Is(err, ErrWriteOffPending) {
		t.Fatalf("prepare pending error = %v", err)
	}

	valid := line
	valid.PendingWriteOffID = uuid.Nil
	if err := repo.SaveOpenLine(ctxOrBackground(), valid); err != nil {
		t.Fatal(err)
	}
	badParams := params
	badParams.JournalID = uuid.Nil
	if _, err := ledger.PrepareWriteOff(line.ID, covMoney(t, "1", "USD"), badParams); !errors.Is(err, ErrInvalidOpenLine) {
		t.Fatalf("nil journal error = %v", err)
	}
	badParams = params
	badParams.AdjustmentAccountID = uuid.Nil
	if _, err := ledger.PrepareWriteOff(line.ID, covMoney(t, "1", "USD"), badParams); !errors.Is(err, ErrInvalidOpenLine) {
		t.Fatalf("nil adjustment account error = %v", err)
	}
	if _, err := ledger.PrepareWriteOff(line.ID, covMoney(t, "0", "USD"), params); !errors.Is(err, ErrInvalidOpenLine) {
		t.Fatalf("zero amount error = %v", err)
	}
	if _, err := ledger.PrepareWriteOff(line.ID, covMoney(t, "101", "USD"), params); !errors.Is(err, ErrInvalidOpenLine) {
		t.Fatalf("excess amount error = %v", err)
	}
	payableLedger := NewPayableLedgerForCompany(repo, companyID)
	if _, err := payableLedger.PrepareWriteOff(line.ID, covMoney(t, "1", "USD"), params); !errors.Is(err, ErrInvalidOpenLine) {
		t.Fatalf("wrong kind error = %v", err)
	}

	if err := ledger.ConfirmWriteOff(uuid.New(), uuid.New()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("confirm unknown line error = %v", err)
	}
	noPending := line
	noPending.PendingWriteOffID = uuid.Nil
	if err := repo.SaveOpenLine(ctxOrBackground(), noPending); err != nil {
		t.Fatal(err)
	}
	if err := ledger.ConfirmWriteOff(line.ID, uuid.New()); !errors.Is(err, ErrNoPendingWriteOff) {
		t.Fatalf("no pending error = %v", err)
	}
}

func ctxOrBackground() context.Context { return context.Background() }

func TestCovReceivablePayableLedgerWrappers(t *testing.T) {
	companyID, otherCompany := uuid.New(), uuid.New()
	partnerID, controlID, journalID := uuid.New(), uuid.New(), uuid.New()
	repo := NewMemoryRepository()
	amount := covMoney(t, "100", "USD")

	receivable := NewReceivableLedger(repo)
	payable := NewPayableLedger(repo)
	scopedReceivable := NewReceivableLedgerForCompany(repo, companyID)
	scopedPayable := NewPayableLedgerForCompany(repo, companyID)

	if _, err := receivable.AddInvoice(partnerID, amount, covDate(1), covDate(3), "AR-1"); err != nil {
		t.Fatalf("AR add: %v", err)
	}
	if _, err := payable.AddInvoice(partnerID, amount, covDate(1), covDate(3), "AP-1"); err != nil {
		t.Fatalf("AP add: %v", err)
	}
	scopedLine, err := scopedReceivable.AddInvoice(partnerID, amount, covDate(1), covDate(3), "AR-2")
	if err != nil {
		t.Fatalf("scoped AR add: %v", err)
	}
	if _, err := scopedPayable.AddInvoice(partnerID, amount, covDate(1), covDate(3), "AP-2"); err != nil {
		t.Fatalf("scoped AP add: %v", err)
	}
	if _, err := receivable.AddInvoiceContext(context.Background(), uuid.Nil, amount, covDate(1), covDate(3), "AR"); !errors.Is(err, ErrInvalidAmount) {
		t.Fatalf("nil partner error = %v", err)
	}
	if _, err := payable.AddInvoiceForCompanyContext(context.Background(), companyID, partnerID,
		covMoney(t, "-1", "USD"), covDate(1), covDate(3), "AP"); !errors.Is(err, ErrInvalidAmount) {
		t.Fatalf("negative amount error = %v", err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := receivable.AddInvoiceContext(cancelled, partnerID, amount, covDate(1), covDate(3), "AR"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled context error = %v", err)
	}

	for _, ledger := range []*ReceivableLedger{receivable, scopedReceivable} {
		if _, err := ledger.AddInvoiceWithControl(partnerID, uuid.Nil, journalID, amount, covDate(1), covDate(3), "AR"); !errors.Is(err, ErrInvalidOpenLine) {
			t.Fatalf("nil control account error = %v", err)
		}
		if _, err := ledger.AddInvoiceForCompanyContext(context.Background(), uuid.Nil, partnerID, amount, covDate(1), covDate(3), "AR"); !errors.Is(err, ErrCompanyScopeRequired) {
			t.Fatalf("nil company error = %v", err)
		}
		_, err := ledger.AddInvoiceFromPostedEntryContext(context.Background(), partnerID, controlID, journalID, amount, covDate(1), covDate(3), "AR")
		if ledger == scopedReceivable {
			if !errors.Is(err, masterdata.ErrAccountNotFound) {
				t.Fatalf("unexpected posted-entry error = %v", err)
			}
		} else if !errors.Is(err, ErrInvalidOpenLine) {
			t.Fatalf("unexpected posted-entry error = %v", err)
		}
	}
	if _, err := scopedReceivable.AddInvoiceForCompanyContext(context.Background(), otherCompany, partnerID,
		amount, covDate(1), covDate(3), "AR"); !errors.Is(err, ErrCompanyMismatch) {
		t.Fatalf("company mismatch error = %v", err)
	}
	if _, err := scopedPayable.AddInvoiceForCompanyWithControlContext(context.Background(), companyID, partnerID,
		uuid.Nil, journalID, amount, covDate(1), covDate(3), "AP"); !errors.Is(err, ErrInvalidOpenLine) {
		t.Fatalf("nil control account error = %v", err)
	}
	if _, err := scopedPayable.AddInvoiceForCompanyWithControlContext(context.Background(), otherCompany, partnerID,
		controlID, journalID, amount, covDate(1), covDate(3), "AP"); !errors.Is(err, ErrCompanyMismatch) {
		t.Fatalf("company mismatch error = %v", err)
	}
	if _, err := scopedPayable.AddInvoiceFromPostedEntryContext(context.Background(), partnerID, controlID, journalID,
		amount, covDate(1), covDate(3), "AP"); !errors.Is(err, ErrInvalidOpenLine) && scopedPayable.companyID == uuid.Nil {
		t.Fatalf("unexpected error = %v", err)
	}
	if _, err := receivable.AddInvoiceFromPostedEntryForCompanyContext(context.Background(), uuid.Nil, partnerID,
		controlID, journalID, amount, covDate(1), covDate(3), "AR"); !errors.Is(err, ErrCompanyScopeRequired) {
		t.Fatalf("nil company error = %v", err)
	}
	if _, err := payable.AddInvoiceFromPostedEntryForCompanyContext(context.Background(), uuid.Nil, partnerID,
		controlID, journalID, amount, covDate(1), covDate(3), "AP"); !errors.Is(err, ErrCompanyScopeRequired) {
		t.Fatalf("nil company error = %v", err)
	}

	if _, err := receivable.PostInvoiceEntry(nil, &posting.JournalEntry{}, partnerID, controlID, amount,
		covDate(1), covDate(3), "AR", "poster"); !errors.Is(err, ErrInvalidOpenLine) {
		t.Fatalf("nil engine error = %v", err)
	}
	if _, err := payable.PostInvoiceEntryContext(context.Background(), nil, &posting.JournalEntry{}, partnerID, controlID,
		amount, covDate(1), covDate(3), "AP", "poster"); !errors.Is(err, ErrInvalidOpenLine) {
		t.Fatalf("nil engine error = %v", err)
	}

	asOf := covDate(5)
	if schedule, err := payable.Aging(asOf, nil); err != nil || len(schedule) != 5 {
		t.Fatalf("payable aging: %+v err=%v", schedule, err)
	}
	if schedule, err := payable.AgingContext(context.Background(), asOf, &partnerID); err != nil || len(schedule) != 5 {
		t.Fatalf("scoped payable aging: %+v err=%v", schedule, err)
	}
	if schedule, err := scopedReceivable.Aging(asOf, &partnerID); err != nil || len(schedule) != 5 {
		t.Fatalf("scoped receivable aging: %+v err=%v", schedule, err)
	}
	if err := scopedReceivable.CloseOpenLine(uuid.New()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("close unknown line error = %v", err)
	}
	if err := scopedReceivable.CloseOpenLineContext(context.Background(), scopedLine.ID); err != nil {
		t.Fatalf("close line: %v", err)
	}
	closed, err := repo.GetOpenLine(context.Background(), companyID, scopedLine.ID)
	if err != nil || !closed.Closed {
		t.Fatalf("closed line: %+v err=%v", closed, err)
	}

	probabilities := ReceivableMatrix{AgingCurrent: decimal.NewFromFloat(0.1), Aging1To30: decimal.NewFromFloat(0.2)}
	if err := probabilities.Validate(); err != nil {
		t.Fatalf("valid matrix: %v", err)
	}
	if err := (ReceivableMatrix{AgingBucket(""): decimal.NewFromInt(1)}).Validate(); !errors.Is(err, ErrInvalidAmount) {
		t.Fatalf("empty bucket error = %v", err)
	}
	if err := (ReceivableMatrix{AgingCurrent: decimal.NewFromInt(-1)}).Validate(); !errors.Is(err, ErrInvalidAmount) {
		t.Fatalf("negative probability error = %v", err)
	}
	if err := (ReceivableMatrix{AgingCurrent: decimal.NewFromInt(2)}).Validate(); !errors.Is(err, ErrInvalidAmount) {
		t.Fatalf("excess probability error = %v", err)
	}

	ecl, err := scopedReceivable.CalculateECL(asOf, &partnerID, probabilities)
	if err != nil || ecl.Currency() != "USD" {
		t.Fatalf("ECL: %v err=%v", ecl, err)
	}
	noLines := NewReceivableLedger(NewMemoryRepository())
	if ecl, err := noLines.CalculateECL(asOf, nil, probabilities); err != nil || !ecl.Amount().IsZero() {
		t.Fatalf("empty ECL: %v err=%v", ecl, err)
	}
}

type covARAPApp struct {
	entries          map[types.JournalEntryID]ledger.JournalEntry
	createErr        error
	createWithIDErr  error
	postErr          error
	postReplay       bool
	posted           int
	getEntry         ledger.JournalEntry
	getEntryErr      error
	withTxErr        error
	txCalls          int
	draftIDs         []types.JournalEntryID
	lastDraftCommand publicapplication.CreateDraftCommand
}

func (a *covARAPApp) CreateDraft(_ context.Context, command publicapplication.CreateDraftCommand) (publicapplication.EntryResult, error) {
	if a.createErr != nil {
		return publicapplication.EntryResult{}, a.createErr
	}
	return a.saveDraft(types.NewJournalEntryID(), command), nil
}

func (a *covARAPApp) CreateDraftWithID(_ context.Context, command publicapplication.CreateDraftCommand, id types.JournalEntryID) (publicapplication.EntryResult, error) {
	a.draftIDs = append(a.draftIDs, id)
	if a.createWithIDErr != nil {
		return publicapplication.EntryResult{}, a.createWithIDErr
	}
	return a.saveDraft(id, command), nil
}

func (a *covARAPApp) saveDraft(id types.JournalEntryID, command publicapplication.CreateDraftCommand) publicapplication.EntryResult {
	a.lastDraftCommand = command
	if a.entries == nil {
		a.entries = make(map[types.JournalEntryID]ledger.JournalEntry)
	}
	entry := ledger.JournalEntry{
		ID: id, CompanyID: command.CompanyID, JournalID: command.JournalID,
		Status: ledger.EntryDraft, EffectiveDate: command.EffectiveDate,
		PostingDate: command.PostingDate, Lines: command.Lines,
	}
	a.entries[id] = entry
	return publicapplication.EntryResult{Entry: entry}
}

func (a *covARAPApp) Validate(context.Context, publicapplication.ValidateEntryCommand) error {
	return nil
}

func (a *covARAPApp) Post(_ context.Context, command publicapplication.PostEntryCommand) (publicapplication.EntryResult, error) {
	if a.postErr != nil {
		return publicapplication.EntryResult{}, a.postErr
	}
	entry, ok := a.entries[command.EntryID]
	if !ok {
		return publicapplication.EntryResult{}, ErrNotFound
	}
	entry.Status = ledger.EntryPosted
	a.entries[command.EntryID] = entry
	a.posted++
	return publicapplication.EntryResult{Entry: entry, IdempotentReplay: a.postReplay}, nil
}

func (a *covARAPApp) Reverse(context.Context, publicapplication.ReverseEntryCommand) (publicapplication.EntryResult, error) {
	return publicapplication.EntryResult{}, nil
}

func (a *covARAPApp) GetEntry(context.Context, publicapplication.GetEntryQuery) (ledger.JournalEntry, error) {
	return a.getEntry, a.getEntryErr
}

func (a *covARAPApp) WithinTransaction(ctx context.Context, _ types.CompanyID, fn func(context.Context) error) error {
	a.txCalls++
	if a.withTxErr != nil {
		return a.withTxErr
	}
	return fn(ctx)
}

type covARAPReceivableStore struct {
	*p1001ReceivableStore
	getErr    error
	saveErr   error
	listErr   error
	getLine   *apisubledger.Receivable
	listLines []apisubledger.Receivable
}

func (s *covARAPReceivableStore) GetReceivable(ctx context.Context, companyID types.CompanyID, id string) (apisubledger.Receivable, error) {
	if s.getErr != nil {
		return apisubledger.Receivable{}, s.getErr
	}
	if s.getLine != nil {
		return *s.getLine, nil
	}
	return s.p1001ReceivableStore.GetReceivable(ctx, companyID, id)
}

func (s *covARAPReceivableStore) SaveReceivable(ctx context.Context, companyID types.CompanyID, line apisubledger.Receivable) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	return s.p1001ReceivableStore.SaveReceivable(ctx, companyID, line)
}

func (s *covARAPReceivableStore) ListReceivables(ctx context.Context, companyID types.CompanyID, partnerID string) ([]apisubledger.Receivable, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	if s.listLines != nil {
		return append([]apisubledger.Receivable(nil), s.listLines...), nil
	}
	return s.p1001ReceivableStore.ListReceivables(ctx, companyID, partnerID)
}

type covARAPPayableStore struct {
	*p1001PayableStore
	getErr    error
	saveErr   error
	listErr   error
	getLine   *apisubledger.Payable
	listLines []apisubledger.Payable
}

func (s *covARAPPayableStore) GetPayable(ctx context.Context, companyID types.CompanyID, id string) (apisubledger.Payable, error) {
	if s.getErr != nil {
		return apisubledger.Payable{}, s.getErr
	}
	if s.getLine != nil {
		return *s.getLine, nil
	}
	return s.p1001PayableStore.GetPayable(ctx, companyID, id)
}

func (s *covARAPPayableStore) SavePayable(ctx context.Context, companyID types.CompanyID, line apisubledger.Payable) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	return s.p1001PayableStore.SavePayable(ctx, companyID, line)
}

func (s *covARAPPayableStore) ListPayables(ctx context.Context, companyID types.CompanyID, partnerID string) ([]apisubledger.Payable, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	if s.listLines != nil {
		return append([]apisubledger.Payable(nil), s.listLines...), nil
	}
	return s.p1001PayableStore.ListPayables(ctx, companyID, partnerID)
}

type covARAPBalanceReader struct {
	amount decimal.Decimal
	err    error
}

func (r *covARAPBalanceReader) GetAccountBalance(context.Context, types.CompanyID, types.AccountID, time.Time, string) (decimal.Decimal, error) {
	return r.amount, r.err
}

func covARAPService(app publicapplication.Service, receivables *covARAPReceivableStore, payables *covARAPPayableStore, balance *covARAPBalanceReader) *PublicARAPService {
	service, err := NewPublicARAPService(app, publicsubledger.ARAPStores{
		Receivables: receivables, Payables: payables, Balance: balance,
	}, false)
	if err != nil {
		panic(err)
	}
	return service
}

func covARAPFixture(t *testing.T) (types.CompanyID, types.AccountID, types.AccountID, types.AccountID, types.JournalID, *covARAPApp, *covARAPReceivableStore, *covARAPPayableStore, *covARAPBalanceReader, *PublicARAPService) {
	t.Helper()
	companyID, control, counter, cash := types.NewCompanyID(), types.NewAccountID(), types.NewAccountID(), types.NewAccountID()
	journalID := types.NewJournalID()
	app := &covARAPApp{}
	receivables := &covARAPReceivableStore{p1001ReceivableStore: &p1001ReceivableStore{values: make(map[string]apisubledger.Receivable)}}
	payables := &covARAPPayableStore{p1001PayableStore: &p1001PayableStore{values: make(map[string]apisubledger.Payable)}}
	balance := &covARAPBalanceReader{amount: decimal.Zero}
	return companyID, control, counter, cash, journalID, app, receivables, payables, balance,
		covARAPService(app, receivables, payables, balance)
}

func covARAPInvoice(companyID types.CompanyID, control, counter types.AccountID, journalID types.JournalID, reference string) apisubledger.InvoiceCommand {
	return apisubledger.InvoiceCommand{
		CompanyID: companyID, PartnerID: "partner-1", ControlAccountID: control,
		CounterAccountID: counter, JournalID: journalID,
		Amount: p1001MoneyRaw("100", "USD"), FunctionalAmount: p1001MoneyRaw("100", "USD"),
		DocumentDate: covDate(2), DueDate: covDate(3), Reference: reference,
	}
}

func p1001MoneyRaw(value, currency string) types.Money {
	money, err := types.NewMoneyFromString(value, currency)
	if err != nil {
		panic(err)
	}
	return money
}

func TestCovARAPDeterministicHelpers(t *testing.T) {
	ctx := context.Background()
	scope := arapScopeKey("receivable", uuid.New().String(), "key", "fingerprint")
	if !strings.HasPrefix(scope, "accounting-engine:arap:receivable:") {
		t.Fatalf("unexpected scope: %s", scope)
	}
	if arapDeterministicID(scope) != arapDeterministicID(scope) || arapDeterministicID(scope) == arapDeterministicID(scope+":other") {
		t.Fatal("deterministic ids must be stable and scope-specific")
	}
	fingerprint := arapCommandFingerprint(struct{ Name string }{"value"})
	if len(fingerprint) != 64 || arapCommandFingerprint(struct{ Name string }{"other"}) == fingerprint {
		t.Fatalf("bad command fingerprint: %s", fingerprint)
	}
	if arapIdempotencyKey(ctx) != "" ||
		arapIdempotencyKey(publicapplication.WithRequestMetadata(ctx, publicapplication.RequestMetadata{IdempotencyKey: " key "})) != "key" {
		t.Fatal("idempotency key extraction failed")
	}
	app := &covARAPApp{}
	service := &PublicARAPService{app: app}
	if _, ok := service.deterministicIDs(scope, 2); !ok {
		t.Fatal("deterministic capability missing")
	}
	ids, ok := service.deterministicIDs(scope, 2)
	if !ok || ids.entryID == (types.JournalEntryID{}) || len(ids.lineIDs) != 2 || ids.line(0) == ids.line(1) || ids.line(5) != (types.JournalEntryLineID{}) {
		t.Fatalf("unexpected deterministic ids: %+v", ids)
	}
	if _, ok := (&PublicARAPService{app: &p1001UseCaseRecorder{}}).deterministicIDs(scope, 1); ok {
		t.Fatal("non-deterministic app must not advertise deterministic ids")
	}
	principal := publicapplication.Principal{ActorID: "actor", CompanyIDs: []types.CompanyID{types.NewCompanyID()}}
	principalCtx := publicapplication.WithPrincipal(ctx, principal)
	metadataCtx := publicapplication.WithRequestMetadata(principalCtx, publicapplication.RequestMetadata{
		CorrelationID: "corr", IdempotencyKey: "key", SourceSystem: "sdk",
	})
	commandContext := arapCommandContext(metadataCtx, types.NewCompanyID(), false)
	if commandContext.Principal.ActorID != "actor" || commandContext.CorrelationID != "corr" ||
		commandContext.IdempotencyKey != "key" || commandContext.SourceSystem != "sdk" {
		t.Fatalf("unexpected command context: %+v", commandContext)
	}
	fallback := arapCommandContext(ctx, types.NewCompanyID(), false)
	if fallback.Principal.ActorID != "sdk" || len(fallback.Principal.CompanyIDs) != 1 {
		t.Fatalf("fallback principal: %+v", fallback)
	}
	strict := arapCommandContext(ctx, types.NewCompanyID(), true)
	if strict.Principal.ActorID != "" {
		t.Fatalf("strict principal must be preserved: %+v", strict)
	}
	if err := service.withinTransaction(ctx, types.NewCompanyID(), func(context.Context) error { return nil }); err != nil || app.txCalls != 1 {
		t.Fatalf("transaction call: err=%v calls=%d", err, app.txCalls)
	}
	app.withTxErr = errors.New("tx failed")
	if err := service.withinTransaction(ctx, types.NewCompanyID(), func(context.Context) error { return nil }); !errors.Is(err, app.withTxErr) {
		t.Fatalf("transaction error = %v", err)
	}
}

func TestCovARAPInvoiceValidationAndFailures(t *testing.T) {
	companyID, control, counter, _, journalID, app, receivables, payables, balance, service := covARAPFixture(t)
	ctx := context.Background()
	base := covARAPInvoice(companyID, control, counter, journalID, "AR-1")

	if line, err := invoiceFunctionalAmount(base); err != nil || !line.Equal(base.Amount) {
		t.Fatalf("default functional amount = %v err=%v", line, err)
	}
	negativeFunctional := base
	negativeFunctional.FunctionalAmount = p1001MoneyRaw("-1", "USD")
	if _, err := invoiceFunctionalAmount(negativeFunctional); !errors.Is(err, ErrInvalidARAPOperation) {
		t.Fatalf("negative functional error = %v", err)
	}
	zeroFunctional := base
	zeroFunctional.FunctionalAmount = p1001MoneyRaw("0", "USD")
	if _, err := invoiceFunctionalAmount(zeroFunctional); !errors.Is(err, ErrInvalidARAPOperation) {
		t.Fatalf("zero functional error = %v", err)
	}
	validations := []struct {
		name   string
		mutate func(*apisubledger.InvoiceCommand)
	}{
		{"nil company", func(c *apisubledger.InvoiceCommand) { c.CompanyID = types.CompanyID{} }},
		{"blank partner", func(c *apisubledger.InvoiceCommand) { c.PartnerID = " " }},
		{"nil control", func(c *apisubledger.InvoiceCommand) { c.ControlAccountID = types.AccountID{} }},
		{"nil counter", func(c *apisubledger.InvoiceCommand) { c.CounterAccountID = types.AccountID{} }},
		{"nil journal", func(c *apisubledger.InvoiceCommand) { c.JournalID = types.JournalID{} }},
		{"negative amount", func(c *apisubledger.InvoiceCommand) { c.Amount = p1001MoneyRaw("-1", "USD") }},
		{"zero amount", func(c *apisubledger.InvoiceCommand) { c.Amount = p1001MoneyRaw("0", "USD") }},
		{"blank reference", func(c *apisubledger.InvoiceCommand) { c.Reference = " " }},
	}
	for _, tc := range validations {
		t.Run(tc.name, func(t *testing.T) {
			candidate := base
			tc.mutate(&candidate)
			if _, err := service.CreateReceivableInvoice(ctx, candidate); !errors.Is(err, ErrInvalidARAPOperation) {
				t.Fatalf("error = %v", err)
			}
		})
	}
	defaulted := base
	defaulted.DocumentDate, defaulted.DueDate = time.Time{}, time.Time{}
	if _, err := service.CreatePayableInvoice(ctx, defaulted); err != nil || app.txCalls != 1 {
		t.Fatalf("defaulted dates invoice: err=%v tx=%d", err, app.txCalls)
	}
	app.withTxErr = errors.New("tx failed")
	if _, err := service.CreateReceivableInvoice(ctx, base); !errors.Is(err, app.withTxErr) {
		t.Fatalf("transaction error = %v", err)
	}
	app.withTxErr = nil
	app.createErr = errors.New("draft failed")
	if _, err := service.CreateReceivableInvoice(ctx, base); !errors.Is(err, app.createErr) {
		t.Fatalf("draft error = %v", err)
	}
	app.createErr = nil
	app.postErr = errors.New("post failed")
	if _, err := service.CreateReceivableInvoice(ctx, base); !errors.Is(err, app.postErr) {
		t.Fatalf("post error = %v", err)
	}
	app.postErr = nil
	receivables.saveErr = errors.New("save failed")
	if _, err := service.CreateReceivableInvoice(ctx, base); !errors.Is(err, ErrPostedARAPSaveFailed) {
		t.Fatalf("save error = %v", err)
	}
	receivables.saveErr = nil
	payables.saveErr = errors.New("save failed")
	if _, err := service.CreatePayableInvoice(ctx, base); !errors.Is(err, ErrPostedARAPSaveFailed) {
		t.Fatalf("payable save error = %v", err)
	}
	payables.saveErr = nil

	if _, err := service.createInvoice(ctx, base, apisubledger.SubledgerType("other")); !errors.Is(err, ErrInvalidARAPOperation) {
		t.Fatalf("unsupported kind error = %v", err)
	}
	if _, _, err := service.postBalanced(ctx, companyID, journalID, time.Time{}, "", "", apisubledger.SubledgerReceivable,
		control, counter, p1001MoneyRaw("1", "USD"), p1001MoneyRaw("1", "EUR"), arapPostingIDs{}); err != nil {
		t.Fatalf("different functional currency: %v", err)
	}
	if _, _, err := service.postBalanced(ctx, companyID, journalID, time.Time{}, "", "", apisubledger.SubledgerReceivable,
		control, counter, p1001MoneyRaw("1", "USD"), p1001MoneyRaw("0", "USD"), arapPostingIDs{}); !errors.Is(err, ErrInvalidARAPOperation) {
		t.Fatalf("non-positive rate error = %v", err)
	}
	zeroMoney, err := types.Zero("USD")
	if err != nil {
		t.Fatal(err)
	}
	line := newPublicLine(types.JournalEntryLineID{}, control, p1001MoneyRaw("10", "USD"), p1001MoneyRaw("10", "USD"), decimal.NewFromInt(1), true, zeroMoney)
	if line.ID == (types.JournalEntryLineID{}) || !line.FunctionalDebit.Equal(p1001MoneyRaw("10", "USD")) || !line.FunctionalCredit.Equal(zeroMoney) {
		t.Fatalf("debit line = %+v", line)
	}
	creditLine := newPublicLine(types.JournalEntryLineID{}, control, p1001MoneyRaw("10", "USD"), p1001MoneyRaw("10", "USD"), decimal.NewFromInt(1), false, zeroMoney)
	if !creditLine.FunctionalCredit.Equal(p1001MoneyRaw("10", "USD")) {
		t.Fatalf("credit line = %+v", creditLine)
	}
	balance.amount = decimal.NewFromInt(0)
}

func TestCovARAPDeterministicReplay(t *testing.T) {
	companyID, control, counter, _, journalID, app, receivables, payables, balance, service := covARAPFixture(t)
	ctx := publicapplication.WithRequestMetadata(context.Background(), publicapplication.RequestMetadata{IdempotencyKey: "idem-1"})
	command := covARAPInvoice(companyID, control, counter, journalID, "AR-IDEM")
	command.DocumentDate = covDate(2)

	result, err := service.CreateReceivableInvoice(ctx, command)
	if err != nil || result.Receivable == nil || len(app.draftIDs) != 1 || app.txCalls != 1 {
		t.Fatalf("first deterministic invoice: %+v err=%v", result, err)
	}
	entryID := result.EntryID
	if _, err := service.CreateReceivableInvoice(ctx, command); err != nil || len(app.draftIDs) != 2 {
		t.Fatalf("repeated deterministic invoice: %v", err)
	}
	stored, err := receivables.p1001ReceivableStore.ListReceivables(ctx, companyID, "")
	if err != nil || len(stored) != 1 {
		t.Fatalf("idempotent retry duplicated open item: %d err=%v", len(stored), err)
	}

	app.createWithIDErr = errors.New("draft conflict")
	app.getEntry = ledger.JournalEntry{ID: entryID, CompanyID: companyID, Status: ledger.EntryPosted}
	other := command
	other.Reference = "AR-IDEM-2"
	if replayed, err := service.CreateReceivableInvoice(ctx, other); err != nil || replayed.EntryID != entryID {
		t.Fatalf("posted replay: %+v err=%v", replayed, err)
	}
	app.getEntry.Status = ledger.EntryDraft
	if _, err := service.CreateReceivableInvoice(ctx, other); err != nil {
		t.Fatalf("draft replay: %v", err)
	}
	app.getEntry.Status = ledger.EntryCancelled
	if _, err := service.CreateReceivableInvoice(ctx, other); !errors.Is(err, app.createWithIDErr) {
		t.Fatalf("non-replayable error = %v", err)
	}
	app.getEntryErr = errors.New("lookup failed")
	if _, err := service.CreateReceivableInvoice(ctx, other); !errors.Is(err, app.createWithIDErr) {
		t.Fatalf("lookup error fallback = %v", err)
	}
	app.getEntryErr = nil
	app.createWithIDErr = nil
	app.postReplay = true
	if _, err := service.CreatePayableInvoice(ctx, other); err != nil || len(payables.p1001PayableStore.values) != 1 {
		t.Fatalf("payable replay: err=%v values=%d", err, len(payables.p1001PayableStore.values))
	}
	app.postReplay = false
	balance.amount = decimal.Zero
}

func TestCovARAPInquiryAndAgingBranches(t *testing.T) {
	companyID, control, _, _, _, app, receivables, payables, _, service := covARAPFixture(t)
	ctx := context.Background()
	otherCompany := types.NewCompanyID()
	receivableID, payableID := "AR-1", "AP-1"
	receivables.values[receivableID] = apisubledger.Receivable{
		ID: receivableID, CompanyID: companyID, PartnerID: "partner-1", ControlAccountID: control,
		RemainingAmount: p1001MoneyRaw("10", "USD"), Status: "open", DueDate: covDate(2),
	}
	payables.values[payableID] = apisubledger.Payable{
		ID: payableID, CompanyID: companyID, PartnerID: "partner-1", ControlAccountID: control,
		RemainingAmount: p1001MoneyRaw("20", "USD"), Status: "open", DueDate: covDate(2),
	}

	if _, err := service.GetReceivable(ctx, types.CompanyID{}, receivableID); !errors.Is(err, ErrInvalidOpenLine) {
		t.Fatalf("nil company receivable error = %v", err)
	}
	if _, err := service.GetReceivable(ctx, companyID, " "); !errors.Is(err, ErrInvalidOpenLine) {
		t.Fatalf("blank receivable id error = %v", err)
	}
	receivables.getErr = errors.New("get failed")
	if _, err := service.GetReceivable(ctx, companyID, receivableID); !errors.Is(err, receivables.getErr) {
		t.Fatalf("receivable get error = %v", err)
	}
	receivables.getErr = nil
	mismatchReceivable := receivables.values[receivableID]
	mismatchReceivable.CompanyID = otherCompany
	receivables.getLine = &mismatchReceivable
	if _, err := service.GetReceivable(ctx, companyID, receivableID); !errors.Is(err, ErrCompanyMismatch) {
		t.Fatalf("receivable mismatch error = %v", err)
	}
	receivables.getLine = nil
	if line, err := service.GetReceivable(ctx, companyID, receivableID); err != nil || line.ID != receivableID {
		t.Fatalf("receivable get: %+v err=%v", line, err)
	}

	if _, err := service.GetPayable(ctx, types.CompanyID{}, payableID); !errors.Is(err, ErrInvalidOpenLine) {
		t.Fatalf("nil company payable error = %v", err)
	}
	if _, err := service.GetPayable(ctx, companyID, " "); !errors.Is(err, ErrInvalidOpenLine) {
		t.Fatalf("blank payable id error = %v", err)
	}
	payables.getErr = errors.New("get failed")
	if _, err := service.GetPayable(ctx, companyID, payableID); !errors.Is(err, payables.getErr) {
		t.Fatalf("payable get error = %v", err)
	}
	payables.getErr = nil
	mismatchPayable := payables.values[payableID]
	mismatchPayable.CompanyID = otherCompany
	payables.getLine = &mismatchPayable
	if _, err := service.GetPayable(ctx, companyID, payableID); !errors.Is(err, ErrCompanyMismatch) {
		t.Fatalf("payable mismatch error = %v", err)
	}
	payables.getLine = nil
	if line, err := service.GetPayable(ctx, companyID, payableID); err != nil || line.ID != payableID {
		t.Fatalf("payable get: %+v err=%v", line, err)
	}

	if _, err := service.ListReceivables(ctx, types.CompanyID{}, ""); !errors.Is(err, ErrCompanyScopeRequired) {
		t.Fatalf("nil company list receivable error = %v", err)
	}
	receivables.listErr = errors.New("list failed")
	if _, err := service.ListReceivables(ctx, companyID, ""); !errors.Is(err, receivables.listErr) {
		t.Fatalf("receivable list error = %v", err)
	}
	receivables.listErr = nil
	receivables.listLines = []apisubledger.Receivable{mismatchReceivable}
	if _, err := service.ListReceivables(ctx, companyID, ""); !errors.Is(err, ErrCompanyMismatch) {
		t.Fatalf("receivable list mismatch error = %v", err)
	}
	receivables.listLines = nil
	if lines, err := service.ListReceivables(ctx, companyID, "partner-1"); err != nil || len(lines) != 1 {
		t.Fatalf("receivable list = %d err=%v", len(lines), err)
	}

	if _, err := service.ListPayables(ctx, types.CompanyID{}, ""); !errors.Is(err, ErrCompanyScopeRequired) {
		t.Fatalf("nil company list payable error = %v", err)
	}
	payables.listErr = errors.New("list failed")
	if _, err := service.ListPayables(ctx, companyID, ""); !errors.Is(err, payables.listErr) {
		t.Fatalf("payable list error = %v", err)
	}
	payables.listErr = nil
	payables.listLines = []apisubledger.Payable{mismatchPayable}
	if _, err := service.ListPayables(ctx, companyID, ""); !errors.Is(err, ErrCompanyMismatch) {
		t.Fatalf("payable list mismatch error = %v", err)
	}
	payables.listLines = nil
	if lines, err := service.ListPayables(ctx, companyID, "partner-1"); err != nil || len(lines) != 1 {
		t.Fatalf("payable list = %d err=%v", len(lines), err)
	}

	asOf := covDate(31)
	receivables.listLines = []apisubledger.Receivable{
		{CompanyID: companyID, DueDate: covDate(2), RemainingAmount: p1001MoneyRaw("10", "USD"), Status: "open"},
		{CompanyID: companyID, DueDate: covDate(2), RemainingAmount: p1001MoneyRaw("5", "USD"), Status: "closed"},
		{CompanyID: companyID, DueDate: covDate(2), RemainingAmount: p1001MoneyRaw("0", "USD"), Status: "open"},
		{CompanyID: companyID, RemainingAmount: p1001MoneyRaw("10", "USD"), Status: "open"},
		{CompanyID: companyID, DueDate: asOf.Add(-40 * 24 * time.Hour), RemainingAmount: p1001MoneyRaw("20", "USD"), Status: "open"},
		{CompanyID: companyID, DueDate: asOf.Add(-70 * 24 * time.Hour), RemainingAmount: p1001MoneyRaw("30", "USD"), Status: "open"},
		{CompanyID: companyID, DueDate: asOf.Add(-100 * 24 * time.Hour), RemainingAmount: p1001MoneyRaw("40", "USD"), Status: "open"},
		{CompanyID: companyID, DueDate: asOf, RemainingAmount: p1001MoneyRaw("50", "EUR"), Status: "open"},
		mismatchReceivable,
	}
	if _, err := service.Aging(ctx, companyID, apisubledger.SubledgerReceivable, "", asOf); !errors.Is(err, ErrCompanyMismatch) {
		t.Fatalf("receivable aging mismatch error = %v", err)
	}
	if _, err := service.Aging(ctx, types.CompanyID{}, apisubledger.SubledgerReceivable, "", asOf); !errors.Is(err, ErrCompanyScopeRequired) {
		t.Fatalf("aging nil company error = %v", err)
	}
	if _, err := service.Aging(ctx, companyID, apisubledger.SubledgerType("other"), "", asOf); !errors.Is(err, ErrInvalidARAPOperation) {
		t.Fatalf("aging invalid kind error = %v", err)
	}
	receivables.listErr = errors.New("list failed")
	if _, err := service.Aging(ctx, companyID, apisubledger.SubledgerReceivable, "", asOf); !errors.Is(err, receivables.listErr) {
		t.Fatalf("receivable aging list error = %v", err)
	}
	receivables.listErr = nil
	receivables.listLines = receivables.listLines[:len(receivables.listLines)-1]
	buckets, err := service.Aging(ctx, companyID, apisubledger.SubledgerReceivable, "", asOf)
	if err != nil || len(buckets) != 6 {
		t.Fatalf("aging buckets = %+v err=%v", buckets, err)
	}
	if buckets[0].RangeLabel != "current" || buckets[0].Currency != "EUR" ||
		buckets[1].RangeLabel != "1_30" || buckets[1].Count != 1 || !buckets[1].TotalAmount.Equal(decimal.NewFromInt(10)) ||
		buckets[2].RangeLabel != "31_60" || buckets[3].RangeLabel != "61_90" || buckets[4].RangeLabel != "current" ||
		buckets[5].RangeLabel != "over_90" {
		t.Fatalf("unexpected aging buckets: %+v", buckets)
	}
	if _, err := service.Aging(ctx, companyID, apisubledger.SubledgerPayable, "", asOf); err != nil {
		t.Fatalf("payable aging: %v", err)
	}
	payables.listErr = errors.New("list failed")
	if _, err := service.Aging(ctx, companyID, apisubledger.SubledgerPayable, "", asOf); !errors.Is(err, payables.listErr) {
		t.Fatalf("payable aging list error = %v", err)
	}
	payables.listErr = nil
	payables.listLines = []apisubledger.Payable{mismatchPayable}
	if _, err := service.Aging(ctx, companyID, apisubledger.SubledgerPayable, "", asOf); !errors.Is(err, ErrCompanyMismatch) {
		t.Fatalf("payable aging mismatch error = %v", err)
	}
	payables.listLines = nil
	if buckets, err := service.Aging(ctx, companyID, apisubledger.SubledgerPayable, "", time.Time{}); err != nil || len(buckets) != 1 {
		t.Fatalf("payable aging = %+v err=%v", buckets, err)
	}
	if app.posted != 0 {
		t.Fatalf("inquiries posted %d entries", app.posted)
	}
}

func TestCovARAPReconcileBranches(t *testing.T) {
	companyID, control, _, _, _, _, receivables, payables, balance, service := covARAPFixture(t)
	ctx := context.Background()
	today := time.Now().UTC()
	if _, err := service.Reconcile(ctx, types.CompanyID{}, apisubledger.SubledgerReceivable, control, "USD", today); !errors.Is(err, ErrInvalidARAPOperation) {
		t.Fatalf("nil company error = %v", err)
	}
	if _, err := service.Reconcile(ctx, companyID, apisubledger.SubledgerReceivable, types.AccountID{}, "USD", today); !errors.Is(err, ErrInvalidARAPOperation) {
		t.Fatalf("nil control error = %v", err)
	}
	if _, err := service.Reconcile(ctx, companyID, apisubledger.SubledgerReceivable, control, " ", today); !errors.Is(err, ErrInvalidARAPOperation) {
		t.Fatalf("blank currency error = %v", err)
	}
	if _, err := service.Reconcile(ctx, companyID, apisubledger.SubledgerReceivable, control, "USD", today.AddDate(0, 0, 1)); !errors.Is(err, ErrARAPHistoryUnavailable) {
		t.Fatalf("future as-of error = %v", err)
	}
	if _, err := service.Reconcile(ctx, companyID, apisubledger.SubledgerReceivable, control, "USD", today.AddDate(1, 0, 0)); !errors.Is(err, ErrARAPHistoryUnavailable) {
		t.Fatalf("different year error = %v", err)
	}
	if _, err := service.Reconcile(ctx, companyID, apisubledger.SubledgerType("other"), control, "USD", today); !errors.Is(err, ErrInvalidARAPOperation) {
		t.Fatalf("invalid kind error = %v", err)
	}
	receivables.listErr = errors.New("list failed")
	if _, err := service.Reconcile(ctx, companyID, apisubledger.SubledgerReceivable, control, "USD", today); !errors.Is(err, receivables.listErr) {
		t.Fatalf("receivable list error = %v", err)
	}
	receivables.listErr = nil
	payables.listErr = errors.New("list failed")
	if _, err := service.Reconcile(ctx, companyID, apisubledger.SubledgerPayable, control, "USD", today); !errors.Is(err, payables.listErr) {
		t.Fatalf("payable list error = %v", err)
	}
	payables.listErr = nil
	receivables.listLines = []apisubledger.Receivable{{CompanyID: types.NewCompanyID()}}
	if _, err := service.Reconcile(ctx, companyID, apisubledger.SubledgerReceivable, control, "USD", today); !errors.Is(err, ErrCompanyMismatch) {
		t.Fatalf("receivable mismatch error = %v", err)
	}
	payables.listLines = []apisubledger.Payable{{CompanyID: types.NewCompanyID()}}
	if _, err := service.Reconcile(ctx, companyID, apisubledger.SubledgerPayable, control, "USD", today); !errors.Is(err, ErrCompanyMismatch) {
		t.Fatalf("payable mismatch error = %v", err)
	}
	receivables.listLines = []apisubledger.Receivable{
		{CompanyID: companyID, ControlAccountID: types.NewAccountID(), RemainingAmount: p1001MoneyRaw("100", "USD"), Status: "open"},
		{CompanyID: companyID, ControlAccountID: control, RemainingAmount: p1001MoneyRaw("100", "USD"), FunctionalRemainingAmount: p1001MoneyRaw("40", "EUR"), Status: "open"},
	}
	if _, err := service.Reconcile(ctx, companyID, apisubledger.SubledgerReceivable, control, "USD", today); err == nil {
		t.Fatal("expected functional currency mismatch")
	}
	receivables.listLines = []apisubledger.Receivable{
		{CompanyID: companyID, ControlAccountID: control, RemainingAmount: p1001MoneyRaw("100", "USD"), Status: "closed"},
		{CompanyID: companyID, ControlAccountID: control, RemainingAmount: p1001MoneyRaw("25", "USD"), Status: "open"},
	}
	if _, err := functionalRemaining(p1001MoneyRaw("10", "EUR"), p1001MoneyRaw("10", "USD"), "USD"); err == nil {
		t.Fatal("expected functional currency mismatch")
	}
	if _, err := functionalRemaining(types.Money{}, p1001MoneyRaw("10", "EUR"), "USD"); err == nil {
		t.Fatal("expected remaining currency mismatch")
	}
	if value, err := functionalRemaining(types.Money{}, p1001MoneyRaw("10", "USD"), "USD"); err != nil || !value.Equal(p1001MoneyRaw("10", "USD")) {
		t.Fatalf("functional fallback = %v err=%v", value, err)
	}
	balance.err = errors.New("balance failed")
	if _, err := service.Reconcile(ctx, companyID, apisubledger.SubledgerReceivable, control, "USD", today); !errors.Is(err, balance.err) {
		t.Fatalf("balance error = %v", err)
	}
	balance.err = nil
	balance.amount = decimal.NewFromInt(25)
	report, err := service.Reconcile(ctx, companyID, apisubledger.SubledgerReceivable, control, "USD", time.Time{})
	if err != nil || !report.Reconciled || !report.SubledgerBalance.Equal(decimal.NewFromInt(25)) {
		t.Fatalf("receivable reconcile = %+v err=%v", report, err)
	}
	payables.listLines = []apisubledger.Payable{
		{CompanyID: companyID, ControlAccountID: control, RemainingAmount: p1001MoneyRaw("20", "USD"), Status: "open"},
		{CompanyID: companyID, ControlAccountID: control, RemainingAmount: p1001MoneyRaw("20", "USD"), FunctionalRemainingAmount: p1001MoneyRaw("20", "USD"), Status: "open"},
	}
	balance.amount = decimal.NewFromInt(-40)
	apReport, err := service.Reconcile(ctx, companyID, apisubledger.SubledgerPayable, control, "USD", today)
	if err != nil || !apReport.Reconciled || !apReport.SubledgerBalance.Equal(decimal.NewFromInt(40)) {
		t.Fatalf("payable reconcile = %+v err=%v", apReport, err)
	}
}

func TestCovARAPPaymentAndWriteOffBranches(t *testing.T) {
	companyID, control, _, cash, journalID, app, receivables, payables, _, service := covARAPFixture(t)
	ctx := publicapplication.WithRequestMetadata(context.Background(), publicapplication.RequestMetadata{IdempotencyKey: "pay-idem"})
	otherCompany := types.NewCompanyID()
	openID := "AR-OPEN"
	open := apisubledger.Receivable{
		ID: openID, CompanyID: companyID, PartnerID: "partner-1", ControlAccountID: control,
		RemainingAmount: p1001MoneyRaw("100", "USD"), FunctionalRemainingAmount: p1001MoneyRaw("100", "USD"), Status: "open",
	}
	receivables.values[openID] = open
	payment := apisubledger.PaymentCommand{
		CompanyID: companyID, Kind: apisubledger.SubledgerReceivable, CashAccountID: cash, JournalID: journalID,
		Amount: p1001MoneyRaw("40", "USD"), FunctionalAmount: p1001MoneyRaw("40", "USD"),
		PaymentDate: covDate(2), Reference: "REC-1",
		Allocations: []apisubledger.PaymentAllocation{{OpenItemID: openID, Amount: p1001MoneyRaw("40", "USD")}},
	}
	invalidPayments := []struct {
		name   string
		mutate func(*apisubledger.PaymentCommand)
	}{
		{"nil company", func(c *apisubledger.PaymentCommand) { c.CompanyID = types.CompanyID{} }},
		{"nil cash", func(c *apisubledger.PaymentCommand) { c.CashAccountID = types.AccountID{} }},
		{"nil journal", func(c *apisubledger.PaymentCommand) { c.JournalID = types.JournalID{} }},
		{"no allocations", func(c *apisubledger.PaymentCommand) { c.Allocations = nil }},
		{"blank reference", func(c *apisubledger.PaymentCommand) { c.Reference = " " }},
		{"invalid kind", func(c *apisubledger.PaymentCommand) { c.Kind = apisubledger.SubledgerType("other") }},
	}
	for _, tc := range invalidPayments {
		t.Run(tc.name, func(t *testing.T) {
			candidate := payment
			tc.mutate(&candidate)
			if _, err := service.RecordPayment(ctx, candidate); !errors.Is(err, ErrInvalidARAPOperation) {
				t.Fatalf("error = %v", err)
			}
		})
	}
	if _, err := service.RecordPayment(ctx, payment); err != nil || len(app.draftIDs) != 1 {
		t.Fatalf("deterministic payment: err=%v ids=%d", err, len(app.draftIDs))
	}
	if line := receivables.values[openID]; line.RemainingAmount.Amount().String() != "60" || line.Status != "open" {
		t.Fatalf("receivable after payment: %+v", line)
	}
	receivables.values[openID] = open
	app.postReplay = true
	if _, err := service.RecordPayment(ctx, payment); err != nil || len(app.draftIDs) != 2 {
		t.Fatalf("idempotent payment retry: err=%v ids=%d", err, len(app.draftIDs))
	}
	app.postReplay = false
	receivables.values[openID] = open
	receivables.getErr = errors.New("get failed")
	if _, err := service.RecordPayment(ctx, payment); !errors.Is(err, receivables.getErr) {
		t.Fatalf("get error = %v", err)
	}
	receivables.getErr = nil
	receivables.getLine = &apisubledger.Receivable{CompanyID: otherCompany}
	if _, err := service.RecordPayment(ctx, payment); !errors.Is(err, ErrInvalidOpenLine) {
		t.Fatalf("foreign receivable error = %v", err)
	}
	receivables.getLine = &apisubledger.Receivable{CompanyID: companyID, Status: "closed"}
	if _, err := service.RecordPayment(ctx, payment); !errors.Is(err, ErrInvalidOpenLine) {
		t.Fatalf("closed receivable error = %v", err)
	}
	receivables.getLine = nil
	receivables.values[openID] = open
	invalidAllocation := payment
	invalidAllocation.Allocations = []apisubledger.PaymentAllocation{{OpenItemID: openID, Amount: p1001MoneyRaw("40", "USD")}, {OpenItemID: openID, Amount: p1001MoneyRaw("1", "USD")}}
	if _, err := service.RecordPayment(ctx, invalidAllocation); !errors.Is(err, ErrInvalidARAPOperation) {
		t.Fatalf("duplicate allocation error = %v", err)
	}
	invalidAllocation.Allocations = []apisubledger.PaymentAllocation{{OpenItemID: "", Amount: p1001MoneyRaw("40", "USD")}}
	if _, err := service.RecordPayment(ctx, invalidAllocation); !errors.Is(err, ErrInvalidARAPOperation) {
		t.Fatalf("blank allocation error = %v", err)
	}
	overPayment := payment
	overPayment.Amount = p1001MoneyRaw("101", "USD")
	overPayment.FunctionalAmount = p1001MoneyRaw("101", "USD")
	if _, err := service.RecordPayment(ctx, overPayment); !errors.Is(err, ErrInvalidAmount) {
		t.Fatalf("over payment error = %v", err)
	}
	mismatchedTotal := payment
	mismatchedTotal.Amount = p1001MoneyRaw("41", "USD")
	if _, err := service.RecordPayment(ctx, mismatchedTotal); !errors.Is(err, ErrInvalidAmount) {
		t.Fatalf("mismatched total error = %v", err)
	}
	partialFunctional := payment
	partialFunctional.FunctionalAmount = p1001MoneyRaw("30", "USD")
	if _, err := service.RecordPayment(ctx, partialFunctional); err == nil {
		t.Fatal("expected functional total mismatch")
	}
	if _, err := proportionalFunctional(p1001MoneyRaw("10", "USD"), types.Money{}, p1001MoneyRaw("1", "USD")); !errors.Is(err, ErrInvalidAmount) {
		t.Fatalf("zero remaining error = %v", err)
	}
	if value, err := proportionalFunctional(types.Money{}, p1001MoneyRaw("10", "USD"), p1001MoneyRaw("1", "USD")); err != nil || !value.Equal(p1001MoneyRaw("1", "USD")) {
		t.Fatalf("proportional fallback = %v err=%v", value, err)
	}

	writeOff := apisubledger.WriteOffCommand{
		CompanyID: companyID, Kind: apisubledger.SubledgerReceivable, OpenItemID: openID,
		AdjustmentAccountID: control, JournalID: journalID, Amount: p1001MoneyRaw("10", "USD"),
		FunctionalAmount: p1001MoneyRaw("10", "USD"), Date: covDate(3), Reference: "WO-1",
	}
	invalidWriteOffs := []struct {
		name   string
		mutate func(*apisubledger.WriteOffCommand)
	}{
		{"nil company", func(c *apisubledger.WriteOffCommand) { c.CompanyID = types.CompanyID{} }},
		{"nil adjustment", func(c *apisubledger.WriteOffCommand) { c.AdjustmentAccountID = types.AccountID{} }},
		{"nil journal", func(c *apisubledger.WriteOffCommand) { c.JournalID = types.JournalID{} }},
		{"blank item", func(c *apisubledger.WriteOffCommand) { c.OpenItemID = "" }},
		{"blank reference", func(c *apisubledger.WriteOffCommand) { c.Reference = " " }},
		{"invalid kind", func(c *apisubledger.WriteOffCommand) { c.Kind = apisubledger.SubledgerType("other") }},
	}
	for _, tc := range invalidWriteOffs {
		t.Run(tc.name, func(t *testing.T) {
			candidate := writeOff
			tc.mutate(&candidate)
			if _, err := service.WriteOff(ctx, candidate); !errors.Is(err, ErrInvalidARAPOperation) {
				t.Fatalf("error = %v", err)
			}
		})
	}
	receivables.values[openID] = open
	if _, err := service.WriteOff(ctx, writeOff); err != nil {
		t.Fatalf("write off: %v", err)
	}
	receivables.values[openID] = open
	receivables.getLine = &apisubledger.Receivable{CompanyID: otherCompany}
	if _, err := service.WriteOff(ctx, writeOff); !errors.Is(err, ErrCompanyMismatch) {
		t.Fatalf("write-off mismatch error = %v", err)
	}
	receivables.getLine = &apisubledger.Receivable{CompanyID: companyID, Status: "closed"}
	if _, err := service.WriteOff(ctx, writeOff); !errors.Is(err, ErrInvalidOpenLine) {
		t.Fatalf("write-off closed error = %v", err)
	}
	receivables.getLine = nil
	receivables.values[openID] = open
	overWriteOff := writeOff
	overWriteOff.Amount = p1001MoneyRaw("101", "USD")
	if _, err := service.WriteOff(ctx, overWriteOff); !errors.Is(err, ErrInvalidAmount) {
		t.Fatalf("over write-off error = %v", err)
	}
	payables.values["AP-OPEN"] = apisubledger.Payable{
		ID: "AP-OPEN", CompanyID: companyID, PartnerID: "supplier-1", ControlAccountID: control,
		RemainingAmount: p1001MoneyRaw("20", "USD"), FunctionalRemainingAmount: p1001MoneyRaw("20", "USD"), Status: "open",
	}
	payableWriteOff := writeOff
	payableWriteOff.Kind = apisubledger.SubledgerPayable
	payableWriteOff.OpenItemID = "AP-OPEN"
	if _, err := service.WriteOff(ctx, payableWriteOff); err != nil {
		t.Fatalf("payable write off: %v", err)
	}
	payablePayment := payment
	payablePayment.Kind = apisubledger.SubledgerPayable
	payablePayment.Amount = p1001MoneyRaw("5", "USD")
	payablePayment.FunctionalAmount = p1001MoneyRaw("5", "USD")
	payablePayment.Allocations = []apisubledger.PaymentAllocation{{OpenItemID: "AP-OPEN", Amount: p1001MoneyRaw("5", "USD")}}
	if _, err := service.RecordPayment(ctx, payablePayment); err != nil {
		t.Fatalf("payable payment: %v", err)
	}
}

func TestCovDeprecatedARAPWrappers(t *testing.T) {
	companyID, partnerID, controlID, journalID, counterpartID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	amount := covMoney(t, "100", "USD")
	arRepo := &covPostedRepo{
		MemoryRepository: NewMemoryRepository(),
		account:          covAccount(companyID, controlID),
		entry:            covControlEntry(companyID, journalID, controlID, partnerID, counterpartID, LineReceivable, posting.EntryPosted),
	}
	apAccount := covAccount(companyID, controlID)
	apAccount.Type = masterdata.AccountTypeLiability
	apAccount.NormalBalance = masterdata.NormalBalanceCredit
	apRepo := &covPostedRepo{
		MemoryRepository: NewMemoryRepository(),
		account:          apAccount,
		entry:            covControlEntry(companyID, journalID, controlID, partnerID, counterpartID, LinePayable, posting.EntryPosted),
	}

	ar := NewReceivableLedger(arRepo)
	if _, err := ar.AddInvoiceWithControl(partnerID, controlID, journalID, amount, covDate(1), covDate(3), "AR-1"); err != nil {
		t.Fatalf("receivable AddInvoiceWithControl: %v", err)
	}
	if _, err := ar.AddInvoiceForCompany(companyID, partnerID, amount, covDate(1), covDate(3), "AR-2"); err != nil {
		t.Fatalf("receivable AddInvoiceForCompany: %v", err)
	}
	if _, err := ar.AddInvoiceForCompanyWithControl(companyID, partnerID, controlID, journalID, amount, covDate(1), covDate(3), "AR-3"); err != nil {
		t.Fatalf("receivable AddInvoiceForCompanyWithControl: %v", err)
	}
	if _, err := ar.AddInvoiceFromPostedEntry(partnerID, controlID, journalID, amount, covDate(1), covDate(3), "AR-4"); !errors.Is(err, ErrInvalidOpenLine) {
		t.Fatalf("unscoped posted invoice error = %v", err)
	}
	arScoped := NewReceivableLedgerForCompany(arRepo, companyID)
	if _, err := arScoped.AddInvoiceFromPostedEntry(partnerID, controlID, journalID, amount, covDate(1), covDate(3), "AR-5"); err != nil {
		t.Fatalf("receivable AddInvoiceFromPostedEntry: %v", err)
	}
	if _, err := arScoped.AddInvoiceFromPostedEntryForCompany(companyID, partnerID, controlID, journalID, amount, covDate(1), covDate(3), "AR-6"); err != nil {
		t.Fatalf("receivable AddInvoiceFromPostedEntryForCompany: %v", err)
	}

	ap := NewPayableLedger(apRepo)
	if _, err := ap.AddInvoiceWithControl(partnerID, controlID, journalID, amount, covDate(1), covDate(3), "AP-1"); err != nil {
		t.Fatalf("payable AddInvoiceWithControl: %v", err)
	}
	if _, err := ap.AddInvoiceForCompany(companyID, partnerID, amount, covDate(1), covDate(3), "AP-2"); err != nil {
		t.Fatalf("payable AddInvoiceForCompany: %v", err)
	}
	if _, err := ap.AddInvoiceForCompanyWithControl(companyID, partnerID, controlID, journalID, amount, covDate(1), covDate(3), "AP-3"); err != nil {
		t.Fatalf("payable AddInvoiceForCompanyWithControl: %v", err)
	}
	if _, err := ap.AddInvoiceFromPostedEntry(partnerID, controlID, journalID, amount, covDate(1), covDate(3), "AP-4"); !errors.Is(err, ErrInvalidOpenLine) {
		t.Fatalf("unscoped posted payable error = %v", err)
	}
	apScoped := NewPayableLedgerForCompany(apRepo, companyID)
	if _, err := apScoped.AddInvoiceFromPostedEntry(partnerID, controlID, journalID, amount, covDate(1), covDate(3), "AP-5"); err != nil {
		t.Fatalf("payable AddInvoiceFromPostedEntry: %v", err)
	}
	if _, err := apScoped.AddInvoiceFromPostedEntryForCompany(companyID, partnerID, controlID, journalID, amount, covDate(1), covDate(3), "AP-6"); err != nil {
		t.Fatalf("payable AddInvoiceFromPostedEntryForCompany: %v", err)
	}
	if _, err := ap.PostInvoiceEntry(nil, nil, partnerID, controlID, amount, covDate(1), covDate(3), "AP-7", "poster"); !errors.Is(err, ErrInvalidOpenLine) {
		t.Fatalf("PostInvoiceEntry error = %v", err)
	}
}

func TestCovTreasuryWrappersAndBankFeeErrors(t *testing.T) {
	companyID, accountID, journalID, feeAccountID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	repo := NewMemoryRepository()
	ledger := NewTreasuryLedgerForCompany(repo, companyID)
	amount := covMoney(t, "25", "USD")
	item, err := ledger.RecordItemForCompany(companyID, accountID, covDate(2), amount, "FEE-1", BankItemUnrecorded)
	if err != nil || item.CompanyID != companyID {
		t.Fatalf("RecordItemForCompany = %+v err=%v", item, err)
	}
	if _, err := ledger.RecordItemForCompany(uuid.Nil, accountID, covDate(2), amount, "FEE-2", BankItemUnrecorded); !errors.Is(err, ErrCompanyScopeRequired) {
		t.Fatalf("nil company error = %v", err)
	}
	if _, err := ledger.RecordItemForCompanyContext(context.Background(), uuid.New(), accountID, covDate(2), amount, "FEE-3", BankItemUnrecorded); !errors.Is(err, ErrCompanyMismatch) {
		t.Fatalf("foreign company error = %v", err)
	}
	if !sameDay(covDate(2), covDate(2).Add(time.Hour)) || sameDay(covDate(2), covDate(3)) {
		t.Fatal("sameDay comparison failed")
	}
	params := BankFeeParams{
		JournalID: journalID, Date: covDate(2), FeeAccountID: feeAccountID,
		BankAccountID: accountID, Description: "service fee",
	}
	approval := BankAdjustmentApproval{
		ApprovalID: "APP-1", ApprovedBy: "controller", Reason: "fee", ApprovedAt: covDate(2),
	}
	if _, _, err := ledger.ProcessBankFeeAtomic(item, params); !errors.Is(err, ErrBankAdjustmentApprovalRequired) {
		t.Fatalf("unapproved fee error = %v", err)
	}
	if _, _, err := ledger.ProcessBankFeeApprovedAtomicContext(nil, item, params, approval); !errors.Is(err, ErrInvalidOpenLine) {
		t.Fatalf("nil context error = %v", err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := ledger.ProcessBankFeeApprovedAtomicContext(cancelled, item, params, approval); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled fee error = %v", err)
	}
	invalidItem := item
	invalidItem.ID = uuid.Nil
	if _, _, err := ledger.ProcessBankFeeApprovedAtomicContext(context.Background(), invalidItem, params, approval); !errors.Is(err, ErrInvalidAmount) {
		t.Fatalf("invalid item error = %v", err)
	}
	invalidParams := params
	invalidParams.JournalID = uuid.Nil
	if _, _, err := ledger.ProcessBankFeeApprovedAtomicContext(context.Background(), item, invalidParams, approval); !errors.Is(err, ErrInvalidOpenLine) {
		t.Fatalf("invalid params error = %v", err)
	}
	foreignItem := item
	foreignItem.CompanyID = uuid.New()
	if _, _, err := ledger.ProcessBankFeeApprovedAtomicContext(context.Background(), foreignItem, params, approval); !errors.Is(err, ErrCompanyMismatch) {
		t.Fatalf("foreign item error = %v", err)
	}
	updated, draft, err := ledger.ProcessBankFeeApprovedAtomicContext(context.Background(), item, params, approval)
	if err != nil || updated.Status != BankItemMatched || len(draft.Lines) != 2 {
		t.Fatalf("approved fee = %+v draft=%+v err=%v", updated, draft, err)
	}
}

func TestCovRegisterAssetContextDefaultsAndComponents(t *testing.T) {
	companyID := uuid.New()
	repo := NewMemoryRepository()
	ledger := NewFixedAssetLedgerForCompany(repo, nil, companyID)
	asset := covAssetFixture(t, companyID, uuid.New())
	asset.AccumulatedDepreciation = financialtypes.Money{}
	asset.AccumulatedImpairment = financialtypes.Money{}
	asset.RevaluationAdjustment = financialtypes.Money{}
	asset.RevaluationSurplus = financialtypes.Money{}
	asset.CurrentBookValue = financialtypes.Money{}
	asset.RemainingUsefulLifeMonths = 0
	asset.Components = []AssetComponent{
		{
			Name: "component-without-defaults", Cost: covMoney(t, "600", "USD"),
			SalvageValue: covMoney(t, "0", "USD"), UsefulLifeMonths: 24, Method: StraightLine,
		},
		{
			ID: uuid.New(), Name: "component-with-defaults", Cost: covMoney(t, "400", "USD"),
			SalvageValue: covMoney(t, "0", "USD"), UsefulLifeMonths: 24, Method: StraightLine,
			InServiceDate: covDate(2), AccumulatedDepreciation: covMoney(t, "0", "USD"),
		},
	}

	if err := ledger.RegisterAssetContext(context.Background(), asset); err != nil {
		t.Fatalf("RegisterAssetContext(): %v", err)
	}
	saved, err := repo.GetFixedAsset(context.Background(), companyID, asset.ID)
	if err != nil {
		t.Fatal(err)
	}
	if saved.RemainingUsefulLifeMonths <= 0 || saved.RemainingUsefulLifeMonths > saved.UsefulLifeMonths ||
		saved.CurrentBookValue.Currency() != "USD" {
		t.Fatalf("derived asset fields = %+v", saved)
	}
	for _, component := range saved.Components {
		if component.ID == uuid.Nil || component.InServiceDate.IsZero() || component.AccumulatedDepreciation.Currency() != "USD" {
			t.Fatalf("component defaults = %+v", component)
		}
	}
}

func TestCovDisposalLinesBranches(t *testing.T) {
	asset := covAssetFixture(t, uuid.New(), uuid.New())
	asset.AccumulatedDepreciation = covMoney(t, "100", "USD")
	asset.AccumulatedImpairment = covMoney(t, "50", "USD")
	asset.CurrentBookValue = covMoney(t, "850", "USD")

	gainLines, err := disposalLines(asset, covMoney(t, "900", "USD"))
	if err != nil || len(gainLines) != 5 || gainLines[4].AccountID != asset.DisposalGainAccountID ||
		!gainLines[4].Credit.Equal(covMoney(t, "50", "USD")) {
		t.Fatalf("gain lines = %+v err=%v", gainLines, err)
	}
	lossLines, err := disposalLines(asset, covMoney(t, "800", "USD"))
	if err != nil || len(lossLines) != 5 || lossLines[4].AccountID != asset.DisposalLossAccountID ||
		!lossLines[4].Debit.Equal(covMoney(t, "50", "USD")) {
		t.Fatalf("loss lines = %+v err=%v", lossLines, err)
	}

	noImpairmentAccount := asset
	noImpairmentAccount.AccumulatedImpairmentAccountID = uuid.Nil
	lines, err := disposalLines(noImpairmentAccount, covMoney(t, "850", "USD"))
	if err != nil || len(lines) != 4 || lines[2].AccountID != asset.AccumulatedAccountID {
		t.Fatalf("fallback impairment line = %+v err=%v", lines, err)
	}

	mismatched := asset
	mismatched.CurrentBookValue = covMoney(t, "800", "USD")
	if _, err := disposalLines(mismatched, covMoney(t, "850", "USD")); !errors.Is(err, ErrInvalidAsset) {
		t.Fatalf("book value mismatch error = %v", err)
	}
	if _, err := disposalLines(asset, covMoney(t, "850", "EUR")); err == nil {
		t.Fatal("expected proceeds currency mismatch error")
	}
}
