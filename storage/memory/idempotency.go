package memory

import (
	"context"
	"fmt"
	"time"

	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/storage"
)

func idempotencyRecordKey(record storage.IdempotencyRecord) idempotencyKey {
	return idempotencyKey{
		companyID: record.CompanyID,
		key:       record.Key,
		scope:     record.Scope,
	}
}

func (d *Driver) Claim(ctx context.Context, record storage.IdempotencyRecord) (existing storage.IdempotencyRecord, claimed bool, err error) {
	if err := ctx.Err(); err != nil {
		return storage.IdempotencyRecord{}, false, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.readOnly {
		return storage.IdempotencyRecord{}, false, ErrReadOnlyTransaction
	}

	key := idempotencyRecordKey(record)
	if ex, ok := d.idempotency[key]; ok {
		if ex.PayloadHash != "" && record.PayloadHash != "" && ex.PayloadHash != record.PayloadHash {
			return storage.IdempotencyRecord{}, false, storage.ErrIdempotencyPayloadConflict
		}
		return ex, false, nil
	}

	record.CreatedAt = time.Now().UTC()
	record.UpdatedAt = record.CreatedAt
	record.Status = storage.IdempotencyStarted
	d.idempotency[key] = record
	return record, true, nil
}

// Lookup is a side-effect-free read used after an unknown outcome. It never
// creates a record, unlike Claim.
func (d *Driver) Lookup(ctx context.Context, companyID types.CompanyID, key string) (storage.IdempotencyRecord, bool, error) {
	if err := ctx.Err(); err != nil {
		return storage.IdempotencyRecord{}, false, err
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	for recordKey, record := range d.idempotency {
		if recordKey.companyID == companyID && recordKey.key == key {
			return record, true, nil
		}
	}
	return storage.IdempotencyRecord{}, false, nil
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

	idKey, rec, err := d.findIdempotency(companyID, key)
	if err != nil {
		return err
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

	idKey, rec, err := d.findIdempotency(companyID, key)
	if err != nil {
		return err
	}

	rec.Status = status
	rec.UpdatedAt = time.Now().UTC()
	d.idempotency[idKey] = rec
	return nil
}

// findIdempotency resolves a key without creating a record. The caller must
// hold d.mu for writing.
func (d *Driver) findIdempotency(companyID types.CompanyID, key string) (idempotencyKey, storage.IdempotencyRecord, error) {
	for recordKey, record := range d.idempotency {
		if recordKey.companyID == companyID && recordKey.key == key {
			return recordKey, record, nil
		}
	}
	return idempotencyKey{}, storage.IdempotencyRecord{}, fmt.Errorf("memory: idempotency key not found")
}
