package ledger

import kernalLedger "github.com/osmai4603-collab/accounting_engine/internal/kernal/ledger"

type PeriodState = kernalLedger.PeriodState
type FiscalYearState = kernalLedger.FiscalYearState
type FiscalPeriodType = kernalLedger.FiscalPeriodType
type FiscalYear = kernalLedger.FiscalYear
type FiscalPeriod = kernalLedger.FiscalPeriod

const (
	PeriodOpen       = kernalLedger.PeriodOpen
	PeriodSoftClosed = kernalLedger.PeriodSoftClosed
	PeriodHardClosed = kernalLedger.PeriodHardClosed

	FiscalYearOpen       = kernalLedger.FiscalYearOpen
	FiscalYearSoftClosed = kernalLedger.FiscalYearSoftClosed
	FiscalYearHardClosed = kernalLedger.FiscalYearHardClosed

	FiscalPeriodTypeNormal            = kernalLedger.FiscalPeriodTypeNormal
	FiscalPeriodTypeYearEndAdjustment = kernalLedger.FiscalPeriodTypeYearEndAdjustment
	FiscalPeriodTypeAuditAdjustment   = kernalLedger.FiscalPeriodTypeAuditAdjustment
)
