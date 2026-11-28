package masterdatacontract

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/masterdata"
)

type coverageStore struct {
	companies      map[types.CompanyID]masterdata.Company
	journals       map[types.CompanyID]map[string]masterdata.Journal
	journalsByID   map[types.CompanyID]map[types.JournalID]masterdata.Journal
	accounts       map[types.CompanyID]map[types.AccountID]masterdata.Account
	accountsByCode map[types.CompanyID]map[string]masterdata.Account
	fiscalYears    map[types.CompanyID]map[types.FiscalYearID]masterdata.FiscalYear
	fiscalPeriods  map[types.CompanyID]map[types.FiscalPeriodID]masterdata.FiscalPeriod
	periodByNumber map[types.CompanyID]map[types.FiscalYearID]map[int]masterdata.FiscalPeriod
}

func newCoverageStore() *coverageStore {
	return &coverageStore{
		companies:      make(map[types.CompanyID]masterdata.Company),
		journals:       make(map[types.CompanyID]map[string]masterdata.Journal),
		journalsByID:   make(map[types.CompanyID]map[types.JournalID]masterdata.Journal),
		accounts:       make(map[types.CompanyID]map[types.AccountID]masterdata.Account),
		accountsByCode: make(map[types.CompanyID]map[string]masterdata.Account),
		fiscalYears:    make(map[types.CompanyID]map[types.FiscalYearID]masterdata.FiscalYear),
		fiscalPeriods:  make(map[types.CompanyID]map[types.FiscalPeriodID]masterdata.FiscalPeriod),
		periodByNumber: make(map[types.CompanyID]map[types.FiscalYearID]map[int]masterdata.FiscalPeriod),
	}
}

func (s *coverageStore) CreateCompany(ctx context.Context, company masterdata.Company) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if company.ID == (types.CompanyID{}) || company.Code == "" {
		return masterdata.ErrInvalidCompany
	}
	if _, exists := s.companies[company.ID]; exists {
		return masterdata.ErrDuplicateCompanyCode
	}
	s.companies[company.ID] = company
	return nil
}

func (s *coverageStore) GetCompany(ctx context.Context, id types.CompanyID) (masterdata.Company, error) {
	if err := ctx.Err(); err != nil {
		return masterdata.Company{}, err
	}
	company, ok := s.companies[id]
	if !ok {
		return masterdata.Company{}, masterdata.ErrInvalidCompany
	}
	return company, nil
}

func (s *coverageStore) CreateJournal(ctx context.Context, journal masterdata.Journal) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := journal.Validate(); err != nil {
		return err
	}
	if _, ok := s.companies[journal.CompanyID]; !ok {
		return masterdata.ErrCompanyNotFound
	}
	if s.journals[journal.CompanyID] == nil {
		s.journals[journal.CompanyID] = make(map[string]masterdata.Journal)
		s.journalsByID[journal.CompanyID] = make(map[types.JournalID]masterdata.Journal)
	}
	if _, exists := s.journals[journal.CompanyID][journal.Code]; exists {
		return masterdata.ErrDuplicateJournalCode
	}
	s.journals[journal.CompanyID][journal.Code] = journal
	s.journalsByID[journal.CompanyID][journal.ID] = journal
	return nil
}

func (s *coverageStore) GetJournalByCode(ctx context.Context, companyID types.CompanyID, code string) (masterdata.Journal, error) {
	if err := ctx.Err(); err != nil {
		return masterdata.Journal{}, err
	}
	journal, ok := s.journals[companyID][code]
	if !ok {
		return masterdata.Journal{}, masterdata.ErrJournalNotFound
	}
	return journal, nil
}

func (s *coverageStore) GetJournalByID(ctx context.Context, companyID types.CompanyID, id types.JournalID) (masterdata.Journal, error) {
	if err := ctx.Err(); err != nil {
		return masterdata.Journal{}, err
	}
	journal, ok := s.journalsByID[companyID][id]
	if !ok {
		return masterdata.Journal{}, masterdata.ErrJournalNotFound
	}
	return journal, nil
}

func (s *coverageStore) CreateAccount(ctx context.Context, account masterdata.Account) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if account.ID == (types.AccountID{}) || account.Code == "" {
		return masterdata.ErrInvalidAccount
	}
	if _, ok := s.companies[account.CompanyID]; !ok {
		return masterdata.ErrCompanyNotFound
	}
	if s.accounts[account.CompanyID] == nil {
		s.accounts[account.CompanyID] = make(map[types.AccountID]masterdata.Account)
		s.accountsByCode[account.CompanyID] = make(map[string]masterdata.Account)
	}
	if _, exists := s.accounts[account.CompanyID][account.ID]; exists {
		return masterdata.ErrDuplicateAccountCode
	}
	s.accounts[account.CompanyID][account.ID] = account
	s.accountsByCode[account.CompanyID][account.Code] = account
	return nil
}

func (s *coverageStore) GetAccount(ctx context.Context, companyID types.CompanyID, id types.AccountID) (masterdata.Account, error) {
	if err := ctx.Err(); err != nil {
		return masterdata.Account{}, err
	}
	account, ok := s.accounts[companyID][id]
	if !ok {
		return masterdata.Account{}, masterdata.ErrAccountNotFound
	}
	return account, nil
}

func (s *coverageStore) GetAccountByCode(ctx context.Context, companyID types.CompanyID, code string) (masterdata.Account, error) {
	if err := ctx.Err(); err != nil {
		return masterdata.Account{}, err
	}
	account, ok := s.accountsByCode[companyID][code]
	if !ok {
		return masterdata.Account{}, masterdata.ErrAccountNotFound
	}
	return account, nil
}

func (s *coverageStore) CreateFiscalYear(ctx context.Context, year masterdata.FiscalYear) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if year.ID == (types.FiscalYearID{}) || year.Name == "" {
		return masterdata.ErrInvalidFiscalYear
	}
	if s.fiscalYears[year.CompanyID] == nil {
		s.fiscalYears[year.CompanyID] = make(map[types.FiscalYearID]masterdata.FiscalYear)
	}
	s.fiscalYears[year.CompanyID][year.ID] = year
	return nil
}

func (s *coverageStore) GetFiscalYear(ctx context.Context, companyID types.CompanyID, id types.FiscalYearID) (masterdata.FiscalYear, error) {
	if err := ctx.Err(); err != nil {
		return masterdata.FiscalYear{}, err
	}
	year, ok := s.fiscalYears[companyID][id]
	if !ok {
		return masterdata.FiscalYear{}, masterdata.ErrFiscalYearNotFound
	}
	return year, nil
}

func (s *coverageStore) CreateFiscalPeriod(ctx context.Context, period masterdata.FiscalPeriod) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if period.ID == (types.FiscalPeriodID{}) || period.Number <= 0 {
		return masterdata.ErrInvalidFiscalPeriod
	}
	if s.fiscalPeriods[period.CompanyID] == nil {
		s.fiscalPeriods[period.CompanyID] = make(map[types.FiscalPeriodID]masterdata.FiscalPeriod)
	}
	if s.periodByNumber[period.CompanyID] == nil {
		s.periodByNumber[period.CompanyID] = make(map[types.FiscalYearID]map[int]masterdata.FiscalPeriod)
	}
	if s.periodByNumber[period.CompanyID][period.FiscalYearID] == nil {
		s.periodByNumber[period.CompanyID][period.FiscalYearID] = make(map[int]masterdata.FiscalPeriod)
	}
	s.fiscalPeriods[period.CompanyID][period.ID] = period
	s.periodByNumber[period.CompanyID][period.FiscalYearID][period.Number] = period
	return nil
}

func (s *coverageStore) GetFiscalPeriod(ctx context.Context, companyID types.CompanyID, id types.FiscalPeriodID) (masterdata.FiscalPeriod, error) {
	if err := ctx.Err(); err != nil {
		return masterdata.FiscalPeriod{}, err
	}
	period, ok := s.fiscalPeriods[companyID][id]
	if !ok {
		return masterdata.FiscalPeriod{}, masterdata.ErrPeriodNotFound
	}
	return period, nil
}

func (s *coverageStore) GetPeriodByNumber(ctx context.Context, companyID types.CompanyID, yearID types.FiscalYearID, number int) (masterdata.FiscalPeriod, error) {
	if err := ctx.Err(); err != nil {
		return masterdata.FiscalPeriod{}, err
	}
	period, ok := s.periodByNumber[companyID][yearID][number]
	if !ok {
		return masterdata.FiscalPeriod{}, masterdata.ErrPeriodNotFound
	}
	return period, nil
}

type coverageUnsupportedStore struct{}

func (coverageUnsupportedStore) CreateCompany(context.Context, masterdata.Company) error {
	return masterdata.ErrCapabilityNotImplemented
}
func (coverageUnsupportedStore) GetCompany(context.Context, types.CompanyID) (masterdata.Company, error) {
	return masterdata.Company{}, masterdata.ErrCapabilityNotImplemented
}
func (coverageUnsupportedStore) CreateJournal(context.Context, masterdata.Journal) error {
	return masterdata.ErrCapabilityNotImplemented
}
func (coverageUnsupportedStore) GetJournalByCode(context.Context, types.CompanyID, string) (masterdata.Journal, error) {
	return masterdata.Journal{}, masterdata.ErrCapabilityNotImplemented
}
func (coverageUnsupportedStore) CreateAccount(context.Context, masterdata.Account) error {
	return masterdata.ErrCapabilityNotImplemented
}
func (coverageUnsupportedStore) GetAccount(context.Context, types.CompanyID, types.AccountID) (masterdata.Account, error) {
	return masterdata.Account{}, masterdata.ErrCapabilityNotImplemented
}
func (coverageUnsupportedStore) GetAccountByCode(context.Context, types.CompanyID, string) (masterdata.Account, error) {
	return masterdata.Account{}, masterdata.ErrCapabilityNotImplemented
}
func (coverageUnsupportedStore) CreateFiscalYear(context.Context, masterdata.FiscalYear) error {
	return masterdata.ErrCapabilityNotImplemented
}
func (coverageUnsupportedStore) GetFiscalYear(context.Context, types.CompanyID, types.FiscalYearID) (masterdata.FiscalYear, error) {
	return masterdata.FiscalYear{}, masterdata.ErrCapabilityNotImplemented
}
func (coverageUnsupportedStore) CreateFiscalPeriod(context.Context, masterdata.FiscalPeriod) error {
	return masterdata.ErrCapabilityNotImplemented
}
func (coverageUnsupportedStore) GetFiscalPeriod(context.Context, types.CompanyID, types.FiscalPeriodID) (masterdata.FiscalPeriod, error) {
	return masterdata.FiscalPeriod{}, masterdata.ErrCapabilityNotImplemented
}

func TestCoverageMasterDataContractAgainstMemoryStore(t *testing.T) {
	Run(t, newCoverageStore())
}

func TestCoverageMasterDataContractUnsupportedStore(t *testing.T) {
	AssertUnsupported(t, coverageUnsupportedStore{})
}

// Run exercises error-reporting branches that cannot be reached with a
// conforming store. A detached testing.T keeps those expected failures from
// failing the enclosing test, while still letting the production contract
// helpers execute their reporting statements.
func coverageWithDetachedT(fn func(*testing.T)) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn(&testing.T{})
	}()
	<-done
}

type coverageFaultStore struct {
	*coverageStore
	companyID           types.CompanyID
	generalCreated      bool
	periodGetError      bool
	periodMismatch      bool
	failPeriod13        bool
	failPeriod14        bool
	wrongPeriodByNumber bool
}

func (s *coverageFaultStore) CreateCompany(ctx context.Context, company masterdata.Company) error {
	if err := s.coverageStore.CreateCompany(ctx, company); err != nil {
		return err
	}
	s.companyID = company.ID
	return nil
}

func (s *coverageFaultStore) CreateJournal(ctx context.Context, journal masterdata.Journal) error {
	if ctx.Err() != nil {
		return nil
	}
	switch journal.Code {
	case "GENERAL":
		if s.generalCreated {
			return errors.New("wrong duplicate journal error")
		}
		s.generalCreated = true
		return s.coverageStore.CreateJournal(ctx, journal)
	case "UNKNOWN-COMPANY":
		return errors.New("wrong unknown-company error")
	default:
		return errors.New("wrong invalid-journal error")
	}
}

func (s *coverageFaultStore) GetJournalByCode(
	ctx context.Context, companyID types.CompanyID, code string,
) (masterdata.Journal, error) {
	if ctx.Err() != nil {
		return masterdata.Journal{}, nil
	}
	if companyID != s.companyID {
		return masterdata.Journal{}, errors.New("wrong cross-company error")
	}
	return s.coverageStore.GetJournalByCode(ctx, companyID, code)
}

func (s *coverageFaultStore) GetJournalByID(
	ctx context.Context, companyID types.CompanyID, id types.JournalID,
) (masterdata.Journal, error) {
	if companyID != s.companyID {
		return masterdata.Journal{}, errors.New("wrong cross-company journal-id error")
	}
	return s.coverageStore.GetJournalByID(ctx, companyID, id)
}

func (s *coverageFaultStore) GetAccount(
	ctx context.Context, companyID types.CompanyID, id types.AccountID,
) (masterdata.Account, error) {
	return masterdata.Account{}, nil
}

func (s *coverageFaultStore) GetAccountByCode(
	ctx context.Context, companyID types.CompanyID, code string,
) (masterdata.Account, error) {
	return masterdata.Account{}, nil
}

func (s *coverageFaultStore) GetFiscalYear(
	ctx context.Context, companyID types.CompanyID, id types.FiscalYearID,
) (masterdata.FiscalYear, error) {
	return masterdata.FiscalYear{}, nil
}

func (s *coverageFaultStore) CreateFiscalPeriod(ctx context.Context, period masterdata.FiscalPeriod) error {
	if s.failPeriod13 && period.Number == 13 {
		return errors.New("wrong period 13 error")
	}
	if s.failPeriod14 && period.Number == 14 {
		return errors.New("wrong period 14 error")
	}
	return s.coverageStore.CreateFiscalPeriod(ctx, period)
}

func (s *coverageFaultStore) GetFiscalPeriod(
	ctx context.Context, companyID types.CompanyID, id types.FiscalPeriodID,
) (masterdata.FiscalPeriod, error) {
	if s.periodGetError {
		return masterdata.FiscalPeriod{}, errors.New("wrong period lookup error")
	}
	period, err := s.coverageStore.GetFiscalPeriod(ctx, companyID, id)
	if err != nil {
		return period, err
	}
	if s.periodMismatch {
		period.ID = types.FiscalPeriodID(uuid.New())
	}
	return period, nil
}

func (s *coverageFaultStore) GetPeriodByNumber(
	ctx context.Context, companyID types.CompanyID, yearID types.FiscalYearID, number int,
) (masterdata.FiscalPeriod, error) {
	if !s.wrongPeriodByNumber {
		return s.coverageStore.GetPeriodByNumber(ctx, companyID, yearID, number)
	}
	if companyID != s.companyID {
		return masterdata.FiscalPeriod{}, errors.New("wrong cross-company period error")
	}
	period, err := s.coverageStore.GetPeriodByNumber(ctx, companyID, yearID, number)
	if err != nil {
		return period, err
	}
	period.Type = masterdata.FiscalPeriodTypeNormal
	return period, nil
}

type coverageUnsupportedFaultStore struct {
	coverageUnsupportedStore
}

func (coverageUnsupportedFaultStore) GetCompany(
	context.Context, types.CompanyID,
) (masterdata.Company, error) {
	return masterdata.Company{}, nil
}

func TestCoverageMasterDataContractErrorReports(t *testing.T) {
	coverageWithDetachedT(func(dt *testing.T) {
		Run(dt, &coverageFaultStore{
			coverageStore:  newCoverageStore(),
			periodMismatch: true,
		})
	})

	coverageWithDetachedT(func(dt *testing.T) {
		Run(dt, &coverageFaultStore{
			coverageStore:  newCoverageStore(),
			periodGetError: true,
		})
	})

	coverageWithDetachedT(func(dt *testing.T) {
		Run(dt, &coverageFaultStore{
			coverageStore: newCoverageStore(),
			failPeriod13:  true,
		})
	})

	coverageWithDetachedT(func(dt *testing.T) {
		Run(dt, &coverageFaultStore{
			coverageStore: newCoverageStore(),
			failPeriod14:  true,
		})
	})

	coverageWithDetachedT(func(dt *testing.T) {
		Run(dt, &coverageFaultStore{
			coverageStore:       newCoverageStore(),
			wrongPeriodByNumber: true,
		})
	})

	coverageWithDetachedT(func(dt *testing.T) {
		AssertUnsupported(dt, coverageUnsupportedFaultStore{})
	})
}
