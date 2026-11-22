package storage

import (
	"context"

	publicgovernance "github.com/osmai4603-collab/accounting_engine/pkg/api/v1/governance"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
)

type GovernanceStore interface {
	RecordAuditLog(ctx context.Context, eventID string, details map[string]any) error
	GetApprovalFingerprint(ctx context.Context, entryID types.JournalEntryID) (string, error)
}

// AuditConformanceRepository is the adapter-neutral audit evidence contract
// used by conformance tests. It carries only public governance types.
type AuditConformanceRepository interface {
	AppendAuditLogAtomic(ctx context.Context, log publicgovernance.AuditLog) (publicgovernance.AuditLog, error)
	GetLastAuditLogForCompany(ctx context.Context, companyID types.CompanyID) (*publicgovernance.AuditLog, error)
	IterateAuditLogs(ctx context.Context, companyID types.CompanyID, fn func(publicgovernance.AuditLog) error) error
	GetAuditVerificationSnapshot(ctx context.Context, companyID types.CompanyID) (publicgovernance.AuditVerificationSnapshot, error)
	SetExternalAuditCheckpoint(ctx context.Context, companyID types.CompanyID, checkpoint publicgovernance.AuditCheckpoint) error
}

// AuditConformanceExtensionKey exposes an optional audit repository through
// Ports.Private. Stable storage.Ports contracts remain unchanged.
const AuditConformanceExtensionKey = "accounting_engine.audit_conformance"

const StorageContractVersion = "storage.spi/v1"
