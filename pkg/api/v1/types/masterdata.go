// Package types re-exports the public master-data vocabulary through the
// sanctioned pkg/api/v1 bridge (see docs/ard/P7_01_public_surface_decision.md).
// SPI and SDK consumers must use these names instead of importing internal/*.
package types

import (
	kernelMasterdata "github.com/osmai4603-collab/accounting_engine/internal/kernel/masterdata"
)

type AccountType = kernelMasterdata.AccountType

const (
	AccountTypeAsset     = kernelMasterdata.AccountTypeAsset
	AccountTypeLiability = kernelMasterdata.AccountTypeLiability
	AccountTypeEquity    = kernelMasterdata.AccountTypeEquity
	AccountTypeIncome    = kernelMasterdata.AccountTypeIncome
	AccountTypeExpense   = kernelMasterdata.AccountTypeExpense
)

type NormalBalance = kernelMasterdata.NormalBalance

const (
	NormalBalanceDebit  = kernelMasterdata.NormalBalanceDebit
	NormalBalanceCredit = kernelMasterdata.NormalBalanceCredit
)

type FiscalYearState = kernelMasterdata.FiscalYearState
type PeriodState = kernelMasterdata.PeriodState
type FiscalPeriodType = kernelMasterdata.FiscalPeriodType

const (
	FiscalYearOpen                    = kernelMasterdata.FiscalYearOpen
	FiscalYearSoftClosed              = kernelMasterdata.FiscalYearSoftClosed
	FiscalYearHardClosed              = kernelMasterdata.FiscalYearHardClosed
	PeriodOpen                        = kernelMasterdata.PeriodOpen
	PeriodSoftClosed                  = kernelMasterdata.PeriodSoftClosed
	PeriodHardClosed                  = kernelMasterdata.PeriodHardClosed
	FiscalPeriodTypeNormal            = kernelMasterdata.FiscalPeriodTypeNormal
	FiscalPeriodTypeYearEndAdjustment = kernelMasterdata.FiscalPeriodTypeYearEndAdjustment
	FiscalPeriodTypeAuditAdjustment   = kernelMasterdata.FiscalPeriodTypeAuditAdjustment
)

type Account = kernelMasterdata.Account
type FiscalYear = kernelMasterdata.FiscalYear
type FiscalPeriod = kernelMasterdata.FiscalPeriod

// Public re-exports of the company-scoped analytical master data vocabulary
// (ADR P11-02). They remain aliases so callers never import internal/*.
type CostCenter = kernelMasterdata.CostCenter
type AnalyticalDimensionValue = kernelMasterdata.AnalyticalDimensionValue
type AnalyticalDimensionKind = kernelMasterdata.AnalyticalDimensionKind

const (
	DimensionProject      = kernelMasterdata.DimensionProject
	DimensionSegment      = kernelMasterdata.DimensionSegment
	DimensionBusinessUnit = kernelMasterdata.DimensionBusinessUnit
)
