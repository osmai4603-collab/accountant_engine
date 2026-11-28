package sdk

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	kernelmasterdata "github.com/osmai4603-collab/accounting_engine/internal/kernel/masterdata"
	kernelposting "github.com/osmai4603-collab/accounting_engine/internal/kernel/posting"
	kernelvaluation "github.com/osmai4603-collab/accounting_engine/internal/kernel/valuation"
	internalconsolidation "github.com/osmai4603-collab/accounting_engine/internal/orbitals/consolidation"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/events"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/journal"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/reporting"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	publicapplication "github.com/osmai4603-collab/accounting_engine/pkg/spi/application"
	publicconsolidation "github.com/osmai4603-collab/accounting_engine/pkg/spi/consolidation"
	publicevents "github.com/osmai4603-collab/accounting_engine/pkg/spi/events"
	publicmasterdata "github.com/osmai4603-collab/accounting_engine/pkg/spi/masterdata"
	publicreconciliation "github.com/osmai4603-collab/accounting_engine/pkg/spi/reconciliation"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/standards"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/storage"
	publicsubledger "github.com/osmai4603-collab/accounting_engine/pkg/spi/subledger"
	"github.com/osmai4603-collab/accounting_engine/storage/memory"
	"github.com/shopspring/decimal"
)

type p2Observer struct {
	posts     int
	reversals int
	tx        int
}

func (o *p2Observer) ObservePosting(context.Context, string, time.Duration, error)  { o.posts++ }
func (o *p2Observer) ObserveReversal(context.Context, string, time.Duration, error) { o.reversals++ }
func (o *p2Observer) ObserveStorageTx(context.Context, string, time.Duration, error) {
	o.tx++
}

type p2Authorizer struct{ err error }

func (a p2Authorizer) Authorize(context.Context, publicapplication.AuthorizationRequest) error {
	return a.err
}
func (p2Authorizer) IsApproved(context.Context, types.CompanyID, types.JournalEntryID) (bool, error) {
	return false, nil
}

type p2PolicyCapability struct{}

func (p2PolicyCapability) RequiresCostCenter(context.Context, types.CompanyID, types.AccountID) (bool, error) {
	return false, nil
}
func (p2PolicyCapability) ValidateCostCenter(context.Context, types.CompanyID, types.CostCenterID) error {
	return nil
}
func (p2PolicyCapability) ValidateAnalyticalDimension(context.Context, types.CompanyID, string, uuid.UUID) error {
	return nil
}
func (p2PolicyCapability) MakerCheckerThreshold(context.Context, types.CompanyID) (decimal.Decimal, bool, error) {
	return decimal.Zero, false, nil
}

type p2StandardModule struct {
	standards.StandardModule
	name        string
	validateErr error
	initErr     error
	rulesErr    error
	closeErr    error
	closed      bool
}

func (m *p2StandardModule) Name() string {
	if m.name == "" {
		return "P2-MODULE"
	}
	return m.name
}
func (m *p2StandardModule) Version() string        { return "1.0.0" }
func (m *p2StandardModule) Dependencies() []string { return nil }
func (m *p2StandardModule) ValidateDependencies(standards.ModulePorts) error {
	return m.validateErr
}
func (m *p2StandardModule) Initialize(context.Context, standards.ModulePorts) error {
	return m.initErr
}
func (m *p2StandardModule) RegisterRules(events.RuleRegistrar) error { return m.rulesErr }
func (m *p2StandardModule) RunScheduledJobs(context.Context, standards.ScheduledJobRunner) error {
	return nil
}
func (m *p2StandardModule) Close(context.Context) error {
	m.closed = true
	return m.closeErr
}

type p2ReportingStore struct{ storage.ReportingStore }

func (p2ReportingStore) GetTrialBalance(context.Context, types.CompanyID, time.Time, string) (reporting.TrialBalance, error) {
	return reporting.TrialBalance{}, nil
}
func (p2ReportingStore) GetGeneralLedger(context.Context, types.CompanyID, types.AccountID, time.Time, time.Time) (reporting.GeneralLedgerReport, error) {
	return reporting.GeneralLedgerReport{}, nil
}
func (p2ReportingStore) GetBalanceSheet(context.Context, types.CompanyID, time.Time, string) (reporting.BalanceSheetReport, error) {
	return reporting.BalanceSheetReport{}, nil
}
func (p2ReportingStore) GetIncomeStatement(context.Context, types.CompanyID, time.Time, time.Time, string) (reporting.IncomeStatementReport, error) {
	return reporting.IncomeStatementReport{}, nil
}
func (p2ReportingStore) GetCashFlow(context.Context, reporting.CashFlowRequest) (reporting.CashFlowStatementReport, error) {
	return reporting.CashFlowStatementReport{CompanyID: types.NewCompanyID()}, nil
}

type p2GovernanceStore struct{ storage.GovernanceStore }

func (p2GovernanceStore) RecordAuditLog(context.Context, string, map[string]any) error { return nil }
func (p2GovernanceStore) GetApprovalFingerprint(context.Context, types.JournalEntryID) (string, error) {
	return "fingerprint", nil
}

type p2MasterDataReader struct {
	storage.MasterDataReader
	companyErr error
	accountErr error
	journalErr error
	periodErr  error
	byIDFn     func(context.Context, types.CompanyID, types.JournalID) (kernelmasterdata.Journal, error)
	periodNoFn func(context.Context, types.CompanyID, types.FiscalYearID, int) (publicmasterdata.FiscalPeriod, error)
}

func (r *p2MasterDataReader) GetCompany(context.Context, types.CompanyID) (publicmasterdata.Company, error) {
	return publicmasterdata.Company{
		ID: types.NewCompanyID(), Code: "P2", FunctionalCurrency: "USD", PresentationCurrency: "USD", Active: true,
	}, r.companyErr
}
func (r *p2MasterDataReader) GetAccount(context.Context, types.CompanyID, types.AccountID) (publicmasterdata.Account, error) {
	return publicmasterdata.Account{ID: types.NewAccountID(), Code: "1000"}, r.accountErr
}
func (r *p2MasterDataReader) GetAccountByCode(context.Context, types.CompanyID, string) (publicmasterdata.Account, error) {
	return publicmasterdata.Account{ID: types.NewAccountID(), Code: "1000"}, r.accountErr
}
func (r *p2MasterDataReader) GetJournalByCode(context.Context, types.CompanyID, string) (publicmasterdata.Journal, error) {
	return publicmasterdata.Journal{ID: types.NewJournalID(), Code: "GENERAL"}, r.journalErr
}
func (r *p2MasterDataReader) HasAccountChildren(context.Context, types.CompanyID, types.AccountID) (bool, error) {
	return false, nil
}
func (r *p2MasterDataReader) GetFiscalYear(context.Context, types.CompanyID, types.FiscalYearID) (publicmasterdata.FiscalYear, error) {
	return publicmasterdata.FiscalYear{}, nil
}
func (r *p2MasterDataReader) GetPeriodForDate(context.Context, types.CompanyID, time.Time) (publicmasterdata.FiscalPeriod, error) {
	return publicmasterdata.FiscalPeriod{}, r.periodErr
}
func (r *p2MasterDataReader) GetJournalByID(ctx context.Context, companyID types.CompanyID, journalID types.JournalID) (kernelmasterdata.Journal, error) {
	if r.byIDFn != nil {
		return r.byIDFn(ctx, companyID, journalID)
	}
	return kernelmasterdata.Journal{}, errors.New("no journal by id")
}
func (r *p2MasterDataReader) GetPeriodByNumber(ctx context.Context, companyID types.CompanyID, fyID types.FiscalYearID, number int) (publicmasterdata.FiscalPeriod, error) {
	if r.periodNoFn != nil {
		return r.periodNoFn(ctx, companyID, fyID, number)
	}
	return publicmasterdata.FiscalPeriod{}, errors.New("no period by number")
}

type p2PublicJournalByID struct {
	*p2MasterDataReader
	err error
}

func (r *p2PublicJournalByID) GetJournalByID(context.Context, types.CompanyID, types.JournalID) (publicmasterdata.Journal, error) {
	return publicmasterdata.Journal{ID: types.NewJournalID()}, r.err
}

type p2ReaderNoPeriod struct{ storage.MasterDataReader }

type p2EffectiveReader struct{ err error }

func (r *p2EffectiveReader) GetAccountAsOf(context.Context, types.CompanyID, types.AccountID, time.Time) (publicmasterdata.Account, error) {
	return publicmasterdata.Account{ID: types.NewAccountID()}, r.err
}

type p2CostCenterReader struct{ err error }

func (r *p2CostCenterReader) GetCostCenterForCompany(context.Context, types.CompanyID, types.CostCenterID) (publicmasterdata.CostCenter, error) {
	return publicmasterdata.CostCenter{}, r.err
}

type p2DimensionReader struct{ err error }

func (r *p2DimensionReader) GetAnalyticalDimensionForCompany(context.Context, types.CompanyID, publicmasterdata.AnalyticalDimensionKind, string) (publicmasterdata.AnalyticalDimensionValue, error) {
	return publicmasterdata.AnalyticalDimensionValue{}, r.err
}

type p2Sequence struct{ err error }

func (s p2Sequence) NextSequence(context.Context, types.CompanyID, types.JournalID) (int64, error) {
	return 7, s.err
}

type p2PostingWriter struct{}

func (p2PostingWriter) CommitPosting(context.Context, storage.PostingCommit) (journal.JournalEntry, error) {
	return journal.JournalEntry{ID: types.NewJournalEntryID()}, nil
}

type p2DraftStore struct{ err error }

func (s p2DraftStore) SaveDraft(context.Context, journal.JournalEntry) error { return s.err }
func (s p2DraftStore) CancelDraft(context.Context, journal.JournalEntry) error {
	return s.err
}

type p2EntryReader struct {
	entry journal.JournalEntry
	err   error
}

func (r p2EntryReader) GetEntry(context.Context, types.CompanyID, types.JournalEntryID) (journal.JournalEntry, error) {
	return r.entry, r.err
}

type p2TxRunner struct{}

func (p2TxRunner) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

type p2IdemLookup struct {
	storage.IdempotencyStore
	record storage.IdempotencyRecord
	found  bool
	err    error
}

func (s p2IdemLookup) Lookup(context.Context, types.CompanyID, string) (storage.IdempotencyRecord, bool, error) {
	return s.record, s.found, s.err
}

type p2ClosingStore struct {
	storage.YearEndClosingStore
	accounts []publicmasterdata.Account
	years    []publicmasterdata.FiscalYear
	periods  []publicmasterdata.FiscalPeriod
	entries  []journal.JournalEntry
	err      error
}

func (s *p2ClosingStore) ListAccounts(context.Context, types.CompanyID) ([]publicmasterdata.Account, error) {
	return s.accounts, s.err
}
func (s *p2ClosingStore) ListFiscalYears(context.Context, types.CompanyID) ([]publicmasterdata.FiscalYear, error) {
	return s.years, s.err
}
func (s *p2ClosingStore) ListFiscalPeriods(context.Context, types.CompanyID, types.FiscalYearID) ([]publicmasterdata.FiscalPeriod, error) {
	return s.periods, s.err
}
func (s *p2ClosingStore) ListEntriesByDateRange(context.Context, types.CompanyID, time.Time, time.Time) ([]journal.JournalEntry, error) {
	return s.entries, s.err
}
func (s *p2ClosingStore) UpdateFiscalPeriodState(context.Context, types.CompanyID, types.FiscalPeriodID, publicmasterdata.PeriodState, string, string) error {
	return s.err
}
func (s *p2ClosingStore) UpdateFiscalYearState(context.Context, types.CompanyID, types.FiscalYearID, publicmasterdata.FiscalYearState, string, string) error {
	return s.err
}
func (s *p2ClosingStore) SaveYearEndCloseRecord(context.Context, storage.YearEndCloseRecord) error {
	return s.err
}
func (s *p2ClosingStore) GetYearEndCloseRecord(context.Context, types.CompanyID, types.FiscalYearID) (storage.YearEndCloseRecord, bool, error) {
	return storage.YearEndCloseRecord{}, false, s.err
}

type p2UseCases struct {
	publicapplication.Service
	createFn func(context.Context, publicapplication.CreateDraftCommand) (publicapplication.EntryResult, error)
	postFn   func(context.Context, publicapplication.PostEntryCommand) (publicapplication.EntryResult, error)
	getFn    func(context.Context, publicapplication.GetEntryQuery) (journal.JournalEntry, error)
}

func (u *p2UseCases) CreateDraft(ctx context.Context, c publicapplication.CreateDraftCommand) (publicapplication.EntryResult, error) {
	if u.createFn != nil {
		return u.createFn(ctx, c)
	}
	return publicapplication.EntryResult{Entry: journal.JournalEntry{ID: types.NewJournalEntryID()}}, nil
}
func (u *p2UseCases) Post(ctx context.Context, c publicapplication.PostEntryCommand) (publicapplication.EntryResult, error) {
	if u.postFn != nil {
		return u.postFn(ctx, c)
	}
	return publicapplication.EntryResult{Entry: journal.JournalEntry{ID: c.EntryID, Status: journal.EntryPosted}}, nil
}
func (u *p2UseCases) GetEntry(ctx context.Context, q publicapplication.GetEntryQuery) (journal.JournalEntry, error) {
	if u.getFn != nil {
		return u.getFn(ctx, q)
	}
	return journal.JournalEntry{}, publicapplication.NewError(publicapplication.ErrNotFound, "not found", nil)
}
func (u *p2UseCases) CancelDraft(context.Context, publicapplication.CancelDraftCommand) (publicapplication.EntryResult, error) {
	return publicapplication.EntryResult{}, nil
}

type p2YearEndClosing struct{}

func (p2YearEndClosing) ExecuteYearEndClosing(context.Context, publicapplication.ExecuteYearEndClosingCommand) (publicapplication.YearEndClosingResult, error) {
	return publicapplication.YearEndClosingResult{}, nil
}
func (p2YearEndClosing) ReopenYearEndClosing(context.Context, publicapplication.ReopenYearEndClosingCommand) (publicapplication.ReopenYearEndClosingResult, error) {
	return publicapplication.ReopenYearEndClosingResult{}, nil
}

type p2IngestionStore struct {
	getFn          func(context.Context, uuid.UUID, string) (publicevents.Ingestion, bool, error)
	scopedGetFn    func(context.Context, uuid.UUID, string, string) (publicevents.Ingestion, bool, error)
	claimFn        func(context.Context, publicevents.Ingestion) (publicevents.Ingestion, bool, error)
	completeFn     func(context.Context, uuid.UUID, string, journal.JournalEntry) error
	scopedComplete func(context.Context, uuid.UUID, string, string, string, journal.JournalEntry) error
	failFn         func(context.Context, uuid.UUID, string, string) error
	scopedFailFn   func(context.Context, uuid.UUID, string, string, string, string) error
	classifiedFail func(context.Context, uuid.UUID, string, string, string, string) error
}

func (s *p2IngestionStore) GetPublicEventIngestion(ctx context.Context, companyID uuid.UUID, key string) (publicevents.Ingestion, bool, error) {
	if s.getFn != nil {
		return s.getFn(ctx, companyID, key)
	}
	return publicevents.Ingestion{}, false, nil
}
func (s *p2IngestionStore) GetPublicEventIngestionForSource(ctx context.Context, companyID uuid.UUID, source, key string) (publicevents.Ingestion, bool, error) {
	if s.scopedGetFn != nil {
		return s.scopedGetFn(ctx, companyID, source, key)
	}
	return s.GetPublicEventIngestion(ctx, companyID, key)
}
func (s *p2IngestionStore) ClaimPublicEventIngestion(ctx context.Context, rec publicevents.Ingestion) (publicevents.Ingestion, bool, error) {
	if s.claimFn != nil {
		return s.claimFn(ctx, rec)
	}
	return rec, true, nil
}
func (s *p2IngestionStore) CompletePublicEventIngestion(ctx context.Context, companyID uuid.UUID, key string, entry journal.JournalEntry) error {
	if s.completeFn != nil {
		return s.completeFn(ctx, companyID, key, entry)
	}
	return nil
}
func (s *p2IngestionStore) CompletePublicEventIngestionForSource(ctx context.Context, companyID uuid.UUID, source, key, hash string, entry journal.JournalEntry) error {
	if s.scopedComplete != nil {
		return s.scopedComplete(ctx, companyID, source, key, hash, entry)
	}
	return s.CompletePublicEventIngestion(ctx, companyID, key, entry)
}
func (s *p2IngestionStore) FailPublicEventIngestion(ctx context.Context, companyID uuid.UUID, key, reason string) error {
	if s.failFn != nil {
		return s.failFn(ctx, companyID, key, reason)
	}
	return nil
}
func (s *p2IngestionStore) FailPublicEventIngestionForSource(ctx context.Context, companyID uuid.UUID, source, key, class, reason string) error {
	if s.scopedFailFn != nil {
		return s.scopedFailFn(ctx, companyID, source, key, class, reason)
	}
	return s.FailPublicEventIngestion(ctx, companyID, key, reason)
}
func (s *p2IngestionStore) FailPublicEventIngestionClassified(ctx context.Context, companyID uuid.UUID, source, key, class, reason string) error {
	if s.classifiedFail != nil {
		return s.classifiedFail(ctx, companyID, source, key, class, reason)
	}
	return s.FailPublicEventIngestionForSource(ctx, companyID, source, key, class, reason)
}

func p2CompanyID() types.CompanyID { return types.NewCompanyID() }

func p2PostedEntry(companyID types.CompanyID, id types.JournalEntryID) journal.JournalEntry {
	return journal.JournalEntry{ID: id, CompanyID: companyID, Status: journal.EntryPosted}
}

func TestP2OptionsAndBootstrapValidation(t *testing.T) {
	b := &EngineBuilder{}
	for name, opt := range map[string]Option{
		"reporting": WithReporting(nil), "governance": WithGovernance(nil),
		"observer": WithObserver(nil), "authorizer": WithAuthorizer(nil),
		"standard": WithStandard(nil), "cache ttl": WithPolicyCacheTTL(-1),
		"wait timeout":  WithEventIdempotencyWaitPolicy(-1, 0),
		"wait interval": WithEventIdempotencyWaitPolicy(0, -1),
		"arap":          WithSubledgerARAP(publicsubledger.ARAPStores{}),
	} {
		if err := opt(b); err == nil {
			t.Fatalf("%s: expected option error", name)
		}
	}
	if err := WithStoragePorts(storage.Ports{})(b); err != nil || !b.portsConfigured {
		t.Fatalf("WithStoragePorts: err=%v configured=%v", err, b.portsConfigured)
	}
	if err := WithReporting(p2ReportingStore{})(b); err != nil || b.reportingStore == nil {
		t.Fatalf("WithReporting: err=%v", err)
	}
	if err := WithGovernance(p2GovernanceStore{})(b); err != nil || b.governanceStore == nil {
		t.Fatalf("WithGovernance: err=%v", err)
	}
	observer := &p2Observer{}
	if err := WithObserver(observer)(b); err != nil || b.observer != observer {
		t.Fatalf("WithObserver: err=%v", err)
	}
	if err := WithAuthorizer(p2Authorizer{})(b); err != nil || b.authorizer == nil {
		t.Fatalf("WithAuthorizer: err=%v", err)
	}
	mod := &p2StandardModule{}
	if err := WithStandard(mod)(b); err != nil || len(b.standards) != 1 {
		t.Fatalf("WithStandard: err=%v modules=%d", err, len(b.standards))
	}
	caps := storage.PostingCapabilities{ApprovalGate: p2Authorizer{}}
	if err := WithPostingCapabilities(caps)(b); err != nil {
		t.Fatalf("WithPostingCapabilities: %v", err)
	}
	if err := WithPolicyCacheTTL(time.Second)(b); err != nil || b.postingConfig.PolicyCacheTTL != time.Second {
		t.Fatalf("WithPolicyCacheTTL: err=%v", err)
	}
	if err := WithEventIdempotencyWaitPolicy(3*time.Millisecond, time.Millisecond)(b); err != nil || b.eventWaitInterval != time.Millisecond {
		t.Fatalf("WithEventIdempotencyWaitPolicy: err=%v", err)
	}
	arap := publicsubledger.ARAPStores{
		Receivables: struct {
			publicsubledger.ReceivableStore
		}{},
		Payables: struct{ publicsubledger.PayableStore }{},
		Balance:  struct{ storage.BalanceReader }{},
	}
	if err := WithSubledgerARAP(arap)(b); err != nil || !b.arapStoresConfigured {
		t.Fatalf("WithSubledgerARAP: err=%v", err)
	}

	bootstrap := &EngineBuilder{}
	if err := WithRequiredCapabilities("unknown")(bootstrap); err == nil {
		t.Fatal("expected unknown bootstrap capability error")
	}
	if err := WithRequiredCapabilities(BootstrapStandards, BootstrapStandards)(bootstrap); err != nil {
		t.Fatalf("duplicate capability should be ignored: %v", err)
	}
	if len(bootstrap.requiredCapabilities) != 1 || !bootstrap.hasRequiredCapability(BootstrapStandards) {
		t.Fatalf("required capabilities = %v", bootstrap.requiredCapabilities)
	}
	if bootstrap.hasRequiredCapability(BootstrapGovernance) {
		t.Fatal("unexpected governance capability")
	}
	if err := bootstrap.validateReportingAvailability(); err == nil {
		t.Fatal("expected missing reporting error")
	}
	bootstrap.reportingStore = p2ReportingStore{}
	if err := bootstrap.validateReportingAvailability(); err != nil {
		t.Fatalf("reporting store should satisfy availability: %v", err)
	}
	bootstrap.reportingStore = nil
	bootstrap.portsConfigured = true
	bootstrap.ports = storage.Ports{Balance: struct{ storage.BalanceReader }{}, MasterData: &p2MasterDataReader{}, Entries: p2EntryReader{}}
	if err := bootstrap.validateReportingAvailability(); err != nil {
		t.Fatalf("reporting ports should satisfy availability: %v", err)
	}
	if err := bootstrap.validateGovernanceAvailability(); err == nil {
		t.Fatal("expected missing governance error")
	}
	bootstrap.ports.Audit = struct{ storage.AuditWriter }{}
	if err := bootstrap.validateGovernanceAvailability(); err != nil {
		t.Fatalf("audit port should satisfy governance: %v", err)
	}
	if err := bootstrap.validateStandardsAvailability(); err == nil {
		t.Fatal("expected missing standards error")
	}
	if err := bootstrap.validateEventIngestionAvailability(); err == nil {
		t.Fatal("expected missing event ingestion error")
	}
	bootstrap.ports.Events = publicevents.Stores{Idempotency: &p2IngestionStore{}, Mappings: struct{ publicevents.MappingStore }{}}
	if err := bootstrap.validateEventIngestionAvailability(); err != nil {
		t.Fatalf("event stores should satisfy availability: %v", err)
	}
	if err := bootstrap.validateSubledgersAvailability(); err == nil {
		t.Fatal("expected missing subledgers error")
	}
	bootstrap.arapStoresConfigured = true
	if err := bootstrap.validateSubledgersAvailability(); err != nil {
		t.Fatalf("arap config should satisfy availability: %v", err)
	}
	bootstrap.arapStoresConfigured = false
	bootstrap.ports.Private = storage.PrivateExtensions{"accounting_engine.posting_policy": struct{}{}}
	if err := bootstrap.validatePostingPolicyAvailability(); err != nil {
		t.Fatalf("private policy should satisfy availability: %v", err)
	}
	bootstrap.ports.Private = nil
	bootstrap.postingCaps.MakerCheckerPolicy = p2PolicyCapability{}
	if err := bootstrap.validatePostingPolicyAvailability(); err != nil {
		t.Fatalf("posting caps should satisfy availability: %v", err)
	}
	bootstrap.postingCaps = storage.PostingCapabilities{}
	if err := bootstrap.validatePostingPolicyAvailability(); err == nil {
		t.Fatal("expected missing posting policy error")
	}
	bootstrap.portsConfigured = false
	bootstrap.requiredCapabilities = []BootstrapCapability{BootstrapStandards, BootstrapReporting}
	if errs := bootstrap.validateRequiredCapabilities(); len(errs) != 2 {
		t.Fatalf("expected two capability errors, got %d: %v", len(errs), errs)
	}

	if (PostingConfig{}).requiresCapabilities() {
		t.Fatal("empty posting config should not require capabilities")
	}
	if !(PostingConfig{RequireApproval: true}).requiresCapabilities() {
		t.Fatal("approval config should require capabilities")
	}
	invalidConfig := &EngineBuilder{postingConfig: PostingConfig{
		RequireApproval: true, RequireFinancialDimensions: true,
		RequireMakerCheckerPolicy: true, RequirePolicy: true,
	}}
	if err := invalidConfig.validatePostingConfig(); err == nil {
		t.Fatal("expected missing posting capabilities error")
	}
	invalidConfig.postingCaps.ApprovalGate = p2Authorizer{}
	invalidConfig.postingCaps.FinancialDimensions = p2PolicyCapability{}
	invalidConfig.postingCaps.MakerCheckerPolicy = p2PolicyCapability{}
	invalidConfig.portsConfigured = true
	invalidConfig.ports.Private = storage.PrivateExtensions{"accounting_engine.posting_policy": struct{}{}}
	if err := invalidConfig.validatePostingConfig(); err != nil {
		t.Fatalf("validated posting config rejected: %v", err)
	}
}

func TestP2BuilderServiceHelpers(t *testing.T) {
	ctx := context.Background()
	companyID := p2CompanyID()

	disabled := &disabledReportingService{}
	if _, err := disabled.GetTrialBalance(ctx, companyID, time.Now(), "USD"); !errors.Is(err, ErrCapabilityNotImplemented) {
		t.Fatalf("disabled trial balance: %v", err)
	}
	if _, err := disabled.GetBalanceSheet(ctx, companyID, time.Now(), "USD"); !errors.Is(err, ErrCapabilityNotImplemented) {
		t.Fatalf("disabled balance sheet: %v", err)
	}
	if _, err := disabled.GetIncomeStatement(ctx, companyID, time.Now(), time.Now(), "USD"); !errors.Is(err, ErrCapabilityNotImplemented) {
		t.Fatalf("disabled income statement: %v", err)
	}
	if _, err := disabled.GetGeneralLedger(ctx, companyID, types.NewAccountID(), time.Now(), time.Now()); !errors.Is(err, ErrCapabilityNotImplemented) {
		t.Fatalf("disabled ledger: %v", err)
	}
	if _, err := disabled.GetCashFlow(ctx, reporting.CashFlowRequest{}); !errors.Is(err, ErrCapabilityNotImplemented) {
		t.Fatalf("disabled cash flow: %v", err)
	}
	if _, err := disabled.GetDisclosures(ctx, reporting.DisclosureQuery{}); !errors.Is(err, ErrCapabilityNotImplemented) {
		t.Fatalf("disabled disclosures: %v", err)
	}

	reportingSvc := &defaultReportingService{}
	if _, err := reportingSvc.GetTrialBalance(ctx, companyID, time.Now(), "USD"); !errors.Is(err, ErrCapabilityNotImplemented) {
		t.Fatalf("nil reporting store: %v", err)
	}
	if _, err := reportingSvc.GetBalanceSheet(ctx, companyID, time.Now(), "USD"); !errors.Is(err, ErrCapabilityNotImplemented) {
		t.Fatalf("nil reporting store balance sheet: %v", err)
	}
	if _, err := reportingSvc.GetIncomeStatement(ctx, companyID, time.Now(), time.Now(), "USD"); !errors.Is(err, ErrCapabilityNotImplemented) {
		t.Fatalf("nil reporting store income: %v", err)
	}
	if _, err := reportingSvc.GetGeneralLedger(ctx, companyID, types.NewAccountID(), time.Now(), time.Now()); !errors.Is(err, ErrCapabilityNotImplemented) {
		t.Fatalf("nil reporting store ledger: %v", err)
	}
	if _, err := reportingSvc.GetDisclosures(ctx, reporting.DisclosureQuery{}); !errors.Is(err, ErrCapabilityNotImplemented) {
		t.Fatalf("default disclosures: %v", err)
	}
	reportingSvc.store = p2ReportingStore{}
	if _, err := reportingSvc.GetTrialBalance(ctx, companyID, time.Now(), "USD"); err != nil {
		t.Fatalf("reporting store trial balance: %v", err)
	}
	if _, err := reportingSvc.GetBalanceSheet(ctx, companyID, time.Now(), "USD"); err != nil {
		t.Fatalf("reporting store balance sheet: %v", err)
	}
	if _, err := reportingSvc.GetIncomeStatement(ctx, companyID, time.Now(), time.Now(), "USD"); err != nil {
		t.Fatalf("reporting store income: %v", err)
	}
	if _, err := reportingSvc.GetGeneralLedger(ctx, companyID, types.NewAccountID(), time.Now(), time.Now()); err != nil {
		t.Fatalf("reporting store ledger: %v", err)
	}
	if report, err := reportingSvc.GetCashFlow(ctx, reporting.CashFlowRequest{}); err != nil || report.CompanyID == (types.CompanyID{}) {
		t.Fatalf("reporting store cash flow: %+v %v", report, err)
	}
	if _, err := (&defaultReportingService{store: struct{ storage.ReportingStore }{}}).GetCashFlow(ctx, reporting.CashFlowRequest{}); !errors.Is(err, ErrCapabilityNotImplemented) {
		t.Fatalf("unsupported cash flow: %v", err)
	}

	disabledGov := &disabledGovernanceService{}
	if err := disabledGov.RecordAuditLog(ctx, "event", nil); !errors.Is(err, ErrCapabilityNotImplemented) {
		t.Fatalf("disabled governance audit: %v", err)
	}
	if _, err := disabledGov.GetApprovalFingerprint(ctx, types.NewJournalEntryID()); !errors.Is(err, ErrCapabilityNotImplemented) {
		t.Fatalf("disabled governance fingerprint: %v", err)
	}
	gov := &defaultGovernanceService{}
	if err := gov.RecordAuditLog(ctx, "event", nil); !errors.Is(err, ErrCapabilityNotImplemented) {
		t.Fatalf("nil governance store audit: %v", err)
	}
	if _, err := gov.GetApprovalFingerprint(ctx, types.NewJournalEntryID()); !errors.Is(err, ErrCapabilityNotImplemented) {
		t.Fatalf("nil governance store fingerprint: %v", err)
	}
	gov.store = p2GovernanceStore{}
	if err := gov.RecordAuditLog(ctx, "event", nil); err != nil {
		t.Fatalf("governance store audit: %v", err)
	}
	if fp, err := gov.GetApprovalFingerprint(ctx, types.NewJournalEntryID()); err != nil || fp != "fingerprint" {
		t.Fatalf("governance fingerprint: %q %v", fp, err)
	}

	nilStandards := &defaultStandardsService{}
	if got := nilStandards.Diagnostics(); got.ActiveModules != nil {
		t.Fatalf("nil registry diagnostics = %+v", got)
	}
	if nilStandards.HasCapability(standards.CapReporting) {
		t.Fatal("nil registry should report no capability")
	}
	registry := standards.NewRegistry()
	standardsSvc := &defaultStandardsService{registry: registry, scheduler: standards.NewDefaultScheduler()}
	if got := standardsSvc.Registry(); got != registry {
		t.Fatal("registry getter did not return configured registry")
	}
	_ = standardsSvc.Diagnostics()
	_ = standardsSvc.HasCapability(standards.CapLedgerPosting)
	if standardsSvc.Scheduler() == nil {
		t.Fatal("scheduler getter returned nil")
	}

	principal := publicapplication.Principal{ActorID: "actor", CompanyIDs: []types.CompanyID{companyID}}
	principalCtx := publicapplication.WithPrincipal(ctx, principal)
	principalCtx = publicapplication.WithRequestMetadata(principalCtx, publicapplication.RequestMetadata{
		CorrelationID: "corr", IdempotencyKey: "key", SourceSystem: "source",
	})
	cmdCtx := commandContext(principalCtx, companyID, "fallback", "sdk", false)
	if cmdCtx.Principal.ActorID != "actor" || cmdCtx.SourceSystem != "source" || cmdCtx.CorrelationID != "corr" || cmdCtx.IdempotencyKey != "key" {
		t.Fatalf("principal command context = %+v", cmdCtx)
	}
	cmdCtx = commandContext(ctx, companyID, "fallback", "sdk", true)
	if cmdCtx.Principal.ActorID != "" {
		t.Fatalf("required principal was synthesized: %+v", cmdCtx)
	}
	cmdCtx = commandContext(ctx, companyID, "fallback", "sdk", false)
	if cmdCtx.Principal.ActorID != "fallback" || cmdCtx.SourceSystem != "sdk" {
		t.Fatalf("fallback command context = %+v", cmdCtx)
	}
	if got := metadataSource(publicapplication.RequestMetadata{SourceSystem: "  "}, "fallback"); got != "fallback" {
		t.Fatalf("metadataSource fallback = %q", got)
	}

	nilLedger := &defaultLedgerService{}
	if _, err := nilLedger.PostEntry(ctx, journal.JournalEntry{}); !errors.Is(err, ErrCapabilityNotImplemented) {
		t.Fatalf("nil ledger post: %v", err)
	}
	if _, err := nilLedger.GetEntry(ctx, companyID, types.NewJournalEntryID()); !errors.Is(err, ErrCapabilityNotImplemented) {
		t.Fatalf("nil ledger get: %v", err)
	}
	if _, err := nilLedger.ReverseEntry(ctx, companyID, types.NewJournalEntryID(), "reason"); !errors.Is(err, ErrCapabilityNotImplemented) {
		t.Fatalf("nil ledger reverse: %v", err)
	}

	ports := memory.New().Ports()
	inst := &engineInstance{
		ports: ports, reportingStore: p2ReportingStore{}, governanceStore: p2GovernanceStore{},
		useCases: &p2UseCases{}, standardsRegistry: standards.NewRegistry(),
		scheduler: standards.NewDefaultScheduler(), closeOnce: sync.Once{},
		eventService: &defaultEventService{}, yearEndClosing: p2YearEndClosing{},
	}
	if inst.Ledger() == nil || inst.Reporting() == nil || inst.Governance() == nil ||
		inst.Events() == nil || inst.Standards() == nil || inst.Scheduler() == nil ||
		inst.YearEndClosing() == nil || inst.DraftCancellation() == nil ||
		inst.Consolidation() == nil || inst.Consolidation() == nil || inst.Reconciliation() == nil {
		t.Fatal("engineInstance returned nil service")
	}
	if optionalYearEndClosingService(&p2UseCases{}) != nil {
		t.Fatal("unexpected year-end closing capability on plain service")
	}

	closeMod := &p2StandardModule{name: "CLOSE-FAIL", closeErr: errors.New("close failed")}
	closingInst := &engineInstance{standards: []standards.StandardModule{closeMod}, closeOnce: sync.Once{}}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := closingInst.CloseWithContext(canceled); err == nil {
		t.Fatal("expected close error for canceled context and failing module")
	}
	if err := closingInst.Close(); err == nil {
		t.Fatal("expected remembered close error")
	}
	if !closeMod.closed {
		t.Fatal("module was not closed")
	}

	first := &p2StandardModule{name: "FIRST", closeErr: errors.New("rollback failed")}
	failing := &p2StandardModule{name: "FAILING", initErr: errors.New("init failed"), closeErr: errors.New("rollback failed")}
	builder := &EngineBuilder{ports: ports, portsConfigured: true, standards: []standards.StandardModule{first, failing}}
	if errs := builder.initializeStandards(ctx, events.NewRouter()); len(errs) != 2 {
		t.Fatalf("rollback errors = %v, want two", errs)
	}
	if !first.closed {
		t.Fatal("initialized module was not rolled back")
	}
}

func TestP2ApplicationHelpers(t *testing.T) {
	ctx := context.Background()
	companyID := p2CompanyID()

	svc, err := NewApplicationServiceWithObserver(storage.Ports{}, PostingConfig{}, nil, nil)
	if err == nil || svc != nil {
		t.Fatalf("invalid ports: service=%v err=%v", svc, err)
	}
	ports := memory.New().Ports()
	svc, err = NewApplicationServiceWithObserver(ports, PostingConfig{}, nil, nil)
	if err != nil || svc == nil {
		t.Fatalf("valid ports: service=%v err=%v", svc, err)
	}
	observer := &p2Observer{}
	svc, err = NewApplicationServiceWithObserver(ports, PostingConfig{}, nil, observer)
	if err != nil {
		t.Fatalf("observer service: %v", err)
	}
	app := svc.(*applicationService)
	if app.observer != observer {
		t.Fatal("observer was not installed")
	}

	reader := &p2MasterDataReader{}
	adapter := masterDataReaderAdapter{reader: reader}
	if _, err := adapter.GetAccount(ctx, companyID, types.NewAccountID()); err != nil {
		t.Fatalf("adapter account: %v", err)
	}
	if _, err := adapter.GetAccountByCode(ctx, companyID, "1000"); err != nil {
		t.Fatalf("adapter account by code: %v", err)
	}
	if _, err := adapter.GetJournalByCode(ctx, companyID, "GENERAL"); err != nil {
		t.Fatalf("adapter journal: %v", err)
	}
	reader.journalErr = errors.New("journal missing")
	if _, err := adapter.GetJournalByCode(ctx, companyID, "GENERAL"); err == nil {
		t.Fatal("expected adapter journal error")
	}
	if _, err := adapter.GetJournalByID(ctx, companyID, types.NewJournalID()); err == nil {
		t.Fatal("expected journal reader required error")
	}
	reader.byIDFn = func(context.Context, types.CompanyID, types.JournalID) (kernelmasterdata.Journal, error) {
		return kernelmasterdata.Journal{ID: types.NewJournalID()}, nil
	}
	if _, err := adapter.GetJournalByID(ctx, companyID, types.NewJournalID()); err != nil {
		t.Fatalf("kernel journal by id: %v", err)
	}
	pubReader := &p2PublicJournalByID{p2MasterDataReader: reader}
	adapter.reader = pubReader
	if _, err := adapter.GetJournalByID(ctx, companyID, types.NewJournalID()); err != nil {
		t.Fatalf("public journal by id: %v", err)
	}
	pubReader.err = errors.New("public journal error")
	if _, err := adapter.GetJournalByID(ctx, companyID, types.NewJournalID()); err == nil {
		t.Fatal("expected public journal by id error")
	}
	adapter.reader = &p2ReaderNoPeriod{}
	if _, err := adapter.GetPeriodByNumber(ctx, companyID, types.NewFiscalYearID(), 13); !errors.Is(err, types.ErrCapabilityNotImplemented) {
		t.Fatalf("period by number capability error = %v", err)
	}
	adapter.reader = reader
	periodReader := &p2MasterDataReader{periodNoFn: func(context.Context, types.CompanyID, types.FiscalYearID, int) (publicmasterdata.FiscalPeriod, error) {
		return publicmasterdata.FiscalPeriod{ID: types.NewFiscalPeriodID()}, nil
	}}
	adapter.reader = periodReader
	if _, err := adapter.GetPeriodByNumber(ctx, companyID, types.NewFiscalYearID(), 13); err != nil {
		t.Fatalf("period by number: %v", err)
	}
	adapter.reader = reader
	allowAdapter := masterDataReaderAdapter{reader: reader, allowSyntheticDefaultJournal: true}
	if _, err := allowAdapter.GetJournalByID(ctx, companyID, types.NewJournalID()); err != nil {
		t.Fatalf("synthetic journal fallback: %v", err)
	}

	capReader := capabilityMasterDataReader{masterDataReaderAdapter: adapter}
	if _, err := capReader.GetAccountAsOf(ctx, companyID, types.NewAccountID(), time.Now()); err != nil {
		t.Fatalf("account as of fallback: %v", err)
	}
	if _, err := capReader.GetCostCenterForCompany(ctx, companyID, types.NewCostCenterID()); !errors.Is(err, kernelposting.ErrDimensionReaderRequired) {
		t.Fatalf("cost center missing: %v", err)
	}
	if _, err := capReader.GetAnalyticalDimensionForCompany(ctx, companyID, kernelmasterdata.DimensionProject, "x"); !errors.Is(err, kernelposting.ErrDimensionReaderRequired) {
		t.Fatalf("dimension missing: %v", err)
	}
	capReader.effective = &p2EffectiveReader{}
	if _, err := capReader.GetAccountAsOf(ctx, companyID, types.NewAccountID(), time.Now()); err != nil {
		t.Fatalf("effective account: %v", err)
	}
	capReader.costCenters = &p2CostCenterReader{err: publicmasterdata.ErrInvalidCostCenter}
	if _, err := capReader.GetCostCenterForCompany(ctx, companyID, types.NewCostCenterID()); !errors.Is(err, kernelmasterdata.ErrInvalidCostCenter) {
		t.Fatalf("cost center error mapping = %v", err)
	}
	capReader.dimensions = &p2DimensionReader{err: publicmasterdata.ErrInvalidAnalyticalDimension}
	if _, err := capReader.GetAnalyticalDimensionForCompany(ctx, companyID, kernelmasterdata.DimensionProject, "x"); !errors.Is(err, kernelmasterdata.ErrInvalidAnalyticalDimension) {
		t.Fatalf("dimension error mapping = %v", err)
	}

	failingPorts := storage.Ports{
		Drafts: p2DraftStore{}, Entries: p2EntryReader{}, Posting: p2PostingWriter{},
		Sequences: p2Sequence{err: errors.New("sequence unavailable")}, MasterData: reader,
	}
	failingApp := newApplication(failingPorts, PostingConfig{}, nil)
	failingApp.observer = observer
	if _, err := failingApp.executePosting(ctx, journal.JournalEntry{CompanyID: companyID}, kernelposting.PostOptions{}); err == nil {
		t.Fatal("expected sequence error on posting")
	}
	if _, err := failingApp.executeReversalPosting(ctx, types.NewJournalEntryID(), journal.JournalEntry{CompanyID: companyID}, kernelposting.PostOptions{}); err == nil {
		t.Fatal("expected sequence error on reversal")
	}
	if observer.posts != 1 || observer.reversals != 1 {
		t.Fatalf("observer post=%d reversal=%d", observer.posts, observer.reversals)
	}
	if idempotency := commitIdempotency(kernelposting.PostOptions{}, storage.OperationPost); idempotency != nil {
		t.Fatalf("unexpected idempotency: %+v", idempotency)
	}
	for _, op := range []storage.PostingOperation{storage.OperationPost, storage.OperationReverse, storage.OperationRevaluation} {
		opts := kernelposting.PostOptions{Scope: kernelposting.IdempotencyScope{Key: "key"}, CallerFingerprint: "hash"}
		idempotency := commitIdempotency(opts, op)
		if idempotency == nil || idempotency.Key != "key" || idempotency.PayloadHash != "hash" {
			t.Fatalf("idempotency for %s = %+v", op, idempotency)
		}
	}
	if idempotency := commitIdempotency(kernelposting.PostOptions{
		Scope: kernelposting.IdempotencyScope{Key: "key", Operation: kernelposting.IdempotencyPosting}, CallerFingerprint: "hash",
	}, storage.OperationPost); idempotency == nil {
		t.Fatal("valid scope should be preserved")
	}
	app.observer = nil
	app.observePosting(ctx, "corr", companyID, false, time.Now(), nil)

	app.ports.MasterData = &p2MasterDataReader{companyErr: errors.New("company unavailable")}
	app.policyStore = nil
	if err := app.ensurePolicies(ctx, companyID, time.Now()); err == nil {
		t.Fatal("expected ensurePolicies company error")
	}

	authApp := &applicationService{authorizer: p2Authorizer{err: errors.New("denied")}}
	authCtx := publicapplication.Context{Principal: publicapplication.Principal{
		ActorID: "actor", CompanyIDs: []types.CompanyID{companyID},
	}}
	if err := authApp.authorize(ctx, authCtx, companyID, publicapplication.ActionPost, nil, ""); !errors.Is(err, publicapplication.ErrForbidden) {
		t.Fatalf("authorizer denial = %v", err)
	}
	authCtx.Principal.ActorID = ""
	if err := authApp.authorize(ctx, authCtx, companyID, publicapplication.ActionPost, nil, ""); !errors.Is(err, publicapplication.ErrUnauthorized) {
		t.Fatalf("missing actor = %v", err)
	}
	noAuth := &applicationService{}
	authCtx.Principal = publicapplication.Principal{ActorID: "actor", CompanyIDs: []types.CompanyID{types.NewCompanyID()}}
	if err := noAuth.authorize(ctx, authCtx, companyID, publicapplication.ActionPost, nil, ""); !errors.Is(err, publicapplication.ErrForbidden) {
		t.Fatalf("company membership = %v", err)
	}
	if err := noAuth.authorize(ctx, publicapplication.Context{}, types.CompanyID{}, publicapplication.ActionRead, nil, ""); !errors.Is(err, publicapplication.ErrValidation) {
		t.Fatalf("empty company = %v", err)
	}

	entry := journal.JournalEntry{CompanyID: companyID, ID: types.NewJournalEntryID()}
	opts := kernelposting.PostOptions{IdempotencyKey: "key", ActorID: "actor", CallerFingerprint: "hash"}
	scope, err := kernelposting.BuildIdempotencyScope(entry, opts)
	if err != nil {
		t.Fatalf("build idempotency scope: %v", err)
	}
	fingerprint, err := kernelposting.FingerprintCallerPayload(entry, opts, scope)
	if err != nil {
		t.Fatalf("fingerprint caller payload: %v", err)
	}
	lookup := &p2IdemLookup{record: storage.IdempotencyRecord{PayloadHash: fingerprint + "-other"}, found: true}
	idempotentApp := &applicationService{ports: storage.Ports{Idempotency: lookup, Entries: p2EntryReader{}}}
	if _, _, err := idempotentApp.checkIdempotencyBeforePost(ctx, entry, opts); !errors.Is(err, publicapplication.ErrConflict) {
		t.Fatalf("payload conflict = %v", err)
	}
	lookup.record = storage.IdempotencyRecord{Status: storage.IdempotencyCompleted, PayloadHash: fingerprint}
	if _, _, err := idempotentApp.checkIdempotencyBeforePost(ctx, entry, opts); err == nil {
		t.Fatal("expected completed record without entry id error")
	}
	entryID := types.NewJournalEntryID()
	lookup.record.EntryID = &entryID
	if replay, found, err := idempotentApp.checkIdempotencyBeforePost(ctx, entry, opts); err != nil || !found || replay.ID != entryID {
		// The entry reader returns an empty entry, so only the found flag matters.
		if err != nil || !found {
			t.Fatalf("completed replay = %+v %v %v", replay, found, err)
		}
	}
	lookup.record = storage.IdempotencyRecord{Status: storage.IdempotencyStarted, PayloadHash: fingerprint}
	if _, _, err := idempotentApp.checkIdempotencyBeforePost(ctx, entry, opts); !errors.Is(err, publicapplication.ErrConflict) {
		t.Fatalf("started record = %v", err)
	}
	lookup.record = storage.IdempotencyRecord{Status: storage.IdempotencyTerminal, PayloadHash: fingerprint}
	if _, _, err := idempotentApp.checkIdempotencyBeforePost(ctx, entry, opts); !errors.Is(err, publicapplication.ErrConflict) {
		t.Fatalf("terminal record = %v", err)
	}
	lookup.record = storage.IdempotencyRecord{Status: storage.IdempotencyRetryable, PayloadHash: fingerprint}
	if _, found, err := idempotentApp.checkIdempotencyBeforePost(ctx, entry, opts); err != nil || found {
		t.Fatalf("retryable record = %v %v", found, err)
	}
	lookup.err = errors.New("lookup failed")
	if _, _, err := idempotentApp.checkIdempotencyBeforePost(ctx, entry, opts); err == nil {
		t.Fatal("expected lookup error")
	}
	if _, found, err := idempotentApp.checkIdempotencyBeforePost(ctx, entry, kernelposting.PostOptions{}); err != nil || found {
		t.Fatalf("empty idempotency key = %v %v", found, err)
	}

	if err := (&applicationService{}).WithinTransaction(ctx, companyID, nil); !errors.Is(err, ErrCapabilityNotImplemented) {
		t.Fatalf("missing tx runner = %v", err)
	}
	txApp := &applicationService{ports: storage.Ports{TxRunner: p2TxRunner{}}}
	if err := txApp.WithinTransaction(ctx, types.CompanyID{}, func(context.Context) error { return nil }); err == nil {
		t.Fatal("expected empty company error")
	}
	called := false
	if err := txApp.WithinTransaction(ctx, companyID, func(context.Context) error { called = true; return nil }); err != nil || !called {
		t.Fatalf("transaction callback = %v %v", called, err)
	}
	findApp := &applicationService{ports: storage.Ports{Entries: p2EntryReader{}}}
	if _, found, err := findApp.FindPostedEntryByIdempotencyKey(ctx, companyID, "key"); !errors.Is(err, ErrCapabilityNotImplemented) || found {
		t.Fatalf("no idempotency lookup = %v %v", found, err)
	}
	findApp.ports.Idempotency = &p2IdemLookup{record: storage.IdempotencyRecord{Status: storage.IdempotencyCompleted}}
	if _, found, err := findApp.FindPostedEntryByIdempotencyKey(ctx, companyID, "key"); err != nil || found {
		t.Fatalf("incomplete completed record = %v %v", found, err)
	}

	cancelApp := &applicationService{ports: storage.Ports{}}
	cancelCmd := publicapplication.CancelDraftCommand{
		Context:   publicapplication.Context{Principal: publicapplication.Principal{ActorID: "actor"}},
		CompanyID: companyID, Reason: "reason",
	}
	if _, err := cancelApp.CancelDraft(ctx, cancelCmd); !errors.Is(err, ErrCapabilityNotImplemented) {
		t.Fatalf("missing draft canceller = %v", err)
	}
	cancelCmd.Context.Principal.ActorID = ""
	if _, err := cancelApp.CancelDraft(ctx, cancelCmd); !errors.Is(err, publicapplication.ErrValidation) {
		t.Fatalf("missing actor = %v", err)
	}
	cancelCmd.Context.Principal.ActorID = "actor"
	cancelCmd.Reason = "  "
	if _, err := cancelApp.CancelDraft(ctx, cancelCmd); !errors.Is(err, publicapplication.ErrValidation) {
		t.Fatalf("missing reason = %v", err)
	}

	draftApp := &applicationService{ports: storage.Ports{Drafts: p2DraftStore{err: errors.New("draft failed")}, MasterData: &p2MasterDataReader{}}}
	createCmd := publicapplication.CreateDraftCommand{CompanyID: companyID}
	if _, err := draftApp.createDraftWithID(ctx, createCmd, types.JournalEntryID{}); err == nil {
		t.Fatal("expected draft store error")
	}
	createCmd.ExchangeRate = "not-a-number"
	if _, err := draftApp.createDraftWithID(ctx, createCmd, types.JournalEntryID{}); !errors.Is(err, publicapplication.ErrValidation) {
		t.Fatalf("invalid exchange rate = %v", err)
	}
	getApp := &applicationService{ports: storage.Ports{Entries: p2EntryReader{err: errors.New("entry missing")}}}
	validateCmd := publicapplication.ValidateEntryCommand{
		Context:   publicapplication.Context{Principal: publicapplication.Principal{ActorID: "actor"}},
		CompanyID: companyID, EntryID: types.NewJournalEntryID(),
	}
	if err := getApp.Validate(ctx, validateCmd); err == nil {
		t.Fatal("expected validate missing entry error")
	}
}

func TestP2OrbitalsServices(t *testing.T) {
	ctx := context.Background()
	provider := staticRateProvider{rates: map[string]decimal.Decimal{
		"USD:EUR:closing": decimal.RequireFromString("0.9"),
	}}
	if rate, err := provider.Rate(consolidationRateRequest("usd", "USD", publicconsolidation.RateClosing)); err != nil || !rate.Equal(decimal.NewFromInt(1)) {
		t.Fatalf("same currency rate = %s %v", rate, err)
	}
	if rate, err := provider.Rate(consolidationRateRequest("USD", "eur", publicconsolidation.RateClosing)); err != nil || !rate.Equal(decimal.RequireFromString("0.9")) {
		t.Fatalf("configured rate = %s %v", rate, err)
	}
	if _, err := provider.Rate(consolidationRateRequest("EUR", "GBP", publicconsolidation.RateClosing)); err == nil {
		t.Fatal("expected missing rate error")
	}

	consolidationSvc := &defaultConsolidationService{}
	if _, err := consolidationSvc.Consolidate(nil, publicconsolidation.ConsolidationRequest{}); err == nil {
		t.Fatal("expected nil context error")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := consolidationSvc.Consolidate(canceled, publicconsolidation.ConsolidationRequest{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled consolidation = %v", err)
	}
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	parentID := uuid.NewString()
	request := publicconsolidation.ConsolidationRequest{
		GroupID: "group", ParentEntityID: parentID, GroupCurrency: "USD",
		StartDate: start, EndDate: start.AddDate(0, 1, 0),
		Entities: []publicconsolidation.Entity{{ID: parentID, FunctionalCurrency: "USD"}},
		GroupAccounts: []publicconsolidation.GroupAccount{
			{ID: "cash", Name: "Cash", Type: publicconsolidation.AccountAsset},
			{ID: "equity", Name: "Equity", Type: publicconsolidation.AccountEquity},
		},
		AccountMappings: []publicconsolidation.AccountMapping{
			{EntityID: parentID, SourceAccountID: "cash", GroupAccountID: "cash", RateClass: publicconsolidation.RateClosing},
			{EntityID: parentID, SourceAccountID: "equity", GroupAccountID: "equity", RateClass: publicconsolidation.RateClosing},
		},
		TrialBalances: []publicconsolidation.TrialBalance{{
			EntityID: parentID, Currency: "USD", StartDate: start, EndDate: start.AddDate(0, 1, 0),
			Lines: []publicconsolidation.TrialBalanceLine{
				{AccountID: "cash", Balance: decimal.NewFromInt(100)},
				{AccountID: "equity", Balance: decimal.NewFromInt(-100)},
			},
		}},
		Rates: []publicconsolidation.ExchangeRate{},
	}
	if result, err := consolidationSvc.Consolidate(ctx, request); err != nil {
		t.Fatalf("consolidation: %v", err)
	} else if result.GroupCurrency != "USD" || result.Balances == nil {
		t.Fatalf("consolidation result = %+v", result)
	}

	reconciliationSvc := &defaultReconciliationService{}
	if _, err := reconciliationSvc.ReconcileRecords(nil, publicreconciliation.ReconciliationRequest{}); err == nil {
		t.Fatal("expected nil reconciliation context error")
	}
	if _, err := reconciliationSvc.ReconcileRecords(canceled, publicreconciliation.ReconciliationRequest{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled reconciliation = %v", err)
	}
	if _, err := reconciliationSvc.ReconcileRecords(ctx, publicreconciliation.ReconciliationRequest{}); err == nil {
		t.Fatal("expected source name validation error")
	}
	if _, err := reconciliationSvc.ReconcileRecords(ctx, publicreconciliation.ReconciliationRequest{
		LeftSourceName: "left", RightSourceName: "right", Currency: "USD", Tolerance: decimal.NewFromInt(-1),
	}); err == nil {
		t.Fatal("expected negative tolerance error")
	}
	result, err := reconciliationSvc.ReconcileRecords(ctx, publicreconciliation.ReconciliationRequest{
		LeftSourceName: "left", RightSourceName: "right", Currency: "USD", Tolerance: decimal.NewFromInt(1),
		LeftRecords: []publicreconciliation.Record{
			{Key: "match", Amount: decimal.NewFromInt(10)},
			{Key: "mismatch", Amount: decimal.NewFromInt(20)},
			{Key: "left-only", Amount: decimal.NewFromInt(30)},
		},
		RightRecords: []publicreconciliation.Record{
			{Key: "match", Amount: decimal.NewFromInt(10)},
			{Key: "mismatch", Amount: decimal.NewFromInt(15)},
			{Key: "right-only", Amount: decimal.NewFromInt(40)},
		},
	})
	if err != nil || result.Balanced || len(result.Matched) != 1 || len(result.Exceptions) != 3 {
		t.Fatalf("reconciliation result = %+v %v", result, err)
	}
}

func TestP2ClosingRepositoryHelpers(t *testing.T) {
	ctx := context.Background()
	companyID := p2CompanyID()
	store := &p2ClosingStore{
		accounts: []publicmasterdata.Account{{ID: types.NewAccountID()}},
		years:    []publicmasterdata.FiscalYear{{ID: types.NewFiscalYearID()}},
		periods:  []publicmasterdata.FiscalPeriod{{ID: types.NewFiscalPeriodID()}},
		entries:  []journal.JournalEntry{{ID: types.NewJournalEntryID(), CompanyID: companyID}},
	}
	entryID := types.NewJournalEntryID()
	repo := &sdkClosingRepository{
		masterData: &p2MasterDataReader{}, entries: p2EntryReader{entry: p2PostedEntry(companyID, entryID)},
		store: store, companyID: companyID,
	}
	if accounts, err := repo.ListAccounts(ctx); err != nil || len(accounts) != 1 {
		t.Fatalf("ListAccounts = %v %v", accounts, err)
	}
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0)
	entries, err := repo.ListPostedEntries(ctx, start, end)
	if err != nil || len(entries) != 1 {
		t.Fatalf("ListPostedEntries = %v %v", entries, err)
	}
	if _, err := repo.ListAccountsForCompanyContext(ctx, uuid.UUID(companyID)); err != nil {
		t.Fatalf("ListAccountsForCompanyContext: %v", err)
	}
	if _, err := repo.ListAccountsForCompanyAsOfContext(ctx, uuid.UUID(companyID), end); err != nil {
		t.Fatalf("ListAccountsForCompanyAsOfContext: %v", err)
	}
	if _, err := repo.ListEntriesForCompanyContext(ctx, uuid.UUID(companyID), start, end); err != nil {
		t.Fatalf("ListEntriesForCompanyContext: %v", err)
	}
	if assets, err := repo.ListFixedAssets(ctx); err != nil || assets != nil {
		t.Fatalf("ListFixedAssets = %v %v", assets, err)
	}
	if assets, err := repo.ListFixedAssetsForCompanyContext(ctx, uuid.UUID(companyID)); err != nil || assets != nil {
		t.Fatalf("ListFixedAssetsForCompanyContext = %v %v", assets, err)
	}
	if lines, err := repo.ListOpenLines(ctx, "AR"); err != nil || lines != nil {
		t.Fatalf("ListOpenLines = %v %v", lines, err)
	}
	if lines, err := repo.ListOpenLinesForCompanyContext(ctx, uuid.UUID(companyID), "AR"); err != nil || lines != nil {
		t.Fatalf("ListOpenLinesForCompanyContext = %v %v", lines, err)
	}
	if rate, err := repo.GetRateForCompany(ctx, uuid.UUID(companyID), "USD", "EUR", end); err != nil || !rate.Equal(decimal.NewFromInt(1)) {
		t.Fatalf("GetRateForCompany = %s %v", rate, err)
	}
	if rate, err := repo.GetRateOfTypeForCompany(ctx, uuid.UUID(companyID), kernelvaluation.SpotRate, "USD", "EUR", end); err != nil || !rate.Equal(decimal.NewFromInt(1)) {
		t.Fatalf("GetRateOfTypeForCompany = %s %v", rate, err)
	}
	if _, err := repo.GetFiscalYearForCompanyContext(ctx, uuid.UUID(companyID), uuid.New()); err != nil {
		t.Fatalf("GetFiscalYearForCompanyContext: %v", err)
	}
	if err := repo.UpdatePeriodStateForCompanyContext(ctx, uuid.UUID(companyID), uuid.New(), publicmasterdata.PeriodHardClosed); err != nil {
		t.Fatalf("UpdatePeriodStateForCompanyContext: %v", err)
	}
	if err := repo.UpdateFiscalYearStateForCompanyContext(ctx, uuid.UUID(companyID), uuid.New(), publicmasterdata.FiscalYearHardClosed); err != nil {
		t.Fatalf("UpdateFiscalYearStateForCompanyContext: %v", err)
	}
	if got, err := repo.GetEntryByIDForCompanyContext(ctx, uuid.UUID(companyID), uuid.UUID(entryID)); err != nil || got.CompanyID != uuid.UUID(companyID) {
		t.Fatalf("GetEntryByIDForCompanyContext = %+v %v", got, err)
	}
	if periods, err := repo.ListFiscalPeriodsForCompanyContext(ctx, uuid.UUID(companyID), uuid.New()); err != nil || len(periods) != 1 {
		t.Fatalf("ListFiscalPeriodsForCompanyContext = %v %v", periods, err)
	}
	if first, err := repo.IsFirstFiscalYearForCompanyContext(ctx, uuid.UUID(companyID), uuid.UUID(store.years[0].ID)); err != nil || !first {
		t.Fatalf("IsFirstFiscalYearForCompanyContext = %v %v", first, err)
	}

	partnerID := uuid.New()
	costCenterID := uuid.New()
	projectID := uuid.New()
	segmentID := uuid.New()
	businessUnitID := uuid.New()
	amount, _ := types.NewMoneyFromString("100", "USD")
	ke := journal.JournalEntry{
		ID: types.NewJournalEntryID(), CompanyID: companyID, JournalID: types.NewJournalID(),
		SequenceNumber: 3, FiscalPeriodID: types.NewFiscalPeriodID(), EffectiveDate: end, PostingDate: end,
		Description: "desc", SourceModule: "test", ReferenceID: "ref", TransactionCurrency: "USD",
		Classification: journal.EntryClassAdjusting, TargetPeriodNumber: 13,
		AdjustingEntryType: journal.AdjustingEntryAccrual, AdjustmentReason: "reason",
		Status: journal.EntryPosted,
		Lines: []journal.EntryLine{{
			ID: types.NewJournalEntryLineID(), AccountID: types.NewAccountID(),
			PartnerID: (*types.PartnerID)(&partnerID), CostCenterID: (*types.CostCenterID)(&costCenterID),
			ProjectID: (*types.ProjectID)(&projectID), SegmentID: (*types.SegmentID)(&segmentID),
			BusinessUnitID:    (*types.BusinessUnitID)(&businessUnitID),
			TransactionAmount: amount, TransactionCurrency: "USD", ExchangeRate: decimal.NewFromInt(1),
			FunctionalDebit: amount, FunctionalCredit: amount,
		}},
	}
	compat := kernelEntryToCompat(ke)
	if compat.CompanyID != uuid.UUID(companyID) || len(compat.Lines) != 1 {
		t.Fatalf("kernelEntryToCompat = %+v", compat)
	}
	converted := compatEntryToKernel(compat, companyID, ke.JournalID)
	if converted.CompanyID != companyID || converted.Status != journal.EntryDraft || len(converted.Lines) != 1 {
		t.Fatalf("compatEntryToKernel = %+v", converted)
	}
	compat.Lines[0].ID = uuid.Nil
	compat.Lines[0].ExchangeRate = decimal.Zero
	compat.Lines[0].TransactionCurrency = ""
	compat.Lines[0].FunctionalDebit = types.Money{}
	compat.Lines[0].FunctionalCredit = types.Money{}
	if got := compatEntryToKernel(compat, companyID, ke.JournalID); got.Lines[0].ID == (types.JournalEntryLineID{}) ||
		got.Lines[0].TransactionCurrency != "USD" || got.Lines[0].FunctionalDebit.Currency() != "USD" {
		t.Fatalf("compat defaults were not applied: %+v", got.Lines[0])
	}

	noCapApp := &applicationService{ports: storage.Ports{MasterData: &p2MasterDataReader{}}}
	if _, err := noCapApp.ExecuteYearEndClosing(ctx, publicapplication.ExecuteYearEndClosingCommand{}); !errors.Is(err, publicapplication.ErrValidation) {
		t.Fatalf("year-end validation = %v", err)
	}
	if _, err := noCapApp.ReopenYearEndClosing(ctx, publicapplication.ReopenYearEndClosingCommand{}); !errors.Is(err, publicapplication.ErrValidation) {
		t.Fatalf("reopen validation = %v", err)
	}
}

func TestP2EventIngestionHelpers(t *testing.T) {
	ctx := context.Background()
	companyID := p2CompanyID()
	companyUUID := uuid.UUID(companyID)
	requestContext := publicapplication.Context{Principal: publicapplication.Principal{ActorID: "actor"}}
	postedID := types.NewJournalEntryID()
	posted := p2PostedEntry(companyID, postedID)

	svc := &defaultEventService{useCases: &p2UseCases{getFn: func(context.Context, publicapplication.GetEntryQuery) (journal.JournalEntry, error) {
		return posted, nil
	}}}

	if err := validatePostingEntry(journal.JournalEntry{}); !errors.Is(err, publicapplication.ErrValidation) {
		t.Fatalf("empty entry = %v", err)
	}
	entry := journal.JournalEntry{CompanyID: companyID}
	if err := validatePostingEntry(entry); !errors.Is(err, publicapplication.ErrValidation) {
		t.Fatalf("missing journal = %v", err)
	}
	entry.JournalID = types.NewJournalID()
	if err := validatePostingEntry(entry); !errors.Is(err, publicapplication.ErrValidation) {
		t.Fatalf("missing lines = %v", err)
	}
	entry.Lines = []journal.EntryLine{{}, {}}
	if err := validatePostingEntry(entry); !errors.Is(err, publicapplication.ErrValidation) {
		t.Fatalf("missing dates = %v", err)
	}
	entry.EffectiveDate = time.Now().UTC()
	if err := validatePostingEntry(entry); err != nil {
		t.Fatalf("valid entry = %v", err)
	}

	if got := classifyEventFailure(nil); got != "" {
		t.Fatalf("nil failure class = %q", got)
	}
	for name, err := range map[string]error{
		"serialization": storage.ErrSerializationFailure,
		"aborted":       storage.ErrTransactionAborted,
		"dependency":    publicapplication.NewError(publicapplication.ErrDependencyUnavailable, "unavailable", nil),
		"deadline":      context.DeadlineExceeded,
		"canceled":      context.Canceled,
		"terminal":      errors.New("terminal"),
	} {
		want := publicevents.FailureClassTerminal
		if name != "terminal" {
			want = publicevents.FailureClassTransient
		}
		if got := classifyEventFailure(err); got != want {
			t.Fatalf("%s class = %q, want %q", name, got, want)
		}
	}
	if err := failureReasonError(publicevents.Ingestion{}); err != nil {
		t.Fatalf("empty failure reason = %v", err)
	}
	if err := failureReasonError(publicevents.Ingestion{FailureReason: "reason"}); err == nil || err.Error() != "reason" {
		t.Fatalf("failure reason = %v", err)
	}
	if wrapped := wrapEventError(publicapplication.ErrConflict, "conflict", ErrIdempotencyKeyConflict); !errors.Is(wrapped, ErrIdempotencyKeyConflict) {
		t.Fatalf("wrapped error = %v", wrapped)
	}
	if timeout, interval := (&defaultEventService{}).waitPolicy(); timeout != defaultEventIdempotencyWaitTimeout || interval != defaultEventIdempotencyWaitInterval {
		t.Fatalf("default wait policy = %s %s", timeout, interval)
	}
	if timeout, interval := (&defaultEventService{waitTimeout: 5 * time.Millisecond, waitInterval: time.Millisecond}).waitPolicy(); timeout != 5*time.Millisecond || interval != time.Millisecond {
		t.Fatalf("configured wait policy = %s %s", timeout, interval)
	}

	noMaster := &defaultEventService{}
	if _, err := noMaster.resolveRuleAccount(ctx, companyID, "1000"); err == nil {
		t.Fatal("expected account resolution without master data error")
	}
	if _, err := noMaster.resolveRuleAccount(ctx, companyID, "  "); err == nil {
		t.Fatal("expected empty account code error")
	}
	driver := memory.New()
	masterData := driver.MasterDataStore()
	if err := masterData.CreateCompany(ctx, publicmasterdata.Company{
		ID: companyID, Code: "EVENT-TEST", LegalName: "Event Test",
		FunctionalCurrency: "USD", PresentationCurrency: "USD", Active: true,
	}); err != nil {
		t.Fatalf("create company: %v", err)
	}
	accountID := types.NewAccountID()
	if err := masterData.CreateAccount(ctx, publicmasterdata.Account{
		ID: accountID, CompanyID: companyID, Code: "1000", Name: "Cash", Currency: "USD",
	}); err != nil {
		t.Fatalf("create account: %v", err)
	}
	withMaster := &defaultEventService{publicMasterData: masterData}
	if got, err := withMaster.resolveRuleAccount(ctx, companyID, "1000"); err != nil || got != accountID {
		t.Fatalf("resolve account = %s %v", got, err)
	}
	if got, err := withMaster.resolveRuleAccount(ctx, companyID, uuid.NewString()); err != nil || got == (types.AccountID{}) {
		t.Fatalf("resolve uuid account = %s %v", got, err)
	}
	if _, err := withMaster.resolveRuleAccount(ctx, companyID, "missing"); err == nil {
		t.Fatal("expected missing account error")
	}

	if _, err := resolveDefaultJournalIDForStore(ctx, nil, companyID, "GENERAL"); !errors.Is(err, ErrCapabilityNotImplemented) {
		t.Fatalf("journal without master data = %v", err)
	}
	if _, err := resolveDefaultJournalIDForStore(ctx, masterData, types.CompanyID{}, "GENERAL"); err == nil {
		t.Fatal("expected missing company error")
	}
	if _, err := resolveDefaultJournalIDForStore(ctx, masterData, companyID, "MISSING"); err == nil {
		t.Fatal("expected missing non-default journal error")
	}
	journalID, err := resolveDefaultJournalIDForStore(ctx, masterData, companyID, "")
	if err != nil || journalID == (types.JournalID{}) {
		t.Fatalf("create default journal = %s %v", journalID, err)
	}
	if _, err := resolveDefaultJournalIDForStore(ctx, masterData, companyID, "GENERAL"); err != nil {
		t.Fatalf("reuse default journal = %v", err)
	}

	if got, ok := svc.recoverPostedEntry(ctx, companyID, postedID, requestContext); !ok || got.ID != postedID {
		t.Fatalf("recover posted entry = %+v %v", got, ok)
	}
	if _, ok := svc.recoverPostedEntry(ctx, companyID, types.JournalEntryID{}, requestContext); ok {
		t.Fatal("empty entry id should not be recoverable")
	}
	if got, err := svc.completedIngestionResult(ctx, publicevents.Ingestion{CompanyID: companyUUID}, requestContext); err == nil || got.ID != (types.JournalEntryID{}) {
		_ = got
		if err == nil {
			t.Fatal("expected completed record without entry id error")
		}
	}
	if _, err := svc.completedIngestionResult(ctx, publicevents.Ingestion{
		CompanyID: companyUUID, EntryID: uuid.UUID(postedID),
	}, requestContext); err != nil {
		t.Fatalf("completed ingestion result: %v", err)
	}
	svc.useCases = &p2UseCases{getFn: func(context.Context, publicapplication.GetEntryQuery) (journal.JournalEntry, error) {
		return journal.JournalEntry{ID: postedID, Status: journal.EntryDraft}, nil
	}}
	if _, err := svc.completedIngestionResult(ctx, publicevents.Ingestion{
		CompanyID: companyUUID, EntryID: uuid.UUID(postedID),
	}, requestContext); !errors.Is(err, ErrIdempotencyInProgress) {
		t.Fatalf("completed but not posted = %v", err)
	}
	svc.useCases = &p2UseCases{getFn: func(context.Context, publicapplication.GetEntryQuery) (journal.JournalEntry, error) {
		return journal.JournalEntry{}, errors.New("get failed")
	}}
	if _, err := svc.completedIngestionResult(ctx, publicevents.Ingestion{
		CompanyID: companyUUID, EntryID: uuid.UUID(postedID),
	}, requestContext); err == nil {
		t.Fatal("expected completed result load error")
	}

	store := &p2IngestionStore{}
	svc.idempotencyStore = store
	svc.waitTimeout = time.Millisecond
	svc.waitInterval = time.Millisecond
	scopedCalls := 0
	store.scopedGetFn = func(context.Context, uuid.UUID, string, string) (publicevents.Ingestion, bool, error) {
		scopedCalls++
		return publicevents.Ingestion{Status: publicevents.StatusCompleted}, true, nil
	}
	if rec, terminal, err := svc.waitForIngestion(ctx, store, companyUUID, "event", "key"); err != nil || !terminal || rec.Status != publicevents.StatusCompleted {
		t.Fatalf("wait for ingestion = %+v %v %v", rec, terminal, err)
	}
	if scopedCalls == 0 {
		t.Fatal("scoped ingestion lookup was not used")
	}
	store.scopedGetFn = func(context.Context, uuid.UUID, string, string) (publicevents.Ingestion, bool, error) {
		return publicevents.Ingestion{}, false, nil
	}
	if rec, terminal, err := svc.waitForIngestion(ctx, store, companyUUID, "event", "key"); err != nil || terminal || rec.Status != "" {
		t.Fatalf("missing ingestion wait = %+v %v %v", rec, terminal, err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, _, err := svc.waitForIngestion(canceled, store, companyUUID, "event", "key"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled wait = %v", err)
	}
	store.scopedGetFn = nil
	store.getFn = func(context.Context, uuid.UUID, string) (publicevents.Ingestion, bool, error) {
		return publicevents.Ingestion{Status: publicevents.StatusTerminalFailure}, true, nil
	}
	if rec, terminal, err := svc.waitForIngestion(ctx, store, companyUUID, "event", "key"); err != nil || !terminal || rec.Status != publicevents.StatusTerminalFailure {
		t.Fatalf("unscoped wait = %+v %v %v", rec, terminal, err)
	}

	scopedComplete := 0
	store.scopedComplete = func(context.Context, uuid.UUID, string, string, string, journal.JournalEntry) error {
		scopedComplete++
		return nil
	}
	if err := svc.completeIngestion(ctx, store, companyUUID, "source", "key", "hash", posted); err != nil || scopedComplete != 1 {
		t.Fatalf("scoped complete = %v %d", err, scopedComplete)
	}
	plainComplete := 0
	store.scopedComplete = nil
	store.completeFn = func(context.Context, uuid.UUID, string, journal.JournalEntry) error {
		plainComplete++
		return nil
	}
	if err := svc.completeIngestion(ctx, store, companyUUID, "event", "key", "hash", posted); err != nil || plainComplete != 1 {
		t.Fatalf("plain complete = %v %d", err, plainComplete)
	}

	classified := 0
	store.classifiedFail = func(context.Context, uuid.UUID, string, string, string, string) error {
		classified++
		return nil
	}
	if err := svc.failIngestion(ctx, store, companyUUID, "source", "key", publicevents.FailureClassTransient, "reason"); err != nil || classified != 1 {
		t.Fatalf("classified fail = %v %d", err, classified)
	}
	store.classifiedFail = nil
	scopedFail := 0
	store.scopedFailFn = func(context.Context, uuid.UUID, string, string, string, string) error {
		scopedFail++
		return nil
	}
	if err := svc.failIngestion(ctx, store, companyUUID, "source", "key", publicevents.FailureClassTerminal, "reason"); err != nil || scopedFail != 1 {
		t.Fatalf("scoped fail = %v %d", err, scopedFail)
	}
	store.scopedFailFn = nil
	plainFail := 0
	store.failFn = func(context.Context, uuid.UUID, string, string) error {
		plainFail++
		return nil
	}
	if err := svc.failIngestion(ctx, store, companyUUID, "event", "key", "", "reason"); err != nil || plainFail != 1 {
		t.Fatalf("plain fail = %v %d", err, plainFail)
	}

	svc.useCases = &p2UseCases{getFn: func(context.Context, publicapplication.GetEntryQuery) (journal.JournalEntry, error) {
		return posted, nil
	}}
	existing := publicevents.Ingestion{Status: publicevents.StatusCompleted, CompanyID: companyUUID, EntryID: uuid.UUID(postedID)}
	if got, err := svc.handleExistingIngestion(ctx, store, companyUUID, "event", "key", "hash", existing, entry, requestContext); err != nil || got.ID != postedID {
		t.Fatalf("completed existing ingestion = %+v %v", got, err)
	}
	if _, err := svc.handleExistingIngestion(ctx, store, companyUUID, "event", "key", "hash", publicevents.Ingestion{
		Status: publicevents.StatusCompleted, PayloadHash: "different",
	}, entry, requestContext); !errors.Is(err, ErrIdempotencyKeyConflict) {
		t.Fatalf("payload conflict = %v", err)
	}
	if _, err := svc.handleExistingIngestion(ctx, store, companyUUID, "event", "key", "hash", publicevents.Ingestion{
		Status: publicevents.StatusTerminalFailure, FailureReason: "terminal",
	}, entry, requestContext); !errors.Is(err, ErrIdempotencyTerminalFailure) {
		t.Fatalf("terminal existing = %v", err)
	}
	if _, err := svc.handleExistingIngestion(ctx, store, companyUUID, "event", "key", "hash", publicevents.Ingestion{
		Status: publicevents.StatusRetryableFailure, FailureReason: "retry",
	}, entry, requestContext); !errors.Is(err, ErrIdempotencyInProgress) {
		t.Fatalf("retryable existing = %v", err)
	}
	if _, err := svc.handleExistingIngestion(ctx, store, companyUUID, "event", "key", "hash", publicevents.Ingestion{
		Status: "invalid",
	}, entry, requestContext); !errors.Is(err, ErrIdempotencyStateConflict) {
		t.Fatalf("invalid existing status = %v", err)
	}

	svc.useCases = &p2UseCases{getFn: func(context.Context, publicapplication.GetEntryQuery) (journal.JournalEntry, error) {
		return posted, nil
	}}
	if got, err := svc.recoverStartedIngestion(ctx, store, companyUUID, "event", "key", "hash", publicevents.Ingestion{
		Status: publicevents.StatusStarted, EntryID: uuid.UUID(postedID),
	}, entry, requestContext); err != nil || got.ID != postedID {
		t.Fatalf("recover started with posted entry = %+v %v", got, err)
	}
	if _, err := svc.recoverStartedIngestion(ctx, store, companyUUID, "event", "key", "hash", publicevents.Ingestion{
		Status: publicevents.StatusStarted,
	}, entry, requestContext); !errors.Is(err, ErrIdempotencyInProgress) {
		t.Fatalf("recover started without entry = %v", err)
	}

	postErr := errors.New("post failed")
	svc.useCases = &p2UseCases{
		postFn: func(context.Context, publicapplication.PostEntryCommand) (publicapplication.EntryResult, error) {
			return publicapplication.EntryResult{}, postErr
		},
		getFn: func(context.Context, publicapplication.GetEntryQuery) (journal.JournalEntry, error) {
			return journal.JournalEntry{}, publicapplication.NewError(publicapplication.ErrNotFound, "not found", nil)
		},
	}
	if _, err := svc.executeClaimedIngestion(ctx, store, companyUUID, "event", "key", "hash", publicevents.Ingestion{
		EntryID: uuid.UUID(postedID),
	}, entry, requestContext); !errors.Is(err, postErr) {
		t.Fatalf("claimed ingestion failure = %v", err)
	}

	svc.useCases = &p2UseCases{
		postFn: func(context.Context, publicapplication.PostEntryCommand) (publicapplication.EntryResult, error) {
			return publicapplication.EntryResult{Entry: posted}, nil
		},
	}
	if got, err := svc.executeClaimedIngestion(ctx, store, companyUUID, "event", "key", "hash", publicevents.Ingestion{}, entry, requestContext); err != nil || got.ID != postedID {
		t.Fatalf("claimed ingestion success = %+v %v", got, err)
	}
}

func TestP2EventConstructionAndProcessing(t *testing.T) {
	ctx := context.Background()
	driver := memory.New()
	masterData := driver.MasterDataStore()
	companyID := p2CompanyID()
	if err := masterData.CreateCompany(ctx, publicmasterdata.Company{
		ID: companyID, Code: "EVENT-CONSTRUCT", LegalName: "Event Construct",
		FunctionalCurrency: "USD", PresentationCurrency: "USD", Active: true,
	}); err != nil {
		t.Fatalf("create company: %v", err)
	}
	journalID := types.NewJournalID()
	if err := masterData.CreateJournal(ctx, publicmasterdata.Journal{
		ID: journalID, CompanyID: companyID, Code: "GENERAL", Name: "General",
	}); err != nil {
		t.Fatalf("create journal: %v", err)
	}
	debitID := types.NewAccountID()
	if err := masterData.CreateAccount(ctx, publicmasterdata.Account{
		ID: debitID, CompanyID: companyID, Code: "1000", Name: "Cash",
		Type: publicmasterdata.AccountTypeAsset, NormalBalance: publicmasterdata.NormalBalanceDebit,
		Currency: "USD", IsActive: true,
	}); err != nil {
		t.Fatalf("create debit account: %v", err)
	}
	creditID := types.NewAccountID()
	if err := masterData.CreateAccount(ctx, publicmasterdata.Account{
		ID: creditID, CompanyID: companyID, Code: "4000", Name: "Revenue",
		Type: publicmasterdata.AccountTypeIncome, NormalBalance: publicmasterdata.NormalBalanceCredit,
		Currency: "USD", IsActive: true,
	}); err != nil {
		t.Fatalf("create credit account: %v", err)
	}

	amount, err := types.NewMoneyFromString("100", "USD")
	if err != nil {
		t.Fatal(err)
	}
	effectiveDate := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	rule := events.PostingRule{
		EventType: "sale", JournalCode: "GENERAL", DebitAccountCode: "1000", CreditAccountCode: "4000",
		DescriptionTemplate: "Invoice {{.EventID}}",
	}
	event := events.AccountingEvent{
		EventID: "evt-1", EventType: "sale", CompanyID: companyID, LedgerID: "GENERAL",
		Amount: amount, EffectiveDate: effectiveDate,
	}

	svc := &defaultEventService{
		publicMasterData: masterData, useCases: &p2UseCases{}, router: events.NewRouter(),
		waitTimeout: time.Millisecond, waitInterval: time.Millisecond,
	}
	svc.router.RegisterRule(rule)
	entry, err := svc.constructEventEntry(ctx, rule, event)
	if err != nil || len(entry.Lines) != 2 || entry.Lines[0].AccountID != debitID || entry.Lines[1].AccountID != creditID {
		t.Fatalf("constructEventEntry = %+v %v", entry, err)
	}
	if _, err := svc.ProcessEvent(ctx, event); err != nil {
		t.Fatalf("ProcessEvent = %v", err)
	}

	if _, err := (&defaultEventService{}).ProcessEvent(ctx, event); err == nil {
		t.Fatal("expected unavailable application error")
	}
	invalid := event
	invalid.EventType = "  "
	if _, err := svc.ProcessEvent(ctx, invalid); err == nil {
		t.Fatal("expected event type validation error")
	}
	invalid = event
	invalid.CompanyID = types.CompanyID{}
	if _, err := svc.ProcessEvent(ctx, invalid); err == nil {
		t.Fatal("expected company validation error")
	}
	invalid = event
	invalid.EventID = ""
	if _, err := svc.ProcessEvent(ctx, invalid); err == nil {
		t.Fatal("expected event id validation error")
	}
	invalid = event
	invalid.Amount, _ = types.NewMoneyFromString("-1", "USD")
	if _, err := svc.ProcessEvent(ctx, invalid); err == nil {
		t.Fatal("expected positive amount error")
	}
	invalid = event
	invalid.Dimensions = map[string]string{"region": "north"}
	if _, err := svc.ProcessEvent(ctx, invalid); err == nil {
		t.Fatal("expected dimensions unsupported error")
	}
	invalid = event
	invalid.Payload = map[string]any{"invoice": "INV-1"}
	invalid.JournalDraft = &journal.JournalEntry{}
	if _, err := svc.ProcessEvent(ctx, invalid); err == nil {
		t.Fatal("expected payload unsupported with draft error")
	}
	invalid = event
	invalid.EffectiveDate = time.Time{}
	if _, err := svc.ProcessEvent(ctx, invalid); err == nil {
		t.Fatal("expected effective date error")
	}
	invalid = event
	invalid.EffectiveDate = time.Time{}
	validDraft := &journal.JournalEntry{
		ID: types.NewJournalEntryID(), CompanyID: companyID, JournalID: journalID,
		EffectiveDate: effectiveDate, PostingDate: effectiveDate,
		Lines: []journal.EntryLine{{AccountID: debitID}, {AccountID: creditID}},
	}
	invalid.JournalDraft = validDraft
	if _, err := svc.ProcessEvent(ctx, invalid); err != nil {
		t.Fatalf("draft effective date should be accepted: %v", err)
	}
	mismatch := journal.JournalEntry{CompanyID: p2CompanyID(), EffectiveDate: effectiveDate, Status: journal.EntryDraft}
	invalid = event
	invalid.JournalDraft = &mismatch
	if _, err := svc.ProcessEvent(ctx, invalid); err == nil {
		t.Fatal("expected draft company mismatch error")
	}
	badStatus := journal.JournalEntry{CompanyID: companyID, EffectiveDate: effectiveDate, Status: journal.EntryPosted}
	invalid = event
	invalid.JournalDraft = &badStatus
	if _, err := svc.ProcessEvent(ctx, invalid); err == nil {
		t.Fatal("expected draft status error")
	}
	noRule := &defaultEventService{publicMasterData: masterData, useCases: &p2UseCases{}}
	if _, err := noRule.ProcessEvent(ctx, event); err == nil {
		t.Fatal("expected missing rule error")
	}

	if _, err := (&defaultEventService{}).constructEventEntry(ctx, rule, event); !errors.Is(err, ErrCapabilityNotImplemented) {
		t.Fatalf("construct without master data = %v", err)
	}
	eurAmount, _ := types.NewMoneyFromString("100", "EUR")
	eurEvent := event
	eurEvent.Amount = eurAmount
	if _, err := svc.constructEventEntry(ctx, rule, eurEvent); !errors.Is(err, ErrExchangeRateUnavailable) {
		t.Fatalf("currency mismatch = %v", err)
	}
	payloadEvent := event
	payloadEvent.Payload = map[string]any{"invoice": "INV-1"}
	if _, err := svc.constructEventEntry(ctx, rule, payloadEvent); err == nil {
		t.Fatal("expected payload unsupported error")
	}
	invalidTemplateRule := rule
	invalidTemplateRule.DescriptionTemplate = "{{"
	if _, err := svc.constructEventEntry(ctx, invalidTemplateRule, event); err == nil {
		t.Fatal("expected description template parse error")
	}

	if got, err := renderEventDescription("", event); err != nil || got != "Event: sale (evt-1)" {
		t.Fatalf("default description = %q %v", got, err)
	}
	if got, err := renderEventDescription("Invoice {{.EventID}}", event); err != nil || got != "Invoice evt-1" {
		t.Fatalf("template description = %q %v", got, err)
	}
	if got, err := renderEventDescription("Flat", event); err != nil || got != "Flat: sale (evt-1)" {
		t.Fatalf("literal description = %q %v", got, err)
	}
	if _, err := renderEventDescription("{{.Missing}}", event); err == nil {
		t.Fatal("expected missing template key error")
	}
	if _, err := renderEventDescription("{{if false}}x{{end}}", event); err == nil {
		t.Fatal("expected empty description error")
	}
	if _, err := renderEventDescription("{{", event); err == nil {
		t.Fatal("expected template parse error")
	}
}

type p2PolicyStore struct {
	versions map[storage.PolicyKind][]storage.PolicyVersion
	err      error
}

func (s p2PolicyStore) AppendVersion(context.Context, storage.PolicyVersion) error {
	return s.err
}

func (s p2PolicyStore) ListVersions(_ context.Context, _ types.CompanyID, kind storage.PolicyKind) ([]storage.PolicyVersion, error) {
	return s.versions[kind], s.err
}

func TestP2PolicyConversionHelpers(t *testing.T) {
	companyID := p2CompanyID()
	effectiveFrom := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	effectiveTo := effectiveFrom.AddDate(0, 1, 0)
	portStore := p2PolicyStore{}
	if got := policyStoreFromPorts(storage.Ports{}); got != nil {
		t.Fatalf("empty private extensions = %v", got)
	}
	if got := policyStoreFromPorts(storage.Ports{Private: storage.PrivateExtensions{
		storage.PolicyStoreExtensionKey: "not a policy store",
	}}); got != nil {
		t.Fatalf("invalid policy store = %v", got)
	}
	if got := policyStoreFromPorts(storage.Ports{Private: storage.PrivateExtensions{
		storage.PolicyStoreExtensionKey: &portStore,
	}}); got != &portStore {
		t.Fatalf("valid policy store = %v", got)
	}
	if clonePolicyEnd(nil) != nil {
		t.Fatal("nil end should stay nil")
	}
	if cloned := clonePolicyEnd(&effectiveTo); cloned == nil || *cloned != effectiveTo || cloned == &effectiveTo {
		t.Fatalf("cloned end = %v", cloned)
	}

	postingPayload, err := json.Marshal(storage.PostingPolicyPayload{MaxBackdatingDays: 5})
	if err != nil {
		t.Fatal(err)
	}
	baseVersion := storage.PolicyVersion{
		CompanyID: companyID, PolicyID: uuid.New(), Version: 1,
		EffectiveFrom: effectiveFrom, EffectiveTo: &effectiveTo, Payload: postingPayload,
	}
	if _, err := postingPolicyFromVersion(baseVersion); err != nil {
		t.Fatalf("valid posting policy = %v", err)
	}
	badJSON := baseVersion
	badJSON.Payload = []byte("{")
	if _, err := postingPolicyFromVersion(badJSON); err == nil {
		t.Fatal("expected posting policy decode error")
	}
	invalidPostingPayload, err := json.Marshal(storage.PostingPolicyPayload{MaxBackdatingDays: -1})
	if err != nil {
		t.Fatal(err)
	}
	invalidPosting := baseVersion
	invalidPosting.Payload = invalidPostingPayload
	if _, err := postingPolicyFromVersion(invalidPosting); err == nil {
		t.Fatal("expected posting policy validation error")
	}

	currencyPayload, err := json.Marshal(storage.CurrencyPolicyPayload{
		FunctionalCurrency: "usd", PresentationCurrency: "usd", RoundingMethod: "bankers",
		AllowedCurrencies: []string{"USD"},
	})
	if err != nil {
		t.Fatal(err)
	}
	currencyVersion := baseVersion
	currencyVersion.Payload = currencyPayload
	if _, err := currencyPolicyFromVersion(currencyVersion); err != nil {
		t.Fatalf("valid currency policy = %v", err)
	}
	badCurrencyJSON := currencyVersion
	badCurrencyJSON.Payload = []byte("{")
	if _, err := currencyPolicyFromVersion(badCurrencyJSON); err == nil {
		t.Fatal("expected currency policy decode error")
	}
	badAccountPayload, err := json.Marshal(storage.CurrencyPolicyPayload{
		FunctionalCurrency: "USD", RoundingDifferenceAccountID: "not-a-uuid",
	})
	if err != nil {
		t.Fatal(err)
	}
	badAccount := currencyVersion
	badAccount.Payload = badAccountPayload
	if _, err := currencyPolicyFromVersion(badAccount); err == nil {
		t.Fatal("expected rounding account parse error")
	}
	invalidCurrencyPayload, err := json.Marshal(storage.CurrencyPolicyPayload{
		FunctionalCurrency: "XXX", AutoSettleRoundingDifferences: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	invalidCurrency := currencyVersion
	invalidCurrency.Payload = invalidCurrencyPayload
	if _, err := currencyPolicyFromVersion(invalidCurrency); err == nil {
		t.Fatal("expected currency policy validation error")
	}
}

func TestP2YearEndValidationBranches(t *testing.T) {
	companyID := p2CompanyID()
	commandContext := publicapplication.Context{Principal: publicapplication.Principal{
		ActorID: "actor", CompanyIDs: []types.CompanyID{companyID},
	}}
	app := &applicationService{}
	closeCommand := publicapplication.ExecuteYearEndClosingCommand{Context: commandContext}
	if _, err := app.ExecuteYearEndClosing(context.Background(), closeCommand); err == nil {
		t.Fatal("expected empty close validation error")
	}
	closeCommand.CompanyID = companyID
	if _, err := app.ExecuteYearEndClosing(context.Background(), closeCommand); err == nil {
		t.Fatal("expected missing fiscal year validation error")
	}
	closeCommand.FiscalYearID = types.NewFiscalYearID()
	if _, err := app.ExecuteYearEndClosing(context.Background(), closeCommand); err == nil {
		t.Fatal("expected missing journal validation error")
	}
	closeCommand.JournalID = types.NewJournalID()
	if _, err := app.ExecuteYearEndClosing(context.Background(), closeCommand); err == nil {
		t.Fatal("expected missing retained earnings account validation error")
	}
	closeCommand.RetainedEarningsAccountID = types.NewAccountID()
	if _, err := app.ExecuteYearEndClosing(context.Background(), closeCommand); err == nil {
		t.Fatal("expected missing reason validation error")
	}
	closeCommand.Reason = "reason"
	if _, err := app.ExecuteYearEndClosing(context.Background(), closeCommand); !errors.Is(err, ErrCapabilityNotImplemented) {
		t.Fatalf("missing year-end capability = %v", err)
	}

	reopenCommand := publicapplication.ReopenYearEndClosingCommand{Context: commandContext}
	if _, err := app.ReopenYearEndClosing(context.Background(), reopenCommand); err == nil {
		t.Fatal("expected empty reopen validation error")
	}
	reopenCommand.CompanyID = companyID
	if _, err := app.ReopenYearEndClosing(context.Background(), reopenCommand); err == nil {
		t.Fatal("expected missing reopen fiscal year validation error")
	}
	reopenCommand.FiscalYearID = types.NewFiscalYearID()
	if _, err := app.ReopenYearEndClosing(context.Background(), reopenCommand); err == nil {
		t.Fatal("expected missing reopen reason validation error")
	}
	reopenCommand.Reason = "reason"
	if _, err := app.ReopenYearEndClosing(context.Background(), reopenCommand); err == nil {
		t.Fatal("expected missing approval reference validation error")
	}
	reopenCommand.ApprovalReference = "approval"
	if _, err := app.ReopenYearEndClosing(context.Background(), reopenCommand); !errors.Is(err, ErrCapabilityNotImplemented) {
		t.Fatalf("missing reopen capability = %v", err)
	}
}

func TestP2EngineCapabilityHelpers(t *testing.T) {
	plain := &p2EngineWithoutCapabilities{}
	if _, err := YearEndClosing(plain); !errors.Is(err, types.ErrCapabilityNotImplemented) {
		t.Fatalf("YearEndClosing = %v", err)
	}
	if _, err := DraftCancellation(plain); !errors.Is(err, types.ErrCapabilityNotImplemented) {
		t.Fatalf("DraftCancellation = %v", err)
	}
	if _, err := SubledgerARAP(plain); !errors.Is(err, types.ErrCapabilityNotImplemented) {
		t.Fatalf("SubledgerARAP = %v", err)
	}
	if _, err := Consolidation(plain); !errors.Is(err, types.ErrCapabilityNotImplemented) {
		t.Fatalf("Consolidation = %v", err)
	}
	if _, err := Reconciliation(plain); !errors.Is(err, types.ErrCapabilityNotImplemented) {
		t.Fatalf("Reconciliation = %v", err)
	}
}

type p2EngineWithoutCapabilities struct{}

func (*p2EngineWithoutCapabilities) Ledger() LedgerService                  { return nil }
func (*p2EngineWithoutCapabilities) Reporting() ReportingService            { return nil }
func (*p2EngineWithoutCapabilities) Governance() GovernanceService          { return nil }
func (*p2EngineWithoutCapabilities) Events() EventIngestionService          { return nil }
func (*p2EngineWithoutCapabilities) Standards() StandardsService            { return nil }
func (*p2EngineWithoutCapabilities) Scheduler() standards.SchedulerPort     { return nil }
func (*p2EngineWithoutCapabilities) Close() error                           { return nil }
func (*p2EngineWithoutCapabilities) CloseWithContext(context.Context) error { return nil }

func consolidationRateRequest(from, to string, class publicconsolidation.RateClass) internalconsolidation.RateRequest {
	return internalconsolidation.RateRequest{FromCurrency: from, ToCurrency: to, Class: internalconsolidation.RateClass(class)}
}

func TestP3BuilderNilOptionAndReportingCashFlow(t *testing.T) {
	builder := NewBuilder(nil)
	if len(builder.optsErrs) != 1 || !errors.Is(builder.optsErrs[0], ErrBuilderOptionNil) {
		t.Fatalf("nil builder option errors = %v", builder.optsErrs)
	}
	if _, err := builder.Build(context.Background()); err == nil {
		t.Fatal("expected build error for nil option")
	}

	if _, err := (&defaultReportingService{}).GetCashFlow(context.Background(), reporting.CashFlowRequest{}); !errors.Is(err, ErrCapabilityNotImplemented) {
		t.Fatalf("cash flow without store = %v", err)
	}
}

func TestP3ClosingRepositoryFailurePaths(t *testing.T) {
	ctx := context.Background()
	companyID := p2CompanyID()
	store := &p2ClosingStore{err: errors.New("closing store unavailable")}
	repo := &sdkClosingRepository{
		masterData: &p2MasterDataReader{}, entries: p2EntryReader{err: store.err}, store: store, companyID: companyID,
	}
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0)
	if _, err := repo.ListEntriesForCompanyContext(ctx, uuid.UUID(companyID), start, end); err == nil {
		t.Fatal("expected entry list error")
	}
	if _, err := repo.GetEntryByIDForCompanyContext(ctx, uuid.UUID(companyID), uuid.New()); err == nil {
		t.Fatal("expected entry lookup error")
	}
	if _, err := repo.IsFirstFiscalYearForCompanyContext(ctx, uuid.UUID(companyID), uuid.New()); err == nil {
		t.Fatal("expected fiscal year list error")
	}
}

func TestP3PostedEntryIdempotencyLookupPaths(t *testing.T) {
	ctx := context.Background()
	companyID := p2CompanyID()
	entryID := types.NewJournalEntryID()
	lookup := &p2IdemLookup{record: storage.IdempotencyRecord{
		Status:  storage.IdempotencyCompleted,
		EntryID: &entryID,
	}, found: true}
	app := &applicationService{ports: storage.Ports{
		Idempotency: lookup,
		Entries:     p2EntryReader{entry: p2PostedEntry(companyID, entryID)},
	}}
	if got, found, err := app.FindPostedEntryByIdempotencyKey(ctx, companyID, " key "); err != nil || !found || got.ID != entryID {
		t.Fatalf("completed lookup = %+v %v %v", got, found, err)
	}
	lookup.found = false
	if _, found, err := app.FindPostedEntryByIdempotencyKey(ctx, companyID, "key"); err != nil || found {
		t.Fatalf("missing lookup = %v %v", found, err)
	}
	lookup.found = true
	lookup.err = errors.New("lookup unavailable")
	if _, found, err := app.FindPostedEntryByIdempotencyKey(ctx, companyID, "key"); err == nil || found {
		t.Fatalf("failed lookup = %v %v", found, err)
	}
	lookup.err = nil
	app.ports.Entries = p2EntryReader{err: errors.New("entry unavailable")}
	if _, found, err := app.FindPostedEntryByIdempotencyKey(ctx, companyID, "key"); err == nil || found {
		t.Fatalf("entry load failure = %v %v", found, err)
	}
}

func TestP3ValidateAndClassifyErrorBranches(t *testing.T) {
	ctx := context.Background()
	companyID := p2CompanyID()
	draft := journal.JournalEntry{
		ID: types.NewJournalEntryID(), CompanyID: companyID, JournalID: types.NewJournalID(),
		Status: journal.EntryDraft, EffectiveDate: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
	}

	notDraft := draft
	notDraft.Status = journal.EntryPosted
	app := &applicationService{ports: storage.Ports{Entries: p2EntryReader{entry: notDraft}}}
	cmd := publicapplication.ValidateEntryCommand{CompanyID: companyID, EntryID: notDraft.ID}
	if err := app.Validate(ctx, cmd); !errors.Is(err, publicapplication.ErrValidation) {
		t.Fatalf("validate posted entry = %v", err)
	}

	policyApp := newApplication(storage.Ports{
		MasterData: &p2MasterDataReader{companyErr: errors.New("company unavailable")},
		Entries:    p2EntryReader{entry: draft},
	}, PostingConfig{}, nil)
	if err := policyApp.Validate(ctx, cmd); err == nil {
		t.Fatal("expected policy registration error during validation")
	}

	for _, tc := range []struct {
		name string
		err  error
		want error
	}{
		{name: "dependency", err: storage.ErrIdempotencyUnavailable, want: publicapplication.ErrDependencyUnavailable},
		{name: "internal", err: storage.ErrAtomicCommitFailed, want: publicapplication.ErrInternal},
	} {
		err := classifyError(tc.err)
		if !errors.Is(err, tc.want) {
			t.Fatalf("%s classification = %v, want %v", tc.name, err, tc.want)
		}
	}
}
