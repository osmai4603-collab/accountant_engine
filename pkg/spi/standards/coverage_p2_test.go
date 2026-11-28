package standards

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/events"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/journal"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	publicapplication "github.com/osmai4603-collab/accounting_engine/pkg/spi/application"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/masterdata"
	"github.com/shopspring/decimal"
)

type coverageModule struct {
	name    string
	version string
	deps    []string
}

func (m coverageModule) Name() string                                  { return m.name }
func (m coverageModule) Version() string                               { return m.version }
func (m coverageModule) Dependencies() []string                        { return m.deps }
func (m coverageModule) ValidateDependencies(ModulePorts) error        { return nil }
func (m coverageModule) Initialize(context.Context, ModulePorts) error { return nil }
func (m coverageModule) RegisterRules(events.RuleRegistrar) error      { return nil }
func (m coverageModule) RunScheduledJobs(context.Context, ScheduledJobRunner) error {
	return nil
}
func (m coverageModule) Close(context.Context) error { return nil }

type coverageDescribedModule struct {
	coverageModule
	desc ModuleDescriptor
}

func (m coverageDescribedModule) Descriptor() ModuleDescriptor { return m.desc }

func coverageValidProfile() StandardProfile {
	return StandardProfile{
		Standard:      "IFRS",
		Edition:       "2026",
		Amendments:    []string{"2026-amendment"},
		EffectiveDate: "2026-01-01",
		Jurisdiction:  "global",
		Scope:         "revenue",
		SupportedRequirements: []StandardRequirement{{
			ID:             "R1",
			Summary:        "Recognise revenue",
			Status:         RequirementLimited,
			Implementation: "ifrs15.go",
			TestEvidence:   "ifrs15_test.go",
		}},
		Exclusions: []string{"none"},
	}
}

func coverageValidWorkflow(profile StandardProfile) WorkflowDescriptor {
	return WorkflowDescriptor{
		ID:             "WF-1",
		Name:           "Recognise revenue",
		Summary:        "Bounded revenue workflow",
		Inputs:         []string{"contract"},
		Steps:          []string{"compute", "review"},
		Outputs:        []string{"draft"},
		RequirementIDs: []string{profile.SupportedRequirements[0].ID},
	}
}

func coverageValidDescribedModule(id ModuleID, name string) coverageDescribedModule {
	profile := coverageValidProfile()
	return coverageDescribedModule{
		coverageModule: coverageModule{name: name, version: "1.2.0"},
		desc: ModuleDescriptor{
			ID:          id,
			Name:        name,
			Version:     "1.2.0",
			Provides:    []Capability{CapRevenueRecognition},
			Profile:     &profile,
			Workflows:   []WorkflowDescriptor{coverageValidWorkflow(profile)},
			Description: "test module",
		},
	}
}

func TestCoverageProfileApplyConfig(t *testing.T) {
	profile := coverageValidProfile()
	profile.ApplyConfig(StandardProfileConfig{})
	if profile.Edition != "2026" || profile.EffectiveDate != "2026-01-01" || profile.Jurisdiction != "global" {
		t.Fatalf("empty config changed profile: %+v", profile)
	}

	cfg := StandardProfileConfig{
		Edition:       "  2027  ",
		Amendments:    []string{"A1", "A2"},
		EffectiveDate: " 2027-02-03 ",
		Jurisdiction:  " eu ",
	}
	profile.ApplyConfig(cfg)
	if profile.Edition != "2027" || profile.EffectiveDate != "2027-02-03" || profile.Jurisdiction != "eu" {
		t.Fatalf("config overrides not trimmed: %+v", profile)
	}
	if len(profile.Amendments) != 2 || profile.Amendments[0] != "A1" {
		t.Fatalf("amendments not copied: %+v", profile.Amendments)
	}
	cfg.Amendments[0] = "MUTATED"
	if profile.Amendments[0] != "A1" {
		t.Fatal("amendments were not defensively copied")
	}

	var nilProfile *StandardProfile
	nilProfile.ApplyConfig(cfg) // must not panic
}

func TestCoverageProfileValidate(t *testing.T) {
	base := coverageValidProfile()
	if err := base.Validate(); err != nil {
		t.Fatalf("valid profile: %v", err)
	}
	if !base.IsConfigured() {
		t.Fatal("valid pinned profile should be configured")
	}

	mutations := []struct {
		name string
		edit func(*StandardProfile)
	}{
		{"standard", func(p *StandardProfile) { p.Standard = "  " }},
		{"edition", func(p *StandardProfile) { p.Edition = "" }},
		{"effective date", func(p *StandardProfile) { p.EffectiveDate = " " }},
		{"jurisdiction", func(p *StandardProfile) { p.Jurisdiction = "" }},
		{"scope", func(p *StandardProfile) { p.Scope = "" }},
		{"amendments", func(p *StandardProfile) { p.Amendments = nil }},
		{"requirements", func(p *StandardProfile) { p.SupportedRequirements = nil }},
		{"exclusions", func(p *StandardProfile) { p.Exclusions = nil }},
		{"blank amendment", func(p *StandardProfile) { p.Amendments = []string{" "} }},
		{"blank exclusion", func(p *StandardProfile) { p.Exclusions = []string{"  "} }},
		{"missing requirement ID", func(p *StandardProfile) { p.SupportedRequirements[0].ID = " " }},
		{"missing requirement summary", func(p *StandardProfile) { p.SupportedRequirements[0].Summary = "" }},
		{"duplicate requirement", func(p *StandardProfile) {
			p.SupportedRequirements = append(p.SupportedRequirements, p.SupportedRequirements[0])
		}},
		{"duplicate across lists", func(p *StandardProfile) {
			p.UnimplementedRequirements = []StandardRequirement{{ID: p.SupportedRequirements[0].ID, Summary: "x", Status: RequirementNotImplemented}}
		}},
		{"unimplemented in supported", func(p *StandardProfile) {
			p.SupportedRequirements[0].Status = RequirementNotImplemented
		}},
		{"unknown status", func(p *StandardProfile) { p.SupportedRequirements[0].Status = "mystery" }},
		{"missing evidence", func(p *StandardProfile) { p.SupportedRequirements[0].TestEvidence = "" }},
		{"missing implementation", func(p *StandardProfile) { p.SupportedRequirements[0].Implementation = "" }},
		{"unimplemented missing ID", func(p *StandardProfile) {
			p.UnimplementedRequirements = []StandardRequirement{{Status: RequirementNotImplemented}}
		}},
		{"unimplemented duplicate ID", func(p *StandardProfile) {
			p.UnimplementedRequirements = []StandardRequirement{
				{ID: "U1", Summary: "x", Status: RequirementNotImplemented},
				{ID: "U1", Summary: "y", Status: RequirementNotImplemented},
			}
		}},
		{"unimplemented wrong status", func(p *StandardProfile) {
			p.UnimplementedRequirements = []StandardRequirement{{ID: "U1", Summary: "x", Status: RequirementPartial}}
		}},
		{"unimplemented with evidence", func(p *StandardProfile) {
			p.UnimplementedRequirements = []StandardRequirement{{ID: "U1", Summary: "x", Status: RequirementNotImplemented, Implementation: "x"}}
		}},
	}
	for _, tc := range mutations {
		t.Run(tc.name, func(t *testing.T) {
			p := base
			p.Amendments = append([]string(nil), base.Amendments...)
			p.Exclusions = append([]string(nil), base.Exclusions...)
			p.SupportedRequirements = append([]StandardRequirement(nil), base.SupportedRequirements...)
			p.UnimplementedRequirements = nil
			tc.edit(&p)
			if err := p.Validate(); err == nil {
				t.Fatalf("expected validation error for %s", tc.name)
			}
		})
	}

	withUnimplemented := base
	withUnimplemented.UnimplementedRequirements = []StandardRequirement{{ID: "U1", Summary: "Out of scope", Status: RequirementNotImplemented}}
	if err := withUnimplemented.Validate(); err != nil {
		t.Fatalf("valid unimplemented requirement: %v", err)
	}

	badDate := base
	badDate.EffectiveDate = "not-a-date"
	if badDate.IsConfigured() {
		t.Fatal("profile with invalid effective date must not be configured")
	}
	placeholder := base
	placeholder.Amendments = []string{"NOT CONFIGURED"}
	if err := placeholder.Validate(); err != nil {
		t.Fatalf("placeholder profile still validates: %v", err)
	}
	if placeholder.IsConfigured() {
		t.Fatal("placeholder profile must not be configured")
	}
	if (StandardProfile{}).IsConfigured() {
		t.Fatal("empty profile must not be configured")
	}
}

func TestCoverageWorkflowDescriptorValidate(t *testing.T) {
	profile := coverageValidProfile()
	valid := coverageValidWorkflow(profile)
	if err := valid.Validate(profile); err != nil {
		t.Fatalf("valid workflow: %v", err)
	}
	mutations := []struct {
		name string
		edit func(*WorkflowDescriptor)
	}{
		{"id", func(w *WorkflowDescriptor) { w.ID = "" }},
		{"name", func(w *WorkflowDescriptor) { w.Name = " " }},
		{"summary", func(w *WorkflowDescriptor) { w.Summary = "" }},
		{"inputs", func(w *WorkflowDescriptor) { w.Inputs = nil }},
		{"steps", func(w *WorkflowDescriptor) { w.Steps = nil }},
		{"outputs", func(w *WorkflowDescriptor) { w.Outputs = nil }},
		{"requirements", func(w *WorkflowDescriptor) { w.RequirementIDs = nil }},
		{"blank input", func(w *WorkflowDescriptor) { w.Inputs = []string{" "} }},
		{"blank step", func(w *WorkflowDescriptor) { w.Steps = []string{" "} }},
		{"blank output", func(w *WorkflowDescriptor) { w.Outputs = []string{""} }},
		{"blank requirement", func(w *WorkflowDescriptor) { w.RequirementIDs = []string{" "} }},
		{"unknown requirement", func(w *WorkflowDescriptor) { w.RequirementIDs = []string{"R404"} }},
	}
	for _, tc := range mutations {
		t.Run(tc.name, func(t *testing.T) {
			w := valid
			tc.edit(&w)
			if err := w.Validate(profile); err == nil {
				t.Fatalf("expected workflow validation error for %s", tc.name)
			}
		})
	}

	posting := valid
	posting.ProducesJournalEntries = true
	if err := posting.Validate(profile); err == nil {
		t.Fatal("posting workflow requires application contract and capability")
	}
	posting.ApplicationContract = ApplicationLedgerService
	if err := posting.Validate(profile); err == nil {
		t.Fatal("posting workflow requires ledger posting capability")
	}
	posting.RequiredCapabilities = []Capability{CapLedgerPosting}
	if err := posting.Validate(profile); err == nil {
		t.Fatal("posting workflow requires journal mapping")
	}
	posting.RequiresJournalMapping = true
	if err := posting.Validate(profile); err != nil {
		t.Fatalf("fully declared posting workflow: %v", err)
	}

	if containsCapability(nil, CapLedgerPosting) {
		t.Fatal("empty capability list cannot contain ledger posting")
	}
	if containsCapability([]Capability{CapReporting}, CapLedgerPosting) {
		t.Fatal("unexpected capability match")
	}
}

type coverageEntryReader struct{}

func (coverageEntryReader) GetEntry(context.Context, types.CompanyID, types.JournalEntryID) (journal.JournalEntry, error) {
	return journal.JournalEntry{}, nil
}

type coverageBalanceReader struct{}

func (coverageBalanceReader) GetAccountBalance(context.Context, types.CompanyID, types.AccountID, time.Time, string) (decimal.Decimal, error) {
	return decimal.Zero, nil
}

type coverageMasterDataReader struct{}

func (coverageMasterDataReader) GetCompany(context.Context, types.CompanyID) (masterdata.Company, error) {
	return masterdata.Company{}, nil
}
func (coverageMasterDataReader) GetAccount(context.Context, types.CompanyID, types.AccountID) (masterdata.Account, error) {
	return masterdata.Account{}, nil
}
func (coverageMasterDataReader) GetAccountByCode(context.Context, types.CompanyID, string) (masterdata.Account, error) {
	return masterdata.Account{}, nil
}
func (coverageMasterDataReader) GetJournalByCode(context.Context, types.CompanyID, string) (masterdata.Journal, error) {
	return masterdata.Journal{}, nil
}

type coverageAppService struct{}

func (coverageAppService) CreateDraft(context.Context, publicapplication.CreateDraftCommand) (publicapplication.EntryResult, error) {
	return publicapplication.EntryResult{}, nil
}
func (coverageAppService) Validate(context.Context, publicapplication.ValidateEntryCommand) error {
	return nil
}
func (coverageAppService) Post(context.Context, publicapplication.PostEntryCommand) (publicapplication.EntryResult, error) {
	return publicapplication.EntryResult{}, nil
}
func (coverageAppService) Reverse(context.Context, publicapplication.ReverseEntryCommand) (publicapplication.EntryResult, error) {
	return publicapplication.EntryResult{}, nil
}
func (coverageAppService) GetEntry(context.Context, publicapplication.GetEntryQuery) (journal.JournalEntry, error) {
	return journal.JournalEntry{}, errors.New("not found")
}

func TestCoverageModulePorts(t *testing.T) {
	ports := NewModulePorts(nil, nil, nil, nil)
	if ports.Entries() != nil || ports.Balance() != nil || ports.MasterData() != nil || ports.Application() != nil {
		t.Fatalf("nil ports were not preserved: %+v", ports)
	}
	if ports.Posting() == nil {
		t.Fatal("Posting() must always return a capability")
	}
	if err := ValidateStandardModulePorts(nil); err == nil {
		t.Fatal("nil ports must be rejected")
	}
	err := ValidateStandardModulePorts(ports)
	if err == nil || !strings.Contains(err.Error(), "application") ||
		!strings.Contains(err.Error(), "master data") || !strings.Contains(err.Error(), "entry reader") {
		t.Fatalf("expected all missing ports to be reported, got %v", err)
	}

	full := NewModulePorts(coverageEntryReader{}, coverageBalanceReader{}, coverageMasterDataReader{}, coverageAppService{})
	if full.Entries() == nil || full.Balance() == nil || full.MasterData() == nil || full.Application() == nil {
		t.Fatal("configured ports were not returned")
	}
	if err := ValidateStandardModulePorts(full); err != nil {
		t.Fatalf("complete ports: %v", err)
	}
}

func TestCoverageRegistryRegisterAndLookup(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(nil); err == nil {
		t.Fatal("nil module must be rejected")
	}

	base := coverageModule{name: "IFRS 15 Revenue", version: "1.3.0", deps: []string{"Base-Store"}}
	if err := r.Register(base); err != nil {
		t.Fatalf("register simple module: %v", err)
	}
	if err := r.Register(base); !errors.Is(err, ErrDuplicateModule) {
		t.Fatalf("duplicate registration error = %v", err)
	}
	id := canonicalModuleID("IFRS 15 Revenue")
	mod, ok := r.GetModule(id)
	if !ok || mod.Name() != base.name {
		t.Fatalf("GetModule = %v, %v", mod, ok)
	}
	desc, ok := r.GetDescriptor(id)
	if !ok || desc.ID != id || len(desc.Requires) != 1 || desc.Requires[0].ID != canonicalModuleID("Base-Store") {
		t.Fatalf("synthesized descriptor = %+v, %v", desc, ok)
	}
	if _, ok := r.GetModule("unknown"); ok {
		t.Fatal("unknown module was found")
	}
	if _, ok := r.GetDescriptor("unknown"); ok {
		t.Fatal("unknown descriptor was found")
	}

	if got := canonicalModuleID("  Mixed_Case-Name  "); got != "mixedcasename" {
		t.Fatalf("canonicalModuleID = %q", got)
	}
	if compareSemVer("v1.2.3", "1.2.3") != 0 || compareSemVer("1.2.3", "1.3.0") >= 0 || compareSemVer("2.0.0", "1.9.9") <= 0 {
		t.Fatal("unexpected compareSemVer results")
	}
	if compareSemVer("1.2", "1.2.0.1") >= 0 {
		t.Fatal("shorter version should compare lower")
	}
	if compareSemVer("x", "1") != -1 {
		t.Fatal("non-numeric component should parse as zero")
	}
	ids := []ModuleID{"c", "a", "b"}
	sortModuleIDs(ids)
	if ids[0] != "a" || ids[1] != "b" || ids[2] != "c" {
		t.Fatalf("sortModuleIDs = %v", ids)
	}
}

func TestCoverageRegistryMetadataValidation(t *testing.T) {
	r := NewRegistry()
	badProfile := coverageValidProfile()
	badProfile.Scope = ""
	bad := coverageDescribedModule{
		coverageModule: coverageModule{name: "Bad Profile"},
		desc:           ModuleDescriptor{ID: "bad", Name: "Bad Profile", Profile: &badProfile},
	}
	if err := r.Register(bad); !errors.Is(err, ErrInvalidMetadata) {
		t.Fatalf("invalid profile error = %v", err)
	}

	workflowOnly := coverageDescribedModule{
		coverageModule: coverageModule{name: "Workflow Only"},
		desc: ModuleDescriptor{
			ID:        "workflow-only",
			Name:      "Workflow Only",
			Workflows: []WorkflowDescriptor{coverageValidWorkflow(coverageValidProfile())},
		},
	}
	if err := r.Register(workflowOnly); !errors.Is(err, ErrInvalidMetadata) {
		t.Fatalf("workflow without profile error = %v", err)
	}

	profileNoWorkflows := coverageValidProfile()
	noWorkflows := coverageDescribedModule{
		coverageModule: coverageModule{name: "No Workflows"},
		desc:           ModuleDescriptor{ID: "no-workflows", Name: "No Workflows", Profile: &profileNoWorkflows},
	}
	if err := r.Register(noWorkflows); !errors.Is(err, ErrInvalidMetadata) {
		t.Fatalf("profile without workflows error = %v", err)
	}

	badWorkflow := coverageValidWorkflow(profileNoWorkflows)
	badWorkflow.RequirementIDs = []string{"unknown"}
	invalidWorkflow := coverageDescribedModule{
		coverageModule: coverageModule{name: "Invalid Workflow"},
		desc: ModuleDescriptor{
			ID:        "invalid-workflow",
			Name:      "Invalid Workflow",
			Profile:   &profileNoWorkflows,
			Workflows: []WorkflowDescriptor{badWorkflow},
		},
	}
	if err := r.Register(invalidWorkflow); !errors.Is(err, ErrInvalidMetadata) {
		t.Fatalf("invalid workflow error = %v", err)
	}

	unconfiguredProfile := coverageValidProfile()
	unconfiguredProfile.Edition = "NOT CONFIGURED"
	unconfigured := coverageDescribedModule{
		coverageModule: coverageModule{name: "Unconfigured"},
		desc: ModuleDescriptor{
			ID:        "unconfigured",
			Name:      "Unconfigured",
			Provides:  []Capability{CapDeferredTax, ""},
			Profile:   &unconfiguredProfile,
			Workflows: []WorkflowDescriptor{coverageValidWorkflow(unconfiguredProfile)},
		},
	}
	if err := r.Register(unconfigured); err != nil {
		t.Fatalf("register unconfigured: %v", err)
	}
	if r.HasCapability(CapDeferredTax) {
		t.Fatal("unconfigured module capability should be inactive")
	}
	reason, ok := r.GetDisabledReason("capability:" + string(CapDeferredTax))
	if !ok || !strings.Contains(reason, "host configuration") {
		t.Fatalf("disabled reason = %q, %v", reason, ok)
	}

	configured := coverageValidDescribedModule("configured", "Configured")
	if err := r.Register(configured); err != nil {
		t.Fatalf("register configured: %v", err)
	}
	if !r.HasCapability(CapRevenueRecognition) {
		t.Fatal("configured capability should be active")
	}
	if provider, ok := r.GetCapabilityProvider(CapRevenueRecognition); !ok || provider != "configured" {
		t.Fatalf("capability provider = %q, %v", provider, ok)
	}
	if _, ok := r.GetDisabledReason("capability:" + string(CapRevenueRecognition)); ok {
		t.Fatal("configured capability should not carry a disabled reason")
	}
}

func TestCoverageRegistryResolve(t *testing.T) {
	r := NewRegistry()
	base := coverageModule{name: "base", version: "2.0.0"}
	middle := coverageModule{name: "middle", version: "1.0.0", deps: []string{"base"}}
	leaf := coverageModule{name: "leaf", version: "1.0.0", deps: []string{"middle"}}
	for _, mod := range []coverageModule{leaf, middle, base} {
		if err := r.Register(mod); err != nil {
			t.Fatalf("register %s: %v", mod.name, err)
		}
	}
	ordered, err := r.Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(ordered) != 3 || ordered[0].Name() != "base" || ordered[1].Name() != "middle" || ordered[2].Name() != "leaf" {
		t.Fatalf("unexpected order: %v", []string{ordered[0].Name(), ordered[1].Name(), ordered[2].Name()})
	}
	diagnostics := r.Diagnostics()
	if len(diagnostics.DependencyOrder) != 3 || diagnostics.DependencyOrder[0] != "base" {
		t.Fatalf("diagnostics order = %v", diagnostics.DependencyOrder)
	}
	if len(diagnostics.ActiveModules) != 3 {
		t.Fatalf("diagnostics modules = %+v", diagnostics.ActiveModules)
	}

	missing := NewRegistry()
	if err := missing.Register(coverageModule{name: "needs-missing", deps: []string{"absent"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := missing.Resolve(); !errors.Is(err, ErrMissingDependency) {
		t.Fatalf("missing dependency error = %v", err)
	}

	optional := NewRegistry()
	if err := optional.Register(coverageModule{name: "optional-user", deps: []string{"optional-dep"}}); err != nil {
		t.Fatal(err)
	}
	rDesc := optional.descriptors["optionaluser"]
	rDesc.Requires[0].Optional = true
	optional.descriptors["optionaluser"] = rDesc
	if _, err := optional.Resolve(); err != nil {
		t.Fatalf("optional dependency should not block resolve: %v", err)
	}
	if reason, ok := optional.GetDisabledReason("module:optionaluser:optional_dep:optionaldep"); !ok || reason != "optional dependency not loaded" {
		t.Fatalf("optional dependency reason = %q, %v", reason, ok)
	}

	conflict := NewRegistry()
	if err := conflict.Register(coverageModule{name: "old-dep", version: "1.0.0"}); err != nil {
		t.Fatal(err)
	}
	if err := conflict.Register(coverageModule{name: "consumer", deps: []string{"old-dep"}}); err != nil {
		t.Fatal(err)
	}
	cDesc := conflict.descriptors["consumer"]
	cDesc.Requires[0].MinVersion = "2.0.0"
	conflict.descriptors["consumer"] = cDesc
	if _, err := conflict.Resolve(); !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("version conflict error = %v", err)
	}

	cycle := NewRegistry()
	if err := cycle.Register(coverageModule{name: "A", deps: []string{"B"}}); err != nil {
		t.Fatal(err)
	}
	if err := cycle.Register(coverageModule{name: "B", deps: []string{"A"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := cycle.Resolve(); !errors.Is(err, ErrCircularDependency) {
		t.Fatalf("cycle error = %v", err)
	}

	selfCycle := NewRegistry()
	if err := selfCycle.Register(coverageModule{name: "Self", deps: []string{"Self"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := selfCycle.Resolve(); !errors.Is(err, ErrCircularDependency) {
		t.Fatalf("self-cycle error = %v", err)
	}

	empty := NewRegistry()
	ordered, err = empty.Resolve()
	if err != nil || len(ordered) != 0 {
		t.Fatalf("empty registry resolve = %v, %v", ordered, err)
	}
}

func TestCoverageRegistryCapabilitiesAndDiagnostics(t *testing.T) {
	r := NewRegistry()
	before := r.ActiveCapabilities()
	if len(before) != 0 {
		t.Fatalf("empty registry active capabilities = %v", before)
	}
	r.RegisterCapability("", "ignored")
	r.SetDisabledReason("", "ignored")
	r.SetDisabledReason("module:disabled", "not loaded")
	if _, ok := r.GetDisabledReason("module:disabled"); !ok {
		t.Fatal("disabled reason was not stored")
	}
	r.RegisterCapability(CapReporting, "system")
	r.RegisterCapability(CapAuditTrail, "auditor")
	r.RegisterCapability(CapGovernance, "governance")
	caps := r.ActiveCapabilities()
	if len(caps) != 3 || caps[0] != CapAuditTrail || caps[1] != CapGovernance || caps[2] != CapReporting {
		t.Fatalf("sorted capabilities = %v", caps)
	}
	if !r.HasCapability(CapReporting) {
		t.Fatal("expected reporting capability")
	}
	if r.HasCapability(CapScheduledJobs) {
		t.Fatal("unexpected scheduled jobs capability")
	}
	if provider, ok := r.GetCapabilityProvider(CapReporting); !ok || provider != "system" {
		t.Fatalf("provider = %q, %v", provider, ok)
	}
	if _, ok := r.GetCapabilityProvider(CapConsolidation); ok {
		t.Fatal("unknown capability had a provider")
	}

	if err := r.Register(coverageModule{name: "simple", version: "1.0.0"}); err != nil {
		t.Fatal(err)
	}
	d := r.Diagnostics()
	if len(d.ActiveModules) != 1 || d.ActiveModules[0].ID != "simple" {
		t.Fatalf("unresolved diagnostics = %+v", d)
	}
	if len(d.ActiveCapabilities) != 3 || len(d.CapabilityProviders) != 3 {
		t.Fatalf("diagnostics capabilities = %+v", d)
	}
	if len(d.DisabledReasons) != 1 {
		t.Fatalf("diagnostics reasons = %+v", d.DisabledReasons)
	}
	d.DisabledReasons["injected"] = "value"
	d.CapabilityProviders[CapReporting] = "tampered"
	if _, ok := r.GetDisabledReason("injected"); ok {
		t.Fatal("diagnostics reasons must be copied")
	}
	if provider, _ := r.GetCapabilityProvider(CapReporting); provider != "system" {
		t.Fatal("diagnostics providers must be copied")
	}
}
