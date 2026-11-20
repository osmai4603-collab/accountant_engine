package governance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	apiTypes "github.com/osmai4603-collab/accounting_engine/internal/kernal/types"
)

const GenesisHash = "0000000000000000000000000000000000000000000000000000000000000000"

var (
	ErrAuditChainBroken                = errors.New("audit chain integrity violation: tampering or broken link detected")
	ErrAtomicAuditRepositoryRequired   = errors.New("atomic audit repository is required")
	ErrAuditCheckpointProviderRequired = errors.New("external audit checkpoint provider is required")
	ErrInvalidAuditCheckpoint          = errors.New("external audit checkpoint is invalid")
	ErrAuditCheckpointMismatch         = errors.New("audit chain does not match external checkpoint")
)

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
	writeAuditHashField(h, fmt.Sprint(log.Timestamp.UTC().UnixNano()))
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

// AuditCheckpoint records a chain position retained outside the mutable audit log.
type AuditCheckpoint struct {
	CompanyID      apiTypes.CompanyID
	SequenceNumber uint64
	LogHash        string
}

// AuditCheckpointProvider supplies a trusted, externally retained chain checkpoint.
type AuditCheckpointProvider interface {
	GetAuditCheckpoint(ctx context.Context, companyID apiTypes.CompanyID) (AuditCheckpoint, error)
}

type AuditChain struct {
	repo AuditLogRepository
}

func NewAuditChain(repo AuditLogRepository) *AuditChain {
	return &AuditChain{repo: repo}
}

// Append appends an audit log into the cryptographic chain, ensuring link to previous hash.
func (c *AuditChain) Append(ctx context.Context, log AuditLog) (AuditLog, error) {
	repository, ok := c.repo.(AtomicAuditLogRepository)
	if !ok {
		return AuditLog{}, ErrAtomicAuditRepositoryRequired
	}
	if log.Timestamp.IsZero() {
		log.Timestamp = time.Now().UTC()
	} else {
		log.Timestamp = log.Timestamp.UTC()
	}
	return repository.AppendAuditLogAtomic(ctx, log)
}

// VerifyAuditIntegrity verifies that all audit records for a company form an unbroken hash chain
// and that none of the payloads or metadata have been modified.
func (c *AuditChain) VerifyAuditIntegrity(ctx context.Context, companyID apiTypes.CompanyID) (bool, error) {
	var expectedPrevHash = GenesisHash
	var expectedSequence uint64
	var logIndex int64

	err := c.repo.IterateAuditLogs(ctx, companyID, func(log AuditLog) error {
		logIndex++
		expectedSequence++
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

// VerifyAuditIntegrityWithCheckpoint also verifies the externally retained chain position.
func (c *AuditChain) VerifyAuditIntegrityWithCheckpoint(
	ctx context.Context,
	companyID apiTypes.CompanyID,
	provider AuditCheckpointProvider,
) (bool, error) {
	if provider == nil {
		return false, ErrAuditCheckpointProviderRequired
	}
	checkpoint, err := provider.GetAuditCheckpoint(ctx, companyID)
	if err != nil {
		return false, err
	}
	if checkpoint.CompanyID != companyID ||
		(checkpoint.SequenceNumber == 0 && checkpoint.LogHash != GenesisHash) ||
		(checkpoint.SequenceNumber > 0 && checkpoint.LogHash == "") {
		return false, ErrInvalidAuditCheckpoint
	}
	valid, err := c.VerifyAuditIntegrity(ctx, companyID)
	if err != nil || !valid {
		return valid, err
	}

	var latest AuditLog
	foundCheckpoint := checkpoint.SequenceNumber == 0
	err = c.repo.IterateAuditLogs(ctx, companyID, func(log AuditLog) error {
		latest = log
		if log.SequenceNumber == checkpoint.SequenceNumber {
			foundCheckpoint = true
			if log.CurrentLogHash != checkpoint.LogHash {
				return ErrAuditCheckpointMismatch
			}
		}
		return nil
	})
	if err != nil {
		return false, err
	}
	if !foundCheckpoint || latest.SequenceNumber != checkpoint.SequenceNumber {
		return false, ErrAuditCheckpointMismatch
	}
	if checkpoint.SequenceNumber == 0 && checkpoint.LogHash != GenesisHash {
		return false, ErrAuditCheckpointMismatch
	}
	return true, nil
}
