package storage

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
)

// This file is the adapter-neutral locking contract of Phase 8. The kernel
// produces the order (internal/kernel/posting/lock_manager.go); an adapter
// acquires the identities inside its own transaction. Nothing here imports a
// driver, SQL, or a concrete storage package.

// ErrInvalidLockTarget reports a lock identity without a complete company and
// resource id. It is a programming error, never retryable.
var ErrInvalidLockTarget = errors.New("storage: incomplete lock target")

// LockKind identifies the resource class protected by one lock target. The
// textual value is part of the canonical key and must stay stable.
type LockKind string

const (
	LockKindAccount     LockKind = "account"
	LockKindEntry       LockKind = "entry"
	LockKindOriginal    LockKind = "original"
	LockKindIdempotency LockKind = "idempotency"
)

func (kind LockKind) valid() bool {
	switch kind {
	case LockKindAccount, LockKindEntry, LockKindOriginal, LockKindIdempotency:
		return true
	default:
		return false
	}
}

// LockTarget is one company-scoped resource to acquire before a posting
// becomes visible. ID is the stable identifier within Kind: account ID, entry
// ID, original entry ID, or the canonical idempotency key/scope.
type LockTarget struct {
	Kind      LockKind
	CompanyID types.CompanyID
	ID        string
}

func (target LockTarget) Validate() error {
	if !target.Kind.valid() || target.CompanyID == (types.CompanyID{}) || strings.TrimSpace(target.ID) == "" {
		return fmt.Errorf("%w: kind=%q company=%s id=%q", ErrInvalidLockTarget, target.Kind, target.CompanyID.String(), target.ID)
	}
	return nil
}

// Canonical is the byte-stable lock identity used for both sorting and actual
// acquisition. Adapters must hash or name this exact string, never a map
// iteration or the caller's line order.
func (target LockTarget) Canonical() string {
	return fmt.Sprintf("v1|company=%s|kind=%s|id=%s", target.CompanyID.String(), target.Kind, target.ID)
}

// OrderLockTargets returns a fresh, deduplicated, strictly ascending copy of
// targets. It never mutates the caller's slice and never reorders by map.
func OrderLockTargets(targets []LockTarget) []LockTarget {
	ordered := make([]LockTarget, 0, len(targets))
	seen := make(map[string]struct{}, len(targets))
	for _, target := range targets {
		canonical := target.Canonical()
		if _, duplicate := seen[canonical]; duplicate {
			continue
		}
		seen[canonical] = struct{}{}
		ordered = append(ordered, target)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Canonical() < ordered[j].Canonical() })
	return ordered
}

// PostingLockTargets derives the complete lock set of one posting commit from
// kernel-stamped fields. A reversal always locks the original before writing;
// an idempotency key always locks its full scope rather than the bare text.
func PostingLockTargets(commit PostingCommit) []LockTarget {
	companyID := commit.Entry.CompanyID
	if companyID == (types.CompanyID{}) {
		return nil
	}
	targets := make([]LockTarget, 0, len(commit.Entry.Lines)+3)
	for _, line := range commit.Entry.Lines {
		if line.AccountID == (types.AccountID{}) {
			continue
		}
		targets = append(targets, LockTarget{Kind: LockKindAccount, CompanyID: companyID, ID: line.AccountID.String()})
	}
	if commit.Entry.ID != (types.JournalEntryID{}) {
		targets = append(targets, LockTarget{Kind: LockKindEntry, CompanyID: companyID, ID: commit.Entry.ID.String()})
	}
	if commit.OriginalEntryID != nil && *commit.OriginalEntryID != (types.JournalEntryID{}) {
		targets = append(targets, LockTarget{Kind: LockKindOriginal, CompanyID: companyID, ID: commit.OriginalEntryID.String()})
	}
	if commit.Idempotency != nil && strings.TrimSpace(commit.Idempotency.Key) != "" {
		identity := commit.Idempotency.Key
		if commit.Idempotency.Scope != "" {
			identity = commit.Idempotency.Scope
		}
		targets = append(targets, LockTarget{Kind: LockKindIdempotency, CompanyID: companyID, ID: identity})
	}
	return OrderLockTargets(targets)
}

// LockObserver is an optional test hook. Production callers do not install
// one; contract tests use it to prove that an adapter acquired the canonical
// order instead of merely sorting a slice and ignoring it.
type LockObserver interface {
	Acquired(target LockTarget)
	Released(target LockTarget)
}

type lockObserverContextKey struct{}

func WithLockObserver(ctx context.Context, observer LockObserver) context.Context {
	if ctx == nil || observer == nil {
		return ctx
	}
	return context.WithValue(ctx, lockObserverContextKey{}, observer)
}

func lockObserverFromContext(ctx context.Context) LockObserver {
	if ctx == nil {
		return nil
	}
	observer, _ := ctx.Value(lockObserverContextKey{}).(LockObserver)
	return observer
}

// LockAcquireFunc acquires one target and returns its release function. For
// transaction-scoped database locks the release may be a no-op because the
// commit or rollback releases them; the observer still records each step.
type LockAcquireFunc func(ctx context.Context, target LockTarget) (release func(), err error)

// WithOrderedLocks acquires every target in canonical order and releases in
// reverse order. If acquisition fails, everything already acquired is released
// before the error is returned and fn is never called.
func WithOrderedLocks(ctx context.Context, targets []LockTarget, acquire LockAcquireFunc, fn func() error) (err error) {
	if ctx == nil {
		return errors.New("storage: lock context is nil")
	}
	if acquire == nil {
		return errors.New("storage: lock acquirer is nil")
	}
	if fn == nil {
		return errors.New("storage: locked callback is nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	ordered := OrderLockTargets(targets)
	releases := make([]func(), 0, len(ordered))
	observer := lockObserverFromContext(ctx)
	defer func() {
		for i := len(releases) - 1; i >= 0; i-- {
			if releases[i] != nil {
				releases[i]()
			}
			if observer != nil && i < len(ordered) {
				observer.Released(ordered[i])
			}
		}
	}()

	for _, target := range ordered {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := target.Validate(); err != nil {
			return err
		}
		release, err := acquire(ctx, target)
		if err != nil {
			return err
		}
		releases = append(releases, release)
		if observer != nil {
			observer.Acquired(target)
		}
	}
	return fn()
}

// ValidateIdempotencyScope rejects a scope whose company, operation, or key
// disagrees with the entry being committed. The canonical format is produced
// by the kernel; older callers without a scope retain the historical company
// column isolation.
func ValidateIdempotencyScope(companyID types.CompanyID, operation PostingOperation, idempotency *PostingIdempotency) error {
	if idempotency == nil || idempotency.Scope == "" {
		return nil
	}
	want := fmt.Sprintf("company=%s|operation=%s|key=%s", companyID.String(), operation, strings.TrimSpace(idempotency.Key))
	if idempotency.Scope != want {
		return fmt.Errorf("%w: scope %q does not match company %s operation %s", ErrScopeViolation, idempotency.Scope, companyID, operation)
	}
	return nil
}
