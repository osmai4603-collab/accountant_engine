package posting

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/osmai4603-collab/accounting_engine/internal/kernal/journal"
)

var ErrEntryHashUnavailable = errors.New("journal entry cannot be hashed")

func EntryContentHash(entry journal.JournalEntry) (string, error) {
	if entry.ID.String() == "" || entry.CompanyID.String() == "" || entry.JournalID.String() == "" ||
		entry.EffectiveDate.IsZero() {
		return "", ErrEntryHashUnavailable
	}

	payload := struct {
		EntryID        string              `json:"entry_id"`
		CompanyID      string              `json:"company_id"`
		JournalID      string              `json:"journal_id"`
		PeriodID       string              `json:"period_id"`
		EffectiveDate  string              `json:"effective_date"`
		CreatedBy      string              `json:"created_by"`
		Description    string              `json:"description"`
		SourceModule   string              `json:"source_module"`
		ReferenceID    string              `json:"reference_id"`
		Classification string              `json:"classification"`
		TargetPeriod   int                 `json:"target_period"`
		Lines          []journal.EntryLine `json:"lines"`
	}{
		EntryID:        entry.ID.String(),
		CompanyID:      entry.CompanyID.String(),
		JournalID:      entry.JournalID.String(),
		PeriodID:       entry.FiscalPeriodID.String(),
		EffectiveDate:  entry.EffectiveDate.UTC().Format(time.RFC3339Nano),
		CreatedBy:      entry.CreatedBy,
		Description:    entry.Description,
		SourceModule:   entry.SourceModule,
		ReferenceID:    entry.ReferenceID,
		Classification: string(entry.Classification),
		TargetPeriod:   entry.TargetPeriodNumber,
		Lines:          entry.Lines,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", errors.Join(ErrEntryHashUnavailable, err)
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}
