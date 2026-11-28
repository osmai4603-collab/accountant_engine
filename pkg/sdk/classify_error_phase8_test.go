package sdk

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/osmai4603-collab/accounting_engine/internal/kernel/apperrors"
	"github.com/osmai4603-collab/accounting_engine/internal/kernel/journal"
	kernelmasterdata "github.com/osmai4603-collab/accounting_engine/internal/kernel/masterdata"
	"github.com/osmai4603-collab/accounting_engine/internal/kernel/posting"
	"github.com/osmai4603-collab/accounting_engine/internal/kernel/types"
	"github.com/osmai4603-collab/accounting_engine/internal/kernel/valuation"
	publicapplication "github.com/osmai4603-collab/accounting_engine/pkg/spi/application"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/storage"
)

func TestPhase8ScopeViolationMapsToForbidden(t *testing.T) {
	cause := fmt.Errorf("cross-company lock: %w", storage.ErrScopeViolation)
	classified := classifyError(cause)
	if !errors.Is(classified, publicapplication.ErrForbidden) {
		t.Fatalf("classified error = %v, want ErrForbidden", classified)
	}
	if errors.Is(classified, publicapplication.ErrConflict) {
		t.Fatal("scope violation must not be classified as retryable conflict")
	}
}

func TestClassifyErrorUsesTypedTaxonomyWithoutStringHeuristics(t *testing.T) {
	// Free-text "not found" without an authoritative sentinel must not be misclassified.
	plainTextErr := errors.New("unexpected downstream text mentioning not found")
	if got := classifyError(plainTextErr); got != plainTextErr {
		t.Fatalf("plain text error was misclassified as %v", got)
	}

	cases := []struct {
		name string
		err  error
		want error
	}{
		{"invalid reversal", posting.ErrInvalidReversal, publicapplication.ErrValidation},
		{"inactive dimension", posting.ErrInactiveFinancialDimension, publicapplication.ErrValidation},
		{"cannot post to header account", kernelmasterdata.ErrCannotPostToParentAccount, publicapplication.ErrValidation},
		{"journal reader required", kernelmasterdata.ErrJournalReaderRequired, publicapplication.ErrValidation},
		{"invalid cancellation", journal.ErrInvalidCancellation, publicapplication.ErrValidation},
		{"apperrors unbalanced", apperrors.ErrUnbalancedEntry, publicapplication.ErrValidation},
		{"period closed", kernelmasterdata.ErrFiscalPeriodClosed, publicapplication.ErrPeriodClosed},
		{"apperrors period closed", apperrors.ErrPeriodClosed, publicapplication.ErrPeriodClosed},
		{"rate not found", valuation.ErrRateNotFound, publicapplication.ErrNotFound},
		{"entry not found", storage.ErrEntryNotFound, publicapplication.ErrNotFound},
		{"account not found", kernelmasterdata.ErrAccountNotFound, publicapplication.ErrNotFound},
		{"company inactive", kernelmasterdata.ErrCompanyInactive, publicapplication.ErrForbidden},
		{"concurrency conflict", posting.ErrConcurrencyConflict, publicapplication.ErrConflict},
		{"coded infra error", apperrors.NewCategorized("ERR_IO", apperrors.CategoryInfrastructure, true, "db timeout", nil), publicapplication.ErrDependencyUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyError(fmt.Errorf("wrapped: %w", tc.err)); !errors.Is(got, tc.want) {
				t.Fatalf("classifyError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

type legacyOnlyMasterDataReader struct {
	storage.MasterDataReader
}

func TestGetJournalByIDFailsClosedUnlessSyntheticOptedIn(t *testing.T) {
	companyID := types.NewCompanyID()
	journalID := types.NewJournalID()
	reader := masterDataReaderAdapter{reader: legacyOnlyMasterDataReader{}}
	if _, err := reader.GetJournalByID(context.Background(), companyID, journalID); !errors.Is(err, kernelmasterdata.ErrJournalReaderRequired) {
		t.Fatalf("default GetJournalByID error = %v, want ErrJournalReaderRequired", err)
	}
}
