package reconciliation

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type coverageRepo struct{}

func (coverageRepo) Create(Reconciliation) error              { return nil }
func (coverageRepo) AddItem(uuid.UUID, Item) error            { return nil }
func (coverageRepo) List(uuid.UUID) ([]Reconciliation, error) { return nil, nil }

func coverageRequirements() Requirements {
	day := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	return Requirements{
		Left:               SourceRequirement{Name: "bank", Currency: "SAR", Start: day, End: day.Add(time.Hour)},
		Right:              SourceRequirement{Name: "ledger", Currency: "SAR", Start: day, End: day.Add(time.Hour)},
		MatchKeys:          []string{"reference"},
		Tolerance:          decimal.Zero,
		ExceptionTreatment: ExceptionReject,
	}
}

func TestCoverageRequirementsValidation(t *testing.T) {
	base := coverageRequirements()
	if err := base.Validate(); err != nil {
		t.Fatalf("valid requirements: %v", err)
	}
	mutations := []struct {
		name string
		edit func(*Requirements)
	}{
		{"blank left name", func(r *Requirements) { r.Left.Name = " " }},
		{"blank right name", func(r *Requirements) { r.Right.Name = "" }},
		{"same name", func(r *Requirements) { r.Right.Name = " BANK " }},
		{"blank left currency", func(r *Requirements) { r.Left.Currency = "" }},
		{"currency mismatch", func(r *Requirements) { r.Right.Currency = "USD" }},
		{"zero left start", func(r *Requirements) { r.Left.Start = time.Time{} }},
		{"zero right end", func(r *Requirements) { r.Right.End = time.Time{} }},
		{"start mismatch", func(r *Requirements) { r.Right.Start = r.Right.Start.Add(24 * time.Hour) }},
		{"end mismatch", func(r *Requirements) { r.Right.End = r.Right.End.Add(24 * time.Hour) }},
		{"negative window", func(r *Requirements) { r.Left.Start, r.Left.End = r.Left.End, r.Left.Start }},
		{"no match keys", func(r *Requirements) { r.MatchKeys = nil }},
		{"blank key", func(r *Requirements) { r.MatchKeys = []string{" "} }},
		{"duplicate key", func(r *Requirements) { r.MatchKeys = []string{"reference", " reference "} }},
		{"negative tolerance", func(r *Requirements) { r.Tolerance = decimal.NewFromInt(-1) }},
		{"unknown treatment", func(r *Requirements) { r.ExceptionTreatment = "ignore" }},
	}
	for _, tc := range mutations {
		t.Run(tc.name, func(t *testing.T) {
			r := base
			r.MatchKeys = append([]string(nil), base.MatchKeys...)
			tc.edit(&r)
			if err := r.Validate(); err == nil {
				t.Fatalf("expected validation error for %s", tc.name)
			}
		})
	}
	for _, treatment := range []ExceptionTreatment{ExceptionReject, ExceptionQueueForReview, ExceptionApprovedAdjust} {
		r := base
		r.ExceptionTreatment = treatment
		if err := r.Validate(); err != nil {
			t.Fatalf("treatment %s: %v", treatment, err)
		}
	}
}

func TestCoverageCapabilityConstructors(t *testing.T) {
	if _, err := NewCapability(nil, coverageRequirements()); err == nil {
		t.Fatal("nil repository must be rejected")
	}
	bad := coverageRequirements()
	bad.MatchKeys = nil
	if _, err := NewCapability(coverageRepo{}, bad); err == nil {
		t.Fatal("invalid requirements must be rejected")
	}
	capability, err := NewCapability(coverageRepo{}, coverageRequirements())
	if err != nil {
		t.Fatalf("NewCapability: %v", err)
	}
	got := capability.Requirements()
	got.MatchKeys[0] = "mutated"
	if capability.Requirements().MatchKeys[0] != "reference" {
		t.Fatal("Requirements must return a defensive copy")
	}
	var nilCapability *Capability
	if len(nilCapability.Requirements().MatchKeys) != 0 {
		t.Fatal("nil capability should have empty requirements")
	}
}

func TestCoverageCompareSourcesErrors(t *testing.T) {
	var nilCapability *Capability
	if _, err := nilCapability.CompareSources(context.Background(), ledgerSource{}, ledgerSource{}); err == nil {
		t.Fatal("nil capability must fail comparison")
	}
	capability, err := NewCapability(coverageRepo{}, coverageRequirements())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := capability.CompareSources(context.Background(), nil, ledgerSource{}); err == nil {
		t.Fatal("nil left source must fail comparison")
	}
	if _, err := capability.CompareSources(context.Background(), ledgerSource{}, nil); err == nil {
		t.Fatal("nil right source must fail comparison")
	}
	loadErr := fmt.Errorf("load failed")
	if _, err := capability.CompareSources(context.Background(), failingSource{err: loadErr}, ledgerSource{}); err == nil ||
		!strings.Contains(err.Error(), "load left source records") {
		t.Fatalf("left load error = %v", err)
	}
	if _, err := capability.CompareSources(context.Background(), ledgerSource{}, failingSource{err: loadErr}); err == nil ||
		!strings.Contains(err.Error(), "load right source records") {
		t.Fatalf("right load error = %v", err)
	}

	invalidCapability := &Capability{requirements: Requirements{}}
	if _, err := invalidCapability.CompareSources(context.Background(), ledgerSource{}, ledgerSource{}); err == nil {
		t.Fatal("invalid stored requirements must fail comparison")
	}
}

type failingSource struct{ err error }

func (s failingSource) LoadRecords(context.Context, time.Time, time.Time, string) ([]SourceRecord, error) {
	return nil, s.err
}

func TestCoverageCompareSourcesToleranceAndExceptions(t *testing.T) {
	req := coverageRequirements()
	req.Tolerance = decimal.NewFromInt(2)
	req.ExceptionTreatment = ExceptionQueueForReview
	capability, err := NewCapability(coverageRepo{}, req)
	if err != nil {
		t.Fatal(err)
	}
	day := req.Left.Start
	left := ledgerSource{records: []SourceRecord{
		{ID: "l1", MatchKey: " ref-1 ", Amount: decimal.NewFromInt(100), Date: day},
		{ID: "l2", MatchKey: "ref-2", Amount: decimal.NewFromInt(200), Date: day},
		{ID: "l3", MatchKey: "ref-3", Amount: decimal.NewFromInt(300), Date: day},
	}}
	right := ledgerSource{records: []SourceRecord{
		{ID: "r1", MatchKey: "ref-1", Amount: decimal.NewFromInt(99), Date: day},
		{ID: "r1b", MatchKey: "ref-1", Amount: decimal.NewFromInt(50), Date: day},
		{ID: "r2", MatchKey: "ref-2", Amount: decimal.NewFromInt(180), Date: day},
		{ID: "r4", MatchKey: "ref-4", Amount: decimal.NewFromInt(400), Date: day},
	}}
	result, err := capability.CompareSources(context.Background(), left, right)
	if err != nil {
		t.Fatalf("CompareSources: %v", err)
	}
	if len(result.Matched) != 1 || result.Matched[0].Left.ID != "l1" || len(result.UnmatchedLeft) != 2 ||
		len(result.UnmatchedRight) != 3 || result.WithinTolerance || result.DifferenceTotal.IsZero() {
		t.Fatalf("comparison result = %+v", result)
	}
	if len(result.Exceptions) == 0 {
		t.Fatal("exceptions should describe unmatched items")
	}

	req.Tolerance = decimal.NewFromInt(500)
	capability, err = NewCapability(coverageRepo{}, req)
	if err != nil {
		t.Fatal(err)
	}
	result, err = capability.CompareSources(context.Background(), ledgerSource{records: left.records[:1]}, ledgerSource{records: right.records[:1]})
	if err != nil || !result.WithinTolerance {
		t.Fatalf("tolerance result = %+v, %v", result, err)
	}
}

func TestCoverageParseStatementErrors(t *testing.T) {
	if _, err := ParseStatement([]byte("   "), StatementFormatAuto); err == nil {
		t.Fatal("empty statement payload must fail")
	}
	if _, err := ParseStatement([]byte("unrecognised"), StatementFormatAuto); err == nil {
		t.Fatal("undetectable statement format must fail")
	}
	if _, err := ParseStatement([]byte(mt940Fixture), StatementFormat("csv")); err == nil {
		t.Fatal("unsupported explicit format must fail")
	}
}

func TestCoverageStatementSourceFilters(t *testing.T) {
	statement, err := ParseStatement([]byte(mt940Fixture), StatementFormatMT940)
	if err != nil {
		t.Fatal(err)
	}
	source := statementSource{statement: statement}
	day := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	records, err := source.LoadRecords(context.Background(), day.Add(time.Hour), day.Add(2*time.Hour), "USD")
	if err != nil || len(records) != 0 {
		t.Fatalf("filtered records = %v, %v", records, err)
	}
	if _, err := (statementSource{}).LoadRecords(context.Background(), day, day, "SAR"); err == nil {
		t.Fatal("nil statement must fail")
	}

	if got := transactionByReference(nil, "missing"); got.ReferenceNumber != "" {
		t.Fatalf("nil statement transaction = %+v", got)
	}
	if got := transactionByReference(statement, "missing"); got.ReferenceNumber != "missing" {
		t.Fatalf("unknown transaction fallback = %+v", got)
	}
}

func TestCoverageReconcileStatementErrors(t *testing.T) {
	var nilMatcher *Matcher
	report, err := nilMatcher.ReconcileStatement(context.Background(), []byte(mt940Fixture), StatementFormatMT940, uuid.New(), uuid.New(), cashSource{})
	if err == nil || !strings.Contains(err.Error(), "matcher is nil") {
		t.Fatalf("nil matcher error = %v", err)
	}
	if report.GeneratedAt.IsZero() {
		t.Fatal("report must be populated before validation")
	}

	matcher, err := NewMatcher(coverageRequirements())
	if err != nil {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := matcher.ReconcileStatement(cancelled, []byte(mt940Fixture), StatementFormatMT940, uuid.New(), uuid.New(), cashSource{}); err == nil {
		t.Fatal("cancelled context must fail")
	}
	if _, err := matcher.ReconcileStatement(nil, []byte(mt940Fixture), StatementFormatMT940, uuid.New(), uuid.New(), cashSource{}); err == nil {
		t.Fatal("nil context should fall back to background and still reconcile")
	}
	if _, err := matcher.ReconcileStatement(context.Background(), []byte(mt940Fixture), StatementFormatMT940, uuid.New(), uuid.New(), nil); err == nil {
		t.Fatal("nil cash source must fail")
	}
	if _, err := matcher.ReconcileStatement(context.Background(), []byte(mt940Fixture), StatementFormatMT940, uuid.New(), uuid.Nil, cashSource{}); err == nil {
		t.Fatal("nil account ID must fail")
	}
	if _, err := matcher.ReconcileStatement(context.Background(), []byte(mt940Fixture), StatementFormatMT940, uuid.New(), uuid.New(), cashSource{err: fmt.Errorf("cash unavailable")}); err == nil {
		t.Fatal("cash load failure must be returned")
	}

	broken := []byte(`:20:STATEMENT-2
:25:SA0380000000608010167519
:60F:C261109SAR1000,00
:61:261110D100,50NTRFNONREF//PAY-44
:86:Supplier payment
:62F:C261110SAR999,99`)
	if _, err := matcher.ReconcileStatement(context.Background(), broken, StatementFormatMT940, uuid.New(), uuid.New(), cashSource{}); err == nil {
		t.Fatal("balance tie-out failure must be returned")
	}
}

func TestCoverageMatcherRequirementsDefensiveCopy(t *testing.T) {
	matcher, err := NewMatcher(coverageRequirements())
	if err != nil {
		t.Fatal(err)
	}
	got := matcher.Requirements()
	got.MatchKeys[0] = "mutated"
	if matcher.Requirements().MatchKeys[0] != "reference" {
		t.Fatal("Matcher.Requirements must return a defensive copy")
	}
	var nilMatcher *Matcher
	if len(nilMatcher.Requirements().MatchKeys) != 0 {
		t.Fatal("nil matcher should have empty requirements")
	}
}
