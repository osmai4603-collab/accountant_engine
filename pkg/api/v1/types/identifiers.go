package types

import "github.com/google/uuid"

// CompanyID is an immutable public identifier for a tenant company.
type CompanyID uuid.UUID

func (id CompanyID) String() string    { return uuid.UUID(id).String() }
func (id CompanyID) UUID() uuid.UUID   { return uuid.UUID(id) }

// AccountID is an immutable public identifier for a chart-of-accounts node.
type AccountID uuid.UUID

func (id AccountID) String() string    { return uuid.UUID(id).String() }
func (id AccountID) UUID() uuid.UUID   { return uuid.UUID(id) }

type FiscalYearID uuid.UUID

func (id FiscalYearID) String() string  { return uuid.UUID(id).String() }
func (id FiscalYearID) UUID() uuid.UUID { return uuid.UUID(id) }

type FiscalPeriodID uuid.UUID

func (id FiscalPeriodID) String() string  { return uuid.UUID(id).String() }
func (id FiscalPeriodID) UUID() uuid.UUID { return uuid.UUID(id) }

type JournalID uuid.UUID

func (id JournalID) String() string    { return uuid.UUID(id).String() }
func (id JournalID) UUID() uuid.UUID   { return uuid.UUID(id) }

type JournalEntryID uuid.UUID

func (id JournalEntryID) String() string  { return uuid.UUID(id).String() }
func (id JournalEntryID) UUID() uuid.UUID { return uuid.UUID(id) }

type JournalEntryLineID uuid.UUID

func (id JournalEntryLineID) String() string  { return uuid.UUID(id).String() }
func (id JournalEntryLineID) UUID() uuid.UUID { return uuid.UUID(id) }

type PartnerID uuid.UUID

func (id PartnerID) String() string    { return uuid.UUID(id).String() }
func (id PartnerID) UUID() uuid.UUID   { return uuid.UUID(id) }

type CostCenterID uuid.UUID

func (id CostCenterID) String() string  { return uuid.UUID(id).String() }
func (id CostCenterID) UUID() uuid.UUID { return uuid.UUID(id) }

type ProjectID uuid.UUID

func (id ProjectID) String() string    { return uuid.UUID(id).String() }
func (id ProjectID) UUID() uuid.UUID   { return uuid.UUID(id) }

type SegmentID uuid.UUID

func (id SegmentID) String() string    { return uuid.UUID(id).String() }
func (id SegmentID) UUID() uuid.UUID   { return uuid.UUID(id) }

type BusinessUnitID uuid.UUID

func (id BusinessUnitID) String() string  { return uuid.UUID(id).String() }
func (id BusinessUnitID) UUID() uuid.UUID { return uuid.UUID(id) }

func NewCompanyID() CompanyID                   { return CompanyID(uuid.New()) }
func NewAccountID() AccountID                   { return AccountID(uuid.New()) }
func NewFiscalYearID() FiscalYearID             { return FiscalYearID(uuid.New()) }
func NewFiscalPeriodID() FiscalPeriodID         { return FiscalPeriodID(uuid.New()) }
func NewJournalID() JournalID                   { return JournalID(uuid.New()) }
func NewJournalEntryID() JournalEntryID         { return JournalEntryID(uuid.New()) }
func NewJournalEntryLineID() JournalEntryLineID { return JournalEntryLineID(uuid.New()) }
func NewPartnerID() PartnerID                   { return PartnerID(uuid.New()) }
func NewCostCenterID() CostCenterID             { return CostCenterID(uuid.New()) }
func NewProjectID() ProjectID                   { return ProjectID(uuid.New()) }
func NewSegmentID() SegmentID                   { return SegmentID(uuid.New()) }
func NewBusinessUnitID() BusinessUnitID         { return BusinessUnitID(uuid.New()) }
