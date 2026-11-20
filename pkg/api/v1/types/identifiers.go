package types

import kernalTypes "github.com/osmai4603-collab/accounting_engine/internal/kernal/types"

type CompanyID = kernalTypes.CompanyID
type AccountID = kernalTypes.AccountID
type FiscalYearID = kernalTypes.FiscalYearID
type FiscalPeriodID = kernalTypes.FiscalPeriodID
type JournalID = kernalTypes.JournalID
type JournalEntryID = kernalTypes.JournalEntryID
type JournalEntryLineID = kernalTypes.JournalEntryLineID
type PartnerID = kernalTypes.PartnerID
type CostCenterID = kernalTypes.CostCenterID
type ProjectID = kernalTypes.ProjectID
type SegmentID = kernalTypes.SegmentID
type BusinessUnitID = kernalTypes.BusinessUnitID

var (
	NewCompanyID          = kernalTypes.NewCompanyID
	NewAccountID          = kernalTypes.NewAccountID
	NewFiscalYearID       = kernalTypes.NewFiscalYearID
	NewFiscalPeriodID     = kernalTypes.NewFiscalPeriodID
	NewJournalID          = kernalTypes.NewJournalID
	NewJournalEntryID     = kernalTypes.NewJournalEntryID
	NewJournalEntryLineID = kernalTypes.NewJournalEntryLineID
	NewPartnerID          = kernalTypes.NewPartnerID
	NewCostCenterID       = kernalTypes.NewCostCenterID
	NewProjectID          = kernalTypes.NewProjectID
	NewSegmentID          = kernalTypes.NewSegmentID
	NewBusinessUnitID     = kernalTypes.NewBusinessUnitID
)
