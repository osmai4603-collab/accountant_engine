package application

import (
	"context"
	"testing"

	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
)

func TestPrincipalContextRoundTrip(t *testing.T) {
	principal := Principal{
		ActorID:     "accountant-1",
		TenantID:    "tenant-1",
		Permissions: []string{"journal.post"},
	}

	ctx := WithPrincipal(context.Background(), principal)
	got, ok := PrincipalFromContext(ctx)
	if !ok {
		t.Fatal("principal missing from context")
	}
	if got.ActorID != principal.ActorID || got.TenantID != principal.TenantID {
		t.Fatalf("principal mismatch: got %+v", got)
	}
}

func TestCompanyContextRoundTrip(t *testing.T) {
	companyID := types.NewCompanyID()
	ctx := WithCompanyContext(context.Background(), companyID)
	got, ok := CompanyFromContext(ctx)
	if !ok {
		t.Fatal("company ID missing from context")
	}
	if got != companyID {
		t.Fatalf("company ID mismatch: got %v want %v", got, companyID)
	}
}

func TestContextExtractionRejectsMissingValues(t *testing.T) {
	if _, ok := PrincipalFromContext(context.Background()); ok {
		t.Fatal("unexpected principal")
	}
	if _, ok := CompanyFromContext(context.Background()); ok {
		t.Fatal("unexpected company ID")
	}
	if _, ok := PrincipalFromContext(nil); ok {
		t.Fatal("unexpected principal from nil context")
	}
	if _, ok := CompanyFromContext(nil); ok {
		t.Fatal("unexpected company ID from nil context")
	}
}
