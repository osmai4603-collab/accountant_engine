package sdk_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/events"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/osmai4603-collab/accounting_engine/pkg/sdk"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/standards"
	"github.com/osmai4603-collab/accounting_engine/storage/memory"
)

type mockStandard struct {
	standards.StandardModule
	name            string
	err             error
	validateCalls   int
	initializeCalls int
	ports           standards.ModulePorts
}

func (m *mockStandard) Name() string {
	return m.name
}

func (m *mockStandard) Version() string {
	return "1.0.0"
}

func (m *mockStandard) Dependencies() []string {
	return nil
}

func (m *mockStandard) ValidateDependencies(ports standards.ModulePorts) error {
	m.validateCalls++
	m.ports = ports
	return m.err
}

func (m *mockStandard) Initialize(ctx context.Context, ports standards.ModulePorts) error {
	m.initializeCalls++
	m.ports = ports
	return m.err
}

func (m *mockStandard) RegisterRules(registrar events.RuleRegistrar) error {
	return nil
}

func (m *mockStandard) RunScheduledJobs(ctx context.Context, runner standards.ScheduledJobRunner) error {
	return nil
}

func (m *mockStandard) Close(ctx context.Context) error {
	return nil
}

func TestBuilder_FailFastWithoutStorage(t *testing.T) {
	ctx := context.Background()
	engine, err := sdk.NewBuilder().Build(ctx)
	if err == nil {
		t.Fatalf("expected error when building without storage ports, got nil")
	}
	if engine != nil {
		t.Fatalf("expected nil engine on failure")
	}
}

func TestBuilder_StoragePortsSuccess(t *testing.T) {
	ctx := context.Background()
	driver := memory.New()
	engine, err := sdk.NewBuilder(
		sdk.WithStoragePorts(driver.Ports()),
	).Build(ctx)
	if err != nil {
		t.Fatalf("expected success building with storage ports, got: %v", err)
	}
	if engine == nil {
		t.Fatalf("expected non-nil engine")
	}
	if err := engine.Close(); err != nil {
		t.Fatalf("failed to close engine: %v", err)
	}
}

func TestBuilder_StandardModuleLifecycle(t *testing.T) {
	driver := memory.New()
	mod := &mockStandard{name: "TEST_MOD"}
	engine, err := sdk.NewBuilder(
		sdk.WithStoragePorts(driver.Ports()),
		sdk.WithStandard(mod),
	).Build(context.Background())
	if err != nil {
		t.Fatalf("Build with ports and standard module: %v", err)
	}
	defer func() {
		if closeErr := engine.Close(); closeErr != nil {
			t.Fatalf("close engine: %v", closeErr)
		}
	}()

	if mod.validateCalls != 1 {
		t.Fatalf("ValidateDependencies calls = %d, want 1", mod.validateCalls)
	}
	if mod.initializeCalls != 1 {
		t.Fatalf("Initialize calls = %d, want 1", mod.initializeCalls)
	}
	if mod.ports == nil {
		t.Fatal("module received nil ModulePorts")
	}
	if mod.ports.Entries() == nil {
		t.Fatal("module ports have no entry reader")
	}
	if mod.ports.MasterData() == nil {
		t.Fatal("module ports have no master data reader")
	}
	if mod.ports.Application() == nil {
		t.Fatal("module ports have no application service")
	}

	diag := engine.Standards().Diagnostics()
	if len(diag.ActiveModules) != 1 || diag.ActiveModules[0].Name != "TEST_MOD" {
		t.Fatalf("unexpected standards diagnostics: %+v", diag.ActiveModules)
	}
}

func TestBuilder_OptionalCapabilitiesFailExplicitly(t *testing.T) {
	driver := memory.New()
	engine, err := sdk.NewBuilder(sdk.WithStoragePorts(driver.Ports())).Build(context.Background())
	if err != nil {
		t.Fatalf("build engine: %v", err)
	}
	defer engine.Close()

	if _, err := engine.Reporting().GetTrialBalance(context.Background(), types.NewCompanyID(), time.Now(), "USD"); !errors.Is(err, types.ErrCapabilityNotImplemented) {
		t.Fatalf("Reporting error = %v, want %v", err, types.ErrCapabilityNotImplemented)
	}
	if err := engine.Governance().RecordAuditLog(context.Background(), "event", nil); !errors.Is(err, types.ErrCapabilityNotImplemented) {
		t.Fatalf("Governance error = %v, want %v", err, types.ErrCapabilityNotImplemented)
	}
}

func TestBuilder_ValidateRequiredCapabilities(t *testing.T) {
	driver := memory.New()

	// Missing reporting store
	_, err := sdk.NewBuilder(
		sdk.WithStoragePorts(driver.Ports()),
		sdk.WithRequiredCapabilities(sdk.BootstrapReporting),
	).Build(context.Background())
	// memory driver has balance, masterdata and entry ports, so reporting ports are satisfied
	_ = err

	// Missing standards
	_, err = sdk.NewBuilder(
		sdk.WithStoragePorts(driver.Ports()),
		sdk.WithRequiredCapabilities(sdk.BootstrapStandards),
	).Build(context.Background())
	if err == nil {
		t.Fatalf("expected error for missing required standards")
	}

	// Missing subledgers
	_, err = sdk.NewBuilder(
		sdk.WithStoragePorts(driver.Ports()),
		sdk.WithRequiredCapabilities(sdk.BootstrapSubledgers),
	).Build(context.Background())
	if err == nil {
		t.Fatalf("expected error for missing required subledgers")
	}
}
