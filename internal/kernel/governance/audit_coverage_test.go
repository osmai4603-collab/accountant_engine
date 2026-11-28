package governance

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/osmai4603-collab/accounting_engine/internal/kernel/types"
	apiTypes "github.com/osmai4603-collab/accounting_engine/internal/kernel/types"
)

type snapshotErrorRepo struct {
	err error
}

func (r *snapshotErrorRepo) AppendAuditLog(ctx context.Context, log AuditLog) error { return nil }
func (r *snapshotErrorRepo) GetLastAuditLogForCompany(ctx context.Context, companyID apiTypes.CompanyID) (*AuditLog, error) {
	return nil, nil
}
func (r *snapshotErrorRepo) IterateAuditLogs(ctx context.Context, companyID apiTypes.CompanyID, fn func(log AuditLog) error) error {
	if r.err != nil {
		return r.err
	}
	return nil
}
func (r *snapshotErrorRepo) GetAuditVerificationSnapshot(ctx context.Context, companyID apiTypes.CompanyID) (AuditVerificationSnapshot, error) {
	return AuditVerificationSnapshot{}, r.err
}

func TestAuditChainAdditionalIntegrityAndBranchCoverage(t *testing.T) {
	companyID := types.NewCompanyID()
	otherCompanyID := types.NewCompanyID()
	ctx := context.Background()

	// 1. AppendAt with explicit timestamp (not IsZero)
	repo := newInMemoryAuditRepo()
	chain := NewAuditChain(repo)
	explicitTime := time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC)
	_, err := chain.AppendAt(ctx, AuditLog{
		CompanyID: companyID,
		Timestamp: explicitTime,
		Action:    "CREATE",
	}, time.Now())
	if err != nil {
		t.Fatalf("AppendAt with explicit time: %v", err)
	}

	// 2. IterateAuditLogs failure branches in VerifyAuditIntegrity:
	// a. log.CompanyID != companyID
	repoWithBadCompany := newInMemoryAuditRepo()
	repoWithBadCompany.logs[companyID] = []AuditLog{
		{ID: 1, SequenceNumber: 1, CompanyID: otherCompanyID, PreviousLogHash: GenesisHash, CurrentLogHash: "x"},
	}
	chainBadComp := NewAuditChain(repoWithBadCompany)
	if _, err := chainBadComp.VerifyAuditIntegrity(ctx, companyID); !errors.Is(err, ErrAuditChainBroken) {
		t.Fatalf("mismatched companyID in VerifyAuditIntegrity error = %v, want ErrAuditChainBroken", err)
	}

	// b. log.SequenceNumber != expectedSequence
	repoWithBadSeq := newInMemoryAuditRepo()
	repoWithBadSeq.logs[companyID] = []AuditLog{
		{ID: 1, SequenceNumber: 2, CompanyID: companyID, PreviousLogHash: GenesisHash, CurrentLogHash: "x"},
	}
	chainBadSeq := NewAuditChain(repoWithBadSeq)
	if _, err := chainBadSeq.VerifyAuditIntegrity(ctx, companyID); !errors.Is(err, ErrAuditChainBroken) {
		t.Fatalf("mismatched sequence in VerifyAuditIntegrity error = %v, want ErrAuditChainBroken", err)
	}

	// c. log.PreviousLogHash != expectedPrevHash
	repoWithBadPrev := newInMemoryAuditRepo()
	repoWithBadPrev.logs[companyID] = []AuditLog{
		{ID: 1, SequenceNumber: 1, CompanyID: companyID, PreviousLogHash: "wrong_hash", CurrentLogHash: "x"},
	}
	chainBadPrev := NewAuditChain(repoWithBadPrev)
	if _, err := chainBadPrev.VerifyAuditIntegrity(ctx, companyID); !errors.Is(err, ErrAuditChainBroken) {
		t.Fatalf("mismatched prev hash in VerifyAuditIntegrity error = %v, want ErrAuditChainBroken", err)
	}

	// 3. VerifyAuditIntegrityWithCheckpoint branches:
	// a. cancelled context before loop or inside loop
	cancCtx, cancel := context.WithCancel(ctx)
	cancel()
	snapshotRepo := newSnapshotAuditRepo()
	chainSnap := NewAuditChain(snapshotRepo)
	if _, err := chainSnap.VerifyAuditIntegrityWithCheckpoint(cancCtx, companyID); err == nil {
		t.Fatal("expected error on cancelled context")
	}

	// b. error from GetAuditVerificationSnapshot other than ErrAuditSnapshotConflict
	customErr := errors.New("custom storage failure")
	errRepo := &snapshotErrorRepo{err: customErr}
	chainErr := NewAuditChain(errRepo)
	if _, err := chainErr.VerifyAuditIntegrityWithCheckpoint(ctx, companyID); !errors.Is(err, customErr) {
		t.Fatalf("custom snapshot error = %v, want customErr", err)
	}

	// c. conflict retry exhausted
	conflictRepo := &snapshotErrorRepo{err: ErrAuditSnapshotConflict}
	chainConflict := NewAuditChain(conflictRepo)
	if _, err := chainConflict.VerifyAuditIntegrityWithCheckpoint(ctx, companyID); !errors.Is(err, ErrAuditSnapshotConsistencyRetryable) {
		t.Fatalf("conflict exhausted error = %v, want ErrAuditSnapshotConsistencyRetryable", err)
	}

	// 4. verifyAuditSnapshot branches:
	// a. companyID mismatch
	if err := verifyAuditSnapshot(companyID, AuditVerificationSnapshot{
		CompanyID: otherCompanyID,
	}); !errors.Is(err, ErrInvalidAuditSnapshot) {
		t.Fatalf("snapshot companyID mismatch error = %v, want ErrInvalidAuditSnapshot", err)
	}

	// b. invalid checkpoint sequence/logHash/version
	badCheckpoint1 := AuditVerificationSnapshot{
		CompanyID:         companyID,
		CheckpointPresent: true,
		Checkpoint: AuditCheckpoint{
			CompanyID:      companyID,
			SequenceNumber: 0,
			LogHash:        "non-genesis",
		},
	}
	if err := verifyAuditSnapshot(companyID, badCheckpoint1); !errors.Is(err, ErrInvalidAuditCheckpoint) {
		t.Fatalf("bad checkpoint 1 error = %v, want ErrInvalidAuditCheckpoint", err)
	}

	badCheckpoint2 := AuditVerificationSnapshot{
		CompanyID:         companyID,
		CheckpointPresent: true,
		Checkpoint: AuditCheckpoint{
			CompanyID:      companyID,
			SequenceNumber: 5,
			LogHash:        "",
		},
	}
	if err := verifyAuditSnapshot(companyID, badCheckpoint2); !errors.Is(err, ErrInvalidAuditCheckpoint) {
		t.Fatalf("bad checkpoint 2 error = %v, want ErrInvalidAuditCheckpoint", err)
	}

	// c. len(Logs) != SnapshotSequence
	mismatchedLen := AuditVerificationSnapshot{
		CompanyID:         companyID,
		CheckpointPresent: true,
		SnapshotSequence:  1,
		Checkpoint: AuditCheckpoint{
			CompanyID:      companyID,
			SequenceNumber: 1,
			LogHash:        strings.Repeat("a", 64),
			Version:        1,
		},
		Logs: nil, // len 0 != 1
	}
	if err := verifyAuditSnapshot(companyID, mismatchedLen); !errors.Is(err, ErrAuditCheckpointMismatch) {
		t.Fatalf("mismatched logs len error = %v, want ErrAuditCheckpointMismatch", err)
	}

	// d. snapshot.Logs companyID mismatch
	snapWithBadCompLog := AuditVerificationSnapshot{
		CompanyID:         companyID,
		CheckpointPresent: true,
		SnapshotSequence:  1,
		Checkpoint: AuditCheckpoint{
			CompanyID:      companyID,
			SequenceNumber: 1,
			LogHash:        strings.Repeat("a", 64),
			Version:        1,
		},
		Logs: []AuditLog{
			{ID: 1, SequenceNumber: 1, CompanyID: otherCompanyID, PreviousLogHash: GenesisHash},
		},
	}
	if err := verifyAuditSnapshot(companyID, snapWithBadCompLog); !errors.Is(err, ErrAuditChainBroken) {
		t.Fatalf("log company mismatch in snapshot error = %v, want ErrAuditChainBroken", err)
	}

	// e. snapshot.Logs SequenceNumber mismatch
	snapWithBadSeqLog := AuditVerificationSnapshot{
		CompanyID:         companyID,
		CheckpointPresent: true,
		SnapshotSequence:  1,
		Checkpoint: AuditCheckpoint{
			CompanyID:      companyID,
			SequenceNumber: 1,
			LogHash:        strings.Repeat("a", 64),
			Version:        1,
		},
		Logs: []AuditLog{
			{ID: 1, SequenceNumber: 2, CompanyID: companyID, PreviousLogHash: GenesisHash},
		},
	}
	if err := verifyAuditSnapshot(companyID, snapWithBadSeqLog); !errors.Is(err, ErrAuditChainBroken) {
		t.Fatalf("log seq mismatch in snapshot error = %v, want ErrAuditChainBroken", err)
	}

	// f. snapshot.Logs PreviousLogHash mismatch
	snapWithBadPrevLog := AuditVerificationSnapshot{
		CompanyID:         companyID,
		CheckpointPresent: true,
		SnapshotSequence:  1,
		Checkpoint: AuditCheckpoint{
			CompanyID:      companyID,
			SequenceNumber: 1,
			LogHash:        strings.Repeat("a", 64),
			Version:        1,
		},
		Logs: []AuditLog{
			{ID: 1, SequenceNumber: 1, CompanyID: companyID, PreviousLogHash: "wrong_prev"},
		},
	}
	if err := verifyAuditSnapshot(companyID, snapWithBadPrevLog); !errors.Is(err, ErrAuditChainBroken) {
		t.Fatalf("log prev hash mismatch in snapshot error = %v, want ErrAuditChainBroken", err)
	}

	// 5. verifier.go VerifyAuditIntegrity error wrapping
	errIterRepo := &snapshotErrorRepo{err: errors.New("iteration failed")}
	if _, err := VerifyAuditIntegrity(ctx, errIterRepo, companyID); !errors.Is(err, ErrAuditChainBroken) {
		t.Fatalf("VerifyAuditIntegrity wrapper error = %v, want ErrAuditChainBroken", err)
	}
}

func TestAuditChainRejectsUninitializedReceiverAndRepositories(t *testing.T) {
	companyID := types.NewCompanyID()
	log := AuditLog{CompanyID: companyID}
	now := time.Now()

	// 1. nil receiver
	if _, err := (*AuditChain)(nil).AppendAt(context.Background(), log, now); !errors.Is(err, ErrAuditChainRequired) {
		t.Fatalf("nil receiver AppendAt error = %v, want ErrAuditChainRequired", err)
	}
	if _, err := (*AuditChain)(nil).VerifyAuditIntegrity(context.Background(), companyID); !errors.Is(err, ErrAuditChainRequired) {
		t.Fatalf("nil receiver VerifyAuditIntegrity error = %v, want ErrAuditChainRequired", err)
	}
	if _, err := (*AuditChain)(nil).VerifyAuditIntegrityWithCheckpoint(context.Background(), companyID); !errors.Is(err, ErrAuditChainRequired) {
		t.Fatalf("nil receiver VerifyAuditIntegrityWithCheckpoint error = %v, want ErrAuditChainRequired", err)
	}

	// 2. nil context
	chain := NewAuditChain(newInMemoryAuditRepo())
	if _, err := chain.AppendAt(nil, log, now); !errors.Is(err, ErrAuditContextRequired) {
		t.Fatalf("nil context AppendAt error = %v, want ErrAuditContextRequired", err)
	}
	if _, err := chain.VerifyAuditIntegrity(nil, companyID); !errors.Is(err, ErrAuditContextRequired) {
		t.Fatalf("nil context VerifyAuditIntegrity error = %v, want ErrAuditContextRequired", err)
	}
	if _, err := chain.VerifyAuditIntegrityWithCheckpoint(nil, companyID); !errors.Is(err, ErrAuditContextRequired) {
		t.Fatalf("nil context VerifyAuditIntegrityWithCheckpoint error = %v, want ErrAuditContextRequired", err)
	}

	// 3. invalid companyID
	if _, err := chain.AppendAt(context.Background(), AuditLog{}, now); !errors.Is(err, ErrInvalidAuditCompany) {
		t.Fatalf("empty company AppendAt error = %v, want ErrInvalidAuditCompany", err)
	}
	if _, err := chain.VerifyAuditIntegrity(context.Background(), types.CompanyID{}); !errors.Is(err, ErrInvalidAuditCompany) {
		t.Fatalf("empty company VerifyAuditIntegrity error = %v, want ErrInvalidAuditCompany", err)
	}
	if _, err := chain.VerifyAuditIntegrityWithCheckpoint(context.Background(), types.CompanyID{}); !errors.Is(err, ErrInvalidAuditCompany) {
		t.Fatalf("empty company VerifyAuditIntegrityWithCheckpoint error = %v, want ErrInvalidAuditCompany", err)
	}

	// 4. nil repo in chain
	chainNilRepo := &AuditChain{repo: nil}
	if _, err := chainNilRepo.AppendAt(context.Background(), log, now); !errors.Is(err, ErrAuditRepositoryRequired) {
		t.Fatalf("nil repo AppendAt error = %v, want ErrAuditRepositoryRequired", err)
	}
	if _, err := chainNilRepo.VerifyAuditIntegrity(context.Background(), companyID); !errors.Is(err, ErrAuditRepositoryRequired) {
		t.Fatalf("nil repo VerifyAuditIntegrity error = %v, want ErrAuditRepositoryRequired", err)
	}
	if _, err := chainNilRepo.VerifyAuditIntegrityWithCheckpoint(context.Background(), companyID); !errors.Is(err, ErrAuditRepositoryRequired) {
		t.Fatalf("nil repo VerifyAuditIntegrityWithCheckpoint error = %v, want ErrAuditRepositoryRequired", err)
	}
}

// TestCalculateHashGoldenVector verifies that CalculateHash is strictly deterministic
// and changes if any length prefix or hashing field is altered.
func TestCalculateHashGoldenVector(t *testing.T) {
	companyID := apiTypes.NewCompanyID()
	fixedTime := time.Date(2026, 11, 21, 6, 0, 0, 0, time.UTC)
	log := AuditLog{
		CompanyID:       companyID,
		SequenceNumber:  1,
		EntityType:      "JOURNAL_ENTRY",
		EntityID:        "JE-100",
		Action:          "POST",
		ActorID:         "actor-1",
		Reason:          "rebalancing",
		CorrelationID:   "corr-1",
		Timestamp:       fixedTime,
		Payload:         []byte(`{"key":"value"}`),
		PreviousLogHash: GenesisHash,
	}
	hash1 := CalculateHash(log)
	if hash1 == "" {
		t.Fatal("expected non-empty hash")
	}
	// Determinism
	hash2 := CalculateHash(log)
	if hash1 != hash2 {
		t.Fatalf("hash non-deterministic: %s != %s", hash1, hash2)
	}

	// Any modification to any field must produce a different hash
	logMod := log
	logMod.Reason = "rebalancing " // single space
	if CalculateHash(logMod) == hash1 {
		t.Fatal("expected hash to change on reason modification")
	}

	logMod2 := log
	logMod2.Payload = []byte(`{"key":"value "}`)
	if CalculateHash(logMod2) == hash1 {
		t.Fatal("expected hash to change on payload modification")
	}
}

func TestCalculateHashCanonicalizesTimestampToStoragePrecision(t *testing.T) {
	baseTime := time.Date(2026, 11, 22, 3, 34, 42, 123456789, time.UTC)
	base := AuditLog{
		SequenceNumber: 1, CompanyID: apiTypes.NewCompanyID(),
		EntityType: "journal_entry", EntityID: "JE-100", Action: "POST",
		Timestamp: baseTime, PreviousLogHash: GenesisHash,
	}
	truncated := base
	truncated.Timestamp = baseTime.Truncate(time.Microsecond)
	if CalculateHash(base) != CalculateHash(truncated) {
		t.Fatal("sub-microsecond timestamp variants must have the same canonical audit hash")
	}
}

func TestCalculateHashProtectsEverySemanticField(t *testing.T) {
	fixedTime := time.Date(2026, 11, 21, 6, 0, 0, 0, time.UTC)
	base := AuditLog{
		SequenceNumber:  1,
		CompanyID:       apiTypes.NewCompanyID(),
		EntityType:      "journal_entry",
		EntityID:        "JE-100",
		Action:          "POST",
		ActorID:         "actor-1",
		Reason:          "approved",
		CorrelationID:   "corr-1",
		Timestamp:       fixedTime,
		Payload:         []byte(`{"amount":"100"}`),
		PreviousLogHash: GenesisHash,
	}
	baseHash := CalculateHash(base)

	mutations := []struct {
		name string
		log  AuditLog
	}{
		{name: "sequence", log: func() AuditLog { l := base; l.SequenceNumber++; return l }()},
		{name: "company", log: func() AuditLog { l := base; l.CompanyID = apiTypes.NewCompanyID(); return l }()},
		{name: "entity type", log: func() AuditLog { l := base; l.EntityType = "reversal"; return l }()},
		{name: "entity id", log: func() AuditLog { l := base; l.EntityID = "JE-101"; return l }()},
		{name: "action", log: func() AuditLog { l := base; l.Action = "REVERSE"; return l }()},
		{name: "actor", log: func() AuditLog { l := base; l.ActorID = "actor-2"; return l }()},
		{name: "reason", log: func() AuditLog { l := base; l.Reason = "rejected"; return l }()},
		{name: "correlation", log: func() AuditLog { l := base; l.CorrelationID = "corr-2"; return l }()},
		{name: "timestamp", log: func() AuditLog { l := base; l.Timestamp = fixedTime.Add(time.Microsecond); return l }()},
		{name: "payload", log: func() AuditLog { l := base; l.Payload = []byte(`{"amount":"101"}`); return l }()},
		{name: "previous hash", log: func() AuditLog { l := base; l.PreviousLogHash = strings.Repeat("a", 64); return l }()},
	}

	for _, test := range mutations {
		t.Run(test.name, func(t *testing.T) {
			if got := CalculateHash(test.log); got == baseHash {
				t.Fatalf("semantic mutation %q did not change audit hash", test.name)
			}
		})
	}
}

func TestValidateCheckpointRejectsMalformedAnchors(t *testing.T) {
	companyID := apiTypes.NewCompanyID()
	validHash := strings.Repeat("a", 64)
	cases := []struct {
		name       string
		checkpoint AuditCheckpoint
	}{
		{name: "missing company", checkpoint: AuditCheckpoint{SequenceNumber: 1, Version: 1, LogHash: validHash}},
		{name: "missing hash", checkpoint: AuditCheckpoint{CompanyID: companyID, SequenceNumber: 1, Version: 1}},
		{name: "short hash", checkpoint: AuditCheckpoint{CompanyID: companyID, SequenceNumber: 1, Version: 1, LogHash: "abc"}},
		{name: "non hex hash", checkpoint: AuditCheckpoint{CompanyID: companyID, SequenceNumber: 1, Version: 1, LogHash: strings.Repeat("z", 64)}},
		{name: "missing version", checkpoint: AuditCheckpoint{CompanyID: companyID, SequenceNumber: 1, LogHash: validHash}},
		{name: "non genesis at zero", checkpoint: AuditCheckpoint{CompanyID: companyID, SequenceNumber: 0, Version: 1, LogHash: validHash}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateCheckpoint(tc.checkpoint); !errors.Is(err, ErrInvalidAuditCheckpoint) {
				t.Fatalf("ValidateCheckpoint(%+v) = %v, want ErrInvalidAuditCheckpoint", tc.checkpoint, err)
			}
		})
	}
	if err := ValidateCheckpoint(AuditCheckpoint{CompanyID: companyID, SequenceNumber: 3, Version: 1, LogHash: validHash}); err != nil {
		t.Fatalf("valid checkpoint rejected: %v", err)
	}
	if err := ValidateCheckpoint(AuditCheckpoint{CompanyID: companyID, LogHash: GenesisHash}); err != nil {
		t.Fatalf("valid genesis checkpoint rejected: %v", err)
	}
}

func TestVerifyAuditSnapshotSequenceMismatchKillsMutant(t *testing.T) {
	companyID := apiTypes.NewCompanyID()
	snapshot := AuditVerificationSnapshot{
		CompanyID:         companyID,
		CheckpointPresent: true,
		SnapshotSequence:  5, // mismatch with checkpoint SequenceNumber 1
		Checkpoint: AuditCheckpoint{
			CompanyID:      companyID,
			SequenceNumber: 1,
			LogHash:        strings.Repeat("a", 64),
			Version:        1,
		},
		Logs: make([]AuditLog, 5),
	}
	err := verifyAuditSnapshot(companyID, snapshot)
	if !errors.Is(err, ErrAuditCheckpointMismatch) {
		t.Fatalf("expected ErrAuditCheckpointMismatch, got %v", err)
	}
}
