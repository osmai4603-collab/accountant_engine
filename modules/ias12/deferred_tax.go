package ias12

import (
	"fmt"
	"strings"
	"time"

	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/shopspring/decimal"
)

func (p MeasurementPeriod) Validate() error {
	if p.AsOf.IsZero() || (p.Kind != Quarterly && p.Kind != Annual) {
		return ErrInvalidMeasurementPeriod
	}
	return nil
}

func (r TaxRate) Validate(asOf time.Time) error {
	if r.Value.IsNegative() || r.Value.GreaterThan(decimal.NewFromInt(1)) {
		return ErrInvalidRate
	}
	if !r.Enacted || r.EnactedOn.IsZero() || r.EffectiveFrom.IsZero() ||
		r.EnactedOn.After(asOf) {
		return ErrRateNotEnacted
	}
	return nil
}

func Measure(balances []TaxBalance, period MeasurementPeriod) (Measurement, error) {
	if err := period.Validate(); err != nil {
		return Measurement{}, err
	}
	if len(balances) == 0 {
		return Measurement{}, ErrNoBalances
	}

	measurement := Measurement{Period: period}
	seen := make(map[string]struct{}, len(balances))
	var currency string
	var totalDTA, totalDTL decimal.Decimal
	for _, balance := range balances {
		if strings.TrimSpace(balance.ID) == "" || (balance.Kind != Asset && balance.Kind != Liability) {
			return Measurement{}, ErrInvalidInput
		}
		if _, exists := seen[balance.ID]; exists {
			return Measurement{}, fmt.Errorf("%w: duplicate balance ID %q", ErrInvalidInput, balance.ID)
		}
		seen[balance.ID] = struct{}{}
		if err := validateNonnegativeMoney(balance.CarryingAmount); err != nil {
			return Measurement{}, err
		}
		if err := validateNonnegativeMoney(balance.TaxBase); err != nil {
			return Measurement{}, err
		}
		if err := sameCurrency(balance.CarryingAmount, balance.TaxBase); err != nil {
			return Measurement{}, err
		}
		if err := balance.Rate.Validate(period.AsOf); err != nil {
			return Measurement{}, err
		}
		if currency == "" {
			currency = balance.CarryingAmount.Currency()
		} else if balance.CarryingAmount.Currency() != currency {
			return Measurement{}, ErrCurrencyMismatch
		}

		difference := balance.CarryingAmount.Amount().Sub(balance.TaxBase.Amount())
		var taxable, deductible decimal.Decimal
		if balance.Kind == Asset {
			if difference.IsPositive() {
				taxable = difference
			} else {
				deductible = difference.Abs()
			}
		} else if difference.IsPositive() {
			deductible = difference
		} else {
			taxable = difference.Abs()
		}
		recognizedDeductible := decimal.Zero
		if balance.RecognitionEvidence != nil {
			evidence := balance.RecognitionEvidence
			if err := validateNonnegativeMoney(evidence.SupportedDeductibleAmount); err != nil {
				return Measurement{}, err
			}
			if evidence.SupportedDeductibleAmount.Currency() != currency {
				return Measurement{}, ErrCurrencyMismatch
			}
			if evidence.Probable {
				recognizedDeductible = decimal.Min(deductible, evidence.SupportedDeductibleAmount.Amount())
			}
		} else if balance.FutureTaxableProfitProbable != nil && *balance.FutureTaxableProfitProbable {
			recognizedDeductible = deductible
		}
		dtaAmount := recognizedDeductible.Mul(balance.Rate.Value)
		dtlAmount := taxable.Mul(balance.Rate.Value)
		taxableMoney, err := types.NewMoney(taxable, currency)
		if err != nil {
			return Measurement{}, err
		}
		deductibleMoney, err := types.NewMoney(deductible, currency)
		if err != nil {
			return Measurement{}, err
		}
		dtaMoney, err := types.NewMoney(dtaAmount, currency)
		if err != nil {
			return Measurement{}, err
		}
		dtlMoney, err := types.NewMoney(dtlAmount, currency)
		if err != nil {
			return Measurement{}, err
		}
		measurement.Balances = append(measurement.Balances, MeasuredBalance{
			ID:                            balance.ID,
			Kind:                          balance.Kind,
			TaxableTemporaryDifference:    taxableMoney,
			DeductibleTemporaryDifference: deductibleMoney,
			DTA:                           dtaMoney,
			DTL:                           dtlMoney,
			Rate:                          balance.Rate,
			ExpectedReversalDate:          balance.ExpectedReversalDate,
			CarryingAmountSource:          balance.CarryingAmountSource,
			TaxBaseSource:                 balance.TaxBaseSource,
			RecognitionEvidenceReference:  evidenceReference(balance.RecognitionEvidence),
		})
		totalDTA = totalDTA.Add(dtaAmount)
		totalDTL = totalDTL.Add(dtlAmount)
	}
	measurement.Currency = currency
	var err error
	measurement.DTA, err = types.NewMoney(totalDTA, currency)
	if err != nil {
		return Measurement{}, err
	}
	measurement.DTL, err = types.NewMoney(totalDTL, currency)
	if err != nil {
		return Measurement{}, err
	}
	return measurement, nil
}

func Reconcile(input TaxReconciliationInput) (TaxReconciliationReport, error) {
	if err := input.Period.Validate(); err != nil {
		return TaxReconciliationReport{}, err
	}
	if err := validateMoney(input.AccountingProfitBeforeTax); err != nil {
		return TaxReconciliationReport{}, err
	}
	if err := validateSignedMoney(input.CurrentTaxExpense); err != nil {
		return TaxReconciliationReport{}, err
	}
	if err := validateSignedMoney(input.DeferredTaxExpense); err != nil {
		return TaxReconciliationReport{}, err
	}
	currency := input.AccountingProfitBeforeTax.Currency()
	if input.CurrentTaxExpense.Currency() != currency || input.DeferredTaxExpense.Currency() != currency {
		return TaxReconciliationReport{}, ErrCurrencyMismatch
	}
	if err := input.StatutoryRate.Validate(input.Period.AsOf); err != nil {
		return TaxReconciliationReport{}, err
	}

	permanentTax := decimal.Zero
	permanentDifferences := make([]TaxReconciliationLine, 0, len(input.PermanentDifferences))
	for _, difference := range input.PermanentDifferences {
		if strings.TrimSpace(difference.Description) == "" {
			return TaxReconciliationReport{}, ErrInvalidInput
		}
		if err := validateSignedMoney(difference.Amount); err != nil {
			return TaxReconciliationReport{}, err
		}
		if difference.Amount.Currency() != currency {
			return TaxReconciliationReport{}, ErrCurrencyMismatch
		}
		taxEffect := difference.Amount.Amount().Mul(input.StatutoryRate.Value)
		taxEffectMoney, err := types.NewMoney(taxEffect, currency)
		if err != nil {
			return TaxReconciliationReport{}, err
		}
		permanentDifferences = append(permanentDifferences, TaxReconciliationLine{
			Description: strings.TrimSpace(difference.Description),
			Amount:      difference.Amount,
			TaxEffect:   taxEffectMoney,
		})
		permanentTax = permanentTax.Add(taxEffect)
	}
	uncertainTreatments := append([]UncertainTaxMeasurement(nil), input.UncertainTaxTreatments...)
	seenTreatments := make(map[string]struct{}, len(uncertainTreatments))
	for _, treatment := range uncertainTreatments {
		if err := validateUncertainTaxMeasurement(treatment, currency); err != nil {
			return TaxReconciliationReport{}, err
		}
		if _, exists := seenTreatments[treatment.TreatmentID]; exists {
			return TaxReconciliationReport{}, fmt.Errorf("%w: duplicate uncertain tax treatment %q", ErrInvalidInput, treatment.TreatmentID)
		}
		seenTreatments[treatment.TreatmentID] = struct{}{}
	}

	expectedTax := input.AccountingProfitBeforeTax.Amount().Mul(input.StatutoryRate.Value)
	effectiveTax := input.CurrentTaxExpense.Amount().Add(input.DeferredTaxExpense.Amount())
	expectedMoney, err := types.NewMoney(expectedTax, currency)
	if err != nil {
		return TaxReconciliationReport{}, err
	}
	permanentMoney, err := types.NewMoney(permanentTax, currency)
	if err != nil {
		return TaxReconciliationReport{}, err
	}
	effectiveMoney, err := types.NewMoney(effectiveTax, currency)
	if err != nil {
		return TaxReconciliationReport{}, err
	}
	report := TaxReconciliationReport{
		Period:                     input.Period,
		AccountingProfitBeforeTax:  input.AccountingProfitBeforeTax,
		StatutoryTaxRate:           input.StatutoryRate.Value,
		ExpectedTaxExpense:         expectedMoney,
		PermanentDifferencesTax:    permanentMoney,
		PermanentDifferences:       permanentDifferences,
		CurrentTaxExpense:          input.CurrentTaxExpense,
		DeferredTaxExpense:         input.DeferredTaxExpense,
		UncertainTaxTreatments:     uncertainTreatments,
		TemporaryDifferencesChange: input.DeferredTaxExpense,
		EffectiveTaxExpense:        effectiveMoney,
	}
	if !input.AccountingProfitBeforeTax.Amount().IsZero() {
		report.EffectiveTaxRate = effectiveTax.Div(input.AccountingProfitBeforeTax.Amount())
		report.EffectiveTaxRateAvailable = true
	}
	return report, nil
}

func validateMoney(m types.Money) error {
	if m.Currency() == "" {
		return ErrInvalidInput
	}
	return nil
}

func validateNonnegativeMoney(m types.Money) error {
	if err := validateMoney(m); err != nil {
		return err
	}
	if m.IsNegative() {
		return types.ErrNegativeAmountProhibited
	}
	return nil
}

func validateSignedMoney(m types.Money) error {
	return validateMoney(m)
}

func sameCurrency(a, b types.Money) error {
	if a.Currency() != b.Currency() {
		return ErrCurrencyMismatch
	}
	return nil
}
