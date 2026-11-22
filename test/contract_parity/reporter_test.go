package contract_parity

import (
	"strings"
	"testing"
	"time"
)

func TestParityReporterDoesNotInventSemanticDifferences(t *testing.T) {
	reporter := &ParityReporter{}
	reporter.Record(ScenarioResult{
		ScenarioName: "example",
		AdapterName:  "memory",
		Passed:       true,
	})

	report := reporter.GenerateMarkdownReport()
	if strings.Contains(report, "none; storage contract assertions passed") {
		t.Fatal("report contains a hard-coded semantic difference")
	}
	if !strings.Contains(report, "not observed") {
		t.Fatal("report does not state that no semantic difference was observed")
	}
	if !strings.Contains(report, "None declared by the executed scenarios.") {
		t.Fatal("report does not distinguish declared differences from observations")
	}
}

func TestParityReporterRecordsDeclaredSemanticDifferences(t *testing.T) {
	reporter := &ParityReporter{}
	reporter.Record(ScenarioResult{
		ScenarioName:        "example",
		AdapterName:         "memory",
		Passed:              true,
		SemanticDifferences: []string{"documented capability difference"},
		ExecutionDuration:   time.Millisecond,
	})

	report := reporter.GenerateMarkdownReport()
	if !strings.Contains(report, "example / memory: documented capability difference") {
		t.Fatal("declared semantic difference missing from report")
	}
}
