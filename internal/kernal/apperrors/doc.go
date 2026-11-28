// Package apperrors: coded error taxonomy for the accounting kernel's public
// error vocabulary.
//
// CodedError carries a stable machine-readable Code, a human-readable Message
// and an optional Cause; IsCode matches errors.As-based wrappers by code.
//
// Scope boundary: this package is a compatibility surface kept for callers that
// consume coded errors. Its domain sentinels (ErrUnbalancedEntry, ErrPeriodClosed)
// wrap the canonical domain sentinels (posting.ErrUnbalancedMove and
// masterdata.ErrFiscalPeriodClosed) so errors.Is works across both surfaces.
// No other kernel package imports apperrors.
//
// Import charter: Go standard library and sibling kernel modules only; no
// storage, network, pkg/*, orbitals/* or modules/* imports.
package apperrors
