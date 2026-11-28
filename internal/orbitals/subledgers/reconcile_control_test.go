package subledgers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	financialtypes "github.com/osmai4603-collab/accounting_engine/internal/kernel/types"
	apitypes "github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/shopspring/decimal"
)

type parityRepo struct {
	lines  []OpenLine
	assets []FixedAsset
	items  []BankItem
	err    error
}

func (r *parityRepo) ListOpenLinesForCompanyContext(
	context.Context, uuid.UUID, string,
) ([]OpenLine, error) {
	return r.lines, r.err
}

func (r *parityRepo) ListFixedAssetsForCompanyContext(context.Context, uuid.UUID) ([]FixedAsset, error) {
	return r.assets, r.err
}

func (r *parityRepo) ListBankItemsForCompany(context.Context, uuid.UUID, string) ([]BankItem, error) {
	return r.items, r.err
}

type fixedBalanceReader struct {
	balance decimal.Decimal
	err     error
	calls   int
}

func (r *fixedBalanceReader) GetAccountBalance(
	_ context.Context, _ apitypes.CompanyID, _ apitypes.AccountID, _ time.Time, _ string,
) (decimal.Decimal, error) {
	r.calls++
	return r.balance, r.err
}

func parityMoney(t *testing.T, value, currency string) financialtypes.Money {
	t.Helper()
	m, err := financialtypes.NewMoneyFromString(value, currency)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestAssertSubledgerParityARBalancesExactly(t *testing.T) {
	companyID, controlID := uuid.New(), uuid.New()
	partner := uuid.New()
	asOf := time.Now().UTC().Truncate(24 * time.Hour)
	repo := &parityRepo{lines: []OpenLine{
		{
			CompanyID: companyID, ControlAccountID: controlID, Kind: LineReceivable,
			PartnerID: &partner, ResidualAmount: parityMoney(t, "100.00", "USD"),
			FunctionalResidualAmount: parityMoney(t, "100.00", "USD"),
			PostingDate:              asOf.Add(-time.Hour),
		},
		{
			CompanyID: companyID, ControlAccountID: controlID, Kind: LineReceivable,
			PartnerID: &partner, ResidualAmount: parityMoney(t, "25.50", "USD"),
			FunctionalResidualAmount: parityMoney(t, "25.50", "USD"),
			PostingDate:              asOf.Add(-time.Hour),
		},
		{
			CompanyID: companyID, ControlAccountID: controlID, Kind: LineReceivable,
			PartnerID: &partner, ResidualAmount: parityMoney(t, "999.00", "USD"),
			FunctionalResidualAmount: parityMoney(t, "999.00", "USD"),
			PostingDate:              asOf.Add(48 * time.Hour), // excluded by as-of cutoff
		},
	}}
	balances := &fixedBalanceReader{balance: decimal.RequireFromString("125.50")}

	evidence, err := AssertSubledgerParity(
		context.Background(), repo, balances, companyID, controlID, LineReceivable, "USD", asOf,
	)
	if err != nil {
		t.Fatalf("expected exact parity, got %v", err)
	}
	if !evidence.Reconciled || !evidence.Difference.IsZero() {
		t.Fatalf("expected zero difference, got %+v", evidence)
	}
	if !evidence.SubledgerBalance.Equal(decimal.RequireFromString("125.50")) ||
		!evidence.GLBalance.Equal(decimal.RequireFromString("125.50")) {
		t.Fatalf("unexpected evidence amounts: %+v", evidence)
	}
	if balances.calls != 1 {
		t.Fatalf("expected one GL read, got %d", balances.calls)
	}
}

func TestAssertSubledgerParityAPCreditNormalNegatesGL(t *testing.T) {
	companyID, controlID := uuid.New(), uuid.New()
	partner := uuid.New()
	asOf := time.Now().UTC().Truncate(24 * time.Hour)
	repo := &parityRepo{lines: []OpenLine{{
		CompanyID: companyID, ControlAccountID: controlID, Kind: LinePayable,
		PartnerID: &partner, ResidualAmount: parityMoney(t, "70.00", "USD"),
		FunctionalResidualAmount: parityMoney(t, "70.00", "USD"),
		PostingDate:              asOf.Add(-time.Hour),
	}}}
	// BalanceReader reports debit-minus-credit, so a credit-normal AP balance is negative.
	balances := &fixedBalanceReader{balance: decimal.RequireFromString("-70.00")}

	evidence, err := AssertSubledgerParity(
		context.Background(), repo, balances, companyID, controlID, LinePayable, "USD", asOf,
	)
	if err != nil || !evidence.Reconciled {
		t.Fatalf("expected AP parity after credit-normal negation, got %+v err=%v", evidence, err)
	}
	if !evidence.GLBalance.Equal(decimal.RequireFromString("70.00")) {
		t.Fatalf("expected normalized GL balance 70.00, got %s", evidence.GLBalance)
	}
}

func TestAssertSubledgerParityRejectsDrift(t *testing.T) {
	companyID, controlID := uuid.New(), uuid.New()
	partner := uuid.New()
	repo := &parityRepo{lines: []OpenLine{{
		CompanyID: companyID, ControlAccountID: controlID, Kind: LineReceivable,
		PartnerID: &partner, ResidualAmount: parityMoney(t, "100.00", "USD"),
		FunctionalResidualAmount: parityMoney(t, "100.00", "USD"),
	}}}
	balances := &fixedBalanceReader{balance: decimal.RequireFromString("99.99")}

	evidence, err := AssertSubledgerParity(
		context.Background(), repo, balances, companyID, controlID, LineReceivable, "USD", time.Now(),
	)
	if !errors.Is(err, ErrSubledgerParityMismatch) {
		t.Fatalf("expected parity mismatch, got %v", err)
	}
	if evidence.Reconciled || !evidence.Difference.Equal(decimal.RequireFromString("0.01")) {
		t.Fatalf("expected 0.01 drift evidence, got %+v", evidence)
	}
}

func TestAssertSubledgerParityFixedAssetsAndTreasury(t *testing.T) {
	companyID, assetControl, bankControl := uuid.New(), uuid.New(), uuid.New()
	asOf := time.Now().UTC()
	repo := &parityRepo{
		assets: []FixedAsset{{
			CompanyID: companyID, AssetAccountID: assetControl,
			CurrentBookValue: parityMoney(t, "500.00", "USD"),
		}},
		items: []BankItem{{
			CompanyID: companyID, AccountID: bankControl, Status: BankItemPending,
			Amount: parityMoney(t, "40.00", "USD"), Date: asOf.Add(-time.Hour),
		}},
	}
	balances := &fixedBalanceReader{balance: decimal.RequireFromString("500.00")}

	if _, err := AssertSubledgerParity(
		context.Background(), repo, balances, companyID, assetControl, LineFixedAsset, "USD", asOf,
	); err != nil {
		t.Fatalf("fixed asset parity: %v", err)
	}
	balances.balance = decimal.RequireFromString("40.00")
	if _, err := AssertSubledgerParity(
		context.Background(), repo, balances, companyID, bankControl, LineBank, "USD", asOf,
	); err != nil {
		t.Fatalf("treasury parity: %v", err)
	}
}

func TestAssertSubledgerParityRequiresPorts(t *testing.T) {
	if _, err := AssertSubledgerParity(
		context.Background(), nil, nil, uuid.New(), uuid.New(), LineReceivable, "USD", time.Now(),
	); err == nil {
		t.Fatal("expected error when dependencies are missing")
	}
}
