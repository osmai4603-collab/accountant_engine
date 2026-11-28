package types

import (
	kernelTypes "github.com/osmai4603-collab/accounting_engine/internal/kernel/types"
)

const (
	FinancialScale = kernelTypes.FinancialScale
	InternalScale  = kernelTypes.InternalScale
)

type Money = kernelTypes.Money

var (
	NewMoney            = kernelTypes.NewMoney
	NewMoneyFromDecimal = kernelTypes.NewMoneyFromDecimal
	NewMoneyFromString  = kernelTypes.NewMoneyFromString
	Zero                = kernelTypes.Zero
	CurrencyMinorUnit   = kernelTypes.CurrencyMinorUnit
)
