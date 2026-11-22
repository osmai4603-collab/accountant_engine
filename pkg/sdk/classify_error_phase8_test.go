package sdk

import (
	"errors"
	"fmt"
	"testing"

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
