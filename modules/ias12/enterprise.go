package ias12

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/reporting"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/standards"
	"github.com/shopspring/decimal"
)

type CurrentTaxFullInput struct {
	CompanyID               uuid.UUID
	PeriodEnd               time.Time
	AccountingProfit        decimal.Decimal
	PermanentAdditions      decimal.Decimal
	PermanentDeductions     decimal.Decimal
	TemporaryNetAdjustment  decimal.Decimal
	TaxLossCarryforwardUsed decimal.Decimal
	StatutoryRate           decimal.Decimal // e.g. 0.25
	TaxCreditsApplied       decimal.Decimal
	Currency                string
	Jurisdiction            string
}

type CurrentTaxFullResult struct {
	TaxableProfit       decimal.Decimal
	GrossCurrentTax     decimal.Decimal
	NetCurrentTax       decimal.Decimal
	LossCarryforwardNet decimal.Decimal
	EffectiveTaxRate    decimal.Decimal
	Currency            string
	Jurisdiction        string
}

// CalculateCurrentTaxFull computes full current tax including permanent/temporary
// jurisdictional adjustments, tax loss carryforwards, and tax credits (IAS 12.12-14).
func CalculateCurrentTaxFull(in CurrentTaxFullInput) (CurrentTaxFullResult, error) {
	if in.CompanyID == uuid.Nil || in.PeriodEnd.IsZero() || strings.TrimSpace(in.Currency) == "" {
		return CurrentTaxFullResult{}, errors.New("ias12: company id, period end, and currency are required")
	}
	if in.StatutoryRate.IsNegative() || in.StatutoryRate.GreaterThan(decimal.NewFromInt(1)) {
		return CurrentTaxFullResult{}, errors.New("ias12: statutory rate must be between 0 and 1")
	}
	if in.TaxLossCarryforwardUsed.IsNegative() || in.TaxCreditsApplied.IsNegative() {
		return CurrentTaxFullResult{}, errors.New("ias12: loss carryforward and tax credits must be non-negative")
	}

	adjustedProfit := in.AccountingProfit.
		Add(in.PermanentAdditions).
		Sub(in.PermanentDeductions).
		Add(in.TemporaryNetAdjustment)

	taxableProfit := adjustedProfit
	if taxableProfit.IsPositive() && in.TaxLossCarryforwardUsed.IsPositive() {
		used := in.TaxLossCarryforwardUsed
		if used.GreaterThan(taxableProfit) {
			used = taxableProfit
		}
		taxableProfit = taxableProfit.Sub(used)
	}
	if taxableProfit.IsNegative() {
		taxableProfit = decimal.Zero
	}

	grossTax := taxableProfit.Mul(in.StatutoryRate).Round(2)
	netTax := grossTax.Sub(in.TaxCreditsApplied)
	if netTax.IsNegative() {
		netTax = decimal.Zero
	}

	etr := decimal.Zero
	if !in.AccountingProfit.IsZero() {
		etr = netTax.Div(in.AccountingProfit).Round(4)
	}

	return CurrentTaxFullResult{
		TaxableProfit:       taxableProfit.Round(2),
		GrossCurrentTax:     grossTax,
		NetCurrentTax:       netTax.Round(2),
		LossCarryforwardNet: in.TaxLossCarryforwardUsed,
		EffectiveTaxRate:    etr,
		Currency:            in.Currency,
		Jurisdiction:        in.Jurisdiction,
	}, nil
}

type TaxPresentationInput struct {
	CompanyID               types.CompanyID
	PeriodStart             time.Time
	PeriodEnd               time.Time
	CurrentTaxAssets        decimal.Decimal
	CurrentTaxLiabilities   decimal.Decimal
	DeferredTaxAssets       decimal.Decimal
	DeferredTaxLiabilities  decimal.Decimal
	LegallyEnforceableRight bool // IAS 12.71 & 12.74 offsetting criterion
	CurrentTaxExpense       decimal.Decimal
	DeferredTaxExpense      decimal.Decimal
	Currency                string
}

type TaxPresentationResult struct {
	NetCurrentTaxAsset      decimal.Decimal
	NetCurrentTaxLiability  decimal.Decimal
	NetDeferredTaxAsset     decimal.Decimal
	NetDeferredTaxLiability decimal.Decimal
	TotalTaxExpense         decimal.Decimal
	Disclosure              reporting.StructuredDisclosure
}

// BuildTaxPresentationAndDisclosure applies IAS 12.71-76 offsetting rules and
// emits a structured IAS 12.79-88 presentation disclosure.
func BuildTaxPresentationAndDisclosure(in TaxPresentationInput) (TaxPresentationResult, error) {
	if in.CompanyID == (types.CompanyID{}) || strings.TrimSpace(in.Currency) == "" {
		return TaxPresentationResult{}, errors.New("ias12: company id and currency are required")
	}
	res := TaxPresentationResult{
		NetCurrentTaxAsset:      in.CurrentTaxAssets,
		NetCurrentTaxLiability:  in.CurrentTaxLiabilities,
		NetDeferredTaxAsset:     in.DeferredTaxAssets,
		NetDeferredTaxLiability: in.DeferredTaxLiabilities,
		TotalTaxExpense:         in.CurrentTaxExpense.Add(in.DeferredTaxExpense),
	}
	if in.LegallyEnforceableRight {
		netCur := in.CurrentTaxAssets.Sub(in.CurrentTaxLiabilities)
		if netCur.IsPositive() {
			res.NetCurrentTaxAsset = netCur
			res.NetCurrentTaxLiability = decimal.Zero
		} else {
			res.NetCurrentTaxAsset = decimal.Zero
			res.NetCurrentTaxLiability = netCur.Neg()
		}
		netDef := in.DeferredTaxAssets.Sub(in.DeferredTaxLiabilities)
		if netDef.IsPositive() {
			res.NetDeferredTaxAsset = netDef
			res.NetDeferredTaxLiability = decimal.Zero
		} else {
			res.NetDeferredTaxAsset = decimal.Zero
			res.NetDeferredTaxLiability = netDef.Neg()
		}
	}

	res.Disclosure = reporting.StructuredDisclosure{
		CompanyID:         in.CompanyID,
		Code:              "IAS12-TAX-PRESENTATION",
		Title:             "IAS 12.79-88 Tax Presentation & Offsetting",
		StandardReference: "IAS 12.79-88",
		Period: reporting.DisclosurePeriod{
			StartDate: in.PeriodStart.UTC(),
			EndDate:   in.PeriodEnd.UTC(),
		},
		Currency: in.Currency,
		Version:  "1.0.0",
		Data: map[string]any{
			"legally_enforceable_offset": in.LegallyEnforceableRight,
			"net_current_tax_asset":      res.NetCurrentTaxAsset.String(),
			"net_current_tax_liability":  res.NetCurrentTaxLiability.String(),
			"net_deferred_tax_asset":     res.NetDeferredTaxAsset.String(),
			"net_deferred_tax_liability": res.NetDeferredTaxLiability.String(),
			"current_tax_expense":        in.CurrentTaxExpense.String(),
			"deferred_tax_expense":       in.DeferredTaxExpense.String(),
			"total_tax_expense":          res.TotalTaxExpense.String(),
		},
	}
	return res, nil
}

// SetProfileConfig configures the host's IAS 12 edition, effective date, and jurisdiction.
func (m *Module) SetProfileConfig(cfg standards.StandardProfileConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.profileConfig = cfg
}
