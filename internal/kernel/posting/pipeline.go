package posting

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/osmai4603-collab/accounting_engine/internal/kernel/journal"
	"github.com/osmai4603-collab/accounting_engine/internal/kernel/masterdata"
	"github.com/osmai4603-collab/accounting_engine/internal/kernel/policy"
	"github.com/osmai4603-collab/accounting_engine/internal/kernel/types"
)

var (
	ErrDimensionReaderRequired    = errors.New("active financial dimension reader is required")
	ErrInactiveFinancialDimension = errors.New("financial dimension is inactive")
	ErrPostingPipelineRequired    = errors.New("posting pipeline is required")
	ErrPostingContextRequired     = errors.New("posting context is required")
	ErrPostingReaderRequired      = errors.New("posting master data reader is required")
	ErrPolicyEngineRequired       = errors.New("posting policy engine is required")
	ErrPostingClockRequired       = errors.New("posting clock is required")
	ErrPostingExecutorRequired    = errors.New("posting executor is required")
	ErrRoundingPolicyRequired     = errors.New("rounding differences are not allowed by currency policy")
	ErrIdempotencyKeyRequired     = errors.New("idempotency key is required")
)

type PostOptions struct {
	ActorID        string
	Reason         string
	CorrelationID  string
	IdempotencyKey string
	// Scope and RequestFingerprint are kernel-stamped immediately before the
	// executor call. Caller-supplied values are overwritten; executors must
	// treat them as the authoritative idempotency contract (RULE-KERNEL-027).
	Scope              IdempotencyScope
	RequestFingerprint string
	// CallerFingerprint is the stable digest of caller-controlled fields. It is
	// stamped by the kernel so the host can detect payload reuse both before
	// and after the draft-to-posted transformation.
	CallerFingerprint string
}

// PostingExecutor persists a validated non-reversal posting. Implementations
// must commit the entry and its related accounting effects atomically.
type PostingExecutor func(context.Context, journal.JournalEntry, PostOptions) (journal.JournalEntry, error)

// PostingReversalExecutor atomically commits a reversal and links it to its
// original entry, rejecting an original entry that has already been reversed.
type PostingReversalExecutor func(
	context.Context,
	types.JournalEntryID,
	journal.JournalEntry,
	PostOptions,
) (journal.JournalEntry, error)

type PipelineOption func(*PostingPipeline)

type Clock interface {
	Now() time.Time
}

type systemClock struct{}

func (systemClock) Now() time.Time {
	return time.Now().UTC()
}

func WithPolicyEngine(engine *policy.PolicyEngine) PipelineOption {
	return func(p *PostingPipeline) {
		p.policyEngine = engine
	}
}

func WithReversalExecutor(execute PostingReversalExecutor) PipelineOption {
	return func(p *PostingPipeline) {
		p.executeReversal = execute
	}
}

func WithClock(clock Clock) PipelineOption {
	return func(p *PostingPipeline) {
		if clock != nil {
			p.clock = clock
		}
	}
}

type PostingPipeline struct {
	reader          masterdata.Reader
	execute         PostingExecutor
	executeReversal PostingReversalExecutor
	policyEngine    *policy.PolicyEngine
	clock           Clock
}

func NewPostingPipeline(reader masterdata.Reader, execute PostingExecutor, opts ...PipelineOption) *PostingPipeline {
	p := &PostingPipeline{reader: reader, execute: execute, clock: systemClock{}}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// PostEntry performs pure kernel validation and delegates persistence to the host.
func (p *PostingPipeline) PostEntry(ctx context.Context, entry journal.JournalEntry, opts PostOptions) (journal.JournalEntry, error) {
	if p == nil {
		return journal.JournalEntry{}, ErrPostingPipelineRequired
	}
	if ctx == nil {
		return journal.JournalEntry{}, ErrPostingContextRequired
	}
	if p.reader == nil {
		return journal.JournalEntry{}, ErrPostingReaderRequired
	}
	if p.policyEngine == nil {
		return journal.JournalEntry{}, ErrPolicyEngineRequired
	}
	if p.clock == nil {
		return journal.JournalEntry{}, ErrPostingClockRequired
	}
	if entry.ReversalOfID == nil && p.execute == nil {
		return journal.JournalEntry{}, ErrPostingExecutorRequired
	}
	if entry.ReversalOfID != nil && p.executeReversal == nil {
		return journal.JournalEntry{}, ErrReversalExecutorRequired
	}
	if err := ValidateEntryIdentity(entry); err != nil {
		return journal.JournalEntry{}, err
	}
	if err := ValidateEntryClassificationMetadata(entry); err != nil {
		return journal.JournalEntry{}, err
	}
	if entry.Status != journal.EntryDraft {
		return journal.JournalEntry{}, fmt.Errorf("%w: entry must be in draft status", ErrInvalidEntryState)
	}
	if entry.JournalID == (types.JournalID{}) {
		return journal.JournalEntry{}, ErrMissingJournalID
	}
	if strings.TrimSpace(opts.ActorID) == "" {
		return journal.JournalEntry{}, ErrPosterIDRequired
	}
	if strings.TrimSpace(opts.IdempotencyKey) == "" {
		return journal.JournalEntry{}, ErrIdempotencyKeyRequired
	}
	opts.IdempotencyKey = strings.TrimSpace(opts.IdempotencyKey)
	if err := EnsureImmutable(entry.Status); err != nil {
		return journal.JournalEntry{}, err
	}
	if entry.EffectiveDate.IsZero() {
		return journal.JournalEntry{}, ErrZeroEntryDate
	}
	entry = entry.Clone()
	if entry.AdjustingEntryType == journal.AdjustingEntryAuditLegacy {
		entry.AdjustingEntryType = journal.AdjustingEntryAudit
	}
	company, err := masterdata.ValidateCompanyScope(ctx, p.reader, entry.CompanyID)
	if err != nil {
		return journal.JournalEntry{}, err
	}
	if strings.TrimSpace(company.FunctionalCurrency) == "" {
		return journal.JournalEntry{}, ErrMissingFunctionalCurrency
	}
	functionalCurrency, ok := types.LookupCurrency(company.FunctionalCurrency)
	if !ok || functionalCurrency.IsSpecial() {
		return journal.JournalEntry{}, types.ErrInvalidCurrency
	}
	postingPolicy, err := p.policyEngine.PostingPolicyFor(entry.CompanyID, entry.EffectiveDate)
	if err != nil {
		return journal.JournalEntry{}, err
	}
	currencyPolicy, err := p.policyEngine.CurrencyPolicyFor(entry.CompanyID, entry.EffectiveDate)
	if err != nil {
		return journal.JournalEntry{}, err
	}
	if !strings.EqualFold(currencyPolicy.FunctionalCurrency, string(functionalCurrency.Code)) {
		return journal.JournalEntry{}, ErrFunctionalCurrencyMismatch
	}
	for i, line := range entry.Lines {
		if line.FunctionalDebit.Currency() != string(functionalCurrency.Code) ||
			line.FunctionalCredit.Currency() != string(functionalCurrency.Code) ||
			line.TransactionAmount.Currency() != line.TransactionCurrency {
			return journal.JournalEntry{}, fmt.Errorf("%w: line %d", ErrFunctionalCurrencyMismatch, i)
		}
		if !currencyPolicy.AllowsCurrency(line.TransactionCurrency) {
			return journal.JournalEntry{}, fmt.Errorf("%w: line %d currency %s", policy.ErrCurrencyNotAllowed, i, line.TransactionCurrency)
		}
	}
	journalReader, ok := p.reader.(masterdata.JournalRepository)
	if !ok {
		return journal.JournalEntry{}, masterdata.ErrJournalReaderRequired
	}
	entryJournal, err := journalReader.GetJournalByID(ctx, entry.CompanyID, entry.JournalID)
	if err != nil {
		return journal.JournalEntry{}, err
	}
	if entryJournal.ID != entry.JournalID || entryJournal.CompanyID != entry.CompanyID {
		return journal.JournalEntry{}, masterdata.ErrInvalidJournal
	}
	if err := entryJournal.Validate(); err != nil {
		return journal.JournalEntry{}, err
	}
	if !entryJournal.IsCurrent(entry.EffectiveDate) {
		return journal.JournalEntry{}, masterdata.ErrJournalNotCurrent
	}

	var period masterdata.FiscalPeriod
	if entry.TargetPeriodNumber == 13 || entry.TargetPeriodNumber == 14 {
		period, err = masterdata.ValidateAdjustmentPeriodForPostingWithSoftClose(
			ctx, p.reader, entry.CompanyID, entry.EffectiveDate, entry.TargetPeriodNumber,
			postingPolicy.AllowSoftClosedPosting,
		)
	} else {
		period, err = masterdata.ValidatePeriodForPostingWithSoftClose(
			ctx, p.reader, entry.CompanyID, entry.EffectiveDate, postingPolicy.AllowSoftClosedPosting,
		)
	}
	if err != nil {
		return journal.JournalEntry{}, err
	}
	if entry.TargetPeriodNumber == 1 && period.Number != 1 {
		return journal.JournalEntry{}, masterdata.ErrInvalidAdjustmentPeriod
	}
	entry.FiscalPeriodID = period.ID

	if err := RoundEntryWithRoundingPolicy(
		&entry,
		roundingAccountID(currencyPolicy),
		currencyPolicy.Round,
	); err != nil {
		if errors.Is(err, ErrRoundingAccountRequired) && !currencyPolicy.AutoSettleRoundingDifferences {
			return journal.JournalEntry{}, ErrRoundingPolicyRequired
		}
		return journal.JournalEntry{}, err
	}
	if err := ValidateEntryDoubleEntry(&entry, currencyPolicy.Round); err != nil {
		return journal.JournalEntry{}, err
	}
	for i, line := range entry.Lines {
		if !entryJournal.AllowsCurrency(line.TransactionCurrency) {
			return journal.JournalEntry{}, fmt.Errorf("%w: line %d currency %s", masterdata.ErrJournalCurrency, i, line.TransactionCurrency)
		}
		if !entryJournal.AllowsAccount(line.AccountID) {
			return journal.JournalEntry{}, fmt.Errorf("%w: line %d account %s", masterdata.ErrJournalAccount, i, line.AccountID)
		}
	}

	accounts := make(map[types.AccountID]masterdata.Account, len(entry.Lines))
	accountTypes := make(map[types.AccountID]masterdata.AccountType, len(entry.Lines))
	for _, line := range entry.Lines {
		// One effective-dated read per distinct AccountID feeds both validation
		// and eligibility (PST-006). Re-reading on repeated lines could observe a
		// different master-data version within one posting.
		if _, cached := accounts[line.AccountID]; !cached {
			account, err := masterdata.ResolvePostableAccountAsOf(
				ctx, p.reader, entry.CompanyID, line.AccountID, entry.EffectiveDate,
			)
			if err != nil {
				return journal.JournalEntry{}, err
			}
			accounts[line.AccountID] = account
			accountTypes[line.AccountID] = account.Type
		}
		if err := validateFinancialDimensions(ctx, p.reader, entry.CompanyID, line); err != nil {
			return journal.JournalEntry{}, err
		}
	}
	if err := policy.CheckAccountEligibility(ctx, &entry, accounts); err != nil {
		return journal.JournalEntry{}, err
	}
	now := p.clock.Now().UTC()
	if err := p.policyEngine.ValidatePostingPolicy(ctx, postingPolicy, &entry, accountTypes, now); err != nil {
		return journal.JournalEntry{}, err
	}
	// Audit evidence of the exact policy versions used for this posting.
	// The digest is immutable together with the posted entry (ADR P5-01).
	entry.PolicyFingerprint = policy.Fingerprint(postingPolicy, currencyPolicy)
	entry.PostingDate = now
	entry.PostedBy = opts.ActorID
	entry.PostedAt = &now
	entry.Status = journal.EntryPosted

	// The idempotency contract is derived from the frozen, fully stamped
	// snapshot. Overwriting caller values prevents a caller from choosing a
	// foreign company or operation scope for the durable key.
	scope, err := BuildIdempotencyScope(entry, opts)
	if err != nil {
		return journal.JournalEntry{}, err
	}
	fingerprint, err := FingerprintRequest(entry, opts, scope)
	if err != nil {
		return journal.JournalEntry{}, err
	}
	callerFingerprint, err := FingerprintCallerPayload(entry, opts, scope)
	if err != nil {
		return journal.JournalEntry{}, err
	}
	opts.Scope = scope
	opts.RequestFingerprint = fingerprint
	opts.CallerFingerprint = callerFingerprint

	if entry.ReversalOfID != nil {
		return p.executeReversal(ctx, *entry.ReversalOfID, entry, opts)
	}
	return p.execute(ctx, entry, opts)
}

func roundingAccountID(currencyPolicy policy.CurrencyPolicy) types.AccountID {
	if !currencyPolicy.AutoSettleRoundingDifferences {
		return types.AccountID{}
	}
	return currencyPolicy.RoundingDifferenceAccountID
}

func validateFinancialDimensions(
	ctx context.Context,
	reader masterdata.Reader,
	companyID types.CompanyID,
	line journal.EntryLine,
) error {
	if line.CostCenterID != nil {
		dimensionReader, ok := reader.(masterdata.CostCenterRepository)
		if !ok {
			return ErrDimensionReaderRequired
		}
		costCenter, err := dimensionReader.GetCostCenterForCompany(ctx, companyID, *line.CostCenterID)
		if err != nil {
			return err
		}
		if costCenter.ID != *line.CostCenterID || costCenter.CompanyID != companyID {
			return masterdata.ErrInvalidCostCenter
		}
		if !costCenter.Active {
			return ErrInactiveFinancialDimension
		}
		seen := map[types.CostCenterID]struct{}{costCenter.ID: {}}
		for costCenter.ParentID != nil {
			parentID := *costCenter.ParentID
			if _, exists := seen[parentID]; exists {
				return masterdata.ErrCostCenterCycle
			}
			seen[parentID] = struct{}{}
			costCenter, err = dimensionReader.GetCostCenterForCompany(ctx, companyID, parentID)
			if err != nil {
				return err
			}
			if costCenter.ID != parentID || costCenter.CompanyID != companyID {
				return masterdata.ErrInvalidCostCenter
			}
			if !costCenter.Active {
				return ErrInactiveFinancialDimension
			}
		}
	}

	dimensionReader, hasDimensionReader := reader.(masterdata.AnalyticalDimensionRepository)
	for _, dimension := range []struct {
		kind masterdata.AnalyticalDimensionKind
		id   string
	}{
		{kind: masterdata.DimensionProject, id: dimensionIDString(line.ProjectID)},
		{kind: masterdata.DimensionSegment, id: dimensionIDString(line.SegmentID)},
		{kind: masterdata.DimensionBusinessUnit, id: dimensionIDString(line.BusinessUnitID)},
	} {
		if dimension.id == "" {
			continue
		}
		if !hasDimensionReader {
			return ErrDimensionReaderRequired
		}
		value, err := dimensionReader.GetAnalyticalDimensionForCompany(ctx, companyID, dimension.kind, dimension.id)
		if err != nil {
			return err
		}
		if value.ID.String() != dimension.id || value.CompanyID != companyID || value.Kind != dimension.kind {
			return masterdata.ErrInvalidAnalyticalDimension
		}
		if !value.Active {
			return ErrInactiveFinancialDimension
		}
	}
	return nil
}

func dimensionIDString[T interface{ String() string }](id *T) string {
	if id == nil {
		return ""
	}
	return (*id).String()
}
