// Package kernal: Pure deterministic accounting kernel.
//
// This package is a documentation-only marker. The kernel consists of nine
// implementation packages under internal/kernal/* (see docs/architecture/KERNEL_ARCHITECTURE_MAP.md):
// types, apperrors, journal, ledger, masterdata, governance, policy, posting and
// valuation. docs/architecture/KERNEL_ARCHITECTURE_DECISION_RECORDS.md records their responsibilities;
// docs/architecture/KERNEL_ARCHITECTURE_DECISION_RECORDS.md records the canonical accounting record.
//
// Import charter: kernel modules may import only the Go standard library,
// github.com/google/uuid, github.com/shopspring/decimal, and sibling kernel
// modules. They must never import pkg/*, storage/*, orbitals/*, modules/* or
// any network/database package. This charter is enforced by doc_test.go and
// scripts/verify_architecture.sh (rules R1, R3a-R3d).
//
// Baseline: AE-PLAN-STEP-P0 / P0-02, NEW_PROJECT_STRUCTURE.md,
// ARCHITECTURAL_PRINCIPLES_CHARTER.md, and the kernel remediation Phase 1 ADRs.
package kernal
