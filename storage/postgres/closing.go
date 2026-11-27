package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/journal"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/masterdata"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/storage"
)

var _ storage.YearEndClosingStore = (*Driver)(nil)

func (d *Driver) ListAccounts(ctx context.Context, companyID types.CompanyID) ([]masterdata.Account, error) {
	if err := d.assertReadScope(ctx, companyID, masterdata.ErrCompanyNotFound); err != nil {
		return nil, err
	}
	var accounts []masterdata.Account
	err := d.WithCompanyTx(ctx, uuid.UUID(companyID), func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, accountSelect+` WHERE company_id=$1 ORDER BY code ASC`, uuid.UUID(companyID))
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var acc masterdata.Account
			var aID, cID uuid.UUID
			var parentID *uuid.UUID
			var aType, nBal string
			if err := rows.Scan(
				&aID, &cID, &acc.Code, &acc.Name, &aType, &nBal,
				&acc.Currency, &parentID, &acc.IsReconcilable, &acc.IsControlAccount,
				&acc.IsContraAccount, &acc.IsActive, &acc.RequiresCostCenter,
				&acc.IsMonetary, &acc.IsCashEquivalent, &acc.IsNonCashAdjustment, &acc.CashFlowActivity,
				&acc.SubClassification, &acc.IFRSCategory, &acc.DisplayOrder,
			); err != nil {
				return err
			}
			acc.ID = types.AccountID(aID)
			acc.CompanyID = types.CompanyID(cID)
			acc.Type = masterdata.AccountType(aType)
			acc.NormalBalance = masterdata.NormalBalance(nBal)
			if parentID != nil {
				pid := types.AccountID(*parentID)
				acc.ParentID = &pid
			}
			accounts = append(accounts, acc)
		}
		return rows.Err()
	})
	return accounts, err
}

func (d *Driver) ListFiscalYears(ctx context.Context, companyID types.CompanyID) ([]masterdata.FiscalYear, error) {
	if err := d.assertReadScope(ctx, companyID, masterdata.ErrFiscalYearNotFound); err != nil {
		return nil, err
	}
	var years []masterdata.FiscalYear
	err := d.WithCompanyTx(ctx, uuid.UUID(companyID), func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id, company_id, COALESCE(name, year::text), start_date, end_date, state
			FROM fiscal_years WHERE company_id=$1 ORDER BY start_date ASC`, uuid.UUID(companyID))
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var yID, cID uuid.UUID
			var state string
			var y masterdata.FiscalYear
			if err := rows.Scan(&yID, &cID, &y.Name, &y.StartDate, &y.EndDate, &state); err != nil {
				return err
			}
			y.ID = types.FiscalYearID(yID)
			y.CompanyID = types.CompanyID(cID)
			y.State = masterdata.FiscalYearState(state)
			years = append(years, y)
		}
		return rows.Err()
	})
	return years, err
}

func (d *Driver) ListFiscalPeriods(ctx context.Context, companyID types.CompanyID, fiscalYearID types.FiscalYearID) ([]masterdata.FiscalPeriod, error) {
	if err := d.assertReadScope(ctx, companyID, masterdata.ErrPeriodNotFound); err != nil {
		return nil, err
	}
	var periods []masterdata.FiscalPeriod
	err := d.WithCompanyTx(ctx, uuid.UUID(companyID), func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id, company_id, year_id, period_number, name, start_date, end_date, state, period_type
			FROM fiscal_periods WHERE company_id=$1 AND year_id=$2 ORDER BY period_number ASC`,
			uuid.UUID(companyID), uuid.UUID(fiscalYearID))
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var pID, cID, yID uuid.UUID
			var state, pType string
			var p masterdata.FiscalPeriod
			if err := rows.Scan(&pID, &cID, &yID, &p.Number, &p.Name, &p.StartDate, &p.EndDate, &state, &pType); err != nil {
				return err
			}
			p.ID = types.FiscalPeriodID(pID)
			p.CompanyID = types.CompanyID(cID)
			p.FiscalYearID = types.FiscalYearID(yID)
			p.State = masterdata.PeriodState(state)
			p.Type = masterdata.FiscalPeriodType(pType)
			periods = append(periods, p)
		}
		return rows.Err()
	})
	return periods, err
}

func (d *Driver) ListEntriesByDateRange(ctx context.Context, companyID types.CompanyID, start, end time.Time) ([]journal.JournalEntry, error) {
	if err := d.assertReadScope(ctx, companyID, ErrEntryNotFound); err != nil {
		return nil, err
	}
	var startArg, endArg any
	if !start.IsZero() {
		startArg = start.UTC()
	}
	if !end.IsZero() {
		endArg = end.UTC()
	}
	var entries []journal.JournalEntry
	err := d.WithCompanyTx(ctx, uuid.UUID(companyID), func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT entry FROM account_moves
			WHERE company_id=$1 AND status IN ('posted', 'reversed')
			  AND ($2::timestamptz IS NULL OR entry_date >= $2)
			  AND ($3::timestamptz IS NULL OR entry_date <= $3)
			ORDER BY entry_date ASC, id ASC`,
			uuid.UUID(companyID), startArg, endArg)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var data []byte
			if err := rows.Scan(&data); err != nil {
				return err
			}
			entry, err := unmarshalEntryFromStorage(data)
			if err != nil {
				return err
			}
			entries = append(entries, entry)
		}
		return rows.Err()
	})
	return entries, err
}

func (d *Driver) UpdateFiscalPeriodState(ctx context.Context, companyID types.CompanyID, periodID types.FiscalPeriodID, state masterdata.PeriodState, actorID, reason string) error {
	return d.WithCompanyTx(ctx, uuid.UUID(companyID), func(tx pgx.Tx) error {
		var closedAt any
		if state != masterdata.PeriodOpen {
			closedAt = time.Now().UTC()
		}
		res, err := tx.Exec(ctx, `UPDATE fiscal_periods
			SET state=$3, closed_at=$4
			WHERE company_id=$1 AND id=$2`,
			uuid.UUID(companyID), uuid.UUID(periodID), string(state), closedAt)
		if err != nil {
			return err
		}
		if res.RowsAffected() != 1 {
			return masterdata.ErrPeriodNotFound
		}
		return nil
	})
}

func (d *Driver) UpdateFiscalYearState(ctx context.Context, companyID types.CompanyID, fiscalYearID types.FiscalYearID, state masterdata.FiscalYearState, actorID, reason string) error {
	return d.WithCompanyTx(ctx, uuid.UUID(companyID), func(tx pgx.Tx) error {
		res, err := tx.Exec(ctx, `UPDATE fiscal_years SET state=$3 WHERE company_id=$1 AND id=$2`,
			uuid.UUID(companyID), uuid.UUID(fiscalYearID), string(state))
		if err != nil {
			return err
		}
		if res.RowsAffected() != 1 {
			return masterdata.ErrFiscalYearNotFound
		}
		return nil
	})
}

func (d *Driver) SaveYearEndCloseRecord(ctx context.Context, record storage.YearEndCloseRecord) error {
	idsJSON, err := json.Marshal(record.ClosingEntryIDs)
	if err != nil {
		return err
	}
	var nextYearID, openingEntryID *uuid.UUID
	if record.NextFiscalYearID != (types.FiscalYearID{}) {
		u := uuid.UUID(record.NextFiscalYearID)
		nextYearID = &u
	}
	if record.OpeningEntryID != (types.JournalEntryID{}) {
		u := uuid.UUID(record.OpeningEntryID)
		openingEntryID = &u
	}
	return d.WithCompanyTx(ctx, uuid.UUID(record.CompanyID), func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO year_end_closings(
			company_id, fiscal_year_id, next_fiscal_year_id, closing_entry_ids, opening_entry_id,
			net_income, policy, closed_by, closed_at, reason, reopened_by, reopened_at, reopen_reason, approval_reference
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		ON CONFLICT (company_id, fiscal_year_id) DO UPDATE SET
			next_fiscal_year_id=EXCLUDED.next_fiscal_year_id,
			closing_entry_ids=EXCLUDED.closing_entry_ids,
			opening_entry_id=EXCLUDED.opening_entry_id,
			net_income=EXCLUDED.net_income,
			policy=EXCLUDED.policy,
			closed_by=EXCLUDED.closed_by,
			closed_at=EXCLUDED.closed_at,
			reason=EXCLUDED.reason,
			reopened_by=EXCLUDED.reopened_by,
			reopened_at=EXCLUDED.reopened_at,
			reopen_reason=EXCLUDED.reopen_reason,
			approval_reference=EXCLUDED.approval_reference`,
			uuid.UUID(record.CompanyID), uuid.UUID(record.FiscalYearID), nextYearID, idsJSON, openingEntryID,
			record.NetIncome, record.Policy, record.ClosedBy, record.ClosedAt.UTC(), record.Reason,
			record.ReopenedBy, record.ReopenedAt, record.ReopenReason, record.ApprovalReference)
		return err
	})
}

func (d *Driver) GetYearEndCloseRecord(ctx context.Context, companyID types.CompanyID, fiscalYearID types.FiscalYearID) (storage.YearEndCloseRecord, bool, error) {
	if err := d.assertReadScope(ctx, companyID, masterdata.ErrFiscalYearNotFound); err != nil {
		return storage.YearEndCloseRecord{}, false, err
	}
	var rec storage.YearEndCloseRecord
	var found bool
	err := d.WithCompanyTx(ctx, uuid.UUID(companyID), func(tx pgx.Tx) error {
		var cID, yID uuid.UUID
		var nextYearID, openingID *uuid.UUID
		var idsJSON []byte
		err := tx.QueryRow(ctx, `SELECT company_id, fiscal_year_id, next_fiscal_year_id, closing_entry_ids, opening_entry_id,
			net_income, policy, closed_by, closed_at, reason, reopened_by, reopened_at, reopen_reason, approval_reference
			FROM year_end_closings WHERE company_id=$1 AND fiscal_year_id=$2`,
			uuid.UUID(companyID), uuid.UUID(fiscalYearID)).Scan(
			&cID, &yID, &nextYearID, &idsJSON, &openingID,
			&rec.NetIncome, &rec.Policy, &rec.ClosedBy, &rec.ClosedAt, &rec.Reason,
			&rec.ReopenedBy, &rec.ReopenedAt, &rec.ReopenReason, &rec.ApprovalReference)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		found = true
		rec.CompanyID = types.CompanyID(cID)
		rec.FiscalYearID = types.FiscalYearID(yID)
		if nextYearID != nil {
			rec.NextFiscalYearID = types.FiscalYearID(*nextYearID)
		}
		if openingID != nil {
			rec.OpeningEntryID = types.JournalEntryID(*openingID)
		}
		if err := json.Unmarshal(idsJSON, &rec.ClosingEntryIDs); err != nil {
			return err
		}
		return nil
	})
	return rec, found, err
}
