package storage

import "errors"

// Adapter-neutral outcome errors. Adapters return these sentinels so contract
// tests and the SDK can classify an outcome without knowing SQL, driver codes
// or process-local implementation details.
var (
	// ErrIdempotencyPayloadConflict means the key was already used for a
	// different semantic payload. It is non-retryable: retrying cannot change
	// the payload and must not create a second accounting effect.
	ErrIdempotencyPayloadConflict = errors.New("storage: idempotency key reused with a different payload")

	// ErrIdempotencyInProgress means another execution currently owns the key.
	// It is retryable only under a bounded host policy after safe key lookup.
	ErrIdempotencyInProgress = errors.New("storage: idempotency operation in progress")

	// ErrIdempotencyUnavailable means the durable key store could not be
	// reached; the operation must not proceed without it.
	ErrIdempotencyUnavailable = errors.New("storage: idempotency store unavailable")

	// ErrEntryAlreadyReversed means the original entry is already linked to a
	// reversal. Reversal is single-winner and non-retryable with a new entry.
	ErrEntryAlreadyReversed = errors.New("storage: entry already reversed")

	// ErrConcurrentModification is a retryable serialization/conflict failure.
	ErrConcurrentModification = errors.New("storage: concurrent modification conflict")

	// ErrAtomicCommitFailed reports a broken atomicity contract. It is never
	// retryable until the adapter is fixed.
	ErrAtomicCommitFailed = errors.New("storage: atomic posting contract violated")

	// ErrScopeViolation means a command or query crossed the trusted company
	// boundary (entry company, idempotency scope, or adapter tenant context).
	// It is non-retryable: a retry with the same scope cannot become valid.
	ErrScopeViolation = errors.New("storage: company scope violation")
)
