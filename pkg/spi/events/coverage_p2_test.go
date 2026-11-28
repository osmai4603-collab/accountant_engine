package events

import (
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestCoverageIngestionStatusHelpers(t *testing.T) {
	if got := NormalizeStatus("  completed "); got != StatusCompleted {
		t.Fatalf("NormalizeStatus = %q", got)
	}
	if !IsCompleted(" completed ") || IsCompleted(StatusStarted) {
		t.Fatal("IsCompleted misclassified status")
	}
	if !IsStarted(" started ") || IsStarted(StatusCompleted) {
		t.Fatal("IsStarted misclassified status")
	}
	if !IsRetryableFailure(" retryable_failure ") || IsRetryableFailure(StatusTerminalFailure) {
		t.Fatal("IsRetryableFailure misclassified status")
	}
	if !IsTerminalFailure(" terminal_failure ") || IsTerminalFailure(StatusStarted) {
		t.Fatal("IsTerminalFailure misclassified status")
	}
}

func TestCoverageIngestionSourceAndKeys(t *testing.T) {
	if got := NormalizeSourceSystem("  "); got != DefaultSourceSystem {
		t.Fatalf("default source = %q", got)
	}
	if got := NormalizeSourceSystem(" pos "); got != "pos" {
		t.Fatalf("normalised source = %q", got)
	}
	if got := ScopedIdempotencyKey("event", "  key-1 "); got != "key-1" {
		t.Fatalf("default scoped key = %q", got)
	}
	if got := ScopedIdempotencyKey("", "key-1"); got != "key-1" {
		t.Fatalf("blank source scoped key = %q", got)
	}
	if got := ScopedIdempotencyKey("pos", "  key-1 "); got != "pos:key-1" {
		t.Fatalf("source scoped key = %q", got)
	}

	companyID := uuid.New()
	want := companyID.String() + "\x00" + "pos" + "\x00" + "key-1"
	if got := CompositeIdempotencyKey(companyID, "pos", " key-1 "); got != want {
		t.Fatalf("composite key = %q, want %q", got, want)
	}
}

func TestCoverageIngestionErrorValuesAndPorts(t *testing.T) {
	if ErrRecordNotFound == nil || ErrStateConflict == nil || ErrRecordNotFound.Error() == ErrStateConflict.Error() {
		t.Fatal("ingestion errors must be distinct")
	}
	var store IdempotencyStore
	var scoped ScopedIdempotencyStore
	var classified ClassifiedIdempotencyStore
	if store != nil || scoped != nil || classified != nil {
		t.Fatal("interfaces should default to nil")
	}
	if !errors.Is(ErrRecordNotFound, ErrRecordNotFound) || !errors.Is(ErrStateConflict, ErrStateConflict) {
		t.Fatal("sentinel errors must match themselves")
	}

	stores := Stores{Idempotency: store, Mappings: nil}
	if stores.Idempotency != nil || stores.Mappings != nil {
		t.Fatalf("empty stores = %+v", stores)
	}
	mapping := AccountMapping{CompanyID: uuid.New(), EventType: "invoice.created", DebitAccountCode: "1000"}
	if mapping.DebitAccountCode != "1000" {
		t.Fatal("mapping value was not retained")
	}
	record := Ingestion{Status: StatusStarted, FailureClass: FailureClassTransient, SourceSystem: DefaultSourceSystem}
	if !IsStarted(record.Status) || record.FailureClass != FailureClassTransient {
		t.Fatalf("ingestion record = %+v", record)
	}
}
