// Package governance re-exports the kernel audit evidence contracts at the
// public API boundary.
package governance

import kernalgovernance "github.com/osmai4603-collab/accounting_engine/internal/kernal/governance"

type AuditLog = kernalgovernance.AuditLog
type AuditCheckpoint = kernalgovernance.AuditCheckpoint
type AuditVerificationSnapshot = kernalgovernance.AuditVerificationSnapshot

const GenesisHash = kernalgovernance.GenesisHash
