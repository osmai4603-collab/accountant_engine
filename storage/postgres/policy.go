package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/storage"
)

const policyVersionColumns = `company_id, policy_id, policy_kind, version, effective_from, effective_to,
	created_by, approved_by, created_at, payload`

func scanPolicyVersion(rows interface{ Scan(...any) error }) (storage.PolicyVersion, error) {
	var version storage.PolicyVersion
	var companyID, policyID uuid.UUID
	var kind string
	var createdAt time.Time
	var effectiveTo *time.Time
	var payload []byte
	if err := rows.Scan(&companyID, &policyID, &kind, &version.Version, &version.EffectiveFrom,
		&effectiveTo, &version.CreatedBy, &version.ApprovedBy, &createdAt, &payload); err != nil {
		return storage.PolicyVersion{}, err
	}
	version.CompanyID = types.CompanyID(companyID)
	version.PolicyID = policyID
	version.Kind = storage.PolicyKind(kind)
	version.EffectiveTo = effectiveTo
	version.CreatedAt = createdAt
	version.Payload = append([]byte(nil), payload...)
	return version, nil
}

func (d *Driver) AppendVersion(ctx context.Context, version storage.PolicyVersion) error {
	if err := version.Validate(); err != nil {
		return err
	}
	companyID := uuid.UUID(version.CompanyID)
	end := time.Date(9999, time.December, 31, 0, 0, 0, 0, time.UTC)
	if version.EffectiveTo != nil {
		end = *version.EffectiveTo
	}
	return d.WithCompanyTx(ctx, companyID, func(tx pgx.Tx) error {
		lockKey := string(version.Kind) + "|" + companyID.String()
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, lockKey); err != nil {
			return err
		}
		var overlaps bool
		// Half-open windows [from, to) overlap when the stored start precedes
		// the new end and the stored end follows the new start.
		if err := tx.QueryRow(ctx, `SELECT EXISTS (
			SELECT 1 FROM accounting_policy_versions
			WHERE company_id = $1 AND policy_kind = $2
			  AND effective_from < $4 AND (effective_to IS NULL OR effective_to > $3)
		)`, companyID, string(version.Kind), version.EffectiveFrom, end).Scan(&overlaps); err != nil {
			return err
		}
		if overlaps {
			return storage.ErrPolicyVersionConflict
		}
		_, err := tx.Exec(ctx, `INSERT INTO accounting_policy_versions (`+policyVersionColumns+`)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
			companyID, version.PolicyID, string(version.Kind), version.Version,
			version.EffectiveFrom.UTC(), nullTime(version.EffectiveTo), version.CreatedBy,
			version.ApprovedBy, version.CreatedAt.UTC(), string(version.Payload))
		if err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "duplicate key") || strings.Contains(err.Error(), "23505") {
				return storage.ErrPolicyVersionConflict
			}
			return fmt.Errorf("postgres: append policy version: %w", err)
		}
		return nil
	})
}

func (d *Driver) ListVersions(ctx context.Context, companyID types.CompanyID, kind storage.PolicyKind) ([]storage.PolicyVersion, error) {
	if companyID == (types.CompanyID{}) {
		return nil, errors.New("postgres: policy listing requires a company")
	}
	if kind != storage.PolicyKindPosting && kind != storage.PolicyKindCurrency {
		return nil, fmt.Errorf("postgres: unsupported policy kind %q", kind)
	}
	var versions []storage.PolicyVersion
	err := d.WithCompanyTx(ctx, uuid.UUID(companyID), func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+policyVersionColumns+`
			FROM accounting_policy_versions
			WHERE company_id = $1 AND policy_kind = $2
			ORDER BY effective_from ASC, version ASC`, uuid.UUID(companyID), string(kind))
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			version, err := scanPolicyVersion(rows)
			if err != nil {
				return err
			}
			versions = append(versions, version)
		}
		return rows.Err()
	})
	return versions, err
}

func nullTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC()
}
