package memory

import (
	"context"
	"fmt"
	"time"

	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/storage"
)

func (d *Driver) Claim(ctx context.Context, record storage.IdempotencyRecord) (existing storage.IdempotencyRecord, claimed bool, err error) {
	if err := ctx.Err(); err != nil {
		return storage.IdempotencyRecord{}, false, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.readOnly {
		return storage.IdempotencyRecord{}, false, ErrReadOnlyTransaction
	}

	key := idempotencyKey{companyID: record.CompanyID, key: record.Key}
	if ex, ok := d.idempotency[key]; ok {
		return ex, false, nil
	}

	record.CreatedAt = time.Now().UTC()
	record.UpdatedAt = record.CreatedAt
	record.Status = storage.IdempotencyStarted
	d.idempotency[key] = record
	return record, true, nil
}

func (d *Driver) Complete(ctx context.Context, companyID types.CompanyID, key, payloadHash string, entryID types.JournalEntryID, response []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.readOnly {
		return ErrReadOnlyTransaction
	}

	idKey := idempotencyKey{companyID: companyID, key: key}
	rec, ok := d.idempotency[idKey]
	if !ok {
		return fmt.Errorf("memory: idempotency key not found")
	}

	rec.Status = storage.IdempotencyCompleted
	rec.EntryID = &entryID
	rec.Response = response
	rec.UpdatedAt = time.Now().UTC()
	d.idempotency[idKey] = rec
	return nil
}

func (d *Driver) Fail(ctx context.Context, companyID types.CompanyID, key, payloadHash, status string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.readOnly {
		return ErrReadOnlyTransaction
	}

	idKey := idempotencyKey{companyID: companyID, key: key}
	rec, ok := d.idempotency[idKey]
	if !ok {
		return fmt.Errorf("memory: idempotency key not found")
	}

	rec.Status = status
	rec.UpdatedAt = time.Now().UTC()
	d.idempotency[idKey] = rec
	return nil
}

