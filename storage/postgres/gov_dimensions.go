package postgres

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/masterdata"
)

var _ masterdata.CostCenterReader = (*Driver)(nil)
var _ masterdata.CostCenterWriter = (*Driver)(nil)
var _ masterdata.AnalyticalDimensionReader = (*Driver)(nil)
var _ masterdata.AnalyticalDimensionWriter = (*Driver)(nil)

// Company cost centers (G-022). All reads and writes run in the tenant
// transaction established by WithCompanyTx and additionally reject a company
// that is not the active tenant of the caller's context.
func (d *Driver) CreateCostCenter(ctx context.Context, center masterdata.CostCenter) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := center.Validate(); err != nil {
		return masterdata.ErrInvalidCostCenter
	}
	var parentID *uuid.UUID
	if center.ParentID != nil {
		value := uuid.UUID(*center.ParentID)
		parentID = &value
	}
	return d.WithCompanyTx(ctx, uuid.UUID(center.CompanyID), func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO cost_centers(id, company_id, parent_id, code, name, is_active)
			VALUES($1,$2,$3,$4,$5,$6)`,
			uuid.UUID(center.ID), uuid.UUID(center.CompanyID), parentID,
			center.Code, center.Name, center.Active)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				return masterdata.ErrDuplicateCostCenter
			}
			return err
		}
		return nil
	})
}

func (d *Driver) GetCostCenterForCompany(ctx context.Context, companyID types.CompanyID, costCenterID types.CostCenterID) (masterdata.CostCenter, error) {
	if err := ctx.Err(); err != nil {
		return masterdata.CostCenter{}, err
	}
	if err := d.assertReadScope(ctx, companyID, masterdata.ErrInvalidCostCenter); err != nil {
		return masterdata.CostCenter{}, err
	}
	var center masterdata.CostCenter
	err := d.WithCompanyTx(ctx, uuid.UUID(companyID), func(tx pgx.Tx) error {
		var id, cid uuid.UUID
		var parentID *uuid.UUID
		err := tx.QueryRow(ctx, `SELECT id, company_id, parent_id, code, name, is_active
			FROM cost_centers WHERE company_id=$1 AND id=$2`,
			uuid.UUID(companyID), uuid.UUID(costCenterID)).Scan(
			&id, &cid, &parentID, &center.Code, &center.Name, &center.Active)
		if errors.Is(err, pgx.ErrNoRows) {
			return masterdata.ErrInvalidCostCenter
		}
		if err != nil {
			return err
		}
		center.ID = types.CostCenterID(id)
		center.CompanyID = types.CompanyID(cid)
		if parentID != nil {
			pid := types.CostCenterID(*parentID)
			center.ParentID = &pid
		}
		return nil
	})
	return center, err
}

// Analytical dimensions (G-022) use the existing company-scoped
// financial_dimension_values table and the same public DTO as the SPI.
func (d *Driver) CreateAnalyticalDimensionValue(ctx context.Context, value masterdata.AnalyticalDimensionValue) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := value.Validate(); err != nil {
		return masterdata.ErrInvalidAnalyticalDimension
	}
	return d.WithCompanyTx(ctx, uuid.UUID(value.CompanyID), func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO financial_dimension_values(company_id, kind, id, code, name, is_active)
			VALUES($1,$2,$3,$4,$5,$6)`,
			uuid.UUID(value.CompanyID), string(value.Kind), uuid.UUID(value.ID),
			value.Code, value.Name, value.Active)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				return masterdata.ErrDuplicateAnalyticalDimension
			}
			return err
		}
		return nil
	})
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
	if err := d.assertReadScope(ctx, companyID, masterdata.ErrInvalidAnalyticalDimension); err != nil {
		return masterdata.AnalyticalDimensionValue{}, err
	}
	parsed, err := uuid.Parse(strings.TrimSpace(id))
	if err != nil {
		return masterdata.AnalyticalDimensionValue{}, masterdata.ErrInvalidAnalyticalDimension
	}
	var value masterdata.AnalyticalDimensionValue
	err = d.WithCompanyTx(ctx, uuid.UUID(companyID), func(tx pgx.Tx) error {
		var cid, valueID uuid.UUID
		var valueKind string
		err := tx.QueryRow(ctx, `SELECT company_id, kind, id, code, name, is_active
			FROM financial_dimension_values WHERE company_id=$1 AND kind=$2 AND id=$3`,
			uuid.UUID(companyID), string(kind), parsed).Scan(
			&cid, &valueKind, &valueID, &value.Code, &value.Name, &value.Active)
		if errors.Is(err, pgx.ErrNoRows) {
			return masterdata.ErrInvalidAnalyticalDimension
		}
		if err != nil {
			return err
		}
		value.CompanyID = types.CompanyID(cid)
		value.Kind = masterdata.AnalyticalDimensionKind(valueKind)
		value.ID = types.ProjectID(valueID)
		if value.ID.String() != id || value.CompanyID != companyID || value.Kind != kind {
			return masterdata.ErrInvalidAnalyticalDimension
		}
		return nil
	})
	return value, err
}
