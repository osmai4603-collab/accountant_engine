package memory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"

	kernalgovernance "github.com/osmai4603-collab/accounting_engine/internal/kernal/governance"
	publicgovernance "github.com/osmai4603-collab/accounting_engine/pkg/api/v1/governance"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
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

func cloneAuditLog(log kernalgovernance.AuditLog) kernalgovernance.AuditLog {
	cloned := log
	cloned.Payload = append([]byte(nil), log.Payload...)
	return cloned
}

func (d *Driver) AppendAuditLogAtomic(ctx context.Context, log publicgovernance.AuditLog) (publicgovernance.AuditLog, error) {
	if err := ctx.Err(); err != nil {
		return kernalgovernance.AuditLog{}, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.appendAuditLogAtomicLocked(log)
}

// appendAuditLogAtomicLocked is the lock-holding core used both by the public
// port and by the posting commit path, which already owns d.mu.
func (d *Driver) appendAuditLogAtomicLocked(log kernalgovernance.AuditLog) (kernalgovernance.AuditLog, error) {
	if d.readOnly {
		return kernalgovernance.AuditLog{}, ErrReadOnlyTransaction
	}
	logs := d.governanceAuditLogs[log.CompanyID]
	log.ID = int64(len(logs) + 1)
	log.SequenceNumber = uint64(len(logs) + 1)
	log.PreviousLogHash = kernalgovernance.GenesisHash
	if len(logs) > 0 {
		log.PreviousLogHash = logs[len(logs)-1].CurrentLogHash
	}
	if log.Timestamp.IsZero() {
		log.Timestamp = time.Now().UTC()
	} else {
		log.Timestamp = log.Timestamp.UTC()
	}
	log.CurrentLogHash = kernalgovernance.CalculateHash(log)
	stored := cloneAuditLog(log)
	d.governanceAuditLogs[log.CompanyID] = append(logs, stored)
	return cloneAuditLog(stored), nil
}

func (d *Driver) AppendAuditLog(ctx context.Context, log publicgovernance.AuditLog) error {
	_, err := d.AppendAuditLogAtomic(ctx, log)
	return err
}

func (d *Driver) GetLastAuditLogForCompany(ctx context.Context, companyID types.CompanyID) (*publicgovernance.AuditLog, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	logs := d.governanceAuditLogs[companyID]
	if len(logs) == 0 {
		return nil, nil
	}
	last := cloneAuditLog(logs[len(logs)-1])
	return &last, nil
}

func (d *Driver) IterateAuditLogs(ctx context.Context, companyID types.CompanyID, fn func(publicgovernance.AuditLog) error) error {
	if fn == nil {
		return kernalgovernance.ErrInvalidAuditSnapshot
	}
	d.mu.RLock()
	logs := d.cloneAuditLogsLocked(companyID)
	d.mu.RUnlock()
	for _, log := range logs {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := fn(log); err != nil {
			return err
		}
	}
	return nil
}

func (d *Driver) cloneAuditLogsLocked(companyID types.CompanyID) []kernalgovernance.AuditLog {
	stored := d.governanceAuditLogs[companyID]
	cloned := make([]kernalgovernance.AuditLog, len(stored))
	for i, log := range stored {
		cloned[i] = cloneAuditLog(log)
	}
	return cloned
}

func (d *Driver) GetAuditVerificationSnapshot(ctx context.Context, companyID types.CompanyID) (publicgovernance.AuditVerificationSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return kernalgovernance.AuditVerificationSnapshot{}, err
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	logs := d.cloneAuditLogsLocked(companyID)
	checkpoint, ok := d.governanceCheckpoints[companyID]
	if !ok {
		checkpoint = kernalgovernance.AuditCheckpoint{
			CompanyID:      companyID,
			SequenceNumber: 0,
			Version:        0,
			LogHash:        kernalgovernance.GenesisHash,
		}
	}
	return kernalgovernance.AuditVerificationSnapshot{
		CompanyID:         companyID,
		SnapshotSequence:  uint64(len(logs)),
		Checkpoint:        checkpoint,
		CheckpointPresent: ok,
		Logs:              logs,
	}, nil
}

func (d *Driver) SetExternalAuditCheckpoint(ctx context.Context, companyID types.CompanyID, checkpoint publicgovernance.AuditCheckpoint) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.readOnly {
		return ErrReadOnlyTransaction
	}
	checkpoint.CompanyID = companyID
	if err := kernalgovernance.ValidateCheckpoint(checkpoint); err != nil {
		return err
	}
	if current, ok := d.governanceCheckpoints[companyID]; ok &&
		(checkpoint.Version <= current.Version || checkpoint.SequenceNumber < current.SequenceNumber) {
		return kernalgovernance.ErrInvalidAuditCheckpoint
	}
	d.governanceCheckpoints[companyID] = checkpoint
	return nil
}
