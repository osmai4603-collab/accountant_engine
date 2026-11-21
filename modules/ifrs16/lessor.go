package ifrs16

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/journal"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/shopspring/decimal"
)

var (
	ErrInvalidLessorAccounts = errors.New("invalid lessor journal accounts")
	ErrNegativeNetInvestment = errors.New("lease receipt exceeds accrued net investment")
	ErrAssetDepreciationUnav = errors.New("no leased-asset depreciation due for this period")
)

// LessorLeaseClassification is the host-approved lessor classification. The
// engine does not assess the transfer of risks and rewards; the host supplies
// the approved classification as a final input and the engine applies the
// corresponding accounting.
type LessorLeaseClassification string

const (
	// LessorOperatingLease keeps the leased asset on the lessor balance sheet.
	LessorOperatingLease LessorLeaseClassification = "operating"
	// LessorFinanceLease derecognizes the leased asset and recognizes the
	// net investment in the lease.
	LessorFinanceLease LessorLeaseClassification = "finance"
)

// OperatingLeaseContract holds a host-classified lessor operating lease and the
// derived commencement balances. The leased asset remains on the lessor balance
// sheet and is depreciated under the host's depreciation policy; lease income is
// recognized on a straight-line basis over the lease term. Initial direct costs
// incurred in obtaining the lease are added to the depreciated asset base and
// expensed on the same straight-line basis as the lease income.
type OperatingLeaseContract struct {
	Reference            string         `json:"reference,omitempty"`
	CommencementDate     time.Time      `json:"commencement_date"`
	Currency             string         `json:"currency"`
	Payments             []LeasePayment `json:"payments"`
	AssetCost            types.Money    `json:"asset_cost"`
	InitialDirectCosts   types.Money    `json:"initial_direct_costs"`
	DepreciationPeriods  int            `json:"depreciation_periods"`
	DepreciableAssetBase types.Money    `json:"depreciable_asset_base"`
	TotalConsideration   types.Money    `json:"total_consideration"`
}

// StraightLineLeasePeriod is one dated operating-lease income and asset
// depreciation roll-forward. DeferredMovement and CumulativeDeferredIncome are
// signed: positive means straight-line income exceeded cash (accrued rent
// receivable asset); negative means cash exceeded straight-line income
// (deferred lease income liability).
type StraightLineLeasePeriod struct {
	LeaseReference           string
	Period                   int
	DueDate                  time.Time
	CashReceived             types.Money
	LeaseIncome              types.Money
	DeferredMovement         types.Money
	CumulativeDeferredIncome types.Money
	AssetDepreciation        types.Money
	AccumulatedDepreciation  types.Money
	ClosingAssetCarrying     types.Money
}

// WithReference returns a copy identified by the caller's stable contract key.
func (c OperatingLeaseContract) WithReference(reference string) OperatingLeaseContract {
	c.Reference = reference
	return c
}

// NewOperatingLeaseContract validates a lessor operating lease and derives the
// depreciated asset base and total consideration. A zero DepreciationPeriods
// defaults to the lease term (the number of payments); a value greater than the
// lease term leaves a carrying amount at lease end, reflecting the lessor's own
// depreciation policy for the retained asset.
func NewOperatingLeaseContract(
	commencementDate time.Time,
	payments []LeasePayment,
	assetCost, initialDirectCosts types.Money,
	depreciationPeriods int,
) (OperatingLeaseContract, error) {
	if commencementDate.IsZero() || len(payments) == 0 || depreciationPeriods < 0 {
		return OperatingLeaseContract{}, ErrInvalidContract
	}
	currency := assetCost.Currency()
	if currency == "" {
		currency = initialDirectCosts.Currency()
	}
	if currency == "" {
		currency = payments[0].Amount.Currency()
	}
	if currency == "" {
		return OperatingLeaseContract{}, ErrInvalidContract
	}
	if err := validateNonnegativeMoney(initialDirectCosts, currency); err != nil {
		return OperatingLeaseContract{}, err
	}
	if assetCost.Currency() != currency || !assetCost.Amount().IsPositive() {
		return OperatingLeaseContract{}, fmt.Errorf("%w: asset cost", ErrInvalidContract)
	}
	if _, err := types.NewMoney(assetCost.Amount(), currency); err != nil {
		return OperatingLeaseContract{}, err
	}
	leasePayments, err := validateRemainingPayments(commencementDate, payments, currency)
	if err != nil {
		return OperatingLeaseContract{}, err
	}

	total := mustZero(currency)
	for _, payment := range leasePayments {
		sum, err := total.Add(payment.Amount)
		if err != nil {
			return OperatingLeaseContract{}, err
		}
		total = sum
	}
	assetBase, err := assetCost.Add(initialDirectCosts)
	if err != nil {
		return OperatingLeaseContract{}, err
	}

	periods := depreciationPeriods
	if periods == 0 {
		periods = len(leasePayments)
	}
	return OperatingLeaseContract{
		CommencementDate:     commencementDate.UTC(),
		Currency:             currency,
		Payments:             leasePayments,
		AssetCost:            assetCost,
		InitialDirectCosts:   initialDirectCosts,
		DepreciationPeriods:  periods,
		DepreciableAssetBase: assetBase,
		TotalConsideration:   total,
	}, nil
}

// StraightLineSchedule returns the straight-line income and asset depreciation
// roll-forward without mutating the contract. The final period absorbs the
// income and depreciation rounding residual.
func (c OperatingLeaseContract) StraightLineSchedule() ([]StraightLineLeasePeriod, error) {
	return BuildStraightLineSchedule(c)
}

// FinanceLeaseContract holds a host-classified lessor finance lease and the
// commencement net investment in the lease. The leased asset is derecognized
// and replaced by the net investment: the present value of lease receipts plus
// the present value of the unguaranteed residual value, plus initial direct
// costs, discounted at the implicit interest rate.
type FinanceLeaseContract struct {
	Reference             string          `json:"reference,omitempty"`
	CommencementDate      time.Time       `json:"commencement_date"`
	Currency              string          `json:"currency"`
	ImplicitRate          decimal.Decimal `json:"implicit_rate"`
	Payments              []LeasePayment  `json:"payments"`
	UnguaranteedResidual  types.Money     `json:"unguaranteed_residual"`
	InitialDirectCosts    types.Money     `json:"initial_direct_costs"`
	AssetCarryingAmount   types.Money     `json:"asset_carrying_amount"`
	NetInvestment         types.Money     `json:"net_investment"`
	FairValueEquivalent   types.Money     `json:"fair_value_equivalent"`
	DerecognitionGainLoss types.Money     `json:"derecognition_gain_loss"`
}

// FinanceLeasePeriod is one dated net-investment roll-forward. The closing
// balance after the final receipt equals the unguaranteed residual value,
// which the host derecognizes when the residual asset is returned and dealt
// with at lease end.
type FinanceLeasePeriod struct {
	LeaseReference       string
	Period               int
	DueDate              time.Time
	OpeningNetInvestment types.Money
	FinanceIncome        types.Money
	CashReceived         types.Money
	PrincipalRecovery    types.Money
	ClosingNetInvestment types.Money
}

// WithReference returns a copy identified by the caller's stable contract key.
func (c FinanceLeaseContract) WithReference(reference string) FinanceLeaseContract {
	c.Reference = reference
	return c
}

// NewFinanceLeaseContract validates a lessor finance lease and derives the net
// investment, the fair-value equivalent, and the derecognition gain or loss.
// Amounts without an explicit currency are treated as zero.
func NewFinanceLeaseContract(
	commencementDate time.Time,
	implicitRate decimal.Decimal,
	payments []LeasePayment,
	unguaranteedResidual, initialDirectCosts, assetCarryingAmount types.Money,
) (FinanceLeaseContract, error) {
	if commencementDate.IsZero() || len(payments) == 0 || implicitRate.IsNegative() {
		return FinanceLeaseContract{}, ErrInvalidContract
	}
	currency := unguaranteedResidual.Currency()
	if currency == "" {
		currency = initialDirectCosts.Currency()
	}
	if currency == "" {
		currency = assetCarryingAmount.Currency()
	}
	if currency == "" {
		currency = payments[0].Amount.Currency()
	}
	if currency == "" {
		return FinanceLeaseContract{}, ErrInvalidContract
	}
	for name, amount := range map[string]types.Money{
		"unguaranteed residual": unguaranteedResidual,
		"initial direct costs":  initialDirectCosts,
		"asset carrying amount": assetCarryingAmount,
	} {
		if amount.Currency() != "" && amount.Currency() != currency {
			return FinanceLeaseContract{}, fmt.Errorf("%w: %s currency", ErrInvalidContract, name)
		}
		if amount.Currency() != "" && amount.IsNegative() {
			return FinanceLeaseContract{}, fmt.Errorf("%w: negative %s", ErrInvalidContract, name)
		}
	}
	residual := normalizeMoney(unguaranteedResidual, currency)
	directCosts := normalizeMoney(initialDirectCosts, currency)
	carrying := normalizeMoney(assetCarryingAmount, currency)

	leasePayments, err := validateRemainingPayments(commencementDate, payments, currency)
	if err != nil {
		return FinanceLeaseContract{}, err
	}
	residualDate := leasePayments[len(leasePayments)-1].DueDate
	residualPV, err := PresentValue([]LeasePayment{{DueDate: residualDate, Amount: residual}},
		commencementDate, implicitRate, currency)
	if err != nil {
		return FinanceLeaseContract{}, err
	}
	receiptsPV, err := PresentValue(leasePayments, commencementDate, implicitRate, currency)
	if err != nil {
		return FinanceLeaseContract{}, err
	}
	netInvestment, err := receiptsPV.Add(residualPV)
	if err != nil {
		return FinanceLeaseContract{}, err
	}
	netInvestment, err = netInvestment.Add(directCosts)
	if err != nil {
		return FinanceLeaseContract{}, err
	}
	if !netInvestment.Amount().IsPositive() {
		return FinanceLeaseContract{}, ErrInvalidContract
	}
	fairValue, err := netInvestment.Subtract(directCosts)
	if err != nil {
		return FinanceLeaseContract{}, err
	}
	gainLoss, err := fairValue.Subtract(carrying)
	if err != nil {
		return FinanceLeaseContract{}, err
	}
	return FinanceLeaseContract{
		CommencementDate:      commencementDate.UTC(),
		Currency:              currency,
		ImplicitRate:          implicitRate,
		Payments:              leasePayments,
		UnguaranteedResidual:  residual,
		InitialDirectCosts:    directCosts,
		AssetCarryingAmount:   carrying,
		NetInvestment:         netInvestment,
		FairValueEquivalent:   fairValue,
		DerecognitionGainLoss: gainLoss,
	}, nil
}

// FinanceIncomeSchedule returns the implicit-rate finance income allocation
// without mutating the contract. Intermediate periods accrue income at the
// implicit rate; the final period absorbs the rounding residual and the
// amortization of initial direct costs so the closing balance equals the
// unguaranteed residual value exactly.
func (c FinanceLeaseContract) FinanceIncomeSchedule() ([]FinanceLeasePeriod, error) {
	return BuildFinanceIncomeSchedule(c)
}

// LessorAccounts identifies the general-ledger accounts used by lessor entries.
type LessorAccounts struct {
	LeasedAsset             uuid.UUID `json:"leased_asset"`
	AccumulatedDepreciation uuid.UUID `json:"accumulated_depreciation"`
	NetInvestmentInLease    uuid.UUID `json:"net_investment_in_lease"`
	Cash                    uuid.UUID `json:"cash"`
	OperatingLeaseIncome    uuid.UUID `json:"operating_lease_income"`
	FinanceIncome           uuid.UUID `json:"finance_income"`
	DepreciationExpense     uuid.UUID `json:"depreciation_expense"`
	DeferredLeaseIncome     uuid.UUID `json:"deferred_lease_income"`
	AccruedRentReceivable   uuid.UUID `json:"accrued_rent_receivable"`
	DerecognitionGain       uuid.UUID `json:"derecognition_gain"`
	DerecognitionLoss       uuid.UUID `json:"derecognition_loss"`
}

// LessorPostingCapability must be backed by the host application's configured
// ledger-posting capability and resolve the selected real journal mapping.
type LessorPostingCapability interface {
	LedgerPostingAvailable() bool
	ResolveLessorAccounts(context.Context, uuid.UUID, string) (LessorAccounts, error)
}

// LessorEntryBuilder only creates drafts after the host confirms posting
// capability and resolves configured accounts for the requested mapping.
type LessorEntryBuilder struct {
	capability LessorPostingCapability
}

func NewLessorEntryBuilder(capability LessorPostingCapability) (*LessorEntryBuilder, error) {
	if capability == nil || !capability.LedgerPostingAvailable() {
		return nil, ErrPostingCapabilityUnavailable
	}
	return &LessorEntryBuilder{capability: capability}, nil
}

func (b *LessorEntryBuilder) resolve(
	ctx context.Context, companyID uuid.UUID, mappingID string,
) (LessorAccounts, error) {
	if b == nil || b.capability == nil || !b.capability.LedgerPostingAvailable() {
		return LessorAccounts{}, ErrPostingCapabilityUnavailable
	}
	if strings.TrimSpace(mappingID) == "" {
		return LessorAccounts{}, ErrJournalMappingUnavailable
	}
	accounts, err := b.capability.ResolveLessorAccounts(ctx, companyID, mappingID)
	if err != nil {
		return LessorAccounts{}, err
	}
	return accounts, nil
}

// BuildOperatingInitialDirectCostsEntry capitalizes initial direct costs on the
// leased asset and credits cash for the payment.
func (b *LessorEntryBuilder) BuildOperatingInitialDirectCostsEntry(
	ctx context.Context, contract OperatingLeaseContract, companyID, journalID uuid.UUID,
	mappingID, createdBy string,
) (journal.JournalEntry, error) {
	accounts, err := b.resolve(ctx, companyID, mappingID)
	if err != nil {
		return journal.JournalEntry{}, err
	}
	return buildOperatingInitialDirectCostsEntry(contract, accounts, companyID, journalID, createdBy)
}

// BuildOperatingIncomeEntry creates a draft for one period's straight-line
// lease income, cash receipt, and deferred/accrued movement.
func (b *LessorEntryBuilder) BuildOperatingIncomeEntry(
	ctx context.Context, period StraightLineLeasePeriod, companyID, journalID uuid.UUID,
	mappingID, createdBy string,
) (journal.JournalEntry, error) {
	accounts, err := b.resolve(ctx, companyID, mappingID)
	if err != nil {
		return journal.JournalEntry{}, err
	}
	return buildOperatingIncomeEntry(period, accounts, companyID, journalID, createdBy)
}

// BuildOperatingDepreciationEntry creates a draft for one period's leased-asset
// depreciation.
func (b *LessorEntryBuilder) BuildOperatingDepreciationEntry(
	ctx context.Context, period StraightLineLeasePeriod, companyID, journalID uuid.UUID,
	mappingID, createdBy string,
) (journal.JournalEntry, error) {
	accounts, err := b.resolve(ctx, companyID, mappingID)
	if err != nil {
		return journal.JournalEntry{}, err
	}
	return buildOperatingDepreciationEntry(period, accounts, companyID, journalID, createdBy)
}

// BuildFinanceCommencementEntry derecognizes the leased asset, recognizes the
// net investment in the lease, and books the resulting gain or loss.
func (b *LessorEntryBuilder) BuildFinanceCommencementEntry(
	ctx context.Context, contract FinanceLeaseContract, companyID, journalID uuid.UUID,
	mappingID, createdBy string,
) (journal.JournalEntry, error) {
	accounts, err := b.resolve(ctx, companyID, mappingID)
	if err != nil {
		return journal.JournalEntry{}, err
	}
	return buildFinanceCommencementEntry(contract, accounts, companyID, journalID, createdBy)
}

// BuildFinanceReceiptEntry creates a draft for one period's cash receipt,
// finance income, and net-investment recovery.
func (b *LessorEntryBuilder) BuildFinanceReceiptEntry(
	ctx context.Context, period FinanceLeasePeriod, companyID, journalID uuid.UUID,
	mappingID, createdBy string,
) (journal.JournalEntry, error) {
	accounts, err := b.resolve(ctx, companyID, mappingID)
	if err != nil {
		return journal.JournalEntry{}, err
	}
	return buildFinanceReceiptEntry(period, accounts, companyID, journalID, createdBy)
}

// BuildLessorOperatingIncomeEntry creates a balanced draft for one period's straight-line lease income.
func BuildLessorOperatingIncomeEntry(
	period StraightLineLeasePeriod, accounts LessorAccounts, companyID, journalID uuid.UUID, createdBy string,
) (journal.JournalEntry, error) {
	return buildOperatingIncomeEntry(period, accounts, companyID, journalID, createdBy)
}

// BuildLessorFinanceCommencementEntry creates a balanced draft derecognizing the asset and recognizing the net investment in lease.
func BuildLessorFinanceCommencementEntry(
	contract FinanceLeaseContract, accounts LessorAccounts, companyID, journalID uuid.UUID, createdBy string,
) (journal.JournalEntry, error) {
	return buildFinanceCommencementEntry(contract, accounts, companyID, journalID, createdBy)
}

// BuildLessorFinanceReceiptEntry creates a balanced draft for one period's finance lease receipt and finance income.
func BuildLessorFinanceReceiptEntry(
	period FinanceLeasePeriod, accounts LessorAccounts, companyID, journalID uuid.UUID, createdBy string,
) (journal.JournalEntry, error) {
	return buildFinanceReceiptEntry(period, accounts, companyID, journalID, createdBy)
}

func buildOperatingInitialDirectCostsEntry(
	contract OperatingLeaseContract, accounts LessorAccounts, companyID, journalID uuid.UUID, createdBy string,
) (journal.JournalEntry, error) {
	if strings.TrimSpace(contract.Reference) == "" {
		return journal.JournalEntry{}, ErrLeaseReferenceRequired
	}
	if !contract.InitialDirectCosts.Amount().IsPositive() {
		return journal.JournalEntry{}, ErrInvalidContract
	}
	if accounts.LeasedAsset == uuid.Nil || accounts.Cash == uuid.Nil {
		return journal.JournalEntry{}, ErrInvalidLessorAccounts
	}
	entry, err := newLeaseEntry(contract.CommencementDate, companyID, journalID, createdBy, []journal.EntryLine{
		debitLine(accounts.LeasedAsset, contract.InitialDirectCosts, nil),
		creditLine(accounts.Cash, contract.InitialDirectCosts, nil),
	})
	return identifyEntry(entry, contract.Reference, "lessor-operating-idc", err)
}

func buildOperatingIncomeEntry(
	period StraightLineLeasePeriod, accounts LessorAccounts, companyID, journalID uuid.UUID, createdBy string,
) (journal.JournalEntry, error) {
	if strings.TrimSpace(period.LeaseReference) == "" {
		return journal.JournalEntry{}, ErrLeaseReferenceRequired
	}
	if !period.LeaseIncome.Amount().IsPositive() || period.CashReceived.Currency() == "" ||
		accounts.Cash == uuid.Nil || accounts.OperatingLeaseIncome == uuid.Nil {
		return journal.JournalEntry{}, ErrInvalidLessorAccounts
	}
	lines := []journal.EntryLine{debitLine(accounts.Cash, period.CashReceived, nil)}
	movement := period.DeferredMovement.Amount()
	switch {
	case movement.IsPositive():
		// Straight-line income recognized ahead of cash: accrued rent receivable.
		if accounts.AccruedRentReceivable == uuid.Nil {
			return journal.JournalEntry{}, ErrInvalidLessorAccounts
		}
		accrual, err := types.NewMoney(movement, period.DeferredMovement.Currency())
		if err != nil {
			return journal.JournalEntry{}, err
		}
		lines = append(lines, debitLine(accounts.AccruedRentReceivable, accrual, nil))
	case movement.IsNegative():
		// Cash received ahead of straight-line income: deferred lease income.
		if accounts.DeferredLeaseIncome == uuid.Nil {
			return journal.JournalEntry{}, ErrInvalidLessorAccounts
		}
		deferral, err := types.NewMoney(movement.Abs(), period.DeferredMovement.Currency())
		if err != nil {
			return journal.JournalEntry{}, err
		}
		lines = append(lines, creditLine(accounts.DeferredLeaseIncome, deferral, nil))
	}
	lines = append(lines, creditLine(accounts.OperatingLeaseIncome, period.LeaseIncome, nil))
	entry, err := newLeaseEntry(period.DueDate, companyID, journalID, createdBy, lines)
	return identifyEntry(entry, period.LeaseReference,
		fmt.Sprintf("lessor-operating-income:%d:%s", period.Period, period.DueDate.Format("2006-01-02")), err)
}

func buildOperatingDepreciationEntry(
	period StraightLineLeasePeriod, accounts LessorAccounts, companyID, journalID uuid.UUID, createdBy string,
) (journal.JournalEntry, error) {
	if strings.TrimSpace(period.LeaseReference) == "" {
		return journal.JournalEntry{}, ErrLeaseReferenceRequired
	}
	if !period.AssetDepreciation.Amount().IsPositive() {
		return journal.JournalEntry{}, ErrAssetDepreciationUnav
	}
	if accounts.DepreciationExpense == uuid.Nil || accounts.AccumulatedDepreciation == uuid.Nil {
		return journal.JournalEntry{}, ErrInvalidLessorAccounts
	}
	entry, err := newLeaseEntry(period.DueDate, companyID, journalID, createdBy, []journal.EntryLine{
		debitLine(accounts.DepreciationExpense, period.AssetDepreciation, nil),
		creditLine(accounts.AccumulatedDepreciation, period.AssetDepreciation, nil),
	})
	return identifyEntry(entry, period.LeaseReference,
		fmt.Sprintf("lessor-operating-depreciation:%d:%s", period.Period, period.DueDate.Format("2006-01-02")), err)
}

func buildFinanceCommencementEntry(
	contract FinanceLeaseContract, accounts LessorAccounts, companyID, journalID uuid.UUID, createdBy string,
) (journal.JournalEntry, error) {
	if strings.TrimSpace(contract.Reference) == "" {
		return journal.JournalEntry{}, ErrLeaseReferenceRequired
	}
	if contract.NetInvestment.Currency() == "" || accounts.NetInvestmentInLease == uuid.Nil ||
		accounts.LeasedAsset == uuid.Nil {
		return journal.JournalEntry{}, ErrInvalidLessorAccounts
	}
	lines := []journal.EntryLine{debitLine(accounts.NetInvestmentInLease, contract.NetInvestment, nil)}
	if contract.AssetCarryingAmount.Amount().IsPositive() {
		lines = append(lines, creditLine(accounts.LeasedAsset, contract.AssetCarryingAmount, nil))
	}
	if contract.DerecognitionGainLoss.Amount().IsPositive() {
		if accounts.DerecognitionGain == uuid.Nil {
			return journal.JournalEntry{}, ErrInvalidLessorAccounts
		}
		lines = append(lines, creditLine(accounts.DerecognitionGain, contract.DerecognitionGainLoss, nil))
	}
	if contract.DerecognitionGainLoss.Amount().IsNegative() {
		if accounts.DerecognitionLoss == uuid.Nil {
			return journal.JournalEntry{}, ErrInvalidLessorAccounts
		}
		loss, err := types.NewMoney(contract.DerecognitionGainLoss.Amount().Abs(), contract.DerecognitionGainLoss.Currency())
		if err != nil {
			return journal.JournalEntry{}, err
		}
		lines = append(lines, debitLine(accounts.DerecognitionLoss, loss, nil))
	}
	if contract.InitialDirectCosts.Amount().IsPositive() {
		if accounts.Cash == uuid.Nil {
			return journal.JournalEntry{}, ErrInvalidLessorAccounts
		}
		lines = append(lines, creditLine(accounts.Cash, contract.InitialDirectCosts, nil))
	}
	entry, err := newLeaseEntry(contract.CommencementDate, companyID, journalID, createdBy, lines)
	return identifyEntry(entry, contract.Reference, "lessor-finance-commencement", err)
}

func buildFinanceReceiptEntry(
	period FinanceLeasePeriod, accounts LessorAccounts, companyID, journalID uuid.UUID, createdBy string,
) (journal.JournalEntry, error) {
	if strings.TrimSpace(period.LeaseReference) == "" {
		return journal.JournalEntry{}, ErrLeaseReferenceRequired
	}
	if !period.CashReceived.Amount().IsPositive() || accounts.Cash == uuid.Nil ||
		accounts.NetInvestmentInLease == uuid.Nil {
		return journal.JournalEntry{}, ErrInvalidLessorAccounts
	}
	lines := []journal.EntryLine{debitLine(accounts.Cash, period.CashReceived, nil)}
	if period.FinanceIncome.Amount().IsPositive() {
		if accounts.FinanceIncome == uuid.Nil {
			return journal.JournalEntry{}, ErrInvalidLessorAccounts
		}
		lines = append(lines, creditLine(accounts.FinanceIncome, period.FinanceIncome, nil))
	}
	if period.PrincipalRecovery.Amount().IsPositive() {
		lines = append(lines, creditLine(accounts.NetInvestmentInLease, period.PrincipalRecovery, nil))
	} else if period.PrincipalRecovery.Amount().IsNegative() {
		increase, err := types.NewMoney(period.PrincipalRecovery.Amount().Abs(), period.PrincipalRecovery.Currency())
		if err != nil {
			return journal.JournalEntry{}, err
		}
		lines = append(lines, debitLine(accounts.NetInvestmentInLease, increase, nil))
	}
	entry, err := newLeaseEntry(period.DueDate, companyID, journalID, createdBy, lines)
	return identifyEntry(entry, period.LeaseReference,
		fmt.Sprintf("lessor-finance-receipt:%d:%s", period.Period, period.DueDate.Format("2006-01-02")), err)
}

func normalizeMoney(amount types.Money, currency string) types.Money {
	if amount.Currency() == currency {
		return amount
	}
	return mustZero(currency)
}
