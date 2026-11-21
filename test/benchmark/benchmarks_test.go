package benchmark

import (
	"context"
	"testing"
	"time"

	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/osmai4603-collab/accounting_engine/pkg/sdk"
)

// The certified baseline is PostgreSQL only. Workloads that require a
// host-provided ReportingStore or YearEndClosing capability are reported as
// not certified for this engine build rather than silently substituted with
// in-memory measurements.

func BenchmarkDraftCreation(b *testing.B) {
	h := newPostgresBench(b)
	defer h.cleanup()
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := h.ports.Drafts.SaveDraft(ctx, h.newEntry(b)); err != nil {
			b.Fatalf("create benchmark draft: %v", err)
		}
	}
}

func BenchmarkPostingPipeline(b *testing.B) {
	h := newPostgresBench(b)
	defer h.cleanup()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.post(b)
	}
}

func BenchmarkConcurrentPosting(b *testing.B) {
	h := newPostgresBench(b)
	defer h.cleanup()
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if !h.postParallel(b) {
				return
			}
		}
	})
}

func BenchmarkBalanceRetrieval(b *testing.B) {
	h := newPostgresBench(b)
	defer h.cleanup()
	ctx := context.Background()
	asOf := time.Date(2026, time.January, 31, 0, 0, 0, 0, time.UTC)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := h.ports.Balance.GetAccountBalance(ctx, h.fixture.Company.ID, h.fixture.CashAccount.ID, asOf, "USD"); err != nil {
			b.Fatalf("benchmark balance retrieval: %v", err)
		}
	}
}

func BenchmarkCashFlowProxy(b *testing.B) {
	// Cash-flow statements require a host-provided ReportingStore, which this
	// engine build does not ship. The certified proxy measures the underlying
	// balance-read path that a reporting adapter would consume.
	b.Skip("host-provided ReportingStore not shipped; cash-flow SLO not certifiable in this build")
	h := newPostgresBench(b)
	defer h.cleanup()
	ctx := context.Background()
	asOf := time.Date(2026, time.January, 31, 0, 0, 0, 0, time.UTC)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := h.ports.Balance.GetAccountBalance(ctx, h.fixture.Company.ID, h.fixture.RevenueAccount.ID, asOf, "USD"); err != nil {
			b.Fatalf("benchmark cash-flow proxy: %v", err)
		}
	}
}

func BenchmarkYearEndClosing(b *testing.B) {
	// YearEndClosing is an optional public capability supplied by the host.
	// Report the absence explicitly instead of fabricating a measurement.
	b.Skip("host-provided YearEndClosing service not shipped; closing SLO not certifiable in this build")
	h := newPostgresBench(b)
	defer h.cleanup()
	if _, err := sdk.YearEndClosing(h.engine); err != nil {
		b.Fatalf("year-end closing capability: %v", err)
	}
}

func BenchmarkTrialBalanceProxy(b *testing.B) {
	// Trial balance is exposed through the optional ReportingStore. This proxy
	// exercises the public BalanceReader for all provisioned accounts.
	h := newPostgresBench(b)
	defer h.cleanup()
	ctx := context.Background()
	asOf := time.Date(2026, time.January, 31, 0, 0, 0, 0, time.UTC)
	accounts := []types.AccountID{
		h.fixture.CashAccount.ID, h.fixture.RevenueAccount.ID,
		h.fixture.EquityAccount.ID, h.fixture.ExpenseAccount.ID,
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, accountID := range accounts {
			if _, err := h.ports.Balance.GetAccountBalance(ctx, h.fixture.Company.ID, accountID, asOf, "USD"); err != nil {
				b.Fatalf("benchmark trial-balance proxy: %v", err)
			}
		}
	}
}
