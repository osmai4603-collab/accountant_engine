package ifrs16

import (
	"errors"
	"fmt"
	"time"

	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/shopspring/decimal"
)

var (
	ErrInvalidContract        = errors.New("invalid lease contract")
	ErrInvalidPayment         = errors.New("invalid lease payment")
	ErrInvalidAccounts        = errors.New("invalid lease journal accounts")
	ErrInvalidCarrying        = errors.New("invalid lease carrying amount")
	ErrNegativeLiability      = errors.New("lease payment exceeds accrued liability")
	ErrDepreciationUnavail    = errors.New("no depreciation due for this period")
	ErrLeaseReferenceRequired = errors.New("lease reference is required for entry generation")
)

// LeasePayment is a contractual payment due on a specified date.
type LeasePayment struct {
	DueDate time.Time   `json:"due_date"`
	Amount  types.Money `json:"amount"`
}

// LeaseContract contains the inputs and calculated commencement balances for a lease.
type LeaseContract struct {
	Reference             string          `json:"reference,omitempty"`
	CommencementDate      time.Time       `json:"commencement_date"`
	Currency              string          `json:"currency"`
	AnnualDiscountRate    decimal.Decimal `json:"annual_discount_rate"`
	Payments              []LeasePayment  `json:"payments"`
	InitialDirectCosts    types.Money     `json:"initial_direct_costs"`
	Prepayments           types.Money     `json:"prepayments"`
	LeaseIncentives       types.Money     `json:"lease_incentives"`
	UsefulLifePeriods     int             `json:"useful_life_periods"`
	InitialLeaseLiability types.Money     `json:"initial_lease_liability"`
	InitialROUAsset       types.Money     `json:"initial_rou_asset"`
}

// SchedulePeriod is one dated liability and ROU depreciation roll-forward.
type SchedulePeriod struct {
	LeaseReference          string
	Period                  int
	DueDate                 time.Time
	Payment                 types.Money
	OpeningLiability        types.Money
	InterestExpense         types.Money
	PrincipalReduction      types.Money
	ClosingLiability        types.Money
	ROUDepreciation         types.Money
	AccumulatedDepreciation types.Money
	ClosingROUAsset         types.Money
}

// WithReference returns a copy identified by the caller's stable contract key.
// The reference is required when generating journal drafts.
func (c LeaseContract) WithReference(reference string) LeaseContract {
	c.Reference = reference
	return c
}

// NewLeaseContract validates lease terms and calculates commencement balances.
func NewLeaseContract(
	commencementDate time.Time,
	annualDiscountRate decimal.Decimal,
	payments []LeasePayment,
	initialDirectCosts, prepayments, leaseIncentives types.Money,
	usefulLifePeriods int,
) (LeaseContract, error) {
	if commencementDate.IsZero() || len(payments) == 0 || annualDiscountRate.IsNegative() ||
		usefulLifePeriods < 0 {
		return LeaseContract{}, ErrInvalidContract
	}
	currency := initialDirectCosts.Currency()
	if currency == "" {
		currency = prepayments.Currency()
	}
	if currency == "" {
		currency = leaseIncentives.Currency()
	}
	if currency == "" && len(payments) != 0 {
		currency = payments[0].Amount.Currency()
	}
	if currency == "" {
		return LeaseContract{}, ErrInvalidContract
	}
	if err := validateNonnegativeMoney(initialDirectCosts, currency); err != nil {
		return LeaseContract{}, err
	}
	if err := validateNonnegativeMoney(prepayments, currency); err != nil {
		return LeaseContract{}, err
	}
	if err := validateNonnegativeMoney(leaseIncentives, currency); err != nil {
		return LeaseContract{}, err
	}

	copyPayments := append([]LeasePayment(nil), payments...)
	lastDate := commencementDate.UTC()
	for i := range copyPayments {
		payment := &copyPayments[i]
		if payment.DueDate.IsZero() || !payment.DueDate.After(lastDate) ||
			payment.Amount.Currency() != currency || !payment.Amount.Amount().IsPositive() {
			return LeaseContract{}, ErrInvalidPayment
		}
		if _, err := types.NewMoney(payment.Amount.Amount(), currency); err != nil {
			return LeaseContract{}, err
		}
		payment.DueDate = payment.DueDate.UTC()
		lastDate = payment.DueDate
	}

	liability, err := PresentValue(copyPayments, commencementDate, annualDiscountRate, currency)
	if err != nil {
		return LeaseContract{}, err
	}
	rou, err := liability.Add(initialDirectCosts)
	if err != nil {
		return LeaseContract{}, err
	}
	rou, err = rou.Add(prepayments)
	if err != nil {
		return LeaseContract{}, err
	}
	rou, err = rou.Subtract(leaseIncentives)
	if err != nil {
		return LeaseContract{}, err
	}
	if !rou.Amount().IsPositive() {
		return LeaseContract{}, ErrInvalidContract
	}

	return LeaseContract{
		CommencementDate:      commencementDate.UTC(),
		Currency:              currency,
		AnnualDiscountRate:    annualDiscountRate,
		Payments:              copyPayments,
		InitialDirectCosts:    initialDirectCosts,
		Prepayments:           prepayments,
		LeaseIncentives:       leaseIncentives,
		UsefulLifePeriods:     usefulLifePeriods,
		InitialLeaseLiability: liability,
		InitialROUAsset:       rou,
	}, nil
}

// AmortizationSchedule returns the complete schedule without mutating the contract.
func (c LeaseContract) AmortizationSchedule() ([]SchedulePeriod, error) {
	return BuildAmortizationSchedule(c)
}

// ModificationResult describes remeasurement of the remaining lease at a modification date.
type ModificationResult struct {
	LeaseReference      string
	EffectiveDate       time.Time
	OldLeaseLiability   types.Money
	RemeasuredLiability types.Money
	LiabilityAdjustment types.Money
	OldROUAsset         types.Money
	NewROUAsset         types.Money
	ROUAdjustment       types.Money
	ModificationGain    types.Money
	DiscountRate        decimal.Decimal
	RemainingPayments   []LeasePayment
}

// RecalculateModification remeasures the liability and adjusts the ROU for a non-separate modification.
// Any liability reduction beyond the remaining ROU carrying amount is recognized as a gain.
func (c LeaseContract) RecalculateModification(
	effectiveDate time.Time,
	remainingPayments []LeasePayment,
	newAnnualDiscountRate decimal.Decimal,
	liabilityCarryingAmount, rouCarryingAmount types.Money,
) (ModificationResult, error) {
	if effectiveDate.IsZero() || effectiveDate.Before(c.CommencementDate) || newAnnualDiscountRate.IsNegative() ||
		!validCarrying(liabilityCarryingAmount, c.Currency) ||
		!validCarrying(rouCarryingAmount, c.Currency) || len(remainingPayments) == 0 {
		return ModificationResult{}, ErrInvalidCarrying
	}
	payments, err := validateRemainingPayments(effectiveDate, remainingPayments, c.Currency)
	if err != nil {
		return ModificationResult{}, err
	}
	remeasured, err := PresentValue(payments, effectiveDate, newAnnualDiscountRate, c.Currency)
	if err != nil {
		return ModificationResult{}, err
	}
	delta, err := remeasured.Subtract(liabilityCarryingAmount)
	if err != nil {
		return ModificationResult{}, err
	}
	rouAdjustment := delta
	gain := mustZero(c.Currency)
	newROU := rouCarryingAmount
	if delta.Amount().IsNegative() && delta.Amount().Abs().GreaterThan(rouCarryingAmount.Amount()) {
		rouAdjustment, err = types.NewMoney(rouCarryingAmount.Amount().Neg(), c.Currency)
		if err != nil {
			return ModificationResult{}, err
		}
		gain, err = types.NewMoney(delta.Amount().Abs().Sub(rouCarryingAmount.Amount()), c.Currency)
		if err != nil {
			return ModificationResult{}, err
		}
		newROU = mustZero(c.Currency)
	} else {
		newROU, err = rouCarryingAmount.Add(rouAdjustment)
		if err != nil {
			return ModificationResult{}, err
		}
	}
	return ModificationResult{
		LeaseReference:      c.Reference,
		EffectiveDate:       effectiveDate.UTC(),
		OldLeaseLiability:   liabilityCarryingAmount,
		RemeasuredLiability: remeasured,
		LiabilityAdjustment: delta,
		OldROUAsset:         rouCarryingAmount,
		NewROUAsset:         newROU,
		ROUAdjustment:       rouAdjustment,
		ModificationGain:    gain,
		DiscountRate:        newAnnualDiscountRate,
		RemainingPayments:   payments,
	}, nil
}

// TerminationResult contains the derecognition amounts and resulting gain or loss.
type TerminationResult struct {
	LeaseReference      string
	EffectiveDate       time.Time
	LeaseLiability      types.Money
	ROUAsset            types.Money
	SettlementPayment   types.Money
	TerminationGainLoss types.Money // Positive is a gain; negative is a loss.
}

// EarlyTerminate derecognizes the remaining liability and ROU, recognizing the net settlement gain or loss.
func (c LeaseContract) EarlyTerminate(
	effectiveDate time.Time,
	liabilityCarryingAmount, rouCarryingAmount, settlementPayment types.Money,
) (TerminationResult, error) {
	if effectiveDate.IsZero() || effectiveDate.Before(c.CommencementDate) ||
		!validCarrying(liabilityCarryingAmount, c.Currency) ||
		!validCarrying(rouCarryingAmount, c.Currency) || !validCarrying(settlementPayment, c.Currency) {
		return TerminationResult{}, ErrInvalidCarrying
	}
	gainLoss := liabilityCarryingAmount.Amount().Sub(rouCarryingAmount.Amount()).Sub(settlementPayment.Amount())
	result, err := types.NewMoney(gainLoss, c.Currency)
	if err != nil {
		return TerminationResult{}, err
	}
	return TerminationResult{
		LeaseReference:      c.Reference,
		EffectiveDate:       effectiveDate.UTC(),
		LeaseLiability:      liabilityCarryingAmount,
		ROUAsset:            rouCarryingAmount,
		SettlementPayment:   settlementPayment,
		TerminationGainLoss: result,
	}, nil
}

func validateNonnegativeMoney(m types.Money, currency string) error {
	if m.Currency() != currency || m.IsNegative() {
		return fmt.Errorf("%w: currency or amount", ErrInvalidContract)
	}
	_, err := types.NewMoney(m.Amount(), currency)
	return err
}

func validateRemainingPayments(date time.Time, payments []LeasePayment, currency string) ([]LeasePayment, error) {
	result := append([]LeasePayment(nil), payments...)
	last := date.UTC()
	for i := range result {
		payment := &result[i]
		if payment.DueDate.IsZero() || !payment.DueDate.After(last) ||
			payment.Amount.Currency() != currency || !payment.Amount.Amount().IsPositive() {
			return nil, ErrInvalidPayment
		}
		payment.DueDate = payment.DueDate.UTC()
		last = payment.DueDate
	}
	return result, nil
}

func validCarrying(amount types.Money, currency string) bool {
	return amount.Currency() == currency && !amount.IsNegative()
}
