package masterdatacontract

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/masterdata"
)

// coverageModeStore drives one specific failure branch of Run per mode so the
// production contract's reporting statements can be covered deterministically.
type coverageModeStore struct {
	*coverageStore
	mode           string
	companyID      types.CompanyID
	generalCreated bool
}

func (s *coverageModeStore) CreateCompany(ctx context.Context, company masterdata.Company) error {
	if s.mode == "createCompany" {
		return errors.New("create company failed")
	}
	if err := s.coverageStore.CreateCompany(ctx, company); err != nil {
		return err
	}
	s.companyID = company.ID
	return nil
}

func (s *coverageModeStore) GetCompany(
	ctx context.Context, id types.CompanyID,
) (masterdata.Company, error) {
	if s.mode == "getCompany" {
		return masterdata.Company{}, errors.New("get company failed")
	}
	return s.coverageStore.GetCompany(ctx, id)
}

func (s *coverageModeStore) CreateJournal(ctx context.Context, journal masterdata.Journal) error {
	if ctx.Err() != nil {
		return nil
	}
	switch journal.Code {
	case "GENERAL":
		if s.mode == "createJournal" {
			return errors.New("create journal failed")
		}
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

func (s *coverageModeStore) GetJournalByCode(
	ctx context.Context, companyID types.CompanyID, code string,
) (masterdata.Journal, error) {
	if ctx.Err() != nil {
		return masterdata.Journal{}, nil
	}
	if companyID != s.companyID {
		return masterdata.Journal{}, errors.New("wrong cross-company error")
	}
	if s.mode == "getJournal" {
		return masterdata.Journal{ID: masterdata.NewJournalID()}, nil
	}
	return s.coverageStore.GetJournalByCode(ctx, companyID, code)
}

func (s *coverageModeStore) GetJournalByID(
	ctx context.Context, companyID types.CompanyID, id types.JournalID,
) (masterdata.Journal, error) {
	if companyID != s.companyID {
		return masterdata.Journal{}, errors.New("wrong cross-company journal-id error")
	}
	if s.mode == "getJournalByID" {
		return masterdata.Journal{ID: masterdata.NewJournalID()}, nil
	}
	return s.coverageStore.GetJournalByID(ctx, companyID, id)
}

func (s *coverageModeStore) GetAccount(
	ctx context.Context, companyID types.CompanyID, id types.AccountID,
) (masterdata.Account, error) {
	if s.mode == "getAccount" {
		return masterdata.Account{}, errors.New("get account failed")
	}
	return s.coverageStore.GetAccount(ctx, companyID, id)
}

func (s *coverageModeStore) GetAccountByCode(
	ctx context.Context, companyID types.CompanyID, code string,
) (masterdata.Account, error) {
	if s.mode == "getAccountByCode" {
		return masterdata.Account{}, errors.New("get account by code failed")
	}
	return s.coverageStore.GetAccountByCode(ctx, companyID, code)
}

func (s *coverageModeStore) CreateFiscalYear(ctx context.Context, year masterdata.FiscalYear) error {
	if s.mode == "createFiscalYear" {
		return errors.New("create fiscal year failed")
	}
	return s.coverageStore.CreateFiscalYear(ctx, year)
}

func (s *coverageModeStore) GetFiscalYear(
	ctx context.Context, companyID types.CompanyID, id types.FiscalYearID,
) (masterdata.FiscalYear, error) {
	if s.mode == "getFiscalYear" {
		return masterdata.FiscalYear{}, errors.New("get fiscal year failed")
	}
	return s.coverageStore.GetFiscalYear(ctx, companyID, id)
}

func (s *coverageModeStore) CreateFiscalPeriod(ctx context.Context, period masterdata.FiscalPeriod) error {
	if s.mode == "createFiscalPeriod" && period.Number == 1 {
		return errors.New("create fiscal period failed")
	}
	if s.mode == "failPeriod13" && period.Number == 13 {
		return errors.New("wrong period 13 error")
	}
	if s.mode == "failPeriod14" && period.Number == 14 {
		return errors.New("wrong period 14 error")
	}
	return s.coverageStore.CreateFiscalPeriod(ctx, period)
}

func (s *coverageModeStore) GetFiscalPeriod(
	ctx context.Context, companyID types.CompanyID, id types.FiscalPeriodID,
) (masterdata.FiscalPeriod, error) {
	if s.mode == "getFiscalPeriod" {
		return masterdata.FiscalPeriod{}, errors.New("get fiscal period failed")
	}
	period, err := s.coverageStore.GetFiscalPeriod(ctx, companyID, id)
	if err != nil {
		return period, err
	}
	if s.mode == "periodMismatch" {
		period.ID = types.FiscalPeriodID(uuid.New())
	}
	return period, nil
}

func (s *coverageModeStore) GetPeriodByNumber(
	ctx context.Context, companyID types.CompanyID, yearID types.FiscalYearID, number int,
) (masterdata.FiscalPeriod, error) {
	if s.mode != "wrongPeriodByNumber" {
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

func TestCoverageMasterDataContractModeErrorReports(t *testing.T) {
	modes := []string{
		"createCompany", "getCompany", "createJournal", "getJournal", "getJournalByID",
		"getAccount", "getAccountByCode", "createFiscalYear", "getFiscalYear",
		"createFiscalPeriod", "getFiscalPeriod", "periodMismatch",
		"failPeriod13", "failPeriod14", "wrongPeriodByNumber",
	}
	for _, mode := range modes {
		t.Run(mode, func(t *testing.T) {
			coverageWithDetachedT(func(dt *testing.T) {
				Run(dt, &coverageModeStore{mode: mode, coverageStore: newCoverageStore()})
			})
		})
	}
}
