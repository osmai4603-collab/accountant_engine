// Package types re-exports the public master-data vocabulary through the
// sanctioned pkg/api/v1 bridge (see docs/ard/P7_01_public_surface_decision.md).
// SPI and SDK consumers must use these names instead of importing internal/*.
package types

import (
	kernalMasterdata "github.com/osmai4603-collab/accounting_engine/internal/kernal/masterdata"
)

type AccountType = kernalMasterdata.AccountType

const (
	AccountTypeAsset     = kernalMasterdata.AccountTypeAsset
	AccountTypeLiability = kernalMasterdata.AccountTypeLiability
	AccountTypeEquity    = kernalMasterdata.AccountTypeEquity
	AccountTypeIncome    = kernalMasterdata.AccountTypeIncome
	AccountTypeExpense   = kernalMasterdata.AccountTypeExpense
)

type NormalBalance = kernalMasterdata.NormalBalance

const (
	NormalBalanceDebit  = kernalMasterdata.NormalBalanceDebit
	NormalBalanceCredit = kernalMasterdata.NormalBalanceCredit
)

type FiscalYearState = kernalMasterdata.FiscalYearState
type PeriodState = kernalMasterdata.PeriodState
type FiscalPeriodType = kernalMasterdata.FiscalPeriodType

const (
	FiscalYearOpen                    = kernalMasterdata.FiscalYearOpen
	FiscalYearSoftClosed              = kernalMasterdata.FiscalYearSoftClosed
	FiscalYearHardClosed              = kernalMasterdata.FiscalYearHardClosed
	PeriodOpen                        = kernalMasterdata.PeriodOpen
	PeriodSoftClosed                  = kernalMasterdata.PeriodSoftClosed
	PeriodHardClosed                  = kernalMasterdata.PeriodHardClosed
	FiscalPeriodTypeNormal            = kernalMasterdata.FiscalPeriodTypeNormal
	FiscalPeriodTypeYearEndAdjustment = kernalMasterdata.FiscalPeriodTypeYearEndAdjustment
	FiscalPeriodTypeAuditAdjustment   = kernalMasterdata.FiscalPeriodTypeAuditAdjustment
)

type Account = kernalMasterdata.Account
type FiscalYear = kernalMasterdata.FiscalYear
type FiscalPeriod = kernalMasterdata.FiscalPeriod

// Public re-exports of the company-scoped analytical master data vocabulary
// (ADR P11-02). They remain aliases so callers never import internal/*.
type CostCenter = kernalMasterdata.CostCenter
type AnalyticalDimensionValue = kernalMasterdata.AnalyticalDimensionValue
type AnalyticalDimensionKind = kernalMasterdata.AnalyticalDimensionKind

const (
	DimensionProject      = kernalMasterdata.DimensionProject
	DimensionSegment      = kernalMasterdata.DimensionSegment
	DimensionBusinessUnit = kernalMasterdata.DimensionBusinessUnit
)
