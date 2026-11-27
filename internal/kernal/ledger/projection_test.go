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
	cancelled := entry2
	cancelled.Status = journal.EntryCancelled
	cancelled.ID = types.NewJournalEntryID()
	for i := range cancelled.Lines {
		cancelled.Lines[i].ID = types.NewJournalEntryLineID()
	}

	a, err := ProjectPostedMovements([]journal.JournalEntry{entry2, draft, cancelled, entry1})
	if err != nil {
		t.Fatalf("project A: %v", err)
	}
	b, err := ProjectPostedMovements([]journal.JournalEntry{entry1, cancelled, entry2, draft})
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

	// Non-nil dimensions and reversal entries clone cleanly without shared pointers.
	cc := types.NewCostCenterID()
	proj := types.NewProjectID()
	seg := types.NewSegmentID()
	bu := types.NewBusinessUnitID()
	rev := postedEntry(company, journalID, periodID, 5, date2, a1, a2, "10.00")
	rev.Status = journal.EntryReversed
	rev.ReversalOfID = &e1.ID
	rev.ReversedByID = &e3.ID
	rev.Lines[0].CostCenterID = &cc
	rev.Lines[0].ProjectID = &proj
	rev.Lines[0].SegmentID = &seg
	rev.Lines[0].BusinessUnitID = &bu
	mvs, err = ProjectPostedMovements([]journal.JournalEntry{rev})
	if err != nil || len(mvs) != 2 {
		t.Fatalf("ProjectPostedMovements(reversed) = (%d, %v)", len(mvs), err)
	}
	// ProjectPostedMovements sorts lines by ID, so locate the dimensioned line
	// explicitly instead of assuming it is the first movement.
	var projected PostedMovement
	for _, mv := range mvs {
		if mv.LineID == rev.Lines[0].ID {
			projected = mv
			break
		}
	}
	if projected.LineID == (types.JournalEntryLineID{}) {
		t.Fatal("projected movement for the dimensioned line was not found")
	}
	if projected.CostCenterID == nil || projected.ProjectID == nil || projected.SegmentID == nil || projected.BusinessUnitID == nil ||
		*projected.CostCenterID != cc || *projected.ProjectID != proj || *projected.SegmentID != seg || *projected.BusinessUnitID != bu ||
		*projected.ReversalOfID != e1.ID || *projected.ReversedByID != e3.ID {
		t.Fatalf("projected dimension/reversal pointers missing: %+v", projected)
	}
	*projected.CostCenterID = types.NewCostCenterID()
	if *rev.Lines[0].CostCenterID != cc {
		t.Fatal("mutating projected movement mutated input entry dimension")
	}

	// ComparePostedEntries ties resolved by EntryID and empty RebuildAccountBalances.
	eSameDateSeq := e1.Clone()
	eSameDateSeq.ID = types.NewJournalEntryID()
	if ComparePostedEntries(e1, eSameDateSeq) == 0 {
		t.Fatal("distinct EntryIDs with same date and sequence must not compare 0")
	}
	emptyBal, err := RebuildAccountBalances(nil)
	if err != nil || len(emptyBal) != 0 {
		t.Fatalf("RebuildAccountBalances(nil) = (%+v, %v), want empty", emptyBal, err)
	}
	// Multi-entry balances including EUR currency branch and debit vs credit lines.
	eurAmt := mustMoney("40.00", "EUR")
	eurZero := mustMoney("0.00", "EUR")
	eEUR := postedEntry(company, journalID, periodID, 6, date2, a2, a1, "40.00")
	eEUR.TransactionCurrency = "EUR"
	eEUR.Lines[0].TransactionAmount, eEUR.Lines[0].TransactionCurrency = eurAmt, "EUR"
	eEUR.Lines[0].FunctionalDebit, eEUR.Lines[0].FunctionalCredit = eurAmt, eurZero
	eEUR.Lines[1].TransactionAmount, eEUR.Lines[1].TransactionCurrency = eurAmt, "EUR"
	eEUR.Lines[1].FunctionalDebit, eEUR.Lines[1].FunctionalCredit = eurZero, eurAmt
	rebuilt, err := RebuildAccountBalances([]journal.JournalEntry{e1, eEUR})
	if err != nil || len(rebuilt) != 4 {
		t.Fatalf("RebuildAccountBalances(USD+EUR) = (%d, %v), want 4 keys", len(rebuilt), err)
	}
}

func TestRebuildForeignCurrencyBalancesAndOrdering(t *testing.T) {
	company := types.NewCompanyID()
	journalID := types.NewJournalID()
	periodID := types.NewFiscalPeriodID()
	fxBank := types.NewAccountID()
	apAccount := types.NewAccountID()
	ccA := types.NewCostCenterID()
	ccB := types.NewCostCenterID()

	d1 := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	d2 := time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC)
	d3 := time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC)

	makeFXEntry := func(seq int64, date time.Time, debitAcc, creditAcc types.AccountID, txCur, txAmt, funcAmt string, cc *types.CostCenterID) journal.JournalEntry {
		e := postedEntry(company, journalID, periodID, seq, date, debitAcc, creditAcc, funcAmt)
		e.TransactionCurrency = txCur
		e.ExchangeRate = decimal.RequireFromString("3.75")
		txMoney := mustMoney(txAmt, txCur)
		funcMoney := mustMoney(funcAmt, "SAR")
		funcZero := mustMoney("0.00", "SAR")
		e.Lines[0].TransactionAmount = txMoney
		e.Lines[0].TransactionCurrency = txCur
		e.Lines[0].ExchangeRate = e.ExchangeRate
		e.Lines[0].FunctionalDebit = funcMoney
		e.Lines[0].FunctionalCredit = funcZero
		e.Lines[0].CostCenterID = cc
		e.Lines[1].TransactionAmount = txMoney
		e.Lines[1].TransactionCurrency = txCur
		e.Lines[1].ExchangeRate = e.ExchangeRate
		e.Lines[1].FunctionalDebit = funcZero
		e.Lines[1].FunctionalCredit = funcMoney
		e.Lines[1].CostCenterID = cc
		return e
	}

	e1 := makeFXEntry(1, d1, fxBank, apAccount, "USD", "100.00", "375.00", &ccA)
	e2 := makeFXEntry(2, d2, fxBank, apAccount, "USD", "50.00", "187.50", &ccB)
	e3 := makeFXEntry(3, d3, fxBank, apAccount, "USD", "25.00", "93.75", &ccA)
	// Swap d3 to be after cutoff and add a cancelled entry that must be excluded.
	cancelled := e1.Clone()
	cancelled.ID = types.NewJournalEntryID()
	cancelled.Status = journal.EntryCancelled
	for i := range cancelled.Lines {
		cancelled.Lines[i].ID = types.NewJournalEntryLineID()
	}

	entries := []journal.JournalEntry{e3, e1, cancelled, e2}
	// Total USD across cost centers up to d2 (+150 USD, +562.50 SAR on fxBank; -150 USD on apAccount)
	asOfEnd := e2.PostingDate
	totals, err := RebuildForeignCurrencyBalances(entries, ForeignCurrencyBalanceFilter{
		CompanyID:           company,
		TransactionCurrency: "usd",
		AsOf:                asOfEnd,
	})
	if err != nil {
		t.Fatalf("RebuildForeignCurrencyBalances failed: %v", err)
	}
	bankKey := ForeignCurrencyBalanceKey{CompanyID: company, AccountID: fxBank, TransactionCurrency: "USD", FunctionalCurrency: "SAR"}
	bankBal := totals[bankKey]
	if !bankBal.TransactionNet.Equal(decimal.RequireFromString("150.00")) ||
		!bankBal.FunctionalNet.Amount().Equal(decimal.RequireFromString("562.50")) ||
		bankBal.MovementCount != 2 {
		t.Fatalf("unexpected bank FX balance: %+v", bankBal)
	}
	if mag, isCredit := bankBal.UnsignedTransactionPosition(); isCredit || !mag.Equal(decimal.RequireFromString("150.00")) {
		t.Fatalf("bank UnsignedTransactionPosition = (%s, %v)", mag, isCredit)
	}
	apKey := ForeignCurrencyBalanceKey{CompanyID: company, AccountID: apAccount, TransactionCurrency: "USD", FunctionalCurrency: "SAR"}
	apBal := totals[apKey]
	if mag, isCredit := apBal.UnsignedTransactionPosition(); !isCredit || !mag.Equal(decimal.RequireFromString("150.00")) {
		t.Fatalf("ap UnsignedTransactionPosition = (%s, %v), want (150.00, true)", mag, isCredit)
	}

	// Dimension-grouped rebuild distinguishes ccA and ccB (VAL-009).
	byDim, err := RebuildForeignCurrencyBalances(entries, ForeignCurrencyBalanceFilter{
		CompanyID:           company,
		AccountID:           fxBank,
		TransactionCurrency: "USD",
		GroupByDimensions:   true,
	})
	if err != nil {
		t.Fatalf("GroupByDimensions rebuild failed: %v", err)
	}
	if len(byDim) != 2 {
		t.Fatalf("expected 2 dimension buckets, got %d", len(byDim))
	}
	sorted := SortedForeignCurrencyBalances(byDim)
	if len(sorted) != 2 || sorted[0].CostCenterID == nil || sorted[1].CostCenterID == nil {
		t.Fatalf("expected sorted slice with populated CostCenterIDs, got %+v", sorted)
	}

	// Specific cost center filter narrows results accurately.
	filteredCC, err := RebuildForeignCurrencyBalances(entries, ForeignCurrencyBalanceFilter{
		CompanyID:         company,
		AccountID:         fxBank,
		CostCenterID:      &ccA,
		GroupByDimensions: true,
	})
	if err != nil || len(filteredCC) != 1 {
		t.Fatalf("CostCenterID filter = (%d, %v), want 1", len(filteredCC), err)
	}
	for _, b := range filteredCC {
		if !b.TransactionNet.Equal(decimal.RequireFromString("125.00")) {
			t.Fatalf("expected ccA net 125.00 USD, got %s", b.TransactionNet)
		}
	}

	// Cover CompanyID mismatch, TransactionCurrency mismatch, nil-dimension filter,
	// zero PostingDate fallback to EffectiveDate, error propagation, and multi-dimension sort keys.
	otherCompany := types.NewCompanyID()
	emptyOtherCo, err := RebuildForeignCurrencyBalances(entries, ForeignCurrencyBalanceFilter{CompanyID: otherCompany})
	if err != nil || len(emptyOtherCo) != 0 {
		t.Fatalf("other company filter = (%d, %v), want 0", len(emptyOtherCo), err)
	}
	emptyOtherCur, err := RebuildForeignCurrencyBalances(entries, ForeignCurrencyBalanceFilter{CompanyID: company, TransactionCurrency: "JPY"})
	if err != nil || len(emptyOtherCur) != 0 {
		t.Fatalf("JPY filter = (%d, %v), want 0", len(emptyOtherCur), err)
	}
	missingDimFilter, err := RebuildForeignCurrencyBalances(entries, ForeignCurrencyBalanceFilter{CompanyID: company, PartnerID: &[]types.PartnerID{types.NewPartnerID()}[0]})
	if err != nil || len(missingDimFilter) != 0 {
		t.Fatalf("nil actual partner vs non-nil filter = (%d, %v), want 0", len(missingDimFilter), err)
	}

	noPostDate := e1.Clone()
	noPostDate.ID = types.NewJournalEntryID()
	noPostDate.PostingDate = time.Time{}
	noPostDate.EffectiveDate = d1
	noPostDate.Lines[0].ID = types.NewJournalEntryLineID()
	noPostDate.Lines[1].ID = types.NewJournalEntryLineID()
	if got, err := RebuildForeignCurrencyBalances([]journal.JournalEntry{noPostDate}, ForeignCurrencyBalanceFilter{
		CompanyID: company,
		AsOf:      d1,
	}); err != nil || len(got) != 2 {
		t.Fatalf("zero PostingDate AsOf fallback = (%d, %v), want 2", len(got), err)
	}

	badPosted := e1.Clone()
	badPosted.CompanyID = types.CompanyID{}
	if _, err := RebuildForeignCurrencyBalances([]journal.JournalEntry{badPosted}, ForeignCurrencyBalanceFilter{}); err == nil {
		t.Fatal("expected error from invalid posted entry")
	}

	// Exercise all tie-breakers inside SortedForeignCurrencyBalances.
	p1, p2 := types.NewPartnerID(), types.NewPartnerID()
	proj1, proj2 := types.NewProjectID(), types.NewProjectID()
	seg1, seg2 := types.NewSegmentID(), types.NewSegmentID()
	bu1, bu2 := types.NewBusinessUnitID(), types.NewBusinessUnitID()
	sortKeys := []ForeignCurrencyBalanceKey{
		{CompanyID: company, AccountID: fxBank, TransactionCurrency: "USD", FunctionalCurrency: "SAR", PartnerID: p1},
		{CompanyID: company, AccountID: fxBank, TransactionCurrency: "USD", FunctionalCurrency: "SAR", PartnerID: p2},
		{CompanyID: company, AccountID: fxBank, TransactionCurrency: "USD", FunctionalCurrency: "SAR", PartnerID: p1, CostCenterID: ccA},
		{CompanyID: company, AccountID: fxBank, TransactionCurrency: "USD", FunctionalCurrency: "SAR", PartnerID: p1, CostCenterID: ccA, ProjectID: proj1},
		{CompanyID: company, AccountID: fxBank, TransactionCurrency: "USD", FunctionalCurrency: "SAR", PartnerID: p1, CostCenterID: ccA, ProjectID: proj2},
		{CompanyID: company, AccountID: fxBank, TransactionCurrency: "USD", FunctionalCurrency: "SAR", PartnerID: p1, CostCenterID: ccA, ProjectID: proj1, SegmentID: seg1},
		{CompanyID: company, AccountID: fxBank, TransactionCurrency: "USD", FunctionalCurrency: "SAR", PartnerID: p1, CostCenterID: ccA, ProjectID: proj1, SegmentID: seg2},
		{CompanyID: company, AccountID: fxBank, TransactionCurrency: "USD", FunctionalCurrency: "SAR", PartnerID: p1, CostCenterID: ccA, ProjectID: proj1, SegmentID: seg1, BusinessUnitID: bu1},
		{CompanyID: company, AccountID: fxBank, TransactionCurrency: "USD", FunctionalCurrency: "SAR", PartnerID: p1, CostCenterID: ccA, ProjectID: proj1, SegmentID: seg1, BusinessUnitID: bu2},
		{CompanyID: company, AccountID: fxBank, TransactionCurrency: "EUR", FunctionalCurrency: "USD"},
		{CompanyID: company, AccountID: fxBank, TransactionCurrency: "EUR", FunctionalCurrency: "SAR"},
		{CompanyID: otherCompany, AccountID: apAccount, TransactionCurrency: "USD", FunctionalCurrency: "SAR"},
	}
	sortMap := make(map[ForeignCurrencyBalanceKey]ForeignCurrencyBalance, len(sortKeys))
	for _, k := range sortKeys {
		sortMap[k] = ForeignCurrencyBalance{CompanyID: k.CompanyID, AccountID: k.AccountID, TransactionCurrency: k.TransactionCurrency, FunctionalCurrency: k.FunctionalCurrency}
	}
	if sortedAll := SortedForeignCurrencyBalances(sortMap); len(sortedAll) != len(sortKeys) {
		t.Fatalf("SortedForeignCurrencyBalances len = %d, want %d", len(sortedAll), len(sortKeys))
	}
}

func mustMoney(amount, currency string) types.Money {
	m, err := types.NewMoney(decimal.RequireFromString(amount), currency)
	if err != nil {
		panic(err)
	}
	return m
}
