package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/masterdata"
)

var _ masterdata.Store = (*Driver)(nil)

func (d *Driver) CreateCompany(ctx context.Context, company masterdata.Company) error {
	company.Code = strings.ToUpper(strings.TrimSpace(company.Code))
	company.FunctionalCurrency = strings.ToUpper(strings.TrimSpace(company.FunctionalCurrency))
	company.PresentationCurrency = strings.ToUpper(strings.TrimSpace(company.PresentationCurrency))
	if company.ID == (types.CompanyID{}) || company.Code == "" {
		return masterdata.ErrInvalidCompany
	}
	return d.WithCompanyTx(ctx, uuid.UUID(company.ID), func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO companies(id, code, legal_name, functional_currency, presentation_currency, is_active)
			VALUES($1,$2,$3,$4,$5,$6)
			ON CONFLICT(id) DO UPDATE SET code=EXCLUDED.code, legal_name=EXCLUDED.legal_name,
			functional_currency=EXCLUDED.functional_currency, presentation_currency=EXCLUDED.presentation_currency,
			is_active=EXCLUDED.is_active`,
			uuid.UUID(company.ID), company.Code, company.LegalName, company.FunctionalCurrency,
			company.PresentationCurrency, company.Active)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				return masterdata.ErrDuplicateCompanyCode
			}
			return err
		}
		return nil
	})
}

func (d *Driver) GetCompany(ctx context.Context, id types.CompanyID) (masterdata.Company, error) {
	var company masterdata.Company
	err := d.WithCompanyTx(ctx, uuid.UUID(id), func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx,
			`SELECT id, code, legal_name, functional_currency, presentation_currency, is_active
			 FROM companies WHERE id=$1`, uuid.UUID(id)).Scan(
			&company.ID, &company.Code, &company.LegalName, &company.FunctionalCurrency,
			&company.PresentationCurrency, &company.Active)
		if errors.Is(err, pgx.ErrNoRows) {
			return masterdata.ErrCompanyNotFound
		}
		return err
	})
	return company, err
}

func (d *Driver) CreateJournal(ctx context.Context, journal masterdata.Journal) error {
	if err := journal.Validate(); err != nil {
		return err
	}
	companyID := uuid.UUID(journal.CompanyID)
	return d.WithCompanyTx(ctx, companyID, func(tx pgx.Tx) error {
		var companyExists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM companies WHERE id=$1)`, companyID).Scan(&companyExists); err != nil {
			return err
		}
		if !companyExists {
			return masterdata.ErrCompanyNotFound
		}
		_, err := tx.Exec(ctx, `INSERT INTO journals(id, company_id, code, name) VALUES($1,$2,$3,$4)`,
			uuid.UUID(journal.ID), companyID, journal.Code, journal.Name)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				switch pgErr.ConstraintName {
				case "uq_journals_code":
					return masterdata.ErrDuplicateJournalCode
				case "journals_pkey":
					return masterdata.ErrDuplicateJournalID
				default:
					return masterdata.ErrDuplicateJournalCode
				}
			}
			return err
		}
		return nil
	})
}

func (d *Driver) GetJournalByCode(ctx context.Context, companyID types.CompanyID, code string) (masterdata.Journal, error) {
	if companyID == (types.CompanyID{}) || strings.TrimSpace(code) == "" || utf8.RuneCountInString(code) > 32 {
		return masterdata.Journal{}, masterdata.ErrInvalidJournal
	}
	var journal masterdata.Journal
	err := d.WithCompanyTx(ctx, uuid.UUID(companyID), func(tx pgx.Tx) error {
		var jID, cID uuid.UUID
		err := tx.QueryRow(ctx, `SELECT id, company_id, code, name FROM journals WHERE company_id=$1 AND code=$2`,
			uuid.UUID(companyID), code).Scan(&jID, &cID, &journal.Code, &journal.Name)
		if errors.Is(err, pgx.ErrNoRows) {
			return masterdata.ErrJournalNotFound
		}
		if err != nil {
			return err
		}
		journal.ID = masterdata.JournalID(jID)
		journal.CompanyID = types.CompanyID(cID)
		return nil
	})
	return journal, err
}

const accountSelect = `SELECT id, company_id, code, name, account_type, normal_balance, currency_id, parent_id,
	is_reconcilable, is_control_account, is_contra_account, is_active, requires_cost_center, is_cash_equivalent, is_non_cash_adjustment,
	cash_flow_activity, sub_classification, ifrs_category, display_order
	FROM accounts`

func (d *Driver) CreateAccount(ctx context.Context, account masterdata.Account) error {
	if account.ID == (types.AccountID{}) || strings.TrimSpace(account.Code) == "" || strings.TrimSpace(account.Name) == "" {
		return masterdata.ErrInvalidAccount
	}
	return d.WithCompanyTx(ctx, uuid.UUID(account.CompanyID), func(tx pgx.Tx) error {
		var parentID *uuid.UUID
		if account.ParentID != nil {
			u := uuid.UUID(*account.ParentID)
			parentID = &u
		}
		_, err := tx.Exec(ctx,
			`INSERT INTO accounts (
				id, company_id, code, name, account_type, normal_balance, currency_id, parent_id,
				is_reconcilable, is_control_account, is_contra_account, is_active, requires_cost_center,
				is_cash_equivalent, is_non_cash_adjustment, cash_flow_activity, sub_classification,
				ifrs_category, display_order, created_at, updated_at
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,NOW(),NOW())
			ON CONFLICT(id) DO UPDATE SET
				code=EXCLUDED.code, name=EXCLUDED.name, account_type=EXCLUDED.account_type,
				normal_balance=EXCLUDED.normal_balance, currency_id=EXCLUDED.currency_id,
				parent_id=EXCLUDED.parent_id, is_reconcilable=EXCLUDED.is_reconcilable,
				is_control_account=EXCLUDED.is_control_account, is_contra_account=EXCLUDED.is_contra_account,
				is_active=EXCLUDED.is_active, requires_cost_center=EXCLUDED.requires_cost_center,
				is_cash_equivalent=EXCLUDED.is_cash_equivalent, is_non_cash_adjustment=EXCLUDED.is_non_cash_adjustment,
				cash_flow_activity=EXCLUDED.cash_flow_activity, sub_classification=EXCLUDED.sub_classification,
				ifrs_category=EXCLUDED.ifrs_category, display_order=EXCLUDED.display_order, updated_at=NOW()`,
			uuid.UUID(account.ID), uuid.UUID(account.CompanyID), account.Code, account.Name,
			string(account.Type), string(account.NormalBalance), nullableString(account.Currency, "USD"), parentID,
			account.IsReconcilable, account.IsControlAccount, account.IsContraAccount, account.IsActive,
			account.RequiresCostCenter, account.IsCashEquivalent, account.IsNonCashAdjustment,
			nullableString(account.CashFlowActivity, "unclassified"),
			nullableString(account.SubClassification, "unclassified"),
			nullableString(account.IFRSCategory, "unclassified"), account.DisplayOrder)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				return masterdata.ErrDuplicateAccountCode
			}
			return err
		}
		return nil
	})
}

func (d *Driver) GetAccount(ctx context.Context, companyID types.CompanyID, accountID types.AccountID) (masterdata.Account, error) {
	var account masterdata.Account
	err := d.WithCompanyTx(ctx, uuid.UUID(companyID), func(tx pgx.Tx) error {
		var aID, cID uuid.UUID
		var parentID *uuid.UUID
		var aType, nBal string
		err := tx.QueryRow(ctx, accountSelect+" WHERE company_id=$1 AND id=$2",
			uuid.UUID(companyID), uuid.UUID(accountID)).Scan(
			&aID, &cID, &account.Code, &account.Name, &aType, &nBal,
			&account.Currency, &parentID, &account.IsReconcilable, &account.IsControlAccount,
			&account.IsContraAccount, &account.IsActive, &account.RequiresCostCenter,
			&account.IsCashEquivalent, &account.IsNonCashAdjustment, &account.CashFlowActivity,
			&account.SubClassification, &account.IFRSCategory, &account.DisplayOrder,
		)
		if errors.Is(err, pgx.ErrNoRows) {
			return masterdata.ErrAccountNotFound
		}
		if err != nil {
			return err
		}
		account.ID = types.AccountID(aID)
		account.CompanyID = types.CompanyID(cID)
		account.Type = masterdata.AccountType(aType)
		account.NormalBalance = masterdata.NormalBalance(nBal)
		if parentID != nil {
			pid := types.AccountID(*parentID)
			account.ParentID = &pid
		}
		return nil
	})
	return account, err
}

func (d *Driver) GetAccountByCode(ctx context.Context, companyID types.CompanyID, code string) (masterdata.Account, error) {
	var account masterdata.Account
	err := d.WithCompanyTx(ctx, uuid.UUID(companyID), func(tx pgx.Tx) error {
		var aID, cID uuid.UUID
		var parentID *uuid.UUID
		var aType, nBal string
		err := tx.QueryRow(ctx, accountSelect+" WHERE company_id=$1 AND lower(code)=lower($2)",
			uuid.UUID(companyID), code).Scan(
			&aID, &cID, &account.Code, &account.Name, &aType, &nBal,
			&account.Currency, &parentID, &account.IsReconcilable, &account.IsControlAccount,
			&account.IsContraAccount, &account.IsActive, &account.RequiresCostCenter,
			&account.IsCashEquivalent, &account.IsNonCashAdjustment, &account.CashFlowActivity,
			&account.SubClassification, &account.IFRSCategory, &account.DisplayOrder,
		)
		if errors.Is(err, pgx.ErrNoRows) {
			return masterdata.ErrAccountNotFound
		}
		if err != nil {
			return err
		}
		account.ID = types.AccountID(aID)
		account.CompanyID = types.CompanyID(cID)
		account.Type = masterdata.AccountType(aType)
		account.NormalBalance = masterdata.NormalBalance(nBal)
		if parentID != nil {
			pid := types.AccountID(*parentID)
			account.ParentID = &pid
		}
		return nil
	})
	return account, err
}

func (d *Driver) HasAccountChildren(ctx context.Context, companyID types.CompanyID, accountID types.AccountID) (bool, error) {
	var hasChildren bool
	err := d.WithCompanyTx(ctx, uuid.UUID(companyID), func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM accounts WHERE company_id=$1 AND parent_id=$2)`,
			uuid.UUID(companyID), uuid.UUID(accountID)).Scan(&hasChildren)
	})
	return hasChildren, err
}

func (d *Driver) CreateFiscalYear(ctx context.Context, year masterdata.FiscalYear) error {
	state := year.State
	if state == "" {
		state = masterdata.FiscalYearOpen
	}
	yearData, err := json.Marshal(year)
	if err != nil {
		return err
	}
	return d.WithCompanyTx(ctx, uuid.UUID(year.CompanyID), func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO fiscal_years(id, company_id, code, year, start_date, end_date, state, is_closed, year_data)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)
			ON CONFLICT(id) DO UPDATE SET code=EXCLUDED.code, start_date=EXCLUDED.start_date,
			end_date=EXCLUDED.end_date, state=EXCLUDED.state, is_closed=EXCLUDED.is_closed, year_data=EXCLUDED.year_data`,
			uuid.UUID(year.ID), uuid.UUID(year.CompanyID), year.Name, year.StartDate.UTC().Year(),
			year.StartDate, year.EndDate, state, state == masterdata.FiscalYearHardClosed, yearData)
		return err
	})
}

func (d *Driver) GetFiscalYear(ctx context.Context, companyID types.CompanyID, fiscalYearID types.FiscalYearID) (masterdata.FiscalYear, error) {
	var year masterdata.FiscalYear
	err := d.WithCompanyTx(ctx, uuid.UUID(companyID), func(tx pgx.Tx) error {
		var yID, cID uuid.UUID
		var state string
		err := tx.QueryRow(ctx, `SELECT id, company_id, code, start_date, end_date, state
			FROM fiscal_years WHERE company_id=$1 AND id=$2`,
			uuid.UUID(companyID), uuid.UUID(fiscalYearID)).Scan(
			&yID, &cID, &year.Name, &year.StartDate, &year.EndDate, &state)
		if errors.Is(err, pgx.ErrNoRows) {
			return masterdata.ErrFiscalYearNotFound
		}
		if err != nil {
			return err
		}
		year.ID = types.FiscalYearID(yID)
		year.CompanyID = types.CompanyID(cID)
		year.State = masterdata.FiscalYearState(state)
		return nil
	})
	return year, err
}

func (d *Driver) CreateFiscalPeriod(ctx context.Context, period masterdata.FiscalPeriod) error {
	state := period.State
	if state == "" {
		state = masterdata.PeriodOpen
	}
	periodType := period.Type
	if periodType == "" {
		periodType = masterdata.FiscalPeriodTypeNormal
	}
	var closedAt any
	if state != masterdata.PeriodOpen {
		closedAt = time.Now().UTC()
	}
	return d.WithCompanyTx(ctx, uuid.UUID(period.CompanyID), func(tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fiscal_years WHERE company_id=$1 AND id=$2)`,
			uuid.UUID(period.CompanyID), uuid.UUID(period.FiscalYearID)).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return masterdata.ErrFiscalYearNotFound
		}
		_, err := tx.Exec(ctx, `INSERT INTO fiscal_periods(
			id,company_id,year_id,code,start_date,end_date,state,period_number,name,period_type,closed_at
		) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
			uuid.UUID(period.ID), uuid.UUID(period.CompanyID), uuid.UUID(period.FiscalYearID),
			fmt.Sprintf("%02d", period.Number), period.StartDate, period.EndDate, state,
			period.Number, period.Name, periodType, closedAt)
		return err
	})
}

func (d *Driver) GetFiscalPeriod(ctx context.Context, companyID types.CompanyID, periodID types.FiscalPeriodID) (masterdata.FiscalPeriod, error) {
	var period masterdata.FiscalPeriod
	err := d.WithCompanyTx(ctx, uuid.UUID(companyID), func(tx pgx.Tx) error {
		var pID, cID, yID uuid.UUID
		var state, pType string
		err := tx.QueryRow(ctx, `SELECT id, company_id, year_id, period_number, name, start_date, end_date, state, period_type
			FROM fiscal_periods WHERE company_id=$1 AND id=$2`,
			uuid.UUID(companyID), uuid.UUID(periodID)).Scan(
			&pID, &cID, &yID, &period.Number, &period.Name,
			&period.StartDate, &period.EndDate, &state, &pType)
		if errors.Is(err, pgx.ErrNoRows) {
			return masterdata.ErrPeriodNotFound
		}
		if err != nil {
			return err
		}
		period.ID = types.FiscalPeriodID(pID)
		period.CompanyID = types.CompanyID(cID)
		period.FiscalYearID = types.FiscalYearID(yID)
		period.State = masterdata.PeriodState(state)
		period.Type = masterdata.FiscalPeriodType(pType)
		return nil
	})
	return period, err
}

func (d *Driver) GetPeriodForDate(ctx context.Context, companyID types.CompanyID, date time.Time) (masterdata.FiscalPeriod, error) {
	var period masterdata.FiscalPeriod
	date = date.UTC().Truncate(time.Microsecond)
	err := d.WithCompanyTx(ctx, uuid.UUID(companyID), func(tx pgx.Tx) error {
		var pID, cID, yID uuid.UUID
		var state, pType string
		err := tx.QueryRow(ctx, `SELECT id, company_id, year_id, period_number, name, start_date, end_date, state, period_type
			FROM fiscal_periods WHERE company_id=$1 AND start_date <= $2 AND end_date >= $2 AND period_type='normal'
			ORDER BY period_number ASC LIMIT 1`, uuid.UUID(companyID), date).Scan(
			&pID, &cID, &yID, &period.Number, &period.Name,
			&period.StartDate, &period.EndDate, &state, &pType)
		if errors.Is(err, pgx.ErrNoRows) {
			return masterdata.ErrPeriodNotFound
		}
		if err != nil {
			return err
		}
		period.ID = types.FiscalPeriodID(pID)
		period.CompanyID = types.CompanyID(cID)
		period.FiscalYearID = types.FiscalYearID(yID)
		period.State = masterdata.PeriodState(state)
		period.Type = masterdata.FiscalPeriodType(pType)
		return nil
	})
	return period, err
}
