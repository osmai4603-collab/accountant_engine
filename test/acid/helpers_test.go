package acid

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/osmai4603-collab/accounting_engine/pkg/sdk"
	publicapplication "github.com/osmai4603-collab/accounting_engine/pkg/spi/application"
	publicmasterdata "github.com/osmai4603-collab/accounting_engine/pkg/spi/masterdata"
	"github.com/osmai4603-collab/accounting_engine/storage/memory"
)

type acidFixture struct {
	engine    sdk.Engine
	app       publicapplication.Service
	companyID types.CompanyID
	journalID types.JournalID
	cashID    types.AccountID
	revenueID types.AccountID
	periodID  types.FiscalPeriodID
	asOf      time.Time
	driver    *memory.Adapter
}

// newAcidFixture provisions the minimum public master data through SPI ports
// only. It is intentionally independent of any concrete kernel helper so the
// acid suite proves infrastructure independence.
func newAcidFixture(t *testing.T, opts ...sdk.Option) acidFixture {
	t.Helper()
	ctx := context.Background()
	driver := memory.New()
	ports := driver.Ports()
	md := driver.MasterDataStore()

	f := acidFixture{
		companyID: types.NewCompanyID(),
		journalID: types.JournalID(publicmasterdata.NewJournalID()),
		cashID:    types.NewAccountID(),
		revenueID: types.NewAccountID(),
		periodID:  types.NewFiscalPeriodID(),
		asOf:      time.Date(2026, time.January, 15, 0, 0, 0, 0, time.UTC),
		driver:    driver,
	}

	if err := md.CreateCompany(ctx, publicmasterdata.Company{
		ID: f.companyID, Code: "ACID-" + uuid.NewString()[:8], LegalName: "Acid Test Co",
		FunctionalCurrency: "USD", PresentationCurrency: "EUR", Active: true,
	}); err != nil {
		t.Fatalf("setup company: %v", err)
	}
	if err := md.CreateJournal(ctx, publicmasterdata.Journal{
		ID: publicmasterdata.JournalID(f.journalID), CompanyID: f.companyID,
		Code: "GENERAL", Name: "General Journal",
	}); err != nil {
		t.Fatalf("setup journal: %v", err)
	}
	for _, account := range []publicmasterdata.Account{
		{
			ID: f.cashID, CompanyID: f.companyID, Code: "1000", Name: "Cash",
			Type: publicmasterdata.AccountTypeAsset, NormalBalance: publicmasterdata.NormalBalanceDebit,
			Currency: "USD", IsActive: true,
		},
		{
			ID: f.revenueID, CompanyID: f.companyID, Code: "4000", Name: "Revenue",
			Type: publicmasterdata.AccountTypeIncome, NormalBalance: publicmasterdata.NormalBalanceCredit,
			Currency: "USD", IsActive: true,
		},
	} {
		if err := md.CreateAccount(ctx, account); err != nil {
			t.Fatalf("setup account %s: %v", account.Code, err)
		}
	}
	yearID := types.NewFiscalYearID()
	start := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	if err := md.CreateFiscalYear(ctx, publicmasterdata.FiscalYear{
		ID: yearID, CompanyID: f.companyID, Name: "FY2026",
		StartDate: start, EndDate: time.Date(2026, time.December, 31, 0, 0, 0, 0, time.UTC),
		State: publicmasterdata.FiscalYearOpen,
	}); err != nil {
		t.Fatalf("setup fiscal year: %v", err)
	}
	if err := md.CreateFiscalPeriod(ctx, publicmasterdata.FiscalPeriod{
		ID: f.periodID, CompanyID: f.companyID, FiscalYearID: yearID,
		Number: 1, Name: "January 2026", StartDate: start,
		EndDate: time.Date(2026, time.January, 31, 0, 0, 0, 0, time.UTC),
		State:   publicmasterdata.PeriodOpen, Type: publicmasterdata.FiscalPeriodTypeNormal,
	}); err != nil {
		t.Fatalf("setup fiscal period: %v", err)
	}

	// Reporting and governance stores are host-provided capabilities in this
	// codebase; the memory adapter only supplies the core storage ports. The
	// SDK deliberately degrades those services to ErrCapabilityNotImplemented,
	// so the ACID suite asserts ledger/event behaviour only.
	allOpts := append([]sdk.Option{
		sdk.WithStoragePorts(ports),
	}, opts...)
	engine, err := sdk.NewBuilder(allOpts...).Build(ctx)
	if err != nil {
		t.Fatalf("build engine: %v", err)
	}
	t.Cleanup(func() { _ = engine.Close() })
	f.engine = engine

	// The public application service is used for reversal because the SDK's
	// convenience ReverseEntry wrapper does not forward an idempotency key to
	// the posting pipeline (recorded as a P7-04 integration gap).
	app, err := sdk.NewApplicationService(ports)
	if err != nil {
		t.Fatalf("build application service: %v", err)
	}
	f.app = app
	return f
}

func acidPostContext(label string) context.Context {
	return publicapplication.WithRequestMetadata(context.Background(), publicapplication.RequestMetadata{
		CorrelationID:  "p7-acid",
		IdempotencyKey: label + "-" + uuid.NewString(),
		SourceSystem:   "p7-acid-test",
	})
}
