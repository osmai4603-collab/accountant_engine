package contract_parity

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	publicmasterdata "github.com/osmai4603-collab/accounting_engine/pkg/spi/masterdata"
	"github.com/osmai4603-collab/accounting_engine/storage/memory"
)

func TestCoverageParityReporterBranches(t *testing.T) {
	reporter := &ParityReporter{}
	reporter.Record(ScenarioResult{
		ScenarioName:          "second",
		AdapterName:           "memory",
		EngineVersion:         "(devel)",
		Passed:                true,
		CapabilityDifferences: []string{"durability=false"},
		SemanticDifferences:   []string{"documented: b"},
		UnexpectedDifferences: []string{"unexpected: b"},
		ExecutionDuration:     2 * time.Millisecond,
	})
	reporter.Record(ScenarioResult{
		ScenarioName:        "first",
		AdapterName:         "memory",
		Passed:              true,
		SemanticDifferences: []string{"documented: a", "pipe|cell"},
		ExecutionDuration:   time.Millisecond,
	})
	reporter.Record(ScenarioResult{
		ScenarioName:          "first",
		AdapterName:           "postgres",
		Skipped:               true,
		CapabilityDifferences: []string{"durability=true"},
		UnexpectedDifferences: []string{"unexpected: a"},
	})
	reporter.Record(ScenarioResult{
		ScenarioName: "second",
		AdapterName:  "postgres",
		Passed:       false,
	})

	report := reporter.GenerateMarkdownReport()
	for _, want := range []string{
		"| (devel) | first | memory | PASS |",
		"| (devel) | first | postgres | SKIP |",
		"| (devel) | second | memory | PASS |",
		"| (devel) | second | postgres | FAIL |",
		"**Totals:** 2 passed, 1 failed, 1 skipped.",
		"- memory: durability=false",
		"- postgres: durability=true",
		"- first / memory: documented: a",
		"- second / memory: documented: b",
		"- first / postgres: unexpected: a",
		"- second / memory: unexpected: b",
	} {
		if !strings.Contains(report, want) {
			t.Fatalf("report missing %q:\n%s", want, report)
		}
	}
	if !strings.Contains(report, `pipe\|cell`) {
		t.Fatal("report should escape table-cell separators")
	}
	if got := joinDifferences([]string{"a", "b"}); got != "a; b" {
		t.Fatalf("joinDifferences = %q", got)
	}
	if got := escapeCell("a|b"); got != `a\|b` {
		t.Fatalf("escapeCell = %q", got)
	}
	if version := EngineVersion(); version == "" {
		t.Fatal("EngineVersion must not be empty")
	}
}

func TestCoverageWriteReportToFileErrors(t *testing.T) {
	reporter := &ParityReporter{}
	reporter.Record(ScenarioResult{ScenarioName: "example", AdapterName: "memory", Passed: true})

	blocked := filepath.Join(string(filepath.Separator), "proc", "coverage-parity", "report.md")
	if err := reporter.WriteReportToFile(blocked); err == nil {
		t.Fatal("invalid report directory should fail")
	}

	if err := reporter.WriteReportToFile(t.TempDir()); err == nil {
		t.Fatal("writing a report over a directory should fail")
	}
}

func TestCoverageDeclareSemanticDifference(t *testing.T) {
	var nilHarness *StorageAdapterHarness
	nilHarness.DeclareSemanticDifference("ignored")

	h := &StorageAdapterHarness{}
	h.DeclareSemanticDifference("  ")
	h.DeclareSemanticDifference(" documented ")
	if len(h.SemanticDifferences) != 1 || h.SemanticDifferences[0] != " documented " {
		t.Fatalf("semantic differences = %v", h.SemanticDifferences)
	}
}

type coverageFailingStore struct {
	publicmasterdata.Store
	failAt string
}

func (s coverageFailingStore) CreateCompany(ctx context.Context, c publicmasterdata.Company) error {
	if s.failAt == "company" {
		return errors.New("company failed")
	}
	return s.Store.CreateCompany(ctx, c)
}

func (s coverageFailingStore) CreateJournal(ctx context.Context, j publicmasterdata.Journal) error {
	if s.failAt == "journal" {
		return errors.New("journal failed")
	}
	return s.Store.CreateJournal(ctx, j)
}

func (s coverageFailingStore) CreateAccount(ctx context.Context, a publicmasterdata.Account) error {
	if s.failAt == "account" {
		return errors.New("account failed")
	}
	return s.Store.CreateAccount(ctx, a)
}

func (s coverageFailingStore) CreateFiscalYear(ctx context.Context, y publicmasterdata.FiscalYear) error {
	if s.failAt == "fiscal-year" {
		return errors.New("fiscal year failed")
	}
	return s.Store.CreateFiscalYear(ctx, y)
}

func (s coverageFailingStore) CreateFiscalPeriod(ctx context.Context, p publicmasterdata.FiscalPeriod) error {
	if s.failAt == "open-period" || s.failAt == "closed-period" {
		return errors.New("fiscal period failed")
	}
	return s.Store.CreateFiscalPeriod(ctx, p)
}

func TestCoverageProvisionFixtureErrors(t *testing.T) {
	for _, failAt := range []string{"company", "journal", "account", "fiscal-year", "open-period", "closed-period"} {
		t.Run(failAt, func(t *testing.T) {
			store := coverageFailingStore{
				Store:  memory.New().MasterDataStore(),
				failAt: failAt,
			}
			if err := ProvisionFixture(context.Background(), store, NewUnifiedFixture()); err == nil {
				t.Fatalf("expected ProvisionFixture failure at %s", failAt)
			}
		})
	}
}
