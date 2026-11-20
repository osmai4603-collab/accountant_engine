package memory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	"github.com/osmai4603-collab/accounting_engine/pkg/spi/storage"
)

func (d *Driver) AppendAudit(ctx context.Context, record storage.AuditRecord) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.readOnly {
		return ErrReadOnlyTransaction
	}
	d.appendAuditInternal(record)
	return nil
}

func (d *Driver) appendAuditInternal(record storage.AuditRecord) {
	// Chain hashes: SHA-256 of previousHash + record payload
	hasher := sha256.New()
	hasher.Write([]byte(d.auditLastHash))
	hasher.Write([]byte(record.CompanyID.String()))
	hasher.Write([]byte(record.EntityType))
	hasher.Write([]byte(record.EntityID))
	hasher.Write([]byte(record.Action))
	hasher.Write([]byte(record.ActorID))
	hasher.Write(record.Payload)
	d.auditLastHash = hex.EncodeToString(hasher.Sum(nil))

	d.auditRecords = append(d.auditRecords, record)
}

// GetAuditRecords returns a copy of stored audit records for testing.
func (d *Driver) GetAuditRecords() []storage.AuditRecord {
	d.mu.RLock()
	defer d.mu.RUnlock()
	res := make([]storage.AuditRecord, len(d.auditRecords))
	copy(res, d.auditRecords)
	return res
}
