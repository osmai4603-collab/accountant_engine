package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/journal"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/masterdata"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/storage"
)

var _ storage.YearEndClosingStore = (*Driver)(nil)

var yearEndClosingsByDriver sync.Map // map[*Driver]map[types.CompanyID]map[types.FiscalYearID]storage.YearEndCloseRecord

func getClosingMap(d *Driver, create bool) map[types.CompanyID]map[types.FiscalYearID]storage.YearEndCloseRecord {
	if val, ok := yearEndClosingsByDriver.Load(d); ok {
		return val.(map[types.CompanyID]map[types.FiscalYearID]storage.YearEndCloseRecord)
	}
	if !create {
		return nil
	}
	m := make(map[types.CompanyID]map[types.FiscalYearID]storage.YearEndCloseRecord)
	actual, _ := yearEndClosingsByDriver.LoadOrStore(d, m)
	return actual.(map[types.CompanyID]map[types.FiscalYearID]storage.YearEndCloseRecord)
}

func cloneYearEndClosings(src, dst *Driver) {
	srcMap := getClosingMap(src, false)
	if len(srcMap) == 0 {
		return
	}
	dstMap := make(map[types.CompanyID]map[types.FiscalYearID]storage.YearEndCloseRecord, len(srcMap))
	for cid, cMap := range srcMap {
		dstMap[cid] = make(map[types.FiscalYearID]storage.YearEndCloseRecord, len(cMap))
		for yid, rec := range cMap {
			dstMap[cid][yid] = cloneYearEndCloseRecord(rec)
		}
	}
	yearEndClosingsByDriver.Store(dst, dstMap)
}

func copyYearEndClosings(src, dst *Driver) {
	srcMap := getClosingMap(src, false)
	if srcMap == nil {
		yearEndClosingsByDriver.Delete(dst)
		return
	}
	yearEndClosingsByDriver.Store(dst, srcMap)
	yearEndClosingsByDriver.Delete(src)
}

func cloneYearEndCloseRecord(rec storage.YearEndCloseRecord) storage.YearEndCloseRecord {
	cloned := rec
	cloned.ClosingEntryIDs = append([]types.JournalEntryID(nil), rec.ClosingEntryIDs...)
	if rec.ReopenedAt != nil {
		t := *rec.ReopenedAt
		cloned.ReopenedAt = &t
	}
	return cloned
}

func (d *Driver) ListAccounts(ctx context.Context, companyID types.CompanyID) ([]masterdata.Account, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	if err := d.assertReadScope(ctx, companyID, masterdata.ErrCompanyNotFound); err != nil {
		return nil, err
	}
	accMap := d.accounts[companyID]
	accounts := make([]masterdata.Account, 0, len(accMap))
	for _, acc := range accMap {
		accounts = append(accounts, cloneAccount(acc))
	}
	sort.Slice(accounts, func(i, j int) bool {
		return accounts[i].Code < accounts[j].Code
	})
	return accounts, nil
}

func (d *Driver) ListFiscalYears(ctx context.Context, companyID types.CompanyID) ([]masterdata.FiscalYear, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	if err := d.assertReadScope(ctx, companyID, masterdata.ErrFiscalYearNotFound); err != nil {
		return nil, err
	}
	yMap := d.years[companyID]
	years := make([]masterdata.FiscalYear, 0, len(yMap))
	for _, y := range yMap {
		years = append(years, y)
	}
	sort.Slice(years, func(i, j int) bool {
		return years[i].StartDate.Before(years[j].StartDate)
	})
	return years, nil
}

func (d *Driver) ListFiscalPeriods(ctx context.Context, companyID types.CompanyID, fiscalYearID types.FiscalYearID) ([]masterdata.FiscalPeriod, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	if err := d.assertReadScope(ctx, companyID, masterdata.ErrPeriodNotFound); err != nil {
		return nil, err
	}
	pMap := d.periods[companyID]
	periods := make([]masterdata.FiscalPeriod, 0)
	for _, p := range pMap {
		if p.FiscalYearID == fiscalYearID {
			periods = append(periods, p)
		}
	}
	sort.Slice(periods, func(i, j int) bool {
		return periods[i].Number < periods[j].Number
	})
	return periods, nil
}

func (d *Driver) ListEntriesByDateRange(ctx context.Context, companyID types.CompanyID, start, end time.Time) ([]journal.JournalEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	if err := d.assertReadScope(ctx, companyID, ErrEntryNotFound); err != nil {
		return nil, err
	}
	eMap := d.entries[companyID]
	entries := make([]journal.JournalEntry, 0)
	for _, e := range eMap {
		if !d.entryVisibleInLedger(ctx, e.ID) {
			continue
		}
		if e.Status != journal.EntryPosted && e.Status != journal.EntryReversed {
			continue
		}
		if !start.IsZero() && e.EffectiveDate.Before(start) {
			continue
		}
		if !end.IsZero() && e.EffectiveDate.After(end) {
			continue
		}
		entries = append(entries, cloneEntry(e))
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].EffectiveDate.Equal(entries[j].EffectiveDate) {
			return entries[i].ID.String() < entries[j].ID.String()
		}
		return entries[i].EffectiveDate.Before(entries[j].EffectiveDate)
	})
	return entries, nil
}

func (d *Driver) UpdateFiscalPeriodState(ctx context.Context, companyID types.CompanyID, periodID types.FiscalPeriodID, state masterdata.PeriodState, actorID, reason string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.readOnly {
		return ErrReadOnlyTransaction
	}
	pMap, ok := d.periods[companyID]
	if !ok {
		return masterdata.ErrPeriodNotFound
	}
	p, ok := pMap[periodID]
	if !ok {
		return masterdata.ErrPeriodNotFound
	}
	p.State = state
	pMap[periodID] = p
	return nil
}

func (d *Driver) UpdateFiscalYearState(ctx context.Context, companyID types.CompanyID, fiscalYearID types.FiscalYearID, state masterdata.FiscalYearState, actorID, reason string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.readOnly {
		return ErrReadOnlyTransaction
	}
	yMap, ok := d.years[companyID]
	if !ok {
		return masterdata.ErrFiscalYearNotFound
	}
	y, ok := yMap[fiscalYearID]
	if !ok {
		return masterdata.ErrFiscalYearNotFound
	}
	y.State = state
	yMap[fiscalYearID] = y
	return nil
}

func (d *Driver) SaveYearEndCloseRecord(ctx context.Context, record storage.YearEndCloseRecord) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.readOnly {
		return ErrReadOnlyTransaction
	}
	closings := getClosingMap(d, true)
	cMap, ok := closings[record.CompanyID]
	if !ok {
		cMap = make(map[types.FiscalYearID]storage.YearEndCloseRecord)
		closings[record.CompanyID] = cMap
	}
	cMap[record.FiscalYearID] = cloneYearEndCloseRecord(record)
	return nil
}

func (d *Driver) GetYearEndCloseRecord(ctx context.Context, companyID types.CompanyID, fiscalYearID types.FiscalYearID) (storage.YearEndCloseRecord, bool, error) {
	if err := ctx.Err(); err != nil {
		return storage.YearEndCloseRecord{}, false, err
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	if err := d.assertReadScope(ctx, companyID, masterdata.ErrFiscalYearNotFound); err != nil {
		return storage.YearEndCloseRecord{}, false, err
	}
	closings := getClosingMap(d, false)
	if closings == nil {
		return storage.YearEndCloseRecord{}, false, nil
	}
	cMap, ok := closings[companyID]
	if !ok {
		return storage.YearEndCloseRecord{}, false, nil
	}
	rec, ok := cMap[fiscalYearID]
	if !ok {
		return storage.YearEndCloseRecord{}, false, nil
	}
	return cloneYearEndCloseRecord(rec), true, nil
}
