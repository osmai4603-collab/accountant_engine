package memory

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/masterdata"
)

// Ensure Driver implements masterdata.Store and storage.MasterDataReader
var _ masterdata.Store = (*Driver)(nil)

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
	return nil
}

func (d *Driver) GetCompany(ctx context.Context, companyID types.CompanyID) (masterdata.Company, error) {
	if err := ctx.Err(); err != nil {
		return masterdata.Company{}, err
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
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
