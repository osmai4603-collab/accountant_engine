package posting

import (
	"sort"

	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/ledger"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
)

// ExtractAndSortAccountIDs extracts all unique account IDs touched by a journal entry
// and sorts them in ascending deterministic order (AccountID ASC).
// This guarantees deadlock-free row locking across concurrent transactions.
func ExtractAndSortAccountIDs(entry *ledger.JournalEntry) []types.AccountID {
	if entry == nil || len(entry.Lines) == 0 {
		return nil
	}

	seen := make(map[types.AccountID]struct{}, len(entry.Lines))
	unique := make([]types.AccountID, 0, len(entry.Lines))

	for _, line := range entry.Lines {
		if line.AccountID == (types.AccountID{}) {
			continue
		}
		if _, ok := seen[line.AccountID]; !ok {
			seen[line.AccountID] = struct{}{}
			unique = append(unique, line.AccountID)
		}
	}

	sort.Slice(unique, func(i, j int) bool {
		return unique[i].String() < unique[j].String()
	})

	return unique
}

