package posting_test

import (
	"reflect"
	"sync"
	"testing"

	"github.com/osmai4603-collab/accounting_engine/internal/kernel/journal"
	"github.com/osmai4603-collab/accounting_engine/internal/kernel/posting"
	"github.com/osmai4603-collab/accounting_engine/internal/kernel/types"
)

func lockOrderEntry(companyID types.CompanyID, accountIDs ...types.AccountID) *journal.JournalEntry {
	entry := &journal.JournalEntry{CompanyID: companyID}
	for _, accountID := range accountIDs {
		entry.Lines = append(entry.Lines, journal.EntryLine{AccountID: accountID})
	}
	return entry
}

// TestOrderedAccountKeysAreDeterministicAndScoped proves that the kernel owns
// ordering only: identical sets of touched accounts produce identical keys in
// identical order regardless of caller line order, and the company is part of
// every key so two tenants can never collide on one lock identity (Phase 8,
// tasks 1-3 and G-002).
func TestOrderedAccountKeysAreDeterministicAndScoped(t *testing.T) {
	company := types.NewCompanyID()
	otherCompany := types.NewCompanyID()
	first := types.NewAccountID()
	second := types.NewAccountID()
	third := types.NewAccountID()

	forward := lockOrderEntry(company, first, second, third)
	reversed := lockOrderEntry(company, third, first, second, first)
	otherTenant := lockOrderEntry(otherCompany, first, second, third)

	want := posting.OrderedAccountKeys(forward)
	if len(want) != 3 {
		t.Fatalf("ordered keys = %d, want 3 unique accounts: %v", len(want), want)
	}
	if got := posting.OrderedAccountKeys(reversed); !reflect.DeepEqual(got, want) {
		t.Fatalf("line order changed lock order:\n got %v\nwant %v", got, want)
	}
	if got := posting.OrderedAccountKeys(otherTenant); reflect.DeepEqual(got, want) {
		t.Fatal("company scope is missing from lock keys; tenants would collide")
	}
	for i := 1; i < len(want); i++ {
		if want[i-1] >= want[i] {
			t.Fatalf("lock keys are not strictly ascending at %d: %v", i, want)
		}
	}
	if got := posting.ExtractAndSortAccountIDs(reversed); len(got) != 3 {
		t.Fatalf("ExtractAndSortAccountIDs = %v, want 3 unique ids", got)
	}
	if got := posting.OrderedAccountKeys(forward); !reflect.DeepEqual(got, posting.OrderedAccountKeys(forward)) {
		t.Fatal("OrderedAccountKeys is not repeatable")
	}
	if len(forward.Lines) != 3 || forward.Lines[0].AccountID != first {
		t.Fatal("lock ordering mutated the caller's entry")
	}
}

func TestOrderedAccountKeysHandlesEmptyEntries(t *testing.T) {
	if got := posting.OrderedAccountKeys(nil); len(got) != 0 {
		t.Fatalf("nil entry produced lock keys: %v", got)
	}
	if got := posting.OrderedAccountKeys(&journal.JournalEntry{CompanyID: types.NewCompanyID()}); len(got) != 0 {
		t.Fatalf("entry without lines produced lock keys: %v", got)
	}
	withoutCompany := lockOrderEntry(types.CompanyID{}, types.NewAccountID())
	if got := posting.OrderedAccountKeys(withoutCompany); len(got) != 0 {
		t.Fatalf("entry without company scope produced lock keys: %v", got)
	}
}

// TestOrderedAccountKeysAreStatelessUnderConcurrency is a race-sensitive check
// that the ordering helper cannot accidentally share a cache or buffer across
// concurrent postings.
func TestOrderedAccountKeysAreStatelessUnderConcurrency(t *testing.T) {
	company := types.NewCompanyID()
	accounts := []types.AccountID{types.NewAccountID(), types.NewAccountID(), types.NewAccountID()}
	want := posting.OrderedAccountKeys(lockOrderEntry(company, accounts...))

	var wg sync.WaitGroup
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func(offset int) {
			defer wg.Done()
			rotated := []types.AccountID{accounts[offset%3], accounts[(offset+1)%3], accounts[(offset+2)%3]}
			got := posting.OrderedAccountKeys(lockOrderEntry(company, rotated...))
			if !reflect.DeepEqual(got, want) {
				t.Errorf("concurrent lock order = %v, want %v", got, want)
			}
		}(i)
	}
	wg.Wait()
}
