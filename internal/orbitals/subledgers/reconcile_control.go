package subledgers

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	apisubledger "github.com/osmai4603-collab/accounting_engine/pkg/api/v1/subledger"
	apitypes "github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/storage"
	"github.com/shopspring/decimal"
)

// Additional parity-only line kinds used to address the fixed-asset and
// treasury control ledgers. AR/AP kinds are declared in models.go.
const (
	LineFixedAsset LineKind = "fixed_asset"
	LineBank       LineKind = "bank"
)

var (
	// ErrSubledgerParityMismatch is returned when the subledger total and the
	// general-ledger control account balance differ by any non-zero amount.
	ErrSubledgerParityMismatch = errors.New("subledger control account parity mismatch")
	// ErrSubledgerParityUnsupported indicates an unknown subledger kind.
	ErrSubledgerParityUnsupported = errors.New("unsupported subledger parity kind")
)

// ParityRepository is the read-only subledger surface required for a parity
// assertion. It is deliberately narrower than Repository so inquiry-only
// adapters (and tests) can provide just the snapshots they hold.
type ParityRepository interface {
	ListOpenLinesForCompanyContext(context.Context, uuid.UUID, string) ([]OpenLine, error)
	ListFixedAssetsForCompanyContext(context.Context, uuid.UUID) ([]FixedAsset, error)
	ListBankItemsForCompany(context.Context, uuid.UUID, string) ([]BankItem, error)
}

// AssertSubledgerParity compares the summed balance of one subledger kind
// against the corresponding control-account balance in the general ledger.
// The GL is read exclusively through the stable storage.BalanceReader port;
// no concrete storage adapter is imported.
//
// A non-zero difference always returns ErrSubledgerParityMismatch together
// with the full ReconciliationEvidence, so callers can both fail fast and
// inspect the exact drift.
func AssertSubledgerParity(
	ctx context.Context,
	repo ParityRepository,
	balances storage.BalanceReader,
	companyID, controlAccountID uuid.UUID,
	kind LineKind,
	currency string,
	asOf time.Time,
) (apisubledger.ReconciliationEvidence, error) {
	evidence := apisubledger.ReconciliationEvidence{
		CompanyID:        apitypes.CompanyID(companyID),
		Kind:             subledgerType(kind),
		ControlAccountID: apitypes.AccountID(controlAccountID),
		Currency:         currency,
		AsOf:             asOf,
		EvidenceAt:       time.Now().UTC(),
	}
	if ctx == nil {
		return evidence, errors.New("parity context is required")
	}
	if err := ctx.Err(); err != nil {
		return evidence, err
	}
	if companyID == uuid.Nil || controlAccountID == uuid.Nil || strings.TrimSpace(currency) == "" {
		return evidence, ErrSubledgerParityUnsupported
	}
	if balances == nil || repo == nil {
		return evidence, ErrSubledgerParityUnsupported
	}
	if asOf.IsZero() {
		asOf = evidence.EvidenceAt
		evidence.AsOf = asOf
	}

	total, err := subledgerParityTotal(ctx, repo, companyID, controlAccountID, kind, currency, asOf)
	if err != nil {
		return evidence, err
	}
	gl, err := balances.GetAccountBalance(
		ctx,
		apitypes.CompanyID(companyID),
		apitypes.AccountID(controlAccountID),
		asOf,
		currency,
	)
	if err != nil {
		return evidence, err
	}
	if kind == LinePayable {
		// BalanceReader reports debit-minus-credit; AP is credit-normal.
		gl = gl.Neg()
	}

	evidence.SubledgerBalance = total
	evidence.GLBalance = gl
	evidence.Difference = total.Sub(gl)
	evidence.Reconciled = evidence.Difference.IsZero()
	if !evidence.Reconciled {
		return evidence, fmt.Errorf("%w: kind=%s subledger=%s gl=%s difference=%s",
			ErrSubledgerParityMismatch, kind, total, gl, evidence.Difference)
	}
	return evidence, nil
}

func subledgerParityTotal(
	ctx context.Context,
	repo ParityRepository,
	companyID, controlAccountID uuid.UUID,
	kind LineKind,
	currency string,
	asOf time.Time,
) (decimal.Decimal, error) {
	total := decimal.Zero
	switch kind {
	case LineReceivable, LinePayable:
		lines, err := repo.ListOpenLinesForCompanyContext(ctx, companyID, "")
		if err != nil {
			return decimal.Zero, err
		}
		for _, line := range lines {
			if line.CompanyID != companyID || line.ControlAccountID != controlAccountID || line.Kind != kind {
				continue
			}
			if line.Closed {
				continue
			}
			amount, ok := parityAmount(line.FunctionalResidualAmount, line.ResidualAmount, currency, asOf, line.PostingDate)
			if !ok {
				continue
			}
			total = total.Add(amount)
		}
	case LineFixedAsset:
		assets, err := repo.ListFixedAssetsForCompanyContext(ctx, companyID)
		if err != nil {
			return decimal.Zero, err
		}
		for _, asset := range assets {
			if asset.CompanyID != companyID || asset.AssetAccountID != controlAccountID || asset.DisposedAt != nil {
				continue
			}
			if asset.CurrentBookValue.Currency() != currency {
				return decimal.Zero, ErrSubledgerParityUnsupported
			}
			total = total.Add(asset.CurrentBookValue.Amount())
		}
	case LineBank:
		items, err := repo.ListBankItemsForCompany(ctx, companyID, "")
		if err != nil {
			return decimal.Zero, err
		}
		for _, item := range items {
			if item.CompanyID != companyID || item.AccountID != controlAccountID {
				continue
			}
			if item.Status == BankItemMatched {
				continue
			}
			if item.Date.After(asOf) {
				continue
			}
			if item.Amount.Currency() != currency {
				return decimal.Zero, ErrSubledgerParityUnsupported
			}
			total = total.Add(item.Amount.Amount())
		}
	default:
		return decimal.Zero, ErrSubledgerParityUnsupported
	}
	return total, nil
}

func parityAmount(
	functional, residual interface {
		Currency() string
		Amount() decimal.Decimal
	},
	currency string,
	asOf, postingDate time.Time,
) (decimal.Decimal, bool) {
	if !postingDate.IsZero() && postingDate.After(asOf) {
		return decimal.Zero, false
	}
	if functional.Currency() != "" {
		if functional.Currency() != currency {
			return decimal.Zero, false
		}
		return functional.Amount(), true
	}
	if residual.Currency() != currency {
		return decimal.Zero, false
	}
	return residual.Amount(), true
}

func subledgerType(kind LineKind) apisubledger.SubledgerType {
	switch kind {
	case LineReceivable:
		return apisubledger.SubledgerReceivable
	case LinePayable:
		return apisubledger.SubledgerPayable
	case LineFixedAsset:
		return apisubledger.SubledgerFixedAsset
	case LineBank:
		return apisubledger.SubledgerTreasury
	default:
		return ""
	}
}
