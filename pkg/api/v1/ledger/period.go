package ledger

import kernelLedger "github.com/osmai4603-collab/accounting_engine/internal/kernel/ledger"

type PeriodState = kernelLedger.PeriodState
type FiscalYearState = kernelLedger.FiscalYearState
type FiscalPeriodType = kernelLedger.FiscalPeriodType
type FiscalYear = kernelLedger.FiscalYear
type FiscalPeriod = kernelLedger.FiscalPeriod

const (
	PeriodOpen       = kernelLedger.PeriodOpen
	PeriodSoftClosed = kernelLedger.PeriodSoftClosed
	PeriodHardClosed = kernelLedger.PeriodHardClosed

	FiscalYearOpen       = kernelLedger.FiscalYearOpen
	FiscalYearSoftClosed = kernelLedger.FiscalYearSoftClosed
	FiscalYearHardClosed = kernelLedger.FiscalYearHardClosed

	FiscalPeriodTypeNormal            = kernelLedger.FiscalPeriodTypeNormal
	FiscalPeriodTypeYearEndAdjustment = kernelLedger.FiscalPeriodTypeYearEndAdjustment
	FiscalPeriodTypeAuditAdjustment   = kernelLedger.FiscalPeriodTypeAuditAdjustment
)
