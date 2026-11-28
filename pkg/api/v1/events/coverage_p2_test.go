package events

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/journal"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
)

func TestCoverageDimensionsRouter(t *testing.T) {
	router := NewDimensionsRouter(nil)
	if router.mappings == nil {
		t.Fatal("nil mappings must be replaced by an empty map")
	}
	withMappings := NewDimensionsRouter(map[string]string{"entity": "mapped"})
	if withMappings.mappings["entity"] != "mapped" {
		t.Fatal("mappings were not retained")
	}

	id := uuid.New()
	resolved := router.ResolveDimensionID(map[string]string{"entity": id.String()}, "entity")
	if resolved == nil || *resolved != types.CompanyID(id) {
		t.Fatalf("resolved dimension = %v", resolved)
	}
	if router.ResolveDimensionID(map[string]string{"entity": "not-a-uuid"}, "entity") != nil {
		t.Fatal("invalid UUID must not resolve")
	}
	if router.ResolveDimensionID(map[string]string{}, "entity") != nil {
		t.Fatal("missing key must not resolve")
	}
}

func TestCoverageRouterRules(t *testing.T) {
	router := NewRouter()
	var registrar RuleRegistrar = router
	if len(router.GetRules("missing")) != 0 {
		t.Fatal("unknown event type must have no rules")
	}
	first := PostingRule{RuleID: "1", EventType: "invoice.created", JournalCode: "GEN"}
	second := PostingRule{RuleID: "2", EventType: "invoice.created", JournalCode: "SAL"}
	registrar.RegisterRule(first)
	registrar.RegisterRule(second)
	rules := router.GetRules("invoice.created")
	if len(rules) != 2 || rules[0].RuleID != "1" || rules[1].RuleID != "2" {
		t.Fatalf("rules = %+v", rules)
	}
	rules[0].RuleID = "mutated"
	if router.GetRules("invoice.created")[0].RuleID != "1" {
		t.Fatal("GetRules must return a copy")
	}

	event := AccountingEvent{
		EventID:       uuid.NewString(),
		EventType:     "invoice.created",
		CompanyID:     types.NewCompanyID(),
		EffectiveDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:        types.Money{},
		Dimensions:    map[string]string{"entity": uuid.NewString()},
		Payload:       map[string]any{"amount": "100"},
		JournalDraft:  &journal.JournalEntry{},
	}
	if event.JournalDraft == nil || event.Dimensions["entity"] == "" {
		t.Fatal("event value was not populated")
	}
	if _, err := EvaluateAndConstructEntry(first, event); !errors.Is(err, types.ErrCapabilityNotImplemented) {
		t.Fatalf("EvaluateAndConstructEntry error = %v", err)
	}
}
