package posting

import (
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/ledger"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
)

func TestExtractAndSortAccountIDs(t *testing.T) {
	acc1 := types.AccountID(uuid.MustParse("33333333-3333-3333-3333-333333333333"))
	acc2 := types.AccountID(uuid.MustParse("11111111-1111-1111-1111-111111111111"))
	acc3 := types.AccountID(uuid.MustParse("22222222-2222-2222-2222-222222222222"))

	entry := &ledger.JournalEntry{
		Lines: []ledger.EntryLine{
			{AccountID: acc1},
			{AccountID: acc2},
			{AccountID: acc3},
			{AccountID: acc1}, // duplicate to test uniqueness
		},
	}

	sorted := ExtractAndSortAccountIDs(entry)
	if len(sorted) != 3 {
		t.Fatalf("expected 3 unique accounts, got %d", len(sorted))
	}

	if sorted[0] != acc2 || sorted[1] != acc3 || sorted[2] != acc1 {
		t.Fatalf("expected ordered [%s, %s, %s], got [%s, %s, %s]",
			acc2, acc3, acc1, sorted[0], sorted[1], sorted[2])
	}
}

func TestConcurrentLockManagerNoDeadlock(t *testing.T) {
	// Simulate 100 concurrent workers locking intersecting accounts in deterministic order
	accA := types.AccountID(uuid.MustParse("11111111-1111-1111-1111-111111111111"))
	accB := types.AccountID(uuid.MustParse("22222222-2222-2222-2222-222222222222"))
	accC := types.AccountID(uuid.MustParse("33333333-3333-3333-3333-333333333333"))

	// Mutexes per account ID
	accountLocks := map[types.AccountID]*sync.Mutex{
		accA: {},
		accB: {},
		accC: {},
	}

	const workers = 100
	var wg sync.WaitGroup
	wg.Add(workers)

	for i := 0; i < workers; i++ {
		go func(workerID int) {
			defer wg.Done()

			// Alternating access patterns: some do A then B, some do B then A, some do C then A
			var entry *ledger.JournalEntry
			if workerID%3 == 0 {
				entry = &ledger.JournalEntry{
					Lines: []ledger.EntryLine{{AccountID: accB}, {AccountID: accA}},
				}
			} else if workerID%3 == 1 {
				entry = &ledger.JournalEntry{
					Lines: []ledger.EntryLine{{AccountID: accA}, {AccountID: accC}},
				}
			} else {
				entry = &ledger.JournalEntry{
					Lines: []ledger.EntryLine{{AccountID: accC}, {AccountID: accB}},
				}
			}

			// Must always lock according to ExtractAndSortAccountIDs
			orderedAccounts := ExtractAndSortAccountIDs(entry)
			for _, acc := range orderedAccounts {
				accountLocks[acc].Lock()
			}

			// Critical section
			// Unlock in reverse or same order
			for i := len(orderedAccounts) - 1; i >= 0; i-- {
				accountLocks[orderedAccounts[i]].Unlock()
			}
		}(i)
	}

	wg.Wait()
}

