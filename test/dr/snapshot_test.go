package drdrill

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
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

// verifyPostgresAuditChain verifies the v1 SHA-256 hash-chain format written by
// storage/postgres.AppendAudit. The kernel governance verifier uses a v2 format
// and is therefore not applicable to rows written by this adapter; the gap is
// recorded in the P7 evidence log.
func verifyPostgresAuditChain(t *testing.T, ctx context.Context, pool *pgxpool.Pool, companyID types.CompanyID) {
	t.Helper()
	rows, err := pool.Query(ctx, `SELECT entity_type, entity_id::text, action, actor_id::text,
		coalesce(payload_after::text, 'null'), timestamp, previous_log_hash, current_log_hash
		FROM financial_audit_logs WHERE company_id=$1 ORDER BY id`, companyID)
	if err != nil {
		t.Fatalf("query audit chain: %v", err)
	}
	defer rows.Close()

	previous := auditGenesisHash
	count := 0
	for rows.Next() {
		var entityType, entityID, action, actorID, payload, previousHash, currentHash string
		var timestamp time.Time
		if err := rows.Scan(&entityType, &entityID, &action, &actorID, &payload, &timestamp, &previousHash, &currentHash); err != nil {
			t.Fatalf("scan audit row: %v", err)
		}
		if previousHash != previous {
			t.Fatalf("audit chain link %d broken: previous=%s want %s", count+1, previousHash, previous)
		}
		// pgx may return jsonb as a hex-escaped byte string. Decode it back to
		// the raw JSON bytes that storage/postgres.AppendAudit hashed.
		hashPayload := []byte(payload)
		if strings.HasPrefix(payload, `\x`) {
			decoded, err := hex.DecodeString(strings.TrimPrefix(payload, `\x`))
			if err != nil {
				t.Fatalf("decode audit payload %q: %v", payload, err)
			}
			hashPayload = decoded
		}

		h := sha256.New()
		for _, field := range []string{previousHash, companyID.String(), entityType, entityID, action, actorID} {
			_, _ = h.Write([]byte(field))
		}
		_, _ = h.Write(hashPayload)
		expected := hex.EncodeToString(h.Sum(nil))
		if currentHash != expected {
			t.Fatalf("audit row %d hash mismatch: stored=%s computed=%s (previous=%s company=%s entity=%s/%s action=%s actor=%s payload=%s)",
				count+1, currentHash, expected, previousHash, companyID, entityType, entityID, action, actorID, payload)
		}
		previous = currentHash
		count++
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate audit chain: %v", err)
	}
	if count == 0 {
		t.Fatal("no audit records found for company")
	}
	t.Logf("verified %d audit-chain records for company %s", count, companyID)
}
