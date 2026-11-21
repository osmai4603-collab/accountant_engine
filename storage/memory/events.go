package memory

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/journal"
	publicevents "github.com/osmai4603-collab/accounting_engine/pkg/spi/events"
)

var (
	_ publicevents.IdempotencyStore       = (*Driver)(nil)
	_ publicevents.ScopedIdempotencyStore = (*Driver)(nil)
	_ publicevents.MappingStore           = (*Driver)(nil)
)

func (d *Driver) GetPublicEventIngestion(ctx context.Context, companyID uuid.UUID, idempotencyKey string) (publicevents.Ingestion, bool, error) {
	return d.GetPublicEventIngestionForSource(ctx, companyID, publicevents.DefaultSourceSystem, idempotencyKey)
}

func (d *Driver) GetPublicEventIngestionForSource(ctx context.Context, companyID uuid.UUID, sourceSystem, idempotencyKey string) (publicevents.Ingestion, bool, error) {
	if err := ctx.Err(); err != nil {
		return publicevents.Ingestion{}, false, err
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	key := publicevents.CompositeIdempotencyKey(companyID, sourceSystem, idempotencyKey)
	rec, ok := d.eventIngestions[key]
	if !ok {
		return publicevents.Ingestion{}, false, nil
	}
	return rec, true, nil
}

func (d *Driver) ClaimPublicEventIngestion(ctx context.Context, record publicevents.Ingestion) (publicevents.Ingestion, bool, error) {
	if err := ctx.Err(); err != nil {
		return publicevents.Ingestion{}, false, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.readOnly {
		return publicevents.Ingestion{}, false, ErrReadOnlyTransaction
	}
	key := publicevents.CompositeIdempotencyKey(record.CompanyID, record.SourceSystem, record.IdempotencyKey)
	if existing, exists := d.eventIngestions[key]; exists {
		if publicevents.IsRetryableFailure(existing.Status) {
			record.Status = publicevents.StatusStarted
			d.eventIngestions[key] = record
			return record, true, nil
		}
		return existing, false, nil
	}
	record.Status = publicevents.StatusStarted
	d.eventIngestions[key] = record
	return record, true, nil
}

func (d *Driver) CompletePublicEventIngestion(ctx context.Context, companyID uuid.UUID, idempotencyKey string, entry journal.JournalEntry) error {
	return d.CompletePublicEventIngestionForSource(ctx, companyID, publicevents.DefaultSourceSystem, idempotencyKey, "", entry)
}

func (d *Driver) CompletePublicEventIngestionForSource(ctx context.Context, companyID uuid.UUID, sourceSystem, idempotencyKey, payloadHash string, entry journal.JournalEntry) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.readOnly {
		return ErrReadOnlyTransaction
	}
	key := publicevents.CompositeIdempotencyKey(companyID, sourceSystem, idempotencyKey)
	rec, ok := d.eventIngestions[key]
	if !ok {
		return publicevents.ErrRecordNotFound
	}
	rec.Status = publicevents.StatusCompleted
	rec.EntryID = uuid.UUID(entry.ID)
	rec.Entry = cloneEntry(entry)
	d.eventIngestions[key] = rec
	return nil
}

func (d *Driver) FailPublicEventIngestion(ctx context.Context, companyID uuid.UUID, idempotencyKey, reason string) error {
	return d.FailPublicEventIngestionForSource(ctx, companyID, publicevents.DefaultSourceSystem, idempotencyKey, publicevents.FailureClassTransient, reason)
}

func (d *Driver) FailPublicEventIngestionForSource(ctx context.Context, companyID uuid.UUID, sourceSystem, idempotencyKey, failureClass, reason string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.readOnly {
		return ErrReadOnlyTransaction
	}
	key := publicevents.CompositeIdempotencyKey(companyID, sourceSystem, idempotencyKey)
	rec, ok := d.eventIngestions[key]
	if !ok {
		return publicevents.ErrRecordNotFound
	}
	if failureClass == publicevents.FailureClassTerminal {
		rec.Status = publicevents.StatusTerminalFailure
	} else {
		rec.Status = publicevents.StatusRetryableFailure
	}
	rec.FailureClass = failureClass
	rec.FailureReason = reason
	d.eventIngestions[key] = rec
	return nil
}

func (d *Driver) GetPublicEventAccountMapping(ctx context.Context, companyID uuid.UUID, eventType, ledgerID string) (publicevents.AccountMapping, bool, error) {
	if err := ctx.Err(); err != nil {
		return publicevents.AccountMapping{}, false, err
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	key := fmt.Sprintf("%s:%s:%s", companyID.String(), strings.TrimSpace(eventType), strings.TrimSpace(ledgerID))
	m, ok := d.eventMappings[key]
	return m, ok, nil
}

func (d *Driver) SavePublicEventAccountMapping(ctx context.Context, mapping publicevents.AccountMapping) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.readOnly {
		return ErrReadOnlyTransaction
	}
	key := fmt.Sprintf("%s:%s:%s", mapping.CompanyID.String(), strings.TrimSpace(mapping.EventType), strings.TrimSpace(mapping.LedgerID))
	d.eventMappings[key] = mapping
	return nil
}
