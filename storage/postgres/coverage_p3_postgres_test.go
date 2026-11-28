package postgres_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/masterdata"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/storage"
)

// Cover the master-data read/write branches that the posting fixtures do not
// reach: validation errors, duplicate mappings, parent links, missing rows,
// tenant scope rejection and list iteration.
func TestCoverageP3PostgresMasterDataCrud(t *testing.T) {
	ctx := context.Background()
	driver, err := newIsolatedPostgresDriver(t, ctx)
	if err != nil {
		t.Fatalf("isolated driver: %v", err)
	}
	f := phase8ProvisionCompany(t, ctx, driver, "P3MD")
	companyCtx := storage.WithCompanyContext(ctx, f.companyID)
	otherCtx := storage.WithCompanyContext(ctx, types.NewCompanyID())

	if err := driver.CreateCompany(ctx, masterdata.Company{}); !errors.Is(err, masterdata.ErrInvalidCompany) {
		t.Fatalf("invalid company = %v", err)
	}
	if err := driver.CreateCompany(companyCtx, masterdata.Company{
		ID: types.NewCompanyID(), Code: "p3md", LegalName: "Duplicate",
		FunctionalCurrency: "usd", PresentationCurrency: "usd", Active: true,
	}); !errors.Is(err, masterdata.ErrDuplicateCompanyCode) {
		t.Fatalf("duplicate company code = %v", err)
	}
	company, err := driver.GetCompany(ctx, f.companyID)
	if err != nil || company.Code != "P3MD" {
		t.Fatalf("GetCompany = %+v, %v", company, err)
	}
	if _, err := driver.GetCompany(otherCtx, types.NewCompanyID()); !errors.Is(err, masterdata.ErrCompanyNotFound) {
		t.Fatalf("missing company = %v", err)
	}

	invalidJournal := masterdata.Journal{ID: masterdata.NewJournalID(), CompanyID: f.companyID, Code: "BAD"}
	if err := driver.CreateJournal(ctx, invalidJournal); !errors.Is(err, masterdata.ErrInvalidJournal) {
		t.Fatalf("invalid journal = %v", err)
	}
	if err := driver.CreateJournal(ctx, masterdata.Journal{
		ID: masterdata.NewJournalID(), CompanyID: types.NewCompanyID(), Code: "MISS", Name: "Missing",
	}); !errors.Is(err, masterdata.ErrCompanyNotFound) {
		t.Fatalf("journal for missing company = %v", err)
	}
	if err := driver.CreateJournal(ctx, masterdata.Journal{
		ID: masterdata.NewJournalID(), CompanyID: f.companyID, Code: "GEN", Name: "Duplicate code",
	}); !errors.Is(err, masterdata.ErrDuplicateJournalCode) {
		t.Fatalf("duplicate journal code = %v", err)
	}
	if err := driver.CreateJournal(ctx, masterdata.Journal{
		ID: f.journalID, CompanyID: f.companyID, Code: "GEN2", Name: "Duplicate id",
	}); !errors.Is(err, masterdata.ErrDuplicateJournalID) {
		t.Fatalf("duplicate journal id = %v", err)
	}

	if _, err := driver.GetJournalByCode(ctx, types.CompanyID{}, "GEN"); !errors.Is(err, masterdata.ErrInvalidJournal) {
		t.Fatalf("journal by code without company = %v", err)
	}
	if _, err := driver.GetJournalByCode(ctx, f.companyID, ""); !errors.Is(err, masterdata.ErrInvalidJournal) {
		t.Fatalf("journal by code without code = %v", err)
	}
	if _, err := driver.GetJournalByCode(ctx, f.companyID, strings.Repeat("x", 33)); !errors.Is(err, masterdata.ErrInvalidJournal) {
		t.Fatalf("journal by code with long code = %v", err)
	}
	if _, err := driver.GetJournalByCode(otherCtx, f.companyID, "GEN"); !errors.Is(err, masterdata.ErrJournalNotFound) {
		t.Fatalf("journal by code scope violation = %v", err)
	}
	if _, err := driver.GetJournalByCode(ctx, f.companyID, "NO-JOURNAL"); !errors.Is(err, masterdata.ErrJournalNotFound) {
		t.Fatalf("missing journal by code = %v", err)
	}
	journalByCode, err := driver.GetJournalByCode(ctx, f.companyID, "GEN")
	if err != nil || journalByCode.ID != f.journalID {
		t.Fatalf("GetJournalByCode = %+v, %v", journalByCode, err)
	}
	if _, err := driver.GetJournalByID(ctx, types.CompanyID{}, f.journalID); !errors.Is(err, masterdata.ErrInvalidJournal) {
		t.Fatalf("journal by id without company = %v", err)
	}
	if _, err := driver.GetJournalByID(ctx, f.companyID, masterdata.JournalID{}); !errors.Is(err, masterdata.ErrInvalidJournal) {
		t.Fatalf("journal by id without id = %v", err)
	}
	if _, err := driver.GetJournalByID(otherCtx, f.companyID, f.journalID); !errors.Is(err, masterdata.ErrJournalNotFound) {
		t.Fatalf("journal by id scope violation = %v", err)
	}
	if _, err := driver.GetJournalByID(ctx, f.companyID, masterdata.NewJournalID()); !errors.Is(err, masterdata.ErrJournalNotFound) {
		t.Fatalf("missing journal by id = %v", err)
	}
	journalByID, err := driver.GetJournalByID(ctx, f.companyID, f.journalID)
	if err != nil || journalByID.Code != "GEN" {
		t.Fatalf("GetJournalByID = %+v, %v", journalByID, err)
	}

	if err := driver.CreateAccount(ctx, masterdata.Account{}); !errors.Is(err, masterdata.ErrInvalidAccount) {
		t.Fatalf("invalid account = %v", err)
	}
	if err := driver.CreateAccount(ctx, masterdata.Account{
		ID: types.NewAccountID(), CompanyID: f.companyID, Code: "1000", Name: "Duplicate",
		Type: masterdata.AccountTypeAsset, NormalBalance: masterdata.NormalBalanceDebit,
	}); !errors.Is(err, masterdata.ErrDuplicateAccountCode) {
		t.Fatalf("duplicate account code = %v", err)
	}
	childID := types.NewAccountID()
	child := masterdata.Account{
		ID: childID, CompanyID: f.companyID, Code: "1100", Name: "Child cash",
		Type: masterdata.AccountTypeAsset, NormalBalance: masterdata.NormalBalanceDebit,
		Currency: "USD", ParentID: &f.debitID, IsActive: true,
	}
	if err := driver.CreateAccount(ctx, child); err != nil {
		t.Fatalf("create child account: %v", err)
	}
	childAccount, err := driver.GetAccount(ctx, f.companyID, childID)
	if err != nil || childAccount.ParentID == nil || *childAccount.ParentID != f.debitID {
		t.Fatalf("GetAccount with parent = %+v, %v", childAccount, err)
	}
	if _, err := driver.GetAccount(ctx, f.companyID, types.NewAccountID()); !errors.Is(err, masterdata.ErrAccountNotFound) {
		t.Fatalf("missing account = %v", err)
	}
	if _, err := driver.GetAccountByCode(otherCtx, f.companyID, "1100"); !errors.Is(err, masterdata.ErrAccountNotFound) {
		t.Fatalf("account by code scope violation = %v", err)
	}
	if _, err := driver.GetAccountByCode(ctx, f.companyID, "NO-ACCOUNT"); !errors.Is(err, masterdata.ErrAccountNotFound) {
		t.Fatalf("missing account by code = %v", err)
	}
	accountByCode, err := driver.GetAccountByCode(ctx, f.companyID, "1100")
	if err != nil || accountByCode.ID != childID {
		t.Fatalf("GetAccountByCode = %+v, %v", accountByCode, err)
	}
	hasChildren, err := driver.HasAccountChildren(ctx, f.companyID, f.debitID)
	if err != nil || !hasChildren {
		t.Fatalf("HasAccountChildren(parent) = %v, %v", hasChildren, err)
	}
	hasChildren, err = driver.HasAccountChildren(ctx, f.companyID, f.creditID)
	if err != nil || hasChildren {
		t.Fatalf("HasAccountChildren(leaf) = %v, %v", hasChildren, err)
	}

	nextYearID := types.NewFiscalYearID()
	if err := driver.CreateFiscalYear(ctx, masterdata.FiscalYear{
		ID: nextYearID, CompanyID: f.companyID, Name: "FY2027",
		StartDate: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2027, 12, 31, 0, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("CreateFiscalYear with default state: %v", err)
	}
	nextYear, err := driver.GetFiscalYear(ctx, f.companyID, nextYearID)
	if err != nil || nextYear.State != masterdata.FiscalYearOpen {
		t.Fatalf("GetFiscalYear = %+v, %v", nextYear, err)
	}
	if _, err := driver.GetFiscalYear(ctx, f.companyID, types.NewFiscalYearID()); !errors.Is(err, masterdata.ErrFiscalYearNotFound) {
		t.Fatalf("missing fiscal year = %v", err)
	}

	nextPeriodID := types.NewFiscalPeriodID()
	if err := driver.CreateFiscalPeriod(ctx, masterdata.FiscalPeriod{
		ID: nextPeriodID, CompanyID: f.companyID, FiscalYearID: nextYearID, Number: 2, Name: "February",
		StartDate: time.Date(2027, 2, 1, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2027, 2, 28, 0, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("CreateFiscalPeriod with defaults: %v", err)
	}
	nextPeriod, err := driver.GetFiscalPeriod(ctx, f.companyID, nextPeriodID)
	if err != nil || nextPeriod.State != masterdata.PeriodOpen || nextPeriod.Type != masterdata.FiscalPeriodTypeNormal {
		t.Fatalf("GetFiscalPeriod = %+v, %v", nextPeriod, err)
	}
	if err := driver.CreateFiscalPeriod(ctx, masterdata.FiscalPeriod{
		ID: types.NewFiscalPeriodID(), CompanyID: f.companyID, FiscalYearID: nextYearID, Number: 3,
		Name: "March", StartDate: time.Date(2027, 3, 1, 0, 0, 0, 0, time.UTC),
		EndDate: time.Date(2027, 3, 31, 0, 0, 0, 0, time.UTC), State: masterdata.PeriodSoftClosed,
	}); err != nil {
		t.Fatalf("CreateFiscalPeriod closed: %v", err)
	}
	if err := driver.CreateFiscalPeriod(ctx, masterdata.FiscalPeriod{
		ID: types.NewFiscalPeriodID(), CompanyID: f.companyID, FiscalYearID: types.NewFiscalYearID(), Number: 4,
		Name: "April", StartDate: time.Date(2027, 4, 1, 0, 0, 0, 0, time.UTC),
		EndDate: time.Date(2027, 4, 30, 0, 0, 0, 0, time.UTC),
	}); !errors.Is(err, masterdata.ErrFiscalYearNotFound) {
		t.Fatalf("period for missing fiscal year = %v", err)
	}
	if _, err := driver.GetFiscalPeriod(ctx, f.companyID, types.NewFiscalPeriodID()); !errors.Is(err, masterdata.ErrPeriodNotFound) {
		t.Fatalf("missing fiscal period = %v", err)
	}

	periodForDate, err := driver.GetPeriodForDate(ctx, f.companyID, time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC))
	if err != nil || periodForDate.ID != f.periodID {
		t.Fatalf("GetPeriodForDate = %+v, %v", periodForDate, err)
	}
	if _, err := driver.GetPeriodForDate(ctx, f.companyID, time.Date(2030, 1, 15, 0, 0, 0, 0, time.UTC)); !errors.Is(err, masterdata.ErrPeriodNotFound) {
		t.Fatalf("period for uncovered date = %v", err)
	}
	if _, err := driver.GetPeriodForDate(otherCtx, f.companyID, time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)); !errors.Is(err, masterdata.ErrPeriodNotFound) {
		t.Fatalf("period scope violation = %v", err)
	}
	periodByNumber, err := driver.GetPeriodByNumber(ctx, f.companyID, periodForDate.FiscalYearID, 1)
	if err != nil || periodByNumber.ID != f.periodID {
		t.Fatalf("GetPeriodByNumber = %+v, %v", periodByNumber, err)
	}
	if _, err := driver.GetPeriodByNumber(ctx, f.companyID, periodForDate.FiscalYearID, 99); !errors.Is(err, masterdata.ErrPeriodNotFound) {
		t.Fatalf("missing period number = %v", err)
	}
	if _, err := driver.GetPeriodByNumber(otherCtx, f.companyID, periodForDate.FiscalYearID, 1); !errors.Is(err, masterdata.ErrPeriodNotFound) {
		t.Fatalf("period number scope violation = %v", err)
	}

	// ListFiscalYears selects COALESCE(name, year::text); older schemas may not
	// carry the name column, so add it in this isolated schema to exercise the
	// row-scanning branch as well.
	if _, err := driver.Pool().Exec(ctx, `ALTER TABLE fiscal_years ADD COLUMN IF NOT EXISTS name text`); err != nil {
		t.Fatalf("add fiscal year name column: %v", err)
	}
	if _, err := driver.Pool().Exec(ctx, `UPDATE fiscal_years SET name = code || '_listed' WHERE company_id=$1`, uuid.UUID(f.companyID)); err != nil {
		t.Fatalf("name fiscal years: %v", err)
	}
	listedAccounts, err := driver.ListAccounts(ctx, f.companyID)
	if err != nil || len(listedAccounts) < 3 {
		t.Fatalf("ListAccounts = %v, %v", listedAccounts, err)
	}
	foundParent := false
	for _, account := range listedAccounts {
		if account.ID == childID && account.ParentID != nil && *account.ParentID == f.debitID {
			foundParent = true
		}
	}
	if !foundParent {
		t.Fatal("ListAccounts did not restore a parent account id")
	}
	listedYears, err := driver.ListFiscalYears(ctx, f.companyID)
	if err != nil || len(listedYears) != 2 {
		t.Fatalf("ListFiscalYears = %v, %v", listedYears, err)
	}
	listedPeriods, err := driver.ListFiscalPeriods(ctx, f.companyID, periodForDate.FiscalYearID)
	if err != nil || len(listedPeriods) != 1 {
		t.Fatalf("ListFiscalPeriods = %v, %v", listedPeriods, err)
	}
}

// Cover effective-dated account version validation, overlap rejection,
// fallback reads and newest-version refresh branches.
func TestCoverageP3PostgresAccountVersions(t *testing.T) {
	ctx := context.Background()
	driver, err := newIsolatedPostgresDriver(t, ctx)
	if err != nil {
		t.Fatalf("isolated driver: %v", err)
	}
	f := phase8ProvisionCompany(t, ctx, driver, "P3VER")
	otherCtx := storage.WithCompanyContext(ctx, types.NewCompanyID())

	base, err := driver.GetAccount(ctx, f.companyID, f.debitID)
	if err != nil {
		t.Fatalf("base account: %v", err)
	}
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	firstTo := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	secondFrom := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	secondTo := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)

	if err := driver.SaveAccountVersion(ctx, masterdata.Account{}); !errors.Is(err, masterdata.ErrInvalidAccount) {
		t.Fatalf("invalid version account = %v", err)
	}
	noDate := base
	noDate.EffectiveFrom = time.Time{}
	if err := driver.SaveAccountVersion(ctx, noDate); !errors.Is(err, masterdata.ErrInvalidEffectiveRange) {
		t.Fatalf("version without effective date = %v", err)
	}
	reversed := base
	reversed.EffectiveFrom = from
	reversedTo := from.Add(-24 * time.Hour)
	reversed.EffectiveTo = &reversedTo
	if err := driver.SaveAccountVersion(ctx, reversed); !errors.Is(err, masterdata.ErrInvalidEffectiveRange) {
		t.Fatalf("reversed version range = %v", err)
	}
	missing := base
	missing.ID = types.NewAccountID()
	missing.EffectiveFrom = from
	if err := driver.SaveAccountVersion(ctx, missing); !errors.Is(err, masterdata.ErrAccountNotFound) {
		t.Fatalf("version for missing account = %v", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := driver.SaveAccountVersion(cancelled, base); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled SaveAccountVersion = %v", err)
	}
	if _, err := driver.GetAccountAsOf(cancelled, f.companyID, f.debitID, from); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled GetAccountAsOf = %v", err)
	}

	first := base
	first.Name = "Cash v1"
	first.EffectiveFrom = from
	first.EffectiveTo = &firstTo
	if err := driver.SaveAccountVersion(ctx, first); err != nil {
		t.Fatalf("save first account version: %v", err)
	}
	overlap := base
	overlap.EffectiveFrom = time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	overlapTo := time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)
	overlap.EffectiveTo = &overlapTo
	if err := driver.SaveAccountVersion(ctx, overlap); !errors.Is(err, masterdata.ErrAccountVersionOverlap) {
		t.Fatalf("overlapping version = %v", err)
	}

	second := base
	second.Name = "Cash v2"
	second.EffectiveFrom = secondFrom
	second.EffectiveTo = &secondTo
	if err := driver.SaveAccountVersion(ctx, second); err != nil {
		t.Fatalf("save second account version: %v", err)
	}

	asOfFirst, err := driver.GetAccountAsOf(ctx, f.companyID, f.debitID, time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC))
	if err != nil || asOfFirst.Name != "Cash v1" || asOfFirst.EffectiveTo == nil {
		t.Fatalf("GetAccountAsOf(first version) = %+v, %v", asOfFirst, err)
	}
	asOfSecond, err := driver.GetAccountAsOf(ctx, f.companyID, f.debitID, time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC))
	if err != nil || asOfSecond.Name != "Cash v2" {
		t.Fatalf("GetAccountAsOf(second version) = %+v, %v", asOfSecond, err)
	}
	fallback, err := driver.GetAccountAsOf(ctx, f.companyID, f.debitID, time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC))
	if err != nil || fallback.EffectiveFrom != (time.Time{}) {
		t.Fatalf("GetAccountAsOf(fallback) = %+v, %v", fallback, err)
	}
	if _, err := driver.GetAccountAsOf(otherCtx, f.companyID, f.debitID, from); !errors.Is(err, masterdata.ErrAccountNotFound) {
		t.Fatalf("account version scope violation = %v", err)
	}
}
