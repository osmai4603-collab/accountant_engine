package posting

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/osmai4603-collab/accounting_engine/internal/kernal/governance"
	"github.com/osmai4603-collab/accounting_engine/internal/kernal/journal"
	"github.com/osmai4603-collab/accounting_engine/internal/kernal/masterdata"
	"github.com/shopspring/decimal"
)

type PostOptions struct {
	ActorID        string
	Reason         string
	CorrelationID  string
	IdempotencyKey string
}

type PostingExecutor func(context.Context, journal.JournalEntry, PostOptions) (journal.JournalEntry, error)

type PipelineOption func(*PostingPipeline)

func WithApprovalGate(gate governance.ApprovalGate) PipelineOption {
	return func(p *PostingPipeline) {
		p.approvalGate = gate
	}
}

func WithMakerCheckerThreshold(threshold decimal.Decimal) PipelineOption {
	return func(p *PostingPipeline) {
		p.makerCheckerThreshold = threshold
	}
}

type PostingPipeline struct {
	reader                masterdata.Reader
	execute               PostingExecutor
	approvalGate          governance.ApprovalGate
	makerCheckerThreshold decimal.Decimal
}

func NewPostingPipeline(reader masterdata.Reader, execute PostingExecutor, opts ...PipelineOption) *PostingPipeline {
	p := &PostingPipeline{reader: reader, execute: execute}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// PostEntry performs pure kernel validation and delegates persistence to the host.
func (p *PostingPipeline) PostEntry(ctx context.Context, entry journal.JournalEntry, opts PostOptions) (journal.JournalEntry, error) {
	if entry.Status != journal.EntryDraft {
		return journal.JournalEntry{}, fmt.Errorf("%w: entry must be in draft status", ErrInvalidEntryState)
	}
	if strings.TrimSpace(opts.ActorID) == "" {
		return journal.JournalEntry{}, ErrPosterIDRequired
	}
	if err := EnsureImmutable(entry.Status); err != nil {
		return journal.JournalEntry{}, err
	}
	if err := ValidateEntryDoubleEntry(&entry, nil); err != nil {
		return journal.JournalEntry{}, err
	}

	company, err := masterdata.ValidateCompanyScope(ctx, p.reader, entry.CompanyID)
	if err != nil {
		return journal.JournalEntry{}, err
	}
	if company.FunctionalCurrency != "" {
		for i, line := range entry.Lines {
			if line.FunctionalDebit.Currency() != company.FunctionalCurrency ||
				line.FunctionalCredit.Currency() != company.FunctionalCurrency ||
				line.TransactionAmount.Currency() != line.TransactionCurrency {
				return journal.JournalEntry{}, fmt.Errorf("%w: line %d", ErrFunctionalCurrencyMismatch, i)
			}
		}
	}

	// Check Maker-Checker and Segregation of Duties if threshold is set
	if !p.makerCheckerThreshold.IsZero() {
		total := decimal.Zero
		for _, line := range entry.Lines {
			total = total.Add(line.FunctionalDebit.Amount())
		}
		if total.GreaterThanOrEqual(p.makerCheckerThreshold) {
			if entry.CreatedBy != "" && entry.CreatedBy == opts.ActorID {
				return journal.JournalEntry{}, governance.ErrSegregationOfDuties
			}
			if p.approvalGate != nil {
				hash, err := EntryContentHash(entry)
				if err != nil {
					return journal.JournalEntry{}, err
				}
				approved, err := p.approvalGate.IsApproved(ctx, entry.CompanyID, entry.ID, hash)
				if err != nil {
					return journal.JournalEntry{}, err
				}
				if !approved {
					return journal.JournalEntry{}, governance.ErrApprovalRequired
				}
			}
		}
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
