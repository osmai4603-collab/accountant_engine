package ledger

import (
	"testing"
	"time"

	"github.com/osmai4603-collab/accounting_engine/internal/kernal/journal"
	"github.com/osmai4603-collab/accounting_engine/internal/kernal/types"
	"github.com/shopspring/decimal"
)

func TestProjectPostedMovementsIsDeterministic(t *testing.T) {
	company := types.NewCompanyID()
	journalID := types.NewJournalID()
	periodID := types.NewFiscalPeriodID()
	cash := types.NewAccountID()
	revenue := types.NewAccountID()

	entry1 := postedEntry(company, journalID, periodID, 1, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), cash, revenue, "100.00")
	entry2 := postedEntry(company, journalID, periodID, 2, time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), cash, revenue, "50.00")
	draft := entry2
	draft.Status = journal.EntryDraft
	draft.ID = types.NewJournalEntryID()
	for i := range draft.Lines {
		draft.Lines[i].ID = types.NewJournalEntryLineID()
	}

	a, err := ProjectPostedMovements([]journal.JournalEntry{entry2, draft, entry1})
	if err != nil {
		t.Fatalf("project A: %v", err)
	}
	b, err := ProjectPostedMovements([]journal.JournalEntry{entry1, entry2, draft})
	if err != nil {
		t.Fatalf("project B: %v", err)
	}
	if len(a) != 4 {
		t.Fatalf("expected 4 movements, got %d", len(a))
	}
	for i := range a {
		if a[i].EntryID != b[i].EntryID || a[i].LineID != b[i].LineID || a[i].AccountID != b[i].AccountID {
			t.Fatalf("movement %d differs: %#v vs %#v", i, a[i], b[i])
		}
	}
	if a[0].EntryID != entry1.ID || a[2].EntryID != entry2.ID {
		t.Fatal("movements not in canonical entry order")
	}
}

func TestRebuildAccountBalancesDeterministic(t *testing.T) {
	company := types.NewCompanyID()
	journalID := types.NewJournalID()
	periodID := types.NewFiscalPeriodID()
	cash := types.NewAccountID()
	revenue := types.NewAccountID()
	entry1 := postedEntry(company, journalID, periodID, 1, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), cash, revenue, "100.00")
	entry2 := postedEntry(company, journalID, periodID, 2, time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), cash, revenue, "40.00")

	b1, err := RebuildAccountBalances([]journal.JournalEntry{entry1, entry2})
	if err != nil {
		t.Fatalf("rebuild 1: %v", err)
	}
	b2, err := RebuildAccountBalances([]journal.JournalEntry{entry2, entry1})
	if err != nil {
		t.Fatalf("rebuild 2: %v", err)
	}
	cashKey := AccountBalanceKey{CompanyID: company, AccountID: cash, FunctionalCurrency: "USD"}
	revKey := AccountBalanceKey{CompanyID: company, AccountID: revenue, FunctionalCurrency: "USD"}
	if !b1[cashKey].Net.Equal(b2[cashKey].Net) || !b1[revKey].Net.Equal(b2[revKey].Net) {
		t.Fatal("rebuild balances differ by input order")
	}
	if b1[cashKey].Net.Amount().Cmp(decimal.RequireFromString("140.00")) != 0 {
		t.Fatalf("cash net = %s", b1[cashKey].Net.Amount())
	}
	if b1[revKey].Net.Amount().Cmp(decimal.RequireFromString("-140.00")) != 0 {
		t.Fatalf("revenue net = %s", b1[revKey].Net.Amount())
	}
}

func TestPostedMovementIncludesRequiredFields(t *testing.T) {
	company := types.NewCompanyID()
	journalID := types.NewJournalID()
	periodID := types.NewFiscalPeriodID()
	account := types.NewAccountID()
	other := types.NewAccountID()
	entry := postedEntry(company, journalID, periodID, 7, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), account, other, "10.00")
	partner := types.NewPartnerID()
	entry.Lines[0].PartnerID = &partner
	entry.SourceModule = "billing"
	entry.ReferenceID = "INV-1"

	movements, err := ProjectPostedMovements([]journal.JournalEntry{entry})
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	var mv PostedMovement
	found := false
	for _, m := range movements {
		if m.LineID == entry.Lines[0].ID {
			mv = m
			found = true
			break
		}
	}
	if !found {
		t.Fatal("movement for line not found")
	}
	if mv.CompanyID != company || mv.JournalID != journalID || mv.FiscalPeriodID != periodID || mv.EntryID != entry.ID || mv.LineID != entry.Lines[0].ID {
		t.Fatal("movement missing identity fields")
	}
	if mv.AccountID != account || mv.FunctionalCurrency != "USD" || mv.TransactionCurrency != "USD" {
		t.Fatal("movement missing account/currency fields")
	}
	if !mv.FunctionalDebit.Amount().Equal(decimal.RequireFromString("10.00")) {
		t.Fatal("movement missing amount")
	}
	if mv.PartnerID == nil || *mv.PartnerID != partner {
		t.Fatal("movement missing dimension")
	}
	if mv.SourceModule != "billing" || mv.ReferenceID != "INV-1" {
		t.Fatal("movement missing source")
	}
	if mv.EffectiveDate.IsZero() || mv.PostingDate.IsZero() {
		t.Fatal("movement missing dates")
	}
}

func TestRebuildExcludesDraftsAndRejectsOrphanPostedLine(t *testing.T) {
	company := types.NewCompanyID()
	journalID := types.NewJournalID()
	periodID := types.NewFiscalPeriodID()
	a1 := types.NewAccountID()
	a2 := types.NewAccountID()
	draft := postedEntry(company, journalID, periodID, 1, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), a1, a2, "5.00")
	draft.Status = journal.EntryDraft
	balances, err := RebuildAccountBalances([]journal.JournalEntry{draft})
	if err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	if len(balances) != 0 {
		t.Fatalf("draft should not contribute balances: %#v", balances)
	}

	bad := postedEntry(company, journalID, periodID, 2, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), a1, a2, "5.00")
	bad.Lines[0].ID = types.JournalEntryLineID{}
	if _, err := ProjectPostedMovements([]journal.JournalEntry{bad}); err == nil {
		t.Fatal("expected error for posted line missing id")
	}
}

func TestAccountTypeRevenueCompatibilityAlias(t *testing.T) {
	if AccountTypeRevenue != AccountTypeIncome {
		t.Fatalf("revenue alias must equal income: %q vs %q", AccountTypeRevenue, AccountTypeIncome)
	}
	if string(AccountTypeRevenue) != "income" {
		t.Fatalf("revenue alias value must be income, got %q", AccountTypeRevenue)
	}
}

func postedEntry(company types.CompanyID, journalID types.JournalID, periodID types.FiscalPeriodID, seq int64, date time.Time, debitAcc, creditAcc types.AccountID, amount string) journal.JournalEntry {
	amt := mustMoney(amount, "USD")
	zero := mustMoney("0", "USD")
	postedAt := date.Add(time.Hour)
	return journal.JournalEntry{
		ID:             types.NewJournalEntryID(),
		CompanyID:      company,
		JournalID:      journalID,
		FiscalPeriodID: periodID,
		EntryNumber:    "JE",
		SequenceNumber: seq,
		Status:         journal.EntryPosted,
		Classification: journal.EntryClassRegular,
		EffectiveDate:  date,
		PostingDate:    postedAt,
		CreatedBy:      "u",
		CreatedAt:      date,
		UpdatedAt:      postedAt,
		PostedBy:       "p",
		PostedAt:       &postedAt,
		Lines: []journal.EntryLine{{
			ID:                  types.NewJournalEntryLineID(),
			AccountID:           debitAcc,
			TransactionAmount:   amt,
			TransactionCurrency: "USD",
			ExchangeRate:        decimal.NewFromInt(1),
			FunctionalDebit:     amt,
			FunctionalCredit:    zero,
		}, {
			ID:                  types.NewJournalEntryLineID(),
			AccountID:           creditAcc,
			TransactionAmount:   amt,
			TransactionCurrency: "USD",
			ExchangeRate:        decimal.NewFromInt(1),
			FunctionalDebit:     zero,
			FunctionalCredit:    amt,
		}},
	}
}

func TestRebuildIncludesReversedEntriesAndReversalNetImpact(t *testing.T) {
	company := types.NewCompanyID()
	journalID := types.NewJournalID()
	periodID := types.NewFiscalPeriodID()
	cash := types.NewAccountID()
	revenue := types.NewAccountID()

	original := postedEntry(company, journalID, periodID, 1, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), cash, revenue, "100.00")
	original.Status = journal.EntryReversed
	reversal := postedEntry(company, journalID, periodID, 2, time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), cash, revenue, "100.00")
	reversal.Classification = journal.EntryClassReversing
	reversal.ReversalOfID = &original.ID
	original.ReversedByID = &reversal.ID
	// swap debit/credit in reversal lines to represent actual reversal
	reversal.Lines[0].FunctionalDebit, reversal.Lines[0].FunctionalCredit = reversal.Lines[0].FunctionalCredit, reversal.Lines[0].FunctionalDebit
	reversal.Lines[1].FunctionalDebit, reversal.Lines[1].FunctionalCredit = reversal.Lines[1].FunctionalCredit, reversal.Lines[1].FunctionalDebit

	balances, err := RebuildAccountBalances([]journal.JournalEntry{original, reversal})
	if err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	cashKey := AccountBalanceKey{CompanyID: company, AccountID: cash, FunctionalCurrency: "USD"}
	revKey := AccountBalanceKey{CompanyID: company, AccountID: revenue, FunctionalCurrency: "USD"}
	if !balances[cashKey].Net.Amount().IsZero() || !balances[revKey].Net.Amount().IsZero() {
		t.Fatalf("expected zero net after reversal: cash=%s rev=%s", balances[cashKey].Net.Amount(), balances[revKey].Net.Amount())
	}
}

func TestProjectPostedMovementsRejectsOrphanIdentity(t *testing.T) {
	company := types.NewCompanyID()
	journalID := types.NewJournalID()
	periodID := types.NewFiscalPeriodID()
	a1 := types.NewAccountID()
	a2 := types.NewAccountID()
	entry := postedEntry(company, journalID, periodID, 1, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), a1, a2, "5.00")
	entry.CompanyID = types.CompanyID{}
	if _, err := ProjectPostedMovements([]journal.JournalEntry{entry}); err == nil {
		t.Fatal("expected missing company id error")
	}
}

func TestProjectionAndRebuildEdgeCases(t *testing.T) {
	company := types.NewCompanyID()
	journalID := types.NewJournalID()
	periodID := types.NewFiscalPeriodID()
	a1 := types.NewAccountID()
	a2 := types.NewAccountID()

	date1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	date2 := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)

	e1 := postedEntry(company, journalID, periodID, 2, date1, a1, a2, "10.00")
	e2 := postedEntry(company, journalID, periodID, 1, date1, a1, a2, "20.00")
	e3 := postedEntry(company, journalID, periodID, 1, date2, a1, a2, "30.00")

	if ComparePostedEntries(e1, e3) >= 0 || ComparePostedEntries(e3, e1) <= 0 {
		t.Fatal("ComparePostedEntries must order by EffectiveDate first")
	}
	if ComparePostedEntries(e2, e1) >= 0 || ComparePostedEntries(e1, e2) <= 0 {
		t.Fatal("ComparePostedEntries must order by SequenceNumber second")
	}
	if ComparePostedEntries(e1, e1) != 0 {
		t.Fatal("ComparePostedEntries must return 0 for identical entry")
	}
	if CompareEntryLines(e1.Lines[0], e1.Lines[0]) != 0 {
		t.Fatal("CompareEntryLines must return 0 for identical line")
	}

	// Missing account ID on posted line
	badAcc := postedEntry(company, journalID, periodID, 3, date1, a1, a2, "5.00")
	badAcc.Lines[0].AccountID = types.AccountID{}
	if _, err := ProjectPostedMovements([]journal.JournalEntry{badAcc}); err == nil {
		t.Fatal("expected error for missing account id")
	}

	// Missing functional currency on posted line
	badCur := postedEntry(company, journalID, periodID, 4, date1, a1, a2, "5.00")
	badCur.Lines[0].FunctionalDebit = types.Money{}
	badCur.Lines[0].FunctionalCredit = types.Money{}
	if _, err := ProjectPostedMovements([]journal.JournalEntry{badCur}); err == nil {
		t.Fatal("expected error for missing functional currency")
	}
	if _, err := RebuildAccountBalances([]journal.JournalEntry{badCur}); err == nil {
		t.Fatal("RebuildAccountBalances must propagate projection errors")
	}

	// Nil dimensions remain nil after projection
	mvs, err := ProjectPostedMovements([]journal.JournalEntry{e1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mvs) != 2 || mvs[0].CostCenterID != nil || mvs[0].ProjectID != nil || mvs[0].SegmentID != nil || mvs[0].BusinessUnitID != nil {
		t.Fatal("expected nil dimensions to remain nil")
	}
}

func mustMoney(amount, currency string) types.Money {
	m, err := types.NewMoney(decimal.RequireFromString(amount), currency)
	if err != nil {
		panic(err)
	}
	return m
}
