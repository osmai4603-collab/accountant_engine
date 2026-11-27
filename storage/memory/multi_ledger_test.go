package memory_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/journal"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/masterdata"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/storage"
	"github.com/osmai4603-collab/accounting_engine/storage/memory"
	"github.com/shopspring/decimal"
)

func TestMultiLedgerIsolation(t *testing.T) {
	ctx := context.Background()
	adapter := memory.New()
	ports := adapter.Ports()

	companyID := types.CompanyID(uuid.New())
	journalID := masterdata.NewJournalID()
	cashID := types.AccountID(uuid.New())
	revID := types.AccountID(uuid.New())
	fyID := types.FiscalYearID(uuid.New())
	fpID := types.FiscalPeriodID(uuid.New())

	_ = adapter.MasterDataStore().CreateCompany(ctx, masterdata.Company{
		ID: companyID, Code: "ML-01", LegalName: "Multi Ledger Co", FunctionalCurrency: "USD", PresentationCurrency: "USD", Active: true,
	})
	_ = adapter.MasterDataStore().CreateJournal(ctx, masterdata.Journal{ID: journalID, CompanyID: companyID, Code: "GEN", Name: "General"})
	_ = adapter.MasterDataStore().CreateAccount(ctx, masterdata.Account{ID: cashID, CompanyID: companyID, Code: "1000", Name: "Cash", Type: masterdata.AccountTypeAsset, NormalBalance: masterdata.NormalBalanceDebit, Currency: "USD", IsActive: true})
	_ = adapter.MasterDataStore().CreateAccount(ctx, masterdata.Account{ID: revID, CompanyID: companyID, Code: "4000", Name: "Rev", Type: masterdata.AccountTypeIncome, NormalBalance: masterdata.NormalBalanceCredit, Currency: "USD", IsActive: true})
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 12, 31, 23, 59, 59, 0, time.UTC)
	_ = adapter.MasterDataStore().CreateFiscalYear(ctx, masterdata.FiscalYear{ID: fyID, CompanyID: companyID, Name: "FY26", StartDate: start, EndDate: end, State: masterdata.FiscalYearOpen})
	_ = adapter.MasterDataStore().CreateFiscalPeriod(ctx, masterdata.FiscalPeriod{ID: fpID, CompanyID: companyID, FiscalYearID: fyID, Number: 1, Name: "FY26", StartDate: start, EndDate: end, State: masterdata.PeriodOpen, Type: masterdata.FiscalPeriodTypeNormal})

	amt100, _ := types.NewMoneyFromString("100.00", "USD")
	amt250, _ := types.NewMoneyFromString("250.00", "USD")
	zeroUSD, _ := types.NewMoneyFromString("0.00", "USD")

	ifrsCtx := storage.WithLedgerContext(ctx, "IFRS")
	taxCtx := storage.WithLedgerContext(ctx, "STATUTORY_TAX")

	entryIFRS := journal.JournalEntry{
		ID: types.NewJournalEntryID(), CompanyID: companyID, JournalID: journalID, FiscalPeriodID: fpID,
		EffectiveDate: start, PostingDate: start, Description: "IFRS Entry", TransactionCurrency: "USD", Status: journal.EntryPosted,
		Lines: []journal.EntryLine{
			{ID: types.NewJournalEntryLineID(), AccountID: cashID, TransactionAmount: amt100, TransactionCurrency: "USD", ExchangeRate: decimal.NewFromInt(1), FunctionalDebit: amt100, FunctionalCredit: zeroUSD},
			{ID: types.NewJournalEntryLineID(), AccountID: revID, TransactionAmount: amt100, TransactionCurrency: "USD", ExchangeRate: decimal.NewFromInt(1), FunctionalDebit: zeroUSD, FunctionalCredit: amt100},
		},
	}
	entryTax := journal.JournalEntry{
		ID: types.NewJournalEntryID(), CompanyID: companyID, JournalID: journalID, FiscalPeriodID: fpID,
		EffectiveDate: start, PostingDate: start, Description: "Tax Entry", TransactionCurrency: "USD", Status: journal.EntryPosted,
		Lines: []journal.EntryLine{
			{ID: types.NewJournalEntryLineID(), AccountID: cashID, TransactionAmount: amt250, TransactionCurrency: "USD", ExchangeRate: decimal.NewFromInt(1), FunctionalDebit: amt250, FunctionalCredit: zeroUSD},
			{ID: types.NewJournalEntryLineID(), AccountID: revID, TransactionAmount: amt250, TransactionCurrency: "USD", ExchangeRate: decimal.NewFromInt(1), FunctionalDebit: zeroUSD, FunctionalCredit: amt250},
		},
	}

	if err := ports.Drafts.SaveDraft(ifrsCtx, entryIFRS); err != nil {
		t.Fatalf("save IFRS draft: %v", err)
	}
	sharedScope := fmt.Sprintf("company=%s|operation=%s|key=%s", companyID.String(), storage.OperationPost, "shared-key")
	if _, err := ports.Posting.CommitPosting(ifrsCtx, storage.PostingCommit{
		Entry: entryIFRS, Operation: storage.OperationPost, ActorID: "a1",
		Idempotency: &storage.PostingIdempotency{Key: "shared-key", Scope: sharedScope, PayloadHash: "hash-ifrs"},
	}); err != nil {
		t.Fatalf("commit IFRS: %v", err)
	}

	if err := ports.Drafts.SaveDraft(taxCtx, entryTax); err != nil {
		t.Fatalf("save Tax draft: %v", err)
	}
	if _, err := ports.Posting.CommitPosting(taxCtx, storage.PostingCommit{
		Entry: entryTax, Operation: storage.OperationPost, ActorID: "a1",
		Idempotency: &storage.PostingIdempotency{Key: "shared-key", Scope: sharedScope, PayloadHash: "hash-tax"},
	}); err != nil {
		t.Fatalf("commit Tax with same idempotency key: %v", err)
	}

	// Verify entry isolation
	if _, err := ports.Entries.GetEntry(ifrsCtx, companyID, entryTax.ID); err == nil {
		t.Fatalf("expected IFRS ledger not to see Tax entry")
	}
	if _, err := ports.Entries.GetEntry(taxCtx, companyID, entryIFRS.ID); err == nil {
		t.Fatalf("expected Tax ledger not to see IFRS entry")
	}

	// Verify balance isolation
	ifrsBal, err := ports.Balance.GetAccountBalance(ifrsCtx, companyID, cashID, end, "USD")
	if err != nil || !ifrsBal.Equal(decimal.NewFromInt(100)) {
		t.Fatalf("IFRS cash balance = %s, %v; want 100", ifrsBal, err)
	}
	taxBal, err := ports.Balance.GetAccountBalance(taxCtx, companyID, cashID, end, "USD")
	if err != nil || !taxBal.Equal(decimal.NewFromInt(250)) {
		t.Fatalf("Tax cash balance = %s, %v; want 250", taxBal, err)
	}
}
