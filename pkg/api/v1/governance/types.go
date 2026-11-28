// Package governance re-exports the kernel audit evidence contracts at the
// public API boundary.
package governance

import kernelgovernance "github.com/osmai4603-collab/accounting_engine/internal/kernel/governance"

type AuditLog = kernelgovernance.AuditLog
type AuditCheckpoint = kernelgovernance.AuditCheckpoint
type AuditVerificationSnapshot = kernelgovernance.AuditVerificationSnapshot
type AuditChain = kernelgovernance.AuditChain
type AuditLogRepository = kernelgovernance.AuditLogRepository
type AtomicAuditLogRepository = kernelgovernance.AtomicAuditLogRepository
type AuditVerificationSnapshotRepository = kernelgovernance.AuditVerificationSnapshotRepository

const GenesisHash = kernelgovernance.GenesisHash

var (
	NewAuditChain      = kernelgovernance.NewAuditChain
	CalculateHash      = kernelgovernance.CalculateHash
	ValidateCheckpoint = kernelgovernance.ValidateCheckpoint

	ErrAuditChainRequired                = kernelgovernance.ErrAuditChainRequired
	ErrAuditRepositoryRequired           = kernelgovernance.ErrAuditRepositoryRequired
	ErrAuditContextRequired              = kernelgovernance.ErrAuditContextRequired
	ErrInvalidAuditCompany               = kernelgovernance.ErrInvalidAuditCompany
	ErrAuditChainBroken                  = kernelgovernance.ErrAuditChainBroken
	ErrAtomicAuditRepositoryRequired     = kernelgovernance.ErrAtomicAuditRepositoryRequired
	ErrAuditSnapshotRepositoryRequired   = kernelgovernance.ErrAuditSnapshotRepositoryRequired
	ErrAuditSnapshotConflict             = kernelgovernance.ErrAuditSnapshotConflict
	ErrAuditSnapshotConsistencyRetryable = kernelgovernance.ErrAuditSnapshotConsistencyRetryable
	ErrInvalidAuditSnapshot              = kernelgovernance.ErrInvalidAuditSnapshot
	ErrInvalidAuditCheckpoint            = kernelgovernance.ErrInvalidAuditCheckpoint
	ErrAuditCheckpointMismatch           = kernelgovernance.ErrAuditCheckpointMismatch
)
