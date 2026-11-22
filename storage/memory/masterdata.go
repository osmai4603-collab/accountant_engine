package memory

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/masterdata"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/storage"
)

// Ensure Driver implements the baseline master-data port and the optional
// effective-account / analytical-dimension capabilities introduced in Phase 11.
var _ masterdata.Store = (*Driver)(nil)
var _ masterdata.EffectiveAccountReader = (*Driver)(nil)
var _ masterdata.AccountVersionWriter = (*Driver)(nil)
var _ masterdata.CostCenterReader = (*Driver)(nil)
var _ masterdata.CostCenterWriter = (*Driver)(nil)
var _ masterdata.AnalyticalDimensionReader = (*Driver)(nil)
var _ masterdata.AnalyticalDimensionWriter = (*Driver)(nil)

func (d *Driver) CreateCompany(ctx context.Context, company masterdata.Company) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if company.ID == (types.CompanyID{}) {
		return masterdata.ErrInvalidCompany
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.readOnly {
		return ErrReadOnlyTransaction
	}
	if _, exists := d.companies[company.ID]; exists {
		return masterdata.ErrDuplicateCompanyCode
	}
	for _, c := range d.companies {
		if strings.EqualFold(c.Code, company.Code) {
			return masterdata.ErrDuplicateCompanyCode
		}
	}
	d.companies[company.ID] = company
	// The first provisioned company binds context-less administrative reads.
	// Callers operating on another tenant must supply storage.WithCompanyContext.
	if d.primaryCompanyID == (types.CompanyID{}) {
		d.primaryCompanyID = company.ID
	}
	return nil
}

// assertReadScope rejects a read that names a company other than the active
// tenant. The active tenant is the explicit storage company context when
// present, otherwise the adapter's first provisioned company. This closes the
// cross-company lookup gap without changing the explicit companyID argument on
// any public port.
func (d *Driver) assertReadScope(ctx context.Context, companyID types.CompanyID, notFound error) error {
	active := d.primaryCompanyID
	if fromContext, ok := storage.CompanyFromContext(ctx); ok && fromContext != (types.CompanyID{}) {
		active = fromContext
	}
	if active != (types.CompanyID{}) && companyID != active {
		return notFound
	}
	return nil
}

func (d *Driver) GetCompany(ctx context.Context, companyID types.CompanyID) (masterdata.Company, error) {
	if err := ctx.Err(); err != nil {
		return masterdata.Company{}, err
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	if err := d.assertReadScope(ctx, companyID, masterdata.ErrCompanyNotFound); err != nil {
		return masterdata.Company{}, err
	}
	c, ok := d.companies[companyID]
	if !ok {
		return masterdata.Company{}, masterdata.ErrCompanyNotFound
	}
	return c, nil
}

func (d *Driver) CreateJournal(ctx context.Context, j masterdata.Journal) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if j.ID == (masterdata.JournalID{}) || strings.TrimSpace(j.Code) == "" || strings.TrimSpace(j.Name) == "" || len(j.Code) > 32 || len(j.Name) > 100 {
		return masterdata.ErrInvalidJournal
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.readOnly {
		return ErrReadOnlyTransaction
	}
	if _, ok := d.companies[j.CompanyID]; !ok {
		return masterdata.ErrCompanyNotFound
	}
	jMap, ok := d.journals[j.CompanyID]
	if !ok {
		jMap = make(map[string]masterdata.Journal)
		d.journals[j.CompanyID] = jMap
	}
	if _, exists := jMap[j.Code]; exists {
		return masterdata.ErrDuplicateJournalCode
	}
	jMap[j.Code] = j
	return nil
}

func (d *Driver) GetJournalByCode(ctx context.Context, companyID types.CompanyID, code string) (masterdata.Journal, error) {
	if err := ctx.Err(); err != nil {
		return masterdata.Journal{}, err
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	if err := d.assertReadScope(ctx, companyID, masterdata.ErrJournalNotFound); err != nil {
		return masterdata.Journal{}, err
	}
	jMap, ok := d.journals[companyID]
	if !ok {
		return masterdata.Journal{}, masterdata.ErrJournalNotFound
	}
	j, ok := jMap[code]
	if !ok {
		return masterdata.Journal{}, masterdata.ErrJournalNotFound
	}
	return j, nil
}

func (d *Driver) CreateAccount(ctx context.Context, acc masterdata.Account) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if acc.ID == (types.AccountID{}) || strings.TrimSpace(acc.Code) == "" || strings.TrimSpace(acc.Name) == "" {
		return masterdata.ErrInvalidAccount
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.readOnly {
		return ErrReadOnlyTransaction
	}
	if _, ok := d.companies[acc.CompanyID]; !ok {
		return masterdata.ErrCompanyNotFound
	}
	accMap, ok := d.accounts[acc.CompanyID]
	if !ok {
		accMap = make(map[types.AccountID]masterdata.Account)
		d.accounts[acc.CompanyID] = accMap
	}
	if _, exists := accMap[acc.ID]; exists {
		return masterdata.ErrDuplicateAccountCode
	}
	codeMap, ok := d.accountCodes[acc.CompanyID]
	if !ok {
		codeMap = make(map[string]types.AccountID)
		d.accountCodes[acc.CompanyID] = codeMap
	}
	if _, exists := codeMap[acc.Code]; exists {
		return masterdata.ErrDuplicateAccountCode
	}
	accMap[acc.ID] = acc
	codeMap[acc.Code] = acc.ID
	return nil
}

func (d *Driver) GetAccount(ctx context.Context, companyID types.CompanyID, accountID types.AccountID) (masterdata.Account, error) {
	if err := ctx.Err(); err != nil {
		return masterdata.Account{}, err
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	accMap, ok := d.accounts[companyID]
	if !ok {
		return masterdata.Account{}, masterdata.ErrAccountNotFound
	}
	acc, ok := accMap[accountID]
	if !ok {
		return masterdata.Account{}, masterdata.ErrAccountNotFound
	}
	return acc, nil
}

func (d *Driver) GetAccountByCode(ctx context.Context, companyID types.CompanyID, code string) (masterdata.Account, error) {
	if err := ctx.Err(); err != nil {
		return masterdata.Account{}, err
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	if err := d.assertReadScope(ctx, companyID, masterdata.ErrAccountNotFound); err != nil {
		return masterdata.Account{}, err
	}
	codeMap, ok := d.accountCodes[companyID]
	if !ok {
		return masterdata.Account{}, masterdata.ErrAccountNotFound
	}
	aid, ok := codeMap[code]
	if !ok {
		return masterdata.Account{}, masterdata.ErrAccountNotFound
	}
	return d.accounts[companyID][aid], nil
}

func (d *Driver) HasAccountChildren(ctx context.Context, companyID types.CompanyID, accountID types.AccountID) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	accMap, ok := d.accounts[companyID]
	if !ok {
		return false, nil
	}
	for _, acc := range accMap {
		if acc.ParentID != nil && *acc.ParentID == accountID {
			return true, nil
		}
	}
	return false, nil
}

func (d *Driver) CreateFiscalYear(ctx context.Context, year masterdata.FiscalYear) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if year.ID == (types.FiscalYearID{}) {
		return masterdata.ErrInvalidFiscalYear
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.readOnly {
		return ErrReadOnlyTransaction
	}
	if _, ok := d.companies[year.CompanyID]; !ok {
		return masterdata.ErrCompanyNotFound
	}
	yMap, ok := d.years[year.CompanyID]
	if !ok {
		yMap = make(map[types.FiscalYearID]masterdata.FiscalYear)
		d.years[year.CompanyID] = yMap
	}
	if _, exists := yMap[year.ID]; exists {
		return fmt.Errorf("duplicate fiscal year id")
	}
	yMap[year.ID] = year
	return nil
}

func (d *Driver) GetFiscalYear(ctx context.Context, companyID types.CompanyID, fiscalYearID types.FiscalYearID) (masterdata.FiscalYear, error) {
	if err := ctx.Err(); err != nil {
		return masterdata.FiscalYear{}, err
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	yMap, ok := d.years[companyID]
	if !ok {
		return masterdata.FiscalYear{}, masterdata.ErrFiscalYearNotFound
	}
	y, ok := yMap[fiscalYearID]
	if !ok {
		return masterdata.FiscalYear{}, masterdata.ErrFiscalYearNotFound
	}
	return y, nil
}

func (d *Driver) CreateFiscalPeriod(ctx context.Context, period masterdata.FiscalPeriod) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if period.ID == (types.FiscalPeriodID{}) {
		return masterdata.ErrInvalidFiscalPeriod
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.readOnly {
		return ErrReadOnlyTransaction
	}
	if _, ok := d.companies[period.CompanyID]; !ok {
		return masterdata.ErrCompanyNotFound
	}
	pMap, ok := d.periods[period.CompanyID]
	if !ok {
		pMap = make(map[types.FiscalPeriodID]masterdata.FiscalPeriod)
		d.periods[period.CompanyID] = pMap
	}
	if _, exists := pMap[period.ID]; exists {
		return fmt.Errorf("duplicate fiscal period id")
	}
	pMap[period.ID] = period
	return nil
}

func (d *Driver) GetFiscalPeriod(ctx context.Context, companyID types.CompanyID, periodID types.FiscalPeriodID) (masterdata.FiscalPeriod, error) {
	if err := ctx.Err(); err != nil {
		return masterdata.FiscalPeriod{}, err
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	pMap, ok := d.periods[companyID]
	if !ok {
		return masterdata.FiscalPeriod{}, masterdata.ErrPeriodNotFound
	}
	p, ok := pMap[periodID]
	if !ok {
		return masterdata.FiscalPeriod{}, masterdata.ErrPeriodNotFound
	}
	return p, nil
}

func (d *Driver) GetPeriodForDate(ctx context.Context, companyID types.CompanyID, date time.Time) (masterdata.FiscalPeriod, error) {
	if err := ctx.Err(); err != nil {
		return masterdata.FiscalPeriod{}, err
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	if err := d.assertReadScope(ctx, companyID, masterdata.ErrPeriodNotFound); err != nil {
		return masterdata.FiscalPeriod{}, err
	}
	pMap, ok := d.periods[companyID]
	if !ok {
		return masterdata.FiscalPeriod{}, masterdata.ErrPeriodNotFound
	}
	for _, p := range pMap {
		if p.Type == masterdata.FiscalPeriodTypeNormal && !date.Before(p.StartDate) && !date.After(p.EndDate) {
			return p, nil
		}
	}
	return masterdata.FiscalPeriod{}, masterdata.ErrPeriodNotFound
}

// Effective-dated account versions (G-019). Bounds are stored at UTC
// calendar-day granularity: EffectiveFrom is midnight UTC and EffectiveTo is
// the final nanosecond of its UTC day. Both bounds are inclusive, which keeps
// the snapshots directly usable by Account.IsCurrent.
const phase11Day = 24 * time.Hour

func phase11UTCDay(t time.Time) time.Time {
	return t.UTC().Truncate(phase11Day)
}

func phase11EndOfUTCDay(t time.Time) time.Time {
	return phase11UTCDay(t).Add(phase11Day).Add(-time.Nanosecond)
}

func normalizeAccountVersion(account masterdata.Account) (masterdata.Account, error) {
	if account.EffectiveFrom.IsZero() {
		return masterdata.Account{}, masterdata.ErrInvalidEffectiveRange
	}
	from := phase11UTCDay(account.EffectiveFrom)
	var to *time.Time
	if account.EffectiveTo != nil {
		toDay := phase11UTCDay(*account.EffectiveTo)
		if toDay.Before(from) {
			return masterdata.Account{}, masterdata.ErrInvalidEffectiveRange
		}
		end := phase11EndOfUTCDay(toDay)
		to = &end
	}
	account.EffectiveFrom = from
	account.EffectiveTo = to
	return cloneAccount(account), nil
}

func accountWindowsOverlap(a, b masterdata.Account) bool {
	if a.EffectiveTo != nil && b.EffectiveFrom.After(*a.EffectiveTo) {
		return false
	}
	if b.EffectiveTo != nil && a.EffectiveFrom.After(*b.EffectiveTo) {
		return false
	}
	return true
}

func (d *Driver) SaveAccountVersion(ctx context.Context, account masterdata.Account) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if account.ID == (types.AccountID{}) || account.CompanyID == (types.CompanyID{}) {
		return masterdata.ErrInvalidAccount
	}
	version, err := normalizeAccountVersion(account)
	if err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.readOnly {
		return ErrReadOnlyTransaction
	}
	if _, ok := d.companies[account.CompanyID]; !ok {
		return masterdata.ErrCompanyNotFound
	}
	base, ok := d.accounts[account.CompanyID][account.ID]
	if !ok {
		return masterdata.ErrAccountNotFound
	}
	key := accountVersionKey{companyID: account.CompanyID, accountID: account.ID}
	for _, existing := range d.accountVersions[key] {
		if accountWindowsOverlap(existing, version) {
			return masterdata.ErrAccountVersionOverlap
		}
	}
	d.accountVersions[key] = append(d.accountVersions[key], version)
	sort.Slice(d.accountVersions[key], func(i, j int) bool {
		return d.accountVersions[key][i].EffectiveFrom.Before(d.accountVersions[key][j].EffectiveFrom)
	})
	// The newest full snapshot also refreshes the base row so ordinary reads
	// observe the latest master data while historical versions remain frozen.
	newest := d.accountVersions[key][len(d.accountVersions[key])-1]
	if newest.EffectiveFrom.Equal(version.EffectiveFrom) {
		if base.Code != version.Code {
			if codeMap, exists := d.accountCodes[account.CompanyID]; exists {
				delete(codeMap, base.Code)
				codeMap[version.Code] = version.ID
			}
		}
		d.accounts[account.CompanyID][account.ID] = cloneAccount(version)
	}
	return nil
}

func (d *Driver) GetAccountAsOf(ctx context.Context, companyID types.CompanyID, accountID types.AccountID, asOf time.Time) (masterdata.Account, error) {
	if err := ctx.Err(); err != nil {
		return masterdata.Account{}, err
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	if err := d.assertReadScope(ctx, companyID, masterdata.ErrAccountNotFound); err != nil {
		return masterdata.Account{}, err
	}
	asOfDay := phase11UTCDay(asOf)
	key := accountVersionKey{companyID: companyID, accountID: accountID}
	var selected *masterdata.Account
	for i := range d.accountVersions[key] {
		candidate := d.accountVersions[key][i]
		if candidate.EffectiveFrom.After(asOfDay) {
			continue
		}
		if candidate.EffectiveTo != nil && asOfDay.After(phase11UTCDay(*candidate.EffectiveTo)) {
			continue
		}
		copy := cloneAccount(candidate)
		if selected == nil || copy.EffectiveFrom.After(selected.EffectiveFrom) {
			selected = &copy
		}
	}
	if selected != nil {
		return *selected, nil
	}
	// Adapters that have not been seeded with versions keep the legacy
	// GetAccount behaviour; the kernel still validates IsCurrent on the date.
	account, ok := d.accounts[companyID][accountID]
	if !ok {
		return masterdata.Account{}, masterdata.ErrAccountNotFound
	}
	return cloneAccount(account), nil
}

// Company-scoped analytical master data (G-022).
func (d *Driver) CreateCostCenter(ctx context.Context, center masterdata.CostCenter) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := center.Validate(); err != nil {
		return masterdata.ErrInvalidCostCenter
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.readOnly {
		return ErrReadOnlyTransaction
	}
	if _, ok := d.companies[center.CompanyID]; !ok {
		return masterdata.ErrCompanyNotFound
	}
	if center.ParentID != nil {
		centers := d.costCenters[center.CompanyID]
		if _, ok := centers[*center.ParentID]; !ok {
			return masterdata.ErrInvalidCostCenter
		}
	}
	centers, ok := d.costCenters[center.CompanyID]
	if !ok {
		centers = make(map[types.CostCenterID]masterdata.CostCenter)
		d.costCenters[center.CompanyID] = centers
	}
	if _, exists := centers[center.ID]; exists {
		return masterdata.ErrDuplicateCostCenter
	}
	for _, existing := range centers {
		if strings.EqualFold(existing.Code, center.Code) {
			return masterdata.ErrDuplicateCostCenter
		}
	}
	centers[center.ID] = cloneCostCenter(center)
	return nil
}

func (d *Driver) GetCostCenterForCompany(ctx context.Context, companyID types.CompanyID, costCenterID types.CostCenterID) (masterdata.CostCenter, error) {
	if err := ctx.Err(); err != nil {
		return masterdata.CostCenter{}, err
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	if err := d.assertReadScope(ctx, companyID, masterdata.ErrInvalidCostCenter); err != nil {
		return masterdata.CostCenter{}, err
	}
	center, ok := d.costCenters[companyID][costCenterID]
	if !ok || center.ID != costCenterID || center.CompanyID != companyID {
		return masterdata.CostCenter{}, masterdata.ErrInvalidCostCenter
	}
	return cloneCostCenter(center), nil
}

func (d *Driver) CreateAnalyticalDimensionValue(ctx context.Context, value masterdata.AnalyticalDimensionValue) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := value.Validate(); err != nil {
		return masterdata.ErrInvalidAnalyticalDimension
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.readOnly {
		return ErrReadOnlyTransaction
	}
	if _, ok := d.companies[value.CompanyID]; !ok {
		return masterdata.ErrCompanyNotFound
	}
	values, ok := d.dimensionValues[value.CompanyID]
	if !ok {
		values = make(map[dimensionKey]masterdata.AnalyticalDimensionValue)
		d.dimensionValues[value.CompanyID] = values
	}
	key := dimensionKey{kind: value.Kind, id: value.ID.String()}
	if _, exists := values[key]; exists {
		return masterdata.ErrDuplicateAnalyticalDimension
	}
	for existingKey, existing := range values {
		if existingKey.kind == value.Kind && strings.EqualFold(existing.Code, value.Code) {
			return masterdata.ErrDuplicateAnalyticalDimension
		}
	}
	values[key] = cloneDimensionValue(value)
	return nil
}

func (d *Driver) GetAnalyticalDimensionForCompany(
	ctx context.Context,
	companyID types.CompanyID,
	kind masterdata.AnalyticalDimensionKind,
	id string,
) (masterdata.AnalyticalDimensionValue, error) {
	if err := ctx.Err(); err != nil {
		return masterdata.AnalyticalDimensionValue{}, err
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	if err := d.assertReadScope(ctx, companyID, masterdata.ErrInvalidAnalyticalDimension); err != nil {
		return masterdata.AnalyticalDimensionValue{}, err
	}
	value, ok := d.dimensionValues[companyID][dimensionKey{kind: kind, id: id}]
	if !ok || value.ID.String() != id || value.CompanyID != companyID || value.Kind != kind {
		return masterdata.AnalyticalDimensionValue{}, masterdata.ErrInvalidAnalyticalDimension
	}
	return cloneDimensionValue(value), nil
}
