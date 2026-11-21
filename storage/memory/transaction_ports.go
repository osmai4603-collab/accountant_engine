package memory

import (
	"context"
	"time"

	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/journal"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/masterdata"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/storage"
	"github.com/shopspring/decimal"
)

func transactionDriver(ctx context.Context, driver *Driver) *Driver {
	if ctx == nil {
		return driver
	}
	scope, ok := ctx.Value(transactionContextKey{}).(memoryTransactionScope)
	if ok && scope.root == driver {
		return scope.driver
	}
	return driver
}

// transactionAwareDriver wraps Driver so that all port operations automatically
// delegate to the transaction snapshot driver when inside a transaction context.
type transactionAwareDriver struct {
	driver *Driver
}

func (t transactionAwareDriver) SaveDraft(ctx context.Context, entry journal.JournalEntry) error {
	return transactionDriver(ctx, t.driver).SaveDraft(ctx, entry)
}

func (t transactionAwareDriver) GetEntry(ctx context.Context, companyID types.CompanyID, entryID types.JournalEntryID) (journal.JournalEntry, error) {
	return transactionDriver(ctx, t.driver).GetEntry(ctx, companyID, entryID)
}

func (t transactionAwareDriver) CommitPosting(ctx context.Context, commit storage.PostingCommit) (journal.JournalEntry, error) {
	return transactionDriver(ctx, t.driver).CommitPosting(ctx, commit)
}

func (t transactionAwareDriver) NextSequence(ctx context.Context, companyID types.CompanyID, journalID types.JournalID) (int64, error) {
	return transactionDriver(ctx, t.driver).NextSequence(ctx, companyID, journalID)
}

func (t transactionAwareDriver) GetCompany(ctx context.Context, companyID types.CompanyID) (masterdata.Company, error) {
	return transactionDriver(ctx, t.driver).GetCompany(ctx, companyID)
}

func (t transactionAwareDriver) GetAccount(ctx context.Context, companyID types.CompanyID, accountID types.AccountID) (masterdata.Account, error) {
	return transactionDriver(ctx, t.driver).GetAccount(ctx, companyID, accountID)
}

func (t transactionAwareDriver) GetAccountByCode(ctx context.Context, companyID types.CompanyID, code string) (masterdata.Account, error) {
	return transactionDriver(ctx, t.driver).GetAccountByCode(ctx, companyID, code)
}

func (t transactionAwareDriver) GetJournalByCode(ctx context.Context, companyID types.CompanyID, code string) (masterdata.Journal, error) {
	return transactionDriver(ctx, t.driver).GetJournalByCode(ctx, companyID, code)
}

func (t transactionAwareDriver) HasAccountChildren(ctx context.Context, companyID types.CompanyID, accountID types.AccountID) (bool, error) {
	return transactionDriver(ctx, t.driver).HasAccountChildren(ctx, companyID, accountID)
}

func (t transactionAwareDriver) GetFiscalYear(ctx context.Context, companyID types.CompanyID, fiscalYearID types.FiscalYearID) (masterdata.FiscalYear, error) {
	return transactionDriver(ctx, t.driver).GetFiscalYear(ctx, companyID, fiscalYearID)
}

func (t transactionAwareDriver) GetPeriodForDate(ctx context.Context, companyID types.CompanyID, date time.Time) (masterdata.FiscalPeriod, error) {
	return transactionDriver(ctx, t.driver).GetPeriodForDate(ctx, companyID, date)
}

func (t transactionAwareDriver) GetAccountBalance(ctx context.Context, companyID types.CompanyID, accountID types.AccountID, asOf time.Time, currency string) (decimal.Decimal, error) {
	return transactionDriver(ctx, t.driver).GetAccountBalance(ctx, companyID, accountID, asOf, currency)
}

func (t transactionAwareDriver) AppendAudit(ctx context.Context, record storage.AuditRecord) error {
	return transactionDriver(ctx, t.driver).AppendAudit(ctx, record)
}

func (t transactionAwareDriver) Claim(ctx context.Context, record storage.IdempotencyRecord) (existing storage.IdempotencyRecord, claimed bool, err error) {
	return transactionDriver(ctx, t.driver).Claim(ctx, record)
}

func (t transactionAwareDriver) Complete(ctx context.Context, companyID types.CompanyID, key, payloadHash string, entryID types.JournalEntryID, response []byte) error {
	return transactionDriver(ctx, t.driver).Complete(ctx, companyID, key, payloadHash, entryID, response)
}

func (t transactionAwareDriver) Fail(ctx context.Context, companyID types.CompanyID, key, payloadHash, status string) error {
	return transactionDriver(ctx, t.driver).Fail(ctx, companyID, key, payloadHash, status)
}

func (t transactionAwareDriver) CreateCompany(ctx context.Context, c masterdata.Company) error {
	return transactionDriver(ctx, t.driver).CreateCompany(ctx, c)
}

func (t transactionAwareDriver) CreateJournal(ctx context.Context, j masterdata.Journal) error {
	return transactionDriver(ctx, t.driver).CreateJournal(ctx, j)
}

func (t transactionAwareDriver) CreateAccount(ctx context.Context, a masterdata.Account) error {
	return transactionDriver(ctx, t.driver).CreateAccount(ctx, a)
}

func (t transactionAwareDriver) CreateFiscalYear(ctx context.Context, fy masterdata.FiscalYear) error {
	return transactionDriver(ctx, t.driver).CreateFiscalYear(ctx, fy)
}

func (t transactionAwareDriver) CreateFiscalPeriod(ctx context.Context, fp masterdata.FiscalPeriod) error {
	return transactionDriver(ctx, t.driver).CreateFiscalPeriod(ctx, fp)
}

func (t transactionAwareDriver) GetFiscalPeriod(ctx context.Context, companyID types.CompanyID, periodID types.FiscalPeriodID) (masterdata.FiscalPeriod, error) {
	return transactionDriver(ctx, t.driver).GetFiscalPeriod(ctx, companyID, periodID)
}

var _ masterdata.Store = transactionAwareDriver{}
