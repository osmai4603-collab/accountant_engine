package memory

import (
	"context"
	"time"

	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/journal"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/ledger"
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

func (t transactionAwareDriver) GetJournalByID(ctx context.Context, companyID types.CompanyID, journalID masterdata.JournalID) (masterdata.Journal, error) {
	return transactionDriver(ctx, t.driver).GetJournalByID(ctx, companyID, journalID)
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

func (t transactionAwareDriver) GetTransactionBalances(ctx context.Context, filter ledger.ForeignCurrencyBalanceFilter) ([]ledger.ForeignCurrencyBalance, error) {
	return transactionDriver(ctx, t.driver).GetTransactionBalances(ctx, filter)
}

func (t transactionAwareDriver) AppendAudit(ctx context.Context, record storage.AuditRecord) error {
	return transactionDriver(ctx, t.driver).AppendAudit(ctx, record)
}

func (t transactionAwareDriver) Claim(ctx context.Context, record storage.IdempotencyRecord) (existing storage.IdempotencyRecord, claimed bool, err error) {
	return transactionDriver(ctx, t.driver).Claim(ctx, record)
}

func (t transactionAwareDriver) Lookup(ctx context.Context, companyID types.CompanyID, key string) (storage.IdempotencyRecord, bool, error) {
	return transactionDriver(ctx, t.driver).Lookup(ctx, companyID, key)
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

// Phase 11 optional master-data capabilities. Each method delegates through
// transactionDriver so G-019/G-022 writes and reads join the active unit of
// work and roll back with it.
func (t transactionAwareDriver) GetAccountAsOf(
	ctx context.Context,
	companyID types.CompanyID,
	accountID types.AccountID,
	asOf time.Time,
) (masterdata.Account, error) {
	return transactionDriver(ctx, t.driver).GetAccountAsOf(ctx, companyID, accountID, asOf)
}

func (t transactionAwareDriver) SaveAccountVersion(ctx context.Context, account masterdata.Account) error {
	return transactionDriver(ctx, t.driver).SaveAccountVersion(ctx, account)
}

func (t transactionAwareDriver) CreateCostCenter(ctx context.Context, center masterdata.CostCenter) error {
	return transactionDriver(ctx, t.driver).CreateCostCenter(ctx, center)
}

func (t transactionAwareDriver) GetCostCenterForCompany(ctx context.Context, companyID types.CompanyID, costCenterID types.CostCenterID) (masterdata.CostCenter, error) {
	return transactionDriver(ctx, t.driver).GetCostCenterForCompany(ctx, companyID, costCenterID)
}

func (t transactionAwareDriver) CreateAnalyticalDimensionValue(ctx context.Context, value masterdata.AnalyticalDimensionValue) error {
	return transactionDriver(ctx, t.driver).CreateAnalyticalDimensionValue(ctx, value)
}

func (t transactionAwareDriver) GetAnalyticalDimensionForCompany(
	ctx context.Context,
	companyID types.CompanyID,
	kind masterdata.AnalyticalDimensionKind,
	id string,
) (masterdata.AnalyticalDimensionValue, error) {
	return transactionDriver(ctx, t.driver).GetAnalyticalDimensionForCompany(ctx, companyID, kind, id)
}

func (t transactionAwareDriver) AppendVersion(ctx context.Context, version storage.PolicyVersion) error {
	return transactionDriver(ctx, t.driver).AppendVersion(ctx, version)
}

func (t transactionAwareDriver) ListVersions(ctx context.Context, companyID types.CompanyID, kind storage.PolicyKind) ([]storage.PolicyVersion, error) {
	return transactionDriver(ctx, t.driver).ListVersions(ctx, companyID, kind)
}

var _ masterdata.Store = transactionAwareDriver{}
var _ masterdata.EffectiveAccountReader = transactionAwareDriver{}
var _ masterdata.AccountVersionWriter = transactionAwareDriver{}
var _ masterdata.CostCenterReader = transactionAwareDriver{}
var _ masterdata.CostCenterWriter = transactionAwareDriver{}
var _ masterdata.AnalyticalDimensionReader = transactionAwareDriver{}
var _ masterdata.AnalyticalDimensionWriter = transactionAwareDriver{}
var _ storage.PolicyStore = transactionAwareDriver{}
var _ storage.TransactionBalanceReader = transactionAwareDriver{}
