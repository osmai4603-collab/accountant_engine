package drdrill

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	kernelgovernance "github.com/osmai4603-collab/accounting_engine/internal/kernel/governance"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
)

const auditGenesisHash = "0000000000000000000000000000000000000000000000000000000000000000"

type drSnapshot struct {
	PostedMoves  int    `json:"posted_moves"`
	MoveLines    int    `json:"move_lines"`
	AuditRecords int    `json:"audit_records"`
	MoveChecksum string `json:"move_checksum"`
}

func captureSnapshot(t *testing.T, ctx context.Context, pool *pgxpool.Pool, companyID types.CompanyID) drSnapshot {
	t.Helper()
	var snap drSnapshot
	if err := pool.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM account_moves WHERE company_id=$1 AND status='posted'),
		(SELECT count(*) FROM account_move_lines WHERE company_id=$1),
		(SELECT count(*) FROM financial_audit_logs WHERE company_id=$1)`, companyID).
		Scan(&snap.PostedMoves, &snap.MoveLines, &snap.AuditRecords); err != nil {
		t.Fatalf("capture snapshot counts: %v", err)
	}
	rows, err := pool.Query(ctx, `SELECT id::text, entry_number, status FROM account_moves
		WHERE company_id=$1 ORDER BY id`, companyID)
	if err != nil {
		t.Fatalf("capture snapshot rows: %v", err)
	}
	defer rows.Close()
	digest := sha256.New()
	for rows.Next() {
		var id, status string
		var entryNumber int64
		if err := rows.Scan(&id, &entryNumber, &status); err != nil {
			t.Fatalf("scan snapshot row: %v", err)
		}
		fmt.Fprintf(digest, "%s:%d:%s;", id, entryNumber, status)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate snapshot rows: %v", err)
	}
	snap.MoveChecksum = hex.EncodeToString(digest.Sum(nil))
	return snap
}

// verifyPostgresAuditChain verifies the canonical v2 audit chain after backup
// restore. When expectCheckpoint is true, the trusted external checkpoint must
// have survived the restore and match the final chain hash; a missing row is a
// failure, not a reason to skip the comparison.
func verifyPostgresAuditChain(t *testing.T, ctx context.Context, pool *pgxpool.Pool, companyID types.CompanyID, expectCheckpoint bool) {
	t.Helper()
	rows, err := pool.Query(ctx, `SELECT sequence_number, entity_type, entity_id, action, actor_id,
		reason, correlation_id, payload_after, timestamp, previous_log_hash, current_log_hash
		FROM financial_audit_logs WHERE company_id=$1 ORDER BY sequence_number, id`, companyID)
	if err != nil {
		t.Fatalf("query audit chain: %v", err)
	}
	defer rows.Close()

	previous := auditGenesisHash
	count := 0
	lastHash := auditGenesisHash
	for rows.Next() {
		var sequence int64
		var entityType, entityID, action, actorID, reason, correlationID, previousHash, currentHash string
		var payload []byte
		var timestamp time.Time
		if err := rows.Scan(&sequence, &entityType, &entityID, &action, &actorID, &reason,
			&correlationID, &payload, &timestamp, &previousHash, &currentHash); err != nil {
			t.Fatalf("scan audit row: %v", err)
		}
		if previousHash != previous {
			t.Fatalf("audit chain link %d broken: previous=%s want %s", count+1, previousHash, previous)
		}
		log := kernelgovernance.AuditLog{
			SequenceNumber: uint64(sequence), CompanyID: companyID,
			EntityType: entityType, EntityID: entityID, Action: action, ActorID: actorID,
			Reason: reason, CorrelationID: correlationID, Payload: payload,
			Timestamp: timestamp, PreviousLogHash: previousHash,
		}
		expected := kernelgovernance.CalculateHash(log)
		if currentHash != expected {
			t.Fatalf("audit row %d hash mismatch: stored=%s computed=%s (sequence=%d previous=%s company=%s entity=%s/%s action=%s actor=%s reason=%s correlation=%s timestamp_ns=%d payload=%x)",
				count+1, currentHash, expected, sequence, previousHash, companyID, entityType, entityID, action, actorID, reason, correlationID, timestamp.UTC().UnixNano(), payload)
		}
		previous = currentHash
		lastHash = currentHash
		count++
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate audit chain: %v", err)
	}
	if count == 0 {
		t.Fatal("no audit records found for company")
	}
	var checkpointSequence int64
	var checkpointHash string
	err = pool.QueryRow(ctx, `SELECT sequence_number, log_hash
		FROM external_audit_checkpoints WHERE company_id=$1`, companyID).
		Scan(&checkpointSequence, &checkpointHash)
	if err == nil {
		if checkpointSequence != int64(count) || checkpointHash != lastHash {
			t.Fatalf("restored external checkpoint mismatch: sequence=%d hash=%s, want sequence=%d hash=%s",
				checkpointSequence, checkpointHash, count, lastHash)
		}
		t.Logf("restored external checkpoint matched: sequence=%d hash=%s", checkpointSequence, checkpointHash)
	} else if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("query restored external checkpoint: %v", err)
	} else if expectCheckpoint {
		t.Fatal("external checkpoint created before backup was lost after restore")
	}
	t.Logf("verified %d audit-chain records for company %s", count, companyID)
}
