package posting

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/osmai4603-collab/accounting_engine/internal/kernal/journal"
	"github.com/osmai4603-collab/accounting_engine/internal/kernal/types"
)

// IdempotencyOperation is the kernel-level kind of an atomic operation. It is
// part of the uniqueness scope: a posting key and a reversal key with the same
// text are different operations (RULE-KERNEL-027).
type IdempotencyOperation string

const (
	IdempotencyPosting     IdempotencyOperation = "post"
	IdempotencyReversal    IdempotencyOperation = "reverse"
	IdempotencyRevaluation IdempotencyOperation = "revaluation"
)

func (operation IdempotencyOperation) Valid() bool {
	switch operation {
	case IdempotencyPosting, IdempotencyReversal, IdempotencyRevaluation:
		return true
	default:
		return false
	}
}

// IdempotencyScope is the complete uniqueness scope of one posting request.
// The host executor must treat the triple, not the bare key, as unique.
type IdempotencyScope struct {
	CompanyID types.CompanyID
	Operation IdempotencyOperation
	Key       string
}

var (
	ErrIdempotencyScopeRequired      = errors.New("complete idempotency scope is required")
	ErrIdempotencyConflict           = errors.New("idempotency key was reused with a different payload")
	ErrIdempotencyInProgress         = errors.New("idempotency operation is still in progress")
	ErrIdempotencyStorageUnavailable = errors.New("idempotency storage is unavailable")
	ErrConcurrencyConflict           = errors.New("concurrent modification conflict")
	ErrReversalAlreadyApplied        = errors.New("entry has already been reversed")
	// ErrRevaluationAlreadyApplied is an operation-level payload conflict: the
	// revaluation key was reused with a different stable result. Retrying cannot
	// change that outcome, so it is non-retryable like ErrIdempotencyConflict.
	ErrRevaluationAlreadyApplied = errors.New("revaluation has already been applied")
	ErrAtomicityViolation        = errors.New("atomic posting contract was violated")
	ErrUnknownOperationOutcome   = errors.New("operation outcome is unknown; query by idempotency key")
)

// RetryableError marks an error whose operation may be retried safely because
// the idempotency contract makes the retry a no-op or a completed replay.
type RetryableError struct {
	Err    error
	Reason string
}

func (e *RetryableError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Err == nil {
		return e.Reason
	}
	if e.Reason == "" {
		return e.Err.Error()
	}
	return fmt.Sprintf("%s: %s", e.Err.Error(), e.Reason)
}

func (e *RetryableError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// Retryable marks the error as safe for a host-controlled bounded retry.
func (e *RetryableError) Retryable() bool { return true }

// IsRetryable classifies an error according to the phase-7 contract. Payload
// conflicts (including revaluation result conflicts), already-applied
// reversals and atomicity violations are non-retryable: a retry cannot change
// their outcome.
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	var classifier interface{ Retryable() bool }
	if errors.As(err, &classifier) {
		return classifier.Retryable()
	}
	return errors.Is(err, ErrIdempotencyInProgress) ||
		errors.Is(err, ErrIdempotencyStorageUnavailable) ||
		errors.Is(err, ErrConcurrencyConflict) ||
		errors.Is(err, context.DeadlineExceeded)
}

func (scope IdempotencyScope) Validate() error {
	if scope.CompanyID == (types.CompanyID{}) ||
		!scope.Operation.Valid() ||
		strings.TrimSpace(scope.Key) == "" {
		return ErrIdempotencyScopeRequired
	}
	return nil
}

// String is the canonical, log-safe representation of the uniqueness scope.
func (scope IdempotencyScope) String() string {
	return fmt.Sprintf("company=%s|operation=%s|key=%s",
		scope.CompanyID.String(), scope.Operation, strings.TrimSpace(scope.Key))
}

// BuildIdempotencyScope derives the kernel-authoritative scope from the entry
// and caller options. Caller-supplied Scope values are never trusted, except
// for the validated revaluation hint: it may select the third operation, but
// the company always comes from the entry and the key from opts.IdempotencyKey.
// A hint that disagrees with either is rejected instead of being ignored.
func BuildIdempotencyScope(entry journal.JournalEntry, opts PostOptions) (IdempotencyScope, error) {
	operation := IdempotencyPosting
	if entry.ReversalOfID != nil {
		operation = IdempotencyReversal
	}
	key := strings.TrimSpace(opts.IdempotencyKey)
	if opts.Scope.Operation == IdempotencyRevaluation {
		if opts.Scope.CompanyID != entry.CompanyID || strings.TrimSpace(opts.Scope.Key) != key {
			return IdempotencyScope{}, ErrIdempotencyScopeRequired
		}
		operation = IdempotencyRevaluation
	}
	scope := IdempotencyScope{
		CompanyID: entry.CompanyID,
		Operation: operation,
		Key:       key,
	}
	if err := scope.Validate(); err != nil {
		return IdempotencyScope{}, err
	}
	return scope, nil
}

// FingerprintRequest returns a deterministic digest of the semantic payload
// guarded by scope. Transport metadata (correlation ID) and the idempotency key
// itself are deliberately excluded: the key identifies the operation, the
// digest detects payload reuse with that key.
//
// All fields are encoded explicitly, timestamps are UTC, decimals use their
// canonical string form, and lines are sorted by their stable ID so map or
// slice ordering in a caller cannot change the digest.
func FingerprintRequest(entry journal.JournalEntry, opts PostOptions, scope IdempotencyScope) (string, error) {
	if err := scope.Validate(); err != nil {
		return "", err
	}
	if err := validateCanonicalEntryFields(entry, opts, scope); err != nil {
		return "", err
	}

	var canonical strings.Builder
	canonical.WriteString("posting-request-fingerprint-v1|")
	field(&canonical, "company=%s|", scope.CompanyID.String())
	field(&canonical, "operation=%s|", scope.Operation)

	field(&canonical, "entry.id=%s|", entry.ID.String())
	field(&canonical, "entry.company=%s|", entry.CompanyID.String())
	field(&canonical, "entry.journal=%s|", entry.JournalID.String())
	field(&canonical, "entry.period=%s|", entry.FiscalPeriodID.String())
	field(&canonical, "entry.number=%s|", entry.EntryNumber)
	field(&canonical, "entry.sequence=%d|", entry.SequenceNumber)
	field(&canonical, "entry.legal_number=%s|", entry.LegalNumber)
	field(&canonical, "entry.status=%s|", entry.Status)
	field(&canonical, "entry.classification=%s|", entry.Classification)
	field(&canonical, "entry.adjusting_type=%s|", entry.AdjustingEntryType)
	field(&canonical, "entry.adjustment_reason=%s|", entry.AdjustmentReason)
	field(&canonical, "entry.target_period=%d|", entry.TargetPeriodNumber)
	field(&canonical, "entry.effective_date=%s|", fingerprintTime(entry.EffectiveDate))
	field(&canonical, "entry.posting_date=%s|", fingerprintTime(entry.PostingDate))
	field(&canonical, "entry.description=%s|", entry.Description)
	field(&canonical, "entry.source_module=%s|", entry.SourceModule)
	field(&canonical, "entry.reference_id=%s|", entry.ReferenceID)
	field(&canonical, "entry.transaction_currency=%s|", strings.ToUpper(strings.TrimSpace(entry.TransactionCurrency)))
	field(&canonical, "entry.exchange_rate=%s|", entry.ExchangeRate.String())
	field(&canonical, "entry.policy_fingerprint=%s|", entry.PolicyFingerprint)
	field(&canonical, "entry.created_by=%s|", entry.CreatedBy)
	field(&canonical, "entry.posted_by=%s|", strings.TrimSpace(opts.ActorID))
	field(&canonical, "entry.reason=%s|", strings.TrimSpace(opts.Reason))
	field(&canonical, "entry.posted_at=%s|", fingerprintTimePtr(entry.PostedAt))
	field(&canonical, "entry.reversal_of=%s|", fingerprintIDPtr(entry.ReversalOfID))
	field(&canonical, "entry.reversed_by=%s|", fingerprintIDPtr(entry.ReversedByID))
	field(&canonical, "entry.reversal_reason=%s|", entry.ReversalReason)

	lines := append([]journal.EntryLine(nil), entry.Lines...)
	sort.SliceStable(lines, func(i, j int) bool {
		return lines[i].ID.String() < lines[j].ID.String()
	})
	field(&canonical, "entry.lines=%d|", len(lines))
	for i, line := range lines {
		prefix := fmt.Sprintf("line[%d].", i)
		field(&canonical, prefix+"id=%s|", line.ID.String())
		field(&canonical, prefix+"account=%s|", line.AccountID.String())
		field(&canonical, prefix+"partner=%s|", fingerprintIDPtr(line.PartnerID))
		field(&canonical, prefix+"cost_center=%s|", fingerprintIDPtr(line.CostCenterID))
		field(&canonical, prefix+"project=%s|", fingerprintIDPtr(line.ProjectID))
		field(&canonical, prefix+"segment=%s|", fingerprintIDPtr(line.SegmentID))
		field(&canonical, prefix+"business_unit=%s|", fingerprintIDPtr(line.BusinessUnitID))
		field(&canonical, prefix+"matching=%s|", line.MatchingNumber)
		field(&canonical, prefix+"reconciled=%s|", line.ReconciledAmount.String())
		field(&canonical, prefix+"transaction_amount=%s|", line.TransactionAmount.Amount().String())
		field(&canonical, prefix+"transaction_currency=%s|", strings.ToUpper(strings.TrimSpace(line.TransactionCurrency)))
		field(&canonical, prefix+"exchange_rate=%s|", line.ExchangeRate.String())
		field(&canonical, prefix+"functional_debit=%s|", line.FunctionalDebit.Amount().String())
		field(&canonical, prefix+"functional_debit_currency=%s|", line.FunctionalDebit.Currency())
		field(&canonical, prefix+"functional_credit=%s|", line.FunctionalCredit.Amount().String())
		field(&canonical, prefix+"functional_credit_currency=%s|", line.FunctionalCredit.Currency())
	}

	digest := sha256.Sum256([]byte(canonical.String()))
	return "sha256:" + fmt.Sprintf("%x", digest[:]), nil
}

// FingerprintCallerPayload hashes only caller-controlled fields. Unlike
// FingerprintRequest it is stable across the posting transformation, so the
// application layer can use it for replay lookup and conflict detection both
// before and after an entry is posted.
func FingerprintCallerPayload(entry journal.JournalEntry, opts PostOptions, scope IdempotencyScope) (string, error) {
	if err := scope.Validate(); err != nil {
		return "", err
	}
	if err := validateCanonicalEntryFields(entry, opts, scope); err != nil {
		return "", err
	}
	var canonical strings.Builder
	canonical.WriteString("posting-caller-payload-v1|")
	field(&canonical, "company=%s|", scope.CompanyID.String())
	field(&canonical, "operation=%s|", scope.Operation)
	field(&canonical, "entry.id=%s|", entry.ID.String())
	field(&canonical, "entry.company=%s|", entry.CompanyID.String())
	field(&canonical, "entry.journal=%s|", entry.JournalID.String())
	field(&canonical, "entry.classification=%s|", entry.Classification)
	field(&canonical, "entry.adjusting_type=%s|", entry.AdjustingEntryType)
	field(&canonical, "entry.adjustment_reason=%s|", entry.AdjustmentReason)
	field(&canonical, "entry.target_period=%d|", entry.TargetPeriodNumber)
	field(&canonical, "entry.effective_date=%s|", fingerprintTime(entry.EffectiveDate))
	field(&canonical, "entry.description=%s|", entry.Description)
	field(&canonical, "entry.source_module=%s|", entry.SourceModule)
	field(&canonical, "entry.reference_id=%s|", entry.ReferenceID)
	field(&canonical, "entry.transaction_currency=%s|", strings.ToUpper(strings.TrimSpace(entry.TransactionCurrency)))
	field(&canonical, "entry.reversal_of=%s|", fingerprintIDPtr(entry.ReversalOfID))
	field(&canonical, "entry.reversal_reason=%s|", entry.ReversalReason)
	field(&canonical, "actor=%s|", strings.TrimSpace(opts.ActorID))
	field(&canonical, "reason=%s|", strings.TrimSpace(opts.Reason))

	lines := append([]journal.EntryLine(nil), entry.Lines...)
	sort.SliceStable(lines, func(i, j int) bool {
		return lines[i].ID.String() < lines[j].ID.String()
	})
	field(&canonical, "entry.lines=%d|", len(lines))
	for i, line := range lines {
		prefix := fmt.Sprintf("line[%d].", i)
		field(&canonical, prefix+"id=%s|", line.ID.String())
		field(&canonical, prefix+"account=%s|", line.AccountID.String())
		field(&canonical, prefix+"partner=%s|", fingerprintIDPtr(line.PartnerID))
		field(&canonical, prefix+"cost_center=%s|", fingerprintIDPtr(line.CostCenterID))
		field(&canonical, prefix+"project=%s|", fingerprintIDPtr(line.ProjectID))
		field(&canonical, prefix+"segment=%s|", fingerprintIDPtr(line.SegmentID))
		field(&canonical, prefix+"business_unit=%s|", fingerprintIDPtr(line.BusinessUnitID))
		field(&canonical, prefix+"matching=%s|", line.MatchingNumber)
		field(&canonical, prefix+"reconciled=%s|", line.ReconciledAmount.String())
		field(&canonical, prefix+"transaction_amount=%s|", line.TransactionAmount.Amount().String())
		field(&canonical, prefix+"transaction_currency=%s|", strings.ToUpper(strings.TrimSpace(line.TransactionCurrency)))
		field(&canonical, prefix+"exchange_rate=%s|", line.ExchangeRate.String())
		field(&canonical, prefix+"functional_debit=%s|", line.FunctionalDebit.Amount().String())
		field(&canonical, prefix+"functional_credit=%s|", line.FunctionalCredit.Amount().String())
	}
	digest := sha256.Sum256([]byte(canonical.String()))
	return "sha256:" + fmt.Sprintf("%x", digest[:]), nil
}

func field(builder *strings.Builder, format string, args ...any) {
	builder.WriteString(fmt.Sprintf(format, args...))
}

func fingerprintTime(value time.Time) string {
	if value.IsZero() {
		return "-"
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func fingerprintTimePtr(value *time.Time) string {
	if value == nil {
		return "-"
	}
	return fingerprintTime(*value)
}

func fingerprintIDPtr[T interface{ String() string }](value *T) string {
	if value == nil {
		return "-"
	}
	return (*value).String()
}

func validateCanonicalEntryFields(entry journal.JournalEntry, opts PostOptions, scope IdempotencyScope) error {
	for _, item := range []struct {
		name  string
		value string
	}{
		{"scope.operation", string(scope.Operation)},
		{"entry.entry_number", entry.EntryNumber},
		{"entry.legal_number", entry.LegalNumber},
		{"entry.status", string(entry.Status)},
		{"entry.classification", string(entry.Classification)},
		{"entry.adjusting_entry_type", string(entry.AdjustingEntryType)},
		{"entry.adjustment_reason", entry.AdjustmentReason},
		{"entry.description", entry.Description},
		{"entry.source_module", entry.SourceModule},
		{"entry.reference_id", entry.ReferenceID},
		{"entry.transaction_currency", entry.TransactionCurrency},
		{"entry.policy_fingerprint", entry.PolicyFingerprint},
		{"entry.created_by", entry.CreatedBy},
		{"entry.reversal_reason", entry.ReversalReason},
		{"opts.actor_id", opts.ActorID},
		{"opts.reason", opts.Reason},
	} {
		if strings.ContainsAny(item.value, "|\r\n\x00") {
			return fmt.Errorf("%w: %s contains reserved delimiter or control character", ErrInvalidEntry, item.name)
		}
	}
	for i, line := range entry.Lines {
		for _, item := range []struct {
			name  string
			value string
		}{
			{"matching_number", line.MatchingNumber},
			{"transaction_currency", line.TransactionCurrency},
			{"functional_debit_currency", line.FunctionalDebit.Currency()},
			{"functional_credit_currency", line.FunctionalCredit.Currency()},
		} {
			if strings.ContainsAny(item.value, "|\r\n\x00") {
				return fmt.Errorf("%w: line[%d].%s contains reserved delimiter or control character", ErrInvalidEntry, i, item.name)
			}
		}
	}
	return nil
}
