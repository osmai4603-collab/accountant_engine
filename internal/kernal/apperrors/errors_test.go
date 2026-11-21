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
