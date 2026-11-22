package contract_parity

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/storage"
)

func phase11PolicyStore(t *testing.T, h *StorageAdapterHarness) storage.PolicyStore {
	t.Helper()
	raw, ok := h.Ports.Private[storage.PolicyStoreExtensionKey]
	if !ok {
		t.Fatal("policy store extension is not exposed")
	}
	store, ok := raw.(storage.PolicyStore)
	if !ok {
		t.Fatal("policy store extension has the wrong type")
	}
	return store
}

func phase11PolicyVersion(f UnifiedFixture, kind storage.PolicyKind, version int, start, end time.Time, payload string) storage.PolicyVersion {
	result := storage.PolicyVersion{
		CompanyID:     f.Company.ID,
		PolicyID:      uuid.New(),
		Kind:          kind,
		Version:       version,
		EffectiveFrom: start,
		CreatedBy:     "phase11-contract",
		ApprovedBy:    "phase11-contract",
		Payload:       []byte(payload),
	}
	if !end.IsZero() {
		result.EffectiveTo = &end
	}
	return result
}

// TestPhase11_PolicyVersionHistoryIsAppendOnlyAndNonOverlapping proves that
// both adapters preserve an immutable, company-scoped version history.
func TestPhase11_PolicyVersionHistoryIsAppendOnlyAndNonOverlapping(t *testing.T) {
	ForEachStorageAdapter(t, "PolicyVersionHistory", func(t *testing.T, h *StorageAdapterHarness, f UnifiedFixture) {
		ctx := phase8TestContext(t)
		store := phase11PolicyStore(t, h)
		start := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
		middle := time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC)
		endOfYear := time.Date(2027, time.January, 1, 0, 0, 0, 0, time.UTC)

		v1 := phase11PolicyVersion(f, storage.PolicyKindPosting, 1, start, middle, `{"max_backdating_days":0}`)
		if err := store.AppendVersion(ctx, v1); err != nil {
			t.Fatalf("append first version: %v", err)
		}

		overlap := phase11PolicyVersion(f, storage.PolicyKindPosting, 2, time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC), endOfYear, `{"max_backdating_days":5}`)
		if err := store.AppendVersion(ctx, overlap); !errors.Is(err, storage.ErrPolicyVersionConflict) {
			t.Fatalf("overlapping version error = %v, want ErrPolicyVersionConflict", err)
		}

		duplicateStart := phase11PolicyVersion(f, storage.PolicyKindPosting, 3, start, middle, `{"max_backdating_days":7}`)
		if err := store.AppendVersion(ctx, duplicateStart); !errors.Is(err, storage.ErrPolicyVersionConflict) {
			t.Fatalf("duplicate effective start error = %v, want ErrPolicyVersionConflict", err)
		}

		v2 := phase11PolicyVersion(f, storage.PolicyKindPosting, 4, middle, endOfYear, `{"max_backdating_days":10}`)
		if err := store.AppendVersion(ctx, v2); err != nil {
			t.Fatalf("append adjacent version: %v", err)
		}

		versions, err := store.ListVersions(ctx, f.Company.ID, storage.PolicyKindPosting)
		if err != nil {
			t.Fatalf("list versions: %v", err)
		}
		if len(versions) != 2 {
			t.Fatalf("version count = %d, want 2", len(versions))
		}
		if versions[0].Version != 1 || versions[1].Version != 4 {
			t.Fatalf("versions not ordered by effective date: %+v", versions)
		}
		// Payloads are canonical JSON documents, not opaque bytes: PostgreSQL
		// stores them as jsonb and may normalise whitespace and key order.
		assertPolicyPayload := func(got, want []byte) {
			t.Helper()
			var gotValue, wantValue any
			if err := json.Unmarshal(got, &gotValue); err != nil {
				t.Fatalf("decode stored payload: %v", err)
			}
			if err := json.Unmarshal(want, &wantValue); err != nil {
				t.Fatalf("decode submitted payload: %v", err)
			}
			if !reflect.DeepEqual(gotValue, wantValue) {
				t.Fatalf("stored policy payload was rewritten: got %s, want %s", got, want)
			}
		}
		assertPolicyPayload(versions[0].Payload, v1.Payload)
		assertPolicyPayload(versions[1].Payload, v2.Payload)
	})
}

// TestPhase11_PolicyVersionCompanyScope proves tenant isolation of the
// append-only policy history on both adapters.
func TestPhase11_PolicyVersionCompanyScope(t *testing.T) {
	ForEachStorageAdapter(t, "PolicyVersionCompanyScope", func(t *testing.T, h *StorageAdapterHarness, f UnifiedFixture) {
		ctx := phase8TestContext(t)
		store := phase11PolicyStore(t, h)
		other := phase8SecondCompany(t, ctx, h)
		start := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
		version := phase11PolicyVersion(f, storage.PolicyKindCurrency, 1, start, time.Time{},
			`{"functional_currency":"USD","rounding_method":"bankers"}`)

		if err := store.AppendVersion(ctx, version); err != nil {
			t.Fatalf("append version: %v", err)
		}
		foreign, err := store.ListVersions(ctx, other.Company.ID, storage.PolicyKindCurrency)
		if err != nil {
			t.Fatalf("list other company versions: %v", err)
		}
		if len(foreign) != 0 {
			t.Fatalf("policy version leaked into another company: %+v", foreign)
		}
	})
}
