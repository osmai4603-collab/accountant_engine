package contract_parity

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/osmai4603-collab/accounting_engine/pkg/spi/storage"
	"github.com/osmai4603-collab/accounting_engine/storage/memory"
	"github.com/shopspring/decimal"
)

// TestPhase11_PortCatalogCoversEveryPort proves that the reference list is
// complete with respect to the public Ports struct. A port added without a
// catalog entry is a documentation and conformance gap.
func TestPhase11_PortCatalogCoversEveryPort(t *testing.T) {
	catalog := storage.PortCatalog()
	listed := make(map[string]storage.PortDefinition, len(catalog))
	for _, definition := range catalog {
		if definition.Name == "" || definition.Description == "" {
			t.Fatalf("incomplete port catalog entry: %+v", definition)
		}
		if _, duplicate := listed[definition.Name]; duplicate {
			t.Fatalf("duplicate port catalog entry %q", definition.Name)
		}
		listed[definition.Name] = definition
	}

	portType := reflect.TypeOf(storage.Ports{})
	for i := 0; i < portType.NumField(); i++ {
		name := portType.Field(i).Name
		if _, ok := listed[name]; !ok {
			t.Errorf("storage.Ports.%s is missing from PortCatalog", name)
		}
	}
}

// TestPhase11_PortsValidateChecksEveryRequiredPort removes each required port
// in turn from an otherwise valid set and requires Validate to name it.
func TestPhase11_PortsValidateChecksEveryRequiredPort(t *testing.T) {
	base := memory.New().Ports()
	if err := base.Validate(); err != nil {
		t.Fatalf("baseline ports invalid: %v", err)
	}

	expectedPhrase := map[string]string{
		"Drafts":     "draft store",
		"Entries":    "entry reader",
		"Posting":    "posting writer",
		"Sequences":  "sequence allocator",
		"MasterData": "master data reader",
	}
	fields := reflect.ValueOf(&base).Elem()
	for _, definition := range storage.PortCatalog() {
		if !definition.Required {
			continue
		}
		field := fields.FieldByName(definition.Name)
		if !field.IsValid() {
			t.Fatalf("required catalog port %q is not a Ports field", definition.Name)
		}
		field.Set(reflect.Zero(field.Type()))
		err := base.Validate()
		if err == nil {
			t.Errorf("Validate accepted nil %s", definition.Name)
		} else if phrase := expectedPhrase[definition.Name]; phrase != "" && !strings.Contains(strings.ToLower(err.Error()), phrase) {
			t.Errorf("Validate error %q does not identify %s", err, definition.Name)
		}
		field.Set(reflect.ValueOf(memory.New().Ports()).FieldByName(definition.Name))
	}
}

func TestPhase11_ContractVersionRejectsIncompatibleAdapters(t *testing.T) {
	if err := storage.ValidateContractVersion(storage.StorageContractVersion); err != nil {
		t.Fatalf("current contract version rejected: %v", err)
	}
	for _, version := range []string{"", "storage.spi/v0", "storage.spi/v2", "STORAGE.SPI/V1"} {
		if err := storage.ValidateContractVersion(version); err == nil {
			t.Errorf("incompatible contract version %q accepted", version)
		}
	}

	ForEachStorageAdapter(t, "AdapterCapabilitiesValidated", func(t *testing.T, h *StorageAdapterHarness, f UnifiedFixture) {
		if err := storage.ValidateAdapterCapabilities(h.Capabilities); err != nil {
			t.Fatalf("adapter capabilities rejected: %v", err)
		}
		bad := h.Capabilities
		bad.ContractVersion = "storage.spi/v2"
		if err := storage.ValidateAdapterCapabilities(bad); err == nil {
			t.Fatal("mismatched contract version accepted")
		}
	})
}

type phase11TransientOnceRunner struct {
	inner    storage.UnitOfWorkRunner
	attempts int
}

func (r *phase11TransientOnceRunner) Execute(ctx context.Context, fn func(context.Context, storage.UnitOfWork) error) error {
	return r.ExecuteWithOptions(ctx, storage.TxOptions{}, fn)
}

func (r *phase11TransientOnceRunner) ExecuteWithOptions(
	ctx context.Context,
	opts storage.TxOptions,
	fn func(context.Context, storage.UnitOfWork) error,
) error {
	r.attempts++
	if r.attempts == 1 {
		return storage.ErrSerializationFailure
	}
	return r.inner.ExecuteWithOptions(ctx, opts, fn)
}

// TestPhase11_BoundedRetryOnAdapterUnitOfWork proves that a transient conflict
// is retried by the host retry policy and that the committed effect is applied
// exactly once by the adapter's unit of work.
func TestPhase11_BoundedRetryOnAdapterUnitOfWork(t *testing.T) {
	ForEachStorageAdapter(t, "BoundedRetryOnUnitOfWork", func(t *testing.T, h *StorageAdapterHarness, f UnifiedFixture) {
		ctx := storage.WithCompanyContext(phase8TestContext(t), f.Company.ID)
		runner := &phase11TransientOnceRunner{inner: h.Ports.UOWRunner}
		commit := phase7PostingCommit(f, "p11-retry-key", "p11-retry-hash")

		err := storage.ExecuteWithRetry(ctx, runner, storage.TxOptions{}, storage.RetryPolicy{
			MaxRetries:     1,
			InitialBackoff: time.Millisecond,
			MaxBackoff:     5 * time.Millisecond,
		}, func(ctx context.Context, uow storage.UnitOfWork) error {
			_, err := uow.Posting().CommitPosting(ctx, commit)
			return err
		})
		if err != nil {
			t.Fatalf("bounded retry failed: %v", err)
		}
		if runner.attempts != 2 {
			t.Fatalf("attempts = %d, want 2", runner.attempts)
		}

		stored, err := h.Ports.Entries.GetEntry(ctx, f.Company.ID, commit.Entry.ID)
		if err != nil {
			t.Fatalf("read committed entry: %v", err)
		}
		if stored.ID != commit.Entry.ID {
			t.Fatalf("stored entry %s != committed %s", stored.ID, commit.Entry.ID)
		}
		phase7RequireBalance(t, ctx, h, f, "100")
	})
}

// TestPhase11_ConnectionInterruptionBeforeCommit models a lost connection
// after all writes are staged but before commit. No accounting effect may
// survive, and the idempotency key must remain safely retryable.
func TestPhase11_ConnectionInterruptionBeforeCommit(t *testing.T) {
	ForEachStorageAdapter(t, "ConnectionInterruptionBeforeCommit", func(t *testing.T, h *StorageAdapterHarness, f UnifiedFixture) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		faultCtx := storage.WithFaultInjector(ctx, func(point storage.FaultPoint) error {
			if point == storage.FaultBeforeCommit {
				cancel()
				return context.Canceled
			}
			return nil
		})
		commit := phase7PostingCommit(f, "p11-connection-key", "p11-connection-hash")

		if _, err := h.Ports.Posting.CommitPosting(faultCtx, commit); !errors.Is(err, context.Canceled) {
			t.Fatalf("interrupted commit error = %v, want context.Canceled", err)
		}
		if _, err := h.Ports.Entries.GetEntry(context.Background(), f.Company.ID, commit.Entry.ID); err == nil {
			t.Fatal("interrupted commit persisted an entry")
		}
		phase7RequireBalance(t, context.Background(), h, f, "0")
		if _, found := phase8Lookup(t, context.Background(), h, f.Company.ID, commit.Idempotency.Key); found {
			t.Fatal("interrupted commit persisted an idempotency record")
		}

		if _, err := h.Ports.Posting.CommitPosting(context.Background(), commit); err != nil {
			t.Fatalf("retry after interruption: %v", err)
		}
		phase7RequireBalance(t, context.Background(), h, f, "100")
	})
}

// TestPhase11_CompanyScopeOnEveryMasterDataPort closes the gap between the
// generic Entry/Balance scope test and the individual master-data methods.
func TestPhase11_CompanyScopeOnEveryMasterDataPort(t *testing.T) {
	ForEachStorageAdapter(t, "CompanyScopeOnEveryMasterDataPort", func(t *testing.T, h *StorageAdapterHarness, f UnifiedFixture) {
		ctx := phase8TestContext(t)
		other := phase8SecondCompany(t, ctx, h)
		reader := h.Ports.MasterData

		if _, err := reader.GetCompany(ctx, other.Company.ID); err == nil {
			t.Error("GetCompany leaked another company")
		}
		if _, err := reader.GetAccount(ctx, other.Company.ID, f.CashAccount.ID); err == nil {
			t.Error("GetAccount leaked an account into another company")
		}
		if _, err := reader.GetAccountByCode(ctx, other.Company.ID, f.CashAccount.Code); err == nil {
			t.Error("GetAccountByCode leaked an account into another company")
		}
		if _, err := reader.GetJournalByCode(ctx, other.Company.ID, f.Journal.Code); err == nil {
			t.Error("GetJournalByCode leaked a journal into another company")
		}
		if has, err := reader.HasAccountChildren(ctx, other.Company.ID, f.CashAccount.ID); err == nil && has {
			t.Error("HasAccountChildren leaked an account into another company")
		}
		if _, err := reader.GetFiscalYear(ctx, other.Company.ID, f.FiscalYear.ID); err == nil {
			t.Error("GetFiscalYear leaked a fiscal year into another company")
		}
		if _, err := reader.GetPeriodForDate(ctx, other.Company.ID, f.OpenPeriod.StartDate); err == nil {
			t.Error("GetPeriodForDate leaked a period into another company")
		}
		if _, err := h.Ports.Balance.GetAccountBalance(ctx, other.Company.ID, f.CashAccount.ID, f.OpenPeriod.StartDate, f.FunctionalCurrency); err != nil {
			t.Fatalf("foreign company balance: %v", err)
		}
	})
}

// TestPhase11_BalanceReaderAsOfAndCurrencyParity proves that both adapters
// interpret asOf and an empty currency deterministically and identically.
func TestPhase11_BalanceReaderAsOfAndCurrencyParity(t *testing.T) {
	ForEachStorageAdapter(t, "BalanceReaderAsOfAndCurrency", func(t *testing.T, h *StorageAdapterHarness, f UnifiedFixture) {
		ctx := phase8TestContext(t)
		early := phase7PostingCommit(f, "p11-balance-early", "p11-balance-early-hash")
		early.Entry.PostingDate = time.Date(2026, time.January, 5, 0, 0, 0, 0, time.UTC)
		late := phase7PostingCommit(f, "p11-balance-late", "p11-balance-late-hash")
		late.Entry.PostingDate = time.Date(2026, time.January, 20, 0, 0, 0, 0, time.UTC)
		late.Entry.SequenceNumber = 2
		late.Entry.EntryNumber = "P11-late"

		for _, commit := range []storage.PostingCommit{early, late} {
			if _, err := h.Ports.Posting.CommitPosting(ctx, commit); err != nil {
				t.Fatalf("commit %s: %v", commit.Idempotency.Key, err)
			}
		}

		cases := []struct {
			name     string
			asOf     time.Time
			currency string
			want     string
		}{
			{name: "before first posting", asOf: time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC), currency: f.FunctionalCurrency, want: "0"},
			{name: "between postings", asOf: time.Date(2026, time.January, 10, 0, 0, 0, 0, time.UTC), currency: f.FunctionalCurrency, want: "100"},
			{name: "after both postings", asOf: time.Date(2026, time.January, 25, 0, 0, 0, 0, time.UTC), currency: f.FunctionalCurrency, want: "200"},
			{name: "empty currency resolves to functional", asOf: time.Date(2026, time.January, 10, 0, 0, 0, 0, time.UTC), currency: "", want: "100"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				got, err := h.Ports.Balance.GetAccountBalance(ctx, f.Company.ID, f.CashAccount.ID, tc.asOf, tc.currency)
				if err != nil {
					t.Fatalf("GetAccountBalance: %v", err)
				}
				if want := decimal.RequireFromString(tc.want); !got.Equal(want) {
					t.Fatalf("balance = %s, want %s", got.String(), want.String())
				}
			})
		}
	})
}
