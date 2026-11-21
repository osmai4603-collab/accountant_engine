// Package posting: Atomic posting pipeline, double-entry validation, reversals and ordered account locks.
//
// Journal entries require a recognized, non-blank classification.
// AdjustingEntryAudit is valid only for target period 14; period 14 rejects all
// non-audit adjusting types. Reversing entries require a non-blank reason in
// both the reversal builder and classification metadata validation.
//
// Import charter: May import only sibling internal/kernal/* modules, uuid and decimal.
//
// Baseline: AE-PLAN-STEP-P0 / P0-02, NEW_PROJECT_STRUCTURE.md,
// ARCHITECTURAL_PRINCIPLES_CHARTER.md.
package posting
