package types

import (
	kernalTypes "github.com/osmai4603-collab/accounting_engine/internal/kernal/types"
)

const (
	FinancialScale = kernalTypes.FinancialScale
	InternalScale  = kernalTypes.InternalScale
)

type Money = kernalTypes.Money

var (
	NewMoney            = kernalTypes.NewMoney
	NewMoneyFromDecimal = kernalTypes.NewMoneyFromDecimal
	NewMoneyFromString  = kernalTypes.NewMoneyFromString
	Zero                = kernalTypes.Zero
	CurrencyMinorUnit   = kernalTypes.CurrencyMinorUnit
)
