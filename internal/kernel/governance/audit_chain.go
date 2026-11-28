package governance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	apiTypes "github.com/osmai4603-collab/accounting_engine/internal/kernel/types"
)

const GenesisHash = "0000000000000000000000000000000000000000000000000000000000000000"

var (
	ErrAuditChainRequired                = errors.New("audit chain is required")
	ErrAuditRepositoryRequired           = errors.New("audit repository is required")
	ErrAuditContextRequired              = errors.New("audit context is required")
	ErrInvalidAuditCompany               = errors.New("audit company id is required")
	ErrAuditChainBroken                  = errors.New("audit chain integrity violation: tampering or broken link detected")
	ErrAtomicAuditRepositoryRequired     = errors.New("atomic audit repository is required")
	ErrAuditSnapshotRepositoryRequired   = errors.New("consistent audit snapshot repository is required")
	ErrAuditSnapshotConflict             = errors.New("audit verification snapshot changed while being read")
	ErrAuditSnapshotConsistencyRetryable = errors.New("audit verification snapshot could not be stabilized; retry")
	ErrInvalidAuditSnapshot              = errors.New("audit verification snapshot is invalid")
	ErrInvalidAuditCheckpoint            = errors.New("external audit checkpoint is invalid")
	ErrAuditCheckpointMismatch           = errors.New("audit chain does not match external checkpoint")
)

const maxAuditSnapshotAttempts = 3

// AuditLog represents a cryptographically hashed immutable audit entry.
type AuditLog struct {
	ID              int64
	SequenceNumber  uint64
	CompanyID       apiTypes.CompanyID
	EntityType      string
	EntityID        string
	Action          string
	ActorID         string
	Reason          string
	CorrelationID   string
	Payload         []byte
	Timestamp       time.Time
	PreviousLogHash string
	CurrentLogHash  string
}

// CalculateHash calculates the SHA-256 digest over the audit entry fields.
func CalculateHash(log AuditLog) string {
	payloadSum := sha256.Sum256(log.Payload)
	payloadHash := hex.EncodeToString(payloadSum[:])
	h := sha256.New()
	writeAuditHashField(h, "v2")
	writeAuditHashField(h, fmt.Sprint(log.SequenceNumber))
	writeAuditHashField(h, log.CompanyID.String())
	writeAuditHashField(h, log.EntityType)
	writeAuditHashField(h, log.EntityID)
	writeAuditHashField(h, log.Action)
	writeAuditHashField(h, log.ActorID)
	writeAuditHashField(h, log.Reason)
	writeAuditHashField(h, log.CorrelationID)
	writeAuditHashField(h, fmt.Sprint(log.Timestamp.UTC().Truncate(time.Microsecond).UnixNano()))
	writeAuditHashField(h, payloadHash)
	writeAuditHashField(h, log.PreviousLogHash)
	sum := h.Sum(nil)
	return hex.EncodeToString(sum[:])
}

func writeAuditHashField(h interface{ Write([]byte) (int, error) }, value string) {
	_, _ = fmt.Fprintf(h, "%d:", len(value))
	_, _ = h.Write([]byte(value))
}

type AuditLogRepository interface {
	AppendAuditLog(ctx context.Context, log AuditLog) error
	GetLastAuditLogForCompany(ctx context.Context, companyID apiTypes.CompanyID) (*AuditLog, error)
	IterateAuditLogs(ctx context.Context, companyID apiTypes.CompanyID, fn func(log AuditLog) error) error
}

// AtomicAuditLogRepository appends an audit record while atomically assigning
// its sequence number and previous hash for the company.
type AtomicAuditLogRepository interface {
	AppendAuditLogAtomic(ctx context.Context, log AuditLog) (AuditLog, error)
}

// AuditCheckpoint records a versioned chain position retained outside the mutable audit log.
type AuditCheckpoint struct {
	CompanyID      apiTypes.CompanyID
	SequenceNumber uint64
	Version        uint64
	LogHash        string
}

// ValidateCheckpoint enforces the shared shape rules for an external audit
// checkpoint. Both adapters must apply the same rules before persisting one.
func ValidateCheckpoint(checkpoint AuditCheckpoint) error {
	if checkpoint.CompanyID == (apiTypes.CompanyID{}) {
		return ErrInvalidAuditCheckpoint
	}
	decoded, err := hex.DecodeString(checkpoint.LogHash)
	if err != nil || len(decoded) != sha256.Size {
		return ErrInvalidAuditCheckpoint
	}
	if checkpoint.SequenceNumber == 0 {
		if checkpoint.LogHash != GenesisHash {
			return ErrInvalidAuditCheckpoint
		}
		return nil
	}
	if checkpoint.Version == 0 {
		return ErrInvalidAuditCheckpoint
	}
	return nil
}

// AuditVerificationSnapshot binds the logs and external checkpoint to one
// stable read. Logs must contain exactly SnapshotSequence records. The
// checkpoint is trusted custody from outside the mutable log; when no trusted
// checkpoint exists, CheckpointPresent must be false and checkpoint-based
// verification must fail closed instead of silently accepting an empty anchor.
type AuditVerificationSnapshot struct {
	CompanyID         apiTypes.CompanyID
	SnapshotSequence  uint64
	Checkpoint        AuditCheckpoint
	CheckpointPresent bool
	Logs              []AuditLog
}

// AuditVerificationSnapshotRepository returns logs and their trusted external
// checkpoint from one consistent snapshot. If the checkpoint or log boundary
// changes while reading, it must return ErrAuditSnapshotConflict.
type AuditVerificationSnapshotRepository interface {
	GetAuditVerificationSnapshot(ctx context.Context, companyID apiTypes.CompanyID) (AuditVerificationSnapshot, error)
}

type AuditChain struct {
	repo AuditLogRepository
}

func NewAuditChain(repo AuditLogRepository) *AuditChain {
	return &AuditChain{repo: repo}
}

// Append appends an audit log into the cryptographic chain, ensuring link to previous hash.
func (c *AuditChain) Append(ctx context.Context, log AuditLog) (AuditLog, error) {
	return c.AppendAt(ctx, log, time.Now().UTC())
}

// AppendAt appends an audit log into the cryptographic chain using an explicit timestamp.
func (c *AuditChain) AppendAt(ctx context.Context, log AuditLog, now time.Time) (AuditLog, error) {
	if c == nil {
		return AuditLog{}, ErrAuditChainRequired
	}
	if ctx == nil {
		return AuditLog{}, ErrAuditContextRequired
	}
	if log.CompanyID == (apiTypes.CompanyID{}) {
		return AuditLog{}, ErrInvalidAuditCompany
	}
	if c.repo == nil {
		return AuditLog{}, ErrAuditRepositoryRequired
	}
	repository, ok := c.repo.(AtomicAuditLogRepository)
	if !ok {
		return AuditLog{}, ErrAtomicAuditRepositoryRequired
	}
	if log.Timestamp.IsZero() {
		log.Timestamp = now.UTC()
	} else {
		log.Timestamp = log.Timestamp.UTC()
	}
	return repository.AppendAuditLogAtomic(ctx, log)
}

// VerifyAuditIntegrity verifies that all audit records for a company form an unbroken hash chain
// and that none of the payloads or metadata have been modified.
func (c *AuditChain) VerifyAuditIntegrity(ctx context.Context, companyID apiTypes.CompanyID) (bool, error) {
	if c == nil {
		return false, ErrAuditChainRequired
	}
	if ctx == nil {
		return false, ErrAuditContextRequired
	}
	if companyID == (apiTypes.CompanyID{}) {
		return false, ErrInvalidAuditCompany
	}
	if c.repo == nil {
		return false, ErrAuditRepositoryRequired
	}
	var expectedPrevHash = GenesisHash
	var expectedSequence uint64
	var logIndex int64

	err := c.repo.IterateAuditLogs(ctx, companyID, func(log AuditLog) error {
		logIndex++
		expectedSequence++
		if log.CompanyID != companyID {
			return fmt.Errorf("%w: at log %d, company id does not match requested company", ErrAuditChainBroken, log.ID)
		}
		if log.SequenceNumber != expectedSequence {
			return fmt.Errorf("%w: at log %d, expected sequence %d, got %d",
				ErrAuditChainBroken, log.ID, expectedSequence, log.SequenceNumber)
		}
		if log.PreviousLogHash != expectedPrevHash {
			return fmt.Errorf("%w: at log %d, expected prev hash %s, got %s",
				ErrAuditChainBroken, log.ID, expectedPrevHash, log.PreviousLogHash)
		}

		computedHash := CalculateHash(log)
		if computedHash != log.CurrentLogHash {
			return fmt.Errorf("%w: at log %d (tampered content), computed %s != stored %s",
				ErrAuditChainBroken, log.ID, computedHash, log.CurrentLogHash)
		}

		expectedPrevHash = log.CurrentLogHash
		return nil
	})

	if err != nil {
		return false, err
	}
	return true, nil
}

// VerifyAuditIntegrityWithCheckpoint verifies one stable log snapshot against
// its external checkpoint. Only snapshot acquisition conflicts are retried;
// mismatches returned in a stable snapshot are integrity failures.
func (c *AuditChain) VerifyAuditIntegrityWithCheckpoint(
	ctx context.Context,
	companyID apiTypes.CompanyID,
) (bool, error) {
	if c == nil {
		return false, ErrAuditChainRequired
	}
	if ctx == nil {
		return false, ErrAuditContextRequired
	}
	if companyID == (apiTypes.CompanyID{}) {
		return false, ErrInvalidAuditCompany
	}
	if c.repo == nil {
		return false, ErrAuditRepositoryRequired
	}
	repository, ok := c.repo.(AuditVerificationSnapshotRepository)
	if !ok {
		return false, ErrAuditSnapshotRepositoryRequired
	}

	for attempt := 0; attempt < maxAuditSnapshotAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		snapshot, err := repository.GetAuditVerificationSnapshot(ctx, companyID)
		if errors.Is(err, ErrAuditSnapshotConflict) {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return false, ctxErr
			}
			continue
		}
		if err != nil {
			return false, err
		}
		if err := verifyAuditSnapshot(companyID, snapshot); err != nil {
			return false, err
		}
		return true, nil
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	return false, fmt.Errorf("%w: %w", ErrAuditSnapshotConsistencyRetryable, ErrAuditSnapshotConflict)
}

func verifyAuditSnapshot(companyID apiTypes.CompanyID, snapshot AuditVerificationSnapshot) error {
	if snapshot.CompanyID != companyID || snapshot.Checkpoint.CompanyID != companyID {
		return ErrInvalidAuditSnapshot
	}
	if !snapshot.CheckpointPresent {
		return ErrAuditCheckpointMismatch
	}
	checkpoint := snapshot.Checkpoint
	if err := ValidateCheckpoint(checkpoint); err != nil {
		return err
	}
	if snapshot.SnapshotSequence != checkpoint.SequenceNumber {
		return ErrAuditCheckpointMismatch
	}
	if uint64(len(snapshot.Logs)) != snapshot.SnapshotSequence {
		return ErrAuditCheckpointMismatch
	}

	expectedPrevHash := GenesisHash
	for index, log := range snapshot.Logs {
		expectedSequence := uint64(index + 1)
		if log.CompanyID != companyID {
			return fmt.Errorf("%w: at log %d, company id does not match snapshot", ErrAuditChainBroken, log.ID)
		}
		if log.SequenceNumber != expectedSequence {
			return fmt.Errorf("%w: at log %d, expected sequence %d, got %d",
				ErrAuditChainBroken, log.ID, expectedSequence, log.SequenceNumber)
		}
		if log.PreviousLogHash != expectedPrevHash {
			return fmt.Errorf("%w: at log %d, expected prev hash %s, got %s",
				ErrAuditChainBroken, log.ID, expectedPrevHash, log.PreviousLogHash)
		}
		if computedHash := CalculateHash(log); computedHash != log.CurrentLogHash {
			return fmt.Errorf("%w: at log %d (tampered content), computed %s != stored %s",
				ErrAuditChainBroken, log.ID, computedHash, log.CurrentLogHash)
		}
		expectedPrevHash = log.CurrentLogHash
	}

	if expectedPrevHash != checkpoint.LogHash {
		return ErrAuditCheckpointMismatch
	}
	return nil
}
