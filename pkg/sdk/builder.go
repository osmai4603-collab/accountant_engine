package sdk

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/events"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/journal"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/reporting"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	publicapplication "github.com/osmai4603-collab/accounting_engine/pkg/spi/application"
	publicconsolidation "github.com/osmai4603-collab/accounting_engine/pkg/spi/consolidation"
	publicmasterdata "github.com/osmai4603-collab/accounting_engine/pkg/spi/masterdata"
	publicreconciliation "github.com/osmai4603-collab/accounting_engine/pkg/spi/reconciliation"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/standards"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/storage"
	publicsubledger "github.com/osmai4603-collab/accounting_engine/pkg/spi/subledger"
)

// EngineBuilder composes the embeddable engine from public storage ports.
// Only WithStoragePorts is supported as the implementation-neutral path.
type EngineBuilder struct {
	ports                storage.Ports
	portsConfigured      bool
	reportingStore       storage.ReportingStore
	governanceStore      storage.GovernanceStore
	authorizer           publicapplication.Authorizer
	standards            []standards.StandardModule
	standardsRegistry    *standards.Registry
	postingConfig        PostingConfig
	postingCaps          storage.PostingCapabilities
	requiredCapabilities []BootstrapCapability
	eventWaitTimeout     time.Duration
	eventWaitInterval    time.Duration
	optsErrs             []error

	useCases             publicapplication.Service
	modulePorts          standards.ModulePorts
	arapStores           publicsubledger.ARAPStores
	arapStoresConfigured bool
}

const (
	defaultEventIdempotencyWaitTimeout  = 2 * time.Second
	defaultEventIdempotencyWaitInterval = 25 * time.Millisecond
)

func NewBuilder(opts ...Option) *EngineBuilder {
	b := &EngineBuilder{
		eventWaitTimeout:  defaultEventIdempotencyWaitTimeout,
		eventWaitInterval: defaultEventIdempotencyWaitInterval,
	}
	for _, opt := range opts {
		if opt == nil {
			b.optsErrs = append(b.optsErrs, ErrBuilderOptionNil)
			continue
		}
		if err := opt(b); err != nil {
			b.optsErrs = append(b.optsErrs, err)
		}
	}
	return b
}

// Build validates dependencies and produces an initialized Engine.
func (b *EngineBuilder) Build(ctx context.Context) (Engine, error) {
	errs := b.preflight()
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	appService, err := NewApplicationServiceWithPostingConfig(
		b.ports, b.postingConfig, b.authorizer,
	)
	if err != nil {
		return nil, fmt.Errorf("application layer initialization: %w", err)
	}
	b.useCases = appService

	var publicMasterData publicmasterdata.Store
	if store, ok := b.ports.MasterData.(publicmasterdata.Store); ok {
		publicMasterData = store
	}

	scheduler := standards.NewDefaultScheduler()
	eventRouter := events.NewRouter()

	if len(b.standards) > 0 {
		b.modulePorts = b.buildModulePorts(publicMasterData)
		if moduleErrs := b.validateStandardModules(); len(moduleErrs) > 0 {
			return nil, errors.Join(moduleErrs...)
		}
	}

	if moduleErrs := b.initializeStandards(ctx, eventRouter); len(moduleErrs) > 0 {
		return nil, errors.Join(moduleErrs...)
	}

	scheduler.SetReady(true)

	// Register scheduled jobs from active modules
	for _, mod := range b.standards {
		if mod == nil {
			continue
		}
		if err := mod.RunScheduledJobs(ctx, scheduler); err != nil {
			if b.standardsRegistry != nil {
				b.standardsRegistry.SetDisabledReason(fmt.Sprintf("module:%s:jobs", mod.Name()), err.Error())
			}
		}
	}

	eventSvc := newDefaultEventService(
		b.ports.Events.Idempotency,
		b.ports.Events.Mappings,
		eventRouter,
		publicMasterData,
		b.useCases,
		b.authorizer != nil,
		b.eventWaitTimeout,
		b.eventWaitInterval,
	)

	inst := &engineInstance{
		ports:             b.ports,
		reportingStore:    b.reportingStore,
		governanceStore:   b.governanceStore,
		standards:         b.standards,
		standardsRegistry: b.standardsRegistry,
		scheduler:         scheduler,
		eventService:      eventSvc,
		publicMasterData:  publicMasterData,
		useCases:          b.useCases,
		yearEndClosing:    optionalYearEndClosingService(b.useCases),
		requirePrincipal:  b.authorizer != nil,
	}

	return inst, nil
}

func (b *EngineBuilder) preflight() []error {
	errs := append([]error(nil), b.optsErrs...)

	if !b.portsConfigured {
		errs = append(errs, ErrStoragePortsRequired)
	} else {
		if err := b.ports.Validate(); err != nil {
			errs = append(errs, fmt.Errorf("storage ports: %w", err))
		}
	}

	if b.standardsRegistry == nil {
		b.standardsRegistry = standards.NewRegistry()
	}

	b.standardsRegistry.RegisterCapability(standards.CapLedgerPosting, "engine_core")
	b.standardsRegistry.RegisterCapability(standards.CapMasterData, "engine_core")

	if b.reportingStore != nil || (b.portsConfigured && b.ports.Balance != nil && b.ports.MasterData != nil && b.ports.Entries != nil) {
		b.standardsRegistry.RegisterCapability(standards.CapReporting, "reporting_service")
	} else {
		b.standardsRegistry.SetDisabledReason(string(standards.CapReporting), "reporting data sources missing")
	}

	if b.governanceStore != nil || (b.portsConfigured && b.ports.Audit != nil) {
		b.standardsRegistry.RegisterCapability(standards.CapGovernance, "governance_service")
		b.standardsRegistry.RegisterCapability(standards.CapAuditTrail, "governance_service")
	} else {
		b.standardsRegistry.SetDisabledReason(string(standards.CapGovernance), "governance required stores missing")
	}

	if len(b.standards) > 0 {
		registry := b.standardsRegistry
		for _, mod := range b.standards {
			if mod == nil {
				errs = append(errs, errors.New("standard module is nil"))
				continue
			}
			if err := registry.Register(mod); err != nil {
				errs = append(errs, err)
			}
		}

		resolvedMods, resErr := registry.Resolve()
		if resErr != nil {
			errs = append(errs, resErr)
		} else {
			b.standards = resolvedMods
			b.standardsRegistry = registry
		}
	}

	if err := b.validatePostingConfig(); err != nil {
		errs = append(errs, err)
	}

	errs = append(errs, b.validateRequiredCapabilities()...)
	return errs
}

func (b *EngineBuilder) validateStandardModules() []error {
	var errs []error
	for _, mod := range b.standards {
		if mod == nil {
			continue
		}
		if err := mod.ValidateDependencies(b.modulePorts); err != nil {
			errs = append(errs, fmt.Errorf("standard module %s: dependency validation failed: %w", mod.Name(), err))
		}
	}
	return errs
}

func (b *EngineBuilder) initializeStandards(ctx context.Context, router *events.Router) []error {
	var initializedMods []standards.StandardModule

	rollback := func(initErr error) []error {
		var combinedErrs []error
		combinedErrs = append(combinedErrs, initErr)
		for i := len(initializedMods) - 1; i >= 0; i-- {
			mod := initializedMods[i]
			if closeErr := mod.Close(ctx); closeErr != nil {
				combinedErrs = append(combinedErrs, fmt.Errorf("module %s rollback close failed: %w", mod.Name(), closeErr))
			}
		}
		return combinedErrs
	}

	for _, mod := range b.standards {
		if mod == nil {
			continue
		}
		name := mod.Name()
		if err := mod.Initialize(ctx, b.modulePorts); err != nil {
			return rollback(fmt.Errorf("standard module %s: initialization failed: %w", name, err))
		}
		initializedMods = append(initializedMods, mod)

		if router != nil {
			if err := mod.RegisterRules(router); err != nil {
				return rollback(fmt.Errorf("standard module %s: rule registration failed: %w", name, err))
			}
		}
	}
	return nil
}

type engineInstance struct {
	ports             storage.Ports
	reportingStore    storage.ReportingStore
	governanceStore   storage.GovernanceStore
	standards         []standards.StandardModule
	standardsRegistry *standards.Registry
	scheduler         standards.SchedulerPort
	eventService      EventIngestionService
	publicMasterData  publicmasterdata.Store
	useCases          publicapplication.Service
	yearEndClosing    publicapplication.YearEndClosingService
	arapService       publicsubledger.ARAPService
	requirePrincipal  bool

	closeOnce sync.Once
	closeErr  error
}

func (e *engineInstance) Ledger() LedgerService {
	return &defaultLedgerService{
		useCases:         e.useCases,
		requirePrincipal: e.requirePrincipal,
	}
}

func (e *engineInstance) Reporting() ReportingService {
	if e.reportingStore == nil {
		return &disabledReportingService{}
	}
	return &defaultReportingService{store: e.reportingStore}
}

func (e *engineInstance) Governance() GovernanceService {
	if e.governanceStore == nil && e.ports.Audit == nil {
		return &disabledGovernanceService{}
	}
	return &defaultGovernanceService{store: e.governanceStore}
}

func (e *engineInstance) Events() EventIngestionService {
	return e.eventService
}

func (e *engineInstance) Standards() StandardsService {
	return &defaultStandardsService{
		registry:  e.standardsRegistry,
		scheduler: e.scheduler,
	}
}

func (e *engineInstance) Scheduler() standards.SchedulerPort {
	return e.scheduler
}

func (e *engineInstance) YearEndClosing() publicapplication.YearEndClosingService {
	return e.yearEndClosing
}

func (e *engineInstance) DraftCancellation() publicapplication.DraftCancellationService {
	capability, _ := e.useCases.(publicapplication.DraftCancellationService)
	return capability
}

func (e *engineInstance) Consolidation() publicconsolidation.Service {
	return &defaultConsolidationService{}
}

func (e *engineInstance) Reconciliation() publicreconciliation.Service {
	return &defaultReconciliationService{}
}

func (e *engineInstance) Close() error {
	return e.CloseWithContext(context.Background())
}

func (e *engineInstance) CloseWithContext(ctx context.Context) error {
	e.closeOnce.Do(func() {
		var errs []error
		if ctx.Err() != nil {
			errs = append(errs, ctx.Err())
		}
		for i := len(e.standards) - 1; i >= 0; i-- {
			mod := e.standards[i]
			if err := mod.Close(ctx); err != nil {
				errs = append(errs, fmt.Errorf("module %s close failed: %w", mod.Name(), err))
			}
		}
		if len(errs) > 0 {
			e.closeErr = errors.Join(errs...)
		}
	})
	return e.closeErr
}

func optionalYearEndClosingService(service publicapplication.Service) publicapplication.YearEndClosingService {
	capability, _ := service.(publicapplication.YearEndClosingService)
	return capability
}

type defaultStandardsService struct {
	registry  *standards.Registry
	scheduler standards.SchedulerPort
}

func (s *defaultStandardsService) Registry() *standards.Registry {
	return s.registry
}

func (s *defaultStandardsService) Diagnostics() standards.EngineDiagnostics {
	if s.registry == nil {
		return standards.EngineDiagnostics{}
	}
	return s.registry.Diagnostics()
}

func (s *defaultStandardsService) HasCapability(cap standards.Capability) bool {
	if s.registry == nil {
		return false
	}
	return s.registry.HasCapability(cap)
}

func (s *defaultStandardsService) Scheduler() standards.SchedulerPort {
	return s.scheduler
}

type defaultLedgerService struct {
	useCases         publicapplication.Service
	requirePrincipal bool
}

func (s *defaultLedgerService) PostEntry(ctx context.Context, entry journal.JournalEntry) (journal.JournalEntry, error) {
	if s.useCases == nil {
		return journal.JournalEntry{}, ErrCapabilityNotImplemented
	}
	if err := validatePostingEntry(entry); err != nil {
		return journal.JournalEntry{}, err
	}
	appCtx := commandContext(ctx, entry.CompanyID, "sdk", "sdk", s.requirePrincipal)
	cmd := publicapplication.CreateDraftCommand{
		Context:             appCtx,
		CompanyID:           entry.CompanyID,
		JournalID:           entry.JournalID,
		FiscalPeriodID:      entry.FiscalPeriodID,
		EffectiveDate:       entry.EffectiveDate,
		PostingDate:         entry.PostingDate,
		Description:         entry.Description,
		SourceModule:        entry.SourceModule,
		ReferenceID:         entry.ReferenceID,
		TransactionCurrency: entry.TransactionCurrency,
		Classification:      entry.Classification,
		TargetPeriodNumber:  entry.TargetPeriodNumber,
		AdjustingEntryType:  entry.AdjustingEntryType,
		AdjustmentReason:    entry.AdjustmentReason,
		Lines:               entry.Lines,
	}

	var draftResult publicapplication.EntryResult
	var err error
	if creator, ok := s.useCases.(publicapplication.DeterministicDraftCreator); ok && entry.ID != (types.JournalEntryID{}) {
		draftResult, err = creator.CreateDraftWithID(ctx, cmd, entry.ID)
	} else {
		draftResult, err = s.useCases.CreateDraft(ctx, cmd)
	}
	if err != nil {
		return journal.JournalEntry{}, err
	}

	postCmd := publicapplication.PostEntryCommand{
		Context:   appCtx,
		CompanyID: entry.CompanyID,
		EntryID:   draftResult.Entry.ID,
	}
	postedResult, err := s.useCases.Post(ctx, postCmd)
	if err != nil {
		return journal.JournalEntry{}, err
	}
	return postedResult.Entry, nil
}

func (s *defaultLedgerService) GetEntry(ctx context.Context, companyID types.CompanyID, entryID types.JournalEntryID) (journal.JournalEntry, error) {
	if s.useCases == nil {
		return journal.JournalEntry{}, ErrCapabilityNotImplemented
	}
	query := publicapplication.GetEntryQuery{
		Context:   commandContext(ctx, companyID, "sdk", "sdk", s.requirePrincipal),
		CompanyID: companyID,
		EntryID:   entryID,
	}
	return s.useCases.GetEntry(ctx, query)
}

func (s *defaultLedgerService) ReverseEntry(ctx context.Context, companyID types.CompanyID, entryID types.JournalEntryID, reason string) (journal.JournalEntry, error) {
	if s.useCases == nil {
		return journal.JournalEntry{}, ErrCapabilityNotImplemented
	}
	cmd := publicapplication.ReverseEntryCommand{
		Context:       commandContext(ctx, companyID, "sdk", "sdk", s.requirePrincipal),
		CompanyID:     companyID,
		EntryID:       entryID,
		Reason:        reason,
		EffectiveDate: time.Now().UTC(),
	}
	reversedResult, err := s.useCases.Reverse(ctx, cmd)
	if err != nil {
		return journal.JournalEntry{}, err
	}
	return reversedResult.Entry, nil
}

func commandContext(ctx context.Context, companyID types.CompanyID, fallbackActor, sourceSystem string, requirePrincipal bool) publicapplication.Context {
	metadata, _ := publicapplication.RequestMetadataFromContext(ctx)
	if principal, ok := publicapplication.PrincipalFromContext(ctx); ok {
		return publicapplication.Context{
			Principal:      principal,
			CorrelationID:  metadata.CorrelationID,
			IdempotencyKey: metadata.IdempotencyKey,
			SourceSystem:   metadataSource(metadata, sourceSystem),
		}
	}
	if requirePrincipal {
		return publicapplication.Context{
			CorrelationID:  metadata.CorrelationID,
			IdempotencyKey: metadata.IdempotencyKey,
			SourceSystem:   metadataSource(metadata, sourceSystem),
		}
	}
	return publicapplication.Context{
		Principal: publicapplication.Principal{
			ActorID:    fallbackActor,
			CompanyIDs: []types.CompanyID{companyID},
		},
		CorrelationID:  metadata.CorrelationID,
		IdempotencyKey: metadata.IdempotencyKey,
		SourceSystem:   metadataSource(metadata, sourceSystem),
	}
}

func metadataSource(metadata publicapplication.RequestMetadata, fallback string) string {
	if source := strings.TrimSpace(metadata.SourceSystem); source != "" {
		return source
	}
	return fallback
}

type disabledReportingService struct{}

func (d *disabledReportingService) GetTrialBalance(ctx context.Context, companyID types.CompanyID, asOf time.Time, currency string) (reporting.TrialBalance, error) {
	return reporting.TrialBalance{}, ErrCapabilityNotImplemented
}
func (d *disabledReportingService) GetBalanceSheet(ctx context.Context, companyID types.CompanyID, asOf time.Time, currency string) (reporting.BalanceSheetReport, error) {
	return reporting.BalanceSheetReport{}, ErrCapabilityNotImplemented
}
func (d *disabledReportingService) GetIncomeStatement(ctx context.Context, companyID types.CompanyID, start, end time.Time, currency string) (reporting.IncomeStatementReport, error) {
	return reporting.IncomeStatementReport{}, ErrCapabilityNotImplemented
}
func (d *disabledReportingService) GetGeneralLedger(ctx context.Context, companyID types.CompanyID, accountID types.AccountID, start, end time.Time) (reporting.GeneralLedgerReport, error) {
	return reporting.GeneralLedgerReport{}, ErrCapabilityNotImplemented
}
func (d *disabledReportingService) GetCashFlow(ctx context.Context, request reporting.CashFlowRequest) (reporting.CashFlowStatementReport, error) {
	return reporting.CashFlowStatementReport{}, ErrCapabilityNotImplemented
}
func (d *disabledReportingService) GetDisclosures(ctx context.Context, query reporting.DisclosureQuery) ([]reporting.StructuredDisclosure, error) {
	return nil, ErrCapabilityNotImplemented
}

type defaultReportingService struct {
	store storage.ReportingStore
}

func (s *defaultReportingService) GetTrialBalance(ctx context.Context, companyID types.CompanyID, asOf time.Time, currency string) (reporting.TrialBalance, error) {
	if s.store == nil {
		return reporting.TrialBalance{}, ErrCapabilityNotImplemented
	}
	return s.store.GetTrialBalance(ctx, companyID, asOf, currency)
}

func (s *defaultReportingService) GetBalanceSheet(ctx context.Context, companyID types.CompanyID, asOf time.Time, currency string) (reporting.BalanceSheetReport, error) {
	if s.store == nil {
		return reporting.BalanceSheetReport{}, ErrCapabilityNotImplemented
	}
	return s.store.GetBalanceSheet(ctx, companyID, asOf, currency)
}

func (s *defaultReportingService) GetIncomeStatement(ctx context.Context, companyID types.CompanyID, start, end time.Time, currency string) (reporting.IncomeStatementReport, error) {
	if s.store == nil {
		return reporting.IncomeStatementReport{}, ErrCapabilityNotImplemented
	}
	return s.store.GetIncomeStatement(ctx, companyID, start, end, currency)
}

func (s *defaultReportingService) GetGeneralLedger(ctx context.Context, companyID types.CompanyID, accountID types.AccountID, start, end time.Time) (reporting.GeneralLedgerReport, error) {
	if s.store == nil {
		return reporting.GeneralLedgerReport{}, ErrCapabilityNotImplemented
	}
	return s.store.GetGeneralLedger(ctx, companyID, accountID, start, end)
}

func (s *defaultReportingService) GetDisclosures(ctx context.Context, query reporting.DisclosureQuery) ([]reporting.StructuredDisclosure, error) {
	return nil, ErrCapabilityNotImplemented
}

type disabledGovernanceService struct{}

func (d *disabledGovernanceService) RecordAuditLog(ctx context.Context, eventID string, details map[string]any) error {
	return ErrCapabilityNotImplemented
}
func (d *disabledGovernanceService) GetApprovalFingerprint(ctx context.Context, entryID types.JournalEntryID) (string, error) {
	return "", ErrCapabilityNotImplemented
}

type defaultGovernanceService struct {
	store storage.GovernanceStore
}

func (s *defaultGovernanceService) RecordAuditLog(ctx context.Context, eventID string, details map[string]any) error {
	if s.store == nil {
		return ErrCapabilityNotImplemented
	}
	return s.store.RecordAuditLog(ctx, eventID, details)
}

func (s *defaultGovernanceService) GetApprovalFingerprint(ctx context.Context, entryID types.JournalEntryID) (string, error) {
	if s.store == nil {
		return "", ErrCapabilityNotImplemented
	}
	return s.store.GetApprovalFingerprint(ctx, entryID)
}
