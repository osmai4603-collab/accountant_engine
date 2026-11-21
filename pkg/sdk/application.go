package sdk

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/osmai4603-collab/accounting_engine/internal/kernal/journal"
	kernalmasterdata "github.com/osmai4603-collab/accounting_engine/internal/kernal/masterdata"
	"github.com/osmai4603-collab/accounting_engine/internal/kernal/policy"
	"github.com/osmai4603-collab/accounting_engine/internal/kernal/posting"
	"github.com/osmai4603-collab/accounting_engine/internal/kernal/types"
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

type masterDataReaderAdapter struct {
	reader storage.MasterDataReader
}

func (a masterDataReaderAdapter) GetCompany(ctx context.Context, companyID types.CompanyID) (kernalmasterdata.Company, error) {
	c, err := a.reader.GetCompany(ctx, companyID)
	if err != nil {
		return kernalmasterdata.Company{}, err
	}
	return kernalmasterdata.Company{
		ID:                   c.ID,
		Code:                 c.Code,
		LegalName:            c.LegalName,
		FunctionalCurrency:   c.FunctionalCurrency,
		PresentationCurrency: c.PresentationCurrency,
		Active:               c.Active,
	}, nil
}

func (a masterDataReaderAdapter) GetAccount(ctx context.Context, companyID types.CompanyID, accountID types.AccountID) (kernalmasterdata.Account, error) {
	return a.reader.GetAccount(ctx, companyID, accountID)
}

func (a masterDataReaderAdapter) GetAccountByCode(ctx context.Context, companyID types.CompanyID, code string) (kernalmasterdata.Account, error) {
	return a.reader.GetAccountByCode(ctx, companyID, code)
}

func (a masterDataReaderAdapter) GetJournalByCode(ctx context.Context, companyID types.CompanyID, code string) (kernalmasterdata.Journal, error) {
	j, err := a.reader.GetJournalByCode(ctx, companyID, code)
	if err != nil {
		return kernalmasterdata.Journal{}, err
	}
	return toKernalJournal(j), nil
}

func (a masterDataReaderAdapter) HasAccountChildren(ctx context.Context, companyID types.CompanyID, accountID types.AccountID) (bool, error) {
	return a.reader.HasAccountChildren(ctx, companyID, accountID)
}

func (a masterDataReaderAdapter) GetFiscalYear(ctx context.Context, companyID types.CompanyID, fiscalYearID types.FiscalYearID) (kernalmasterdata.FiscalYear, error) {
	return a.reader.GetFiscalYear(ctx, companyID, fiscalYearID)
}

func (a masterDataReaderAdapter) GetPeriodForDate(ctx context.Context, companyID types.CompanyID, date time.Time) (kernalmasterdata.FiscalPeriod, error) {
	return a.reader.GetPeriodForDate(ctx, companyID, date)
}

func (a masterDataReaderAdapter) GetJournalByID(ctx context.Context, companyID types.CompanyID, journalID types.JournalID) (kernalmasterdata.Journal, error) {
	if repo, ok := a.reader.(interface {
		GetJournalByID(ctx context.Context, companyID types.CompanyID, journalID types.JournalID) (kernalmasterdata.Journal, error)
	}); ok {
		return repo.GetJournalByID(ctx, companyID, journalID)
	}
	if repo, ok := a.reader.(interface {
		GetJournalByID(ctx context.Context, companyID types.CompanyID, journalID types.JournalID) (publicmasterdata.Journal, error)
	}); ok {
		j, err := repo.GetJournalByID(ctx, companyID, journalID)
		if err != nil {
			return kernalmasterdata.Journal{}, err
		}
		return toKernalJournal(j), nil
	}
	j, err := a.reader.GetJournalByCode(ctx, companyID, "GENERAL")
	if err == nil && j.ID == journalID {
		return toKernalJournal(j), nil
	}
	return kernalmasterdata.Journal{
		ID:            journalID,
		CompanyID:     companyID,
		Code:          "GENERAL",
		Name:          "General Journal",
		EffectiveFrom: time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
	}, nil
}

func (a masterDataReaderAdapter) GetPeriodByNumber(ctx context.Context, companyID types.CompanyID, fiscalYearID types.FiscalYearID, number int) (kernalmasterdata.FiscalPeriod, error) {
	if repo, ok := a.reader.(interface {
		GetPeriodByNumber(ctx context.Context, companyID types.CompanyID, fiscalYearID types.FiscalYearID, number int) (kernalmasterdata.FiscalPeriod, error)
	}); ok {
		return repo.GetPeriodByNumber(ctx, companyID, fiscalYearID, number)
	}
	return kernalmasterdata.FiscalPeriod{}, errors.New("adjustment periods not supported")
}

func toKernalJournal(j publicmasterdata.Journal) kernalmasterdata.Journal {
	return kernalmasterdata.Journal{
		ID:                j.ID,
		CompanyID:         j.CompanyID,
		Code:              j.Code,
		Name:              j.Name,
		AllowedCurrencies: j.AllowedCurrencies,
		AllowedAccountIDs: j.AllowedAccountIDs,
		EffectiveFrom:     time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
	}
}

type applicationService struct {
	ports        storage.Ports
	config       PostingConfig
	authorizer   publicapplication.Authorizer
	policyEngine *policy.PolicyEngine
	pipeline     *posting.PostingPipeline
}

var _ publicapplication.Service = (*applicationService)(nil)
var _ publicapplication.TransactionalService = (*applicationService)(nil)
var _ publicapplication.IdempotentPostingLookup = (*applicationService)(nil)
var _ publicapplication.DeterministicDraftCreator = (*applicationService)(nil)

func newApplication(ports storage.Ports, config PostingConfig, authorizer publicapplication.Authorizer) *applicationService {
	policyEngine := policy.NewPolicyEngine()
	mdAdapter := masterDataReaderAdapter{reader: ports.MasterData}

	app := &applicationService{
		ports:        ports,
		config:       config,
		authorizer:   authorizer,
		policyEngine: policyEngine,
	}

	pipeline := posting.NewPostingPipeline(
		mdAdapter,
		app.executePosting,
		posting.WithPolicyEngine(policyEngine),
		posting.WithReversalExecutor(app.executeReversalPosting),
	)
	app.pipeline = pipeline

	return app
}

func (s *applicationService) executePosting(ctx context.Context, entry journal.JournalEntry, opts posting.PostOptions) (journal.JournalEntry, error) {
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
	}

	committed, err := s.ports.Posting.CommitPosting(ctx, commit)
	if err != nil {
		return journal.JournalEntry{}, err
	}

	if s.ports.Audit != nil {
		_ = s.ports.Audit.AppendAudit(ctx, storage.AuditRecord{
			CompanyID:     entry.CompanyID,
			EntityType:    "journal_entry",
			EntityID:      entry.ID.String(),
			Action:        "post",
			ActorID:       opts.ActorID,
			Reason:        opts.Reason,
			CorrelationID: opts.CorrelationID,
			OccurredAt:    time.Now().UTC(),
		})
	}

	return committed, nil
}

func (s *applicationService) executeReversalPosting(
	ctx context.Context,
	originalEntryID types.JournalEntryID,
	entry journal.JournalEntry,
	opts posting.PostOptions,
) (journal.JournalEntry, error) {
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
	}

	committed, err := s.ports.Posting.CommitPosting(ctx, commit)
	if err != nil {
		return journal.JournalEntry{}, err
	}

	if s.ports.Audit != nil {
		_ = s.ports.Audit.AppendAudit(ctx, storage.AuditRecord{
			CompanyID:     entry.CompanyID,
			EntityType:    "journal_entry",
			EntityID:      entry.ID.String(),
			Action:        "reverse",
			ActorID:       opts.ActorID,
			Reason:        opts.Reason,
			CorrelationID: opts.CorrelationID,
			OccurredAt:    time.Now().UTC(),
		})
	}

	return committed, nil
}

func (s *applicationService) ensurePolicies(ctx context.Context, companyID types.CompanyID, asOf time.Time) error {
	company, err := s.ports.MasterData.GetCompany(ctx, companyID)
	if err != nil {
		return err
	}

	if _, err := s.policyEngine.CurrencyPolicyFor(companyID, asOf); errors.Is(err, policy.ErrPolicyNotFound) {
		curPolicy := policy.CurrencyPolicy{
			CompanyID:                     companyID,
			FunctionalCurrency:            company.FunctionalCurrency,
			PresentationCurrency:          company.PresentationCurrency,
			RoundingMethod:                policy.RoundingBankers,
			AllowedCurrencies:             []string{company.FunctionalCurrency, company.PresentationCurrency},
			AutoSettleRoundingDifferences: false,
			EffectiveDate:                 time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
		}
		_ = s.policyEngine.SetCurrencyPolicy(curPolicy)
	}

	if _, err := s.policyEngine.PostingPolicyFor(companyID, asOf); errors.Is(err, policy.ErrPolicyNotFound) {
		postPolicy := policy.PostingPolicy{
			CompanyID:     companyID,
			EffectiveDate: time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
		}
		_ = s.policyEngine.SetPolicy(postPolicy)
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
	if key != "" && s.ports.Idempotency != nil {
		existing, claimed, err := s.ports.Idempotency.Claim(ctx, storage.IdempotencyRecord{
			CompanyID: command.CompanyID,
			Key:       key,
			Operation: "post",
			CreatedAt: time.Now().UTC(),
		})
		if err != nil {
			return publicapplication.EntryResult{}, classifyError(err)
		}
		if !claimed {
			if existing.Status == storage.IdempotencyCompleted && existing.EntryID != nil {
				posted, err := s.ports.Entries.GetEntry(ctx, command.CompanyID, *existing.EntryID)
				if err == nil {
					return publicapplication.EntryResult{Entry: posted, IdempotentReplay: true}, nil
				}
			}
			return publicapplication.EntryResult{}, ErrIdempotencyInProgress
		}
	}

	entry, err := s.ports.Entries.GetEntry(ctx, command.CompanyID, command.EntryID)
	if err != nil {
		return publicapplication.EntryResult{}, classifyError(err)
	}

	if err := s.ensurePolicies(ctx, entry.CompanyID, entry.EffectiveDate); err != nil {
		return publicapplication.EntryResult{}, classifyError(err)
	}

	posted, err := s.pipeline.PostEntry(ctx, entry, posting.PostOptions{
		ActorID:        actorID,
		CorrelationID:  command.Context.CorrelationID,
		IdempotencyKey: key,
	})
	if err != nil {
		if key != "" && s.ports.Idempotency != nil {
			_ = s.ports.Idempotency.Fail(ctx, command.CompanyID, key, "", storage.IdempotencyTerminal)
		}
		return publicapplication.EntryResult{}, classifyError(err)
	}

	if key != "" && s.ports.Idempotency != nil {
		_ = s.ports.Idempotency.Complete(ctx, command.CompanyID, key, "", posted.ID, nil)
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

	if err := s.ensurePolicies(ctx, reversal.CompanyID, reversal.EffectiveDate); err != nil {
		return publicapplication.EntryResult{}, classifyError(err)
	}

	posted, err := s.pipeline.PostEntry(ctx, reversal, posting.PostOptions{
		ActorID:        actorID,
		Reason:         command.Reason,
		CorrelationID:  command.Context.CorrelationID,
		IdempotencyKey: command.IdempotencyKey,
	})
	if err != nil {
		return publicapplication.EntryResult{}, classifyError(err)
	}

	return publicapplication.EntryResult{Entry: posted}, nil
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
	if s.ports.Idempotency == nil {
		return journal.JournalEntry{}, false, ErrCapabilityNotImplemented
	}
	return journal.JournalEntry{}, false, nil
}

func classifyError(err error) error {
	if err == nil {
		return nil
	}
	var pubErr interface{ PublicMessage() string }
	if errors.As(err, &pubErr) {
		return err
	}
	if errors.Is(err, kernalmasterdata.ErrCompanyNotFound) ||
		errors.Is(err, kernalmasterdata.ErrJournalNotFound) ||
		strings.Contains(strings.ToLower(err.Error()), "not found") {
		return publicapplication.NewError(publicapplication.ErrNotFound, err.Error(), err)
	}
	if errors.Is(err, posting.ErrUnbalancedEntry) ||
		errors.Is(err, posting.ErrMinimumLines) ||
		errors.Is(err, posting.ErrInvalidEntry) ||
		errors.Is(err, types.ErrInvalidAmount) ||
		errors.Is(err, types.ErrInvalidCurrency) {
		return publicapplication.NewError(publicapplication.ErrValidation, err.Error(), err)
	}
	return err
}
