package types

import kernelTypes "github.com/osmai4603-collab/accounting_engine/internal/kernel/types"

type CompanyID = kernelTypes.CompanyID
type AccountID = kernelTypes.AccountID
type FiscalYearID = kernelTypes.FiscalYearID
type FiscalPeriodID = kernelTypes.FiscalPeriodID
type JournalID = kernelTypes.JournalID
type JournalEntryID = kernelTypes.JournalEntryID
type JournalEntryLineID = kernelTypes.JournalEntryLineID
type PartnerID = kernelTypes.PartnerID
type CostCenterID = kernelTypes.CostCenterID
type ProjectID = kernelTypes.ProjectID
type SegmentID = kernelTypes.SegmentID
type BusinessUnitID = kernelTypes.BusinessUnitID

var (
	NewCompanyID          = kernelTypes.NewCompanyID
	NewAccountID          = kernelTypes.NewAccountID
	NewFiscalYearID       = kernelTypes.NewFiscalYearID
	NewFiscalPeriodID     = kernelTypes.NewFiscalPeriodID
	NewJournalID          = kernelTypes.NewJournalID
	NewJournalEntryID     = kernelTypes.NewJournalEntryID
	NewJournalEntryLineID = kernelTypes.NewJournalEntryLineID
	NewPartnerID          = kernelTypes.NewPartnerID
	NewCostCenterID       = kernelTypes.NewCostCenterID
	NewProjectID          = kernelTypes.NewProjectID
	NewSegmentID          = kernelTypes.NewSegmentID
	NewBusinessUnitID     = kernelTypes.NewBusinessUnitID
)
