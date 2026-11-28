package ifrs16

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/reporting"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/standards"
	"github.com/shopspring/decimal"
)

// SaleAndLeasebackInput captures seller-lessee sale-and-leaseback terms (IFRS 16.98-103).
type SaleAndLeasebackInput struct {
	ContractID          uuid.UUID
	AssetCarryingAmount decimal.Decimal
	AssetFairValue      decimal.Decimal
	SaleProceeds        decimal.Decimal
	LeaseLiabilityPV    decimal.Decimal
	TransferSatisfied   bool // IFRS 15 sale criteria satisfied
	Currency            string
	EffectiveDate       time.Time
}

type SaleAndLeasebackResult struct {
	TransferSatisfied  bool
	ROUAsset           decimal.Decimal
	LeaseLiability     decimal.Decimal
	GainLossRecognized decimal.Decimal
	FinancingLiability decimal.Decimal
	Currency           string
}

// EvaluateSaleAndLeaseback measures seller-lessee ROU asset (retained proportion)
// and gain/loss on rights transferred (IFRS 16.100), or financing liability if not a sale (IFRS 16.103).
func EvaluateSaleAndLeaseback(in SaleAndLeasebackInput) (SaleAndLeasebackResult, error) {
	if in.ContractID == uuid.Nil || strings.TrimSpace(in.Currency) == "" || in.EffectiveDate.IsZero() {
		return SaleAndLeasebackResult{}, errors.New("ifrs16: contract id, currency, and effective date are required")
	}
	if !in.TransferSatisfied {
		return SaleAndLeasebackResult{
			TransferSatisfied:  false,
			FinancingLiability: in.SaleProceeds,
			Currency:           in.Currency,
		}, nil
	}
	if in.AssetFairValue.LessThanOrEqual(decimal.Zero) {
		return SaleAndLeasebackResult{}, errors.New("ifrs16: asset fair value must be positive")
	}
	// Adjust lease liability for off-market terms (IFRS 16.101)
	offMarket := in.SaleProceeds.Sub(in.AssetFairValue)
	adjustedLiability := in.LeaseLiabilityPV.Sub(offMarket)
	if adjustedLiability.IsNegative() {
		adjustedLiability = decimal.Zero
	}
	retainedRatio := adjustedLiability.Div(in.AssetFairValue)
	if retainedRatio.GreaterThan(decimal.NewFromInt(1)) {
		retainedRatio = decimal.NewFromInt(1)
	}
	rou := in.AssetCarryingAmount.Mul(retainedRatio).Round(2)
	totalGain := in.AssetFairValue.Sub(in.AssetCarryingAmount)
	recognizedGain := totalGain.Mul(decimal.NewFromInt(1).Sub(retainedRatio)).Round(2)
	return SaleAndLeasebackResult{
		TransferSatisfied:  true,
		ROUAsset:           rou,
		LeaseLiability:     adjustedLiability.Round(2),
		GainLossRecognized: recognizedGain,
		Currency:           in.Currency,
	}, nil
}

type SubleaseClassification string

const (
	SubleaseOperating SubleaseClassification = "operating"
	SubleaseFinance   SubleaseClassification = "finance"
)

type SubleaseInput struct {
	SubleaseID           uuid.UUID
	HeadLeaseROUCarrying decimal.Decimal
	SubleaseReceiptsPV   decimal.Decimal
	Classification       SubleaseClassification
	Currency             string
}

type SubleaseResult struct {
	Classification     SubleaseClassification
	NetInvestment      decimal.Decimal
	ROUDerecognized    decimal.Decimal
	GainLossOnSublease decimal.Decimal
	Currency           string
}

// ClassifyAndMeasureSublease measures an intermediate lessor's sublease (IFRS 16.B58).
func ClassifyAndMeasureSublease(in SubleaseInput) (SubleaseResult, error) {
	if in.SubleaseID == uuid.Nil || strings.TrimSpace(in.Currency) == "" {
		return SubleaseResult{}, errors.New("ifrs16: sublease id and currency are required")
	}
	switch in.Classification {
	case SubleaseOperating:
		return SubleaseResult{
			Classification: SubleaseOperating,
			Currency:       in.Currency,
		}, nil
	case SubleaseFinance:
		gainLoss := in.SubleaseReceiptsPV.Sub(in.HeadLeaseROUCarrying)
		return SubleaseResult{
			Classification:     SubleaseFinance,
			NetInvestment:      in.SubleaseReceiptsPV,
			ROUDerecognized:    in.HeadLeaseROUCarrying,
			GainLossOnSublease: gainLoss,
			Currency:           in.Currency,
		}, nil
	default:
		return SubleaseResult{}, fmt.Errorf("ifrs16: unsupported sublease classification %q", in.Classification)
	}
}

type VariablePaymentInput struct {
	LeaseID             uuid.UUID
	BasePayment         decimal.Decimal
	LinkedToIndexOrRate bool
	InitialIndex        decimal.Decimal
	CurrentIndex        decimal.Decimal
	UsageVolume         decimal.Decimal
	UsageRate           decimal.Decimal
	Currency            string
}

type VariablePaymentResult struct {
	LeaseLiabilityIncluded decimal.Decimal
	PeriodExpensePL        decimal.Decimal
	Currency               string
}

// RemeasureVariablePayments separates index/rate-linked payments (included in
// lease liability under IFRS 16.27(b)) from usage/sales-linked payments (P&L under IFRS 16.38(b)).
func RemeasureVariablePayments(in VariablePaymentInput) (VariablePaymentResult, error) {
	if in.LeaseID == uuid.Nil || strings.TrimSpace(in.Currency) == "" {
		return VariablePaymentResult{}, errors.New("ifrs16: lease id and currency are required")
	}
	if in.LinkedToIndexOrRate {
		if in.InitialIndex.LessThanOrEqual(decimal.Zero) || in.CurrentIndex.LessThanOrEqual(decimal.Zero) {
			return VariablePaymentResult{}, errors.New("ifrs16: initial and current index must be positive")
		}
		adjusted := in.BasePayment.Mul(in.CurrentIndex).Div(in.InitialIndex).Round(2)
		return VariablePaymentResult{
			LeaseLiabilityIncluded: adjusted,
			Currency:               in.Currency,
		}, nil
	}
	usageExpense := in.UsageVolume.Mul(in.UsageRate).Round(2)
	return VariablePaymentResult{
		PeriodExpensePL: usageExpense,
		Currency:        in.Currency,
	}, nil
}

// ReassessIndexOrRate computes the lease liability and ROU asset adjustment when
// index/rate-linked payments change (IFRS 16.42(b)).
func ReassessIndexOrRate(currentLiability, revisedPV, currentROU decimal.Decimal) (newLiability, newROU, delta decimal.Decimal) {
	delta = revisedPV.Sub(currentLiability)
	newLiability = revisedPV
	newROU = currentROU.Add(delta)
	if newROU.IsNegative() {
		newROU = decimal.Zero
	}
	return newLiability, newROU, delta
}

// AssessROUImpairment measures IAS 36 impairment on a right-of-use asset (IFRS 16.33).
func AssessROUImpairment(rouCarrying, recoverableAmount decimal.Decimal) (newROU, impairmentLoss decimal.Decimal) {
	if recoverableAmount.LessThan(rouCarrying) {
		impairmentLoss = rouCarrying.Sub(recoverableAmount)
		return recoverableAmount, impairmentLoss
	}
	return rouCarrying, decimal.Zero
}

// TerminateLease computes derecognition of lease liability and ROU asset on
// full or partial termination (IFRS 16.46(a)).
func TerminateLease(liabilityCarrying, rouCarrying, terminationPenalty decimal.Decimal) (gainLoss decimal.Decimal) {
	return liabilityCarrying.Sub(rouCarrying).Sub(terminationPenalty)
}

// SetProfileConfig configures the host's IFRS 16 edition, effective date, and jurisdiction.
func (m *Module) SetProfileConfig(cfg standards.StandardProfileConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.profileConfig = cfg
}

// SetDisclosures configures a durable disclosure provider for IFRS 16.
func (m *Module) SetDisclosures(provider standards.DisclosureProvider) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.disclosures = provider
}

// GetDisclosures implements standards.DisclosureProvider for IFRS 16.
func (m *Module) GetDisclosures(ctx context.Context, query reporting.DisclosureQuery) ([]reporting.StructuredDisclosure, error) {
	if m == nil {
		return nil, fmt.Errorf("IFRS 16: module is nil")
	}
	if ctx == nil {
		return nil, fmt.Errorf("IFRS 16: context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.RLock()
	provider := m.disclosures
	m.mu.RUnlock()
	if provider == nil {
		return []reporting.StructuredDisclosure{}, nil
	}
	return provider.GetDisclosures(ctx, query)
}
