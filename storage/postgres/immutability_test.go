package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/journal"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/masterdata"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/storage"
	"github.com/shopspring/decimal"
)

func TestPostgresWORMImmutability(t *testing.T) {
	ctx := context.Background()
	driver, err := newIsolatedPostgresDriver(t, ctx)
	if err != nil {
		t.Skipf("isolated PostgreSQL unavailable: %v", err)
	}
	defer driver.Close()

	companyID := types.NewCompanyID()
	journalID := masterdata.NewJournalID()
	debitAccID := types.NewAccountID()
	creditAccID := types.NewAccountID()

	// 1. Provision Company
	if err := driver.CreateCompany(ctx, masterdata.Company{
		ID: companyID, Code: "WORM-CO", LegalName: "WORM Co",
		FunctionalCurrency: "USD", PresentationCurrency: "USD", Active: true,
	}); err != nil {
		t.Fatalf("create company: %v", err)
	}

	// 2. Provision Fiscal Year and Period
	fyID := types.NewFiscalYearID()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 12, 31, 23, 59, 59, 0, time.UTC)
	if err := driver.CreateFiscalYear(ctx, masterdata.FiscalYear{
		ID: fyID, CompanyID: companyID, Name: "FY2026",
		StartDate: start, EndDate: end, State: masterdata.FiscalYearOpen,
	}); err != nil {
		t.Fatalf("create fiscal year: %v", err)
	}

	periodID := types.NewFiscalPeriodID()
	if err := driver.CreateFiscalPeriod(ctx, masterdata.FiscalPeriod{
		ID: periodID, CompanyID: companyID, FiscalYearID: fyID,
		Number: 1, Name: "P1-2026", StartDate: start,
		EndDate: time.Date(2026, 1, 31, 23, 59, 59, 0, time.UTC),
		State: masterdata.PeriodOpen, Type: masterdata.FiscalPeriodTypeNormal,
	}); err != nil {
		t.Fatalf("create fiscal period: %v", err)
	}

	// 3. Provision Journal
	if err := driver.CreateJournal(ctx, masterdata.Journal{
		ID: journalID, CompanyID: companyID, Code: "GEN", Name: "General",
	}); err != nil {
		t.Fatalf("create journal: %v", err)
	}

	// 3. Provision Accounts
	if err := driver.CreateAccount(ctx, masterdata.Account{
		ID: debitAccID, CompanyID: companyID, Code: "1000", Name: "Cash",
		Type: masterdata.AccountTypeAsset, NormalBalance: masterdata.NormalBalanceDebit,
		Currency: "USD", IsActive: true,
	}); err != nil {
		t.Fatalf("create debit account: %v", err)
	}
	if err := driver.CreateAccount(ctx, masterdata.Account{
		ID: creditAccID, CompanyID: companyID, Code: "2000", Name: "Payable",
		Type: masterdata.AccountTypeLiability, NormalBalance: masterdata.NormalBalanceCredit,
		Currency: "USD", IsActive: true,
	}); err != nil {
		t.Fatalf("create credit account: %v", err)
	}

	// 5. Create and Post an entry
	entryID := types.NewJournalEntryID()
	money100, _ := types.NewMoney(decimal.NewFromInt(100), "USD")
	entry := journal.JournalEntry{
		ID:             entryID,
		CompanyID:      companyID,
		JournalID:      types.JournalID(journalID),
		FiscalPeriodID: periodID,
		EffectiveDate:  time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
		Status:         journal.EntryDraft,
		Lines: []journal.EntryLine{
			{
				ID:                  types.NewJournalEntryLineID(),
				AccountID:           debitAccID,
				TransactionCurrency: "USD",
				TransactionAmount:   money100,
				ExchangeRate:        decimal.NewFromInt(1),
				FunctionalDebit:     money100,
			},
			{
				ID:                  types.NewJournalEntryLineID(),
				AccountID:           creditAccID,
				TransactionCurrency: "USD",
				TransactionAmount:   money100,
				ExchangeRate:        decimal.NewFromInt(1),
				FunctionalCredit:    money100,
			},
		},
	}

	postCtx := storage.WithCompanyContext(ctx, companyID)
	err = driver.Execute(postCtx, func(uowCtx context.Context, uow storage.UnitOfWork) error {
		_, postErr := uow.Posting().CommitPosting(uowCtx, storage.PostingCommit{
			Entry:     entry,
			Operation: storage.OperationPost,
			ActorID:   "auditor-1",
		})
		return postErr
	})
	if err != nil {
		t.Fatalf("commit posting failed: %v", err)
	}

	// 5. Verify WORM: Direct UPDATE on posted entry in account_moves must FAIL
	t.Run("PreventPostedMoveUpdate", func(t *testing.T) {
		_, err := driver.Pool().Exec(ctx,
			`UPDATE account_moves SET entry='{"tampered":true}' WHERE company_id=$1 AND id=$2`,
			uuid.UUID(companyID), uuid.UUID(entryID))
		if err == nil {
			t.Fatal("direct UPDATE on posted account_moves succeeded, expected WORM rejection")
		}
	})

	// 6. Verify WORM: Direct DELETE on posted entry in account_moves must FAIL
	t.Run("PreventPostedMoveDelete", func(t *testing.T) {
		_, err := driver.Pool().Exec(ctx,
			`DELETE FROM account_moves WHERE company_id=$1 AND id=$2`,
			uuid.UUID(companyID), uuid.UUID(entryID))
		if err == nil {
			t.Fatal("direct DELETE on posted account_moves succeeded, expected WORM rejection")
		}
	})

	// 7. Verify WORM: Direct UPDATE on financial_audit_logs must FAIL
	t.Run("PreventAuditLogUpdate", func(t *testing.T) {
		_, err := driver.Pool().Exec(ctx,
			`UPDATE financial_audit_logs SET action='TAMPERED' WHERE company_id=$1`,
			uuid.UUID(companyID))
		if err == nil {
			t.Fatal("direct UPDATE on financial_audit_logs succeeded, expected WORM rejection")
		}
	})

	// 8. Verify WORM: Direct DELETE on financial_audit_logs must FAIL
	t.Run("PreventAuditLogDelete", func(t *testing.T) {
		_, err := driver.Pool().Exec(ctx,
			`DELETE FROM financial_audit_logs WHERE company_id=$1`,
			uuid.UUID(companyID))
		if err == nil {
			t.Fatal("direct DELETE on financial_audit_logs succeeded, expected WORM rejection")
		}
	})
}
