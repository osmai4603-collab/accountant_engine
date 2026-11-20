package posting

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/osmai4603-collab/accounting_engine/internal/kernal/journal"
	"github.com/osmai4603-collab/accounting_engine/internal/kernal/masterdata"
	"github.com/osmai4603-collab/accounting_engine/internal/kernal/policy"
	"github.com/osmai4603-collab/accounting_engine/internal/kernal/types"
	"github.com/shopspring/decimal"
)

type exactCurrencyReader struct {
	masterdata.Reader
	currency string
}

func (r exactCurrencyReader) GetCompany(_ context.Context, companyID types.CompanyID) (masterdata.Company, error) {
	return masterdata.Company{ID: companyID, Active: true, FunctionalCurrency: r.currency}, nil
}

type adjustmentPeriodReader struct {
	masterdata.Reader
	year        masterdata.FiscalYear
	regular     masterdata.FiscalPeriod
	adjustments map[int]masterdata.FiscalPeriod
}

func (r adjustmentPeriodReader) GetPeriodForDate(context.Context, types.CompanyID, time.Time) (masterdata.FiscalPeriod, error) {
	return r.regular, nil
}

func (r adjustmentPeriodReader) GetFiscalYear(context.Context, types.CompanyID, types.FiscalYearID) (masterdata.FiscalYear, error) {
	return r.year, nil
}

func (r adjustmentPeriodReader) GetPeriodByNumber(
	_ context.Context,
	_ types.CompanyID,
	_ types.FiscalYearID,
	number int,
) (masterdata.FiscalPeriod, error) {
	period, ok := r.adjustments[number]
	if !ok {
		return masterdata.FiscalPeriod{}, types.ErrFiscalPeriodNotFound
	}
	return period, nil
}

func (r adjustmentPeriodReader) GetJournalByID(
	ctx context.Context,
	companyID types.CompanyID,
	journalID types.JournalID,
) (masterdata.Journal, error) {
	return getTestJournal(ctx, r.Reader, companyID, journalID)
}

type dimensionPostingReader struct {
	masterdata.Reader
	costCenter masterdata.CostCenter
	dimension  masterdata.AnalyticalDimensionValue
}

type costCenterGraphReader struct {
	masterdata.Reader
	costCenters map[types.CostCenterID]masterdata.CostCenter
}

func (r dimensionPostingReader) GetJournalByID(
	ctx context.Context,
	companyID types.CompanyID,
	journalID types.JournalID,
) (masterdata.Journal, error) {
	return getTestJournal(ctx, r.Reader, companyID, journalID)
}

func (r costCenterGraphReader) GetJournalByID(
	ctx context.Context,
	companyID types.CompanyID,
	journalID types.JournalID,
) (masterdata.Journal, error) {
	return getTestJournal(ctx, r.Reader, companyID, journalID)
}

func getTestJournal(
	ctx context.Context,
	reader masterdata.Reader,
	companyID types.CompanyID,
	journalID types.JournalID,
) (masterdata.Journal, error) {
	repository, ok := reader.(masterdata.JournalRepository)
	if !ok {
		return masterdata.Journal{}, masterdata.ErrJournalReaderRequired
	}
	return repository.GetJournalByID(ctx, companyID, journalID)
}

func (r costCenterGraphReader) GetCostCenterForCompany(
	_ context.Context,
	companyID types.CompanyID,
	id types.CostCenterID,
) (masterdata.CostCenter, error) {
	center, ok := r.costCenters[id]
	if !ok || center.CompanyID != companyID {
		return masterdata.CostCenter{}, masterdata.ErrInvalidCostCenter
	}
	return center, nil
}

func testPostingPolicyEngine(
	t *testing.T,
	companyID types.CompanyID,
	functionalCurrency string,
	roundingAccountID ...types.AccountID,
) *policy.PolicyEngine {
	t.Helper()
	engine := policy.NewPolicyEngine()
	effectiveDate := time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC)
	if err := engine.SetPolicy(policy.PostingPolicy{
		CompanyID: companyID, EffectiveDate: effectiveDate,
	}); err != nil {
		t.Fatal(err)
	}
	currencyPolicy := policy.CurrencyPolicy{
		CompanyID: companyID, FunctionalCurrency: functionalCurrency,
		AllowedCurrencies: []string{"USD", "EUR"}, EffectiveDate: effectiveDate,
	}
	if len(roundingAccountID) > 0 {
		currencyPolicy.AutoSettleRoundingDifferences = true
		currencyPolicy.RoundingDifferenceAccountID = roundingAccountID[0]
	}
	if err := engine.SetCurrencyPolicy(currencyPolicy); err != nil {
		t.Fatal(err)
	}
	return engine
}

func TestPostEntryNormalizesBothCurrenciesAndSettlesRounding(t *testing.T) {
	companyID, journalID := types.NewCompanyID(), types.NewJournalID()
	debitA, debitB, credit := makeMoney(t, "10.005", "USD"), makeMoney(t, "10.005", "USD"), makeMoney(t, "20.010", "USD")
	zero := makeMoney(t, "0", "USD")
	roundingID := types.NewAccountID()
	entry := journal.JournalEntry{
		ID: types.NewJournalEntryID(), CompanyID: companyID, JournalID: journalID,
		EffectiveDate: time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC), Status: journal.EntryDraft,
		CreatedBy: "creator",
		Lines: []journal.EntryLine{
			{ID: types.NewJournalEntryLineID(), AccountID: types.NewAccountID(), TransactionAmount: debitA, TransactionCurrency: "USD",
				ExchangeRate: decimal.NewFromInt(1), FunctionalDebit: debitA, FunctionalCredit: zero},
			{ID: types.NewJournalEntryLineID(), AccountID: types.NewAccountID(), TransactionAmount: debitB, TransactionCurrency: "USD",
				ExchangeRate: decimal.NewFromInt(1), FunctionalDebit: debitB, FunctionalCredit: zero},
			{ID: types.NewJournalEntryLineID(), AccountID: types.NewAccountID(), TransactionAmount: credit, TransactionCurrency: "USD",
				ExchangeRate: decimal.NewFromInt(1), FunctionalDebit: zero, FunctionalCredit: credit},
		},
	}
	reader := &mockMasterReaderForPosting{compActive: true, periodOpen: true}
	executed := false
	pipeline := NewPostingPipeline(reader, func(_ context.Context, posted journal.JournalEntry, _ PostOptions) (journal.JournalEntry, error) {
		executed = true
		if len(posted.Lines) != 4 {
			t.Fatalf("posted line count = %d, want 4 including rounding settlement", len(posted.Lines))
		}
		for i, line := range posted.Lines {
			if _, err := line.TransactionAmount.MinorUnitsExact(); err != nil {
				t.Errorf("line %d transaction amount was not normalized: %v", i, err)
			}
			if _, err := line.FunctionalDebit.MinorUnitsExact(); err != nil {
				t.Errorf("line %d functional debit was not normalized: %v", i, err)
			}
			if _, err := line.FunctionalCredit.MinorUnitsExact(); err != nil {
				t.Errorf("line %d functional credit was not normalized: %v", i, err)
			}
		}
		settlement := posted.Lines[3]
		if settlement.AccountID != roundingID || settlement.FunctionalDebit.Amount().StringFixed(2) != "0.01" {
			t.Errorf("rounding settlement = %+v, want 0.01 debit to configured account", settlement)
		}
		return posted, nil
	}, WithPolicyEngine(testPostingPolicyEngine(t, companyID, "USD", roundingID)))

	if _, err := pipeline.PostEntry(context.Background(), entry, PostOptions{ActorID: "poster"}); err != nil {
		t.Fatalf("PostEntry rejected a normalizable entry: %v", err)
	}
	if !executed {
		t.Fatal("posting executor was not called after normalization")
	}
	if entry.Lines[0].TransactionAmount.Amount().String() != "10.005" ||
		entry.Lines[0].FunctionalDebit.Amount().String() != "10.005" {
		t.Fatal("PostEntry mutated the caller's draft while normalizing its copy")
	}
}

func TestPostEntryNormalizesTransactionAndFunctionalCurrenciesIndependently(t *testing.T) {
	companyID, journalID := types.NewCompanyID(), types.NewJournalID()
	transactionAmount := makeMoney(t, "10.005", "EUR")
	functionalAmount := makeMoney(t, "12.006", "USD")
	zero := makeMoney(t, "0", "USD")
	entry := journal.JournalEntry{
		ID: types.NewJournalEntryID(), CompanyID: companyID, JournalID: journalID,
		EffectiveDate: time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC), Status: journal.EntryDraft,
		Lines: []journal.EntryLine{
			{ID: types.NewJournalEntryLineID(), AccountID: types.NewAccountID(), TransactionAmount: transactionAmount,
				TransactionCurrency: "EUR", ExchangeRate: decimal.RequireFromString("1.2"),
				FunctionalDebit: functionalAmount, FunctionalCredit: zero},
			{ID: types.NewJournalEntryLineID(), AccountID: types.NewAccountID(), TransactionAmount: transactionAmount,
				TransactionCurrency: "EUR", ExchangeRate: decimal.RequireFromString("1.2"),
				FunctionalDebit: zero, FunctionalCredit: functionalAmount},
		},
	}
	pipeline := NewPostingPipeline(
		&mockMasterReaderForPosting{compActive: true, periodOpen: true},
		func(_ context.Context, posted journal.JournalEntry, _ PostOptions) (journal.JournalEntry, error) {
			for i, line := range posted.Lines {
				if line.TransactionCurrency != "EUR" || line.TransactionAmount.Amount().StringFixed(2) != "10.00" {
					t.Errorf("line %d transaction amount = %s, want 10.00 EUR", i, line.TransactionAmount)
				}
				functional := line.FunctionalDebit
				if functional.Amount().IsZero() {
					functional = line.FunctionalCredit
				}
				if functional.Currency() != "USD" || functional.Amount().StringFixed(2) != "12.00" {
					t.Errorf("line %d functional amount = %s, want 12.00 USD", i, functional)
				}
			}
			return posted, nil
		},
		WithPolicyEngine(testPostingPolicyEngine(t, companyID, "USD")),
	)
	if _, err := pipeline.PostEntry(context.Background(), entry, PostOptions{ActorID: "poster"}); err != nil {
		t.Fatalf("PostEntry rejected independently normalizable currencies: %v", err)
	}
}

func TestPostEntryRejectsRoundingWithoutPolicySettlement(t *testing.T) {
	entry := newPostingTestEntry(t, "creator")
	debitA, debitB, credit := makeMoney(t, "10.005", "USD"), makeMoney(t, "10.005", "USD"), makeMoney(t, "20.010", "USD")
	zero := makeMoney(t, "0", "USD")
	entry.Lines = []journal.EntryLine{
		{ID: types.NewJournalEntryLineID(), AccountID: types.NewAccountID(), TransactionAmount: debitA,
			TransactionCurrency: "USD", ExchangeRate: decimal.NewFromInt(1), FunctionalDebit: debitA, FunctionalCredit: zero},
		{ID: types.NewJournalEntryLineID(), AccountID: types.NewAccountID(), TransactionAmount: debitB,
			TransactionCurrency: "USD", ExchangeRate: decimal.NewFromInt(1), FunctionalDebit: debitB, FunctionalCredit: zero},
		{ID: types.NewJournalEntryLineID(), AccountID: types.NewAccountID(), TransactionAmount: credit,
			TransactionCurrency: "USD", ExchangeRate: decimal.NewFromInt(1), FunctionalDebit: zero, FunctionalCredit: credit},
	}
	executed := false
	pipeline := NewPostingPipeline(
		&mockMasterReaderForPosting{compActive: true, periodOpen: true},
		func(context.Context, journal.JournalEntry, PostOptions) (journal.JournalEntry, error) {
			executed = true
			return journal.JournalEntry{}, nil
		},
		WithPolicyEngine(testPostingPolicyEngine(t, entry.CompanyID, "USD")),
	)
	if _, err := pipeline.PostEntry(context.Background(), entry, PostOptions{ActorID: "poster"}); !errors.Is(err, ErrRoundingPolicyRequired) {
		t.Fatalf("PostEntry error = %v, want rounding policy required", err)
	}
	if executed {
		t.Fatal("executor ran despite missing rounding settlement policy")
	}
}

func TestPostEntryFailsClosedWithoutRegisteredPolicies(t *testing.T) {
	entry := newPostingTestEntry(t, "creator")
	executed := false
	pipeline := NewPostingPipeline(
		&mockMasterReaderForPosting{compActive: true, periodOpen: true},
		func(context.Context, journal.JournalEntry, PostOptions) (journal.JournalEntry, error) {
			executed = true
			return journal.JournalEntry{}, nil
		},
	)
	if _, err := pipeline.PostEntry(context.Background(), entry, PostOptions{ActorID: "poster"}); !errors.Is(err, ErrPolicyEngineRequired) {
		t.Fatalf("PostEntry error = %v, want policy engine required", err)
	}
	if executed {
		t.Fatal("executor ran without registered policies")
	}
}

func TestSoftClosedPostingUsesPolicyOnly(t *testing.T) {
	date := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	entry := newPostingTestEntry(t, "creator")
	entry.EffectiveDate = date
	engine := policy.NewPolicyEngine()
	effectiveDate := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := engine.SetPolicy(policy.PostingPolicy{
		CompanyID: entry.CompanyID, EffectiveDate: effectiveDate,
		AllowSoftClosedPosting: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := engine.SetCurrencyPolicy(policy.CurrencyPolicy{
		CompanyID: entry.CompanyID, FunctionalCurrency: "USD",
		AllowedCurrencies: []string{"USD"}, EffectiveDate: effectiveDate,
	}); err != nil {
		t.Fatal(err)
	}
	reader := &mockMasterReaderForPosting{
		compActive: true, periodState: masterdata.PeriodSoftClosed,
	}
	executed := false
	pipeline := NewPostingPipeline(reader, func(_ context.Context, posted journal.JournalEntry, _ PostOptions) (journal.JournalEntry, error) {
		executed = true
		return posted, nil
	}, WithPolicyEngine(engine))
	if _, err := pipeline.PostEntry(context.Background(), entry, PostOptions{ActorID: "poster"}); err != nil {
		t.Fatalf("soft-closed posting with explicit company policy: %v", err)
	}
	if !executed {
		t.Fatal("executor was not called after policy validation")
	}
}

func (r dimensionPostingReader) GetCostCenterForCompany(
	context.Context,
	types.CompanyID,
	types.CostCenterID,
) (masterdata.CostCenter, error) {
	return r.costCenter, nil
}

func (r dimensionPostingReader) GetAnalyticalDimensionForCompany(
	context.Context,
	types.CompanyID,
	masterdata.AnalyticalDimensionKind,
	string,
) (masterdata.AnalyticalDimensionValue, error) {
	return r.dimension, nil
}

func newPostingTestEntry(t *testing.T, createdBy string) journal.JournalEntry {
	t.Helper()
	amount := makeMoney(t, "100.00", "USD")
	zero := makeMoney(t, "0", "USD")
	entry, err := NewJournalEntry(
		types.NewCompanyID(),
		types.NewJournalID(),
		time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC),
		createdBy,
		[]journal.EntryLine{
			{
				AccountID: types.NewAccountID(), TransactionAmount: amount, TransactionCurrency: "USD",
				ExchangeRate: decimal.NewFromInt(1), FunctionalDebit: amount, FunctionalCredit: zero,
			},
			{
				AccountID: types.NewAccountID(), TransactionAmount: amount, TransactionCurrency: "USD",
				ExchangeRate: decimal.NewFromInt(1), FunctionalDebit: zero, FunctionalCredit: amount,
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return entry
}

func TestPostEntryRejectsIncompleteClassificationMetadataBeforeRepositoryAccess(t *testing.T) {
	entry := newPostingTestEntry(t, "creator")
	entry.Classification = journal.EntryClassAdjusting
	pipeline := NewPostingPipeline(nil, nil)
	if _, err := pipeline.PostEntry(context.Background(), entry, PostOptions{ActorID: "poster"}); !errors.Is(err, ErrAdjustingEntryMetadataRequired) {
		t.Fatalf("incomplete adjusting entry error = %v, want required metadata", err)
	}

	entry.Classification = journal.EntryClassRegular
	entry.ReversalOfID = new(types.JournalEntryID)
	if _, err := pipeline.PostEntry(context.Background(), entry, PostOptions{ActorID: "poster"}); !errors.Is(err, ErrInvalidReversal) {
		t.Fatalf("regular entry with reversal reference error = %v, want invalid reversal", err)
	}
}

func TestPostingRejectsMissingCompanyFunctionalCurrency(t *testing.T) {
	baseReader := &mockMasterReaderForPosting{compActive: true, periodOpen: true}
	reader := exactCurrencyReader{Reader: baseReader}
	entry := newPostingTestEntry(t, "creator")
	executed := false
	pipeline := NewPostingPipeline(reader, func(context.Context, journal.JournalEntry, PostOptions) (journal.JournalEntry, error) {
		executed = true
		return journal.JournalEntry{}, nil
	}, WithPolicyEngine(testPostingPolicyEngine(t, entry.CompanyID, "USD")))

	_, err := pipeline.PostEntry(context.Background(), entry, PostOptions{ActorID: "poster"})
	if !errors.Is(err, ErrMissingFunctionalCurrency) {
		t.Fatalf("posting with empty company currency error = %v, want missing currency", err)
	}
	if executed {
		t.Fatal("executor ran despite missing company functional currency")
	}
}

func TestPostingEnforcesJournalCompanyAndAllowLists(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		journal func(journal.JournalEntry) masterdata.Journal
		wantErr error
	}{
		{
			name: "company mismatch",
			journal: func(entry journal.JournalEntry) masterdata.Journal {
				return masterdata.Journal{
					ID: entry.JournalID, CompanyID: types.NewCompanyID(), Code: "GEN", Name: "General",
				}
			},
			wantErr: masterdata.ErrInvalidJournal,
		},
		{
			name: "journal expired",
			journal: func(entry journal.JournalEntry) masterdata.Journal {
				expired := entry.EffectiveDate.Add(-time.Nanosecond)
				return masterdata.Journal{
					ID: entry.JournalID, CompanyID: entry.CompanyID, Code: "GEN", Name: "General",
					EffectiveTo: &expired,
				}
			},
			wantErr: masterdata.ErrJournalNotCurrent,
		},
		{
			name: "currency not allowed",
			journal: func(entry journal.JournalEntry) masterdata.Journal {
				return masterdata.Journal{
					ID: entry.JournalID, CompanyID: entry.CompanyID, Code: "GEN", Name: "General",
					AllowedCurrencies: []string{"EUR"},
				}
			},
			wantErr: masterdata.ErrJournalCurrency,
		},
		{
			name: "account not allowed",
			journal: func(entry journal.JournalEntry) masterdata.Journal {
				return masterdata.Journal{
					ID: entry.JournalID, CompanyID: entry.CompanyID, Code: "GEN", Name: "General",
					AllowedCurrencies: []string{"USD"},
					AllowedAccountIDs: []types.AccountID{entry.Lines[0].AccountID},
				}
			},
			wantErr: masterdata.ErrJournalAccount,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			entry := newPostingTestEntry(t, "creator")
			reader := &mockMasterReaderForPosting{
				compActive: true, periodOpen: true, journal: testCase.journal(entry),
			}
			executed := false
			pipeline := NewPostingPipeline(
				reader,
				func(_ context.Context, posted journal.JournalEntry, _ PostOptions) (journal.JournalEntry, error) {
					executed = true
					return posted, nil
				},
				WithPolicyEngine(testPostingPolicyEngine(t, entry.CompanyID, "USD")),
			)
			if _, err := pipeline.PostEntry(context.Background(), entry, PostOptions{ActorID: "poster"}); !errors.Is(err, testCase.wantErr) {
				t.Fatalf("PostEntry error = %v, want %v", err, testCase.wantErr)
			}
			if executed {
				t.Fatal("executor ran despite journal policy failure")
			}
		})
	}
}

func TestPostingAcceptsEntryWithinJournalAllowLists(t *testing.T) {
	entry := newPostingTestEntry(t, "creator")
	allowedAccounts := []types.AccountID{entry.Lines[0].AccountID, entry.Lines[1].AccountID}
	reader := &mockMasterReaderForPosting{
		compActive: true, periodOpen: true,
		journal: masterdata.Journal{
			ID: entry.JournalID, CompanyID: entry.CompanyID, Code: "GEN", Name: "General",
			AllowedCurrencies: []string{"USD"}, AllowedAccountIDs: allowedAccounts,
		},
	}
	executed := false
	pipeline := NewPostingPipeline(
		reader,
		func(_ context.Context, posted journal.JournalEntry, _ PostOptions) (journal.JournalEntry, error) {
			executed = true
			return posted, nil
		},
		WithPolicyEngine(testPostingPolicyEngine(t, entry.CompanyID, "USD")),
	)
	if _, err := pipeline.PostEntry(context.Background(), entry, PostOptions{ActorID: "poster"}); err != nil {
		t.Fatalf("posting within journal allow-lists: %v", err)
	}
	if !executed {
		t.Fatal("executor was not called for a journal-compliant entry")
	}
}

func TestPostingRequiresJournalRepository(t *testing.T) {
	entry := newPostingTestEntry(t, "creator")
	baseReader := &mockMasterReaderForPosting{compActive: true, periodOpen: true}
	reader := exactCurrencyReader{Reader: baseReader, currency: "USD"}
	pipeline := NewPostingPipeline(
		reader,
		func(context.Context, journal.JournalEntry, PostOptions) (journal.JournalEntry, error) {
			t.Fatal("executor ran without journal validation")
			return journal.JournalEntry{}, nil
		},
		WithPolicyEngine(testPostingPolicyEngine(t, entry.CompanyID, "USD")),
	)
	if _, err := pipeline.PostEntry(context.Background(), entry, PostOptions{ActorID: "poster"}); !errors.Is(err, masterdata.ErrJournalReaderRequired) {
		t.Fatalf("posting without journal repository error = %v, want journal repository required", err)
	}

}

func TestPostingRejectsMissingEntryIdentityBeforeValidation(t *testing.T) {
	entry := newPostingTestEntry(t, "creator")
	entry.ID = types.JournalEntryID{}
	executed := false
	pipeline := NewPostingPipeline(
		&mockMasterReaderForPosting{compActive: true, periodOpen: true},
		func(context.Context, journal.JournalEntry, PostOptions) (journal.JournalEntry, error) {
			executed = true
			return journal.JournalEntry{}, nil
		},
		WithPolicyEngine(testPostingPolicyEngine(t, entry.CompanyID, "USD")),
	)
	if _, err := pipeline.PostEntry(context.Background(), entry, PostOptions{ActorID: "poster"}); !errors.Is(err, ErrMissingEntryID) {
		t.Fatalf("posting with zero entry id error = %v, want missing entry id", err)
	}
	if executed {
		t.Fatal("executor ran for an entry without an ID")
	}
}
func TestPostingResolvesAdjustmentPeriodByAccountingMetadata(t *testing.T) {
	ctx := context.Background()
	baseReader := &mockMasterReaderForPosting{compActive: true, periodOpen: true}
	companyID := types.NewCompanyID()
	fiscalYearID := types.NewFiscalYearID()
	period13ID, period14ID := types.NewFiscalPeriodID(), types.NewFiscalPeriodID()
	reader := adjustmentPeriodReader{
		Reader: baseReader,
		year: masterdata.FiscalYear{
			ID: fiscalYearID, CompanyID: companyID, State: masterdata.FiscalYearOpen,
			StartDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			EndDate:   time.Date(2026, 12, 31, 23, 59, 59, 999999999, time.UTC),
		},
		regular: masterdata.FiscalPeriod{
			ID: types.NewFiscalPeriodID(), CompanyID: companyID, FiscalYearID: fiscalYearID,
			Number: 12, State: masterdata.PeriodOpen, Type: masterdata.FiscalPeriodTypeNormal,
			StartDate: time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC),
			EndDate:   time.Date(2026, 12, 31, 23, 59, 59, 999999999, time.UTC),
		},
		adjustments: map[int]masterdata.FiscalPeriod{
			13: {ID: period13ID, CompanyID: companyID, FiscalYearID: fiscalYearID,
				Number: 13, State: masterdata.PeriodOpen, Type: masterdata.FiscalPeriodTypeYearEndAdjustment,
				StartDate: time.Date(2026, 12, 31, 23, 59, 59, 999999999, time.UTC),
				EndDate:   time.Date(2026, 12, 31, 23, 59, 59, 999999999, time.UTC)},
			14: {ID: period14ID, CompanyID: companyID, FiscalYearID: fiscalYearID,
				Number: 14, State: masterdata.PeriodOpen, Type: masterdata.FiscalPeriodTypeAuditAdjustment,
				StartDate: time.Date(2026, 12, 31, 23, 59, 59, 999999999, time.UTC),
				EndDate:   time.Date(2026, 12, 31, 23, 59, 59, 999999999, time.UTC)},
		},
	}
	var persistedPeriod types.FiscalPeriodID
	pipeline := NewPostingPipeline(reader, func(_ context.Context, entry journal.JournalEntry, _ PostOptions) (journal.JournalEntry, error) {
		persistedPeriod = entry.FiscalPeriodID
		return entry, nil
	}, WithPolicyEngine(testPostingPolicyEngine(t, companyID, "USD")))

	for _, testCase := range []struct {
		number int
		class  journal.EntryClassification
		want   types.FiscalPeriodID
	}{
		{number: 13, class: journal.EntryClassClosing, want: period13ID},
		{number: 14, class: journal.EntryClassAdjusting, want: period14ID},
	} {
		entry := newPostingTestEntry(t, "creator")
		entry.CompanyID = companyID
		entry.EffectiveDate = time.Date(2026, 12, 30, 0, 0, 0, 0, time.UTC)
		entry.TargetPeriodNumber = testCase.number
		entry.Classification = testCase.class
		if testCase.class == journal.EntryClassAdjusting {
			entry.AdjustingEntryType = journal.AdjustingEntryAccrual
			entry.AdjustmentReason = "Year-end adjustment"
		}
		if _, err := pipeline.PostEntry(ctx, entry, PostOptions{ActorID: "poster"}); err != nil {
			t.Fatalf("post to period %d: %v", testCase.number, err)
		}
		if persistedPeriod != testCase.want {
			t.Errorf("period %d resolved to %s, want %s", testCase.number, persistedPeriod, testCase.want)
		}
	}

	entry := newPostingTestEntry(t, "creator")
	entry.CompanyID = companyID
	entry.TargetPeriodNumber = 13
	entry.Classification = journal.EntryClassRegular
	if _, err := pipeline.PostEntry(ctx, entry, PostOptions{ActorID: "poster"}); !errors.Is(err, masterdata.ErrInvalidAdjustmentPeriod) {
		t.Fatalf("regular entry target period 13 error = %v, want invalid adjustment period", err)
	}
}

func TestReversalDraftResolvesDestinationPeriodDuringPosting(t *testing.T) {
	entry := newPostingTestEntry(t, "creator")
	entry.Status = journal.EntryPosted
	originalPeriodID := types.NewFiscalPeriodID()
	entry.FiscalPeriodID = originalPeriodID
	reversalDate := time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC)
	reversal, err := BuildReversalEntry(entry, reversalDate, "poster", "Correction")
	if err != nil {
		t.Fatal(err)
	}
	if reversal.Status != journal.EntryDraft || reversal.FiscalPeriodID != (types.FiscalPeriodID{}) {
		t.Fatalf("reversal builder must return unassigned draft: %+v", reversal)
	}

	companyID, yearID := entry.CompanyID, types.NewFiscalYearID()
	reversalPeriodID := types.NewFiscalPeriodID()
	reader := adjustmentPeriodReader{
		Reader: &mockMasterReaderForPosting{compActive: true, periodOpen: true},
		year: masterdata.FiscalYear{
			ID: yearID, CompanyID: companyID, State: masterdata.FiscalYearOpen,
			StartDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			EndDate:   time.Date(2026, 12, 31, 23, 59, 59, 999999999, time.UTC),
		},
		regular: masterdata.FiscalPeriod{
			ID: reversalPeriodID, CompanyID: companyID, FiscalYearID: yearID,
			Number: 9, State: masterdata.PeriodOpen, Type: masterdata.FiscalPeriodTypeNormal,
			StartDate: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
			EndDate:   time.Date(2026, 9, 30, 23, 59, 59, 999999999, time.UTC),
		},
		adjustments: map[int]masterdata.FiscalPeriod{},
	}
	var persisted journal.JournalEntry
	missingReversalExecutor := NewPostingPipeline(
		reader,
		func(context.Context, journal.JournalEntry, PostOptions) (journal.JournalEntry, error) {
			t.Fatal("ordinary posting executor received a reversal")
			return journal.JournalEntry{}, nil
		},
		WithPolicyEngine(testPostingPolicyEngine(t, companyID, "USD")),
	)
	if _, err := missingReversalExecutor.PostEntry(context.Background(), reversal, PostOptions{ActorID: "poster"}); !errors.Is(err, ErrReversalExecutorRequired) {
		t.Fatalf("reversal without atomic executor error = %v, want reversal executor required", err)
	}
	pipeline := NewPostingPipeline(
		reader,
		func(context.Context, journal.JournalEntry, PostOptions) (journal.JournalEntry, error) {
			t.Fatal("ordinary posting executor received a reversal")
			return journal.JournalEntry{}, nil
		},
		WithPolicyEngine(testPostingPolicyEngine(t, companyID, "USD")),
		WithReversalExecutor(func(
			_ context.Context,
			originalID types.JournalEntryID,
			posted journal.JournalEntry,
			_ PostOptions,
		) (journal.JournalEntry, error) {
			if originalID != entry.ID {
				t.Fatalf("reversal original ID = %s, want %s", originalID, entry.ID)
			}
			persisted = posted
			return posted, nil
		}),
	)
	if _, err := pipeline.PostEntry(context.Background(), reversal, PostOptions{ActorID: "poster"}); err != nil {
		t.Fatalf("post reversal draft: %v", err)
	}
	if persisted.FiscalPeriodID != reversalPeriodID || persisted.FiscalPeriodID == originalPeriodID ||
		persisted.Status != journal.EntryPosted || persisted.ReversalOfID == nil ||
		*persisted.ReversalOfID != entry.ID {
		t.Fatalf("posted reversal did not use destination period or preserve source link: %+v", persisted)
	}
}

func TestPostingRejectsInactiveCostCentersAndDimensions(t *testing.T) {
	ctx := context.Background()
	baseReader := &mockMasterReaderForPosting{compActive: true, periodOpen: true}
	companyID, centerID := types.NewCompanyID(), types.NewCostCenterID()
	reader := dimensionPostingReader{
		Reader: baseReader,
		costCenter: masterdata.CostCenter{
			ID: centerID, CompanyID: companyID, Code: "CC1", Name: "Inactive", Active: false,
		},
	}
	entry := newPostingTestEntry(t, "creator")
	entry.CompanyID = companyID
	pipeline := NewPostingPipeline(reader, func(context.Context, journal.JournalEntry, PostOptions) (journal.JournalEntry, error) {
		t.Fatal("executor ran with inactive cost center")
		return journal.JournalEntry{}, nil
	}, WithPolicyEngine(testPostingPolicyEngine(t, companyID, "USD")))
	entry.Lines[0].CostCenterID = &centerID
	if _, err := pipeline.PostEntry(ctx, entry, PostOptions{ActorID: "poster"}); !errors.Is(err, ErrInactiveFinancialDimension) {
		t.Fatalf("inactive cost center error = %v, want inactive-dimension rejection", err)
	}

	projectID := types.NewProjectID()
	reader.costCenter.Active = true
	reader.dimension = masterdata.AnalyticalDimensionValue{
		ID: projectID, CompanyID: companyID, Kind: masterdata.DimensionProject,
		Code: "P1", Name: "Inactive", Active: false,
	}
	pipeline = NewPostingPipeline(reader, func(context.Context, journal.JournalEntry, PostOptions) (journal.JournalEntry, error) {
		t.Fatal("executor ran with inactive analytical dimension")
		return journal.JournalEntry{}, nil
	}, WithPolicyEngine(testPostingPolicyEngine(t, companyID, "USD")))
	entry = newPostingTestEntry(t, "creator")
	entry.CompanyID = companyID
	entry.Lines[0].ProjectID = &projectID
	if _, err := pipeline.PostEntry(ctx, entry, PostOptions{ActorID: "poster"}); !errors.Is(err, ErrInactiveFinancialDimension) {
		t.Fatalf("inactive analytical dimension error = %v, want inactive-dimension rejection", err)
	}
}

func TestPostingRequiresDimensionReaderWhenDimensionIsPresent(t *testing.T) {
	reader := &mockMasterReaderForPosting{compActive: true, periodOpen: true}
	entry := newPostingTestEntry(t, "creator")
	pipeline := NewPostingPipeline(reader, func(context.Context, journal.JournalEntry, PostOptions) (journal.JournalEntry, error) {
		t.Fatal("executor ran without dimension validation")
		return journal.JournalEntry{}, nil
	}, WithPolicyEngine(testPostingPolicyEngine(t, entry.CompanyID, "USD")))
	projectID := types.NewProjectID()
	entry.Lines[0].ProjectID = &projectID
	if _, err := pipeline.PostEntry(context.Background(), entry, PostOptions{ActorID: "poster"}); !errors.Is(err, ErrDimensionReaderRequired) {
		t.Fatalf("missing dimension reader error = %v, want reader-required error", err)
	}

}

func TestPostingRejectsCyclicCostCenterHierarchy(t *testing.T) {
	companyID := types.NewCompanyID()
	centerA, centerB := types.NewCostCenterID(), types.NewCostCenterID()
	reader := costCenterGraphReader{
		Reader: &mockMasterReaderForPosting{compActive: true, periodOpen: true},
		costCenters: map[types.CostCenterID]masterdata.CostCenter{
			centerA: {ID: centerA, CompanyID: companyID, ParentID: &centerB, Code: "A", Name: "A", Active: true},
			centerB: {ID: centerB, CompanyID: companyID, ParentID: &centerA, Code: "B", Name: "B", Active: true},
		},
	}
	entry := newPostingTestEntry(t, "creator")
	entry.CompanyID = companyID
	entry.Lines[0].CostCenterID = &centerA
	pipeline := NewPostingPipeline(
		reader,
		func(context.Context, journal.JournalEntry, PostOptions) (journal.JournalEntry, error) {
			t.Fatal("executor ran with cyclic cost-center hierarchy")
			return journal.JournalEntry{}, nil
		},
		WithPolicyEngine(testPostingPolicyEngine(t, companyID, "USD")),
	)
	if _, err := pipeline.PostEntry(context.Background(), entry, PostOptions{ActorID: "poster"}); !errors.Is(err, masterdata.ErrCostCenterCycle) {
		t.Fatalf("cyclic cost-center error = %v, want cycle error", err)
	}
}
