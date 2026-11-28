package valuation

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/osmai4603-collab/accounting_engine/internal/kernel/journal"
	"github.com/osmai4603-collab/accounting_engine/internal/kernel/masterdata"
	"github.com/osmai4603-collab/accounting_engine/internal/kernel/types"
	apiTypes "github.com/osmai4603-collab/accounting_engine/internal/kernel/types"
	"github.com/shopspring/decimal"
)

type PositionType string

const (
	Monetary    PositionType = "MONETARY"
	NonMonetary PositionType = "NON_MONETARY"
)

// RevaluationDimensions holds optional analytical dimensions that scope an
// IAS 21 FX revaluation balance (VAL-009 / DEC-VAL-001).
type RevaluationDimensions struct {
	PartnerID      *apiTypes.PartnerID
	CostCenterID   *apiTypes.CostCenterID
	ProjectID      *apiTypes.ProjectID
	SegmentID      *apiTypes.SegmentID
	BusinessUnitID *apiTypes.BusinessUnitID
}

func (d RevaluationDimensions) isEmpty() bool {
	return d.PartnerID == nil && d.CostCenterID == nil && d.ProjectID == nil &&
		d.SegmentID == nil && d.BusinessUnitID == nil
}

// RevaluationEntryOptions customizes metadata and analytical dimensions of a
// generated IAS 21 revaluation journal entry.
type RevaluationEntryOptions struct {
	AdjustingEntryType journal.AdjustingEntryType
	CreatedBy          string
	Dimensions         *RevaluationDimensions
}

type MonetaryPosition struct {
	AccountID              apiTypes.AccountID
	AccountType            masterdata.AccountType
	IsMonetary             *bool
	ForeignCurrencyBalance decimal.Decimal
	TransactionCurrency    string
	FunctionalCurrency     string
	CarryingAmount         types.Money
	Type                   PositionType
	IsLiability            bool
	Dimensions             RevaluationDimensions
}

type FXResult struct {
	AccountID              apiTypes.AccountID
	Currency               string
	Difference             types.Money
	IsGain                 bool
	ClosingRate            decimal.Decimal
	RateDate               time.Time
	RevaluedAmount         types.Money
	RateReference          ExchangeRate
	RateReferenceAvailable bool
	RateInverted           bool
	Dimensions             RevaluationDimensions
}

type RateProvider interface {
	GetRate(ctx context.Context, rateType RateType, from, to string, asOf time.Time) (decimal.Decimal, ExchangeRate, bool, error)
}

// EntryValidator optionally validates a generated revaluation entry before
// returning it to the caller.
type EntryValidator interface {
	ValidateDoubleEntry(entry *journal.JournalEntry) error
}

// ReversalBuilder constructs a reversing draft entry for a posted revaluation
// entry without coupling valuation directly to posting.
type ReversalBuilder interface {
	BuildReversalAt(original journal.JournalEntry, date time.Time, actor, reason string, now time.Time) (journal.JournalEntry, error)
}

type RevaluationEngine struct {
	provider        RateProvider
	validator       EntryValidator
	reversalBuilder ReversalBuilder
}

// NewRevaluationEngineWithDeps constructs a RevaluationEngine with optional
// injected validation and reversal-building adapters.
func NewRevaluationEngineWithDeps(provider RateProvider, validator EntryValidator, reversalBuilder ReversalBuilder) *RevaluationEngine {
	return &RevaluationEngine{
		provider:        provider,
		validator:       validator,
		reversalBuilder: reversalBuilder,
	}
}

type revaluationRecord struct {
	entry   journal.JournalEntry
	payload string
}

type RevaluationRegistry struct {
	mu      sync.Mutex
	entries map[string]revaluationRecord
}

func NewRevaluationRegistry() *RevaluationRegistry {
	return &RevaluationRegistry{entries: make(map[string]revaluationRecord)}
}

// RevaluationOperationKey derives the operation-level key of one revaluation.
// currency is the foreign transaction currency of the monetary position, never
// the functional currency; date is the valuation day truncated to UTC.
func RevaluationOperationKey(company apiTypes.CompanyID, account apiTypes.AccountID, currency string, date time.Time) (string, error) {
	return RevaluationOperationKeyWithDimensions(company, account, currency, date, RevaluationDimensions{})
}

// RevaluationOperationKeyWithDimensions derives a deterministic operation key
// scoped by analytical dimensions (VAL-009 / DEC-VAL-001). When all dimensions
// are unset it returns the canonical dimensionless key for compatibility.
func RevaluationOperationKeyWithDimensions(
	company apiTypes.CompanyID,
	account apiTypes.AccountID,
	currency string,
	date time.Time,
	dims RevaluationDimensions,
) (string, error) {
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if company == (apiTypes.CompanyID{}) || account == (apiTypes.AccountID{}) || date.IsZero() {
		return "", ErrInvalidPosition
	}
	if currencyInfo, ok := types.LookupCurrency(currency); !ok || currencyInfo.IsSpecial() {
		return "", types.ErrInvalidCurrency
	}
	base := fmt.Sprintf("fx-revaluation|company=%s|account=%s|currency=%s|date=%s",
		company.String(), account.String(), currency, date.UTC().Format("2006-01-02"))
	if dims.isEmpty() {
		return base, nil
	}
	return fmt.Sprintf("%s|partner=%s|cost_center=%s|project=%s|segment=%s|business_unit=%s",
		base,
		dimOrDash(dims.PartnerID),
		dimOrDash(dims.CostCenterID),
		dimOrDash(dims.ProjectID),
		dimOrDash(dims.SegmentID),
		dimOrDash(dims.BusinessUnitID),
	), nil
}

// RevaluationIdempotencyScope maps the deterministic revaluation operation key
// onto the neutral phase-7 idempotency contract. The returned scope is
// kernel-authoritative and is carried by PostOptions.Scope into
// BuildIdempotencyScope and the durable storage SPI (G-024); the host stamps
// RevaluationPayloadFingerprint as the payload hash. PostEntry itself is not
// extended with a revaluation-specific parameter.
func RevaluationIdempotencyScope(company apiTypes.CompanyID, account apiTypes.AccountID, currency string, date time.Time) (types.IdempotencyScope, error) {
	return RevaluationIdempotencyScopeWithDimensions(company, account, currency, date, RevaluationDimensions{})
}

// RevaluationIdempotencyScopeWithDimensions derives the idempotency scope
// including analytical dimension boundaries per VAL-009.
func RevaluationIdempotencyScopeWithDimensions(
	company apiTypes.CompanyID,
	account apiTypes.AccountID,
	currency string,
	date time.Time,
	dims RevaluationDimensions,
) (types.IdempotencyScope, error) {
	key, err := RevaluationOperationKeyWithDimensions(company, account, currency, date, dims)
	if err != nil {
		return types.IdempotencyScope{}, err
	}
	scope := types.IdempotencyScope{
		CompanyID: company,
		Operation: types.IdempotencyRevaluation,
		Key:       key,
	}
	if err := scope.Validate(); err != nil {
		return types.IdempotencyScope{}, err
	}
	return scope, nil
}

func dimOrDash[T interface{ String() string }](id *T) string {
	if id == nil {
		return "-"
	}
	return (*id).String()
}

func RevaluationReferenceID(result FXResult, date time.Time) (string, error) {
	currency := strings.ToUpper(strings.TrimSpace(result.Currency))
	if result.AccountID == (apiTypes.AccountID{}) || date.IsZero() {
		return "", ErrInvalidPosition
	}
	if currencyInfo, ok := types.LookupCurrency(currency); !ok || currencyInfo.IsSpecial() {
		return "", types.ErrInvalidCurrency
	}
	return fmt.Sprintf("FX-REVAL-%s-%s-%s", result.AccountID.String(), currency, date.UTC().Format("2006-01-02")), nil
}

// fpField appends a length-prefixed canonical field. Lengths remove delimiter
// ambiguity, so caller-supplied text cannot forge field or line boundaries.
func fpField(b *strings.Builder, name, value string) {
	fmt.Fprintf(b, "%s=%d:%s;", name, len(value), value)
}

func fpTime(b *strings.Builder, name string, value time.Time) {
	fpField(b, name, value.UTC().Format(time.RFC3339Nano))
}

func fpMoney(b *strings.Builder, name string, value apiTypes.Money) {
	fpField(b, name, value.ExactString())
}

func fpDecimal(b *strings.Builder, name string, value decimal.Decimal) {
	fpField(b, name, value.String())
}

func fpOptionalID[T any](b *strings.Builder, name string, value *T) {
	if value == nil {
		fpField(b, name, "")
		return
	}
	fpField(b, name, fmt.Sprint(*value))
}

// revaluationPayloadFingerprint hashes every stable accounting field. Only
// generation artefacts are excluded: entry/line IDs, status, and
// created/updated/posted timestamps. Money uses ExactString because Money.String
// is a rounded display format and must not be used for hashing. Lines are
// sorted after canonical encoding so line order is not part of identity.
func revaluationPayloadFingerprint(entry journal.JournalEntry) string {
	var canonical strings.Builder
	canonical.WriteString("fx-revaluation-payload-v2|")
	fpField(&canonical, "company", entry.CompanyID.String())
	fpField(&canonical, "journal", entry.JournalID.String())
	fpField(&canonical, "fiscal_period", entry.FiscalPeriodID.String())
	fpField(&canonical, "reference", entry.ReferenceID)
	fpField(&canonical, "entry_number", entry.EntryNumber)
	fpField(&canonical, "sequence_number", fmt.Sprint(entry.SequenceNumber))
	fpField(&canonical, "legal_number", entry.LegalNumber)
	fpField(&canonical, "classification", string(entry.Classification))
	fpField(&canonical, "adjusting_type", string(entry.AdjustingEntryType))
	fpField(&canonical, "adjustment_reason", entry.AdjustmentReason)
	fpField(&canonical, "target_period_number", fmt.Sprint(entry.TargetPeriodNumber))
	fpTime(&canonical, "effective_date", entry.EffectiveDate)
	fpTime(&canonical, "posting_date", entry.PostingDate)
	fpField(&canonical, "description", entry.Description)
	fpField(&canonical, "source_module", entry.SourceModule)
	fpField(&canonical, "transaction_currency", strings.ToUpper(strings.TrimSpace(entry.TransactionCurrency)))
	fpDecimal(&canonical, "exchange_rate", entry.ExchangeRate)
	fpField(&canonical, "policy_fingerprint", entry.PolicyFingerprint)
	fpField(&canonical, "created_by", entry.CreatedBy)
	fpField(&canonical, "posted_by", entry.PostedBy)
	fpField(&canonical, "reversal_reason", entry.ReversalReason)
	fpOptionalID(&canonical, "reversal_of", entry.ReversalOfID)
	fpOptionalID(&canonical, "reversed_by", entry.ReversedByID)
	fpField(&canonical, "lines", fmt.Sprint(len(entry.Lines)))

	lines := make([]string, 0, len(entry.Lines))
	for _, line := range entry.Lines {
		var encoded strings.Builder
		encoded.WriteString("line{")
		fpField(&encoded, "account", line.AccountID.String())
		fpOptionalID(&encoded, "partner", line.PartnerID)
		fpOptionalID(&encoded, "cost_center", line.CostCenterID)
		fpOptionalID(&encoded, "project", line.ProjectID)
		fpOptionalID(&encoded, "segment", line.SegmentID)
		fpOptionalID(&encoded, "business_unit", line.BusinessUnitID)
		fpField(&encoded, "matching_number", line.MatchingNumber)
		fpDecimal(&encoded, "reconciled_amount", line.ReconciledAmount)
		fpMoney(&encoded, "transaction_amount", line.TransactionAmount)
		fpField(&encoded, "transaction_currency", strings.ToUpper(strings.TrimSpace(line.TransactionCurrency)))
		fpDecimal(&encoded, "exchange_rate", line.ExchangeRate)
		fpMoney(&encoded, "functional_debit", line.FunctionalDebit)
		fpMoney(&encoded, "functional_credit", line.FunctionalCredit)
		encoded.WriteString("}")
		lines = append(lines, encoded.String())
	}
	sort.Strings(lines)
	for _, line := range lines {
		canonical.WriteString(line)
		canonical.WriteByte(';')
	}
	digest := sha256.Sum256([]byte(canonical.String()))
	return "sha256:" + fmt.Sprintf("%x", digest[:])
}

// RevaluationPayloadFingerprint is the exported v2 payload digest the host
// stamps onto the durable revaluation idempotency record. It is a thin wrapper
// around the kernel-authoritative algorithm and never redefines it.
func RevaluationPayloadFingerprint(entry journal.JournalEntry) string {
	return revaluationPayloadFingerprint(entry)
}

func (r *RevaluationRegistry) Register(key string, entry journal.JournalEntry) (journal.JournalEntry, bool, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return journal.JournalEntry{}, false, ErrInvalidPosition
	}
	payload := revaluationPayloadFingerprint(entry)
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.entries[key]; ok {
		if existing.payload != payload {
			return existing.entry.Clone(), true, ErrRevaluationAlreadyApplied
		}
		return existing.entry.Clone(), true, nil
	}
	stored := entry.Clone()
	r.entries[key] = revaluationRecord{entry: stored, payload: payload}
	return stored.Clone(), false, nil
}

func (r *RevaluationRegistry) Lookup(key string) (journal.JournalEntry, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	record, ok := r.entries[strings.TrimSpace(key)]
	if !ok {
		return journal.JournalEntry{}, false
	}
	return record.entry.Clone(), true
}

func NewRevaluationEngine(provider RateProvider) *RevaluationEngine {
	return &RevaluationEngine{provider: provider}
}

// ValidateAccountForRevaluation enforces VAL-006 / MD-004: only Asset and
// Liability accounts explicitly marked IsMonetary=true may be revalued at the
// IAS 21 closing rate; non-monetary accounts and Equity/Income/Expense fail closed.
func ValidateAccountForRevaluation(account masterdata.Account) error {
	if account.ID == (apiTypes.AccountID{}) {
		return ErrInvalidPosition
	}
	if account.Type != masterdata.AccountTypeAsset && account.Type != masterdata.AccountTypeLiability {
		return fmt.Errorf("%w: %w: account %s type %s is not eligible for FX revaluation",
			ErrInvalidAccountType, ErrInvalidPosition, account.Code, account.Type)
	}
	if account.IsMonetary == nil || !*account.IsMonetary {
		return fmt.Errorf("%w: %w: account %s is non-monetary or missing IsMonetary classification",
			ErrInvalidAccountType, ErrInvalidPosition, account.Code)
	}
	return nil
}

// CalculateForAccount validates the account against VAL-006 / MD-004 and computes
// the IAS 21 unrealized FX revaluation result for that position.
func (e *RevaluationEngine) CalculateForAccount(
	ctx context.Context,
	position MonetaryPosition,
	account masterdata.Account,
	asOf time.Time,
) (FXResult, error) {
	if position.AccountID != (apiTypes.AccountID{}) && position.AccountID != account.ID {
		return FXResult{}, ErrInvalidPosition
	}
	if err := ValidateAccountForRevaluation(account); err != nil {
		return FXResult{}, err
	}
	position.AccountID = account.ID
	position.AccountType = account.Type
	isMonetary := *account.IsMonetary
	position.IsMonetary = &isMonetary
	position.IsLiability = account.Type == masterdata.AccountTypeLiability
	return e.Calculate(ctx, position, asOf)
}

func validatePositionAccountEligibility(position MonetaryPosition) error {
	if position.IsMonetary != nil && !*position.IsMonetary {
		return fmt.Errorf("%w: %w: non-monetary item cannot be revalued", ErrInvalidAccountType, ErrInvalidPosition)
	}
	if position.AccountType == "" {
		return nil
	}
	switch position.AccountType {
	case masterdata.AccountTypeAsset:
		if position.IsLiability {
			return ErrInvalidPosition
		}
	case masterdata.AccountTypeLiability:
		if !position.IsLiability {
			return ErrInvalidPosition
		}
	default:
		return fmt.Errorf("%w: %w: account type %q cannot be revalued", ErrInvalidAccountType, ErrInvalidPosition, position.AccountType)
	}
	return nil
}

func (e *RevaluationEngine) Calculate(ctx context.Context, position MonetaryPosition, asOf time.Time) (FXResult, error) {
	if position.Type != Monetary {
		return FXResult{}, ErrInvalidPosition
	}
	if err := validatePositionAccountEligibility(position); err != nil {
		return FXResult{}, err
	}
	if asOf.IsZero() {
		return FXResult{}, ErrInvalidPosition
	}
	position.TransactionCurrency = strings.ToUpper(strings.TrimSpace(position.TransactionCurrency))
	position.FunctionalCurrency = strings.ToUpper(strings.TrimSpace(position.FunctionalCurrency))
	if position.AccountID == (apiTypes.AccountID{}) || position.TransactionCurrency == "" ||
		position.FunctionalCurrency == "" ||
		position.CarryingAmount.Currency() != position.FunctionalCurrency ||
		!position.ForeignCurrencyBalance.IsPositive() ||
		position.CarryingAmount.Amount().IsNegative() {
		return FXResult{}, ErrInvalidPosition
	}
	for _, code := range []string{position.TransactionCurrency, position.FunctionalCurrency} {
		currency, ok := types.LookupCurrency(code)
		if !ok || currency.IsSpecial() {
			return FXResult{}, types.ErrInvalidCurrency
		}
	}
	if e.provider == nil {
		return FXResult{}, ErrRateNotFound
	}

	rate, ref, inverted, err := e.provider.GetRate(ctx, ClosingRate, position.TransactionCurrency, position.FunctionalCurrency, asOf)
	if err != nil {
		return FXResult{}, err
	}
	if err := validateRevaluationRate(
		rate, ref, inverted, position.TransactionCurrency, position.FunctionalCurrency, asOf,
	); err != nil {
		return FXResult{}, err
	}
	if position.TransactionCurrency == position.FunctionalCurrency && !rate.Equal(decimal.NewFromInt(1)) {
		return FXResult{}, ErrInvalidRate
	}

	revaluedAmount, err := types.NewMoney(position.ForeignCurrencyBalance.Mul(rate), position.FunctionalCurrency)
	if err != nil {
		return FXResult{}, err
	}
	revaluedAmount, err = revaluedAmount.Rounded()
	if err != nil {
		return FXResult{}, err
	}

	diffAmt := revaluedAmount.Amount().Sub(position.CarryingAmount.Amount())
	diffMoney, err := types.NewMoney(diffAmt, position.FunctionalCurrency)
	if err != nil {
		return FXResult{}, err
	}

	isGain := diffAmt.IsPositive()
	if position.IsLiability {
		isGain = diffAmt.IsNegative()
	}

	return FXResult{
		AccountID:              position.AccountID,
		Currency:               position.TransactionCurrency,
		Difference:             diffMoney,
		IsGain:                 isGain,
		ClosingRate:            rate,
		RateDate:               ref.EffectiveDate,
		RevaluedAmount:         revaluedAmount,
		RateReference:          ref,
		RateReferenceAvailable: true,
		RateInverted:           inverted,
		Dimensions:             position.Dimensions,
	}, nil
}

func validateRevaluationRate(
	rate decimal.Decimal,
	reference ExchangeRate,
	inverted bool,
	from, to string,
	asOf time.Time,
) error {
	if !rate.IsPositive() || reference.Type != ClosingRate ||
		reference.EffectiveDate.IsZero() || reference.EffectiveDate.After(asOf) {
		return ErrInvalidRate
	}
	if err := reference.Validate(); err != nil {
		return ErrInvalidRate
	}

	expectedFrom, expectedTo := strings.ToUpper(strings.TrimSpace(from)), strings.ToUpper(strings.TrimSpace(to))
	if inverted {
		expectedFrom, expectedTo = expectedTo, expectedFrom
	}
	if strings.ToUpper(strings.TrimSpace(reference.FromCurrency)) != expectedFrom ||
		strings.ToUpper(strings.TrimSpace(reference.ToCurrency)) != expectedTo {
		return ErrInvalidRate
	}

	referencedRate := reference.Rate
	if inverted {
		referencedRate = decimal.NewFromInt(1).DivRound(referencedRate, 12)
	}
	if !rate.Equal(referencedRate) {
		return ErrInvalidRate
	}
	return nil
}

type FXJournalAccounts struct {
	CompanyID        apiTypes.CompanyID
	JournalID        apiTypes.JournalID
	UnrealizedGainID apiTypes.AccountID
	UnrealizedLossID apiTypes.AccountID
}

// valuationSystemClock is the only wall-clock boundary in this package.
// Deterministic callers must use the ...At constructors with an explicit now.
type valuationSystemClock struct{}

func (valuationSystemClock) Now() time.Time { return time.Now().UTC() }

// GenerateRevaluationEntry generates a strictly balanced double-entry for IAS 21
// FX adjustments using the package clock boundary. Deterministic callers use
// GenerateRevaluationEntryAt.
func GenerateRevaluationEntry(result FXResult, accounts FXJournalAccounts, date time.Time) (journal.JournalEntry, error) {
	return GenerateRevaluationEntryAt(result, accounts, date, valuationSystemClock{}.Now())
}

// GenerateRevaluationEntryAt generates a strictly balanced double-entry for IAS 21 FX adjustments with explicit timestamp.
func GenerateRevaluationEntryAt(result FXResult, accounts FXJournalAccounts, date, now time.Time) (journal.JournalEntry, error) {
	return GenerateRevaluationEntryAtWithOptions(result, accounts, date, now, RevaluationEntryOptions{})
}

// GenerateRevaluationEntryAtWithOptions generates a strictly balanced double-entry
// for IAS 21 FX adjustments with explicit timestamp, adjusting metadata, actor, and dimensions.
func GenerateRevaluationEntryAtWithOptions(
	result FXResult,
	accounts FXJournalAccounts,
	date, now time.Time,
	opts RevaluationEntryOptions,
) (journal.JournalEntry, error) {
	if result.Difference.Amount().IsZero() {
		return journal.JournalEntry{}, fmt.Errorf("%w: %w", ErrZeroRevaluationDifference, ErrInvalidPosition)
	}
	if date.IsZero() || now.IsZero() ||
		accounts.CompanyID == (apiTypes.CompanyID{}) ||
		accounts.JournalID == (apiTypes.JournalID{}) ||
		accounts.UnrealizedGainID == (apiTypes.AccountID{}) ||
		accounts.UnrealizedLossID == (apiTypes.AccountID{}) ||
		result.AccountID == (apiTypes.AccountID{}) {
		return journal.JournalEntry{}, ErrInvalidPosition
	}
	currency := strings.ToUpper(strings.TrimSpace(result.Difference.Currency()))
	if currencyInfo, ok := types.LookupCurrency(currency); !ok || currencyInfo.IsSpecial() {
		return journal.JournalEntry{}, types.ErrInvalidCurrency
	}
	adjType := opts.AdjustingEntryType
	if adjType == "" {
		adjType = journal.AdjustingEntryAccrual
	}
	targetPeriod := 0
	switch adjType {
	case journal.AdjustingEntryAccrual,
		journal.AdjustingEntryDeferral,
		journal.AdjustingEntryProvisionEstimate,
		journal.AdjustingEntryReclassification:
	case journal.AdjustingEntryAudit, journal.AdjustingEntryAuditLegacy:
		targetPeriod = 14
	default:
		return journal.JournalEntry{}, ErrInvalidAdjustingEntryMetadata
	}
	createdBy := strings.TrimSpace(opts.CreatedBy)
	if createdBy == "" {
		createdBy = "system-valuation"
	}
	dims := result.Dimensions
	if opts.Dimensions != nil {
		dims = *opts.Dimensions
	}

	absDiff := result.Difference.Amount().Abs()
	funcAmt, err := apiTypes.NewMoney(absDiff, currency)
	if err != nil {
		return journal.JournalEntry{}, err
	}
	zeroAmt, err := apiTypes.Zero(currency)
	if err != nil {
		return journal.JournalEntry{}, err
	}
	referenceID, err := RevaluationReferenceID(result, date)
	if err != nil {
		return journal.JournalEntry{}, err
	}

	now = now.UTC()
	entry := journal.JournalEntry{
		ID:                  apiTypes.NewJournalEntryID(),
		CompanyID:           accounts.CompanyID,
		JournalID:           accounts.JournalID,
		EffectiveDate:       date.UTC(),
		PostingDate:         date.UTC(),
		Status:              journal.EntryDraft,
		Classification:      journal.EntryClassAdjusting,
		AdjustingEntryType:  adjType,
		TargetPeriodNumber:  targetPeriod,
		AdjustmentReason:    fmt.Sprintf("IAS 21 FX Revaluation for %s", result.Currency),
		ReferenceID:         referenceID,
		TransactionCurrency: currency,
		ExchangeRate:        decimal.NewFromInt(1),
		CreatedBy:           createdBy,
		CreatedAt:           now,
		UpdatedAt:           now,
	}

	var positionLine, pnlLine journal.EntryLine

	if result.IsGain {
		// Gain: Asset revalued upward -> Debit Asset, Credit Gain OR Liability revalued downward -> Debit Liability, Credit Gain
		positionLine = journal.EntryLine{
			ID:                  apiTypes.NewJournalEntryLineID(),
			AccountID:           result.AccountID,
			TransactionCurrency: result.Difference.Currency(),
			TransactionAmount:   funcAmt,
			ExchangeRate:        decimal.NewFromInt(1),
			FunctionalDebit:     funcAmt,
			FunctionalCredit:    zeroAmt,
		}
		pnlLine = journal.EntryLine{
			ID:                  apiTypes.NewJournalEntryLineID(),
			AccountID:           accounts.UnrealizedGainID,
			TransactionCurrency: result.Difference.Currency(),
			TransactionAmount:   funcAmt,
			ExchangeRate:        decimal.NewFromInt(1),
			FunctionalDebit:     zeroAmt,
			FunctionalCredit:    funcAmt,
		}
	} else {
		// Loss: Debit Loss, Credit Asset/Liability
		pnlLine = journal.EntryLine{
			ID:                  apiTypes.NewJournalEntryLineID(),
			AccountID:           accounts.UnrealizedLossID,
			TransactionCurrency: result.Difference.Currency(),
			TransactionAmount:   funcAmt,
			ExchangeRate:        decimal.NewFromInt(1),
			FunctionalDebit:     funcAmt,
			FunctionalCredit:    zeroAmt,
		}
		positionLine = journal.EntryLine{
			ID:                  apiTypes.NewJournalEntryLineID(),
			AccountID:           result.AccountID,
			TransactionCurrency: result.Difference.Currency(),
			TransactionAmount:   funcAmt,
			ExchangeRate:        decimal.NewFromInt(1),
			FunctionalDebit:     zeroAmt,
			FunctionalCredit:    funcAmt,
		}
	}
	positionLine.PartnerID = cloneDimPtr(dims.PartnerID)
	positionLine.CostCenterID = cloneDimPtr(dims.CostCenterID)
	positionLine.ProjectID = cloneDimPtr(dims.ProjectID)
	positionLine.SegmentID = cloneDimPtr(dims.SegmentID)
	positionLine.BusinessUnitID = cloneDimPtr(dims.BusinessUnitID)
	pnlLine.CostCenterID = cloneDimPtr(dims.CostCenterID)
	pnlLine.ProjectID = cloneDimPtr(dims.ProjectID)
	pnlLine.SegmentID = cloneDimPtr(dims.SegmentID)
	pnlLine.BusinessUnitID = cloneDimPtr(dims.BusinessUnitID)

	entry.Lines = []journal.EntryLine{positionLine, pnlLine}

	return entry, nil
}

func cloneDimPtr[T any](v *T) *T {
	if v == nil {
		return nil
	}
	copied := *v
	return &copied
}

// BuildRevaluationReversalWith creates the exact reversal of a posted revaluation
// entry using the injected ReversalBuilder and assigns it to the caller's explicit reversal date.
func BuildRevaluationReversalWith(
	builder ReversalBuilder,
	original journal.JournalEntry,
	reversalDate time.Time,
	actorID, reason string,
	now time.Time,
) (journal.JournalEntry, error) {
	if builder == nil {
		return journal.JournalEntry{}, ErrReversalBuilderRequired
	}
	return builder.BuildReversalAt(original, reversalDate, actorID, reason, now)
}

// BuildRevaluationReversalAt creates the exact reversal of a posted revaluation
// entry using the engine's configured ReversalBuilder.
func (e *RevaluationEngine) BuildRevaluationReversalAt(
	original journal.JournalEntry,
	reversalDate time.Time,
	actorID, reason string,
	now time.Time,
) (journal.JournalEntry, error) {
	if e == nil {
		return journal.JournalEntry{}, ErrReversalBuilderRequired
	}
	return BuildRevaluationReversalWith(e.reversalBuilder, original, reversalDate, actorID, reason, now)
}
