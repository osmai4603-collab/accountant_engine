package posting

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/osmai4603-collab/accounting_engine/internal/core/masterdata"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/ledger"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/storage"
)

// PostingPipeline orchestrates the 4-step atomic posting process:
// 1. Pre-validation: mathematical double-entry balance (sum D == sum C)
// 2. Domain validation: company active, accounts active & postable (non-header), fiscal period open
// 3. Idempotency reservation (if idempotency key supplied)
// 4. Atomic execution inside UnitOfWork: sequence allocation, commit, audit log, idempotency completion
type PostingPipeline struct {
	uowRunner storage.UnitOfWorkRunner
	reader    storage.MasterDataReader
}

func NewPostingPipeline(uowRunner storage.UnitOfWorkRunner, reader storage.MasterDataReader) *PostingPipeline {
	return &PostingPipeline{
		uowRunner: uowRunner,
		reader:    reader,
	}
}

type PostOptions struct {
	ActorID        string
	Reason         string
	CorrelationID  string
	IdempotencyKey string
}

// PostEntry executes the 4-stage atomic posting pipeline.
func (p *PostingPipeline) PostEntry(ctx context.Context, entry ledger.JournalEntry, opts PostOptions) (ledger.JournalEntry, error) {
	// Stage 1: Mathematical double-entry pre-check
	if err := ValidateEntryDoubleEntry(&entry, nil); err != nil {
		return ledger.JournalEntry{}, err
	}

	// Stage 2: Domain validation (Company, Accounts, Fiscal Period)
	if _, err := masterdata.ValidateCompanyScope(ctx, p.reader, entry.CompanyID); err != nil {
		return ledger.JournalEntry{}, err
	}

	period, err := masterdata.ValidatePeriodForPosting(ctx, p.reader, entry.CompanyID, entry.EffectiveDate)
	if err != nil {
		return ledger.JournalEntry{}, err
	}
	entry.FiscalPeriodID = period.ID

	for _, line := range entry.Lines {
		if err := masterdata.ValidateCanPostToAccount(ctx, p.reader, entry.CompanyID, line.AccountID); err != nil {
			return ledger.JournalEntry{}, err
		}
	}

	// Stage 3 & 4: Atomic Execution in Unit of Work
	var postedEntry ledger.JournalEntry
	now := time.Now().UTC()
	entry.PostingDate = now
	entry.PostedBy = opts.ActorID
	entry.PostedAt = &now
	entry.Status = ledger.EntryPosted

	err = p.uowRunner.Execute(ctx, func(txCtx context.Context, uow storage.UnitOfWork) error {
		// Handle Idempotency claim if key present
		var payloadHash string
		if opts.IdempotencyKey != "" && uow.Idempotency() != nil {
			payloadHash = fmt.Sprintf("%s:%s", entry.CompanyID.String(), entry.ID.String())
			record := storage.IdempotencyRecord{
				CompanyID:   entry.CompanyID,
				Key:         opts.IdempotencyKey,
				Operation:   string(storage.OperationPost),
				PayloadHash: payloadHash,
				Status:      storage.IdempotencyStarted,
				CreatedAt:   now,
				UpdatedAt:   now,
			}
			existing, claimed, err := uow.Idempotency().Claim(txCtx, record)
			if err != nil {
				return err
			}
			if !claimed {
				if existing.Status == storage.IdempotencyCompleted && len(existing.Response) > 0 {
					return json.Unmarshal(existing.Response, &postedEntry)
				}
				return fmt.Errorf("idempotency conflict for key %s", opts.IdempotencyKey)
			}
		}

		// Sequence allocation
		seq, err := uow.Sequences().NextSequence(txCtx, entry.CompanyID, entry.JournalID)
		if err != nil {
			return err
		}
		entry.SequenceNumber = seq
		if entry.EntryNumber == "" {
			entry.EntryNumber = fmt.Sprintf("JE-%d", seq)
		}

		// Commit posting
		commit := storage.PostingCommit{
			Entry:         entry,
			Operation:     storage.OperationPost,
			ActorID:       opts.ActorID,
			Reason:        opts.Reason,
			CorrelationID: opts.CorrelationID,
		}
		res, err := uow.Posting().CommitPosting(txCtx, commit)
		if err != nil {
			return err
		}
		postedEntry = res

		// Append Audit record
		if uow.Audit() != nil {
			payloadBytes, _ := json.Marshal(postedEntry)
			auditRec := storage.AuditRecord{
				CompanyID:     entry.CompanyID,
				EntityType:    "journal_entry",
				EntityID:      postedEntry.ID.String(),
				Action:        "POST",
				ActorID:       opts.ActorID,
				Reason:        opts.Reason,
				CorrelationID: opts.CorrelationID,
				Payload:       payloadBytes,
				OccurredAt:    now,
			}
			if err := uow.Audit().AppendAudit(txCtx, auditRec); err != nil {
				return err
			}
		}

		// Complete Idempotency
		if opts.IdempotencyKey != "" && uow.Idempotency() != nil {
			respBytes, _ := json.Marshal(postedEntry)
			if err := uow.Idempotency().Complete(txCtx, entry.CompanyID, opts.IdempotencyKey, payloadHash, postedEntry.ID, respBytes); err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		return ledger.JournalEntry{}, err
	}

	return postedEntry, nil
}
