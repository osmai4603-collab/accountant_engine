package memory

import (
	"sync"

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

type idempotencyKey struct {
	companyID types.CompanyID
	key       string
}

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

	journalSequences map[string]int64
	entries          map[types.CompanyID]map[types.JournalEntryID]journal.JournalEntry
	balances         map[accountBalanceKey]decimal.Decimal
	auditRecords     []storage.AuditRecord
	auditLastHash    string
	idempotency      map[idempotencyKey]storage.IdempotencyRecord

	// Public event ingestion
	eventIngestions map[string]publicevents.Ingestion
	eventMappings   map[string]types.AccountID

	// Concurrency lock manager
	postingLocks sync.Map
}

func newDriver() *Driver {
	return &Driver{
		companies:        make(map[types.CompanyID]masterdata.Company),
		accounts:         make(map[types.CompanyID]map[types.AccountID]masterdata.Account),
		accountCodes:     make(map[types.CompanyID]map[string]types.AccountID),
		journals:         make(map[types.CompanyID]map[string]masterdata.Journal),
		years:            make(map[types.CompanyID]map[types.FiscalYearID]masterdata.FiscalYear),
		periods:          make(map[types.CompanyID]map[types.FiscalPeriodID]masterdata.FiscalPeriod),
		journalSequences: make(map[string]int64),
		entries:          make(map[types.CompanyID]map[types.JournalEntryID]journal.JournalEntry),
		balances:         make(map[accountBalanceKey]decimal.Decimal),
		auditRecords:     make([]storage.AuditRecord, 0),
		idempotency:      make(map[idempotencyKey]storage.IdempotencyRecord),
		eventIngestions:  make(map[string]publicevents.Ingestion),
		eventMappings:    make(map[string]types.AccountID),
	}
}

// clone creates a deep snapshot of state for unit of work isolation and rollback.
func (d *Driver) clone() *Driver {
	cp := &Driver{
		readOnly:         d.readOnly,
		companies:        make(map[types.CompanyID]masterdata.Company, len(d.companies)),
		accounts:         make(map[types.CompanyID]map[types.AccountID]masterdata.Account, len(d.accounts)),
		accountCodes:     make(map[types.CompanyID]map[string]types.AccountID, len(d.accountCodes)),
		journals:         make(map[types.CompanyID]map[string]masterdata.Journal, len(d.journals)),
		years:            make(map[types.CompanyID]map[types.FiscalYearID]masterdata.FiscalYear, len(d.years)),
		periods:          make(map[types.CompanyID]map[types.FiscalPeriodID]masterdata.FiscalPeriod, len(d.periods)),
		journalSequences: make(map[string]int64, len(d.journalSequences)),
		entries:          make(map[types.CompanyID]map[types.JournalEntryID]journal.JournalEntry, len(d.entries)),
		balances:         make(map[accountBalanceKey]decimal.Decimal, len(d.balances)),
		auditRecords:     make([]storage.AuditRecord, len(d.auditRecords)),
		auditLastHash:    d.auditLastHash,
		idempotency:      make(map[idempotencyKey]storage.IdempotencyRecord, len(d.idempotency)),
		eventIngestions:  make(map[string]publicevents.Ingestion, len(d.eventIngestions)),
		eventMappings:    make(map[string]types.AccountID, len(d.eventMappings)),
	}

	for k, v := range d.companies {
		cp.companies[k] = v
	}
	for cid, accMap := range d.accounts {
		cp.accounts[cid] = make(map[types.AccountID]masterdata.Account, len(accMap))
		for aid, acc := range accMap {
			cp.accounts[cid][aid] = acc
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
	for k, v := range d.idempotency {
		cp.idempotency[k] = v
	}
	for k, v := range d.eventIngestions {
		cp.eventIngestions[k] = v
	}
	for k, v := range d.eventMappings {
		cp.eventMappings[k] = v
	}
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
	d.journalSequences = src.journalSequences
	d.entries = src.entries
	d.balances = src.balances
	d.auditRecords = src.auditRecords
	d.auditLastHash = src.auditLastHash
	d.idempotency = src.idempotency
	d.eventIngestions = src.eventIngestions
	d.eventMappings = src.eventMappings
}

func cloneEntry(entry journal.JournalEntry) journal.JournalEntry {
	cloned := entry
	if entry.Lines != nil {
		cloned.Lines = make([]journal.EntryLine, len(entry.Lines))
		copy(cloned.Lines, entry.Lines)
	}
	return cloned
}
