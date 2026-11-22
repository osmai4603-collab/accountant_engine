// Package apperrors: coded error taxonomy for the accounting kernel's public
// error vocabulary.
//
// CodedError carries a stable machine-readable Code, a human-readable Message
// and an optional Cause; IsCode matches errors.As-based wrappers by code.
//
// Scope boundary: this package is a compatibility surface kept for callers that
// consume coded errors. No other kernel package currently imports it; kernel
// validation paths use package-local sentinel errors in types, masterdata,
// policy and posting. Do not delete or merge this package without a separate
// ADR and an API compatibility migration (remediation rule 3.1/5).
//
// Import charter: Go standard library and sibling kernel modules only; no
// storage, network, pkg/*, orbitals/* or modules/* imports.
package apperrors
