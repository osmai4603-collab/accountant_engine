package storage

import (
	"strings"
	"testing"
)

func TestPortsValidateReportsMissingRequiredPorts(t *testing.T) {
	err := (Ports{}).Validate()
	if err == nil {
		t.Fatal("expected validation error")
	}

	message := err.Error()
	for _, want := range []string{"draft store", "entry reader", "posting writer", "sequence allocator", "master data reader"} {
		if !strings.Contains(message, want) {
			t.Errorf("validation error %q does not contain %q", message, want)
		}
	}
}
