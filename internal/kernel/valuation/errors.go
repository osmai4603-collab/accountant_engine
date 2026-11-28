package valuation

import (
	"errors"

	"github.com/osmai4603-collab/accounting_engine/internal/kernel/masterdata"
)

var (
	ErrInvalidPosition           = errors.New("invalid monetary position")
	ErrInvalidAccountType        = masterdata.ErrInvalidAccountType
	ErrZeroRevaluationDifference = errors.New("fx revaluation difference cannot be zero")
	// ErrRevaluationAlreadyApplied is an operation-level payload conflict: the
	// revaluation key was reused with a different stable result. Retrying cannot
	// change that outcome, so it is non-retryable.
	ErrRevaluationAlreadyApplied     = errors.New("revaluation has already been applied")
	ErrInvalidAdjustingEntryMetadata = errors.New("invalid adjusting entry metadata")
	ErrReversalBuilderRequired       = errors.New("reversal builder is required")
)
