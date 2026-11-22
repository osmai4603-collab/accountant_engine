package governance

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/osmai4603-collab/accounting_engine/internal/kernal/types"
	apiTypes "github.com/osmai4603-collab/accounting_engine/internal/kernal/types"
)

type snapshotAuditRepo struct {
	mu          sync.Mutex
	logs        map[apiTypes.CompanyID][]AuditLog
	checkpoints map[apiTypes.CompanyID]AuditCheckpoint
}

func newSnapshotAuditRepo() *snapshotAuditRepo {
	return &snapshotAuditRepo{
		logs:        make(map[apiTypes.CompanyID][]AuditLog),
		checkpoints: make(map[apiTypes.CompanyID]AuditCheckpoint),
	}
}

func (r *snapshotAuditRepo) AppendAuditLog(ctx context.Context, log AuditLog) error {
	_, err := r.AppendAuditLogAtomic(ctx, log)
	return err
}

func (r *snapshotAuditRepo) AppendAuditLogAtomic(ctx context.Context, log AuditLog) (AuditLog, error) {
	if err := ctx.Err(); err != nil {
		return AuditLog{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	logs := r.logs[log.CompanyID]
	log.ID = int64(len(logs) + 1)
	log.SequenceNumber = uint64(len(logs) + 1)
	log.PreviousLogHash = GenesisHash
	if len(logs) > 0 {
		log.PreviousLogHash = logs[len(logs)-1].CurrentLogHash
	}
	log.CurrentLogHash = CalculateHash(log)
	r.logs[log.CompanyID] = append(logs, log)

	checkpoint := r.checkpoints[log.CompanyID]
	r.checkpoints[log.CompanyID] = AuditCheckpoint{
		CompanyID:      log.CompanyID,
		SequenceNumber: log.SequenceNumber,
		Version:        checkpoint.Version + 1,
		LogHash:        log.CurrentLogHash,
	}
	return log, nil
}

func (r *snapshotAuditRepo) GetLastAuditLogForCompany(
	ctx context.Context,
	companyID apiTypes.CompanyID,
) (*AuditLog, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	logs := r.logs[companyID]
	if len(logs) == 0 {
		return nil, nil
	}
	last := logs[len(logs)-1]
	last.Payload = append([]byte(nil), last.Payload...)
	return &last, nil
}

func (r *snapshotAuditRepo) IterateAuditLogs(
	ctx context.Context,
	companyID apiTypes.CompanyID,
	fn func(AuditLog) error,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	logs := cloneSnapshotAuditLogs(r.logs[companyID])
	r.mu.Unlock()
	for _, log := range logs {
		if err := fn(log); err != nil {
			return err
		}
	}
	return nil
}

func (r *snapshotAuditRepo) GetAuditVerificationSnapshot(
	ctx context.Context,
	companyID apiTypes.CompanyID,
) (AuditVerificationSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return AuditVerificationSnapshot{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	logs := cloneSnapshotAuditLogs(r.logs[companyID])
	checkpoint, ok := r.checkpoints[companyID]
	if !ok {
		checkpoint = AuditCheckpoint{CompanyID: companyID, LogHash: GenesisHash}
	}
	return AuditVerificationSnapshot{
		CompanyID:         companyID,
		SnapshotSequence:  uint64(len(logs)),
		Checkpoint:        checkpoint,
		CheckpointPresent: ok,
		Logs:              logs,
	}, nil
}

func cloneSnapshotAuditLogs(logs []AuditLog) []AuditLog {
	cloned := make([]AuditLog, len(logs))
	for i, log := range logs {
		cloned[i] = log
		cloned[i].Payload = append([]byte(nil), log.Payload...)
	}
	return cloned
}

type conflictedSnapshotRepo struct {
	*snapshotAuditRepo
	mu         sync.Mutex
	conflicts  int
	calls      int
	onConflict func()
}

func (r *conflictedSnapshotRepo) GetAuditVerificationSnapshot(
	ctx context.Context,
	companyID apiTypes.CompanyID,
) (AuditVerificationSnapshot, error) {
	r.mu.Lock()
	r.calls++
	if r.conflicts > 0 {
		r.conflicts--
		onConflict := r.onConflict
		r.mu.Unlock()
		if onConflict != nil {
			onConflict()
		}
		return AuditVerificationSnapshot{}, ErrAuditSnapshotConflict
	}
	r.mu.Unlock()
	return r.snapshotAuditRepo.GetAuditVerificationSnapshot(ctx, companyID)
}

func (r *conflictedSnapshotRepo) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

type pausedSnapshotRepo struct {
	*snapshotAuditRepo
	captured chan AuditVerificationSnapshot
	release  chan struct{}
}

func (r *pausedSnapshotRepo) GetAuditVerificationSnapshot(
	ctx context.Context,
	companyID apiTypes.CompanyID,
) (AuditVerificationSnapshot, error) {
	snapshot, err := r.snapshotAuditRepo.GetAuditVerificationSnapshot(ctx, companyID)
	if err != nil {
		return AuditVerificationSnapshot{}, err
	}
	r.captured <- snapshot
	select {
	case <-ctx.Done():
		return AuditVerificationSnapshot{}, ctx.Err()
	case <-r.release:
		return snapshot, nil
	}
}

func TestAuditCheckpointDetectsDeletedTail(t *testing.T) {
	ctx := context.Background()
	repo := newSnapshotAuditRepo()
	chain := NewAuditChain(repo)
	companyID := types.NewCompanyID()
	var last AuditLog
	for range 2 {
		var err error
		last, err = chain.Append(ctx, AuditLog{
			CompanyID: companyID, EntityType: "journal_entry",
			EntityID: types.NewJournalEntryID().String(), Action: "POST",
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if valid, err := chain.VerifyAuditIntegrityWithCheckpoint(ctx, companyID); !valid || err != nil {
		t.Fatalf("audit chain with matching checkpoint = (%v, %v), want valid", valid, err)
	}

	newest, err := chain.Append(ctx, AuditLog{
		CompanyID: companyID, EntityType: "journal_entry",
		EntityID: types.NewJournalEntryID().String(), Action: "POST",
	})
	if err != nil {
		t.Fatal(err)
	}
	if newest.SequenceNumber != last.SequenceNumber+1 {
		t.Fatalf("new sequence = %d, want %d", newest.SequenceNumber, last.SequenceNumber+1)
	}
	if valid, err := chain.VerifyAuditIntegrityWithCheckpoint(ctx, companyID); !valid || err != nil {
		t.Fatalf("updated external checkpoint = (%v, %v), want valid", valid, err)
	}

	repo.mu.Lock()
	repo.logs[companyID] = repo.logs[companyID][:1]
	repo.mu.Unlock()
	if valid, err := chain.VerifyAuditIntegrity(ctx, companyID); !valid || err != nil {
		t.Fatalf("truncated hash chain alone = (%v, %v), want valid without an anchor", valid, err)
	}
	if valid, err := chain.VerifyAuditIntegrityWithCheckpoint(ctx, companyID); valid || !errors.Is(err, ErrAuditCheckpointMismatch) {
		t.Fatalf("truncated chain with external checkpoint = (%v, %v), want checkpoint mismatch", valid, err)
	}
}

func TestAuditCheckpointRetryTracksConcurrentAdvancement(t *testing.T) {
	ctx := context.Background()
	baseRepo := newSnapshotAuditRepo()
	companyID := types.NewCompanyID()
	chain := NewAuditChain(baseRepo)
	if _, err := chain.Append(ctx, AuditLog{
		CompanyID: companyID, EntityType: "journal_entry",
		EntityID: types.NewJournalEntryID().String(), Action: "POST",
	}); err != nil {
		t.Fatal(err)
	}

	repo := &conflictedSnapshotRepo{snapshotAuditRepo: baseRepo, conflicts: 1}
	chain = NewAuditChain(repo)
	repo.onConflict = func() {
		if _, err := chain.Append(ctx, AuditLog{
			CompanyID: companyID, EntityType: "journal_entry",
			EntityID: types.NewJournalEntryID().String(), Action: "POST",
		}); err != nil {
			t.Errorf("append while snapshot advanced: %v", err)
		}
	}

	if valid, err := chain.VerifyAuditIntegrityWithCheckpoint(ctx, companyID); !valid || err != nil {
		t.Fatalf("verification after a retryable checkpoint advance = (%v, %v), want valid", valid, err)
	}
	if calls := repo.callCount(); calls != 2 {
		t.Fatalf("snapshot calls = %d, want one conflict and one consistent read", calls)
	}
}

func TestAuditCheckpointConflictsExhaustIntoRetryableError(t *testing.T) {
	ctx := context.Background()
	repo := &conflictedSnapshotRepo{
		snapshotAuditRepo: newSnapshotAuditRepo(),
		conflicts:         maxAuditSnapshotAttempts,
	}
	chain := NewAuditChain(repo)

	valid, err := chain.VerifyAuditIntegrityWithCheckpoint(ctx, types.NewCompanyID())
	if valid || !errors.Is(err, ErrAuditSnapshotConsistencyRetryable) ||
		!errors.Is(err, ErrAuditSnapshotConflict) {
		t.Fatalf("persistent snapshot conflicts = (%v, %v), want retryable consistency error", valid, err)
	}
	if calls := repo.callCount(); calls != maxAuditSnapshotAttempts {
		t.Fatalf("snapshot calls = %d, want bounded limit %d", calls, maxAuditSnapshotAttempts)
	}
}

func TestAuditVerificationUsesCapturedSnapshotBoundary(t *testing.T) {
	ctx := context.Background()
	baseRepo := newSnapshotAuditRepo()
	companyID := types.NewCompanyID()
	baseChain := NewAuditChain(baseRepo)
	if _, err := baseChain.Append(ctx, AuditLog{
		CompanyID: companyID, EntityType: "journal_entry",
		EntityID: types.NewJournalEntryID().String(), Action: "POST",
	}); err != nil {
		t.Fatal(err)
	}

	repo := &pausedSnapshotRepo{
		snapshotAuditRepo: baseRepo,
		captured:          make(chan AuditVerificationSnapshot, 1),
		release:           make(chan struct{}),
	}
	chain := NewAuditChain(repo)
	result := make(chan struct {
		valid bool
		err   error
	}, 1)
	go func() {
		valid, err := chain.VerifyAuditIntegrityWithCheckpoint(ctx, companyID)
		result <- struct {
			valid bool
			err   error
		}{valid: valid, err: err}
	}()

	var captured AuditVerificationSnapshot
	select {
	case captured = <-repo.captured:
	case <-time.After(5 * time.Second):
		t.Fatal("verification did not capture a snapshot")
	}
	if captured.SnapshotSequence != 1 || captured.Checkpoint.Version != 1 ||
		captured.Checkpoint.SequenceNumber != captured.SnapshotSequence ||
		captured.Checkpoint.LogHash != captured.Logs[0].CurrentLogHash {
		t.Fatalf("captured snapshot boundary/checkpoint mismatch: %+v", captured)
	}
	if _, err := chain.Append(ctx, AuditLog{
		CompanyID: companyID, EntityType: "journal_entry",
		EntityID: types.NewJournalEntryID().String(), Action: "POST",
	}); err != nil {
		t.Fatal(err)
	}
	close(repo.release)

	select {
	case verification := <-result:
		if !verification.valid || verification.err != nil {
			t.Fatalf("captured snapshot verification = (%v, %v), want valid", verification.valid, verification.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("verification did not finish")
	}
	if valid, err := baseChain.VerifyAuditIntegrityWithCheckpoint(ctx, companyID); !valid || err != nil {
		t.Fatalf("verification including appended record = (%v, %v), want valid", valid, err)
	}
}

func TestAuditSnapshotVerificationDetectsTampering(t *testing.T) {
	ctx := context.Background()
	repo := newSnapshotAuditRepo()
	chain := NewAuditChain(repo)
	companyID := types.NewCompanyID()
	if _, err := chain.Append(ctx, AuditLog{
		CompanyID: companyID, EntityType: "journal_entry",
		EntityID: types.NewJournalEntryID().String(), Action: "POST",
		Payload: []byte("original"),
	}); err != nil {
		t.Fatal(err)
	}

	repo.mu.Lock()
	repo.logs[companyID][0].Payload = []byte("tampered")
	repo.mu.Unlock()
	if valid, err := chain.VerifyAuditIntegrityWithCheckpoint(ctx, companyID); valid || !errors.Is(err, ErrAuditChainBroken) {
		t.Fatalf("tampered snapshot = (%v, %v), want broken chain", valid, err)
	}
}

func TestAuditSnapshotVerificationDetectsReorderedLogs(t *testing.T) {
	ctx := context.Background()
	repo := newSnapshotAuditRepo()
	chain := NewAuditChain(repo)
	companyID := types.NewCompanyID()
	for _, payload := range []string{"first", "second"} {
		if _, err := chain.Append(ctx, AuditLog{
			CompanyID: companyID, EntityType: "journal_entry",
			EntityID: types.NewJournalEntryID().String(), Action: "POST",
			Payload: []byte(payload),
		}); err != nil {
			t.Fatal(err)
		}
	}

	// Simulate an attacker with write access swapping two stored records while
	// leaving each record's own hash intact. The boundary still matches the
	// checkpoint, so the verifier must catch the broken sequence/links.
	repo.mu.Lock()
	repo.logs[companyID][0], repo.logs[companyID][1] = repo.logs[companyID][1], repo.logs[companyID][0]
	repo.mu.Unlock()

	if valid, err := chain.VerifyAuditIntegrityWithCheckpoint(ctx, companyID); valid || !errors.Is(err, ErrAuditChainBroken) {
		t.Fatalf("reordered snapshot = (%v, %v), want broken chain", valid, err)
	}
}

func TestAuditSnapshotVerificationDetectsMiddleDeletion(t *testing.T) {
	ctx := context.Background()
	repo := newSnapshotAuditRepo()
	chain := NewAuditChain(repo)
	companyID := types.NewCompanyID()
	for _, payload := range []string{"first", "middle", "last"} {
		if _, err := chain.Append(ctx, AuditLog{
			CompanyID: companyID, EntityType: "journal_entry",
			EntityID: types.NewJournalEntryID().String(), Action: "POST",
			Payload: []byte(payload),
		}); err != nil {
			t.Fatal(err)
		}
	}

	repo.mu.Lock()
	logs := repo.logs[companyID]
	repo.logs[companyID] = append(logs[:1], logs[2:]...)
	repo.mu.Unlock()

	if valid, err := chain.VerifyAuditIntegrity(ctx, companyID); valid || !errors.Is(err, ErrAuditChainBroken) {
		t.Fatalf("middle deletion, chain verification = (%v, %v), want broken chain", valid, err)
	}
	if valid, err := chain.VerifyAuditIntegrityWithCheckpoint(ctx, companyID); valid ||
		!errors.Is(err, ErrAuditCheckpointMismatch) {
		t.Fatalf("middle deletion, checkpoint verification = (%v, %v), want checkpoint mismatch", valid, err)
	}
}

func TestAuditSnapshotRejectsCheckpointHashMismatch(t *testing.T) {
	ctx := context.Background()
	repo := newSnapshotAuditRepo()
	chain := NewAuditChain(repo)
	companyID := types.NewCompanyID()
	if _, err := chain.Append(ctx, AuditLog{
		CompanyID: companyID, EntityType: "journal_entry",
		EntityID: types.NewJournalEntryID().String(), Action: "POST",
	}); err != nil {
		t.Fatal(err)
	}
	repo.mu.Lock()
	checkpoint := repo.checkpoints[companyID]
	repo.checkpoints[companyID] = AuditCheckpoint{
		CompanyID: checkpoint.CompanyID, SequenceNumber: checkpoint.SequenceNumber,
		Version: checkpoint.Version, LogHash: strings.Repeat("b", 64),
	}
	repo.mu.Unlock()

	if valid, err := chain.VerifyAuditIntegrityWithCheckpoint(ctx, companyID); valid ||
		!errors.Is(err, ErrAuditCheckpointMismatch) {
		t.Fatalf("checkpoint hash mismatch = (%v, %v), want checkpoint mismatch", valid, err)
	}
}

func TestAuditSnapshotVerificationIsSafeDuringConcurrentAppends(t *testing.T) {
	ctx := context.Background()
	repo := newSnapshotAuditRepo()
	chain := NewAuditChain(repo)
	companyID := types.NewCompanyID()
	// Establish the initial trusted checkpoint before concurrent verification
	// begins; without one the verifier must fail closed.
	if _, err := chain.Append(ctx, AuditLog{
		CompanyID: companyID, EntityType: "journal_entry", Action: "POST",
	}); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	appendErr := make(chan error, 1)
	verifyErr := make(chan error, 1)
	var wait sync.WaitGroup

	wait.Add(2)
	go func() {
		defer wait.Done()
		<-start
		for i := 0; i < 40; i++ {
			if _, err := chain.Append(ctx, AuditLog{
				CompanyID: companyID, EntityType: "journal_entry",
				EntityID: types.NewJournalEntryID().String(), Action: "POST",
			}); err != nil {
				appendErr <- err
				return
			}
		}
	}()
	go func() {
		defer wait.Done()
		<-start
		for i := 0; i < 40; i++ {
			if valid, err := chain.VerifyAuditIntegrityWithCheckpoint(ctx, companyID); !valid || err != nil {
				if err == nil {
					err = ErrAuditCheckpointMismatch
				}
				verifyErr <- err
				return
			}
		}
	}()
	close(start)
	wait.Wait()
	close(appendErr)
	close(verifyErr)
	for err := range appendErr {
		t.Fatal(err)
	}
	for err := range verifyErr {
		t.Fatal(err)
	}
	if valid, err := chain.VerifyAuditIntegrityWithCheckpoint(ctx, companyID); !valid || err != nil {
		t.Fatalf("final concurrent snapshot verification = (%v, %v), want valid", valid, err)
	}
}

func FuzzAuditSnapshotDetectsPayloadTampering(f *testing.F) {
	for _, payload := range [][]byte{
		nil,
		[]byte("posting"),
		[]byte(`{"amount":"100.00","currency":"USD"}`),
		{0, 1, 2, 255},
	} {
		f.Add(payload)
	}

	f.Fuzz(func(t *testing.T, payload []byte) {
		ctx := context.Background()
		repo := newSnapshotAuditRepo()
		chain := NewAuditChain(repo)
		companyID := types.NewCompanyID()
		if _, err := chain.Append(ctx, AuditLog{
			CompanyID:  companyID,
			EntityType: "journal_entry",
			EntityID:   types.NewJournalEntryID().String(),
			Action:     "POST",
			Payload:    payload,
		}); err != nil {
			t.Fatal(err)
		}

		repo.mu.Lock()
		repo.logs[companyID][0].Payload = append(append([]byte(nil), payload...), 0xff)
		repo.mu.Unlock()

		if valid, err := chain.VerifyAuditIntegrityWithCheckpoint(ctx, companyID); valid ||
			!errors.Is(err, ErrAuditChainBroken) {
			t.Fatalf("tampered payload verification = (%v, %v), want broken chain", valid, err)
		}
	})
}

func TestCheckpointVerificationFailsClosedWithoutTrustedCheckpoint(t *testing.T) {
	ctx := context.Background()
	companyID := types.NewCompanyID()
	repo := newSnapshotAuditRepo()
	chain := NewAuditChain(repo)

	// An empty chain without a checkpoint must not be reported as verified.
	if valid, err := chain.VerifyAuditIntegrityWithCheckpoint(ctx, companyID); valid ||
		!errors.Is(err, ErrAuditCheckpointMismatch) {
		t.Fatalf("empty chain without checkpoint = (%v, %v), want checkpoint mismatch", valid, err)
	}

	if _, err := chain.Append(ctx, AuditLog{
		CompanyID: companyID, EntityType: "journal_entry", Action: "POST",
	}); err != nil {
		t.Fatal(err)
	}
	// The in-memory fake advances a checkpoint on every append; simulate a
	// host that never persisted one outside the log.
	repo.mu.Lock()
	delete(repo.checkpoints, companyID)
	repo.mu.Unlock()
	if valid, err := chain.VerifyAuditIntegrityWithCheckpoint(ctx, companyID); valid ||
		!errors.Is(err, ErrAuditCheckpointMismatch) {
		t.Fatalf("non-empty chain without checkpoint = (%v, %v), want checkpoint mismatch", valid, err)
	}
}

func TestCheckpointVerificationRequiresSnapshotRepository(t *testing.T) {
	repo := nonAtomicAuditRepo{AuditLogRepository: newInMemoryAuditRepo()}
	chain := NewAuditChain(repo)
	if valid, err := chain.VerifyAuditIntegrityWithCheckpoint(context.Background(), types.NewCompanyID()); valid ||
		!errors.Is(err, ErrAuditSnapshotRepositoryRequired) {
		t.Fatalf("verification without snapshot repository = (%v, %v), want explicit repository error", valid, err)
	}
}
