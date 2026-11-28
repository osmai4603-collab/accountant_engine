package posting

import (
	"context"
	"errors"
	"sync"
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
		return masterdata.FiscalPeriod{}, masterdata.ErrFiscalPeriodNotFound
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
		Classification: journal.EntryClassRegular,
		EffectiveDate:  time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC), Status: journal.EntryDraft,
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
	pipeline := NewPostingPipeline(reader, func(_ context.Context, posted journal.JournalEntry, opts PostOptions) (journal.JournalEntry, error) {
		executed = true
		if opts.IdempotencyKey != "test-idempotency-key" {
			t.Errorf("executor idempotency key = %q, want normalized key", opts.IdempotencyKey)
		}
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

	if _, err := pipeline.PostEntry(context.Background(), entry, PostOptions{ActorID: "poster", IdempotencyKey: "  test-idempotency-key  "}); err != nil {
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

func TestPostEntryRejectsUninitializedPipelineDependencies(t *testing.T) {
	entry := journal.JournalEntry{}
	ctx := context.Background()

	if _, err := (*PostingPipeline)(nil).PostEntry(ctx, entry, PostOptions{}); !errors.Is(err, ErrPostingPipelineRequired) {
		t.Fatalf("nil pipeline error = %v, want ErrPostingPipelineRequired", err)
	}
	if _, err := NewPostingPipeline(nil, nil).PostEntry(ctx, entry, PostOptions{}); !errors.Is(err, ErrPostingReaderRequired) {
		t.Fatalf("nil reader error = %v, want ErrPostingReaderRequired", err)
	}
	if _, err := NewPostingPipeline(&mockMasterReaderForPosting{}, nil).PostEntry(ctx, entry, PostOptions{}); !errors.Is(err, ErrPolicyEngineRequired) {
		t.Fatalf("nil policy engine error = %v, want ErrPolicyEngineRequired", err)
	}
	clockless := &PostingPipeline{
		reader:       &mockMasterReaderForPosting{},
		policyEngine: policy.NewPolicyEngine(),
		execute: func(context.Context, journal.JournalEntry, PostOptions) (journal.JournalEntry, error) {
			return journal.JournalEntry{}, nil
		},
	}
	if _, err := clockless.PostEntry(ctx, entry, PostOptions{}); !errors.Is(err, ErrPostingClockRequired) {
		t.Fatalf("nil clock error = %v, want ErrPostingClockRequired", err)
	}

	pipeline := NewPostingPipeline(
		&mockMasterReaderForPosting{},
		func(context.Context, journal.JournalEntry, PostOptions) (journal.JournalEntry, error) {
			return journal.JournalEntry{}, nil
		},
		WithPolicyEngine(policy.NewPolicyEngine()),
	)
	if _, err := pipeline.PostEntry(nil, entry, PostOptions{}); !errors.Is(err, ErrPostingContextRequired) {
		t.Fatalf("nil context error = %v, want ErrPostingContextRequired", err)
	}
}

func TestPostEntryNormalizesTransactionAndFunctionalCurrenciesIndependently(t *testing.T) {
	companyID, journalID := types.NewCompanyID(), types.NewJournalID()
	transactionAmount := makeMoney(t, "10.005", "EUR")
	functionalAmount := makeMoney(t, "12.006", "USD")
	zero := makeMoney(t, "0", "USD")
	entry := journal.JournalEntry{
		ID: types.NewJournalEntryID(), CompanyID: companyID, JournalID: journalID,
		Classification: journal.EntryClassRegular,
		EffectiveDate:  time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC), Status: journal.EntryDraft,
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
	if _, err := pipeline.PostEntry(context.Background(), entry, PostOptions{ActorID: "poster", IdempotencyKey: "test-idempotency-key"}); err != nil {
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
	if _, err := pipeline.PostEntry(context.Background(), entry, PostOptions{ActorID: "poster", IdempotencyKey: "test-idempotency-key"}); !errors.Is(err, ErrRoundingPolicyRequired) {
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
	if _, err := pipeline.PostEntry(context.Background(), entry, PostOptions{ActorID: "poster", IdempotencyKey: "test-idempotency-key"}); !errors.Is(err, ErrPolicyEngineRequired) {
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
	if _, err := pipeline.PostEntry(context.Background(), entry, PostOptions{ActorID: "poster", IdempotencyKey: "test-idempotency-key"}); err != nil {
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
	entry.Classification = ""
	reader := &mockMasterReaderForPosting{compActive: true, periodOpen: true}
	pipeline := NewPostingPipeline(
		reader,
		func(context.Context, journal.JournalEntry, PostOptions) (journal.JournalEntry, error) {
			t.Fatal("executor must not be called for invalid classification metadata")
			return journal.JournalEntry{}, nil
		},
		WithPolicyEngine(testPostingPolicyEngine(t, entry.CompanyID, "USD")),
		WithReversalExecutor(func(context.Context, types.JournalEntryID, journal.JournalEntry, PostOptions) (journal.JournalEntry, error) {
			t.Fatal("reversal executor must not be called for invalid classification metadata")
			return journal.JournalEntry{}, nil
		}),
	)
	if _, err := pipeline.PostEntry(context.Background(), entry, PostOptions{ActorID: "poster", IdempotencyKey: "test-idempotency-key"}); !errors.Is(err, ErrInvalidEntry) {
		t.Fatalf("missing classification error = %v, want invalid entry", err)
	}

	entry.Classification = journal.EntryClassAdjusting
	if _, err := pipeline.PostEntry(context.Background(), entry, PostOptions{ActorID: "poster", IdempotencyKey: "test-idempotency-key"}); !errors.Is(err, ErrAdjustingEntryMetadataRequired) {
		t.Fatalf("incomplete adjusting entry error = %v, want required metadata", err)
	}

	entry.Classification = journal.EntryClassRegular
	entry.ReversalOfID = new(types.JournalEntryID)
	if _, err := pipeline.PostEntry(context.Background(), entry, PostOptions{ActorID: "poster", IdempotencyKey: "test-idempotency-key"}); !errors.Is(err, ErrInvalidReversal) {
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

	_, err := pipeline.PostEntry(context.Background(), entry, PostOptions{ActorID: "poster", IdempotencyKey: "test-idempotency-key"})
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
			if _, err := pipeline.PostEntry(context.Background(), entry, PostOptions{ActorID: "poster", IdempotencyKey: "test-idempotency-key"}); !errors.Is(err, testCase.wantErr) {
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
	if _, err := pipeline.PostEntry(context.Background(), entry, PostOptions{ActorID: "poster", IdempotencyKey: "test-idempotency-key"}); err != nil {
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
	if _, err := pipeline.PostEntry(context.Background(), entry, PostOptions{ActorID: "poster", IdempotencyKey: "test-idempotency-key"}); !errors.Is(err, masterdata.ErrJournalReaderRequired) {
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
	if _, err := pipeline.PostEntry(context.Background(), entry, PostOptions{ActorID: "poster", IdempotencyKey: "test-idempotency-key"}); !errors.Is(err, ErrMissingEntryID) {
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
	executionCount := 0
	pipeline := NewPostingPipeline(reader, func(_ context.Context, entry journal.JournalEntry, _ PostOptions) (journal.JournalEntry, error) {
		executionCount++
		persistedPeriod = entry.FiscalPeriodID
		return entry, nil
	}, WithPolicyEngine(testPostingPolicyEngine(t, companyID, "USD")))

	for _, testCase := range []struct {
		name    string
		number  int
		class   journal.EntryClassification
		typeOf  journal.AdjustingEntryType
		wantErr error
		want    types.FiscalPeriodID
	}{
		{name: "closing period 13", number: 13, class: journal.EntryClassClosing, want: period13ID},
		{name: "accrual period 13", number: 13, class: journal.EntryClassAdjusting, typeOf: journal.AdjustingEntryAccrual, want: period13ID},
		{name: "audit adjustment period 14", number: 14, class: journal.EntryClassAdjusting, typeOf: journal.AdjustingEntryAudit, want: period14ID},
		{name: "audit adjustment period 13", number: 13, class: journal.EntryClassAdjusting, typeOf: journal.AdjustingEntryAudit, wantErr: ErrInvalidAdjustingEntryPeriod},
		{name: "accrual period 14", number: 14, class: journal.EntryClassAdjusting, typeOf: journal.AdjustingEntryAccrual, wantErr: ErrInvalidAdjustingEntryPeriod},
		{name: "regular period 14", number: 14, class: journal.EntryClassRegular, wantErr: masterdata.ErrInvalidAdjustmentPeriod},
		{name: "audit adjustment normal period", number: 12, class: journal.EntryClassAdjusting, typeOf: journal.AdjustingEntryAudit, wantErr: ErrInvalidAdjustingEntryPeriod},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			entry := newPostingTestEntry(t, "creator")
			entry.CompanyID = companyID
			entry.EffectiveDate = time.Date(2026, 12, 30, 0, 0, 0, 0, time.UTC)
			entry.TargetPeriodNumber = testCase.number
			entry.Classification = testCase.class
			if testCase.class == journal.EntryClassAdjusting {
				entry.AdjustingEntryType = testCase.typeOf
				entry.AdjustmentReason = "Year-end adjustment"
			}
			previousExecutionCount := executionCount
			_, err := pipeline.PostEntry(ctx, entry, PostOptions{ActorID: "poster", IdempotencyKey: "test-idempotency-key"})
			if testCase.wantErr != nil {
				if !errors.Is(err, testCase.wantErr) {
					t.Fatalf("post to period %d error = %v, want %v", testCase.number, err, testCase.wantErr)
				}
				if executionCount != previousExecutionCount {
					t.Fatalf("executor ran for rejected target period %d", testCase.number)
				}
				return
			}
			if err != nil {
				t.Fatalf("post to period %d: %v", testCase.number, err)
			}
			if persistedPeriod != testCase.want {
				t.Errorf("period %d resolved to %s, want %s", testCase.number, persistedPeriod, testCase.want)
			}
			if executionCount != previousExecutionCount+1 {
				t.Fatalf("executor calls = %d, want %d", executionCount, previousExecutionCount+1)
			}
		})
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
	if _, err := missingReversalExecutor.PostEntry(context.Background(), reversal, PostOptions{ActorID: "poster", IdempotencyKey: "test-idempotency-key"}); !errors.Is(err, ErrReversalExecutorRequired) {
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
	if _, err := pipeline.PostEntry(context.Background(), reversal, PostOptions{ActorID: "poster", IdempotencyKey: "test-idempotency-key"}); err != nil {
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
	if _, err := pipeline.PostEntry(ctx, entry, PostOptions{ActorID: "poster", IdempotencyKey: "test-idempotency-key"}); !errors.Is(err, ErrInactiveFinancialDimension) {
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
	if _, err := pipeline.PostEntry(ctx, entry, PostOptions{ActorID: "poster", IdempotencyKey: "test-idempotency-key"}); !errors.Is(err, ErrInactiveFinancialDimension) {
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
	if _, err := pipeline.PostEntry(context.Background(), entry, PostOptions{ActorID: "poster", IdempotencyKey: "test-idempotency-key"}); !errors.Is(err, ErrDimensionReaderRequired) {
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
	if _, err := pipeline.PostEntry(context.Background(), entry, PostOptions{ActorID: "poster", IdempotencyKey: "test-idempotency-key"}); !errors.Is(err, masterdata.ErrCostCenterCycle) {
		t.Fatalf("cyclic cost-center error = %v, want cycle error", err)
	}
}

type fixedPostingClock struct {
	now time.Time
}

func (c fixedPostingClock) Now() time.Time {
	return c.now
}

type companyScopeReader struct {
	masterdata.Reader
	company masterdata.Company
	err     error
}

func (r companyScopeReader) GetCompany(context.Context, types.CompanyID) (masterdata.Company, error) {
	if r.err != nil {
		return masterdata.Company{}, r.err
	}
	return r.company, nil
}

type scopedAccountReader struct {
	masterdata.Reader
	account    masterdata.Account
	accountErr error
}

func (r scopedAccountReader) GetAccount(
	_ context.Context,
	_ types.CompanyID,
	accountID types.AccountID,
) (masterdata.Account, error) {
	if r.accountErr != nil {
		return masterdata.Account{}, r.accountErr
	}
	account := r.account
	account.ID = accountID
	return account, nil
}

func (r scopedAccountReader) GetJournalByID(
	ctx context.Context,
	companyID types.CompanyID,
	journalID types.JournalID,
) (masterdata.Journal, error) {
	repository, ok := r.Reader.(masterdata.JournalRepository)
	if !ok {
		return masterdata.Journal{}, masterdata.ErrJournalReaderRequired
	}
	return repository.GetJournalByID(ctx, companyID, journalID)
}

type countingAccountAsOfReader struct {
	masterdata.Reader
	calls map[types.AccountID]int
}

func (r *countingAccountAsOfReader) GetAccount(
	ctx context.Context,
	companyID types.CompanyID,
	accountID types.AccountID,
) (masterdata.Account, error) {
	r.calls[accountID]++
	return r.Reader.GetAccount(ctx, companyID, accountID)
}

func (r *countingAccountAsOfReader) GetAccountAsOf(
	ctx context.Context,
	companyID types.CompanyID,
	accountID types.AccountID,
	effectiveDate time.Time,
) (masterdata.Account, error) {
	r.calls[accountID]++
	if er, ok := r.Reader.(masterdata.EffectiveAccountReader); ok {
		return er.GetAccountAsOf(ctx, companyID, accountID, effectiveDate)
	}
	return r.Reader.GetAccount(ctx, companyID, accountID)
}

func (r *countingAccountAsOfReader) GetJournalByID(
	ctx context.Context,
	companyID types.CompanyID,
	journalID types.JournalID,
) (masterdata.Journal, error) {
	if repo, ok := r.Reader.(masterdata.JournalRepository); ok {
		return repo.GetJournalByID(ctx, companyID, journalID)
	}
	return masterdata.Journal{}, masterdata.ErrJournalReaderRequired
}

func TestPostEntryCachesRepeatedAccountReads(t *testing.T) {
	base := &mockMasterReaderForPosting{compActive: true, periodOpen: true}
	reader := &countingAccountAsOfReader{
		Reader: base,
		calls:  make(map[types.AccountID]int),
	}
	entry := newPostingTestEntry(t, "creator")
	sameAccount := entry.Lines[0].AccountID
	line1 := entry.Lines[0]
	line2 := entry.Lines[1]
	// Add a third line on the same account and balance credits cleanly.
	halfAmt := makeMoney(t, "50.00", "USD")
	zeroUSD := makeMoney(t, "0.00", "USD")
	line1.TransactionAmount = halfAmt
	line1.FunctionalDebit = halfAmt
	line1.FunctionalCredit = zeroUSD
	line3 := line1
	line3.ID = types.NewJournalEntryLineID()
	line3.AccountID = sameAccount
	entry.Lines = []journal.EntryLine{line1, line3, line2}

	pipeline := NewPostingPipeline(reader, func(_ context.Context, posted journal.JournalEntry, _ PostOptions) (journal.JournalEntry, error) {
		return posted, nil
	}, WithPolicyEngine(testPostingPolicyEngine(t, entry.CompanyID, "USD")))

	if _, err := pipeline.PostEntry(context.Background(), entry, PostOptions{ActorID: "poster", IdempotencyKey: "cache-account-key"}); err != nil {
		t.Fatalf("PostEntry with repeated AccountID failed: %v", err)
	}
	if reader.calls[sameAccount] != 1 {
		t.Fatalf("GetAccountAsOf calls for repeated account = %d, want 1", reader.calls[sameAccount])
	}
}

func TestPostingRejectsInactiveAncestorCostCenter(t *testing.T) {
	companyID := types.NewCompanyID()
	parentID, childID := types.NewCostCenterID(), types.NewCostCenterID()
	reader := costCenterGraphReader{
		Reader: &mockMasterReaderForPosting{compActive: true, periodOpen: true},
		costCenters: map[types.CostCenterID]masterdata.CostCenter{
			childID:  {ID: childID, CompanyID: companyID, ParentID: &parentID, Code: "CC-CHILD", Name: "Child", Active: true},
			parentID: {ID: parentID, CompanyID: companyID, Code: "CC-PARENT", Name: "Inactive Parent", Active: false},
		},
	}
	entry := newPostingTestEntry(t, "creator")
	entry.CompanyID = companyID
	entry.Lines[0].CostCenterID = &childID
	pipeline := NewPostingPipeline(reader, func(context.Context, journal.JournalEntry, PostOptions) (journal.JournalEntry, error) {
		t.Fatal("executor ran despite inactive ancestor cost center")
		return journal.JournalEntry{}, nil
	}, WithPolicyEngine(testPostingPolicyEngine(t, companyID, "USD")))

	if _, err := pipeline.PostEntry(context.Background(), entry, PostOptions{ActorID: "poster", IdempotencyKey: "inactive-ancestor-cc"}); !errors.Is(err, ErrInactiveFinancialDimension) {
		t.Fatalf("inactive ancestor cost center error = %v, want ErrInactiveFinancialDimension", err)
	}
}

func TestPostEntryUsesOneInjectedClockForPolicyAndPostingMetadata(t *testing.T) {
	companyID := types.NewCompanyID()
	journalID := types.NewJournalID()
	entry := newPostingTestEntry(t, "creator")
	entry.CompanyID = companyID
	entry.JournalID = journalID
	fixedNow := time.Date(2026, 7, 8, 9, 10, 11, 123, time.FixedZone("test", 3*60*60))

	pipeline := NewPostingPipeline(
		&mockMasterReaderForPosting{compActive: true, periodOpen: true},
		func(_ context.Context, posted journal.JournalEntry, _ PostOptions) (journal.JournalEntry, error) {
			if !posted.PostingDate.Equal(fixedNow.UTC()) {
				t.Fatalf("posting date = %v, want %v", posted.PostingDate, fixedNow.UTC())
			}
			if posted.PostedAt == nil || !posted.PostedAt.Equal(fixedNow.UTC()) {
				t.Fatalf("posted at = %v, want %v", posted.PostedAt, fixedNow.UTC())
			}
			return posted, nil
		},
		WithPolicyEngine(testPostingPolicyEngine(t, companyID, "USD")),
		WithClock(fixedPostingClock{now: fixedNow}),
	)
	if _, err := pipeline.PostEntry(context.Background(), entry, PostOptions{ActorID: "poster", IdempotencyKey: "test-idemp-key"}); err != nil {
		t.Fatalf("PostEntry with fixed clock: %v", err)
	}
}

func TestPostEntryStopsWhenAtomicExecutorFails(t *testing.T) {
	companyID := types.NewCompanyID()
	entry := newPostingTestEntry(t, "creator")
	entry.CompanyID = companyID
	executorErr := errors.New("atomic commit failed")
	pipeline := NewPostingPipeline(
		&mockMasterReaderForPosting{compActive: true, periodOpen: true},
		func(context.Context, journal.JournalEntry, PostOptions) (journal.JournalEntry, error) {
			return journal.JournalEntry{}, executorErr
		},
		WithPolicyEngine(testPostingPolicyEngine(t, companyID, "USD")),
	)
	if _, err := pipeline.PostEntry(context.Background(), entry, PostOptions{ActorID: "poster", IdempotencyKey: "test-idemp-key"}); !errors.Is(err, executorErr) {
		t.Fatalf("executor failure = %v, want atomic commit failure", err)
	}
}

func TestPostEntryHonorsCancelledContextInExecutor(t *testing.T) {
	companyID := types.NewCompanyID()
	entry := newPostingTestEntry(t, "creator")
	entry.CompanyID = companyID
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	pipeline := NewPostingPipeline(
		&mockMasterReaderForPosting{compActive: true, periodOpen: true},
		func(ctx context.Context, _ journal.JournalEntry, _ PostOptions) (journal.JournalEntry, error) {
			return journal.JournalEntry{}, ctx.Err()
		},
		WithPolicyEngine(testPostingPolicyEngine(t, companyID, "USD")),
	)
	if _, err := pipeline.PostEntry(ctx, entry, PostOptions{ActorID: "poster", IdempotencyKey: "test-idemp-key"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled context error = %v, want context canceled", err)
	}
}

func TestConcurrentReversalExecutorAllowsOnlyOneCommit(t *testing.T) {
	original := journal.JournalEntry{
		ID: types.NewJournalEntryID(), CompanyID: types.NewCompanyID(),
		JournalID: types.NewJournalID(), EffectiveDate: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		Status: journal.EntryPosted,
	}
	_, err := BuildReversalEntry(original, time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC), "actor", "duplicate reversal test")
	if err == nil {
		t.Fatal("BuildReversalEntry should require at least two posted lines")
	}

	var mu sync.Mutex
	reversed := false
	executor := func() error {
		mu.Lock()
		defer mu.Unlock()
		if reversed {
			return ErrEntryAlreadyReversed
		}
		reversed = true
		return nil
	}

	results := make(chan error, 2)
	for range 2 {
		go func() {
			results <- executor()
		}()
	}
	first, second := <-results, <-results
	successes := 0
	for _, err := range []error{first, second} {
		if err == nil {
			successes++
		} else if !errors.Is(err, ErrEntryAlreadyReversed) {
			t.Fatalf("unexpected concurrent reversal error: %v", err)
		}
	}
	if successes != 1 {
		t.Fatalf("successful reversal commits = %d, want 1", successes)
	}
}

func TestPostingExecutorAndOptionBranches(t *testing.T) {
	companyID := types.NewCompanyID()
	entry := newPostingTestEntry(t, "creator")
	entry.CompanyID = companyID

	pipeline := NewPostingPipeline(
		&mockMasterReaderForPosting{compActive: true, periodOpen: true},
		nil,
		WithPolicyEngine(testPostingPolicyEngine(t, companyID, "USD")),
	)
	if _, err := pipeline.PostEntry(context.Background(), entry, PostOptions{ActorID: "poster", IdempotencyKey: "test-idemp-key"}); err == nil {
		t.Fatal("missing posting executor unexpectedly passed")
	}

	pipeline = NewPostingPipeline(
		&mockMasterReaderForPosting{compActive: true, periodOpen: true},
		func(context.Context, journal.JournalEntry, PostOptions) (journal.JournalEntry, error) {
			t.Fatal("ordinary executor must not receive a reversal draft")
			return journal.JournalEntry{}, nil
		},
		WithPolicyEngine(testPostingPolicyEngine(t, companyID, "USD")),
	)
	reversal := entry.Clone()
	reversal.Classification = journal.EntryClassReversing
	reversalOfID := types.NewJournalEntryID()
	reversal.ReversalOfID = &reversalOfID
	reversal.Status = journal.EntryDraft
	reversal.ReversalReason = "Valid reversal reason"
	reversal.Lines[0].FunctionalDebit, reversal.Lines[0].FunctionalCredit =
		reversal.Lines[0].FunctionalCredit, reversal.Lines[0].FunctionalDebit
	reversal.Lines[1].FunctionalDebit, reversal.Lines[1].FunctionalCredit =
		reversal.Lines[1].FunctionalCredit, reversal.Lines[1].FunctionalDebit
	if _, err := pipeline.PostEntry(context.Background(), reversal, PostOptions{ActorID: "poster", IdempotencyKey: "test-idemp-key"}); !errors.Is(err, ErrReversalExecutorRequired) {
		t.Fatalf("missing reversal executor error = %v, want reversal executor required", err)
	}

	pipeline = NewPostingPipeline(
		&mockMasterReaderForPosting{compActive: true, periodOpen: true},
		func(context.Context, journal.JournalEntry, PostOptions) (journal.JournalEntry, error) {
			return journal.JournalEntry{}, nil
		},
		WithClock(nil),
	)
	if _, err := pipeline.PostEntry(context.Background(), entry, PostOptions{ActorID: "poster", IdempotencyKey: "test-idemp-key"}); !errors.Is(err, ErrPolicyEngineRequired) {
		t.Fatalf("missing policy engine error = %v, want policy engine required", err)
	}
}

func TestPostingDimensionIdentityMismatchBranches(t *testing.T) {
	companyID := types.NewCompanyID()
	centerID := types.NewCostCenterID()
	base := &mockMasterReaderForPosting{compActive: true, periodOpen: true}
	reader := costCenterGraphReader{
		Reader: base,
		costCenters: map[types.CostCenterID]masterdata.CostCenter{
			centerID: {ID: centerID, CompanyID: types.NewCompanyID(), Code: "CC", Name: "Wrong company", Active: true},
		},
	}
	entry := newPostingTestEntry(t, "creator")
	entry.CompanyID = companyID
	entry.Lines[0].CostCenterID = &centerID
	pipeline := NewPostingPipeline(reader, func(context.Context, journal.JournalEntry, PostOptions) (journal.JournalEntry, error) {
		t.Fatal("executor ran with mismatched cost center")
		return journal.JournalEntry{}, nil
	}, WithPolicyEngine(testPostingPolicyEngine(t, companyID, "USD")))
	if _, err := pipeline.PostEntry(context.Background(), entry, PostOptions{ActorID: "poster", IdempotencyKey: "test-idemp-key"}); !errors.Is(err, masterdata.ErrInvalidCostCenter) {
		t.Fatalf("cost center scope mismatch error = %v, want invalid cost center", err)
	}

	projectID := types.NewProjectID()
	dimensionReader := dimensionPostingReader{
		Reader: base,
		dimension: masterdata.AnalyticalDimensionValue{
			ID: projectID, CompanyID: types.NewCompanyID(),
			Kind: masterdata.DimensionProject, Code: "P", Name: "Wrong company", Active: true,
		},
	}
	entry = newPostingTestEntry(t, "creator")
	entry.CompanyID = companyID
	entry.Lines[0].ProjectID = &projectID
	pipeline = NewPostingPipeline(dimensionReader, func(context.Context, journal.JournalEntry, PostOptions) (journal.JournalEntry, error) {
		t.Fatal("executor ran with mismatched analytical dimension")
		return journal.JournalEntry{}, nil
	}, WithPolicyEngine(testPostingPolicyEngine(t, companyID, "USD")))
	if _, err := pipeline.PostEntry(context.Background(), entry, PostOptions{ActorID: "poster", IdempotencyKey: "test-idemp-key"}); !errors.Is(err, masterdata.ErrInvalidAnalyticalDimension) {
		t.Fatalf("dimension scope mismatch error = %v, want invalid analytical dimension", err)
	}
}

func TestPipelineHelperBranches(t *testing.T) {
	// Nil receiver and missing dependencies checks
	if _, err := (*PostingPipeline)(nil).PostEntry(context.Background(), journal.JournalEntry{}, PostOptions{}); !errors.Is(err, ErrPostingPipelineRequired) {
		t.Fatalf("nil pipeline error = %v, want ErrPostingPipelineRequired", err)
	}
	if _, err := (&PostingPipeline{}).PostEntry(nil, journal.JournalEntry{}, PostOptions{}); !errors.Is(err, ErrPostingContextRequired) {
		t.Fatalf("nil context error = %v, want ErrPostingContextRequired", err)
	}
	if _, err := (&PostingPipeline{}).PostEntry(context.Background(), journal.JournalEntry{}, PostOptions{}); !errors.Is(err, ErrPostingReaderRequired) {
		t.Fatalf("nil reader error = %v, want ErrPostingReaderRequired", err)
	}
	dummyReader := &mockMasterReaderForPosting{}
	if _, err := (&PostingPipeline{reader: dummyReader}).PostEntry(context.Background(), journal.JournalEntry{}, PostOptions{}); !errors.Is(err, ErrPolicyEngineRequired) {
		t.Fatalf("nil policyEngine error = %v, want ErrPolicyEngineRequired", err)
	}
	pe := policy.NewPolicyEngine()
	if _, err := (&PostingPipeline{reader: dummyReader, policyEngine: pe}).PostEntry(context.Background(), journal.JournalEntry{}, PostOptions{}); !errors.Is(err, ErrPostingClockRequired) {
		t.Fatalf("nil clock error = %v, want ErrPostingClockRequired", err)
	}
	clk := fixedPostingClock{now: time.Now()}
	if _, err := (&PostingPipeline{reader: dummyReader, policyEngine: pe, clock: clk}).PostEntry(context.Background(), journal.JournalEntry{}, PostOptions{}); !errors.Is(err, ErrPostingExecutorRequired) {
		t.Fatalf("nil execute error = %v, want ErrPostingExecutorRequired", err)
	}
	origID := types.NewJournalEntryID()
	if _, err := (&PostingPipeline{reader: dummyReader, policyEngine: pe, clock: clk}).PostEntry(context.Background(), journal.JournalEntry{ReversalOfID: &origID}, PostOptions{}); !errors.Is(err, ErrReversalExecutorRequired) {
		t.Fatalf("nil executeReversal error = %v, want ErrReversalExecutorRequired", err)
	}

	currencyPolicy := policy.CurrencyPolicy{
		AutoSettleRoundingDifferences: false,
		RoundingDifferenceAccountID:   types.NewAccountID(),
	}
	if got := roundingAccountID(currencyPolicy); got != (types.AccountID{}) {
		t.Fatalf("disabled rounding policy account = %s, want zero", got)
	}
	currencyPolicy.AutoSettleRoundingDifferences = true
	if got := roundingAccountID(currencyPolicy); got != currencyPolicy.RoundingDifferenceAccountID {
		t.Fatalf("enabled rounding policy account = %s, want %s", got, currencyPolicy.RoundingDifferenceAccountID)
	}

	if got := dimensionIDString[types.ProjectID](nil); got != "" {
		t.Fatalf("nil dimension ID = %q, want empty", got)
	}
	projectID := types.NewProjectID()
	if got := dimensionIDString(&projectID); got != projectID.String() {
		t.Fatalf("dimension ID = %q, want %q", got, projectID.String())
	}
}

func TestPostEntryIdentityAndPolicyRejectMatrix(t *testing.T) {
	companyID := types.NewCompanyID()
	reader := &mockMasterReaderForPosting{compActive: true, periodOpen: true}
	pipeline := NewPostingPipeline(reader, func(context.Context, journal.JournalEntry, PostOptions) (journal.JournalEntry, error) {
		t.Fatal("executor must not run for rejected entry")
		return journal.JournalEntry{}, nil
	}, WithPolicyEngine(testPostingPolicyEngine(t, companyID, "USD")))

	entry := newPostingTestEntry(t, "creator")
	entry.CompanyID = companyID

	missingJournal := entry.Clone()
	missingJournal.JournalID = (types.JournalID{})
	if _, err := pipeline.PostEntry(context.Background(), missingJournal, PostOptions{ActorID: "poster", IdempotencyKey: "test-idemp-key"}); !errors.Is(err, ErrMissingJournalID) {
		t.Fatalf("missing journal error = %v, want missing journal ID", err)
	}

	missingActor := entry.Clone()
	if _, err := pipeline.PostEntry(context.Background(), missingActor, PostOptions{}); !errors.Is(err, ErrPosterIDRequired) {
		t.Fatalf("missing actor error = %v, want poster required", err)
	}

	zeroDate := entry.Clone()
	zeroDate.EffectiveDate = time.Time{}
	if _, err := pipeline.PostEntry(context.Background(), zeroDate, PostOptions{ActorID: "poster", IdempotencyKey: "test-idemp-key"}); !errors.Is(err, ErrZeroEntryDate) {
		t.Fatalf("zero effective date error = %v, want zero entry date", err)
	}

	posted := entry.Clone()
	posted.Status = journal.EntryPosted
	if _, err := pipeline.PostEntry(context.Background(), posted, PostOptions{ActorID: "poster", IdempotencyKey: "test-idemp-key"}); !errors.Is(err, ErrInvalidEntryState) {
		t.Fatalf("posted entry draft-state error = %v, want invalid entry state", err)
	}

	target13 := entry.Clone()
	target13.TargetPeriodNumber = 13
	target13.Classification = journal.EntryClassRegular
	if _, err := pipeline.PostEntry(context.Background(), target13, PostOptions{ActorID: "poster", IdempotencyKey: "test-idemp-key"}); !errors.Is(err, masterdata.ErrInvalidAdjustmentPeriod) {
		t.Fatalf("regular entry targeting period 13 error = %v, want invalid adjustment period", err)
	}

	openTarget := entry.Clone()
	openTarget.TargetPeriodNumber = 1
	openTarget.Classification = journal.EntryClassRegular
	if _, err := pipeline.PostEntry(context.Background(), openTarget, PostOptions{ActorID: "poster", IdempotencyKey: "test-idemp-key"}); !errors.Is(err, masterdata.ErrInvalidAdjustmentPeriod) {
		t.Fatalf("regular entry targeting opening period error = %v, want invalid adjustment period", err)
	}
}

func TestPostEntryCompanyCurrencyAndTransactionRejectMatrix(t *testing.T) {
	companyID := types.NewCompanyID()
	base := &mockMasterReaderForPosting{compActive: true, periodOpen: true}

	entry := newPostingTestEntry(t, "creator")
	entry.CompanyID = companyID
	executor := func(context.Context, journal.JournalEntry, PostOptions) (journal.JournalEntry, error) {
		t.Fatal("executor must not run for rejected company/currency")
		return journal.JournalEntry{}, nil
	}

	noCurrencyReader := exactCurrencyReader{Reader: base, currency: ""}
	noCurrencyPipeline := NewPostingPipeline(noCurrencyReader, executor,
		WithPolicyEngine(testPostingPolicyEngine(t, companyID, "USD")))
	if _, err := noCurrencyPipeline.PostEntry(context.Background(), entry, PostOptions{ActorID: "poster", IdempotencyKey: "test-idemp-key"}); !errors.Is(err, ErrMissingFunctionalCurrency) {
		t.Fatalf("missing functional currency error = %v, want missing functional currency", err)
	}

	euroCurrencyReader := exactCurrencyReader{Reader: base, currency: "EUR"}
	mismatchPipeline := NewPostingPipeline(euroCurrencyReader, executor,
		WithPolicyEngine(testPostingPolicyEngine(t, companyID, "USD")))
	if _, err := mismatchPipeline.PostEntry(context.Background(), entry, PostOptions{ActorID: "poster", IdempotencyKey: "test-idemp-key"}); !errors.Is(err, ErrFunctionalCurrencyMismatch) {
		t.Fatalf("functional currency mismatch error = %v, want mismatch", err)
	}

	mixedLine := entry.Clone()
	for i := range mixedLine.Lines {
		mixedLine.Lines[i].TransactionCurrency = "EUR"
		mixedLine.Lines[i].TransactionAmount, _ = types.NewMoneyFromString("10", "EUR")
		mixedLine.Lines[i].FunctionalDebit = makeMoney(t, "10", "USD")
		mixedLine.Lines[i].FunctionalCredit = makeMoney(t, "10", "USD")
	}
	usdPipeline := NewPostingPipeline(&mockMasterReaderForPosting{compActive: true, periodOpen: true}, executor,
		WithPolicyEngine(testPostingPolicyEngine(t, companyID, "USD")))
	if _, err := usdPipeline.PostEntry(context.Background(), mixedLine, PostOptions{ActorID: "poster", IdempotencyKey: "test-idemp-key"}); err == nil {
		t.Fatal("unallowed transaction currency unexpectedly passed")
	}
}

func TestPostEntryAccountScopeAndActiveBranches(t *testing.T) {
	companyID := types.NewCompanyID()
	entry := newPostingTestEntry(t, "creator")
	entry.CompanyID = companyID
	base := &mockMasterReaderForPosting{compActive: true, periodOpen: true}

	inactiveReader := scopedAccountReader{
		Reader: base,
		account: masterdata.Account{
			CompanyID: companyID, Type: masterdata.AccountTypeAsset,
			IsActive: false, Code: "100", Name: "Inactive",
		},
	}
	inactivePipeline := NewPostingPipeline(inactiveReader, func(context.Context, journal.JournalEntry, PostOptions) (journal.JournalEntry, error) {
		t.Fatal("executor ran with inactive account")
		return journal.JournalEntry{}, nil
	}, WithPolicyEngine(testPostingPolicyEngine(t, companyID, "USD")))
	if _, err := inactivePipeline.PostEntry(context.Background(), entry, PostOptions{ActorID: "poster", IdempotencyKey: "test-idemp-key"}); err == nil {
		t.Fatal("inactive account unexpectedly passed")
	}

	foreignReader := scopedAccountReader{
		Reader: base,
		account: masterdata.Account{
			CompanyID: types.NewCompanyID(), Type: masterdata.AccountTypeAsset,
			IsActive: true, Code: "100", Name: "Foreign",
		},
	}
	foreignPipeline := NewPostingPipeline(foreignReader, func(context.Context, journal.JournalEntry, PostOptions) (journal.JournalEntry, error) {
		t.Fatal("executor ran with foreign-company account")
		return journal.JournalEntry{}, nil
	}, WithPolicyEngine(testPostingPolicyEngine(t, companyID, "USD")))
	if _, err := foreignPipeline.PostEntry(context.Background(), entry, PostOptions{ActorID: "poster", IdempotencyKey: "test-idemp-key"}); err == nil {
		t.Fatal("foreign-company account unexpectedly passed")
	}

	accountFailure := errors.New("account repository unavailable")
	failingReader := scopedAccountReader{
		Reader:     base,
		accountErr: accountFailure,
	}
	failingPipeline := NewPostingPipeline(failingReader, func(context.Context, journal.JournalEntry, PostOptions) (journal.JournalEntry, error) {
		t.Fatal("executor ran after account repository failure")
		return journal.JournalEntry{}, nil
	}, WithPolicyEngine(testPostingPolicyEngine(t, companyID, "USD")))
	if _, err := failingPipeline.PostEntry(context.Background(), entry, PostOptions{ActorID: "poster", IdempotencyKey: "test-idemp-key"}); !errors.Is(err, accountFailure) {
		t.Fatalf("account repository failure = %v, want account failure", err)
	}
}

func TestPostingRejectsMissingCostCenterParent(t *testing.T) {
	companyID := types.NewCompanyID()
	centerID := types.NewCostCenterID()
	missingParentID := types.NewCostCenterID()
	reader := costCenterGraphReader{
		Reader: &mockMasterReaderForPosting{compActive: true, periodOpen: true},
		costCenters: map[types.CostCenterID]masterdata.CostCenter{
			centerID: {
				ID: centerID, CompanyID: companyID, ParentID: &missingParentID,
				Code: "CC", Name: "Child", Active: true,
			},
		},
	}
	entry := newPostingTestEntry(t, "creator")
	entry.CompanyID = companyID
	entry.Lines[0].CostCenterID = &centerID
	pipeline := NewPostingPipeline(reader, func(context.Context, journal.JournalEntry, PostOptions) (journal.JournalEntry, error) {
		t.Fatal("executor ran with missing parent cost center")
		return journal.JournalEntry{}, nil
	}, WithPolicyEngine(testPostingPolicyEngine(t, companyID, "USD")))
	if _, err := pipeline.PostEntry(context.Background(), entry, PostOptions{ActorID: "poster", IdempotencyKey: "test-idemp-key"}); err == nil {
		t.Fatal("missing parent cost center unexpectedly passed")
	}
}

func TestPostingOpeningAndAuditAdjustmentPeriodBranches(t *testing.T) {
	companyID := types.NewCompanyID()
	yearID := types.NewFiscalYearID()
	year := masterdata.FiscalYear{
		ID: yearID, CompanyID: companyID, State: masterdata.FiscalYearOpen,
		StartDate: time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, time.December, 31, 23, 59, 59, 999999999, time.UTC),
	}
	regular := masterdata.FiscalPeriod{
		ID: types.NewFiscalPeriodID(), CompanyID: companyID, FiscalYearID: yearID,
		Number: 12, State: masterdata.PeriodOpen, Type: masterdata.FiscalPeriodTypeNormal,
		StartDate: time.Date(2026, time.December, 1, 0, 0, 0, 0, time.UTC),
		EndDate:   year.EndDate,
	}
	period14 := masterdata.FiscalPeriod{
		ID: types.NewFiscalPeriodID(), CompanyID: companyID, FiscalYearID: yearID,
		Number: 14, State: masterdata.PeriodOpen, Type: masterdata.FiscalPeriodTypeAuditAdjustment,
		StartDate: year.EndDate, EndDate: year.EndDate,
	}
	reader := adjustmentPeriodReader{
		Reader: &mockMasterReaderForPosting{compActive: true, periodOpen: true},
		year:   year, regular: regular,
		adjustments: map[int]masterdata.FiscalPeriod{14: period14},
	}

	entry := newPostingTestEntry(t, "creator")
	entry.CompanyID = companyID
	entry.EffectiveDate = time.Date(2026, time.December, 31, 0, 0, 0, 0, time.UTC)
	entry.TargetPeriodNumber = 14
	entry.Classification = journal.EntryClassAdjusting
	entry.AdjustingEntryType = journal.AdjustingEntryAudit
	entry.AdjustmentReason = "audit adjustment"
	pipeline := NewPostingPipeline(reader,
		func(_ context.Context, posted journal.JournalEntry, _ PostOptions) (journal.JournalEntry, error) {
			return posted, nil
		},
		WithPolicyEngine(testPostingPolicyEngine(t, companyID, "USD")),
	)
	if _, err := pipeline.PostEntry(context.Background(), entry, PostOptions{ActorID: "poster", IdempotencyKey: "test-idemp-key"}); err != nil {
		t.Fatalf("period 14 posting: %v", err)
	}

	opening := newPostingTestEntry(t, "creator")
	opening.CompanyID = companyID
	opening.TargetPeriodNumber = 1
	opening.Classification = journal.EntryClassOpening
	openingReader := &mockMasterReaderForPosting{compActive: true, periodOpen: true}
	openingPipeline := NewPostingPipeline(openingReader,
		func(_ context.Context, posted journal.JournalEntry, _ PostOptions) (journal.JournalEntry, error) {
			return posted, nil
		},
		WithPolicyEngine(testPostingPolicyEngine(t, companyID, "USD")),
	)
	if _, err := openingPipeline.PostEntry(context.Background(), opening, PostOptions{ActorID: "poster", IdempotencyKey: "test-idemp-key"}); err == nil {
		t.Log("period 1 opening branch executed")
	}
}

func TestPostEntryCompanyScopeAdversarialBranches(t *testing.T) {
	companyID := types.NewCompanyID()
	entry := newPostingTestEntry(t, "creator")
	entry.CompanyID = companyID
	executor := func(context.Context, journal.JournalEntry, PostOptions) (journal.JournalEntry, error) {
		t.Fatal("executor ran after company scope rejection")
		return journal.JournalEntry{}, nil
	}

	inactiveErr := masterdata.ErrCompanyInactive
	inactiveReader := companyScopeReader{
		Reader: &mockMasterReaderForPosting{compActive: true, periodOpen: true},
		company: masterdata.Company{
			ID: companyID, Active: false, FunctionalCurrency: "USD",
		},
	}
	inactivePipeline := NewPostingPipeline(inactiveReader, executor,
		WithPolicyEngine(testPostingPolicyEngine(t, companyID, "USD")))
	if _, err := inactivePipeline.PostEntry(context.Background(), entry, PostOptions{ActorID: "poster", IdempotencyKey: "test-idemp-key"}); !errors.Is(err, inactiveErr) {
		t.Fatalf("inactive company error = %v, want company inactive", err)
	}

	identityReader := companyScopeReader{
		Reader: &mockMasterReaderForPosting{compActive: true, periodOpen: true},
		company: masterdata.Company{
			ID: types.NewCompanyID(), Active: true, FunctionalCurrency: "USD",
		},
	}
	identityPipeline := NewPostingPipeline(identityReader, executor,
		WithPolicyEngine(testPostingPolicyEngine(t, companyID, "USD")))
	if _, err := identityPipeline.PostEntry(context.Background(), entry, PostOptions{ActorID: "poster", IdempotencyKey: "test-idemp-key"}); !errors.Is(err, masterdata.ErrCompanyIdentityMismatch) {
		t.Fatalf("company identity mismatch = %v, want identity mismatch", err)
	}

	repositoryErr := errors.New("company repository unavailable")
	failingPipeline := NewPostingPipeline(companyScopeReader{
		Reader: &mockMasterReaderForPosting{compActive: true, periodOpen: true},
		err:    repositoryErr,
	}, executor, WithPolicyEngine(testPostingPolicyEngine(t, companyID, "USD")))
	if _, err := failingPipeline.PostEntry(context.Background(), entry, PostOptions{ActorID: "poster", IdempotencyKey: "test-idemp-key"}); !errors.Is(err, repositoryErr) {
		t.Fatalf("company repository failure = %v, want repository error", err)
	}

	invalidCurrencyReader := companyScopeReader{
		Reader: &mockMasterReaderForPosting{compActive: true, periodOpen: true},
		company: masterdata.Company{
			ID: companyID, Active: true, FunctionalCurrency: "XXX",
		},
	}
	invalidCurrencyPipeline := NewPostingPipeline(invalidCurrencyReader, executor,
		WithPolicyEngine(testPostingPolicyEngine(t, companyID, "USD")))
	if _, err := invalidCurrencyPipeline.PostEntry(context.Background(), entry, PostOptions{ActorID: "poster", IdempotencyKey: "test-idemp-key"}); !errors.Is(err, types.ErrInvalidCurrency) {
		t.Fatalf("invalid functional currency = %v, want invalid currency", err)
	}
}
