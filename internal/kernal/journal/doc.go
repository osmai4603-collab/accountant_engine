// Package journal defines the canonical journal-entry aggregate and its
// lifecycle rules.
//
// This package owns the shape of a journal entry: EntryLine and JournalEntry
// with statuses (draft, posted, reversed), classifications, adjustment metadata,
// effect dates, posting attribution and reversal links (ReversalOfID,
// ReversedByID, ReversalReason). It also defines the domain state machine
// (draft->posted->reversed), immutability guards for posted/reversed facts, and
// reversal linkage validation helpers.
//
// Boundaries: journal performs no balance validation and no persistence.
// Validation and posting orchestration live in internal/kernal/posting
// (PostingPipeline.PostEntry); journal entry definitions are consumed by policy,
// valuation, and ledger projections. Posted entries with their lines are the
// canonical accounting facts of the kernel — see
// docs/ard/P1_02_gl_source_of_truth.md. Corrections are represented by linked
// reversal/correction entries; posted facts are not updated or deleted in place.
//
// Import charter: Go standard library, github.com/shopspring/decimal and sibling
// kernel modules only. Never import storage, network, pkg/*, orbitals/* or modules/*.
package journal
