// Package governance re-exports the kernel audit evidence contracts at the
// public API boundary.
package governance

import kernalgovernance "github.com/osmai4603-collab/accounting_engine/internal/kernal/governance"

type AuditLog = kernalgovernance.AuditLog
type AuditCheckpoint = kernalgovernance.AuditCheckpoint
type AuditVerificationSnapshot = kernalgovernance.AuditVerificationSnapshot
type AuditChain = kernalgovernance.AuditChain
type AuditLogRepository = kernalgovernance.AuditLogRepository
type AtomicAuditLogRepository = kernalgovernance.AtomicAuditLogRepository
type AuditVerificationSnapshotRepository = kernalgovernance.AuditVerificationSnapshotRepository

const GenesisHash = kernalgovernance.GenesisHash

var (
	NewAuditChain      = kernalgovernance.NewAuditChain
	CalculateHash      = kernalgovernance.CalculateHash
	ValidateCheckpoint = kernalgovernance.ValidateCheckpoint

	ErrAuditChainRequired                = kernalgovernance.ErrAuditChainRequired
	ErrAuditRepositoryRequired           = kernalgovernance.ErrAuditRepositoryRequired
	ErrAuditContextRequired              = kernalgovernance.ErrAuditContextRequired
	ErrInvalidAuditCompany               = kernalgovernance.ErrInvalidAuditCompany
	ErrAuditChainBroken                  = kernalgovernance.ErrAuditChainBroken
	ErrAtomicAuditRepositoryRequired     = kernalgovernance.ErrAtomicAuditRepositoryRequired
	ErrAuditSnapshotRepositoryRequired   = kernalgovernance.ErrAuditSnapshotRepositoryRequired
	ErrAuditSnapshotConflict             = kernalgovernance.ErrAuditSnapshotConflict
	ErrAuditSnapshotConsistencyRetryable = kernalgovernance.ErrAuditSnapshotConsistencyRetryable
	ErrInvalidAuditSnapshot              = kernalgovernance.ErrInvalidAuditSnapshot
	ErrInvalidAuditCheckpoint            = kernalgovernance.ErrInvalidAuditCheckpoint
	ErrAuditCheckpointMismatch           = kernalgovernance.ErrAuditCheckpointMismatch
)
