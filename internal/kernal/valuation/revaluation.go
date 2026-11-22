package valuation

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/osmai4603-collab/accounting_engine/internal/kernal/journal"
	"github.com/osmai4603-collab/accounting_engine/internal/kernal/posting"
	"github.com/osmai4603-collab/accounting_engine/internal/kernal/types"
	apiTypes "github.com/osmai4603-collab/accounting_engine/internal/kernal/types"
	"github.com/shopspring/decimal"
)

var ErrInvalidPosition = errors.New("invalid monetary position")

type PositionType string

const (
	Monetary    PositionType = "MONETARY"
	NonMonetary PositionType = "NON_MONETARY"
)

type MonetaryPosition struct {
	AccountID              apiTypes.AccountID
	ForeignCurrencyBalance decimal.Decimal
	TransactionCurrency    string
	FunctionalCurrency     string
	CarryingAmount         types.Money
	Type                   PositionType
	IsLiability            bool
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
}

type RateProvider interface {
	GetRate(ctx context.Context, rateType RateType, from, to string, asOf time.Time) (decimal.Decimal, ExchangeRate, bool, error)
}

type RevaluationEngine struct {
	provider RateProvider
}

// ErrRevaluationAlreadyApplied is the valuation-facing alias of the phase-7
// error catalog entry; a conflicting revaluation payload is terminal, not
// retryable. Keeping one sentinel preserves errors.Is across both packages.
var ErrRevaluationAlreadyApplied = posting.ErrRevaluationAlreadyApplied

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
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if company == (apiTypes.CompanyID{}) || account == (apiTypes.AccountID{}) || date.IsZero() {
		return "", ErrInvalidPosition
	}
	if _, ok := types.LookupCurrency(currency); !ok {
		return "", types.ErrInvalidCurrency
	}
	if currency == string(types.XXX) {
		return "", types.ErrInvalidCurrency
	}
	return fmt.Sprintf("fx-revaluation|company=%s|account=%s|currency=%s|date=%s",
		company.String(), account.String(), currency, date.UTC().Format("2006-01-02")), nil
}

// RevaluationIdempotencyScope maps the deterministic revaluation operation key
// onto the neutral phase-7 idempotency contract. The returned scope is
// kernel-authoritative and is carried by PostOptions.Scope into
// BuildIdempotencyScope and the durable storage SPI (G-024); the host stamps
// RevaluationPayloadFingerprint as the payload hash. PostEntry itself is not
// extended with a revaluation-specific parameter.
func RevaluationIdempotencyScope(company apiTypes.CompanyID, account apiTypes.AccountID, currency string, date time.Time) (posting.IdempotencyScope, error) {
	key, err := RevaluationOperationKey(company, account, currency, date)
	if err != nil {
		return posting.IdempotencyScope{}, err
	}
	scope := posting.IdempotencyScope{
		CompanyID: company,
		Operation: posting.IdempotencyRevaluation,
		Key:       key,
	}
	if err := scope.Validate(); err != nil {
		return posting.IdempotencyScope{}, err
	}
	return scope, nil
}

func RevaluationReferenceID(result FXResult, date time.Time) (string, error) {
	currency := strings.ToUpper(strings.TrimSpace(result.Currency))
	if result.AccountID == (apiTypes.AccountID{}) || date.IsZero() {
		return "", ErrInvalidPosition
	}
	if _, ok := types.LookupCurrency(currency); !ok {
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

func (e *RevaluationEngine) Calculate(ctx context.Context, position MonetaryPosition, asOf time.Time) (FXResult, error) {
	if position.Type != Monetary {
		return FXResult{}, ErrInvalidPosition
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
		!position.CarryingAmount.Amount().IsPositive() {
		return FXResult{}, ErrInvalidPosition
	}
	for _, code := range []string{position.TransactionCurrency, position.FunctionalCurrency} {
		currency, ok := types.LookupCurrency(code)
		if !ok || currency.Code == types.XXX {
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
	if result.Difference.Amount().IsZero() {
		return journal.JournalEntry{}, ErrInvalidPosition
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
	if _, ok := types.LookupCurrency(currency); !ok || currency == string(types.XXX) {
		return journal.JournalEntry{}, types.ErrInvalidCurrency
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
		AdjustingEntryType:  journal.AdjustingEntryAccrual,
		AdjustmentReason:    fmt.Sprintf("IAS 21 FX Revaluation for %s", result.Currency),
		ReferenceID:         referenceID,
		TransactionCurrency: currency,
		ExchangeRate:        decimal.NewFromInt(1),
		CreatedBy:           "system-valuation",
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

	entry.Lines = []journal.EntryLine{positionLine, pnlLine}

	if err := posting.ValidateEntryDoubleEntry(&entry, nil); err != nil {
		return journal.JournalEntry{}, err
	}

	return entry, nil
}

// BuildRevaluationReversalAt creates the exact reversal of a posted revaluation
// entry and assigns it to the caller's explicit reversal date.
func BuildRevaluationReversalAt(
	original journal.JournalEntry,
	reversalDate time.Time,
	actorID, reason string,
	now time.Time,
) (journal.JournalEntry, error) {
	return posting.BuildReversalEntryAt(original, reversalDate, actorID, reason, now)
}
