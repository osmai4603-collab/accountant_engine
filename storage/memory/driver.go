package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	publicgovernance "github.com/osmai4603-collab/accounting_engine/pkg/api/v1/governance"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/journal"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	publicevents "github.com/osmai4603-collab/accounting_engine/pkg/spi/events"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/masterdata"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/storage"
	"github.com/shopspring/decimal"
)

type accountBalanceKey struct {
	companyID types.CompanyID
	accountID types.AccountID
	currency  string
}

type accountVersionKey struct {
	companyID types.CompanyID
	accountID types.AccountID
}

type dimensionKey struct {
	kind types.AnalyticalDimensionKind
	id   string
}

type phase11PolicyVersionKey struct {
	companyID types.CompanyID
	policyID  uuid.UUID
	kind      storage.PolicyKind
	version   int
}

type idempotencyKey struct {
	companyID types.CompanyID
	key       string
	// scope is the canonical company/operation/key triple. Including it makes
	// the same text key reusable for a posting and a reversal, matching the
	// kernel idempotency scope contract.
	scope string
}

var _ storage.PolicyStore = (*Driver)(nil)

// Driver is the thread-safe in-memory state engine.
type Driver struct {
	mu           sync.RWMutex
	readOnly     bool
	companies    map[types.CompanyID]masterdata.Company
	accounts     map[types.CompanyID]map[types.AccountID]masterdata.Account
	accountCodes map[types.CompanyID]map[string]types.AccountID
	journals     map[types.CompanyID]map[string]masterdata.Journal
	years        map[types.CompanyID]map[types.FiscalYearID]masterdata.FiscalYear
	periods      map[types.CompanyID]map[types.FiscalPeriodID]masterdata.FiscalPeriod

	// G-019/G-022 effective master data. primaryCompanyID binds reads without
	// an explicit storage company context to the first provisioned tenant.
	primaryCompanyID types.CompanyID
	accountVersions  map[accountVersionKey][]masterdata.Account
	costCenters      map[types.CompanyID]map[types.CostCenterID]masterdata.CostCenter
	dimensionValues  map[types.CompanyID]map[dimensionKey]masterdata.AnalyticalDimensionValue

	// G-020 append-only policy history.
	policyVersions map[phase11PolicyVersionKey]storage.PolicyVersion

	journalSequences      map[string]int64
	entries               map[types.CompanyID]map[types.JournalEntryID]journal.JournalEntry
	balances              map[accountBalanceKey]decimal.Decimal
	auditRecords          []storage.AuditRecord
	auditLastHash         string
	governanceAuditLogs   map[types.CompanyID][]publicgovernance.AuditLog
	governanceCheckpoints map[types.CompanyID]publicgovernance.AuditCheckpoint
	idempotency           map[idempotencyKey]storage.IdempotencyRecord

	// Public event ingestion
	eventIngestions map[string]publicevents.Ingestion
	eventMappings   map[string]publicevents.AccountMapping

	// Concurrency lock manager
	postingLocks sync.Map
}

func newDriver() *Driver {
	return &Driver{
		companies:             make(map[types.CompanyID]masterdata.Company),
		accounts:              make(map[types.CompanyID]map[types.AccountID]masterdata.Account),
		accountCodes:          make(map[types.CompanyID]map[string]types.AccountID),
		journals:              make(map[types.CompanyID]map[string]masterdata.Journal),
		years:                 make(map[types.CompanyID]map[types.FiscalYearID]masterdata.FiscalYear),
		periods:               make(map[types.CompanyID]map[types.FiscalPeriodID]masterdata.FiscalPeriod),
		accountVersions:       make(map[accountVersionKey][]masterdata.Account),
		costCenters:           make(map[types.CompanyID]map[types.CostCenterID]masterdata.CostCenter),
		dimensionValues:       make(map[types.CompanyID]map[dimensionKey]masterdata.AnalyticalDimensionValue),
		policyVersions:        make(map[phase11PolicyVersionKey]storage.PolicyVersion),
		journalSequences:      make(map[string]int64),
		entries:               make(map[types.CompanyID]map[types.JournalEntryID]journal.JournalEntry),
		balances:              make(map[accountBalanceKey]decimal.Decimal),
		auditRecords:          make([]storage.AuditRecord, 0),
		governanceAuditLogs:   make(map[types.CompanyID][]publicgovernance.AuditLog),
		governanceCheckpoints: make(map[types.CompanyID]publicgovernance.AuditCheckpoint),
		idempotency:           make(map[idempotencyKey]storage.IdempotencyRecord),
		eventIngestions:       make(map[string]publicevents.Ingestion),
		eventMappings:         make(map[string]publicevents.AccountMapping),
	}
}

// clone creates a deep snapshot of state for unit of work isolation and rollback.
func (d *Driver) clone() *Driver {
	cp := &Driver{
		readOnly:              d.readOnly,
		companies:             make(map[types.CompanyID]masterdata.Company, len(d.companies)),
		accounts:              make(map[types.CompanyID]map[types.AccountID]masterdata.Account, len(d.accounts)),
		accountCodes:          make(map[types.CompanyID]map[string]types.AccountID, len(d.accountCodes)),
		journals:              make(map[types.CompanyID]map[string]masterdata.Journal, len(d.journals)),
		years:                 make(map[types.CompanyID]map[types.FiscalYearID]masterdata.FiscalYear, len(d.years)),
		periods:               make(map[types.CompanyID]map[types.FiscalPeriodID]masterdata.FiscalPeriod, len(d.periods)),
		primaryCompanyID:      d.primaryCompanyID,
		accountVersions:       make(map[accountVersionKey][]masterdata.Account, len(d.accountVersions)),
		costCenters:           make(map[types.CompanyID]map[types.CostCenterID]masterdata.CostCenter, len(d.costCenters)),
		dimensionValues:       make(map[types.CompanyID]map[dimensionKey]masterdata.AnalyticalDimensionValue, len(d.dimensionValues)),
		policyVersions:        make(map[phase11PolicyVersionKey]storage.PolicyVersion, len(d.policyVersions)),
		journalSequences:      make(map[string]int64, len(d.journalSequences)),
		entries:               make(map[types.CompanyID]map[types.JournalEntryID]journal.JournalEntry, len(d.entries)),
		balances:              make(map[accountBalanceKey]decimal.Decimal, len(d.balances)),
		auditRecords:          make([]storage.AuditRecord, len(d.auditRecords)),
		auditLastHash:         d.auditLastHash,
		governanceAuditLogs:   make(map[types.CompanyID][]publicgovernance.AuditLog, len(d.governanceAuditLogs)),
		governanceCheckpoints: make(map[types.CompanyID]publicgovernance.AuditCheckpoint, len(d.governanceCheckpoints)),
		idempotency:           make(map[idempotencyKey]storage.IdempotencyRecord, len(d.idempotency)),
		eventIngestions:       make(map[string]publicevents.Ingestion, len(d.eventIngestions)),
		eventMappings:         make(map[string]publicevents.AccountMapping, len(d.eventMappings)),
	}

	for k, v := range d.companies {
		cp.companies[k] = v
	}
	for cid, accMap := range d.accounts {
		cp.accounts[cid] = make(map[types.AccountID]masterdata.Account, len(accMap))
		for aid, acc := range accMap {
			cp.accounts[cid][aid] = cloneAccount(acc)
		}
	}
	for cid, codeMap := range d.accountCodes {
		cp.accountCodes[cid] = make(map[string]types.AccountID, len(codeMap))
		for code, aid := range codeMap {
			cp.accountCodes[cid][code] = aid
		}
	}
	for cid, jMap := range d.journals {
		cp.journals[cid] = make(map[string]masterdata.Journal, len(jMap))
		for code, j := range jMap {
			cp.journals[cid][code] = j
		}
	}
	for cid, yMap := range d.years {
		cp.years[cid] = make(map[types.FiscalYearID]masterdata.FiscalYear, len(yMap))
		for yid, y := range yMap {
			cp.years[cid][yid] = y
		}
	}
	for cid, pMap := range d.periods {
		cp.periods[cid] = make(map[types.FiscalPeriodID]masterdata.FiscalPeriod, len(pMap))
		for pid, p := range pMap {
			cp.periods[cid][pid] = p
		}
	}
	for key, versions := range d.accountVersions {
		cp.accountVersions[key] = make([]masterdata.Account, len(versions))
		for i, version := range versions {
			cp.accountVersions[key][i] = cloneAccount(version)
		}
	}
	for companyID, centers := range d.costCenters {
		cp.costCenters[companyID] = make(map[types.CostCenterID]masterdata.CostCenter, len(centers))
		for id, center := range centers {
			cp.costCenters[companyID][id] = cloneCostCenter(center)
		}
	}
	for companyID, values := range d.dimensionValues {
		cp.dimensionValues[companyID] = make(map[dimensionKey]masterdata.AnalyticalDimensionValue, len(values))
		for key, value := range values {
			cp.dimensionValues[companyID][key] = cloneDimensionValue(value)
		}
	}
	for key, version := range d.policyVersions {
		cp.policyVersions[key] = clonePolicyVersion(version)
	}
	for k, v := range d.journalSequences {
		cp.journalSequences[k] = v
	}
	for cid, eMap := range d.entries {
		cp.entries[cid] = make(map[types.JournalEntryID]journal.JournalEntry, len(eMap))
		for eid, e := range eMap {
			cp.entries[cid][eid] = cloneEntry(e)
		}
	}
	for k, v := range d.balances {
		cp.balances[k] = v
	}
	copy(cp.auditRecords, d.auditRecords)
	for companyID, logs := range d.governanceAuditLogs {
		cp.governanceAuditLogs[companyID] = append([]publicgovernance.AuditLog(nil), logs...)
	}
	for companyID, checkpoint := range d.governanceCheckpoints {
		cp.governanceCheckpoints[companyID] = checkpoint
	}
	for k, v := range d.idempotency {
		cp.idempotency[k] = v
	}
	for k, v := range d.eventIngestions {
		cp.eventIngestions[k] = v
	}
	for k, v := range d.eventMappings {
		cp.eventMappings[k] = v
	}
	cloneMultiLedgerOwners(d, cp)
	cloneYearEndClosings(d, cp)
	return cp
}

// copyFrom overwrites state from a cloned snapshot (commit).
func (d *Driver) copyFrom(src *Driver) {
	d.companies = src.companies
	d.accounts = src.accounts
	d.accountCodes = src.accountCodes
	d.journals = src.journals
	d.years = src.years
	d.periods = src.periods
	d.primaryCompanyID = src.primaryCompanyID
	d.accountVersions = src.accountVersions
	d.costCenters = src.costCenters
	d.dimensionValues = src.dimensionValues
	d.policyVersions = src.policyVersions
	d.journalSequences = src.journalSequences
	d.entries = src.entries
	d.balances = src.balances
	d.auditRecords = src.auditRecords
	d.auditLastHash = src.auditLastHash
	d.governanceAuditLogs = src.governanceAuditLogs
	d.governanceCheckpoints = src.governanceCheckpoints
	d.idempotency = src.idempotency
	d.eventIngestions = src.eventIngestions
	d.eventMappings = src.eventMappings
	copyMultiLedgerOwners(src, d)
	copyYearEndClosings(src, d)
}

// Close releases in-memory state held by the driver, including global sync.Map
// entries keyed by this driver pointer.
func (d *Driver) Close() error {
	if d == nil {
		return nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.companies = nil
	d.accounts = nil
	d.accountCodes = nil
	d.journals = nil
	d.years = nil
	d.periods = nil
	d.accountVersions = nil
	d.costCenters = nil
	d.dimensionValues = nil
	d.policyVersions = nil
	d.journalSequences = nil
	d.entries = nil
	d.balances = nil
	d.auditRecords = nil
	d.governanceAuditLogs = nil
	d.governanceCheckpoints = nil
	d.idempotency = nil
	d.eventIngestions = nil
	d.eventMappings = nil
	multiLedgerEntryOwners.Delete(d)
	yearEndClosingsByDriver.Delete(d)
	d.postingLocks.Range(func(key, _ any) bool {
		d.postingLocks.Delete(key)
		return true
	})
	return nil
}

func cloneEntry(entry journal.JournalEntry) journal.JournalEntry {
	return entry.Clone()
}

func cloneAccount(account masterdata.Account) masterdata.Account {
	cloned := account
	if account.EffectiveTo != nil {
		to := *account.EffectiveTo
		cloned.EffectiveTo = &to
	}
	if account.ParentID != nil {
		parent := *account.ParentID
		cloned.ParentID = &parent
	}
	if account.IsMonetary != nil {
		monetary := *account.IsMonetary
		cloned.IsMonetary = &monetary
	}
	return cloned
}

func cloneCostCenter(center masterdata.CostCenter) masterdata.CostCenter {
	cloned := center
	if center.ParentID != nil {
		parent := *center.ParentID
		cloned.ParentID = &parent
	}
	return cloned
}

func cloneDimensionValue(value masterdata.AnalyticalDimensionValue) masterdata.AnalyticalDimensionValue {
	return value
}

func clonePolicyVersion(version storage.PolicyVersion) storage.PolicyVersion {
	cloned := version
	if version.EffectiveTo != nil {
		to := *version.EffectiveTo
		cloned.EffectiveTo = &to
	}
	if version.Payload != nil {
		cloned.Payload = append([]byte(nil), version.Payload...)
	}
	return cloned
}

// AppendVersion implements storage.PolicyStore for the in-memory driver. The
// rules mirror the PostgreSQL implementation: company-scoped, append-only,
// non-overlapping windows ordered by effective_from.
func (d *Driver) AppendVersion(ctx context.Context, version storage.PolicyVersion) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := version.Validate(); err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.readOnly {
		return ErrReadOnlyTransaction
	}
	key := phase11PolicyVersionKey{
		companyID: version.CompanyID,
		policyID:  version.PolicyID,
		kind:      version.Kind,
		version:   version.Version,
	}
	if _, exists := d.policyVersions[key]; exists {
		return storage.ErrPolicyVersionConflict
	}
	end := time.Date(9999, time.December, 31, 0, 0, 0, 0, time.UTC)
	if version.EffectiveTo != nil {
		end = version.EffectiveTo.UTC()
	}
	for _, existing := range d.policyVersions {
		if existing.CompanyID != version.CompanyID || existing.Kind != version.Kind {
			continue
		}
		if existing.EffectiveFrom.UTC().Before(end) &&
			(existing.EffectiveTo == nil || existing.EffectiveTo.UTC().After(version.EffectiveFrom.UTC())) {
			return storage.ErrPolicyVersionConflict
		}
	}
	d.policyVersions[key] = clonePolicyVersion(version)
	return nil
}

func (d *Driver) ListVersions(ctx context.Context, companyID types.CompanyID, kind storage.PolicyKind) ([]storage.PolicyVersion, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if companyID == (types.CompanyID{}) {
		return nil, storage.ErrPolicyVersionNotFound
	}
	if kind != storage.PolicyKindPosting && kind != storage.PolicyKindCurrency {
		return nil, storage.ErrPolicyVersionNotFound
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	versions := make([]storage.PolicyVersion, 0)
	for _, version := range d.policyVersions {
		if version.CompanyID == companyID && version.Kind == kind {
			versions = append(versions, clonePolicyVersion(version))
		}
	}
	sort.Slice(versions, func(i, j int) bool {
		if versions[i].EffectiveFrom.Equal(versions[j].EffectiveFrom) {
			return versions[i].Version < versions[j].Version
		}
		return versions[i].EffectiveFrom.Before(versions[j].EffectiveFrom)
	})
	return versions, nil
}
