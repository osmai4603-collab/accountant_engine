package sdk

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"text/template"
	"time"

	"github.com/google/uuid"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/events"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/journal"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	publicapplication "github.com/osmai4603-collab/accounting_engine/pkg/spi/application"
	publicevents "github.com/osmai4603-collab/accounting_engine/pkg/spi/events"
	publicmasterdata "github.com/osmai4603-collab/accounting_engine/pkg/spi/masterdata"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/storage"
	"github.com/shopspring/decimal"
)

type PublicEventIngestion = publicevents.Ingestion
type PublicEventAccountMapping = publicevents.AccountMapping
type PublicEventIdempotencyStore = publicevents.IdempotencyStore
type PublicEventScopedIdempotencyStore = publicevents.ScopedIdempotencyStore
type PublicEventMappingStore = publicevents.MappingStore

const (
	PublicEventPayloadHashVersion = "v1"

	IngestionStatusStarted          = publicevents.StatusStarted
	IngestionStatusCompleted        = publicevents.StatusCompleted
	IngestionStatusRetryableFailure = publicevents.StatusRetryableFailure
	IngestionStatusTerminalFailure  = publicevents.StatusTerminalFailure
)

func publicEventFingerprint(event events.AccountingEvent, rule events.PostingRule) (string, error) {
	payload := struct {
		HashVersion   string                `json:"hash_version"`
		EventType     string                `json:"event_type"`
		EventID       string                `json:"event_id"`
		CompanyID     types.CompanyID       `json:"company_id"`
		LedgerID      string                `json:"ledger_id"`
		EffectiveDate time.Time             `json:"effective_date"`
		Amount        types.Money           `json:"amount"`
		Dimensions    map[string]string     `json:"dimensions"`
		Payload       map[string]any        `json:"payload"`
		JournalDraft  *journal.JournalEntry `json:"journal_draft"`
		Rule          events.PostingRule    `json:"rule"`
	}{
		HashVersion:   PublicEventPayloadHashVersion,
		EventType:     event.EventType,
		EventID:       event.EventID,
		CompanyID:     event.CompanyID,
		LedgerID:      strings.TrimSpace(event.LedgerID),
		EffectiveDate: event.EffectiveDate.UTC(),
		Amount:        event.Amount,
		Dimensions:    event.Dimensions,
		Payload:       event.Payload,
		JournalDraft:  event.JournalDraft,
		Rule:          rule,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

func normalizeLedgerEntryIDs(entry *journal.JournalEntry) {
	if entry == nil {
		return
	}
	if uuid.UUID(entry.ID) == uuid.Nil {
		entry.ID = types.NewJournalEntryID()
	}
	for i := range entry.Lines {
		if uuid.UUID(entry.Lines[i].ID) == uuid.Nil {
			entry.Lines[i].ID = types.NewJournalEntryLineID()
		}
	}
	if entry.EffectiveDate.IsZero() && !entry.PostingDate.IsZero() {
		entry.EffectiveDate = entry.PostingDate
	}
	if entry.PostingDate.IsZero() && !entry.EffectiveDate.IsZero() {
		entry.PostingDate = entry.EffectiveDate
	}
	if entry.Status == "" {
		entry.Status = journal.EntryDraft
	}
}

type defaultEventService struct {
	idempotencyStore PublicEventIdempotencyStore
	mappingStore     PublicEventMappingStore
	router           *events.Router
	publicMasterData publicmasterdata.Store
	useCases         publicapplication.Service
	requirePrincipal bool
	waitTimeout      time.Duration
	waitInterval     time.Duration
}

func newDefaultEventService(
	idemStore PublicEventIdempotencyStore,
	mapStore PublicEventMappingStore,
	router *events.Router,
	masterData publicmasterdata.Store,
	useCases publicapplication.Service,
	requirePrincipal bool,
	waitTimeout, waitInterval time.Duration,
) *defaultEventService {
	return &defaultEventService{
		idempotencyStore: idemStore,
		mappingStore:     mapStore,
		router:           router,
		publicMasterData: masterData,
		useCases:         useCases,
		requirePrincipal: requirePrincipal,
		waitTimeout:      waitTimeout,
		waitInterval:     waitInterval,
	}
}

func (s *defaultEventService) ProcessEvent(ctx context.Context, event events.AccountingEvent) (journal.JournalEntry, error) {
	if s.useCases == nil {
		return journal.JournalEntry{}, publicapplication.NewError(
			publicapplication.ErrDependencyUnavailable, "event ingestion application is unavailable", ErrCapabilityNotImplemented,
		)
	}
	if strings.TrimSpace(event.EventType) == "" {
		return journal.JournalEntry{}, publicapplication.NewError(publicapplication.ErrValidation, "event type is required", nil)
	}
	if uuid.UUID(event.CompanyID) == uuid.Nil {
		return journal.JournalEntry{}, publicapplication.NewError(publicapplication.ErrValidation, "event company id is required", nil)
	}
	if strings.TrimSpace(event.EventID) == "" {
		return journal.JournalEntry{}, publicapplication.NewError(publicapplication.ErrValidation, "event id is required", nil)
	}
	if event.JournalDraft == nil && (event.Amount.Amount().IsNegative() || event.Amount.Amount().IsZero()) {
		return journal.JournalEntry{}, publicapplication.NewError(publicapplication.ErrValidation, "event amount must be positive", nil)
	}
	if len(event.Dimensions) > 0 {
		return journal.JournalEntry{}, wrapEventError(publicapplication.ErrValidation, ErrEventDimensionsUnsupported.Error(), ErrEventDimensionsUnsupported)
	}
	if event.JournalDraft != nil && len(event.Payload) > 0 {
		return journal.JournalEntry{}, wrapEventError(publicapplication.ErrValidation, ErrEventPayloadUnsupported.Error(), ErrEventPayloadUnsupported)
	}
	if event.EffectiveDate.IsZero() && (event.JournalDraft == nil ||
		(event.JournalDraft.EffectiveDate.IsZero() && event.JournalDraft.PostingDate.IsZero())) {
		return journal.JournalEntry{}, publicapplication.NewError(publicapplication.ErrValidation, "event effective date is required", nil)
	}

	requestContext := eventCommandContext(ctx, event, s.requirePrincipal)
	key := requestContext.IdempotencyKey
	companyUUID := uuid.UUID(event.CompanyID)
	idemStore := s.idempotencyStore
	mapStore := s.mappingStore

	var (
		entry    journal.JournalEntry
		err      error
		ruleUsed events.PostingRule
		hasRule  bool
	)

	if event.JournalDraft != nil {
		draft := *event.JournalDraft
		if draft.CompanyID == (types.CompanyID{}) || draft.CompanyID == types.CompanyID(uuid.Nil) {
			draft.CompanyID = event.CompanyID
		}
		if draft.CompanyID != event.CompanyID {
			return journal.JournalEntry{}, publicapplication.NewError(publicapplication.ErrValidation, "journal draft company mismatch", nil)
		}
		if strings.TrimSpace(draft.SourceModule) == "" {
			draft.SourceModule = event.EventType
		}
		if strings.TrimSpace(draft.ReferenceID) == "" {
			draft.ReferenceID = event.EventID
		}
		if draft.EffectiveDate.IsZero() {
			draft.EffectiveDate = event.EffectiveDate
		}
		if draft.PostingDate.IsZero() {
			draft.PostingDate = draft.EffectiveDate
		}
		if draft.Status != "" && draft.Status != journal.EntryDraft {
			return journal.JournalEntry{}, publicapplication.NewError(
				publicapplication.ErrValidation, "event journal draft must be in draft status", nil,
			)
		}
		normalizeLedgerEntryIDs(&draft)
		entry = draft
	} else {
		if mapStore != nil && companyUUID != uuid.Nil {
			m, ok, mErr := mapStore.GetPublicEventAccountMapping(ctx, companyUUID, event.EventType, strings.TrimSpace(event.LedgerID))
			if mErr != nil {
				return journal.JournalEntry{}, mErr
			}
			if !ok && strings.TrimSpace(event.LedgerID) != "" {
				m, ok, mErr = mapStore.GetPublicEventAccountMapping(ctx, companyUUID, event.EventType, "")
				if mErr != nil {
					return journal.JournalEntry{}, mErr
				}
			}
			if ok {
				ruleUsed = events.PostingRule{
					EventType:           event.EventType,
					JournalCode:         m.JournalCode,
					DebitAccountCode:    m.DebitAccountCode,
					CreditAccountCode:   m.CreditAccountCode,
					DescriptionTemplate: m.DescriptionTemplate,
				}
				hasRule = true
			}
		}
		if !hasRule && s.router != nil {
			rules := s.router.GetRules(event.EventType)
			if len(rules) > 0 {
				ruleUsed = rules[0]
				hasRule = true
			}
		}
		if !hasRule {
			return journal.JournalEntry{}, fmt.Errorf("no account mapping/rule registered for event type %q", event.EventType)
		}

		entry, err = s.constructEventEntry(ctx, ruleUsed, event)
		if err != nil {
			if errors.Is(err, ErrEventPayloadUnsupported) {
				return journal.JournalEntry{}, wrapEventError(publicapplication.ErrValidation, ErrEventPayloadUnsupported.Error(), ErrEventPayloadUnsupported)
			}
			if errors.Is(err, ErrExchangeRateUnavailable) {
				return journal.JournalEntry{}, wrapEventError(publicapplication.ErrDependencyUnavailable, ErrExchangeRateUnavailable.Error(), ErrExchangeRateUnavailable)
			}
			return journal.JournalEntry{}, err
		}
	}

	normalizeLedgerEntryIDs(&entry)
	if err := validatePostingEntry(entry); err != nil {
		return journal.JournalEntry{}, err
	}

	fp, err := publicEventFingerprint(event, ruleUsed)
	if err != nil {
		return journal.JournalEntry{}, fmt.Errorf("fingerprint accounting event: %w", err)
	}

	if key == "" {
		return s.postEventEntry(ctx, entry, requestContext)
	}

	if idemStore == nil {
		return journal.JournalEntry{}, wrapEventError(
			publicapplication.ErrDependencyUnavailable,
			"durable event idempotency store is not configured",
			ErrIdempotencyStateUnavailable,
		)
	}

	sourceSystem := publicevents.NormalizeSourceSystem(requestContext.SourceSystem)
	if sourceSystem != publicevents.DefaultSourceSystem {
		if _, ok := idemStore.(PublicEventScopedIdempotencyStore); !ok {
			return journal.JournalEntry{}, wrapEventError(
				publicapplication.ErrDependencyUnavailable,
				"idempotency store does not support source-system scoping",
				ErrIdempotencyStateUnavailable,
			)
		}
	}

	rec := PublicEventIngestion{
		CompanyID:          companyUUID,
		IdempotencyKey:     key,
		SourceSystem:       sourceSystem,
		EventType:          event.EventType,
		PayloadHash:        fp,
		PayloadHashVersion: PublicEventPayloadHashVersion,
		EntryID:            uuid.UUID(entry.ID),
		Entry:              entry,
		Status:             publicevents.StatusStarted,
	}

	existing, claimed, cErr := idemStore.ClaimPublicEventIngestion(ctx, rec)
	if cErr != nil {
		if errors.Is(cErr, storage.ErrUniqueConstraint) {
			return journal.JournalEntry{}, wrapEventError(
				publicapplication.ErrConflict, "idempotency key conflicts with a different event", ErrIdempotencyKeyConflict,
			)
		}
		return journal.JournalEntry{}, cErr
	}
	if !claimed {
		return s.handleExistingIngestion(ctx, idemStore, companyUUID, sourceSystem, key, fp, existing, entry, requestContext)
	}
	return s.executeClaimedIngestion(ctx, idemStore, companyUUID, sourceSystem, key, fp, existing, entry, requestContext)
}

func (s *defaultEventService) constructEventEntry(ctx context.Context, rule events.PostingRule, event events.AccountingEvent) (journal.JournalEntry, error) {
	amt := event.Amount
	if amt.Currency() == "" {
		return journal.JournalEntry{}, errors.New("event amount currency is required")
	}
	if s.publicMasterData == nil {
		return journal.JournalEntry{}, ErrCapabilityNotImplemented
	}
	company, err := s.publicMasterData.GetCompany(ctx, event.CompanyID)
	if err != nil {
		return journal.JournalEntry{}, err
	}
	if !strings.EqualFold(amt.Currency(), company.FunctionalCurrency) {
		return journal.JournalEntry{}, ErrExchangeRateUnavailable
	}
	journalID, err := resolveDefaultJournalIDForStore(ctx, s.publicMasterData, event.CompanyID, rule.JournalCode)
	if err != nil {
		return journal.JournalEntry{}, err
	}
	if len(event.Payload) > 0 && !strings.Contains(rule.DescriptionTemplate, ".Payload") {
		return journal.JournalEntry{}, publicapplication.NewError(
			publicapplication.ErrValidation, ErrEventPayloadUnsupported.Error(), ErrEventPayloadUnsupported,
		)
	}
	zeroAmt, err := types.Zero(amt.Currency())
	if err != nil {
		return journal.JournalEntry{}, err
	}
	debitAccountID, err := s.resolveRuleAccount(ctx, event.CompanyID, rule.DebitAccountCode)
	if err != nil {
		return journal.JournalEntry{}, fmt.Errorf("resolve debit account %q: %w", rule.DebitAccountCode, err)
	}
	creditAccountID, err := s.resolveRuleAccount(ctx, event.CompanyID, rule.CreditAccountCode)
	if err != nil {
		return journal.JournalEntry{}, fmt.Errorf("resolve credit account %q: %w", rule.CreditAccountCode, err)
	}
	desc, err := renderEventDescription(rule.DescriptionTemplate, event)
	if err != nil {
		return journal.JournalEntry{}, err
	}
	effective := event.EffectiveDate
	if effective.IsZero() {
		effective = time.Now().UTC()
	}
	return journal.JournalEntry{
		ID:             types.NewJournalEntryID(),
		CompanyID:      event.CompanyID,
		JournalID:      journalID,
		Status:         journal.EntryDraft,
		Classification: journal.EntryClassRegular,
		EffectiveDate:  effective,
		PostingDate:    effective,
		Description:    desc,
		SourceModule:   event.EventType,
		ReferenceID:    event.EventID,
		Lines: []journal.EntryLine{
			{
				ID:                  types.NewJournalEntryLineID(),
				AccountID:           debitAccountID,
				TransactionAmount:   amt,
				TransactionCurrency: amt.Currency(),
				ExchangeRate:        decimal.NewFromInt(1),
				FunctionalDebit:     amt,
				FunctionalCredit:    zeroAmt,
			},
			{
				ID:                  types.NewJournalEntryLineID(),
				AccountID:           creditAccountID,
				TransactionAmount:   amt,
				TransactionCurrency: amt.Currency(),
				ExchangeRate:        decimal.NewFromInt(1),
				FunctionalDebit:     zeroAmt,
				FunctionalCredit:    amt,
			},
		},
		CreatedBy: "event:" + event.EventType,
	}, nil
}

func renderEventDescription(descriptionTemplate string, event events.AccountingEvent) (string, error) {
	if strings.TrimSpace(descriptionTemplate) == "" {
		return fmt.Sprintf("Event: %s (%s)", event.EventType, event.EventID), nil
	}
	parsed, err := template.New("event-description").Option("missingkey=error").Parse(descriptionTemplate)
	if err != nil {
		return "", fmt.Errorf("parse event description template: %w", err)
	}
	var rendered bytes.Buffer
	if err := parsed.Execute(&rendered, struct {
		EventID       string
		EventType     string
		CompanyID     types.CompanyID
		LedgerID      string
		EffectiveDate time.Time
		Amount        types.Money
		Payload       map[string]any
	}{
		EventID: event.EventID, EventType: event.EventType, CompanyID: event.CompanyID,
		LedgerID: event.LedgerID, EffectiveDate: event.EffectiveDate,
		Amount: event.Amount, Payload: event.Payload,
	}); err != nil {
		return "", fmt.Errorf("render event description template: %w", err)
	}
	description := strings.TrimSpace(rendered.String())
	if !strings.Contains(descriptionTemplate, "{{") {
		description = fmt.Sprintf("%s: %s (%s)", description, event.EventType, event.EventID)
	}
	if description == "" {
		return "", errors.New("event description template rendered an empty description")
	}
	return description, nil
}

func (s *defaultEventService) resolveRuleAccount(ctx context.Context, companyID types.CompanyID, code string) (types.AccountID, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return types.AccountID{}, errors.New("account code is required")
	}
	if s.publicMasterData != nil {
		acc, err := s.publicMasterData.GetAccountByCode(ctx, companyID, code)
		if err == nil {
			return acc.ID, nil
		}
		// If code is already a valid UUID string
		if parsed, parseErr := uuid.Parse(code); parseErr == nil {
			return types.AccountID(parsed), nil
		}
		return types.AccountID{}, err
	}
	return types.AccountID{}, errors.New("account code resolution requires master data service")
}

func resolveDefaultJournalIDForStore(ctx context.Context, masterData publicmasterdata.Store, companyID types.CompanyID, journalCode string) (types.JournalID, error) {
	if masterData == nil {
		return types.JournalID{}, ErrCapabilityNotImplemented
	}
	companyID = types.CompanyID(uuid.UUID(companyID))
	if uuid.UUID(companyID) == uuid.Nil {
		return types.JournalID{}, errors.New("company id is required")
	}
	journalCode = strings.TrimSpace(journalCode)
	if journalCode == "" {
		journalCode = "GENERAL"
	}
	j, err := masterData.GetJournalByCode(ctx, companyID, journalCode)
	if err == nil {
		return types.JournalID(j.ID), nil
	}
	if !errors.Is(err, publicmasterdata.ErrJournalNotFound) {
		return types.JournalID{}, fmt.Errorf("resolve journal %q: %w", journalCode, err)
	}
	if journalCode != "GENERAL" {
		return types.JournalID{}, fmt.Errorf("resolve journal %q: %w", journalCode, err)
	}
	journalID := publicmasterdata.NewJournalID()
	if err := masterData.CreateJournal(ctx, publicmasterdata.Journal{
		ID:        journalID,
		CompanyID: companyID,
		Code:      journalCode,
		Name:      "General Journal",
	}); err != nil {
		if errors.Is(err, publicmasterdata.ErrDuplicateJournalCode) {
			j, retryErr := masterData.GetJournalByCode(ctx, companyID, journalCode)
			if retryErr == nil {
				return types.JournalID(j.ID), nil
			}
			return types.JournalID{}, fmt.Errorf("resolve journal %q: %w", journalCode, retryErr)
		}
		return types.JournalID{}, fmt.Errorf("create journal %q: %w", journalCode, err)
	}
	return types.JournalID(journalID), nil
}

func (s *defaultEventService) executeClaimedIngestion(
	ctx context.Context,
	idemStore PublicEventIdempotencyStore,
	companyID uuid.UUID,
	sourceSystem, key, payloadHash string,
	record PublicEventIngestion,
	entry journal.JournalEntry,
	requestContext publicapplication.Context,
) (journal.JournalEntry, error) {
	working := entry
	if record.EntryID != uuid.Nil {
		working.ID = types.JournalEntryID(record.EntryID)
	}
	if uuid.UUID(record.Entry.ID) != uuid.Nil {
		working = record.Entry
	}
	if uuid.UUID(working.ID) != uuid.Nil {
		current, getErr := s.fetchPostedEntry(ctx, entry.CompanyID, working.ID, requestContext)
		if getErr == nil && uuid.UUID(current.ID) != uuid.Nil {
			if current.Status == journal.EntryPosted {
				if err := s.completeIngestion(ctx, idemStore, companyID, sourceSystem, key, payloadHash, current); err != nil {
					return journal.JournalEntry{}, fmt.Errorf("complete recovered event idempotency record: %w", err)
				}
				return current, nil
			}
			if current.Status == journal.EntryDraft {
				working = current
			}
		} else if getErr != nil && !errors.Is(getErr, publicapplication.ErrNotFound) {
			return journal.JournalEntry{}, fmt.Errorf("check event entry before retry: %w", getErr)
		}
	}

	posted, postErr := s.postEventEntry(ctx, working, requestContext)
	if postErr != nil {
		if recovered, ok := s.recoverPostedEntry(ctx, entry.CompanyID, working.ID, requestContext); ok {
			if err := s.completeIngestion(ctx, idemStore, companyID, sourceSystem, key, payloadHash, recovered); err != nil {
				return journal.JournalEntry{}, fmt.Errorf("complete committed event idempotency record: %w", err)
			}
			return recovered, nil
		}
		failureClass := classifyEventFailure(postErr)
		if failErr := s.failIngestion(ctx, idemStore, companyID, sourceSystem, key, failureClass, postErr.Error()); failErr != nil {
			return journal.JournalEntry{}, errors.Join(postErr, fmt.Errorf("mark event idempotency record failed: %w", failErr))
		}
		return journal.JournalEntry{}, postErr
	}
	if err := s.completeIngestion(ctx, idemStore, companyID, sourceSystem, key, payloadHash, posted); err != nil {
		return journal.JournalEntry{}, fmt.Errorf("complete event idempotency record after posting: %w", err)
	}
	return posted, nil
}

func (s *defaultEventService) handleExistingIngestion(
	ctx context.Context,
	idemStore PublicEventIdempotencyStore,
	companyID uuid.UUID,
	sourceSystem, key, payloadHash string,
	existing PublicEventIngestion,
	entry journal.JournalEntry,
	requestContext publicapplication.Context,
) (journal.JournalEntry, error) {
	if existing.PayloadHash != "" && existing.PayloadHash != payloadHash {
		return journal.JournalEntry{}, wrapEventError(
			publicapplication.ErrConflict, "idempotency key conflicts with a different event", ErrIdempotencyKeyConflict,
		)
	}
	switch publicevents.NormalizeStatus(existing.Status) {
	case publicevents.StatusCompleted:
		return s.completedIngestionResult(ctx, existing, requestContext)
	case publicevents.StatusTerminalFailure:
		return journal.JournalEntry{}, wrapEventError(
			publicapplication.ErrConflict,
			"idempotency key previously failed terminally",
			ErrIdempotencyTerminalFailure,
		)
	case publicevents.StatusRetryableFailure:
		return journal.JournalEntry{}, wrapEventError(
			publicapplication.ErrDependencyUnavailable,
			"previous event ingestion attempt failed transiently; retry the event",
			errors.Join(ErrIdempotencyInProgress, failureReasonError(existing)),
		)
	case publicevents.StatusStarted:
		rec, terminal, waitErr := s.waitForIngestion(ctx, idemStore, companyID, sourceSystem, key)
		if waitErr != nil {
			return journal.JournalEntry{}, waitErr
		}
		if terminal {
			if rec.PayloadHash != "" && rec.PayloadHash != payloadHash {
				return journal.JournalEntry{}, wrapEventError(
					publicapplication.ErrConflict, "idempotency key conflicts with a different event", ErrIdempotencyKeyConflict,
				)
			}
			switch publicevents.NormalizeStatus(rec.Status) {
			case publicevents.StatusCompleted:
				return s.completedIngestionResult(ctx, rec, requestContext)
			case publicevents.StatusTerminalFailure:
				return journal.JournalEntry{}, wrapEventError(
					publicapplication.ErrConflict,
					"idempotency key previously failed terminally",
					ErrIdempotencyTerminalFailure,
				)
			}
		}
		return s.recoverStartedIngestion(ctx, idemStore, companyID, sourceSystem, key, payloadHash, existing, entry, requestContext)
	default:
		return journal.JournalEntry{}, wrapEventError(
			publicapplication.ErrConflict, "idempotency key is in an invalid state", ErrIdempotencyStateConflict,
		)
	}
}

func (s *defaultEventService) waitForIngestion(
	ctx context.Context,
	idemStore PublicEventIdempotencyStore,
	companyID uuid.UUID,
	sourceSystem, key string,
) (PublicEventIngestion, bool, error) {
	timeout, interval := s.waitPolicy()
	deadline := time.Now().Add(timeout)
	for {
		if time.Now().After(deadline) {
			return PublicEventIngestion{}, false, nil
		}
		select {
		case <-ctx.Done():
			return PublicEventIngestion{}, false, ctx.Err()
		case <-time.After(interval):
		}
		rec, found, err := s.getIngestion(ctx, idemStore, companyID, sourceSystem, key)
		if err != nil {
			return PublicEventIngestion{}, false, err
		}
		if !found {
			return PublicEventIngestion{}, false, nil
		}
		switch publicevents.NormalizeStatus(rec.Status) {
		case publicevents.StatusCompleted, publicevents.StatusTerminalFailure,
			publicevents.StatusRetryableFailure:
			return rec, true, nil
		}
	}
}

func (s *defaultEventService) recoverStartedIngestion(
	ctx context.Context,
	idemStore PublicEventIdempotencyStore,
	companyID uuid.UUID,
	sourceSystem, key, payloadHash string,
	existing PublicEventIngestion,
	entry journal.JournalEntry,
	requestContext publicapplication.Context,
) (journal.JournalEntry, error) {
	recovery := journal.JournalEntry{}
	if existing.EntryID != uuid.Nil {
		recovery.ID = types.JournalEntryID(existing.EntryID)
	}
	if uuid.UUID(existing.Entry.ID) != uuid.Nil {
		recovery = existing.Entry
	}
	if uuid.UUID(recovery.ID) == uuid.Nil {
		return journal.JournalEntry{}, wrapEventError(
			publicapplication.ErrConflict, "event ingestion is still in progress", ErrIdempotencyInProgress,
		)
	}
	if posted, ok := s.recoverPostedEntry(ctx, entry.CompanyID, recovery.ID, requestContext); ok {
		if err := s.completeIngestion(ctx, idemStore, companyID, sourceSystem, key, payloadHash, posted); err != nil {
			return journal.JournalEntry{}, fmt.Errorf("complete recovered event idempotency record: %w", err)
		}
		return posted, nil
	}
	posted, postErr := s.postEventEntry(ctx, recovery, requestContext)
	if postErr != nil {
		if recovered, ok := s.recoverPostedEntry(ctx, entry.CompanyID, recovery.ID, requestContext); ok {
			if err := s.completeIngestion(ctx, idemStore, companyID, sourceSystem, key, payloadHash, recovered); err != nil {
				return journal.JournalEntry{}, fmt.Errorf("complete recovered event idempotency record: %w", err)
			}
			return recovered, nil
		}
		return journal.JournalEntry{}, wrapEventError(
			publicapplication.ErrConflict, "event ingestion is still in progress",
			errors.Join(ErrIdempotencyInProgress, postErr),
		)
	}
	if err := s.completeIngestion(ctx, idemStore, companyID, sourceSystem, key, payloadHash, posted); err != nil {
		return journal.JournalEntry{}, fmt.Errorf("complete recovered event idempotency record: %w", err)
	}
	return posted, nil
}

func (s *defaultEventService) completedIngestionResult(
	ctx context.Context,
	record PublicEventIngestion,
	requestContext publicapplication.Context,
) (journal.JournalEntry, error) {
	if record.EntryID == uuid.Nil {
		return journal.JournalEntry{}, wrapEventError(
			publicapplication.ErrConflict, "completed event record has no entry id", ErrIdempotencyInProgress,
		)
	}
	posted, err := s.fetchPostedEntry(ctx, types.CompanyID(record.CompanyID), types.JournalEntryID(record.EntryID), requestContext)
	if err != nil {
		return journal.JournalEntry{}, fmt.Errorf("load completed event idempotency result: %w", err)
	}
	if posted.Status != journal.EntryPosted {
		return journal.JournalEntry{}, wrapEventError(
			publicapplication.ErrConflict, "completed event entry is not posted", ErrIdempotencyInProgress,
		)
	}
	return posted, nil
}

func (s *defaultEventService) recoverPostedEntry(
	ctx context.Context,
	companyID types.CompanyID,
	entryID types.JournalEntryID,
	requestContext publicapplication.Context,
) (journal.JournalEntry, bool) {
	if uuid.UUID(entryID) == uuid.Nil {
		return journal.JournalEntry{}, false
	}
	entry, err := s.fetchPostedEntry(ctx, companyID, entryID, requestContext)
	if err != nil || uuid.UUID(entry.ID) == uuid.Nil || entry.Status != journal.EntryPosted {
		return journal.JournalEntry{}, false
	}
	return entry, true
}

func (s *defaultEventService) getIngestion(
	ctx context.Context,
	idemStore PublicEventIdempotencyStore,
	companyID uuid.UUID,
	sourceSystem, key string,
) (PublicEventIngestion, bool, error) {
	if scoped, ok := idemStore.(PublicEventScopedIdempotencyStore); ok {
		return scoped.GetPublicEventIngestionForSource(ctx, companyID, sourceSystem, key)
	}
	return idemStore.GetPublicEventIngestion(ctx, companyID, key)
}

func (s *defaultEventService) completeIngestion(
	ctx context.Context,
	idemStore PublicEventIdempotencyStore,
	companyID uuid.UUID,
	sourceSystem, key, payloadHash string,
	entry journal.JournalEntry,
) error {
	if scoped, ok := idemStore.(PublicEventScopedIdempotencyStore); ok {
		return scoped.CompletePublicEventIngestionForSource(ctx, companyID, sourceSystem, key, payloadHash, entry)
	}
	return idemStore.CompletePublicEventIngestion(ctx, companyID, key, entry)
}

func (s *defaultEventService) failIngestion(
	ctx context.Context,
	idemStore PublicEventIdempotencyStore,
	companyID uuid.UUID,
	sourceSystem, key, failureClass, reason string,
) error {
	if classified, ok := idemStore.(publicevents.ClassifiedIdempotencyStore); ok {
		return classified.FailPublicEventIngestionClassified(ctx, companyID, sourceSystem, key, failureClass, reason)
	}
	if scoped, ok := idemStore.(PublicEventScopedIdempotencyStore); ok {
		return scoped.FailPublicEventIngestionForSource(ctx, companyID, sourceSystem, key, failureClass, reason)
	}
	return idemStore.FailPublicEventIngestion(ctx, companyID, key, reason)
}

func classifyEventFailure(err error) string {
	if err == nil {
		return ""
	}
	if storage.IsRetryable(err) ||
		errors.Is(err, storage.ErrTransactionAborted) ||
		errors.Is(err, publicapplication.ErrDependencyUnavailable) ||
		errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, context.Canceled) {
		return publicevents.FailureClassTransient
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return publicevents.FailureClassTransient
	}
	return publicevents.FailureClassTerminal
}

func failureReasonError(record PublicEventIngestion) error {
	if strings.TrimSpace(record.FailureReason) == "" {
		return nil
	}
	return errors.New(record.FailureReason)
}

func wrapEventError(kind error, message string, cause error) error {
	return fmt.Errorf("%w: %w", publicapplication.NewError(kind, message, nil), cause)
}

func validatePostingEntry(entry journal.JournalEntry) error {
	switch {
	case uuid.UUID(entry.CompanyID) == uuid.Nil:
		return publicapplication.NewError(publicapplication.ErrValidation, "company id is required", nil)
	case uuid.UUID(entry.JournalID) == uuid.Nil:
		return publicapplication.NewError(publicapplication.ErrValidation, "journal id is required", nil)
	case len(entry.Lines) < 2:
		return publicapplication.NewError(publicapplication.ErrValidation, "at least two journal lines are required", nil)
	case entry.EffectiveDate.IsZero() && entry.PostingDate.IsZero():
		return publicapplication.NewError(publicapplication.ErrValidation, "effective or posting date is required", nil)
	default:
		return nil
	}
}

func (s *defaultEventService) postEventEntry(
	ctx context.Context,
	entry journal.JournalEntry,
	requestContext publicapplication.Context,
) (journal.JournalEntry, error) {
	command := createDraftCommand(entry, requestContext)
	created, err := s.useCases.CreateDraft(ctx, command)
	if err != nil {
		return journal.JournalEntry{}, err
	}
	posted, err := s.useCases.Post(ctx, publicapplication.PostEntryCommand{
		Context:   command.Context,
		CompanyID: entry.CompanyID,
		EntryID:   created.Entry.ID,
	})
	if err != nil {
		return journal.JournalEntry{}, err
	}
	return posted.Entry, nil
}

func (s *defaultEventService) fetchPostedEntry(
	ctx context.Context,
	companyID types.CompanyID,
	entryID types.JournalEntryID,
	requestContext publicapplication.Context,
) (journal.JournalEntry, error) {
	return s.useCases.GetEntry(ctx, publicapplication.GetEntryQuery{
		Context:   requestContext,
		CompanyID: companyID,
		EntryID:   entryID,
	})
}

func (s *defaultEventService) waitPolicy() (time.Duration, time.Duration) {
	timeout := s.waitTimeout
	if timeout <= 0 {
		timeout = defaultEventIdempotencyWaitTimeout
	}
	interval := s.waitInterval
	if interval <= 0 {
		interval = defaultEventIdempotencyWaitInterval
	}
	return timeout, interval
}

func eventCommandContext(ctx context.Context, event events.AccountingEvent, requirePrincipal bool) publicapplication.Context {
	principal, hasPrincipal := publicapplication.PrincipalFromContext(ctx)
	if !hasPrincipal && !requirePrincipal {
		principal = publicapplication.Principal{
			ActorID:    "event-ingestion",
			CompanyIDs: []types.CompanyID{event.CompanyID},
		}
	}
	metadata, _ := publicapplication.RequestMetadataFromContext(ctx)
	correlationID := strings.TrimSpace(metadata.CorrelationID)
	if correlationID == "" {
		correlationID = strings.TrimSpace(event.CorrelationID)
	}
	if correlationID == "" {
		correlationID = strings.TrimSpace(event.EventID)
	}
	idempotencyKey := strings.TrimSpace(event.IdempotencyKey)
	if idempotencyKey == "" {
		idempotencyKey = strings.TrimSpace(metadata.IdempotencyKey)
	}
	sourceSystem := strings.TrimSpace(metadata.SourceSystem)
	if sourceSystem == "" {
		sourceSystem = strings.TrimSpace(event.SourceSystem)
	}
	if sourceSystem == "" {
		sourceSystem = "event"
	}
	return publicapplication.Context{
		Principal:      principal,
		CorrelationID:  correlationID,
		IdempotencyKey: idempotencyKey,
		SourceSystem:   sourceSystem,
	}
}

func createDraftCommand(entry journal.JournalEntry, requestContext publicapplication.Context) publicapplication.CreateDraftCommand {
	exchangeRate := ""
	if !entry.ExchangeRate.IsZero() {
		exchangeRate = entry.ExchangeRate.String()
	}
	return publicapplication.CreateDraftCommand{
		Context:             requestContext,
		CompanyID:           entry.CompanyID,
		JournalID:           entry.JournalID,
		FiscalPeriodID:      entry.FiscalPeriodID,
		EffectiveDate:       entry.EffectiveDate,
		PostingDate:         entry.PostingDate,
		Description:         entry.Description,
		SourceModule:        entry.SourceModule,
		ReferenceID:         entry.ReferenceID,
		TransactionCurrency: entry.TransactionCurrency,
		ExchangeRate:        exchangeRate,
		Classification:      entry.Classification,
		TargetPeriodNumber:  entry.TargetPeriodNumber,
		AdjustingEntryType:  entry.AdjustingEntryType,
		AdjustmentReason:    entry.AdjustmentReason,
		Lines:               entry.Lines,
	}
}
