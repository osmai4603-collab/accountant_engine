package reporting_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/osmai4603-collab/accounting_engine/internal/kernal/masterdata"
	"github.com/osmai4603-collab/accounting_engine/internal/kernal/policy"
	financialtypes "github.com/osmai4603-collab/accounting_engine/internal/kernal/types"
	"github.com/osmai4603-collab/accounting_engine/internal/kernal/valuation"
	"github.com/osmai4603-collab/accounting_engine/internal/orbitals/postingcompat"
	"github.com/osmai4603-collab/accounting_engine/internal/orbitals/reporting"
	"github.com/osmai4603-collab/accounting_engine/internal/orbitals/subledgers"
	"github.com/shopspring/decimal"
)

type testRepository struct {
	companyID uuid.UUID
	accounts  []masterdata.Account
	entries   []posting.JournalEntry
}

func (r *testRepository) ListAccounts(context.Context) ([]masterdata.Account, error) {
	return r.accounts, nil
}

func (r *testRepository) ListPostedEntries(_ context.Context, start, end time.Time) ([]posting.JournalEntry, error) {
	return r.entriesInRange(start, end), nil
}

func (r *testRepository) entriesInRange(start, end time.Time) []posting.JournalEntry {
	result := make([]posting.JournalEntry, 0, len(r.entries))
	for _, entry := range r.entries {
		date := entry.EntryDate
		if date.IsZero() {
			date = entry.Date
		}
		if !start.IsZero() && date.Before(start) || !end.IsZero() && date.After(end) {
			continue
		}
		result = append(result, entry)
	}
	return result
}

func (r *testRepository) ListAccountsForCompanyContext(_ context.Context, companyID uuid.UUID) ([]masterdata.Account, error) {
	return r.accountsForCompany(companyID), nil
}

func (r *testRepository) ListAccountsForCompanyAsOfContext(ctx context.Context, companyID uuid.UUID, _ time.Time) ([]masterdata.Account, error) {
	return r.ListAccountsForCompanyContext(ctx, companyID)
}

func (r *testRepository) accountsForCompany(companyID uuid.UUID) []masterdata.Account {
	result := make([]masterdata.Account, 0, len(r.accounts))
	for _, account := range r.accounts {
		if account.CompanyID.UUID() == companyID {
			result = append(result, account)
		}
	}
	return result
}

func (r *testRepository) ListEntriesForCompanyContext(_ context.Context, companyID uuid.UUID, start, end time.Time) ([]posting.JournalEntry, error) {
	result := make([]posting.JournalEntry, 0)
	for _, entry := range r.entriesInRange(start, end) {
		if entry.CompanyID == companyID {
			result = append(result, entry)
		}
	}
	return result, nil
}

func (r *testRepository) ListFixedAssets(context.Context) ([]subledgers.FixedAsset, error) {
	return nil, nil
}

func (r *testRepository) ListFixedAssetsForCompanyContext(context.Context, uuid.UUID) ([]subledgers.FixedAsset, error) {
	return nil, nil
}

func (r *testRepository) ListOpenLines(context.Context, string) ([]subledgers.OpenLine, error) {
	return nil, nil
}

func (r *testRepository) ListOpenLinesForCompanyContext(context.Context, uuid.UUID, string) ([]subledgers.OpenLine, error) {
	return nil, nil
}

func (r *testRepository) GetRateForCompany(_ context.Context, _ uuid.UUID, from, to string, _ time.Time) (decimal.Decimal, error) {
	if from == to {
		return decimal.NewFromInt(1), nil
	}
	return decimal.Zero, valuation.ErrRateNotFound
}

func (r *testRepository) GetRateOfTypeForCompany(_ context.Context, _ uuid.UUID, _ valuation.RateType, from, to string, _ time.Time) (decimal.Decimal, error) {
	return r.GetRateForCompany(context.Background(), uuid.Nil, from, to, time.Time{})
}

func (r *testRepository) GetRate(_ context.Context, _ valuation.RateType, from, to string, _ time.Time) (decimal.Decimal, error) {
	return r.GetRateForCompany(context.Background(), uuid.Nil, from, to, time.Time{})
}

func (r *testRepository) GetFiscalYearForCompanyContext(context.Context, uuid.UUID, uuid.UUID) (masterdata.FiscalYear, error) {
	return masterdata.FiscalYear{}, errors.New("fiscal year not found")
}

func (r *testRepository) UpdatePeriodStateForCompanyContext(context.Context, uuid.UUID, uuid.UUID, masterdata.PeriodState) error {
	return nil
}

func (r *testRepository) UpdateFiscalYearStateForCompanyContext(context.Context, uuid.UUID, uuid.UUID, masterdata.FiscalYearState) error {
	return nil
}

func (r *testRepository) GetEntryByIDForCompanyContext(_ context.Context, companyID, entryID uuid.UUID) (posting.JournalEntry, error) {
	for _, entry := range r.entries {
		if entry.ID == entryID && entry.CompanyID == companyID {
			return entry, nil
		}
	}
	return posting.JournalEntry{}, errors.New("entry not found")
}

func testAccount(companyID, id uuid.UUID, code, name string, accountType masterdata.AccountType, subClass string) masterdata.Account {
	account := masterdata.Account{
		CompanyID:         financialtypes.CompanyID(companyID),
		ID:                financialtypes.AccountID(id),
		Code:              code,
		Name:              name,
		Type:              accountType,
		NormalBalance:     masterdata.DefaultNormalBalance(accountType),
		SubClassification: subClass,
	}
	switch accountType {
	case masterdata.AccountTypeAsset:
		if subClass == masterdata.SubClassCashAndEquivalents {
			account.CashFlowActivity = "cash_equivalents"
			account.IsCashEquivalent = true
		} else {
			account.CashFlowActivity = "operating"
		}
		account.IFRSCategory = "balance_sheet_perpetual"
	case masterdata.AccountTypeLiability:
		account.CashFlowActivity = "financing"
		account.IFRSCategory = "balance_sheet_perpetual"
	case masterdata.AccountTypeEquity:
		account.CashFlowActivity = "financing"
		account.IFRSCategory = "balance_sheet_perpetual"
	case masterdata.AccountTypeIncome:
		account.CashFlowActivity = "operating"
		account.IFRSCategory = "operating"
	case masterdata.AccountTypeExpense:
		account.CashFlowActivity = "operating"
		account.IFRSCategory = "operating"
	}
	return account
}

func testLine(accountID uuid.UUID, debit, credit string) posting.JournalEntryLine {
	zero, _ := financialtypes.Zero("USD")
	debitMoney, _ := financialtypes.NewMoneyFromString(debit, "USD")
	creditMoney, _ := financialtypes.NewMoneyFromString(credit, "USD")
	if debit == "0" {
		debitMoney = zero
	}
	if credit == "0" {
		creditMoney = zero
	}
	return posting.JournalEntryLine{
		AccountID: accountID,
		Debit:     debitMoney,
		Credit:    creditMoney,
	}
}

func testEntry(t *testing.T, companyID uuid.UUID, date time.Time, lines ...posting.JournalEntryLine) posting.JournalEntry {
	t.Helper()
	entry, err := posting.NewJournalEntry(uuid.New(), date, "test", lines)
	if err != nil {
		t.Fatal(err)
	}
	entry.CompanyID = companyID
	entry.Status = posting.EntryPosted
	for i := range entry.Lines {
		entry.Lines[i].CompanyID = companyID
	}
	return entry
}

func balancedReportingFixture(t *testing.T) (*testRepository, uuid.UUID, time.Time, time.Time) {
	t.Helper()
	companyID := uuid.New()
	cashID := uuid.New()
	capitalID := uuid.New()
	revenueID := uuid.New()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 12, 31, 23, 59, 59, 0, time.UTC)
	repository := &testRepository{
		companyID: companyID,
		accounts: []masterdata.Account{
			testAccount(companyID, cashID, "1000", "Cash", masterdata.AccountTypeAsset, masterdata.SubClassCashAndEquivalents),
			testAccount(companyID, capitalID, "3000", "Capital", masterdata.AccountTypeEquity, masterdata.SubClassShareCapital),
			testAccount(companyID, revenueID, "4000", "Revenue", masterdata.AccountTypeIncome, masterdata.SubClassOperatingRevenue),
		},
	}
	repository.entries = []posting.JournalEntry{
		testEntry(t, companyID, start.Add(-24*time.Hour),
			testLine(cashID, "100", "0"),
			testLine(capitalID, "0", "100"),
		),
		testEntry(t, companyID, start.Add(time.Hour),
			testLine(cashID, "20", "0"),
			testLine(revenueID, "0", "20"),
		),
	}
	return repository, companyID, start, end
}

func TestReportingStatementsBalanceFromPostedEntries(t *testing.T) {
	repository, companyID, start, end := balancedReportingFixture(t)
	engine := reporting.NewEngine(repository)

	trialBalance, err := engine.TrialBalance.GenerateForCompany(companyID, start, end, "USD")
	if err != nil {
		t.Fatal(err)
	}
	if !trialBalance.Balanced ||
		!trialBalance.DebitTotal.Equal(decimal.NewFromInt(120)) ||
		!trialBalance.CreditTotal.Equal(decimal.NewFromInt(120)) {
		t.Fatalf("unbalanced trial balance: %+v", trialBalance)
	}

	sheet, err := engine.BalanceSheet.GenerateForCompany(companyID, end, "USD")
	if err != nil {
		t.Fatal(err)
	}
	if !sheet.Balanced || !sheet.TotalAssets.Equal(decimal.NewFromInt(120)) ||
		!sheet.TotalLiabilitiesAndEquity.Equal(decimal.NewFromInt(120)) {
		t.Fatalf("unbalanced balance sheet: %+v", sheet)
	}

	cashFlow, err := engine.CashFlow.GenerateForCompany(companyID, start, end, "USD")
	if err != nil {
		t.Fatal(err)
	}
	if !cashFlow.Reconciled ||
		!cashFlow.OpeningCash.Equal(decimal.NewFromInt(100)) ||
		!cashFlow.ClosingCash.Equal(decimal.NewFromInt(120)) ||
		!cashFlow.NetChangeInCash.Equal(decimal.NewFromInt(20)) {
		t.Fatalf("unreconciled cash flow: %+v", cashFlow)
	}
}

type testPolicyUnit struct {
	policy policy.CurrencyPolicy
}

func (u testPolicyUnit) GetCurrencyPolicyAsOf(context.Context, uuid.UUID, time.Time) (policy.CurrencyPolicy, error) {
	return u.policy, nil
}

func TestReportingCurrencyDefaultsToEffectivePresentationCurrency(t *testing.T) {
	ctx := context.Background()
	effective := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	engine := reporting.NewEngine(nil)
	engine.SetPolicyUnit(testPolicyUnit{policy: policy.CurrencyPolicy{
		FunctionalCurrency:   "SAR",
		PresentationCurrency: "USD",
		AllowedCurrencies:    []string{"SAR", "USD", "EUR"},
		EffectiveDate:        effective,
	}})

	got, err := engine.ResolvePresentationCurrency(ctx, uuid.New(), effective, "")
	if err != nil || got != "USD" {
		t.Fatalf("default currency = %q, error %v", got, err)
	}
	got, err = engine.ResolvePresentationCurrency(ctx, uuid.New(), effective, "EUR")
	if err != nil || got != "EUR" {
		t.Fatalf("explicit currency = %q, error %v", got, err)
	}
	if _, err := engine.ResolvePresentationCurrency(ctx, uuid.New(), effective, "JPY"); !errors.Is(err, policy.ErrCurrencyNotAllowed) {
		t.Fatalf("unauthorized translation error = %v", err)
	}
}
