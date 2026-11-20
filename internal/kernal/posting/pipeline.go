package posting

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/osmai4603-collab/accounting_engine/internal/kernal/journal"
	"github.com/osmai4603-collab/accounting_engine/internal/kernal/masterdata"
	"github.com/osmai4603-collab/accounting_engine/internal/kernal/policy"
	"github.com/osmai4603-collab/accounting_engine/internal/kernal/types"
)

var (
	ErrDimensionReaderRequired    = errors.New("active financial dimension reader is required")
	ErrInactiveFinancialDimension = errors.New("financial dimension is inactive")
	ErrPolicyEngineRequired       = errors.New("posting policy engine is required")
	ErrRoundingPolicyRequired     = errors.New("rounding differences are not allowed by currency policy")
)

type PostOptions struct {
	ActorID        string
	Reason         string
	CorrelationID  string
	IdempotencyKey string
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

type PostingPipeline struct {
	reader          masterdata.Reader
	execute         PostingExecutor
	executeReversal PostingReversalExecutor
	policyEngine    *policy.PolicyEngine
}

func NewPostingPipeline(reader masterdata.Reader, execute PostingExecutor, opts ...PipelineOption) *PostingPipeline {
	p := &PostingPipeline{reader: reader, execute: execute}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// PostEntry performs pure kernel validation and delegates persistence to the host.
func (p *PostingPipeline) PostEntry(ctx context.Context, entry journal.JournalEntry, opts PostOptions) (journal.JournalEntry, error) {
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
	if err := EnsureImmutable(entry.Status); err != nil {
		return journal.JournalEntry{}, err
	}
	if entry.EffectiveDate.IsZero() {
		return journal.JournalEntry{}, ErrZeroEntryDate
	}
	entry = entry.Clone()
	company, err := masterdata.ValidateCompanyScope(ctx, p.reader, entry.CompanyID)
	if err != nil {
		return journal.JournalEntry{}, err
	}
	if p.policyEngine == nil {
		return journal.JournalEntry{}, ErrPolicyEngineRequired
	}
	if strings.TrimSpace(company.FunctionalCurrency) == "" {
		return journal.JournalEntry{}, ErrMissingFunctionalCurrency
	}
	functionalCurrency, ok := types.LookupCurrency(company.FunctionalCurrency)
	if !ok || functionalCurrency.Code == types.XXX {
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
		if entry.Classification != journal.EntryClassAdjusting &&
			!(entry.TargetPeriodNumber == 13 && entry.Classification == journal.EntryClassClosing) {
			return journal.JournalEntry{}, masterdata.ErrInvalidAdjustmentPeriod
		}
		period, err = masterdata.ValidateAdjustmentPeriodForPostingWithSoftClose(
			ctx, p.reader, entry.CompanyID, entry.EffectiveDate, entry.TargetPeriodNumber,
			postingPolicy.AllowSoftClosedPosting,
		)
	} else {
		if entry.TargetPeriodNumber != 0 && entry.TargetPeriodNumber != 1 {
			return journal.JournalEntry{}, masterdata.ErrInvalidAdjustmentPeriod
		}
		period, err = masterdata.ValidatePeriodForPostingWithSoftClose(
			ctx, p.reader, entry.CompanyID, entry.EffectiveDate, postingPolicy.AllowSoftClosedPosting,
		)
	}
	if err != nil {
		return journal.JournalEntry{}, err
	}
	if entry.TargetPeriodNumber == 1 &&
		(entry.Classification != journal.EntryClassOpening || period.Number != 1) {
		return journal.JournalEntry{}, masterdata.ErrInvalidAdjustmentPeriod
	}
	if entry.TargetPeriodNumber == 0 && entry.Classification == journal.EntryClassOpening {
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
		if err := masterdata.ValidateCanPostToAccountAsOf(ctx, p.reader, entry.CompanyID, line.AccountID, entry.EffectiveDate); err != nil {
			return journal.JournalEntry{}, err
		}
		account, err := p.reader.GetAccount(ctx, entry.CompanyID, line.AccountID)
		if err != nil {
			return journal.JournalEntry{}, err
		}
		if err := masterdata.ValidateAccountScope(account, entry.CompanyID, line.AccountID); err != nil {
			return journal.JournalEntry{}, err
		}
		accounts[line.AccountID] = account
		accountTypes[line.AccountID] = account.Type
		if err := validateFinancialDimensions(ctx, p.reader, entry.CompanyID, line); err != nil {
			return journal.JournalEntry{}, err
		}
	}
	if err := policy.CheckAccountEligibility(ctx, &entry, accounts); err != nil {
		return journal.JournalEntry{}, err
	}
	if err := p.policyEngine.ValidatePostingPolicy(ctx, postingPolicy, &entry, accountTypes, time.Now().UTC()); err != nil {
		return journal.JournalEntry{}, err
	}
	now := time.Now().UTC()
	entry.PostingDate = now
	entry.PostedBy = opts.ActorID
	entry.PostedAt = &now
	entry.Status = journal.EntryPosted

	if entry.ReversalOfID != nil {
		if p.executeReversal == nil {
			return journal.JournalEntry{}, ErrReversalExecutorRequired
		}
		return p.executeReversal(ctx, *entry.ReversalOfID, entry, opts)
	}
	if p.execute == nil {
		return journal.JournalEntry{}, fmt.Errorf("posting executor is required")
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
