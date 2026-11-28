package memory_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	publicgovernance "github.com/osmai4603-collab/accounting_engine/pkg/api/v1/governance"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/journal"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/ledger"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	publicevents "github.com/osmai4603-collab/accounting_engine/pkg/spi/events"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/masterdata"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/storage"
	"github.com/osmai4603-collab/accounting_engine/storage/memory"
	"github.com/shopspring/decimal"
)

func TestMemoryDriverComprehensiveCoverage(t *testing.T) {
	ctx := context.Background()
	adapter := memory.New()
	defer adapter.Close()
	d := adapter.Driver()
	ports := adapter.Ports()

	companyID := types.NewCompanyID()
	journalID := masterdata.NewJournalID()
	cashID := types.NewAccountID()
	childAccID := types.NewAccountID()
	revID := types.NewAccountID()
	fyID := types.FiscalYearID(uuid.New())
	fpID := types.FiscalPeriodID(uuid.New())
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 12, 31, 23, 59, 59, 0, time.UTC)

	if err := d.CreateCompany(ctx, masterdata.Company{
		ID: companyID, Code: "MEM-1", LegalName: "Mem Co", FunctionalCurrency: "USD", PresentationCurrency: "USD", Active: true,
	}); err != nil {
		t.Fatalf("CreateCompany: %v", err)
	}
	if err := d.CreateJournal(ctx, masterdata.Journal{ID: journalID, CompanyID: companyID, Code: "GEN", Name: "General"}); err != nil {
		t.Fatalf("CreateJournal: %v", err)
	}
	if err := d.CreateAccount(ctx, masterdata.Account{ID: cashID, CompanyID: companyID, Code: "1000", Name: "Cash", Type: masterdata.AccountTypeAsset, NormalBalance: masterdata.NormalBalanceDebit, Currency: "USD", IsActive: true, EffectiveFrom: start}); err != nil {
		t.Fatalf("CreateAccount cash: %v", err)
	}
	if err := d.CreateAccount(ctx, masterdata.Account{ID: childAccID, CompanyID: companyID, Code: "1001", Name: "Petty Cash", Type: masterdata.AccountTypeAsset, NormalBalance: masterdata.NormalBalanceDebit, Currency: "USD", IsActive: true, ParentID: &cashID, EffectiveFrom: start}); err != nil {
		t.Fatalf("CreateAccount child: %v", err)
	}
	if err := d.CreateAccount(ctx, masterdata.Account{ID: revID, CompanyID: companyID, Code: "4000", Name: "Revenue", Type: masterdata.AccountTypeIncome, NormalBalance: masterdata.NormalBalanceCredit, Currency: "USD", IsActive: true, EffectiveFrom: start}); err != nil {
		t.Fatalf("CreateAccount rev: %v", err)
	}
	if err := d.CreateFiscalYear(ctx, masterdata.FiscalYear{ID: fyID, CompanyID: companyID, Name: "FY26", StartDate: start, EndDate: end, State: masterdata.FiscalYearOpen}); err != nil {
		t.Fatalf("CreateFiscalYear: %v", err)
	}
	if err := d.CreateFiscalPeriod(ctx, masterdata.FiscalPeriod{ID: fpID, CompanyID: companyID, FiscalYearID: fyID, Number: 1, Name: "P1", StartDate: start, EndDate: end, State: masterdata.PeriodOpen, Type: masterdata.FiscalPeriodTypeNormal}); err != nil {
		t.Fatalf("CreateFiscalPeriod: %v", err)
	}

	// Masterdata extras: HasAccountChildren, GetPeriodForDate, SaveAccountVersion, GetAccountAsOf, CostCenter, AnalyticalDimension
	hasChildren, err := d.HasAccountChildren(ctx, companyID, cashID)
	if err != nil || !hasChildren {
		t.Fatalf("HasAccountChildren: got=%v err=%v", hasChildren, err)
	}
	if _, err := d.GetPeriodForDate(ctx, companyID, start.Add(24*time.Hour)); err != nil {
		t.Fatalf("GetPeriodForDate: %v", err)
	}
	v2 := masterdata.Account{
		ID: cashID, CompanyID: companyID, Code: "1000", Name: "Cash v2", Type: masterdata.AccountTypeAsset,
		NormalBalance: masterdata.NormalBalanceDebit, Currency: "USD", IsActive: true, EffectiveFrom: start.AddDate(1, 0, 0),
	}
	if err := d.SaveAccountVersion(ctx, v2); err != nil {
		t.Fatalf("SaveAccountVersion: %v", err)
	}
	if gotAcc, err := d.GetAccountAsOf(ctx, companyID, cashID, start.AddDate(1, 0, 1)); err != nil || gotAcc.Name != "Cash v2" {
		t.Fatalf("GetAccountAsOf: got=%+v err=%v", gotAcc, err)
	}
	ccID := types.CostCenterID(uuid.New())
	if err := d.CreateCostCenter(ctx, masterdata.CostCenter{ID: ccID, CompanyID: companyID, Code: "CC1", Name: "HQ", Active: true}); err != nil {
		t.Fatalf("CreateCostCenter: %v", err)
	}
	if _, err := d.GetCostCenterForCompany(ctx, companyID, ccID); err != nil {
		t.Fatalf("GetCostCenterForCompany: %v", err)
	}
	dimID := types.ProjectID(uuid.New())
	if err := d.CreateAnalyticalDimensionValue(ctx, masterdata.AnalyticalDimensionValue{
		ID: dimID, CompanyID: companyID, Kind: types.DimensionProject, Code: "PRJ1", Name: "Proj 1", Active: true,
	}); err != nil {
		t.Fatalf("CreateAnalyticalDimensionValue: %v", err)
	}
	if _, err := d.GetAnalyticalDimensionForCompany(ctx, companyID, types.DimensionProject, dimID.String()); err != nil {
		t.Fatalf("GetAnalyticalDimensionForCompany: %v", err)
	}

	// PolicyStore
	if err := d.AppendVersion(ctx, storage.PolicyVersion{
		CompanyID:     companyID,
		PolicyID:      uuid.New(),
		Kind:          storage.PolicyKindCurrency,
		Version:       1,
		EffectiveFrom: start,
		Payload:       []byte(`{"functional_currency":"USD"}`),
	}); err != nil {
		t.Fatalf("AppendVersion: %v", err)
	}
	if vers, err := d.ListVersions(ctx, companyID, storage.PolicyKindCurrency); err != nil || len(vers) != 1 {
		t.Fatalf("ListVersions: len=%d err=%v", len(vers), err)
	}

	// Posting + Draft cancellation + Balances + Audit + Idempotency + Locks
	amt, _ := types.NewMoneyFromString("100.00", "USD")
	zero, _ := types.NewMoneyFromString("0.00", "USD")
	draftCancel := journal.JournalEntry{
		ID:                  types.NewJournalEntryID(),
		CompanyID:           companyID,
		JournalID:           journalID,
		FiscalPeriodID:      fpID,
		EffectiveDate:       start.Add(time.Hour),
		TransactionCurrency: "USD",
		ExchangeRate:        decimal.NewFromInt(1),
		Lines: []journal.EntryLine{
			{ID: types.NewJournalEntryLineID(), AccountID: cashID, TransactionAmount: amt, TransactionCurrency: "USD", ExchangeRate: decimal.NewFromInt(1), FunctionalDebit: amt, FunctionalCredit: zero},
			{ID: types.NewJournalEntryLineID(), AccountID: revID, TransactionAmount: amt, TransactionCurrency: "USD", ExchangeRate: decimal.NewFromInt(1), FunctionalDebit: zero, FunctionalCredit: amt},
		},
	}
	if err := ports.Drafts.SaveDraft(ctx, draftCancel); err != nil {
		t.Fatalf("SaveDraft: %v", err)
	}
	if err := d.CancelDraft(ctx, draftCancel); err != nil {
		t.Fatalf("CancelDraft: %v", err)
	}

	unlock, err := d.Lock(ctx, "posting", "test-lock")
	if err != nil {
		t.Fatalf("Lock: %v", err)
	}
	unlock()

	if _, err := d.GetTransactionBalances(ctx, ledger.ForeignCurrencyBalanceFilter{CompanyID: companyID, AccountID: cashID, AsOf: end}); err != nil {
		t.Fatalf("GetTransactionBalances: %v", err)
	}

	// Audit
	if err := d.AppendAudit(ctx, storage.AuditRecord{
		CompanyID:  companyID,
		EntityType: "JournalEntry",
		EntityID:   draftCancel.ID.String(),
		Action:     "ENTRY_POSTED",
		ActorID:    "tester",
		OccurredAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("AppendAudit: %v", err)
	}
	if recs := d.GetAuditRecords(); len(recs) == 0 {
		t.Fatalf("GetAuditRecords: expected non-empty")
	}
	if err := d.AppendAuditLog(ctx, publicgovernance.AuditLog{
		ID:        1,
		CompanyID: companyID,
		Action:    "POST",
	}); err != nil {
		t.Fatalf("AppendAuditLog: %v", err)
	}
	if _, err := d.GetLastAuditLogForCompany(ctx, companyID); err != nil {
		t.Fatalf("GetLastAuditLogForCompany: %v", err)
	}

	// Idempotency Lookup / Fail
	if _, _, err := d.Lookup(ctx, companyID, "missing"); err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if _, _, err := d.Claim(ctx, storage.IdempotencyRecord{
		CompanyID:   companyID,
		Key:         "k-fail",
		PayloadHash: "hash",
	}); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if err := d.Fail(ctx, companyID, "k-fail", "hash", "failed"); err != nil {
		t.Fatalf("Fail: %v", err)
	}

	// Events
	if _, claimed, err := d.ClaimPublicEventIngestion(ctx, publicevents.Ingestion{
		CompanyID:      companyID.UUID(),
		SourceSystem:   "erp",
		IdempotencyKey: "ev-1",
		EventType:      "SALE",
	}); err != nil || !claimed {
		t.Fatalf("ClaimPublicEventIngestion: claimed=%v err=%v", claimed, err)
	}
	if _, found, err := d.GetPublicEventIngestion(ctx, companyID.UUID(), "ev-1"); err != nil || found {
		t.Fatalf("GetPublicEventIngestion default source: found=%v err=%v", found, err)
	}
	if err := d.CompletePublicEventIngestionForSource(ctx, companyID.UUID(), "erp", "ev-1", "h1", draftCancel); err != nil {
		t.Fatalf("CompletePublicEventIngestionForSource: %v", err)
	}
	if _, claimed, err := d.ClaimPublicEventIngestion(ctx, publicevents.Ingestion{
		CompanyID:      companyID.UUID(),
		SourceSystem:   publicevents.DefaultSourceSystem,
		IdempotencyKey: "ev-2",
		EventType:      "SALE",
	}); err != nil || !claimed {
		t.Fatalf("ClaimPublicEventIngestion ev-2: claimed=%v err=%v", claimed, err)
	}
	if err := d.FailPublicEventIngestion(ctx, companyID.UUID(), "ev-2", "bad"); err != nil {
		t.Fatalf("FailPublicEventIngestion: %v", err)
	}
	if err := d.SavePublicEventAccountMapping(ctx, publicevents.AccountMapping{
		CompanyID: companyID.UUID(), EventType: "SALE", LedgerID: "DEFAULT", DebitAccountCode: "1000", CreditAccountCode: "4000",
	}); err != nil {
		t.Fatalf("SavePublicEventAccountMapping: %v", err)
	}
	if _, found, err := d.GetPublicEventAccountMapping(ctx, companyID.UUID(), "SALE", "DEFAULT"); err != nil || !found {
		t.Fatalf("GetPublicEventAccountMapping: found=%v err=%v", found, err)
	}

	// YearEndClosingStore methods
	if accs, err := d.ListAccounts(ctx, companyID); err != nil || len(accs) == 0 {
		t.Fatalf("ListAccounts: len=%d err=%v", len(accs), err)
	}
	if fys, err := d.ListFiscalYears(ctx, companyID); err != nil || len(fys) == 0 {
		t.Fatalf("ListFiscalYears: len=%d err=%v", len(fys), err)
	}
	if fps, err := d.ListFiscalPeriods(ctx, companyID, fyID); err != nil || len(fps) == 0 {
		t.Fatalf("ListFiscalPeriods: len=%d err=%v", len(fps), err)
	}
	if _, err := d.ListEntriesByDateRange(ctx, companyID, start, end); err != nil {
		t.Fatalf("ListEntriesByDateRange: %v", err)
	}
	if err := d.UpdateFiscalPeriodState(ctx, companyID, fpID, masterdata.PeriodSoftClosed, "tester", "close"); err != nil {
		t.Fatalf("UpdateFiscalPeriodState: %v", err)
	}
	if err := d.UpdateFiscalYearState(ctx, companyID, fyID, masterdata.FiscalYearHardClosed, "tester", "close"); err != nil {
		t.Fatalf("UpdateFiscalYearState: %v", err)
	}
	if err := d.SaveYearEndCloseRecord(ctx, storage.YearEndCloseRecord{
		CompanyID:    companyID,
		FiscalYearID: fyID,
		ClosedAt:     time.Now().UTC(),
		ClosedBy:     "tester",
	}); err != nil {
		t.Fatalf("SaveYearEndCloseRecord: %v", err)
	}
	if _, found, err := d.GetYearEndCloseRecord(ctx, companyID, fyID); err != nil || !found {
		t.Fatalf("GetYearEndCloseRecord: found=%v err=%v", found, err)
	}
}
