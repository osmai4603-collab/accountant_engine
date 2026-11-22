package posting

import (
	"sort"

	"github.com/osmai4603-collab/accounting_engine/internal/kernal/journal"
	"github.com/osmai4603-collab/accounting_engine/internal/kernal/types"
)

// Lock key ordering is a domain decision, not lock acquisition. The kernel
// produces canonical, deterministic identities for everything a posting
// touches; the storage adapter decides how to acquire them (row locks,
// advisory locks, or an equivalent process lock) inside its own transaction.
// This package never blocks, never stores lock state, and never calls a
// database: treating an ordered key list as an actual lock is the G-002 defect
// closed in Phase 8.

// AccountLockKey is the canonical, company-scoped identity of one account in
// the locking contract. The same representation is used by the SPI lock target
// so a kernel-produced order and an adapter-acquired order can be compared.
func AccountLockKey(companyID types.CompanyID, accountID types.AccountID) string {
	return "v1|company=" + companyID.String() + "|kind=account|id=" + accountID.String()
}

// ExtractAndSortAccountIDs extracts all unique account IDs touched by a journal
// entry and sorts them in ascending deterministic order (AccountID ASC).
// It only computes an order; it does not acquire a lock or guarantee that any
// adapter has applied the order. Adapter enforcement is the Phase 8 Locking
// Contract in pkg/spi/storage.
func ExtractAndSortAccountIDs(entry *journal.JournalEntry) []types.AccountID {
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

// OrderedAccountKeys returns the canonical, unique, sorted lock identities of
// every account touched by entry. Caller line order cannot change the result;
// a missing company scope yields no keys so an unscoped entry can never claim
// a shared lock. The returned slice is freshly allocated on every call.
func OrderedAccountKeys(entry *journal.JournalEntry) []string {
	if entry == nil || entry.CompanyID == (types.CompanyID{}) || len(entry.Lines) == 0 {
		return nil
	}
	accountIDs := ExtractAndSortAccountIDs(entry)
	keys := make([]string, 0, len(accountIDs))
	for _, accountID := range accountIDs {
		keys = append(keys, AccountLockKey(entry.CompanyID, accountID))
	}
	sort.Strings(keys)
	return keys
}
