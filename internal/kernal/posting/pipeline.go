package posting

import (
	"context"
	"fmt"
	"time"

	"github.com/osmai4603-collab/accounting_engine/internal/kernal/journal"
	"github.com/osmai4603-collab/accounting_engine/internal/kernal/masterdata"
)

type PostOptions struct {
	ActorID        string
	Reason         string
	CorrelationID  string
	IdempotencyKey string
}

type PostingExecutor func(context.Context, journal.JournalEntry, PostOptions) (journal.JournalEntry, error)

type PostingPipeline struct {
	reader  masterdata.Reader
	execute PostingExecutor
}

func NewPostingPipeline(reader masterdata.Reader, execute PostingExecutor) *PostingPipeline {
	return &PostingPipeline{reader: reader, execute: execute}
}

// PostEntry performs pure kernel validation and delegates persistence to the host.
func (p *PostingPipeline) PostEntry(ctx context.Context, entry journal.JournalEntry, opts PostOptions) (journal.JournalEntry, error) {
	if err := ValidateEntryDoubleEntry(&entry, nil); err != nil {
		return journal.JournalEntry{}, err
	}

	if _, err := masterdata.ValidateCompanyScope(ctx, p.reader, entry.CompanyID); err != nil {
		return journal.JournalEntry{}, err
	}

	period, err := masterdata.ValidatePeriodForPosting(ctx, p.reader, entry.CompanyID, entry.EffectiveDate)
	if err != nil {
		return journal.JournalEntry{}, err
	}
	entry.FiscalPeriodID = period.ID

	for _, line := range entry.Lines {
		if err := masterdata.ValidateCanPostToAccount(ctx, p.reader, entry.CompanyID, line.AccountID); err != nil {
			return journal.JournalEntry{}, err
		}
	}

	now := time.Now().UTC()
	entry.PostingDate = now
	entry.PostedBy = opts.ActorID
	entry.PostedAt = &now
	entry.Status = journal.EntryPosted

	if p.execute == nil {
		return journal.JournalEntry{}, fmt.Errorf("posting executor is required")
	}
	return p.execute(ctx, entry, opts)
}
