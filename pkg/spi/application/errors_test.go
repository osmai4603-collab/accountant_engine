package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/osmai4603-collab/accounting_engine/pkg/spi/application"
)

func TestApplicationErrorAndRequestMetadata(t *testing.T) {
	cause := errors.New("sql: no rows")
	err := application.NewError(application.ErrNotFound, "entry missing", cause)
	if !errors.Is(err, application.ErrNotFound) {
		t.Fatalf("expected errors.Is(ErrNotFound)")
	}
	if err.Error() != "entry missing: sql: no rows" {
		t.Fatalf("unexpected Error(): %q", err.Error())
	}
	appErr := err.(*application.Error)
	if appErr.PublicMessage() != "entry missing" {
		t.Fatalf("unexpected PublicMessage(): %q", appErr.PublicMessage())
	}
	noCause := application.NewError(application.ErrValidation, "bad input", nil)
	if noCause.Error() != "bad input" {
		t.Fatalf("unexpected Error() without cause: %q", noCause.Error())
	}

	ctx := context.Background()
	if _, ok := application.RequestMetadataFromContext(ctx); ok {
		t.Fatal("expected no metadata in empty context")
	}
	meta := application.RequestMetadata{
		CorrelationID:  "corr-1",
		IdempotencyKey: "idem-1",
		SourceSystem:   "erp",
	}
	ctx = application.WithRequestMetadata(ctx, meta)
	got, ok := application.RequestMetadataFromContext(ctx)
	if !ok || got.CorrelationID != "corr-1" || got.IdempotencyKey != "idem-1" {
		t.Fatalf("unexpected metadata: ok=%v got=%+v", ok, got)
	}
}
