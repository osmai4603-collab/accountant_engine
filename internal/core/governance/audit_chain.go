package governance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	apiTypes "github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
)

const GenesisHash = "0000000000000000000000000000000000000000000000000000000000000000"

var ErrAuditChainBroken = errors.New("audit chain integrity violation: tampering or broken link detected")

// AuditLog represents a cryptographically hashed immutable audit entry.
type AuditLog struct {
	ID              int64
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
	timestamp := log.Timestamp.UTC().Truncate(time.Microsecond).Format(time.RFC3339Nano)
	input := log.PreviousLogHash +
		log.CompanyID.String() +
		log.EntityType +
		log.EntityID +
		log.Action +
		log.ActorID +
		log.Reason +
		log.CorrelationID +
		string(log.Payload) +
		timestamp

	sum := sha256.Sum256([]byte(input))
	return hex.EncodeToString(sum[:])
}

type AuditLogRepository interface {
	AppendAuditLog(ctx context.Context, log AuditLog) error
	GetLastAuditLogForCompany(ctx context.Context, companyID apiTypes.CompanyID) (*AuditLog, error)
	IterateAuditLogs(ctx context.Context, companyID apiTypes.CompanyID, fn func(log AuditLog) error) error
}

type AuditChain struct {
	repo AuditLogRepository
	mu   sync.Mutex
}

func NewAuditChain(repo AuditLogRepository) *AuditChain {
	return &AuditChain{repo: repo}
}

// Append appends an audit log into the cryptographic chain, ensuring link to previous hash.
func (c *AuditChain) Append(ctx context.Context, log AuditLog) (AuditLog, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	last, err := c.repo.GetLastAuditLogForCompany(ctx, log.CompanyID)
	if err != nil {
		return AuditLog{}, err
	}

	if last == nil || last.CurrentLogHash == "" {
		log.PreviousLogHash = GenesisHash
	} else {
		log.PreviousLogHash = last.CurrentLogHash
	}

	if log.Timestamp.IsZero() {
		log.Timestamp = time.Now().UTC()
	} else {
		log.Timestamp = log.Timestamp.UTC()
	}

	log.CurrentLogHash = CalculateHash(log)

	if err := c.repo.AppendAuditLog(ctx, log); err != nil {
		return AuditLog{}, err
	}

	return log, nil
}

// VerifyAuditIntegrity verifies that all audit records for a company form an unbroken hash chain
// and that none of the payloads or metadata have been modified.
func (c *AuditChain) VerifyAuditIntegrity(ctx context.Context, companyID apiTypes.CompanyID) (bool, error) {
	var expectedPrevHash = GenesisHash
	var logIndex int64

	err := c.repo.IterateAuditLogs(ctx, companyID, func(log AuditLog) error {
		logIndex++
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
