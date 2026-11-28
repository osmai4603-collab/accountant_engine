package sdk

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/osmai4603-collab/accounting_engine/internal/kernel/apperrors"
	"github.com/osmai4603-collab/accounting_engine/internal/kernel/governance"
	"github.com/osmai4603-collab/accounting_engine/internal/kernel/journal"
	kernelmasterdata "github.com/osmai4603-collab/accounting_engine/internal/kernel/masterdata"
	"github.com/osmai4603-collab/accounting_engine/internal/kernel/policy"
	"github.com/osmai4603-collab/accounting_engine/internal/kernel/posting"
	"github.com/osmai4603-collab/accounting_engine/internal/kernel/types"
	"github.com/osmai4603-collab/accounting_engine/internal/kernel/valuation"
	publictypes "github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	publicapplication "github.com/osmai4603-collab/accounting_engine/pkg/spi/application"
	publicmasterdata "github.com/osmai4603-collab/accounting_engine/pkg/spi/masterdata"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/storage"
	"github.com/shopspring/decimal"
)

func NewApplicationService(ports storage.Ports) (publicapplication.Service, error) {
	return NewApplicationServiceWithAuthorizer(ports, nil)
}

func NewApplicationServiceWithAuthorizer(
	ports storage.Ports,
	authorizer publicapplication.Authorizer,
) (publicapplication.Service, error) {
	return NewApplicationServiceWithPostingConfig(ports, PostingConfig{}, authorizer)
}

func NewApplicationServiceWithPostingConfig(
	ports storage.Ports,
	config PostingConfig,
	authorizer publicapplication.Authorizer,
) (publicapplication.Service, error) {
	if err := ports.Validate(); err != nil {
		return nil, fmt.Errorf("storage ports invalid: %w", err)
	}

	app := newApplication(ports, config, authorizer)
	return app, nil
}

// NewApplicationServiceWithObserver builds an application service with an
// optional operational Observer. A nil observer falls back to the no-op
// implementation, so instrumentation never becomes a dependency.
func NewApplicationServiceWithObserver(
	ports storage.Ports,
	config PostingConfig,
	authorizer publicapplication.Authorizer,
	observer publicapplication.Observer,
) (publicapplication.Service, error) {
	service, err := NewApplicationServiceWithPostingConfig(ports, config, authorizer)
	if err != nil {
		return nil, err
	}
	if observer == nil {
		return service, nil
	}
	app, ok := service.(*applicationService)
	if !ok {
		return service, nil
	}
	app.observer = observer
	return app, nil
}

type masterDataReaderAdapter struct {
	reader                       storage.MasterDataReader
	allowSyntheticDefaultJournal bool
}

func (a masterDataReaderAdapter) GetCompany(ctx context.Context, companyID types.CompanyID) (kernelmasterdata.Company, error) {
	c, err := a.reader.GetCompany(ctx, companyID)
	if err != nil {
		return kernelmasterdata.Company{}, err
	}
	return kernelmasterdata.Company{
		ID:                   c.ID,
		Code:                 c.Code,
		LegalName:            c.LegalName,
		FunctionalCurrency:   c.FunctionalCurrency,
		PresentationCurrency: c.PresentationCurrency,
		Active:               c.Active,
	}, nil
}

func (a masterDataReaderAdapter) GetAccount(ctx context.Context, companyID types.CompanyID, accountID types.AccountID) (kernelmasterdata.Account, error) {
	return a.reader.GetAccount(ctx, companyID, accountID)
}

func (a masterDataReaderAdapter) GetAccountByCode(ctx context.Context, companyID types.CompanyID, code string) (kernelmasterdata.Account, error) {
	return a.reader.GetAccountByCode(ctx, companyID, code)
}

func (a masterDataReaderAdapter) GetJournalByCode(ctx context.Context, companyID types.CompanyID, code string) (kernelmasterdata.Journal, error) {
	j, err := a.reader.GetJournalByCode(ctx, companyID, code)
	if err != nil {
		return kernelmasterdata.Journal{}, err
	}
	return toKernelJournal(j), nil
}

func (a masterDataReaderAdapter) HasAccountChildren(ctx context.Context, companyID types.CompanyID, accountID types.AccountID) (bool, error) {
	return a.reader.HasAccountChildren(ctx, companyID, accountID)
}

func (a masterDataReaderAdapter) GetFiscalYear(ctx context.Context, companyID types.CompanyID, fiscalYearID types.FiscalYearID) (kernelmasterdata.FiscalYear, error) {
	return a.reader.GetFiscalYear(ctx, companyID, fiscalYearID)
}

func (a masterDataReaderAdapter) GetPeriodForDate(ctx context.Context, companyID types.CompanyID, date time.Time) (kernelmasterdata.FiscalPeriod, error) {
	return a.reader.GetPeriodForDate(ctx, companyID, date)
}

func (a masterDataReaderAdapter) GetJournalByID(ctx context.Context, companyID types.CompanyID, journalID types.JournalID) (kernelmasterdata.Journal, error) {
	if repo, ok := a.reader.(interface {
		GetJournalByID(ctx context.Context, companyID types.CompanyID, journalID types.JournalID) (kernelmasterdata.Journal, error)
	}); ok {
		return repo.GetJournalByID(ctx, companyID, journalID)
	}
	if repo, ok := a.reader.(interface {
		GetJournalByID(ctx context.Context, companyID types.CompanyID, journalID types.JournalID) (publicmasterdata.Journal, error)
	}); ok {
		j, err := repo.GetJournalByID(ctx, companyID, journalID)
		if err != nil {
			return kernelmasterdata.Journal{}, err
		}
		return toKernelJournal(j), nil
	}
	if !a.allowSyntheticDefaultJournal {
		return kernelmasterdata.Journal{}, kernelmasterdata.ErrJournalReaderRequired
	}
	j, err := a.reader.GetJournalByCode(ctx, companyID, "GENERAL")
	if err == nil && j.ID == journalID {
		return toKernelJournal(j), nil
	}
	return kernelmasterdata.Journal{
		ID:            journalID,
		CompanyID:     companyID,
		Code:          "GENERAL",
		Name:          "General Journal",
		EffectiveFrom: time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
	}, nil
}

func (a masterDataReaderAdapter) GetPeriodByNumber(ctx context.Context, companyID types.CompanyID, fiscalYearID types.FiscalYearID, number int) (kernelmasterdata.FiscalPeriod, error) {
	if repo, ok := a.reader.(publicmasterdata.FiscalPeriodNumberReader); ok {
		return repo.GetPeriodByNumber(ctx, companyID, fiscalYearID, number)
	}
	if repo, ok := a.reader.(interface {
		GetPeriodByNumber(ctx context.Context, companyID types.CompanyID, fiscalYearID types.FiscalYearID, number int) (kernelmasterdata.FiscalPeriod, error)
	}); ok {
		return repo.GetPeriodByNumber(ctx, companyID, fiscalYearID, number)
	}
	return kernelmasterdata.FiscalPeriod{}, fmt.Errorf("%w: adjustment periods not supported", publictypes.ErrCapabilityNotImplemented)
}

func toKernelJournal(j publicmasterdata.Journal) kernelmasterdata.Journal {
	return kernelmasterdata.Journal{
		ID:                j.ID,
		CompanyID:         j.CompanyID,
		Code:              j.Code,
		Name:              j.Name,
		AllowedCurrencies: j.AllowedCurrencies,
		AllowedAccountIDs: j.AllowedAccountIDs,
		EffectiveFrom:     time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
	}
}

// capabilityMasterDataReader adds the optional Phase 11 capabilities only when
// the underlying port actually implements them. It is deliberately a distinct
// type from masterDataReaderAdapter: hosts without the capabilities must keep
// the baseline method set so the kernel's fail-closed type assertions still
// fire (ErrDimensionReaderRequired) instead of seeing an always-present stub.
type capabilityMasterDataReader struct {
	masterDataReaderAdapter
	effective   publicmasterdata.EffectiveAccountReader
	costCenters publicmasterdata.CostCenterReader
	dimensions  publicmasterdata.AnalyticalDimensionReader
}

func (a capabilityMasterDataReader) GetAccountAsOf(
	ctx context.Context,
	companyID types.CompanyID,
	accountID types.AccountID,
	asOf time.Time,
) (kernelmasterdata.Account, error) {
	if a.effective == nil {
		// Absence of the version reader is not an error: the kernel falls back
		// to the ordinary current account read and still checks IsCurrent.
		return a.masterDataReaderAdapter.GetAccount(ctx, companyID, accountID)
	}
	return a.effective.GetAccountAsOf(ctx, companyID, accountID, asOf)
}

func (a capabilityMasterDataReader) GetCostCenterForCompany(
	ctx context.Context,
	companyID types.CompanyID,
	costCenterID types.CostCenterID,
) (kernelmasterdata.CostCenter, error) {
	if a.costCenters == nil {
		return kernelmasterdata.CostCenter{}, posting.ErrDimensionReaderRequired
	}
	center, err := a.costCenters.GetCostCenterForCompany(ctx, companyID, costCenterID)
	if errors.Is(err, publicmasterdata.ErrInvalidCostCenter) {
		return kernelmasterdata.CostCenter{}, kernelmasterdata.ErrInvalidCostCenter
	}
	return center, err
}

func (a capabilityMasterDataReader) GetAnalyticalDimensionForCompany(
	ctx context.Context,
	companyID types.CompanyID,
	kind kernelmasterdata.AnalyticalDimensionKind,
	id string,
) (kernelmasterdata.AnalyticalDimensionValue, error) {
	if a.dimensions == nil {
		return kernelmasterdata.AnalyticalDimensionValue{}, posting.ErrDimensionReaderRequired
	}
	value, err := a.dimensions.GetAnalyticalDimensionForCompany(ctx, companyID, publicmasterdata.AnalyticalDimensionKind(kind), id)
	if errors.Is(err, publicmasterdata.ErrInvalidAnalyticalDimension) {
		return kernelmasterdata.AnalyticalDimensionValue{}, kernelmasterdata.ErrInvalidAnalyticalDimension
	}
	return value, err
}

type applicationService struct {
	ports          storage.Ports
	config         PostingConfig
	authorizer     publicapplication.Authorizer
	observer       publicapplication.Observer
	policyEngine   *policy.PolicyEngine
	pipeline       *posting.PostingPipeline
	policyStore    storage.PolicyStore
	policiesLoaded map[types.CompanyID]bool

	// Policy configuration is process-wide state. Locks are per company so
	// unrelated tenants do not block each other on first load.
	policyLocksMu        sync.Mutex
	policyLocks          map[types.CompanyID]*sync.Mutex
	policyLoadedAt       map[types.CompanyID]time.Time
	policyVersionDigests map[types.CompanyID]string
	syntheticPolicies    map[types.CompanyID]bool
}

func (s *applicationService) companyPolicyLock(companyID types.CompanyID) *sync.Mutex {
	s.policyLocksMu.Lock()
	defer s.policyLocksMu.Unlock()
	if s.policyLocks == nil {
		s.policyLocks = make(map[types.CompanyID]*sync.Mutex)
	}
	lock, ok := s.policyLocks[companyID]
	if !ok {
		lock = &sync.Mutex{}
		s.policyLocks[companyID] = lock
	}
	return lock
}

var _ publicapplication.Service = (*applicationService)(nil)
var _ publicapplication.TransactionalService = (*applicationService)(nil)
var _ publicapplication.IdempotentPostingLookup = (*applicationService)(nil)
var _ publicapplication.DeterministicDraftCreator = (*applicationService)(nil)
var _ publicapplication.DraftCancellationService = (*applicationService)(nil)
var _ publicapplication.YearEndClosingService = (*applicationService)(nil)

func newApplication(ports storage.Ports, config PostingConfig, authorizer publicapplication.Authorizer) *applicationService {
	policyEngine := policy.NewPolicyEngine()
	mdAdapter := masterDataReaderAdapter{
		reader:                       ports.MasterData,
		allowSyntheticDefaultJournal: config.AllowSyntheticDefaultJournal,
	}

	app := &applicationService{
		ports:                ports,
		config:               config,
		authorizer:           authorizer,
		observer:             publicapplication.NoopObserver{},
		policyEngine:         policyEngine,
		policyStore:          policyStoreFromPorts(ports),
		policiesLoaded:       make(map[types.CompanyID]bool),
		policyLocks:          make(map[types.CompanyID]*sync.Mutex),
		policyLoadedAt:       make(map[types.CompanyID]time.Time),
		policyVersionDigests: make(map[types.CompanyID]string),
		syntheticPolicies:    make(map[types.CompanyID]bool),
	}

	// Optional capabilities are detected, never assumed. A reader that exposes
	// none of them keeps the baseline method set and the kernel fails closed
	// for dimensioned lines; an effective-account capability is additive and
	// otherwise falls back to GetAccount.
	var reader kernelmasterdata.Reader = mdAdapter
	effective, hasEffective := ports.MasterData.(publicmasterdata.EffectiveAccountReader)
	costCenters, hasCostCenters := ports.MasterData.(publicmasterdata.CostCenterReader)
	dimensions, hasDimensions := ports.MasterData.(publicmasterdata.AnalyticalDimensionReader)
	if hasEffective || hasCostCenters || hasDimensions {
		reader = capabilityMasterDataReader{
			masterDataReaderAdapter: mdAdapter,
			effective:               effective,
			costCenters:             costCenters,
			dimensions:              dimensions,
		}
	}

	pipeline := posting.NewPostingPipeline(
		reader,
		app.executePosting,
		posting.WithPolicyEngine(policyEngine),
		posting.WithReversalExecutor(app.executeReversalPosting),
	)
	app.pipeline = pipeline

	return app
}

func (s *applicationService) executePosting(ctx context.Context, entry journal.JournalEntry, opts posting.PostOptions) (result journal.JournalEntry, err error) {
	started := time.Now()
	defer func() {
		s.observePosting(ctx, opts.CorrelationID, entry.CompanyID, false, started, err)
	}()

	seq, err := s.ports.Sequences.NextSequence(ctx, entry.CompanyID, entry.JournalID)
	if err != nil {
		return journal.JournalEntry{}, fmt.Errorf("allocate sequence: %w", err)
	}
	entry.SequenceNumber = seq
	if entry.EntryNumber == "" {
		entry.EntryNumber = fmt.Sprintf("%d", seq)
	}

	commit := storage.PostingCommit{
		Entry:         entry,
		Operation:     storage.OperationPost,
		ActorID:       opts.ActorID,
		Reason:        opts.Reason,
		CorrelationID: opts.CorrelationID,
		Idempotency:   commitIdempotency(opts, storage.OperationPost),
	}

	// The adapter writes the entry, audit evidence and idempotency completion
	// in one transaction. The SDK must not append audit records here: a failed
	// append would otherwise leave unreconciled evidence outside that unit.
	return s.ports.Posting.CommitPosting(ctx, commit)
}

func (s *applicationService) executeReversalPosting(
	ctx context.Context,
	originalEntryID types.JournalEntryID,
	entry journal.JournalEntry,
	opts posting.PostOptions,
) (result journal.JournalEntry, err error) {
	started := time.Now()
	defer func() {
		s.observePosting(ctx, opts.CorrelationID, entry.CompanyID, true, started, err)
	}()

	seq, err := s.ports.Sequences.NextSequence(ctx, entry.CompanyID, entry.JournalID)
	if err != nil {
		return journal.JournalEntry{}, fmt.Errorf("allocate reversal sequence: %w", err)
	}
	entry.SequenceNumber = seq
	if entry.EntryNumber == "" {
		entry.EntryNumber = fmt.Sprintf("%d", seq)
	}

	commit := storage.PostingCommit{
		Entry:           entry,
		Operation:       storage.OperationReverse,
		OriginalEntryID: &originalEntryID,
		ActorID:         opts.ActorID,
		Reason:          opts.Reason,
		CorrelationID:   opts.CorrelationID,
		Idempotency:     commitIdempotency(opts, storage.OperationReverse),
	}

	return s.ports.Posting.CommitPosting(ctx, commit)
}

// observePosting forwards the outcome of a posting or reversal to the optional
// Observer, carrying the request correlation id with the signal.
func (s *applicationService) observePosting(
	ctx context.Context,
	correlationID string,
	companyID types.CompanyID,
	reversal bool,
	started time.Time,
	err error,
) {
	observer := s.observer
	if observer == nil {
		return
	}
	observedCtx := publicapplication.WithObserverCorrelation(ctx, correlationID)
	duration := time.Since(started)
	if reversal {
		observer.ObserveReversal(observedCtx, companyID.String(), duration, err)
		return
	}
	observer.ObservePosting(observedCtx, companyID.String(), duration, err)
}

// commitIdempotency copies the kernel-stamped scope and stable payload digest
// onto the durable commit. The kernel is the only authority for both fields.
// A valid scope hint (post, reverse or revaluation) is preserved; legacy callers
// that did not stamp one still get the operation inferred from the executor.
func commitIdempotency(opts posting.PostOptions, operation storage.PostingOperation) *storage.PostingIdempotency {
	if opts.Scope.Key == "" || opts.CallerFingerprint == "" {
		return nil
	}
	scope := opts.Scope
	if !scope.Operation.Valid() {
		switch operation {
		case storage.OperationReverse:
			scope.Operation = posting.IdempotencyReversal
		case storage.OperationRevaluation:
			scope.Operation = posting.IdempotencyRevaluation
		default:
			scope.Operation = posting.IdempotencyPosting
		}
	}
	return &storage.PostingIdempotency{
		Key:         scope.Key,
		Scope:       scope.String(),
		PayloadHash: opts.CallerFingerprint,
	}
}

func (s *applicationService) ensurePolicies(ctx context.Context, companyID types.CompanyID, asOf time.Time) error {
	lock := s.companyPolicyLock(companyID)
	lock.Lock()
	defer lock.Unlock()

	// When the host supplies an authoritative policy history it is the single
	// source of truth: load every version once and fail closed for dates that
	// no stored version covers. No synthetic 2000-01-01 policy is created.
	if s.policyStore != nil {
		err := s.loadVersionedPolicies(ctx, companyID)
		if err != nil && !errors.Is(err, errNoVersionedPolicies) {
			return err
		}
		if err == nil {
			return s.ensureVersionedPoliciesIsEffective(companyID, asOf)
		}
		// The store holds no versions for this tenant: fall through to the
		// legacy derivation below. This keeps existing hosts working until
		// they explicitly register their first policy version.
	}

	// Legacy fallback for hosts without a policy store. Registration errors
	// must be propagated: silently ignoring an overlap or invalid payload
	// would let a posting proceed under an unrecorded policy decision.
	company, err := s.ports.MasterData.GetCompany(ctx, companyID)
	if err != nil {
		return err
	}

	registeredSynthetic := false
	if _, err := s.policyEngine.CurrencyPolicyFor(companyID, asOf); errors.Is(err, policy.ErrPolicyNotFound) {
		allowed := make([]string, 0, 2)
		for _, currency := range []string{company.FunctionalCurrency, company.PresentationCurrency} {
			currency = strings.ToUpper(strings.TrimSpace(currency))
			if currency == "" {
				continue
			}
			if !slices.Contains(allowed, currency) {
				allowed = append(allowed, currency)
			}
		}
		curPolicy := policy.CurrencyPolicy{
			CompanyID:                     companyID,
			FunctionalCurrency:            company.FunctionalCurrency,
			PresentationCurrency:          company.PresentationCurrency,
			RoundingMethod:                policy.RoundingBankers,
			AllowedCurrencies:             allowed,
			AutoSettleRoundingDifferences: false,
			EffectiveDate:                 time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
		}
		if err := s.policyEngine.SetCurrencyPolicy(curPolicy); err != nil {
			return fmt.Errorf("register fallback currency policy: %w", err)
		}
		registeredSynthetic = true
	}

	if _, err := s.policyEngine.PostingPolicyFor(companyID, asOf); errors.Is(err, policy.ErrPolicyNotFound) {
		postPolicy := policy.PostingPolicy{
			CompanyID:     companyID,
			EffectiveDate: time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
		}
		if err := s.policyEngine.SetPolicy(postPolicy); err != nil {
			return fmt.Errorf("register fallback posting policy: %w", err)
		}
		registeredSynthetic = true
	}

	if registeredSynthetic {
		if s.syntheticPolicies == nil {
			s.syntheticPolicies = make(map[types.CompanyID]bool)
		}
		s.syntheticPolicies[companyID] = true
	}
	return nil
}

func (s *applicationService) authorize(
	ctx context.Context,
	commandContext publicapplication.Context,
	companyID types.CompanyID,
	action publicapplication.Action,
	entryID *types.JournalEntryID,
	approvalReference string,
) error {
	principal := commandContext.Principal
	actorID := strings.TrimSpace(principal.ActorID)
	if actorID == "" {
		if s.authorizer != nil {
			return publicapplication.NewError(publicapplication.ErrUnauthorized, "verified actor identity is required", nil)
		}
	}

	if companyID == (types.CompanyID{}) {
		return publicapplication.NewError(publicapplication.ErrValidation, "company id is required", nil)
	}

	if actorID != "" && len(principal.CompanyIDs) > 0 {
		member := false
		for _, allowedCompanyID := range principal.CompanyIDs {
			if allowedCompanyID == companyID {
				member = true
				break
			}
		}
		if !member {
			return publicapplication.NewError(publicapplication.ErrForbidden, "principal is not authorized for this company", nil)
		}
	}

	if s.authorizer != nil {
		request := publicapplication.AuthorizationRequest{
			Principal:         principal,
			CompanyID:         companyID,
			Action:            action,
			EntryID:           entryID,
			ApprovalReference: approvalReference,
		}
		if err := s.authorizer.Authorize(ctx, request); err != nil {
			return publicapplication.NewError(publicapplication.ErrForbidden, "authorization denied", err)
		}
	}
	return nil
}

func (s *applicationService) CreateDraft(ctx context.Context, command publicapplication.CreateDraftCommand) (publicapplication.EntryResult, error) {
	return s.createDraftWithID(ctx, command, types.JournalEntryID{})
}

func (s *applicationService) CreateDraftWithID(
	ctx context.Context,
	command publicapplication.CreateDraftCommand,
	entryID types.JournalEntryID,
) (publicapplication.EntryResult, error) {
	return s.createDraftWithID(ctx, command, entryID)
}

func (s *applicationService) createDraftWithID(
	ctx context.Context,
	command publicapplication.CreateDraftCommand,
	entryID types.JournalEntryID,
) (publicapplication.EntryResult, error) {
	if err := s.authorize(ctx, command.Context, command.CompanyID, publicapplication.ActionCreate, nil, ""); err != nil {
		return publicapplication.EntryResult{}, err
	}

	if command.CompanyID == (types.CompanyID{}) {
		return publicapplication.EntryResult{}, publicapplication.NewError(publicapplication.ErrValidation, "company id is required", nil)
	}

	now := time.Now().UTC()
	if entryID == (types.JournalEntryID{}) {
		entryID = types.NewJournalEntryID()
	}

	effectiveDate := command.EffectiveDate
	if effectiveDate.IsZero() {
		effectiveDate = now
	}
	postingDate := command.PostingDate
	if postingDate.IsZero() {
		postingDate = effectiveDate
	}

	if command.JournalID == (types.JournalID{}) {
		// Look up default journal for the company (try GENERAL, then first available)
		j, err := s.ports.MasterData.GetJournalByCode(ctx, command.CompanyID, "GENERAL")
		if err == nil && j.ID != (types.JournalID{}) {
			command.JournalID = j.ID
		} else {
			for _, code := range []string{"GENERAL", "RETAIL", "AGRICULTURE", "SAAS", "DEFAULT"} {
				if cand, err := s.ports.MasterData.GetJournalByCode(ctx, command.CompanyID, code); err == nil && cand.ID != (types.JournalID{}) {
					command.JournalID = cand.ID
					break
				}
			}
		}
	}

	lines := make([]journal.EntryLine, len(command.Lines))
	for i, l := range command.Lines {
		if l.ID == (types.JournalEntryLineID{}) {
			l.ID = types.NewJournalEntryLineID()
		}
		lines[i] = l
	}

	classification := command.Classification
	if classification == "" {
		classification = journal.EntryClassRegular
	}

	entry := journal.JournalEntry{
		ID:                  entryID,
		CompanyID:           command.CompanyID,
		JournalID:           command.JournalID,
		FiscalPeriodID:      command.FiscalPeriodID,
		EffectiveDate:       effectiveDate,
		PostingDate:         postingDate,
		Description:         command.Description,
		SourceModule:        command.SourceModule,
		ReferenceID:         command.ReferenceID,
		TransactionCurrency: command.TransactionCurrency,
		Classification:      classification,
		TargetPeriodNumber:  command.TargetPeriodNumber,
		AdjustingEntryType:  command.AdjustingEntryType,
		AdjustmentReason:    command.AdjustmentReason,
		Lines:               lines,
		Status:              journal.EntryDraft,
		CreatedBy:           strings.TrimSpace(command.Context.Principal.ActorID),
		CreatedAt:           now,
		UpdatedAt:           now,
	}

	if command.ExchangeRate != "" {
		rate, err := decimal.NewFromString(command.ExchangeRate)
		if err != nil {
			return publicapplication.EntryResult{}, publicapplication.NewError(publicapplication.ErrValidation, "invalid exchange rate", err)
		}
		entry.ExchangeRate = rate
	}

	if err := s.ports.Drafts.SaveDraft(ctx, entry); err != nil {
		return publicapplication.EntryResult{}, classifyError(err)
	}

	return publicapplication.EntryResult{Entry: entry}, nil
}

func (s *applicationService) Validate(ctx context.Context, command publicapplication.ValidateEntryCommand) error {
	entryID := command.EntryID
	if err := s.authorize(ctx, command.Context, command.CompanyID, publicapplication.ActionValidate, &entryID, ""); err != nil {
		return err
	}

	entry, err := s.ports.Entries.GetEntry(ctx, command.CompanyID, command.EntryID)
	if err != nil {
		return classifyError(err)
	}

	if entry.Status != journal.EntryDraft {
		return publicapplication.NewError(publicapplication.ErrValidation, "entry is not in draft status", nil)
	}

	if err := s.ensurePolicies(ctx, entry.CompanyID, entry.EffectiveDate); err != nil {
		return classifyError(err)
	}

	if err := posting.ValidateEntryDoubleEntry(&entry, nil); err != nil {
		return classifyError(err)
	}

	return nil
}

// checkIdempotencyBeforePost is a side-effect-free lookup. It never claims a
// key: the durable claim and its release are owned by the storage adapter's
// atomic CommitPosting. It exists only to avoid re-running kernel policy on a
// replay and to surface payload conflicts explicitly.
func (s *applicationService) checkIdempotencyBeforePost(
	ctx context.Context,
	entry journal.JournalEntry,
	opts posting.PostOptions,
) (journal.JournalEntry, bool, error) {
	key := strings.TrimSpace(opts.IdempotencyKey)
	if key == "" || s.ports.Idempotency == nil {
		return journal.JournalEntry{}, false, nil
	}
	lookup, ok := s.ports.Idempotency.(storage.IdempotencyLookup)
	if !ok {
		return journal.JournalEntry{}, false, nil
	}
	scope, err := posting.BuildIdempotencyScope(entry, opts)
	if err != nil {
		return journal.JournalEntry{}, false, err
	}
	fingerprint, err := posting.FingerprintCallerPayload(entry, opts, scope)
	if err != nil {
		return journal.JournalEntry{}, false, err
	}
	record, found, err := lookup.Lookup(ctx, entry.CompanyID, key)
	if err != nil {
		return journal.JournalEntry{}, false, fmt.Errorf("lookup idempotency key: %w", err)
	}
	if !found {
		return journal.JournalEntry{}, false, nil
	}
	if record.PayloadHash != "" && record.PayloadHash != fingerprint {
		return journal.JournalEntry{}, false, publicapplication.NewError(
			publicapplication.ErrConflict, "idempotency key was reused with a different payload", ErrIdempotencyKeyConflict)
	}
	switch record.Status {
	case storage.IdempotencyCompleted:
		if record.EntryID == nil {
			return journal.JournalEntry{}, false, publicapplication.NewError(
				publicapplication.ErrConflict, "completed idempotency record has no entry", ErrIdempotencyInProgress)
		}
		posted, getErr := s.ports.Entries.GetEntry(ctx, entry.CompanyID, *record.EntryID)
		if getErr != nil {
			return journal.JournalEntry{}, false, publicapplication.NewError(
				publicapplication.ErrInternal, "idempotent result is unavailable", getErr)
		}
		return posted, true, nil
	case storage.IdempotencyStarted:
		return journal.JournalEntry{}, false, publicapplication.NewError(
			publicapplication.ErrConflict, "idempotency key is still processing", ErrIdempotencyInProgress)
	case storage.IdempotencyTerminal:
		return journal.JournalEntry{}, false, publicapplication.NewError(
			publicapplication.ErrConflict, "idempotency key previously failed", ErrIdempotencyTerminalFailure)
	default:
		// A retryable failure may safely be retried; the adapter will decide
		// whether the durable key is still owned by a live execution.
		return journal.JournalEntry{}, false, nil
	}
}

func (s *applicationService) Post(ctx context.Context, command publicapplication.PostEntryCommand) (publicapplication.EntryResult, error) {
	entryID := command.EntryID
	if err := s.authorize(ctx, command.Context, command.CompanyID, publicapplication.ActionPost, &entryID, ""); err != nil {
		return publicapplication.EntryResult{}, err
	}

	actorID := strings.TrimSpace(command.Context.Principal.ActorID)
	if actorID == "" {
		actorID = "system"
	}
	key := strings.TrimSpace(command.Context.IdempotencyKey)
	opts := posting.PostOptions{
		ActorID:        actorID,
		CorrelationID:  command.Context.CorrelationID,
		IdempotencyKey: key,
	}

	entry, err := s.ports.Entries.GetEntry(ctx, command.CompanyID, command.EntryID)
	if err != nil {
		return publicapplication.EntryResult{}, classifyError(err)
	}
	if replay, found, err := s.checkIdempotencyBeforePost(ctx, entry, opts); err != nil {
		return publicapplication.EntryResult{}, classifyError(err)
	} else if found {
		return publicapplication.EntryResult{Entry: replay, IdempotentReplay: true}, nil
	}

	if err := s.ensurePolicies(ctx, entry.CompanyID, entry.EffectiveDate); err != nil {
		return publicapplication.EntryResult{}, classifyError(err)
	}

	posted, err := s.pipeline.PostEntry(ctx, entry, opts)
	if err != nil {
		return publicapplication.EntryResult{}, classifyError(err)
	}

	return publicapplication.EntryResult{Entry: posted, IdempotentReplay: false}, nil
}

func (s *applicationService) Reverse(ctx context.Context, command publicapplication.ReverseEntryCommand) (publicapplication.EntryResult, error) {
	entryID := command.EntryID
	if err := s.authorize(ctx, command.Context, command.CompanyID, publicapplication.ActionReverse, &entryID, ""); err != nil {
		return publicapplication.EntryResult{}, err
	}

	actorID := strings.TrimSpace(command.Context.Principal.ActorID)
	if actorID == "" {
		actorID = "system"
	}

	original, err := s.ports.Entries.GetEntry(ctx, command.CompanyID, command.EntryID)
	if err != nil {
		return publicapplication.EntryResult{}, classifyError(err)
	}

	effectiveDate := command.EffectiveDate
	if effectiveDate.IsZero() {
		effectiveDate = time.Now().UTC()
	}

	reversal, err := posting.BuildReversalEntry(original, effectiveDate, actorID, command.Reason)
	if err != nil {
		return publicapplication.EntryResult{}, classifyError(err)
	}
	opts := posting.PostOptions{
		ActorID:        actorID,
		Reason:         command.Reason,
		CorrelationID:  command.Context.CorrelationID,
		IdempotencyKey: strings.TrimSpace(command.IdempotencyKey),
	}
	if replay, found, err := s.checkIdempotencyBeforePost(ctx, reversal, opts); err != nil {
		return publicapplication.EntryResult{}, classifyError(err)
	} else if found {
		return publicapplication.EntryResult{Entry: replay, IdempotentReplay: true}, nil
	}

	if err := s.ensurePolicies(ctx, reversal.CompanyID, reversal.EffectiveDate); err != nil {
		return publicapplication.EntryResult{}, classifyError(err)
	}

	posted, err := s.pipeline.PostEntry(ctx, reversal, opts)
	if err != nil {
		return publicapplication.EntryResult{}, classifyError(err)
	}

	return publicapplication.EntryResult{Entry: posted, IdempotentReplay: false}, nil
}

func (s *applicationService) CancelDraft(ctx context.Context, command publicapplication.CancelDraftCommand) (publicapplication.EntryResult, error) {
	entryID := command.EntryID
	if err := s.authorize(ctx, command.Context, command.CompanyID, publicapplication.ActionCancel, &entryID, ""); err != nil {
		return publicapplication.EntryResult{}, err
	}

	actorID := strings.TrimSpace(command.Context.Principal.ActorID)
	if actorID == "" {
		return publicapplication.EntryResult{}, publicapplication.NewError(
			publicapplication.ErrValidation, "actor id is required to cancel a draft", journal.ErrInvalidCancellation)
	}
	reason := strings.TrimSpace(command.Reason)
	if reason == "" {
		return publicapplication.EntryResult{}, publicapplication.NewError(
			publicapplication.ErrValidation, "cancellation reason is required", journal.ErrInvalidCancellation)
	}
	canceller, ok := s.ports.Drafts.(storage.DraftCanceller)
	if !ok {
		return publicapplication.EntryResult{}, ErrCapabilityNotImplemented
	}

	entry, err := s.ports.Entries.GetEntry(ctx, command.CompanyID, command.EntryID)
	if err != nil {
		return publicapplication.EntryResult{}, classifyError(err)
	}

	cancelledAt := command.CancelledAt
	if cancelledAt.IsZero() {
		cancelledAt = time.Now().UTC()
	}
	if err := entry.Cancel(actorID, reason, cancelledAt); err != nil {
		return publicapplication.EntryResult{}, classifyError(err)
	}

	if err := canceller.CancelDraft(ctx, entry); err != nil {
		return publicapplication.EntryResult{}, classifyError(err)
	}

	if s.ports.Audit != nil {
		_ = s.ports.Audit.AppendAudit(ctx, storage.AuditRecord{
			CompanyID:     entry.CompanyID,
			EntityType:    "journal_entry",
			EntityID:      entry.ID.String(),
			Action:        string(publicapplication.ActionCancel),
			ActorID:       actorID,
			Reason:        reason,
			CorrelationID: command.Context.CorrelationID,
			OccurredAt:    cancelledAt.UTC(),
		})
	}

	return publicapplication.EntryResult{Entry: entry, IdempotentReplay: false}, nil
}

func (s *applicationService) GetEntry(ctx context.Context, query publicapplication.GetEntryQuery) (journal.JournalEntry, error) {
	if err := s.authorize(ctx, query.Context, query.CompanyID, publicapplication.ActionRead, &query.EntryID, ""); err != nil {
		return journal.JournalEntry{}, err
	}

	entry, err := s.ports.Entries.GetEntry(ctx, query.CompanyID, query.EntryID)
	if err != nil {
		return journal.JournalEntry{}, classifyError(err)
	}
	return entry, nil
}

func (s *applicationService) WithinTransaction(ctx context.Context, companyID types.CompanyID, fn func(ctx context.Context) error) error {
	if s.ports.TxRunner == nil {
		return ErrCapabilityNotImplemented
	}
	if companyID == (types.CompanyID{}) {
		return errors.New("company id is required")
	}
	return s.ports.TxRunner.WithinTransaction(storage.WithCompanyContext(ctx, companyID), fn)
}

func (s *applicationService) FindPostedEntryByIdempotencyKey(ctx context.Context, companyID types.CompanyID, key string) (journal.JournalEntry, bool, error) {
	lookup, ok := s.ports.Idempotency.(storage.IdempotencyLookup)
	if !ok {
		return journal.JournalEntry{}, false, ErrCapabilityNotImplemented
	}
	record, found, err := lookup.Lookup(ctx, companyID, strings.TrimSpace(key))
	if err != nil {
		return journal.JournalEntry{}, false, err
	}
	if !found || record.Status != storage.IdempotencyCompleted || record.EntryID == nil {
		return journal.JournalEntry{}, false, nil
	}
	entry, err := s.ports.Entries.GetEntry(ctx, companyID, *record.EntryID)
	if err != nil {
		return journal.JournalEntry{}, false, err
	}
	return entry, true, nil
}

func classifyError(err error) error {
	if err == nil {
		return nil
	}
	var pubErr interface{ PublicMessage() string }
	if errors.As(err, &pubErr) {
		return err
	}
	if errors.Is(err, kernelmasterdata.ErrCompanyNotFound) ||
		errors.Is(err, kernelmasterdata.ErrJournalNotFound) ||
		errors.Is(err, kernelmasterdata.ErrAccountNotFound) ||
		errors.Is(err, kernelmasterdata.ErrFiscalPeriodNotFound) ||
		errors.Is(err, valuation.ErrRateNotFound) ||
		errors.Is(err, storage.ErrEntryNotFound) ||
		errors.Is(err, storage.ErrPolicyVersionNotFound) ||
		errors.Is(err, policy.ErrPolicyNotFound) ||
		errors.Is(err, policy.ErrPolicyNotEffective) ||
		errors.Is(err, publicmasterdata.ErrCompanyNotFound) ||
		errors.Is(err, publicmasterdata.ErrAccountNotFound) ||
		errors.Is(err, publicmasterdata.ErrJournalNotFound) ||
		errors.Is(err, publicmasterdata.ErrFiscalYearNotFound) ||
		errors.Is(err, publicmasterdata.ErrPeriodNotFound) {
		return publicapplication.NewError(publicapplication.ErrNotFound, err.Error(), err)
	}
	if errors.Is(err, kernelmasterdata.ErrFiscalPeriodClosed) ||
		errors.Is(err, kernelmasterdata.ErrClosedFiscalYear) ||
		errors.Is(err, kernelmasterdata.ErrClosedPeriod) {
		return publicapplication.NewError(publicapplication.ErrPeriodClosed, err.Error(), err)
	}
	if errors.Is(err, posting.ErrUnbalancedEntry) ||
		errors.Is(err, posting.ErrMinimumLines) ||
		errors.Is(err, posting.ErrInvalidEntry) ||
		errors.Is(err, posting.ErrLineMustHaveOneSide) ||
		errors.Is(err, posting.ErrMissingJournalID) ||
		errors.Is(err, posting.ErrMissingEntryID) ||
		errors.Is(err, posting.ErrMissingLineID) ||
		errors.Is(err, posting.ErrDuplicateLineID) ||
		errors.Is(err, posting.ErrZeroEntryDate) ||
		errors.Is(err, posting.ErrCompanyMismatch) ||
		errors.Is(err, posting.ErrMissingTransactionCurrency) ||
		errors.Is(err, posting.ErrMissingFunctionalCurrency) ||
		errors.Is(err, posting.ErrInvalidExchangeRate) ||
		errors.Is(err, posting.ErrFunctionalBalanceMismatch) ||
		errors.Is(err, posting.ErrInvalidAdjustingEntryMetadata) ||
		errors.Is(err, valuation.ErrInvalidAdjustingEntryMetadata) ||
		errors.Is(err, posting.ErrAdjustingEntryMetadataRequired) ||
		errors.Is(err, posting.ErrInvalidAdjustingEntryPeriod) ||
		errors.Is(err, posting.ErrInvalidReversal) ||
		errors.Is(err, posting.ErrReversalDateBeforeOriginal) ||
		errors.Is(err, posting.ErrReversalActorRequired) ||
		errors.Is(err, posting.ErrReversalReasonRequired) ||
		errors.Is(err, posting.ErrRoundingAccountRequired) ||
		errors.Is(err, posting.ErrPosterIDRequired) ||
		errors.Is(err, posting.ErrInvalidEntryState) ||
		errors.Is(err, posting.ErrSourceTraceabilityRequired) ||
		errors.Is(err, posting.ErrSourceReferenceRequired) ||
		errors.Is(err, posting.ErrInactiveFinancialDimension) ||
		errors.Is(err, posting.ErrRoundingPolicyRequired) ||
		errors.Is(err, posting.ErrDimensionReaderRequired) ||
		errors.Is(err, posting.ErrInvalidDimensionReference) ||
		errors.Is(err, types.ErrInvalidAmount) ||
		errors.Is(err, types.ErrAmountExceedsPrecision) ||
		errors.Is(err, types.ErrSubMinorUnitAmount) ||
		errors.Is(err, types.ErrNegativeAmountProhibited) ||
		errors.Is(err, posting.ErrZeroAmountLine) ||
		errors.Is(err, posting.ErrUnbalancedMove) ||
		errors.Is(err, types.ErrCurrencyMismatch) ||
		errors.Is(err, types.ErrInvalidCurrency) ||
		errors.Is(err, kernelmasterdata.ErrCannotPostToParentAccount) ||
		errors.Is(err, types.ErrZeroDenominator) ||
		errors.Is(err, kernelmasterdata.ErrHeaderAccountPosting) ||
		errors.Is(err, kernelmasterdata.ErrAccountCycle) ||
		errors.Is(err, kernelmasterdata.ErrDuplicateAccountID) ||
		errors.Is(err, kernelmasterdata.ErrInvalidParent) ||
		errors.Is(err, kernelmasterdata.ErrAccountNotCurrent) ||
		errors.Is(err, kernelmasterdata.ErrAccountIdentityMismatch) ||
		errors.Is(err, kernelmasterdata.ErrInvalidCostCenter) ||
		errors.Is(err, kernelmasterdata.ErrCostCenterCycle) ||
		errors.Is(err, kernelmasterdata.ErrInvalidAnalyticalDimension) ||
		errors.Is(err, kernelmasterdata.ErrInvalidJournal) ||
		errors.Is(err, kernelmasterdata.ErrInvalidJournalType) ||
		errors.Is(err, kernelmasterdata.ErrJournalCurrency) ||
		errors.Is(err, kernelmasterdata.ErrJournalAccount) ||
		errors.Is(err, kernelmasterdata.ErrJournalNotCurrent) ||
		errors.Is(err, kernelmasterdata.ErrJournalReaderRequired) ||
		errors.Is(err, kernelmasterdata.ErrJournalIdentityMismatch) ||
		errors.Is(err, kernelmasterdata.ErrInvalidEffectiveRange) ||
		errors.Is(err, kernelmasterdata.ErrInvalidAdjustmentPeriod) ||
		errors.Is(err, kernelmasterdata.ErrInvalidFiscalPeriod) ||
		errors.Is(err, kernelmasterdata.ErrPeriodOutsideFiscalYear) ||
		errors.Is(err, kernelmasterdata.ErrInvalidPeriodTransition) ||
		errors.Is(err, kernelmasterdata.ErrInvalidFiscalYearTransition) ||
		errors.Is(err, kernelmasterdata.ErrPeriodTransitionNeedsAudit) ||
		errors.Is(err, kernelmasterdata.ErrPeriodReopenRequiresAudit) ||
		errors.Is(err, kernelmasterdata.ErrAccountCodeRequired) ||
		errors.Is(err, kernelmasterdata.ErrAccountNameRequired) ||
		errors.Is(err, kernelmasterdata.ErrInvalidAccountType) ||
		errors.Is(err, kernelmasterdata.ErrInvalidNormalBalance) ||
		errors.Is(err, kernelmasterdata.ErrInvalidSubClassificationForType) ||
		errors.Is(err, kernelmasterdata.ErrInvalidControlAccount) ||
		errors.Is(err, policy.ErrInvalidPolicy) ||
		errors.Is(err, valuation.ErrInvalidRate) ||
		errors.Is(err, valuation.ErrInvalidCurrency) ||
		errors.Is(err, valuation.ErrInvalidPosition) ||
		errors.Is(err, valuation.ErrZeroRevaluationDifference) ||
		errors.Is(err, journal.ErrInvalidEntryStatus) ||
		errors.Is(err, journal.ErrInvalidStatusTransition) ||
		errors.Is(err, journal.ErrInvalidReversalLinkage) ||
		errors.Is(err, journal.ErrInvalidLineMutation) ||
		errors.Is(err, journal.ErrInvalidCancellation) ||
		errors.Is(err, governance.ErrInvalidAuditCompany) ||
		errors.Is(err, governance.ErrInvalidAuditCheckpoint) ||
		errors.Is(err, governance.ErrInvalidAuditSnapshot) {
		return publicapplication.NewError(publicapplication.ErrValidation, err.Error(), err)
	}
	if errors.Is(err, storage.ErrIdempotencyPayloadConflict) ||
		errors.Is(err, storage.ErrIdempotencyInProgress) ||
		errors.Is(err, storage.ErrEntryAlreadyReversed) ||
		errors.Is(err, storage.ErrConcurrentModification) ||
		errors.Is(err, storage.ErrPolicyVersionConflict) ||
		errors.Is(err, policy.ErrPolicyOverlap) ||
		errors.Is(err, valuation.ErrRevaluationAlreadyApplied) ||
		errors.Is(err, posting.ErrIdempotencyConflict) ||
		errors.Is(err, posting.ErrIdempotencyInProgress) ||
		errors.Is(err, posting.ErrConcurrencyConflict) ||
		errors.Is(err, posting.ErrEntryAlreadyReversed) ||
		errors.Is(err, posting.ErrReversalAlreadyApplied) ||
		errors.Is(err, posting.ErrEntryNotDraft) ||
		errors.Is(err, posting.ErrEntryNotPosted) ||
		errors.Is(err, journal.ErrMoveAlreadyPosted) ||
		errors.Is(err, journal.ErrCannotModifyPostedMove) ||
		errors.Is(err, kernelmasterdata.ErrDuplicateAccountCode) ||
		errors.Is(err, journal.ErrImmutableEntry) ||
		errors.Is(err, valuation.ErrDuplicateRate) ||
		errors.Is(err, publicmasterdata.ErrDuplicateCompanyCode) ||
		errors.Is(err, publicmasterdata.ErrDuplicateAccountCode) ||
		errors.Is(err, publicmasterdata.ErrDuplicateJournalCode) ||
		errors.Is(err, publicmasterdata.ErrDuplicateJournalID) {
		return publicapplication.NewError(publicapplication.ErrConflict, err.Error(), err)
	}
	if errors.Is(err, storage.ErrIdempotencyUnavailable) ||
		errors.Is(err, posting.ErrIdempotencyStorageUnavailable) {
		return publicapplication.NewError(publicapplication.ErrDependencyUnavailable, err.Error(), err)
	}
	if errors.Is(err, storage.ErrAtomicCommitFailed) ||
		errors.Is(err, posting.ErrAtomicityViolation) {
		return publicapplication.NewError(publicapplication.ErrInternal, err.Error(), err)
	}
	// A company-scope violation is never a conflict to retry: the caller asked
	// to act outside its own tenant boundary. Phase 8 classifies it as forbidden.
	if errors.Is(err, storage.ErrScopeViolation) ||
		errors.Is(err, kernelmasterdata.ErrCompanyScopeRequired) ||
		errors.Is(err, kernelmasterdata.ErrCompanyIdentityMismatch) ||
		errors.Is(err, kernelmasterdata.ErrCompanyInactive) ||
		errors.Is(err, publictypes.ErrMakerCheckerViolation) {
		return publicapplication.NewError(publicapplication.ErrForbidden, err.Error(), err)
	}
	var coded *apperrors.CodedError
	if errors.As(err, &coded) {
		switch apperrors.CategoryOf(err) {
		case apperrors.CategoryConcurrency:
			return publicapplication.NewError(publicapplication.ErrConflict, err.Error(), err)
		case apperrors.CategoryPersistence, apperrors.CategoryInfrastructure:
			return publicapplication.NewError(publicapplication.ErrDependencyUnavailable, err.Error(), err)
		case apperrors.CategoryValidation, apperrors.CategoryDomain:
			return publicapplication.NewError(publicapplication.ErrValidation, err.Error(), err)
		}
	}
	return err
}
