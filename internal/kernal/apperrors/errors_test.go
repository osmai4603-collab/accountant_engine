package apperrors

import (
	"errors"
	"testing"
)

func TestCodedErrorContract(t *testing.T) {
	cause := errors.New("root cause")
	err := New("TEST_CODE", "test message", cause)
	if err.Error() != "TEST_CODE: test message: root cause" {
		t.Fatalf("Error() = %q", err.Error())
	}
	if !errors.Is(err, cause) {
		t.Fatal("CodedError must unwrap its cause")
	}
	if !IsCode(err, "TEST_CODE") {
		t.Fatal("IsCode should recognize matching coded errors")
	}
	if IsCode(errors.New("plain"), "TEST_CODE") {
		t.Fatal("plain errors must not match a code")
	}

	withoutCause := New("NO_CAUSE", "message", nil)
	if withoutCause.Error() != "NO_CAUSE: message" {
		t.Fatalf("Error() without cause = %q", withoutCause.Error())
	}
	if withoutCause.Unwrap() != nil {
		t.Fatal("Unwrap without cause must return nil")
	}
}

func TestErrorTaxonomyCategoriesAndRetryability(t *testing.T) {
	domainErr := NewCategorized("ERR_DOMAIN", CategoryDomain, true, "domain invariant violated", nil)
	if CategoryOf(domainErr) != CategoryDomain {
		t.Fatalf("expected domain category, got %s", CategoryOf(domainErr))
	}
	if IsRetryable(domainErr) {
		t.Fatal("domain error must never be retryable")
	}

	valErr := NewCategorized("ERR_VALIDATION", CategoryValidation, true, "invalid input", nil)
	if CategoryOf(valErr) != CategoryValidation || IsRetryable(valErr) {
		t.Fatal("validation error must be non-retryable")
	}

	concErr := NewCategorized("ERR_CONCURRENCY", CategoryConcurrency, true, "serialization conflict", nil)
	if CategoryOf(concErr) != CategoryConcurrency || !IsRetryable(concErr) {
		t.Fatal("concurrency error must be retryable when marked")
	}

	persErr := NewCategorized("ERR_PERSISTENCE", CategoryPersistence, false, "unique constraint", nil)
	if CategoryOf(persErr) != CategoryPersistence || IsRetryable(persErr) {
		t.Fatal("non-retryable persistence error must not be retryable")
	}

	if CategoryOf(errors.New("plain")) != CategoryDomain || IsRetryable(errors.New("plain")) {
		t.Fatal("plain unclassified error should default to non-retryable domain")
	}
}
