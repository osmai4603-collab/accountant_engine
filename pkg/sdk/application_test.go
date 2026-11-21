package sdk_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/osmai4603-collab/accounting_engine/pkg/sdk"
	publicapplication "github.com/osmai4603-collab/accounting_engine/pkg/spi/application"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/storage"
	"github.com/osmai4603-collab/accounting_engine/storage/memory"
	"github.com/shopspring/decimal"
)

type countingAuthorizer struct {
	calls int
}

func (a *countingAuthorizer) Authorize(context.Context, publicapplication.AuthorizationRequest) error {
	a.calls++
	return nil
}

type approvalCapabilityStub struct{}

func (approvalCapabilityStub) IsApproved(context.Context, types.CompanyID, types.JournalEntryID) (bool, error) {
	return true, nil
}

type dimensionCapabilityStub struct{}

func (dimensionCapabilityStub) RequiresCostCenter(context.Context, types.CompanyID, types.AccountID) (bool, error) {
	return false, nil
}

func (dimensionCapabilityStub) ValidateCostCenter(context.Context, types.CompanyID, types.CostCenterID) error {
	return nil
}

func (dimensionCapabilityStub) ValidateAnalyticalDimension(context.Context, types.CompanyID, string, uuid.UUID) error {
	return nil
}

type makerCheckerCapabilityStub struct{}

func (makerCheckerCapabilityStub) MakerCheckerThreshold(context.Context, types.CompanyID) (decimal.Decimal, bool, error) {
	return decimal.Zero, false, nil
}

func TestBuilderRequiresVerifiedPrincipalWhenAuthorizerConfigured(t *testing.T) {
	companyID := types.NewCompanyID()
	authorizer := &countingAuthorizer{}
	engine, err := sdk.NewBuilder(
		sdk.WithStoragePorts(memory.New().Ports()),
		sdk.WithAuthorizer(authorizer),
	).Build(context.Background())
	if err != nil {
		t.Fatalf("build engine: %v", err)
	}

	_, err = engine.Ledger().GetEntry(context.Background(), companyID, types.NewJournalEntryID())
	if !errors.Is(err, publicapplication.ErrUnauthorized) {
		t.Fatalf("GetEntry without principal error = %v, want %v", err, publicapplication.ErrUnauthorized)
	}
	if authorizer.calls != 0 {
		t.Fatalf("authorizer called %d times without verified principal", authorizer.calls)
	}

	ctx := publicapplication.WithPrincipal(context.Background(), publicapplication.Principal{
		ActorID:    "reader",
		CompanyIDs: []types.CompanyID{companyID},
	})
	_, _ = engine.Ledger().GetEntry(ctx, companyID, types.NewJournalEntryID())
	if authorizer.calls != 1 {
		t.Fatalf("authorizer calls = %d, want 1", authorizer.calls)
	}
}

func TestBuilderRejectsMissingRequiredPostingCapabilities(t *testing.T) {
	tests := []struct {
		name       string
		config     sdk.PostingConfig
		capability string
	}{
		{name: "approval", config: sdk.PostingConfig{RequireApproval: true}, capability: "approval"},
		{name: "financial dimensions", config: sdk.PostingConfig{RequireFinancialDimensions: true}, capability: "financial dimensions"},
		{name: "maker-checker policy", config: sdk.PostingConfig{RequireMakerCheckerPolicy: true}, capability: "maker-checker policy"},
		{name: "posting policy", config: sdk.PostingConfig{RequirePolicy: true}, capability: "posting policy"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := sdk.NewBuilder(
				sdk.WithStoragePorts(memory.New().Ports()),
				sdk.WithPostingConfig(test.config),
			).Build(context.Background())
			if err == nil || !strings.Contains(err.Error(), test.capability) {
				t.Fatalf("Build error = %v, want missing %q capability", err, test.capability)
			}
		})
	}
}

func TestBuilderAcceptsSuppliedPostingCapabilities(t *testing.T) {
	engine, err := sdk.NewBuilder(
		sdk.WithStoragePorts(memory.New().Ports()),
		sdk.WithPostingConfig(sdk.PostingConfig{
			RequireApproval:            true,
			RequireFinancialDimensions: true,
			RequireMakerCheckerPolicy:  true,
		}),
		sdk.WithPostingCapabilities(storage.PostingCapabilities{
			ApprovalGate:        approvalCapabilityStub{},
			FinancialDimensions: dimensionCapabilityStub{},
			MakerCheckerPolicy:  makerCheckerCapabilityStub{},
		}),
	).Build(context.Background())
	if err != nil {
		t.Fatalf("Build with required posting capabilities: %v", err)
	}
	if err := engine.Close(); err != nil {
		t.Fatalf("close engine: %v", err)
	}
}

func TestBuilderTreatsTypedNilPostingCapabilityAsMissing(t *testing.T) {
	var gate *approvalCapabilityStub
	_, err := sdk.NewBuilder(
		sdk.WithStoragePorts(memory.New().Ports()),
		sdk.WithPostingConfig(sdk.PostingConfig{RequireApproval: true}),
		sdk.WithPostingCapabilities(storage.PostingCapabilities{ApprovalGate: gate}),
	).Build(context.Background())
	if err == nil || !strings.Contains(err.Error(), "approval") {
		t.Fatalf("Build error = %v, want missing approval capability", err)
	}
}
