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
	"github.com/shopspring/decimal"
)

func covMoney(t *testing.T, value, currency string) financialtypes.Money {
	t.Helper()
	amount, err := financialtypes.NewMoneyFromString(value, currency)
	if err != nil {
		t.Fatal(err)
	}
	return amount
}

func covMoneyRaw(value, currency string) financialtypes.Money {
	amount, err := financialtypes.NewMoneyFromString(value, currency)
	if err != nil {
		panic(err)
	}
	return amount
}

func postingEntryForControl(
	companyID, accountID uuid.UUID, debit, credit string, status posting.EntryStatus,
) posting.JournalEntry {
	entry := posting.JournalEntry{
		ID: uuid.New(), CompanyID: companyID, Status: status,
		Lines: []posting.JournalEntryLine{{
			ID: uuid.New(), CompanyID: companyID, AccountID: accountID,
			Debit: covMoneyRaw(debit, "USD"), Credit: covMoneyRaw(credit, "USD"),
			FunctionalDebit: covMoneyRaw(debit, "USD"), FunctionalCredit: covMoneyRaw(credit, "USD"),
		}},
	}
	return entry
}

func covUUIDPtr(id uuid.UUID) *uuid.UUID { return &id }

func covDate(day int) time.Time {
	return time.Date(2026, 1, day, 12, 0, 0, 0, time.UTC)
}

func covAccount(companyID, accountID uuid.UUID) masterdata.Account {
	return masterdata.Account{
		ID:               financialtypes.AccountID(accountID),
		CompanyID:        financialtypes.CompanyID(companyID),
		Type:             masterdata.AccountTypeAsset,
		NormalBalance:    masterdata.NormalBalanceDebit,
		IsControlAccount: true,
		IsReconcilable:   true,
	}
}

// covLineReader implements both the legacy and contextual open-line reader
// interfaces used by the reconciliation engine.
type covLineReader struct {
	lines         []OpenLine
	err           error
	legacyCalls   int
	contextCalls  int
	disableLegacy bool
}

func (r *covLineReader) ListOpenLinesForCompany(companyID uuid.UUID, partnerID string) ([]OpenLine, error) {
	r.legacyCalls++
	if r.disableLegacy {
		return nil, ErrNotFound
	}
	return r.lines, r.err
}

func (r *covLineReader) ListOpenLinesForCompanyContext(
	_ context.Context, companyID uuid.UUID, partnerID string,
) ([]OpenLine, error) {
	r.contextCalls++
	return r.lines, r.err
}

type covLegacyLineReader struct {
	lines []OpenLine
	err   error
}

func (r *covLegacyLineReader) ListOpenLinesForCompany(uuid.UUID, string) ([]OpenLine, error) {
	return r.lines, r.err
}

// covGLRepo deliberately omits GetAccountBalanceAsOf so tests can exercise
// compatibility paths that do not support dated balances.
type covGLRepo struct {
	balance      financialtypes.Money
	balanceErr   error
	accounts     []masterdata.Account
	accountsErr  error
	assets       []FixedAsset
	assetsErr    error
	bankItems    []BankItem
	bankItemsErr error
	balanceCalls int
	accountCalls int
	assetsCalls  int
	bankCalls    int
}

func (r *covGLRepo) GetAccountBalance(context.Context, uuid.UUID, uuid.UUID) (financialtypes.Money, error) {
	r.balanceCalls++
	return r.balance, r.balanceErr
}

func (r *covGLRepo) ListAccountsForCompanyContext(
	context.Context, uuid.UUID,
) ([]masterdata.Account, error) {
	r.accountCalls++
	return r.accounts, r.accountsErr
}

func (r *covGLRepo) ListFixedAssetsForCompanyContext(context.Context, uuid.UUID) ([]FixedAsset, error) {
	r.assetsCalls++
	return r.assets, r.assetsErr
}

func (r *covGLRepo) ListBankItemsForCompany(context.Context, uuid.UUID, string) ([]BankItem, error) {
	r.bankCalls++
	return r.bankItems, r.bankItemsErr
}

type covAsOfGLRepo struct {
	*covGLRepo
	asOfBalance financialtypes.Money
	asOfErr     error
	asOfCalls   int
	seenAsOf    time.Time
}

func (r *covAsOfGLRepo) GetAccountBalanceAsOf(
	_ context.Context, _ uuid.UUID, _ uuid.UUID, asOf time.Time,
) (financialtypes.Money, error) {
	r.asOfCalls++
	r.seenAsOf = asOf
	return r.asOfBalance, r.asOfErr
}

type covPlainGLRepo struct {
	balance financialtypes.Money
}

func (r *covPlainGLRepo) GetAccountBalance(context.Context, uuid.UUID, uuid.UUID) (financialtypes.Money, error) {
	return r.balance, nil
}

func covReconciliationLine(
	companyID, controlID uuid.UUID, kind LineKind, transaction, functional string,
) OpenLine {
	return OpenLine{
		CompanyID:                companyID,
		ID:                       uuid.New(),
		ControlAccountID:         controlID,
		Kind:                     kind,
		PartnerID:                covUUIDPtr(uuid.New()),
		OriginalAmount:           covMoneyRaw(transaction, "USD"),
		ResidualAmount:           covMoneyRaw(transaction, "USD"),
		FunctionalOriginalAmount: covMoneyRaw(functional, "USD"),
		FunctionalResidualAmount: covMoneyRaw(functional, "USD"),
		PostingDate:              covDate(2),
		DocumentDate:             covDate(2),
	}
}

func TestCovLedgerReconciliationCurrentBalance(t *testing.T) {
	companyID, arID, apID := uuid.New(), uuid.New(), uuid.New()
	ar := &covLineReader{lines: []OpenLine{
		covReconciliationLine(companyID, arID, LineReceivable, "100", "100"),
	}}
	ap := &covLineReader{lines: []OpenLine{
		covReconciliationLine(companyID, apID, LinePayable, "40", "40"),
	}}
	gl := &covGLRepo{balance: covMoney(t, "100", "USD")}
	engine := NewLedgerReconciliationEngine(ar, ap, gl)

	if report, err := engine.ReconcileARControl(context.Background(), companyID, arID); err != nil || report != nil {
		t.Fatalf("balanced AR: report=%+v err=%v", report, err)
	}
	if ar.contextCalls != 1 || gl.balanceCalls != 1 {
		t.Fatalf("unexpected reader calls: ar=%d gl=%d", ar.contextCalls, gl.balanceCalls)
	}

	gl.balance = covMoney(t, "99", "USD")
	report, err := engine.ReconcileARControl(context.Background(), companyID, arID)
	if err == nil || report == nil {
		t.Fatalf("variance must produce discrepancy and error: report=%+v err=%v", report, err)
	}
	if !report.Difference.Equal(decimal.NewFromInt(1)) || report.Currency != "USD" {
		t.Fatalf("unexpected AR discrepancy: %+v", report)
	}

	gl.balance = covMoney(t, "40", "USD")
	if discrepancy, err := engine.ReconcileAPControl(context.Background(), companyID, apID); err != nil || discrepancy != nil {
		t.Fatalf("balanced AP: discrepancy=%+v err=%v", discrepancy, err)
	}
	if _, err := engine.ReconcileControl(context.Background(), companyID, arID, LineKind("other")); !errors.Is(err, ErrInvalidReconciliationKind) {
		t.Fatalf("invalid kind error = %v", err)
	}
}

func TestCovLedgerReconciliationConfigurationAndValidationErrors(t *testing.T) {
	companyID, accountID := uuid.New(), uuid.New()
	ctx := context.Background()

	if _, err := (&LedgerReconciliationEngine{}).ReconcileARControl(ctx, companyID, accountID); !errors.Is(err, ErrReconciliationNotConfigured) {
		t.Fatalf("nil GL error = %v", err)
	}
	glErr := errors.New("gl failed")
	gl := &covGLRepo{balanceErr: glErr}
	if _, err := NewLedgerReconciliationEngine(nil, nil, gl).ReconcileARControl(ctx, companyID, accountID); !errors.Is(err, glErr) {
		t.Fatalf("GL read error = %v", err)
	}
	gl = &covGLRepo{balance: covMoney(t, "10", "USD")}
	if _, err := NewLedgerReconciliationEngine(nil, nil, gl).ReconcileAPControl(ctx, companyID, accountID); !errors.Is(err, ErrReconciliationNotConfigured) {
		t.Fatalf("missing AP reader error = %v", err)
	}

	if _, err := NewLedgerReconciliationEngine(nil, nil, gl).reconcileControlWithGL(ctx, uuid.Nil, accountID, LineReceivable, time.Time{}, covMoney(t, "1", "USD")); !errors.Is(err, ErrInvalidOpenLine) {
		t.Fatalf("nil company error = %v", err)
	}
	if _, err := NewLedgerReconciliationEngine(nil, nil, gl).reconcileControlWithGL(ctx, companyID, uuid.Nil, LineReceivable, time.Time{}, covMoney(t, "1", "USD")); !errors.Is(err, ErrInvalidOpenLine) {
		t.Fatalf("nil account error = %v", err)
	}
	if _, err := NewLedgerReconciliationEngine(nil, nil, gl).reconcileControlWithGL(ctx, companyID, accountID, LineReceivable, time.Time{}, financialtypes.Money{}); !errors.Is(err, ErrInvalidOpenLine) {
		t.Fatalf("empty GL currency error = %v", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := NewLedgerReconciliationEngine(nil, nil, gl).reconcileControlWithGL(cancelled, companyID, accountID, LineReceivable, time.Time{}, covMoney(t, "1", "USD")); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled context error = %v", err)
	}

	lineErr := errors.New("line listing failed")
	reader := &covLineReader{err: lineErr}
	engine := NewLedgerReconciliationEngine(reader, reader, gl)
	if _, err := engine.ReconcileARControl(ctx, companyID, accountID); !errors.Is(err, lineErr) {
		t.Fatalf("line read error = %v", err)
	}

	reader = &covLineReader{lines: []OpenLine{{
		CompanyID: uuid.New(), ControlAccountID: accountID, Kind: LineReceivable,
		ResidualAmount: covMoney(t, "1", "USD"), FunctionalResidualAmount: covMoney(t, "1", "USD"),
	}}}
	if _, err := NewLedgerReconciliationEngine(reader, nil, gl).ReconcileARControl(ctx, companyID, accountID); !errors.Is(err, ErrCompanyMismatch) {
		t.Fatalf("company mismatch error = %v", err)
	}

	reader = &covLineReader{lines: []OpenLine{{
		CompanyID: companyID, ControlAccountID: accountID, Kind: LineReceivable,
		ResidualAmount: covMoney(t, "1", "EUR"), FunctionalResidualAmount: covMoney(t, "1", "EUR"),
	}}}
	if _, err := NewLedgerReconciliationEngine(reader, nil, gl).ReconcileARControl(ctx, companyID, accountID); !errors.Is(err, ErrReconciliationFunctionalAmountMissing) {
		t.Fatalf("functional currency mismatch error = %v", err)
	}

	legacy := &covLegacyLineReader{lines: []OpenLine{
		covReconciliationLine(companyID, accountID, LineReceivable, "5", "5"),
	}}
	if discrepancy, err := NewLedgerReconciliationEngine(legacy, nil, &covGLRepo{balance: covMoney(t, "5", "USD")}).
		ReconcileARControl(ctx, companyID, accountID); err != nil || discrepancy != nil {
		t.Fatalf("legacy line reader path: discrepancy=%+v err=%v", discrepancy, err)
	}
}

func TestCovOpenLineBalanceAsOfVariants(t *testing.T) {
	companyID, accountID := uuid.New(), uuid.New()
	line := covReconciliationLine(companyID, accountID, LineReceivable, "100", "100")

	transaction, functional, include, err := openLineBalanceAsOf(line, time.Time{})
	if err != nil || !include || !transaction.Equal(covMoney(t, "100", "USD")) ||
		!functional.Equal(covMoney(t, "100", "USD")) {
		t.Fatalf("current balance: %v %v %v %v", transaction, functional, include, err)
	}

	closed := line
	closed.Closed = true
	transaction, functional, include, err = openLineBalanceAsOf(closed, time.Time{})
	if err != nil || !include || !transaction.Amount().IsZero() || !functional.Amount().IsZero() {
		t.Fatalf("closed balance: %v %v %v %v", transaction, functional, include, err)
	}

	transaction, functional, include, err = openLineBalanceAsOf(line, covDate(3))
	if err != nil || !include || !transaction.Equal(covMoney(t, "100", "USD")) || !functional.Equal(covMoney(t, "100", "USD")) {
		t.Fatalf("dated balance: %v %v %v %v", transaction, functional, include, err)
	}

	future := line
	future.PostingDate = covDate(10)
	_, _, include, err = openLineBalanceAsOf(future, covDate(3))
	if err != nil || include {
		t.Fatalf("future line should be excluded: include=%v err=%v", include, err)
	}

	noDate := line
	noDate.PostingDate, noDate.DocumentDate = time.Time{}, time.Time{}
	if _, _, _, err := openLineBalanceAsOf(noDate, covDate(3)); !errors.Is(err, ErrReconciliationHistoryUnavailable) {
		t.Fatalf("missing date error = %v", err)
	}

	partial := line
	partial.ResidualAmount = covMoney(t, "90", "USD")
	if _, _, _, err := openLineBalanceAsOf(partial, covDate(3)); !errors.Is(err, ErrReconciliationHistoryUnavailable) {
		t.Fatalf("partial line without history error = %v", err)
	}

	matched := line
	matched.ResidualAmount = covMoney(t, "90", "USD")
	matched.FunctionalResidualAmount = covMoney(t, "90", "USD")
	matched.MatchHistory = []OpenLineMatch{{
		MatchedAt:        covDate(8), // matched after cutoff: restore it to the as-of balance
		Amount:           covMoney(t, "10", "USD"),
		FunctionalAmount: covMoney(t, "10", "USD"),
	}}
	transaction, functional, include, err = openLineBalanceAsOf(matched, covDate(3))
	if err != nil || !include || !transaction.Equal(covMoney(t, "100", "USD")) ||
		!functional.Equal(covMoney(t, "100", "USD")) {
		t.Fatalf("unmatched later date balance: %v %v %v %v", transaction, functional, include, err)
	}

	reversed := line
	reversed.ResidualAmount = covMoney(t, "100", "USD")
	reversed.FunctionalResidualAmount = covMoney(t, "100", "USD")
	reversed.MatchHistory = []OpenLineMatch{{
		MatchedAt:        covDate(2),
		ReversedAt:       covDate(8), // reversal after cutoff restores the matched amount
		Amount:           covMoney(t, "10", "USD"),
		FunctionalAmount: covMoney(t, "10", "USD"),
	}}
	transaction, functional, include, err = openLineBalanceAsOf(reversed, covDate(3))
	if err != nil || !include || !transaction.Equal(covMoney(t, "90", "USD")) ||
		!functional.Equal(covMoney(t, "90", "USD")) {
		t.Fatalf("reversed history balance: %v %v %v %v", transaction, functional, include, err)
	}

	reversedLater := line
	reversedLater.MatchHistory = []OpenLineMatch{{
		MatchedAt:        covDate(2),
		ReversedAt:       covDate(8),
		Amount:           covMoney(t, "10", "USD"),
		FunctionalAmount: covMoney(t, "10", "USD"),
	}}
	transaction, _, include, err = openLineBalanceAsOf(reversedLater, covDate(3))
	if err != nil || !include || !transaction.Equal(covMoney(t, "90", "USD")) {
		t.Fatalf("reverse after cutoff: %v %v %v", transaction, include, err)
	}

	badHistory := line
	badHistory.MatchHistory = []OpenLineMatch{{Amount: covMoney(t, "10", "USD")}}
	if _, _, _, err := openLineBalanceAsOf(badHistory, covDate(3)); !errors.Is(err, ErrReconciliationHistoryUnavailable) {
		t.Fatalf("missing matched-at error = %v", err)
	}

	badFunctional := line
	badFunctional.MatchHistory = []OpenLineMatch{{
		MatchedAt: covDate(2), Amount: covMoney(t, "10", "EUR"),
	}}
	if _, _, _, err := openLineBalanceAsOf(badFunctional, covDate(3)); !errors.Is(err, ErrReconciliationHistoryUnavailable) {
		t.Fatalf("mismatched match functional error = %v", err)
	}

	overstated := line
	overstated.MatchHistory = []OpenLineMatch{{
		MatchedAt:        covDate(4),
		Amount:           covMoney(t, "10", "USD"),
		FunctionalAmount: covMoney(t, "10", "USD"),
	}}
	if _, _, _, err := openLineBalanceAsOf(overstated, covDate(3)); !errors.Is(err, ErrReconciliationHistoryUnavailable) {
		t.Fatalf("overstated historical amount error = %v", err)
	}

	proportional := line
	proportional.OriginalAmount = covMoney(t, "100", "EUR")
	proportional.ResidualAmount = covMoney(t, "50", "EUR")
	proportional.FunctionalResidualAmount = financialtypes.Money{}
	proportional.FunctionalOriginalAmount = covMoney(t, "200", "USD")
	_, functional, _, err = openLineBalanceAsOf(proportional, time.Time{})
	if err != nil || !functional.Equal(covMoney(t, "50", "EUR")) {
		t.Fatalf("transaction currency functional fallback = %v err=%v", functional, err)
	}

	sameCurrency := line
	sameCurrency.FunctionalResidualAmount = financialtypes.Money{}
	_, functional, _, err = openLineBalanceAsOf(sameCurrency, time.Time{})
	if err != nil || !functional.Equal(covMoney(t, "100", "USD")) {
		t.Fatalf("same currency functional fallback = %v err=%v", functional, err)
	}
}

func TestCovLedgerReconciliationAsOf(t *testing.T) {
	companyID, accountID := uuid.New(), uuid.New()
	reader := &covLineReader{lines: []OpenLine{
		covReconciliationLine(companyID, accountID, LineReceivable, "100", "100"),
	}}
	asOf := covDate(5)
	gl := &covAsOfGLRepo{covGLRepo: &covGLRepo{balance: covMoney(t, "100", "USD")}, asOfBalance: covMoney(t, "100", "USD")}
	engine := NewLedgerReconciliationEngine(reader, nil, gl)

	if _, err := engine.ReconcileARControlAsOf(context.Background(), companyID, accountID, time.Time{}); !errors.Is(err, ErrReconciliationHistoryUnavailable) {
		t.Fatalf("zero as-of error = %v", err)
	}
	if _, err := NewLedgerReconciliationEngine(reader, nil, &covGLRepo{balance: covMoney(t, "100", "USD")}).
		ReconcileARControlAsOf(context.Background(), companyID, accountID, asOf); !errors.Is(err, ErrAsOfReconciliationNotSupported) {
		t.Fatalf("unsupported as-of error = %v", err)
	}

	glErr := errors.New("as-of failed")
	gl.asOfErr = glErr
	if _, err := engine.ReconcileARControlAsOf(context.Background(), companyID, accountID, asOf); !errors.Is(err, glErr) {
		t.Fatalf("as-of GL error = %v", err)
	}
	gl.asOfErr = nil
	gl.asOfBalance = covMoney(t, "99", "USD")

	report, err := engine.ReconcileARControlAsOf(context.Background(), companyID, accountID, asOf)
	if err != nil || report.Difference.IsZero() || report.AsOfDate.Day() != 5 {
		t.Fatalf("as-of variance report: %+v err=%v", report, err)
	}
	if gl.asOfCalls != 2 || !gl.seenAsOf.Equal(reconciliationCutoff(asOf)) {
		t.Fatalf("as-of reader calls=%d cutoff=%v", gl.asOfCalls, gl.seenAsOf)
	}

	gl.balance = covMoney(t, "100", "USD")
	gl.asOfBalance = covMoney(t, "100", "USD")
	if _, err := engine.ReconcileAPControlAsOf(context.Background(), companyID, accountID, asOf); !errors.Is(err, ErrReconciliationNotConfigured) {
		t.Fatalf("missing AP reader error = %v", err)
	}
	engine.apRepo = reader
	if _, err := engine.ReconcileControlAsOf(context.Background(), companyID, accountID, LinePayable, asOf); err != nil {
		t.Fatalf("payable as-of with AR lines: %v", err)
	}
}

func TestCovReconcileAllControls(t *testing.T) {
	companyID := uuid.New()
	accountA, accountB, accountC := uuid.New(), uuid.New(), uuid.New()
	lines := []OpenLine{
		covReconciliationLine(companyID, accountA, LineReceivable, "10", "10"),
		covReconciliationLine(companyID, accountB, LineReceivable, "20", "20"),
		covReconciliationLine(companyID, uuid.New(), LinePayable, "30", "30"),
	}
	reader := &covLineReader{lines: lines}
	gl := &covAsOfGLRepo{
		covGLRepo: &covGLRepo{
			accounts: []masterdata.Account{covAccount(companyID, accountC)},
			balance:  covMoney(t, "0", "USD"),
		},
		asOfBalance: covMoney(t, "0", "USD"),
	}
	engine := NewLedgerReconciliationEngine(reader, reader, gl)

	reports, err := engine.ReconcileAllARControls(context.Background(), companyID, covDate(5))
	if err != nil || len(reports) != 3 {
		t.Fatalf("all AR reports: %d err=%v", len(reports), err)
	}
	for i := 1; i < len(reports); i++ {
		if reports[i-1].ControlAccountID.String() >= reports[i].ControlAccountID.String() {
			t.Fatalf("reports not sorted by account: %+v", reports)
		}
	}
	seen := map[uuid.UUID]bool{}
	for _, report := range reports {
		seen[report.ControlAccountID] = true
	}
	if !seen[accountA] || !seen[accountB] || !seen[accountC] {
		t.Fatalf("missing control accounts from reports: %+v", reports)
	}

	if _, err := NewLedgerReconciliationEngine(nil, nil, gl).ReconcileAllAPControls(context.Background(), companyID, covDate(5)); !errors.Is(err, ErrReconciliationNotConfigured) {
		t.Fatalf("missing reader error = %v", err)
	}

	badCompany := &covLineReader{lines: []OpenLine{{CompanyID: uuid.New(), Kind: LineReceivable}}}
	if _, err := NewLedgerReconciliationEngine(badCompany, nil, gl).
		ReconcileAllARControls(context.Background(), companyID, covDate(5)); !errors.Is(err, ErrCompanyMismatch) {
		t.Fatalf("cross-company line error = %v", err)
	}

	gl.accountsErr = errors.New("accounts failed")
	if _, err := engine.ReconcileAllARControls(context.Background(), companyID, covDate(5)); err == nil {
		t.Fatal("expected account listing failure")
	}
	gl.accountsErr = nil
	gl.accounts = []masterdata.Account{covAccount(uuid.New(), accountC)}
	if _, err := engine.ReconcileAllARControls(context.Background(), companyID, covDate(5)); !errors.Is(err, ErrCompanyMismatch) {
		t.Fatalf("cross-company account error = %v", err)
	}
	gl.accounts = nil
	gl.asOfErr = errors.New("as-of failed")
	if _, err := engine.ReconcileAllARControls(context.Background(), companyID, covDate(5)); err == nil {
		t.Fatal("expected as-of failure propagation")
	}
}

func TestCovReconcileFixedAssetsControl(t *testing.T) {
	companyID, costID, depID, impairmentID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	asOf := covDate(5)
	asset := FixedAsset{
		CompanyID:                      companyID,
		ID:                             uuid.New(),
		AssetAccountID:                 costID,
		AccumulatedAccountID:           depID,
		AccumulatedImpairmentAccountID: impairmentID,
		InServiceDate:                  covDate(1),
		HistoricalCost:                 covMoney(t, "1000", "USD"),
		RevaluationAdjustment:          covMoney(t, "50", "USD"),
		AccumulatedDepreciation:        covMoney(t, "100", "USD"),
		AccumulatedImpairment:          covMoney(t, "20", "USD"),
	}
	gl := &covAsOfGLRepo{
		covGLRepo:   &covGLRepo{assets: []FixedAsset{asset}},
		asOfBalance: covMoney(t, "1050", "USD"),
	}
	engine := NewLedgerReconciliationEngine(nil, nil, gl)

	reports, err := engine.ReconcileFixedAssetsControl(context.Background(), companyID, asOf)
	if err != nil || len(reports) != 3 {
		t.Fatalf("asset reports: %d err=%v", len(reports), err)
	}
	foundVariance := false
	for _, report := range reports {
		if report.HasVariance() {
			foundVariance = true
		}
		if report.ControlAccountID == costID && report.Domain != "FIXED_ASSETS_COST" {
			t.Fatalf("unexpected cost domain: %+v", report)
		}
	}
	if !foundVariance {
		t.Fatalf("variance expected: %+v", reports)
	}

	component := AssetComponent{
		ID:                      uuid.New(),
		AccumulatedAccountID:    depID,
		AccumulatedDepreciation: covMoney(t, "15", "USD"),
	}
	componentAsset := asset
	componentAsset.AssetAccountID = uuid.New()
	componentAsset.AccumulatedAccountID = uuid.New()
	componentAsset.AccumulatedImpairment = financialtypes.Money{}
	componentAsset.Components = []AssetComponent{component}
	gl.assets = []FixedAsset{componentAsset}
	if _, err := engine.ReconcileFixedAssetsControl(context.Background(), companyID, asOf); err != nil {
		t.Fatalf("component balances: %v", err)
	}

	gl.assets = nil
	if _, err := engine.ReconcileFixedAssetsControl(context.Background(), uuid.Nil, asOf); !errors.Is(err, ErrInvalidOpenLine) {
		t.Fatalf("nil company error = %v", err)
	}
	if _, err := engine.ReconcileFixedAssetsControl(context.Background(), companyID, time.Time{}); !errors.Is(err, ErrInvalidOpenLine) {
		t.Fatalf("zero as-of error = %v", err)
	}
	if _, err := NewLedgerReconciliationEngine(nil, nil, &covPlainGLRepo{balance: covMoney(t, "1", "USD")}).
		ReconcileFixedAssetsControl(context.Background(), companyID, asOf); !errors.Is(err, ErrReconciliationNotConfigured) {
		t.Fatalf("missing asset reader error = %v", err)
	}
	gl.assetsErr = errors.New("assets failed")
	if _, err := engine.ReconcileFixedAssetsControl(context.Background(), companyID, asOf); err == nil {
		t.Fatal("expected asset listing failure")
	}
	gl.assetsErr = nil

	invalidCases := []struct {
		name  string
		asset FixedAsset
		want  error
	}{
		{"company", func() FixedAsset { a := asset; a.CompanyID = uuid.New(); return a }(), ErrCompanyMismatch},
		{"no service date", func() FixedAsset { a := asset; a.InServiceDate = time.Time{}; return a }(), ErrReconciliationHistoryUnavailable},
		{"future depreciation", func() FixedAsset { a := asset; a.LastDepreciationDate = covDate(9); return a }(), ErrReconciliationHistoryUnavailable},
		{"future revaluation", func() FixedAsset { a := asset; a.LastRevaluationDate = covDate(9); return a }(), ErrReconciliationHistoryUnavailable},
		{"nil asset account", func() FixedAsset { a := asset; a.AssetAccountID = uuid.Nil; return a }(), ErrInvalidAsset},
		{"nil depreciation account", func() FixedAsset { a := asset; a.AccumulatedAccountID = uuid.Nil; return a }(), ErrInvalidAsset},
		{"nil impairment account", func() FixedAsset { a := asset; a.AccumulatedImpairmentAccountID = uuid.Nil; return a }(), ErrInvalidAsset},
		{"currency mismatch", func() FixedAsset {
			a := asset
			a.HistoricalCost = covMoney(t, "1000", "EUR")
			a.RevaluationAdjustment = covMoney(t, "50", "EUR")
			return a
		}(), ErrReconciliationFunctionalAmountMissing},
		{"depreciation currency mismatch", func() FixedAsset {
			a := asset
			a.AccumulatedDepreciation = covMoney(t, "100", "EUR")
			return a
		}(), ErrReconciliationFunctionalAmountMissing},
		{"component currency mismatch", func() FixedAsset {
			a := asset
			a.AccumulatedDepreciation = financialtypes.Money{}
			a.Components = []AssetComponent{{AccumulatedAccountID: depID, AccumulatedDepreciation: covMoney(t, "1", "EUR")}}
			return a
		}(), ErrReconciliationFunctionalAmountMissing},
	}
	for _, tc := range invalidCases {
		t.Run(tc.name, func(t *testing.T) {
			gl.assets = []FixedAsset{tc.asset}
			if _, err := engine.ReconcileFixedAssetsControl(context.Background(), companyID, asOf); !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
		})
	}

	disposed := asset
	disposedAt := covDate(4)
	disposed.DisposedAt = &disposedAt
	gl.assets = []FixedAsset{disposed}
	if reports, err := engine.ReconcileFixedAssetsControl(context.Background(), companyID, asOf); err != nil || len(reports) != 0 {
		t.Fatalf("disposed asset should be excluded: %d err=%v", len(reports), err)
	}

	gl.assets = []FixedAsset{func() FixedAsset {
		a := asset
		a.AssetAccountID = depID // cost and depreciation posted to one account
		return a
	}()}
	if _, err := engine.ReconcileFixedAssetsControl(context.Background(), companyID, asOf); err == nil {
		t.Fatal("expected mixed-domain asset balance error")
	}

	if err := addAssetBalance(map[uuid.UUID]assetBalanceTotals{}, uuid.New(), "A", decimal.NewFromInt(1)); err != nil {
		t.Fatal(err)
	}
}

func TestCovReconcileTreasuryControls(t *testing.T) {
	companyID, bankID := uuid.New(), uuid.New()
	asOf := covDate(5)
	gl := &covAsOfGLRepo{
		covGLRepo: &covGLRepo{
			bankItems: []BankItem{
				{CompanyID: companyID, AccountID: bankID, Amount: covMoney(t, "100", "USD"), Date: covDate(1)},
				{CompanyID: companyID, AccountID: bankID, Amount: covMoney(t, "20", "USD"), Date: covDate(1),
					JournalEntryID: uuid.New(), Status: BankItemPending},
			},
		},
		asOfBalance: covMoney(t, "120", "USD"),
	}
	engine := NewLedgerReconciliationEngine(nil, nil, gl)

	report, err := engine.ReconcileTreasuryControl(context.Background(), companyID, bankID, asOf)
	if err != nil || report.HasVariance() {
		t.Fatalf("balanced treasury: %+v err=%v", report, err)
	}
	if report.Domain != "TREASURY" || gl.bankCalls != 1 || gl.asOfCalls != 1 {
		t.Fatalf("unexpected treasury report: %+v bankCalls=%d asOfCalls=%d", report, gl.bankCalls, gl.asOfCalls)
	}

	if _, err := engine.ReconcileTreasuryControl(context.Background(), uuid.Nil, bankID, asOf); !errors.Is(err, ErrInvalidOpenLine) {
		t.Fatalf("nil company error = %v", err)
	}
	if _, err := engine.ReconcileTreasuryControl(context.Background(), companyID, uuid.Nil, asOf); !errors.Is(err, ErrInvalidOpenLine) {
		t.Fatalf("nil bank account error = %v", err)
	}
	if _, err := engine.ReconcileTreasuryControl(context.Background(), companyID, bankID, time.Time{}); !errors.Is(err, ErrInvalidOpenLine) {
		t.Fatalf("zero as-of error = %v", err)
	}
	if _, err := NewLedgerReconciliationEngine(nil, nil, &covPlainGLRepo{balance: covMoney(t, "1", "USD")}).
		ReconcileTreasuryControl(context.Background(), companyID, bankID, asOf); !errors.Is(err, ErrReconciliationNotConfigured) {
		t.Fatalf("missing bank reader error = %v", err)
	}

	gl.bankItemsErr = errors.New("bank items failed")
	if _, err := engine.ReconcileTreasuryControl(context.Background(), companyID, bankID, asOf); err == nil {
		t.Fatal("expected bank item failure")
	}
	gl.bankItemsErr = nil
	gl.asOfErr = errors.New("balance failed")
	if _, err := engine.ReconcileTreasuryControl(context.Background(), companyID, bankID, asOf); err == nil {
		t.Fatal("expected GL as-of failure")
	}
	gl.asOfErr = nil

	badCases := []struct {
		name  string
		items []BankItem
		want  error
	}{
		{"company mismatch", []BankItem{{CompanyID: uuid.New(), AccountID: bankID, Amount: covMoney(t, "1", "USD"), Date: covDate(1)}}, ErrCompanyMismatch},
		{"missing date", []BankItem{{CompanyID: companyID, AccountID: bankID, Amount: covMoney(t, "1", "USD")}}, ErrReconciliationHistoryUnavailable},
		{"currency mismatch", []BankItem{{CompanyID: companyID, AccountID: bankID, Amount: covMoney(t, "1", "EUR"), Date: covDate(1)}}, ErrReconciliationFunctionalAmountMissing},
		{"matched without timestamp", []BankItem{{CompanyID: companyID, AccountID: bankID, Amount: covMoney(t, "1", "USD"), Date: covDate(1),
			JournalEntryID: uuid.New(), Status: BankItemMatched}}, ErrReconciliationHistoryUnavailable},
	}
	for _, tc := range badCases {
		t.Run(tc.name, func(t *testing.T) {
			gl.bankItems = tc.items
			if _, err := engine.ReconcileTreasuryControl(context.Background(), companyID, bankID, asOf); !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
		})
	}

	gl.bankItems = []BankItem{
		{CompanyID: companyID, AccountID: bankID, Amount: covMoney(t, "999", "USD"), Date: covDate(9)},
		{CompanyID: companyID, AccountID: bankID, Amount: covMoney(t, "10", "USD"), Date: covDate(1),
			JournalEntryID: uuid.New(), Status: BankItemMatched, MatchedAt: covDate(8)},
	}
	if report, err := engine.ReconcileTreasuryControl(context.Background(), companyID, bankID, asOf); err != nil || !report.SubledgerBalance.Equal(decimal.NewFromInt(10)) {
		t.Fatalf("matched and future items: %+v err=%v", report, err)
	}

	gl.accounts = []masterdata.Account{
		{ID: financialtypes.AccountID(bankID), CompanyID: financialtypes.CompanyID(companyID), SubClassification: masterdata.SubClassCashAndEquivalents},
		{ID: financialtypes.AccountID(uuid.New()), CompanyID: financialtypes.CompanyID(companyID), SubClassification: "expense"},
	}
	gl.bankItems = nil
	reports, err := engine.ReconcileAllTreasuryControls(context.Background(), companyID, asOf)
	if err != nil || len(reports) != 1 || reports[0].ControlAccountID != bankID {
		t.Fatalf("all treasury: %+v err=%v", reports, err)
	}
	gl.accounts = nil
	if _, err := engine.ReconcileAllTreasuryControls(context.Background(), uuid.Nil, asOf); !errors.Is(err, ErrInvalidOpenLine) {
		t.Fatalf("nil company error = %v", err)
	}
	if _, err := NewLedgerReconciliationEngine(nil, nil, &covPlainGLRepo{balance: covMoney(t, "1", "USD")}).
		ReconcileAllTreasuryControls(context.Background(), companyID, asOf); !errors.Is(err, ErrReconciliationNotConfigured) {
		t.Fatalf("missing account reader error = %v", err)
	}
	gl.accountsErr = errors.New("accounts failed")
	if _, err := engine.ReconcileAllTreasuryControls(context.Background(), companyID, asOf); err == nil {
		t.Fatal("expected account listing failure")
	}
	gl.accountsErr = nil
	gl.accounts = []masterdata.Account{{CompanyID: financialtypes.CompanyID(uuid.New())}}
	if _, err := engine.ReconcileAllTreasuryControls(context.Background(), companyID, asOf); !errors.Is(err, ErrCompanyMismatch) {
		t.Fatalf("cross company account error = %v", err)
	}
	gl.accounts = []masterdata.Account{{ID: financialtypes.AccountID(bankID), CompanyID: financialtypes.CompanyID(companyID), SubClassification: subledgerSubClassBank}}
	gl.asOfErr = errors.New("balance failed")
	if _, err := engine.ReconcileAllTreasuryControls(context.Background(), companyID, asOf); err == nil {
		t.Fatal("expected propagated balance failure")
	}
}

func TestCovControlAccountBalanceFromEntries(t *testing.T) {
	companyID, accountID := uuid.New(), uuid.New()
	account := covAccount(companyID, accountID)
	if _, err := ControlAccountBalanceFromEntries(masterdata.Account{}, nil, "USD"); !errors.Is(err, ErrInvalidOpenLine) {
		t.Fatalf("invalid account error = %v", err)
	}
	entry := postingEntryForControl(companyID, accountID, "100", "40", posting.EntryPosted)
	if balance, err := ControlAccountBalanceFromEntries(account, []posting.JournalEntry{entry}, "USD"); err != nil ||
		!balance.Equal(covMoney(t, "60", "USD")) {
		t.Fatalf("posted balance = %v err=%v", balance, err)
	}
	draft := postingEntryForControl(companyID, accountID, "100", "40", posting.EntryDraft)
	if balance, err := ControlAccountBalanceFromEntries(account, []posting.JournalEntry{draft}, "USD"); err != nil || !balance.Amount().IsZero() {
		t.Fatalf("drafts must be skipped: %v err=%v", balance, err)
	}
	creditNormal := account
	creditNormal.NormalBalance = masterdata.NormalBalanceCredit
	if balance, err := ControlAccountBalanceFromEntries(creditNormal, []posting.JournalEntry{entry}, "USD"); err != nil ||
		!balance.Equal(covMoney(t, "-60", "USD")) {
		t.Fatalf("credit-normal balance = %v err=%v", balance, err)
	}

	wrongCompany := postingEntryForControl(uuid.New(), accountID, "100", "40", posting.EntryPosted)
	if _, err := ControlAccountBalanceFromEntries(account, []posting.JournalEntry{wrongCompany}, "USD"); !errors.Is(err, ErrCompanyMismatch) {
		t.Fatalf("company mismatch error = %v", err)
	}
	wrongCurrency := postingEntryForControl(companyID, accountID, "100", "40", posting.EntryPosted)
	wrongCurrency.Lines[0].FunctionalDebit = covMoney(t, "100", "EUR")
	if _, err := ControlAccountBalanceFromEntries(account, []posting.JournalEntry{wrongCurrency}, "USD"); !errors.Is(err, ErrControlBalanceCurrencyMismatch) {
		t.Fatalf("currency mismatch error = %v", err)
	}
	noCurrency := postingEntryForControl(companyID, accountID, "100", "40", posting.EntryPosted)
	noCurrency.Lines[0].FunctionalDebit = financialtypes.Money{}
	noCurrency.Lines[0].FunctionalCredit = financialtypes.Money{}
	noCurrency.Lines[0].Debit = financialtypes.Money{}
	noCurrency.Lines[0].Credit = financialtypes.Money{}
	if _, err := ControlAccountBalanceFromEntries(account, []posting.JournalEntry{noCurrency}, "USD"); !errors.Is(err, ErrControlBalanceCurrencyMismatch) {
		t.Fatalf("missing currency error = %v", err)
	}
	creditOnly := postingEntryForControl(companyID, accountID, "0", "40", posting.EntryPosted)
	if balance, err := ControlAccountBalanceFromEntries(account, []posting.JournalEntry{creditOnly}, "USD"); err != nil ||
		!balance.Equal(covMoney(t, "-40", "USD")) {
		t.Fatalf("credit-only balance = %v err=%v", balance, err)
	}
}

func TestCovReconciliationExceptions(t *testing.T) {
	companyID, accountID := uuid.New(), uuid.New()
	asOf := covDate(5)
	report := DiscrepancyReport{
		CompanyID: companyID, Domain: "AR", ControlAccountID: accountID, AsOfDate: asOf,
		FunctionalCurr: "USD", SubledgerBalance: decimal.NewFromInt(101), GLBalance: decimal.NewFromInt(100),
		Difference: decimal.NewFromInt(1), DiscoveredAt: asOf,
	}
	if !report.HasVariance() {
		t.Fatal("expected variance")
	}
	report.Difference = decimal.Zero
	if report.HasVariance() {
		t.Fatal("zero variance expected")
	}
	report.Difference = decimal.NewFromInt(1)

	exception := NewReconciliationException(report)
	if err := ValidateReconciliationException(exception); err != nil {
		t.Fatalf("generated exception invalid: %v", err)
	}
	if exception.Status != ReconciliationExceptionUnmatched ||
		exception.ActorID != reconciliationDaemonActorID() || !exception.CreatedAt.Equal(asOf) {
		t.Fatalf("unexpected generated exception: %+v", exception)
	}
	if err := ValidateReconciliationOutcome(report, &exception); err != nil {
		t.Fatalf("valid outcome: %v", err)
	}
	clean := report
	clean.Difference = decimal.Zero
	if err := ValidateReconciliationOutcome(clean, nil); err != nil {
		t.Fatalf("clean outcome: %v", err)
	}
	if err := ValidateReconciliationOutcome(report, nil); !errors.Is(err, ErrInvalidOpenLine) {
		t.Fatalf("missing exception error = %v", err)
	}
	if err := ValidateReconciliationOutcome(clean, &exception); !errors.Is(err, ErrInvalidOpenLine) {
		t.Fatalf("unexpected exception error = %v", err)
	}
	if err := ValidateReconciliationOutcome(DiscrepancyReport{}, nil); !errors.Is(err, ErrInvalidOpenLine) {
		t.Fatalf("invalid report error = %v", err)
	}

	invalid := exception
	invalid.ID = uuid.Nil
	if err := ValidateReconciliationException(invalid); !errors.Is(err, ErrInvalidReconciliationException) {
		t.Fatalf("invalid exception error = %v", err)
	}
	mismatch := exception
	mismatch.CompanyID = uuid.New()
	if err := ValidateReconciliationOutcome(report, &mismatch); !errors.Is(err, ErrInvalidOpenLine) {
		t.Fatalf("outcome mismatch error = %v", err)
	}

	transitions := []struct {
		from, to ReconciliationExceptionStatus
		want     bool
	}{
		{ReconciliationExceptionUnmatched, ReconciliationExceptionMatching, true},
		{ReconciliationExceptionUnmatched, ReconciliationExceptionDisputed, true},
		{ReconciliationExceptionUnmatched, ReconciliationExceptionResolved, false},
		{ReconciliationExceptionMatching, ReconciliationExceptionUnmatched, true},
		{ReconciliationExceptionMatching, ReconciliationExceptionDisputed, true},
		{ReconciliationExceptionMatching, ReconciliationExceptionResolved, true},
		{ReconciliationExceptionMatching, ReconciliationExceptionMatching, false},
		{ReconciliationExceptionDisputed, ReconciliationExceptionUnmatched, true},
		{ReconciliationExceptionDisputed, ReconciliationExceptionMatching, true},
		{ReconciliationExceptionDisputed, ReconciliationExceptionResolved, true},
		{ReconciliationExceptionDisputed, ReconciliationExceptionDisputed, false},
		{ReconciliationExceptionResolved, ReconciliationExceptionUnmatched, false},
		{ReconciliationExceptionStatus("other"), ReconciliationExceptionMatching, false},
	}
	for _, tc := range transitions {
		if got := ValidReconciliationExceptionTransition(tc.from, tc.to); got != tc.want {
			t.Fatalf("transition %q -> %q = %v, want %v", tc.from, tc.to, got, tc.want)
		}
	}
}

func TestCovEventFormulaNodes(t *testing.T) {
	companyID := uuid.New()
	event := EconomicEvent{
		CompanyID: companyID, IdempotencyKey: "formula", SourceSystem: "test",
		EventType: "sale", Amount: covMoney(t, "100", "USD"),
		Values: map[string]financialtypes.Money{
			"fee": covMoney(t, "2", "USD"),
			"tax": covMoney(t, "0", "USD"),
		},
		Attributes: map[string]string{"tier": "gold"},
	}
	cases := []struct {
		formula string
		want    string
		wantErr bool
	}{
		{"amount", "100", false},
		{"fee", "2", false},
		{"1", "1", false},
		{"1.25", "1.25", false},
		{"(amount)", "100", false},
		{"+amount", "100", false},
		{"-fee", "-2", false},
		{"amount + fee", "102", false},
		{"amount - fee", "98", false},
		{"amount * fee", "200", false},
		{"amount / fee", "50", false},
		{"missing", "", true},
		{"amount / tax", "", true},
		{"amount % 2", "", true},
		{`"text"`, "", true},
		{"amount > 2", "", true},
		{"amount +", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.formula, func(t *testing.T) {
			value, err := evaluateAmountFormula(tc.formula, event)
			if tc.wantErr {
				if !errors.Is(err, ErrInvalidEvent) {
					t.Fatalf("error = %v, want invalid event", err)
				}
				return
			}
			if err != nil || !value.Equal(decimal.RequireFromString(tc.want)) {
				t.Fatalf("value = %s err=%v, want %s", value, err, tc.want)
			}
		})
	}
	if !ruleMatchesEvent(EventAccountingRule{CompanyID: companyID, EventType: "sale"}, event) {
		t.Fatal("rule with no conditions must match")
	}
	if ruleMatchesEvent(EventAccountingRule{CompanyID: uuid.New(), EventType: "sale"}, event) ||
		ruleMatchesEvent(EventAccountingRule{CompanyID: companyID, EventType: "other"}, event) ||
		ruleMatchesEvent(EventAccountingRule{
			CompanyID: companyID, EventType: "sale", Conditions: map[string]string{"tier": "silver"},
		}, event) {
		t.Fatal("mismatched rule must not match")
	}
	if !ruleMatchesEvent(EventAccountingRule{
		CompanyID: companyID, EventType: "sale", Conditions: map[string]string{"tier": "gold"},
	}, event) {
		t.Fatal("matching conditions must match")
	}
}

func TestCovEventFormulaLineTranslation(t *testing.T) {
	companyID, debitID, creditID, journalID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	event := EconomicEvent{
		CompanyID: companyID, EventType: "sale", Amount: covMoney(t, "100", "USD"),
		Values: map[string]financialtypes.Money{"fee": covMoney(t, "2", "USD")},
	}
	valid := EventAccountingRule{
		CompanyID: companyID, EventType: "sale", RuleName: "default", JournalID: journalID,
		Accounts: map[string]uuid.UUID{"cash": debitID},
		Lines: []EventLineTemplate{
			{AccountLookupKey: "cash", Side: EventLineDebit, AmountFormula: "amount"},
			{AccountID: creditID, Side: EventLineCredit, AmountFormula: "amount - fee"},
		},
	}
	lines, err := translateEventLines(valid, event)
	if err != nil || len(lines) != 2 || !lines[0].Debit.Equal(covMoney(t, "100", "USD")) ||
		!lines[1].Credit.Equal(covMoney(t, "98", "USD")) {
		t.Fatalf("translated lines = %+v err=%v", lines, err)
	}
	zeroCurrency := event
	zeroCurrency.Amount = financialtypes.Money{}
	if _, err := translateEventLines(valid, zeroCurrency); err == nil {
		t.Fatal("zero currency must fail")
	}
	cases := []struct {
		name  string
		rule  EventAccountingRule
		event EconomicEvent
	}{
		{"invalid rule", EventAccountingRule{}, event},
		{"missing account", func() EventAccountingRule {
			r := valid
			r.Lines[0].AccountLookupKey = "missing"
			return r
		}(), event},
		{"unknown formula", func() EventAccountingRule {
			r := valid
			r.Lines[0].AmountFormula = "unknown"
			return r
		}(), event},
		{"negative amount", func() EventAccountingRule {
			r := valid
			r.Lines[0].AmountFormula = "-amount"
			return r
		}(), event},
		{"zero amount", func() EventAccountingRule {
			r := valid
			r.Lines[0].AmountFormula = "0"
			return r
		}(), event},
		{"rounded to zero", func() EventAccountingRule {
			r := valid
			r.Lines[0].AmountFormula = "0.0001"
			return r
		}(), event},
		{"bad side", func() EventAccountingRule {
			r := valid
			r.Lines[0].Side = EventLineSide("other")
			return r
		}(), event},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := translateEventLines(tc.rule, tc.event); !errors.Is(err, ErrMappingNotFound) &&
				!errors.Is(err, ErrInvalidEvent) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestCovEconomicEventPayloadHashVariants(t *testing.T) {
	companyID, journalID, partnerID := uuid.New(), uuid.New(), uuid.New()
	event := EconomicEvent{
		CompanyID: companyID, IdempotencyKey: "hash", SourceSystem: "test",
		EventType: "sale", Date: covDate(2), Amount: covMoney(t, "100", "USD"),
		Values:     map[string]financialtypes.Money{"b": covMoney(t, "2", "USD"), "a": covMoney(t, "1", "USD")},
		Attributes: map[string]string{"z": "last", "a": "first"},
		PartnerID:  &partnerID, PayloadJSON: []byte(`{"x":1}`), EventVersion: 2,
		Reference: "INV-1",
	}
	entry := posting.JournalEntry{
		CompanyID: companyID, JournalID: journalID,
		Lines: []posting.JournalEntryLine{
			{CompanyID: companyID, AccountID: uuid.New(), Debit: covMoney(t, "100", "USD"), Credit: covMoney(t, "0", "USD")},
			{CompanyID: companyID, AccountID: uuid.New(), Debit: covMoney(t, "0", "USD"), Credit: covMoney(t, "100", "USD")},
		},
	}
	first, err := EconomicEventPayloadHash(event, entry)
	if err != nil || len(first) != 64 {
		t.Fatalf("hash = %q err=%v", first, err)
	}
	reordered := event
	reordered.Values = map[string]financialtypes.Money{"a": covMoney(t, "1", "USD"), "b": covMoney(t, "2", "USD")}
	reordered.Attributes = map[string]string{"a": "first", "z": "last"}
	second, err := EconomicEventPayloadHash(reordered, entry)
	if err != nil || second != first {
		t.Fatalf("map ordering changed hash: %q vs %q err=%v", first, second, err)
	}
	changed := event
	changed.Reference = "INV-2"
	if third, err := EconomicEventPayloadHash(changed, entry); err != nil || third == first {
		t.Fatalf("payload change did not change hash: %q err=%v", third, err)
	}
	if _, err := EconomicEventPayloadHash(EconomicEvent{}, entry); !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("invalid event error = %v", err)
	}
	wrongCompany := entry
	wrongCompany.CompanyID = uuid.New()
	if _, err := EconomicEventPayloadHash(event, wrongCompany); !errors.Is(err, posting.ErrInvalidEntry) {
		t.Fatalf("company mismatch error = %v", err)
	}
	noJournal := entry
	noJournal.JournalID = uuid.Nil
	if _, err := EconomicEventPayloadHash(event, noJournal); !errors.Is(err, posting.ErrInvalidEntry) {
		t.Fatalf("missing journal error = %v", err)
	}
}

func TestCovModelValidationsAndCompatibilityHelpers(t *testing.T) {
	validEvent := EconomicEvent{
		CompanyID: uuid.New(), IdempotencyKey: "key", SourceSystem: "src", EventType: "sale",
		Amount: covMoney(t, "10", "USD"),
	}
	cases := []struct {
		name   string
		mutate func(*EconomicEvent)
	}{
		{"long key", func(e *EconomicEvent) { e.IdempotencyKey = strings.Repeat("x", 256) }},
		{"long source", func(e *EconomicEvent) { e.SourceSystem = strings.Repeat("x", 256) }},
		{"long external id", func(e *EconomicEvent) { e.ExternalID = strings.Repeat("x", 256) }},
		{"long event type", func(e *EconomicEvent) { e.EventType = strings.Repeat("x", 129) }},
		{"negative event amount", func(e *EconomicEvent) { e.Amount = covMoney(t, "-1", "USD") }},
		{"bad payload hash", func(e *EconomicEvent) { e.PayloadHash = "short" }},
		{"negative version", func(e *EconomicEvent) { e.EventVersion = -1 }},
		{"nil generated entry", func(e *EconomicEvent) { e.GeneratedEntryID = covUUIDPtr(uuid.Nil) }},
		{"bad status", func(e *EconomicEvent) { e.Status = EventStatus("other") }},
		{"nameless value", func(e *EconomicEvent) { e.Values = map[string]financialtypes.Money{" ": covMoney(t, "1", "USD")} }},
		{"foreign value", func(e *EconomicEvent) { e.Values = map[string]financialtypes.Money{"v": covMoney(t, "1", "EUR")} }},
		{"negative value", func(e *EconomicEvent) { e.Values = map[string]financialtypes.Money{"v": covMoney(t, "-1", "USD")} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			candidate := validEvent
			tc.mutate(&candidate)
			if err := candidate.Validate(); !errors.Is(err, ErrInvalidEvent) {
				t.Fatalf("error = %v", err)
			}
		})
	}
	if err := validEvent.Validate(); err != nil {
		t.Fatalf("valid event: %v", err)
	}

	rule := EventAccountingRule{
		CompanyID: uuid.New(), EventType: "sale", RuleName: "r", JournalID: uuid.New(),
		Lines: []EventLineTemplate{{AccountID: uuid.New(), Side: EventLineDebit, AmountFormula: "amount"}},
	}
	if err := rule.Validate(); !errors.Is(err, ErrMappingNotFound) {
		t.Fatalf("too few lines error = %v", err)
	}
	rule.Lines = append(rule.Lines, EventLineTemplate{Side: EventLineCredit, AmountFormula: "amount", AccountLookupKey: "missing"})
	if err := rule.Validate(); !errors.Is(err, ErrMappingNotFound) {
		t.Fatalf("missing account lookup error = %v", err)
	}
	rule.Lines[1].AccountLookupKey = "cash"
	rule.Accounts = map[string]uuid.UUID{"cash": uuid.New()}
	if err := rule.Validate(); err != nil {
		t.Fatalf("valid rule: %v", err)
	}
	rule.Lines[1].Side = EventLineSide("other")
	if err := rule.Validate(); !errors.Is(err, ErrMappingNotFound) {
		t.Fatalf("bad side error = %v", err)
	}
	if linePartner(OpenLine{}) != "" {
		t.Fatal("nil partner must produce empty id")
	}

	companyID := uuid.New()
	account := BankAccount{
		ID: uuid.New(), CompanyID: companyID, BankName: "bank", IBAN: "TR1", Currency: "USD",
		GLAccountID: uuid.New(), ClearingAccountID: uuid.New(),
	}
	if err := account.ValidateForCompany(companyID); err != nil {
		t.Fatalf("valid bank account: %v", err)
	}
	bankCases := []struct {
		name   string
		mutate func(*BankAccount)
		want   error
	}{
		{"company mismatch", func(a *BankAccount) { a.CompanyID = uuid.New() }, ErrCompanyMismatch},
		{"same accounts", func(a *BankAccount) { a.ClearingAccountID = a.GLAccountID }, ErrInvalidBankAccount},
		{"blank bank name", func(a *BankAccount) { a.BankName = " " }, ErrInvalidBankAccount},
		{"unknown currency", func(a *BankAccount) { a.Currency = "ZZZ" }, ErrInvalidBankAccount},
	}
	for _, tc := range bankCases {
		t.Run(tc.name, func(t *testing.T) {
			candidate := account
			tc.mutate(&candidate)
			if err := candidate.ValidateForCompany(companyID); !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
		})
	}

	if cloned := cloneStringMap(nil); cloned != nil {
		t.Fatal("nil map must clone to nil")
	}
	conditions := map[string]string{"a": "1"}
	clonedConditions := cloneStringMap(conditions)
	clonedConditions["a"] = "2"
	if conditions["a"] != "1" {
		t.Fatal("string map was not cloned")
	}
	cloneRule := rule.Clone()
	cloneRule.Conditions = map[string]string{"x": "1"}
	cloneRule.Lines[0].AmountFormula = "1"
	cloneRule.Accounts["cash"] = uuid.New()
	if rule.Conditions != nil || rule.Lines[0].AmountFormula != "amount" || rule.Accounts["cash"] == cloneRule.Accounts["cash"] {
		t.Fatal("event rule was not cloned")
	}
	if _, err := subledgerRealizedFXDifference(decimal.NewFromInt(10), decimal.Zero, decimal.NewFromInt(2), "USD"); !errors.Is(err, errSubledgerInvalidRate) {
		t.Fatalf("zero rate error = %v", err)
	}
	if _, err := subledgerRealizedFXDifference(decimal.NewFromInt(10), decimal.NewFromInt(1), decimal.NewFromInt(-2), "USD"); !errors.Is(err, errSubledgerInvalidRate) {
		t.Fatalf("negative rate error = %v", err)
	}
	if value, err := subledgerRealizedFXDifference(decimal.NewFromInt(10), decimal.NewFromInt(1), decimal.NewFromInt(2), "USD"); err != nil ||
		!value.Equal(covMoney(t, "10", "USD")) {
		t.Fatalf("FX difference = %v err=%v", value, err)
	}
	entry := postingEntryForControl(companyID, uuid.New(), "100", "0", posting.EntryPosted)
	entry.DocumentReference = "REF"
	entry.EntryDate, entry.Date = covDate(2), covDate(2)
	if !FixedAssetEntryMatches(entry, entry) {
		t.Fatal("identical entries must match")
	}
	changedLine := entry
	changedLine.Lines = append([]posting.JournalEntryLine(nil), entry.Lines...)
	changedLine.Lines[0].AccountID = uuid.New()
	if FixedAssetEntryMatches(changedLine, entry) {
		t.Fatal("changed line must not match")
	}
}

type covEventRepo struct {
	*MemoryRepository
	saveEventErr         error
	saveDraftErr         error
	identityFailCall     int
	identityOverrideFrom int
	identityEvent        EconomicEvent
	identityFound        bool
	byIDErr              error
	byIDEvent            EconomicEvent
	updateErr            error
	listRulesErr         error
	saveRuleErr          error
	identityCalls        int
}

func (r *covEventRepo) SaveEvent(ctx context.Context, event EconomicEvent) error {
	if r.saveEventErr != nil {
		return r.saveEventErr
	}
	return r.MemoryRepository.SaveEvent(ctx, event)
}

func (r *covEventRepo) SaveEventAndDraft(ctx context.Context, event EconomicEvent, entry posting.JournalEntry) error {
	if r.saveDraftErr != nil {
		return r.saveDraftErr
	}
	return r.MemoryRepository.SaveEventAndDraft(ctx, event, entry)
}

func (r *covEventRepo) GetEventByIdentity(ctx context.Context, companyID uuid.UUID, sourceSystem, key string) (EconomicEvent, bool, error) {
	r.identityCalls++
	if r.identityFailCall == r.identityCalls {
		return EconomicEvent{}, false, r.identityFailCallError()
	}
	if r.identityOverrideFrom > 0 && r.identityCalls >= r.identityOverrideFrom {
		return r.identityEvent, r.identityFound, nil
	}
	return r.MemoryRepository.GetEventByIdentity(ctx, companyID, sourceSystem, key)
}

func (r *covEventRepo) identityFailCallError() error {
	if r.saveDraftErr != nil && errors.Is(r.saveDraftErr, ErrAlreadyExists) {
		return errors.New("identity lookup failed")
	}
	return errors.New("identity lookup failed")
}

func (r *covEventRepo) GetEventByID(ctx context.Context, companyID, eventID uuid.UUID) (EconomicEvent, error) {
	if r.byIDErr != nil {
		return EconomicEvent{}, r.byIDErr
	}
	if r.byIDEvent.EventID != uuid.Nil {
		return r.byIDEvent, nil
	}
	return r.MemoryRepository.GetEventByID(ctx, companyID, eventID)
}

func (r *covEventRepo) UpdateEventStatus(ctx context.Context, event EconomicEvent) error {
	if r.updateErr != nil {
		return r.updateErr
	}
	return r.MemoryRepository.UpdateEventStatus(ctx, event)
}

func (r *covEventRepo) ListEventAccountingRulesForCompany(ctx context.Context, companyID uuid.UUID, eventType string) ([]EventAccountingRule, error) {
	if r.listRulesErr != nil {
		return nil, r.listRulesErr
	}
	return r.MemoryRepository.ListEventAccountingRulesForCompany(ctx, companyID, eventType)
}

func (r *covEventRepo) SaveEventAccountingRule(ctx context.Context, rule EventAccountingRule) error {
	if r.saveRuleErr != nil {
		return r.saveRuleErr
	}
	return r.MemoryRepository.SaveEventAccountingRule(ctx, rule)
}

func covEventBridge(t *testing.T, repo *covEventRepo, companyID uuid.UUID) *EventBridge {
	t.Helper()
	bridge := NewEventBridgeForCompany(repo, &draftCreator{}, companyID)
	seed := companyID.String() + ":invoice_issued"
	if err := bridge.RegisterMapping(SubledgerEventMapping{
		CompanyID: companyID, EventType: "invoice_issued",
		DebitAccountID:  uuid.NewSHA1(uuid.NameSpaceOID, []byte(seed+":debit")),
		CreditAccountID: uuid.NewSHA1(uuid.NameSpaceOID, []byte(seed+":credit")),
		JournalID:       uuid.NewSHA1(uuid.NameSpaceOID, []byte(seed+":journal")),
	}); err != nil {
		t.Fatal(err)
	}
	return bridge
}

func covEvent(companyID uuid.UUID, key string) EconomicEvent {
	return EconomicEvent{
		CompanyID: companyID, SourceSystem: "billing", IdempotencyKey: key,
		EventType: "invoice_issued", Date: covDate(2),
		Amount: covMoneyRaw("100", "USD"), Reference: "INV-" + key,
	}
}

func TestCovEventBridgeValidationAndProcessErrors(t *testing.T) {
	companyID := uuid.New()
	repo := &covEventRepo{MemoryRepository: NewMemoryRepository()}
	bridge := NewEventBridgeForCompany(repo, &draftCreator{}, companyID)

	mappings := []struct {
		name    string
		mapping SubledgerEventMapping
	}{
		{"wrong company", SubledgerEventMapping{CompanyID: uuid.New(), EventType: "sale", DebitAccountID: uuid.New(), CreditAccountID: uuid.New(), JournalID: uuid.New()}},
		{"nil company", SubledgerEventMapping{EventType: "sale", DebitAccountID: uuid.New(), CreditAccountID: uuid.New(), JournalID: uuid.New()}},
		{"blank type", SubledgerEventMapping{CompanyID: companyID, DebitAccountID: uuid.New(), CreditAccountID: uuid.New(), JournalID: uuid.New()}},
		{"nil debit", SubledgerEventMapping{CompanyID: companyID, EventType: "sale", CreditAccountID: uuid.New(), JournalID: uuid.New()}},
		{"nil credit", SubledgerEventMapping{CompanyID: companyID, EventType: "sale", DebitAccountID: uuid.New(), JournalID: uuid.New()}},
		{"nil journal", SubledgerEventMapping{CompanyID: companyID, EventType: "sale", DebitAccountID: uuid.New(), CreditAccountID: uuid.New()}},
	}
	for _, tc := range mappings {
		t.Run(tc.name, func(t *testing.T) {
			if err := bridge.RegisterMapping(tc.mapping); !errors.Is(err, ErrMappingNotFound) && !errors.Is(err, ErrCompanyMismatch) {
				t.Fatalf("error = %v", err)
			}
		})
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := bridge.RegisterRuleContext(cancelled, EventAccountingRule{CompanyID: companyID}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled rule error = %v", err)
	}
	if err := bridge.RegisterRuleContext(context.Background(), EventAccountingRule{CompanyID: uuid.New()}); !errors.Is(err, ErrCompanyMismatch) {
		t.Fatalf("wrong company rule error = %v", err)
	}
	if err := bridge.RegisterRuleContext(context.Background(), EventAccountingRule{CompanyID: companyID}); !errors.Is(err, ErrMappingNotFound) {
		t.Fatalf("invalid rule error = %v", err)
	}
	repo.saveRuleErr = errors.New("save rule failed")
	if err := bridge.RegisterRule(EventAccountingRule{
		CompanyID: companyID, EventType: "rule-error", RuleName: "r", JournalID: uuid.New(),
		Lines: []EventLineTemplate{
			{AccountID: uuid.New(), Side: EventLineDebit, AmountFormula: "amount"},
			{AccountID: uuid.New(), Side: EventLineCredit, AmountFormula: "amount"},
		},
	}); !errors.Is(err, repo.saveRuleErr) {
		t.Fatalf("save rule error = %v", err)
	}
	repo.saveRuleErr = nil

	event := covEvent(companyID, "process-errors")
	if _, err := bridge.ProcessContext(cancelled, event); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled process error = %v", err)
	}
	if _, err := bridge.ProcessContext(context.Background(), EconomicEvent{}); !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("invalid event error = %v", err)
	}
	foreign := event
	foreign.CompanyID = uuid.New()
	if _, err := bridge.ProcessContext(context.Background(), foreign); !errors.Is(err, ErrCompanyMismatch) {
		t.Fatalf("foreign event error = %v", err)
	}

	freshBridge := func() *EventBridge {
		return covEventBridge(t, &covEventRepo{MemoryRepository: NewMemoryRepository()}, companyID)
	}
	lookupRepo := &covEventRepo{MemoryRepository: NewMemoryRepository(), identityFailCall: 1}
	if _, err := covEventBridge(t, lookupRepo, companyID).ProcessContext(context.Background(), event); err == nil {
		t.Fatal("expected identity lookup error")
	}
	incomplete := &covEventRepo{
		MemoryRepository: NewMemoryRepository(), identityOverrideFrom: 1,
		identityEvent: EconomicEvent{CompanyID: companyID, EventID: uuid.New()}, identityFound: true,
	}
	if _, err := covEventBridge(t, incomplete, companyID).ProcessContext(context.Background(), event); !errors.Is(err, ErrEventResultMissing) {
		t.Fatalf("incomplete stored event error = %v", err)
	}
	noHash := &covEventRepo{
		MemoryRepository: NewMemoryRepository(), identityOverrideFrom: 1, identityFound: true,
		identityEvent: EconomicEvent{CompanyID: companyID, EventID: uuid.New(), ResultEntryID: uuid.New()},
	}
	if _, err := covEventBridge(t, noHash, companyID).ProcessContext(context.Background(), event); !errors.Is(err, ErrIdempotencyPayloadConflict) {
		t.Fatalf("missing payload hash error = %v", err)
	}
	saveRepo := &covEventRepo{MemoryRepository: NewMemoryRepository(), saveEventErr: errors.New("save failed")}
	if _, err := covEventBridge(t, saveRepo, companyID).ProcessContext(context.Background(), event); !errors.Is(err, saveRepo.saveEventErr) {
		t.Fatalf("save event error = %v", err)
	}
	postSaveLookup := &covEventRepo{MemoryRepository: NewMemoryRepository(), identityFailCall: 2}
	if _, err := covEventBridge(t, postSaveLookup, companyID).ProcessContext(context.Background(), event); err == nil {
		t.Fatal("expected post-save identity lookup error")
	}

	ruleRepo := &covEventRepo{MemoryRepository: NewMemoryRepository(), listRulesErr: errors.New("rules failed")}
	if _, err := covEventBridge(t, ruleRepo, companyID).ProcessContext(context.Background(), event); !errors.Is(err, ruleRepo.listRulesErr) {
		t.Fatalf("list rules error = %v", err)
	}
	badFormulaRepo := &covEventRepo{MemoryRepository: NewMemoryRepository()}
	badBridge := covEventBridge(t, badFormulaRepo, companyID)
	if err := badBridge.RegisterRule(EventAccountingRule{
		CompanyID: companyID, EventType: "invoice_issued", RuleName: "bad formula", JournalID: uuid.New(),
		Lines: []EventLineTemplate{
			{AccountID: uuid.New(), Side: EventLineDebit, AmountFormula: "unknown"},
			{AccountID: uuid.New(), Side: EventLineCredit, AmountFormula: "amount"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := badBridge.ProcessContext(context.Background(), event); !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("bad formula error = %v", err)
	}
	if freshBridge() == nil {
		t.Fatal("bridge constructor failed")
	}
}

func TestCovEventBridgePersistedReplayAndSaveErrors(t *testing.T) {
	companyID := uuid.New()
	event := covEvent(companyID, "persisted-replay")
	repo := &covEventRepo{MemoryRepository: NewMemoryRepository()}
	firstBridge := covEventBridge(t, repo, companyID)
	first, err := firstBridge.ProcessContext(context.Background(), event)
	if err != nil {
		t.Fatal(err)
	}
	secondBridge := covEventBridge(t, repo, companyID)
	replayed, err := secondBridge.ProcessContext(context.Background(), event)
	if err != nil || replayed.ID != first.ID {
		t.Fatalf("persisted replay = %+v err=%v", replayed, err)
	}

	saveErrRepo := &covEventRepo{MemoryRepository: NewMemoryRepository(), saveDraftErr: errors.New("atomic save failed")}
	if _, err := covEventBridge(t, saveErrRepo, companyID).ProcessContext(context.Background(), event); !errors.Is(err, saveErrRepo.saveDraftErr) {
		t.Fatalf("atomic save error = %v", err)
	}
	lookupRepo := &covEventRepo{
		MemoryRepository: NewMemoryRepository(), saveDraftErr: ErrAlreadyExists, identityFailCall: 2,
	}
	if _, err := covEventBridge(t, lookupRepo, companyID).ProcessContext(context.Background(), event); err == nil {
		t.Fatal("expected duplicate lookup error")
	}
	missingResult := &covEventRepo{
		MemoryRepository: NewMemoryRepository(), saveDraftErr: ErrAlreadyExists,
		identityOverrideFrom: 2, identityFound: false,
	}
	if _, err := covEventBridge(t, missingResult, companyID).ProcessContext(context.Background(), event); !errors.Is(err, ErrEventResultMissing) {
		t.Fatalf("missing result error = %v", err)
	}
	conflict := &covEventRepo{
		MemoryRepository: NewMemoryRepository(), saveDraftErr: ErrAlreadyExists,
		identityOverrideFrom: 2, identityFound: true,
		identityEvent: EconomicEvent{ResultEntryID: uuid.New(), PayloadHash: strings.Repeat("x", 64)},
	}
	if _, err := covEventBridge(t, conflict, companyID).ProcessContext(context.Background(), event); !errors.Is(err, ErrIdempotencyPayloadConflict) {
		t.Fatalf("stored conflict error = %v", err)
	}
	seedRepo := &covEventRepo{MemoryRepository: NewMemoryRepository()}
	if _, err := covEventBridge(t, seedRepo, companyID).ProcessContext(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	seedEvent, found, err := seedRepo.GetEventByIdentity(context.Background(), companyID, event.SourceSystem, event.IdempotencyKey)
	if err != nil || !found || seedEvent.PayloadHash == "" {
		t.Fatalf("seed event = %+v found=%v err=%v", seedEvent, found, err)
	}
	stored := &covEventRepo{
		MemoryRepository: NewMemoryRepository(), saveDraftErr: ErrAlreadyExists,
		identityOverrideFrom: 2, identityFound: true, identityEvent: seedEvent,
	}
	storedEntry, err := covEventBridge(t, stored, companyID).ProcessContext(context.Background(), event)
	if err != nil || storedEntry.ID != stored.identityEvent.ResultEntryID {
		t.Fatalf("already-exists replay = %+v err=%v", storedEntry, err)
	}
	statusRepo := &covEventRepo{MemoryRepository: NewMemoryRepository(), listRulesErr: errors.New("rules failed")}
	if _, err := covEventBridge(t, statusRepo, companyID).ProcessContext(context.Background(), event); !errors.Is(err, statusRepo.listRulesErr) {
		t.Fatalf("rule failure = %v", err)
	}
	statusRepo.updateErr = errors.New("status failed")
	if _, err := covEventBridge(t, statusRepo, companyID).ProcessContext(context.Background(), event); !errors.Is(err, statusRepo.listRulesErr) {
		t.Fatalf("status update failure must not mask cause: %v", err)
	}
}

func TestCovEventBridgeReplayBranches(t *testing.T) {
	companyID := uuid.New()
	event := covEvent(companyID, "replay-branches")
	repo := &covEventRepo{MemoryRepository: NewMemoryRepository()}
	bridge := covEventBridge(t, repo, companyID)
	first, err := bridge.ProcessContext(context.Background(), event)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := repo.GetEventByID(context.Background(), companyID, *first.SourceEventID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bridge.ReplayEvent(stored.EventID); err != nil || err == nil && first.ID == uuid.Nil {
		t.Fatalf("ReplayEvent: %v", err)
	}
	if _, err := NewEventBridge(repo, &draftCreator{}).ReplayEventContext(context.Background(), stored.EventID); !errors.Is(err, ErrCompanyScopeRequired) {
		t.Fatalf("unscoped replay error = %v", err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := bridge.ReplayEventForCompanyContext(cancelled, companyID, stored.EventID); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled replay error = %v", err)
	}
	if _, err := bridge.ReplayEventForCompanyContext(context.Background(), uuid.Nil, stored.EventID); !errors.Is(err, ErrCompanyScopeRequired) {
		t.Fatalf("nil company replay error = %v", err)
	}
	if _, err := bridge.ReplayEventForCompanyContext(context.Background(), uuid.New(), stored.EventID); !errors.Is(err, ErrCompanyMismatch) {
		t.Fatalf("foreign company replay error = %v", err)
	}
	if _, err := bridge.ReplayEventForCompanyContext(context.Background(), companyID, uuid.Nil); !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("nil event replay error = %v", err)
	}
	byIDErr := &covEventRepo{MemoryRepository: NewMemoryRepository(), byIDErr: errors.New("lookup failed")}
	scoped := NewEventBridgeForCompany(byIDErr, &draftCreator{}, companyID)
	if _, err := scoped.ReplayEventContext(context.Background(), uuid.New()); !errors.Is(err, byIDErr.byIDErr) {
		t.Fatalf("replay lookup error = %v", err)
	}
	processedEvent := covEvent(companyID, "processed")
	processedEvent.EventID = uuid.New()
	processedEvent.Status = EventProcessed
	nonReplayable := &covEventRepo{
		MemoryRepository: NewMemoryRepository(),
		byIDEvent:        processedEvent,
	}
	scoped = NewEventBridgeForCompany(nonReplayable, &draftCreator{}, companyID)
	if _, err := scoped.ReplayEventContext(context.Background(), nonReplayable.byIDEvent.EventID); err == nil {
		t.Fatal("expected replay of processed event to fail processing")
	}
	rejectedEvent := covEvent(companyID, "rejected")
	rejectedEvent.EventID = uuid.New()
	rejectedEvent.Status = EventRejected
	rejectedEvent.Amount = financialtypes.Money{}
	rejectedMemory := NewMemoryRepository()
	if err := rejectedMemory.SaveEvent(context.Background(), rejectedEvent); err != nil {
		t.Fatal(err)
	}
	rejected := &covEventRepo{
		MemoryRepository: rejectedMemory,
		byIDEvent:        rejectedEvent,
	}
	scoped = NewEventBridgeForCompany(rejected, &draftCreator{}, companyID)
	if err := scoped.RegisterMapping(SubledgerEventMapping{
		CompanyID: companyID, EventType: rejectedEvent.EventType,
		DebitAccountID: uuid.New(), CreditAccountID: uuid.New(), JournalID: uuid.New(),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := scoped.ReplayEventContext(context.Background(), rejectedEvent.EventID); !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("invalid rejected event error = %v", err)
	}
	updateErr := &covEventRepo{
		MemoryRepository: NewMemoryRepository(), updateErr: errors.New("update failed"),
		byIDEvent: EconomicEvent{CompanyID: companyID, EventID: uuid.New(), Status: EventDeadLetter},
	}
	scoped = NewEventBridgeForCompany(updateErr, &draftCreator{}, companyID)
	if _, err := scoped.ReplayEventContext(context.Background(), updateErr.byIDEvent.EventID); !errors.Is(err, updateErr.updateErr) {
		t.Fatalf("replay status error = %v", err)
	}
}

type covMatchRepo struct {
	*MemoryRepository
	getOverride     map[uuid.UUID]OpenLine
	getErr          error
	compareErr      error
	compareDraftErr error
}

func (r *covMatchRepo) GetOpenLine(ctx context.Context, companyID, id uuid.UUID) (OpenLine, error) {
	if r.getErr != nil {
		return OpenLine{}, r.getErr
	}
	if line, ok := r.getOverride[id]; ok {
		return cloneOpenLine(line), nil
	}
	return r.MemoryRepository.GetOpenLine(ctx, companyID, id)
}

func (r *covMatchRepo) CompareAndSaveOpenLines(ctx context.Context, expected, updated []OpenLine) error {
	if r.compareErr != nil {
		return r.compareErr
	}
	return r.MemoryRepository.CompareAndSaveOpenLines(ctx, expected, updated)
}

func (r *covMatchRepo) CompareAndSaveOpenLinesAndDraft(
	ctx context.Context, expected, updated []OpenLine, draft posting.JournalEntry,
) error {
	if r.compareDraftErr != nil {
		return r.compareDraftErr
	}
	return r.MemoryRepository.CompareAndSaveOpenLinesAndDraft(ctx, expected, updated, draft)
}

func covMatchFixture(
	t *testing.T, lineKind, counterpartKind LineKind,
) (*covMatchRepo, *MatchingEngine, uuid.UUID, OpenLine, OpenLine) {
	t.Helper()
	companyID, controlID := uuid.New(), uuid.New()
	repo := &covMatchRepo{MemoryRepository: NewMemoryRepository()}
	line := covReconciliationLine(companyID, controlID, lineKind, "100", "100")
	counterpart := covReconciliationLine(companyID, controlID, counterpartKind, "100", "100")
	if err := repo.MemoryRepository.SaveOpenLine(context.Background(), line); err != nil {
		t.Fatal(err)
	}
	if err := repo.MemoryRepository.SaveOpenLine(context.Background(), counterpart); err != nil {
		t.Fatal(err)
	}
	return repo, NewMatchingEngineForCompany(repo, companyID), companyID, line, counterpart
}

func TestCovMatchingEngineMatchAndUnreconcile(t *testing.T) {
	ctx := context.Background()
	repo, engine, companyID, line, counterpart := covMatchFixture(t, LineReceivable, LinePayable)

	matchingNumber, fxEntry, err := engine.MatchWithFXContext(
		ctx, line.ID, counterpart.ID, covMoneyRaw("40", "USD"), FXMatchingParams{Date: covDate(2)},
	)
	if err != nil || fxEntry != nil || matchingNumber == "" {
		t.Fatalf("match = %q entry=%+v err=%v", matchingNumber, fxEntry, err)
	}
	storedLine, err := repo.GetOpenLine(ctx, companyID, line.ID)
	if err != nil {
		t.Fatal(err)
	}
	storedCounterpart, err := repo.GetOpenLine(ctx, companyID, counterpart.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !storedLine.ResidualAmount.Equal(covMoneyRaw("60", "USD")) ||
		!storedCounterpart.ResidualAmount.Equal(covMoneyRaw("60", "USD")) ||
		!storedLine.FunctionalResidualAmount.Equal(covMoneyRaw("60", "USD")) {
		t.Fatalf("residuals after match: line=%+v counterpart=%+v", storedLine, storedCounterpart)
	}

	if err := engine.UnreconcileContext(ctx, line.ID); err != nil {
		t.Fatalf("UnreconcileContext(): %v", err)
	}
	reopened, err := repo.GetOpenLine(ctx, companyID, line.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reopened.ResidualAmount.Equal(covMoneyRaw("100", "USD")) || reopened.Closed || len(reopened.Matches) != 0 {
		t.Fatalf("reopened line = %+v", reopened)
	}
}

func TestCovMatchingEngineMatchErrors(t *testing.T) {
	cases := []struct {
		name    string
		kind    LineKind
		other   LineKind
		amount  financialtypes.Money
		arrange func(*covMatchRepo, OpenLine, OpenLine)
		ctxErr  bool
		want    error
	}{
		{name: "same line", kind: LineReceivable, other: LinePayable, amount: covMoneyRaw("40", "USD"), want: ErrUnmatchedLine},
		{name: "cancelled", kind: LineReceivable, other: LinePayable, amount: covMoneyRaw("40", "USD"), ctxErr: true, want: context.Canceled},
		{name: "lookup error", kind: LineReceivable, other: LinePayable, amount: covMoneyRaw("40", "USD"), want: errors.New("")},
		{name: "invalid line", kind: LineReceivable, other: LinePayable, amount: covMoneyRaw("40", "USD"), want: ErrInvalidOpenLine},
		{name: "same kind", kind: LineReceivable, other: LineReceivable, amount: covMoneyRaw("40", "USD"), want: ErrUnmatchedLine},
		{name: "company mismatch", kind: LineReceivable, other: LinePayable, amount: covMoneyRaw("40", "USD"), want: ErrCompanyMismatch},
		{name: "zero amount", kind: LineReceivable, other: LinePayable, amount: covMoneyRaw("0", "USD"), want: ErrInvalidAmount},
		{name: "closed line", kind: LineReceivable, other: LinePayable, amount: covMoneyRaw("40", "USD"), want: ErrInvalidAmount},
		{name: "invalid legacy match", kind: LineReceivable, other: LinePayable, amount: covMoneyRaw("40", "USD"), want: ErrInvalidOpenLine},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo, engine, _, line, counterpart := covMatchFixture(t, tc.kind, tc.other)
			lineID, counterpartID := line.ID, counterpart.ID
			if tc.name == "same line" {
				counterpartID = lineID
			}
			switch tc.name {
			case "lookup error":
				repo.getErr = errors.New("lookup failed")
				tc.want = repo.getErr
			case "invalid line":
				repo.getOverride = map[uuid.UUID]OpenLine{line.ID: {CompanyID: line.CompanyID}}
			case "company mismatch":
				foreign := counterpart
				foreign.CompanyID = uuid.New()
				repo.getOverride = map[uuid.UUID]OpenLine{counterpart.ID: foreign}
			case "closed line":
				closed := line
				closed.Closed = true
				repo.getOverride = map[uuid.UUID]OpenLine{line.ID: closed}
			case "invalid legacy match":
				legacy := line
				legacy.MatchingNumber = "MATCH-LEGACY"
				legacy.MatchIDs = []uuid.UUID{uuid.New(), uuid.New()}
				repo.getOverride = map[uuid.UUID]OpenLine{line.ID: legacy}
			}
			ctx := context.Background()
			if tc.ctxErr {
				cancelled, cancel := context.WithCancel(ctx)
				cancel()
				ctx = cancelled
			}
			if _, _, err := engine.MatchWithFXContext(ctx, lineID, counterpartID, tc.amount, FXMatchingParams{}); !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestCovMatchingEngineFXAndErrorPropagation(t *testing.T) {
	ctx := context.Background()
	fxAccounts := func() FXMatchingParams {
		return FXMatchingParams{
			JournalID: uuid.New(), Date: covDate(2), OriginalRate: decimal.NewFromInt(1),
			SettlementRate: decimal.NewFromInt(2), FunctionalCurrency: "USD",
			ARAPAccountID: uuid.New(), FXGainAccountID: uuid.New(), FXLossAccountID: uuid.New(),
			Description: "FX settlement",
		}
	}

	for _, tc := range []struct {
		name      string
		lineKind  LineKind
		otherKind LineKind
		wantDebit func(FXMatchingParams) uuid.UUID
	}{
		{
			name: "receivable gain", lineKind: LineReceivable, otherKind: LinePayable,
			wantDebit: func(p FXMatchingParams) uuid.UUID { return p.ARAPAccountID },
		},
		{
			name: "payable loss", lineKind: LinePayable, otherKind: LineReceivable,
			wantDebit: func(p FXMatchingParams) uuid.UUID { return p.FXLossAccountID },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, engine, _, line, counterpart := covMatchFixture(t, tc.lineKind, tc.otherKind)
			params := fxAccounts()
			_, entry, err := engine.MatchWithFXContext(
				ctx, line.ID, counterpart.ID, covMoneyRaw("40", "USD"), params,
			)
			if err != nil || entry == nil || len(entry.Lines) != 2 {
				t.Fatalf("FX match = %+v err=%v", entry, err)
			}
			if entry.Lines[0].AccountID != tc.wantDebit(params) || !entry.Lines[0].Debit.Equal(covMoneyRaw("40", "USD")) {
				t.Fatalf("FX entry lines = %+v", entry.Lines)
			}
			if entry.DocumentReference != params.Description {
				t.Fatalf("FX reference = %q", entry.DocumentReference)
			}
			repo.compareDraftErr = errors.New("atomic save failed")
			_, _, err = engine.MatchWithFXContext(
				ctx, line.ID, counterpart.ID, covMoneyRaw("40", "USD"), params,
			)
			if !errors.Is(err, repo.compareDraftErr) {
				t.Fatalf("atomic save error = %v", err)
			}
		})
	}

	repo, engine, _, line, counterpart := covMatchFixture(t, LineReceivable, LinePayable)
	missingAccounts := fxAccounts()
	missingAccounts.ARAPAccountID = uuid.Nil
	if _, _, err := engine.MatchWithFXContext(
		ctx, line.ID, counterpart.ID, covMoneyRaw("40", "USD"), missingAccounts,
	); !errors.Is(err, ErrInvalidOpenLine) {
		t.Fatalf("missing FX accounts error = %v", err)
	}

	repo, engine, _, line, counterpart = covMatchFixture(t, LineReceivable, LinePayable)
	badRate := fxAccounts()
	badRate.OriginalRate = decimal.NewFromInt(-1)
	if _, _, err := engine.MatchWithFXContext(
		ctx, line.ID, counterpart.ID, covMoneyRaw("40", "USD"), badRate,
	); err == nil {
		t.Fatal("expected invalid FX rate error")
	}

	repo, engine, _, line, counterpart = covMatchFixture(t, LineReceivable, LinePayable)
	repo.compareErr = errors.New("compare failed")
	if _, _, err := engine.MatchWithFXContext(
		ctx, line.ID, counterpart.ID, covMoneyRaw("40", "USD"), FXMatchingParams{Date: covDate(2)},
	); !errors.Is(err, repo.compareErr) {
		t.Fatalf("compare error = %v", err)
	}
}

func TestCovMatchingEngineUnreconcileErrors(t *testing.T) {
	ctx := context.Background()
	repo, engine, _, line, counterpart := covMatchFixture(t, LineReceivable, LinePayable)
	if err := engine.UnreconcileContext(ctx, line.ID); err != nil {
		t.Fatalf("unreconcile without matches: %v", err)
	}
	if _, _, err := engine.MatchWithFXContext(
		ctx, line.ID, counterpart.ID, covMoneyRaw("40", "USD"), FXMatchingParams{Date: covDate(2)},
	); err != nil {
		t.Fatal(err)
	}

	t.Run("cancelled", func(t *testing.T) {
		cancelled, cancel := context.WithCancel(ctx)
		cancel()
		if err := engine.UnreconcileContext(cancelled, line.ID); !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("lookup error", func(t *testing.T) {
		repo.getErr = errors.New("lookup failed")
		defer func() { repo.getErr = nil }()
		if err := engine.UnreconcileContext(ctx, line.ID); !errors.Is(err, repo.getErr) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("foreign counterpart", func(t *testing.T) {
		foreign := counterpart
		foreign.CompanyID = uuid.New()
		repo.getOverride = map[uuid.UUID]OpenLine{counterpart.ID: foreign}
		defer func() { repo.getOverride = nil }()
		if err := engine.UnreconcileContext(ctx, line.ID); !errors.Is(err, ErrCompanyMismatch) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("missing reciprocal", func(t *testing.T) {
		noMatches := counterpart
		noMatches.Matches = nil
		noMatches.MatchHistory = nil
		repo.getOverride = map[uuid.UUID]OpenLine{counterpart.ID: noMatches}
		defer func() { repo.getOverride = nil }()
		if err := engine.UnreconcileContext(ctx, line.ID); !errors.Is(err, ErrInvalidOpenLine) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("save error", func(t *testing.T) {
		repo.compareErr = errors.New("compare failed")
		defer func() { repo.compareErr = nil }()
		if err := engine.UnreconcileContext(ctx, line.ID); !errors.Is(err, repo.compareErr) {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestCovDepreciationScheduleVariants(t *testing.T) {
	companyID, assetID := uuid.New(), uuid.New()
	cost := covMoneyRaw("1200", "USD")
	salvage := covMoneyRaw("0", "USD")
	base := FixedAsset{
		CompanyID: companyID, ID: assetID, HistoricalCost: cost, SalvageValue: salvage,
		UsefulLifeMonths: 12, Method: StraightLine, InServiceDate: covDate(15),
	}

	t.Run("straight line prorata", func(t *testing.T) {
		lines, err := GenerateDepreciationSchedule(base)
		if err != nil || len(lines) == 0 {
			t.Fatalf("schedule = %+v err=%v", lines, err)
		}
		for _, line := range lines {
			if line.AssetID != assetID || line.Amount.Amount().IsNegative() || line.BookValue.Amount().IsNegative() {
				t.Fatalf("unexpected schedule line = %+v", line)
			}
		}
	})

	for _, tc := range []struct {
		name string
		rate string
	}{
		{name: "declining balance default rate"},
		{name: "declining balance explicit rate", rate: "0.5"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			asset := base
			asset.Method = DecliningBalance
			if tc.rate != "" {
				asset.DecliningRate = decimal.RequireFromString(tc.rate)
			}
			lines, err := GenerateDepreciationSchedule(asset)
			if err != nil || len(lines) == 0 {
				t.Fatalf("schedule = %+v err=%v", lines, err)
			}
		})
	}

	t.Run("component defaults in service date", func(t *testing.T) {
		asset := base
		asset.Components = []AssetComponent{{
			ID: uuid.New(), Name: "component", Cost: cost, SalvageValue: salvage,
			UsefulLifeMonths: 12, Method: StraightLine,
		}}
		lines, err := GenerateDepreciationSchedule(asset)
		if err != nil || len(lines) == 0 || lines[0].ComponentID != asset.Components[0].ID {
			t.Fatalf("component schedule = %+v err=%v", lines, err)
		}
	})

	for _, tc := range []struct {
		name  string
		asset FixedAsset
	}{
		{name: "cwip", asset: func() FixedAsset { a := base; a.IsCWIP = true; return a }()},
		{name: "unknown method", asset: func() FixedAsset { a := base; a.Method = "unknown"; return a }()},
		{name: "component total mismatch", asset: func() FixedAsset {
			a := base
			a.Components = []AssetComponent{{
				ID: uuid.New(), Name: "partial", Cost: covMoneyRaw("100", "USD"),
				SalvageValue: salvage, UsefulLifeMonths: 12, Method: StraightLine,
			}}
			return a
		}()},
		{name: "invalid declining rate", asset: func() FixedAsset {
			a := base
			a.Method = DecliningBalance
			a.DecliningRate = decimal.NewFromInt(-1)
			return a
		}()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := GenerateDepreciationSchedule(tc.asset); !errors.Is(err, ErrInvalidAsset) {
				t.Fatalf("error = %v", err)
			}
		})
	}

	if amount, err := CalculateProrataDepreciation(
		cost.Amount(), salvage.Amount(), 12, covDate(15), covDate(15).AddDate(0, 13, 0),
	); err != nil || !amount.IsZero() {
		t.Fatalf("post-life prorata = %s err=%v", amount, err)
	}
}
