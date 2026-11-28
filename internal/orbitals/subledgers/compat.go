package subledgers

import (
	"errors"

	"github.com/google/uuid"
	financialtypes "github.com/osmai4603-collab/accounting_engine/internal/kernel/types"
	posting "github.com/osmai4603-collab/accounting_engine/internal/orbitals/postingcompat"
	"github.com/shopspring/decimal"
)

// subledgerSubClassBank preserves the bank classification token used by the
// source ledger even though the kernel masterdata package renamed it.
const subledgerSubClassBank = "bank"

// reconciliationDaemonActorID is the deterministic system actor used when the
// reconciliation engine raises an exception.
func reconciliationDaemonActorID() uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte("SYSTEM_DAEMON"))
}

var errSubledgerInvalidRate = errors.New("invalid exchange rate")

// subledgerRealizedFXDifference reproduces the valuation helper used during
// cash-settlement reconciliation without depending on a non-kernel package.
func subledgerRealizedFXDifference(
	foreignAmount, originalRate, settlementRate decimal.Decimal,
	functionalCurrency string,
) (financialtypes.Money, error) {
	if originalRate.IsZero() || settlementRate.IsZero() ||
		originalRate.IsNegative() || settlementRate.IsNegative() {
		return financialtypes.Money{}, errSubledgerInvalidRate
	}
	return financialtypes.NewMoney(foreignAmount.Mul(settlementRate.Sub(originalRate)), functionalCurrency)
}

// cloneOpenLine returns an independent copy of an open line.
func cloneOpenLine(line OpenLine) OpenLine {
	line.MatchIDs = append([]uuid.UUID(nil), line.MatchIDs...)
	line.Matches = append([]OpenLineMatch(nil), line.Matches...)
	line.MatchHistory = append([]OpenLineMatch(nil), line.MatchHistory...)
	return line
}

func cloneStringMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func cloneEventAccountingRule(rule EventAccountingRule) EventAccountingRule {
	rule.Lines = append([]EventLineTemplate(nil), rule.Lines...)
	rule.Conditions = cloneStringMap(rule.Conditions)
	rule.Accounts = make(map[string]uuid.UUID, len(rule.Accounts))
	for key, id := range rule.Accounts {
		rule.Accounts[key] = id
	}
	return rule
}

// Clone returns an independent copy of an event accounting rule.
func (r EventAccountingRule) Clone() EventAccountingRule { return cloneEventAccountingRule(r) }

// cloneFixedAsset returns an independent copy of a fixed asset.
func cloneFixedAsset(asset FixedAsset) FixedAsset {
	asset.AppliedEntryIDs = append([]uuid.UUID(nil), asset.AppliedEntryIDs...)
	asset.Components = append([]AssetComponent(nil), asset.Components...)
	if asset.DisposedAt != nil {
		disposedAt := *asset.DisposedAt
		asset.DisposedAt = &disposedAt
	}
	return asset
}

// FixedAssetEntryMatches reports whether a posted journal entry matches the
// expected fixed-asset mutation payload.
func FixedAssetEntryMatches(actual, expected posting.JournalEntry) bool {
	if actual.ID != expected.ID || actual.CompanyID != expected.CompanyID ||
		actual.Status != posting.EntryPosted || expected.Status != posting.EntryPosted ||
		actual.DocumentReference != expected.DocumentReference ||
		!actual.EntryDate.Equal(expected.EntryDate) || !actual.Date.Equal(expected.Date) ||
		len(actual.Lines) != len(expected.Lines) {
		return false
	}
	for i, actualLine := range actual.Lines {
		expectedLine := expected.Lines[i]
		if actualLine.CompanyID != expectedLine.CompanyID || actualLine.AccountID != expectedLine.AccountID ||
			!actualLine.Debit.Equal(expectedLine.Debit) || !actualLine.Credit.Equal(expectedLine.Credit) {
			return false
		}
	}
	return true
}
