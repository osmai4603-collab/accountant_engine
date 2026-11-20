package valuation

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/osmai4603-collab/accounting_engine/internal/core/posting"
	"github.com/osmai4603-collab/accounting_engine/internal/core/types"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/ledger"
	apiTypes "github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/shopspring/decimal"
)

var ErrInvalidPosition = errors.New("invalid monetary position")

type PositionType string

const (
	Monetary    PositionType = "MONETARY"
	NonMonetary PositionType = "NON_MONETARY"
)

type MonetaryPosition struct {
	AccountID              apiTypes.AccountID
	ForeignCurrencyBalance decimal.Decimal
	TransactionCurrency    string
	FunctionalCurrency     string
	CarryingAmount         types.Money
	Type                   PositionType
	IsLiability            bool
}

type FXResult struct {
	AccountID              apiTypes.AccountID
	Currency               string
	Difference             types.Money
	IsGain                 bool
	ClosingRate            decimal.Decimal
	RateDate               time.Time
	RevaluedAmount         types.Money
	RateReference          ExchangeRate
	RateReferenceAvailable bool
	RateInverted           bool
}

type RateProvider interface {
	GetRate(ctx context.Context, rateType RateType, from, to string, asOf time.Time) (decimal.Decimal, ExchangeRate, bool, error)
}

type RevaluationEngine struct {
	provider RateProvider
}

func NewRevaluationEngine(provider RateProvider) *RevaluationEngine {
	return &RevaluationEngine{provider: provider}
}

func (e *RevaluationEngine) Calculate(ctx context.Context, position MonetaryPosition, asOf time.Time) (FXResult, error) {
	if position.Type != Monetary {
		return FXResult{}, ErrInvalidPosition
	}
	if position.AccountID == (apiTypes.AccountID{}) || position.TransactionCurrency == "" ||
		position.FunctionalCurrency == "" ||
		position.CarryingAmount.Currency() != position.FunctionalCurrency ||
		!position.ForeignCurrencyBalance.IsPositive() ||
		!position.CarryingAmount.Amount().IsPositive() {
		return FXResult{}, ErrInvalidPosition
	}
	if e.provider == nil {
		return FXResult{}, ErrRateNotFound
	}

	rate, ref, inverted, err := e.provider.GetRate(ctx, ClosingRate, position.TransactionCurrency, position.FunctionalCurrency, asOf)
	if err != nil {
		return FXResult{}, err
	}

	revaluedAmount, err := types.NewMoney(position.ForeignCurrencyBalance.Mul(rate), position.FunctionalCurrency)
	if err != nil {
		return FXResult{}, err
	}
	revaluedAmount, err = revaluedAmount.Rounded()
	if err != nil {
		return FXResult{}, err
	}

	diffAmt := revaluedAmount.Amount().Sub(position.CarryingAmount.Amount())
	diffMoney, err := types.NewMoney(diffAmt, position.FunctionalCurrency)
	if err != nil {
		return FXResult{}, err
	}

	isGain := diffAmt.IsPositive()
	if position.IsLiability {
		isGain = diffAmt.IsNegative()
	}

	return FXResult{
		AccountID:              position.AccountID,
		Currency:               position.TransactionCurrency,
		Difference:             diffMoney,
		IsGain:                 isGain,
		ClosingRate:            rate,
		RateDate:               asOf,
		RevaluedAmount:         revaluedAmount,
		RateReference:          ref,
		RateReferenceAvailable: true,
		RateInverted:           inverted,
	}, nil
}

type FXJournalAccounts struct {
	CompanyID        apiTypes.CompanyID
	JournalID        apiTypes.JournalID
	UnrealizedGainID apiTypes.AccountID
	UnrealizedLossID apiTypes.AccountID
}

// GenerateRevaluationEntry generates a strictly balanced double-entry for IAS 21 FX adjustments.
func GenerateRevaluationEntry(result FXResult, accounts FXJournalAccounts, date time.Time) (ledger.JournalEntry, error) {
	if result.Difference.Amount().IsZero() {
		return ledger.JournalEntry{}, ErrInvalidPosition
	}

	absDiff := result.Difference.Amount().Abs()
	funcAmt, err := apiTypes.NewMoney(absDiff, result.Difference.Currency())
	if err != nil {
		return ledger.JournalEntry{}, err
	}
	zeroAmt, err := apiTypes.Zero(result.Difference.Currency())
	if err != nil {
		return ledger.JournalEntry{}, err
	}

	entry := ledger.JournalEntry{
		ID:                  apiTypes.NewJournalEntryID(),
		CompanyID:           accounts.CompanyID,
		JournalID:           accounts.JournalID,
		EffectiveDate:       date.UTC(),
		PostingDate:         date.UTC(),
		Status:              ledger.EntryDraft,
		Classification:      ledger.EntryClassAdjusting,
		AdjustingEntryType:  ledger.AdjustingEntryAccrual,
		AdjustmentReason:    fmt.Sprintf("IAS 21 FX Revaluation for %s", result.Currency),
		ReferenceID:         fmt.Sprintf("FX-REVAL-%s-%s", result.Currency, date.Format("2006-01-02")),
		TransactionCurrency: result.Difference.Currency(),
		ExchangeRate:        decimal.NewFromInt(1),
		CreatedBy:           "system-valuation",
		CreatedAt:           time.Now().UTC(),
		UpdatedAt:           time.Now().UTC(),
	}

	var positionLine, pnlLine ledger.EntryLine

	if result.IsGain {
		// Gain: Asset revalued upward -> Debit Asset, Credit Gain OR Liability revalued downward -> Debit Liability, Credit Gain
		positionLine = ledger.EntryLine{
			ID:                  apiTypes.NewJournalEntryLineID(),
			AccountID:           result.AccountID,
			TransactionCurrency: result.Difference.Currency(),
			TransactionAmount:   funcAmt,
			ExchangeRate:        decimal.NewFromInt(1),
			FunctionalDebit:     funcAmt,
			FunctionalCredit:    zeroAmt,
		}
		pnlLine = ledger.EntryLine{
			ID:                  apiTypes.NewJournalEntryLineID(),
			AccountID:           accounts.UnrealizedGainID,
			TransactionCurrency: result.Difference.Currency(),
			TransactionAmount:   funcAmt,
			ExchangeRate:        decimal.NewFromInt(1),
			FunctionalDebit:     zeroAmt,
			FunctionalCredit:    funcAmt,
		}
	} else {
		// Loss: Debit Loss, Credit Asset/Liability
		pnlLine = ledger.EntryLine{
			ID:                  apiTypes.NewJournalEntryLineID(),
			AccountID:           accounts.UnrealizedLossID,
			TransactionCurrency: result.Difference.Currency(),
			TransactionAmount:   funcAmt,
			ExchangeRate:        decimal.NewFromInt(1),
			FunctionalDebit:     funcAmt,
			FunctionalCredit:    zeroAmt,
		}
		positionLine = ledger.EntryLine{
			ID:                  apiTypes.NewJournalEntryLineID(),
			AccountID:           result.AccountID,
			TransactionCurrency: result.Difference.Currency(),
			TransactionAmount:   funcAmt,
			ExchangeRate:        decimal.NewFromInt(1),
			FunctionalDebit:     zeroAmt,
			FunctionalCredit:    funcAmt,
		}
	}

	entry.Lines = []ledger.EntryLine{positionLine, pnlLine}

	if err := posting.ValidateEntryDoubleEntry(&entry, nil); err != nil {
		return ledger.JournalEntry{}, err
	}

	return entry, nil
}

