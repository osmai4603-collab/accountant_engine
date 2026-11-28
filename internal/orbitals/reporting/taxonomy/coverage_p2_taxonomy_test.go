package taxonomy

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/osmai4603-collab/accounting_engine/internal/kernel/masterdata"
	financialtypes "github.com/osmai4603-collab/accounting_engine/internal/kernel/types"
	"github.com/shopspring/decimal"
)

type covBalanceReaderFunc func(context.Context, uuid.UUID, uuid.UUID, string) (PeriodBalances, error)

func (f covBalanceReaderFunc) GetPeriodBalances(
	ctx context.Context,
	companyID, periodID uuid.UUID,
	currency string,
) (PeriodBalances, error) {
	return f(ctx, companyID, periodID, currency)
}

func covTaxAccount(accountType masterdata.AccountType, sub, ifrs string, effectiveTo *time.Time) masterdata.Account {
	return masterdata.Account{
		CompanyID:         financialtypes.CompanyID(uuid.New()),
		ID:                financialtypes.AccountID(uuid.New()),
		Code:              "1000",
		Name:              "Coverage account",
		Type:              accountType,
		SubClassification: sub,
		IFRSCategory:      ifrs,
		EffectiveTo:       effectiveTo,
	}
}

func covReportNode(code string, nodeType NodeType) *ReportNode {
	return &ReportNode{
		ID:           uuid.New(),
		Code:         code,
		TitleAr:      "عنوان",
		TitleEn:      "Title " + code,
		NodeType:     nodeType,
		DisplayOrder: 1,
	}
}

func covDefinition(nodes ...*ReportNode) FinancialReportDefinition {
	return FinancialReportDefinition{
		ID:        uuid.New(),
		Code:      "COVERAGE",
		Name:      "Coverage statement",
		Framework: FrameworkIFRS,
		Type:      StatementIncome,
		RootNodes: nodes,
		Version:   1,
		CreatedAt: time.Now().UTC(),
	}
}

func covRequireTags(t *testing.T, tags []string, expected ...string) {
	t.Helper()
	available := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		available[tag] = struct{}{}
	}
	for _, tag := range expected {
		if _, ok := available[tag]; !ok {
			t.Fatalf("tag %q missing from %v", tag, tags)
		}
	}
}

func TestCoverageTagsForAccountBranches(t *testing.T) {
	cases := []struct {
		name     string
		account  masterdata.Account
		expected []string
	}{
		{"operating revenue", covTaxAccount(masterdata.AccountTypeIncome, masterdata.SubClassOperatingRevenue, "", nil), []string{"revenue"}},
		{"other operating income", covTaxAccount(masterdata.AccountTypeIncome, masterdata.SubClassOtherOperatingIncome, "", nil), []string{"revenue"}},
		{"cost of sales", covTaxAccount(masterdata.AccountTypeExpense, masterdata.SubClassCostOfSales, ifrsCategoryOperating, nil), []string{"cost_of_sales", "materials_consumed"}},
		{"selling expense", covTaxAccount(masterdata.AccountTypeExpense, masterdata.SubClassSellingDistributionExpense, ifrsCategoryOperating, nil), []string{"selling_expense"}},
		{"admin expense", covTaxAccount(masterdata.AccountTypeExpense, masterdata.SubClassGeneralAdminExpense, ifrsCategoryOperating, nil), []string{"administrative_expense", "employee_expense"}},
		{"depreciation", covTaxAccount(masterdata.AccountTypeExpense, masterdata.SubClassDepreciationAmortExpense, ifrsCategoryOperating, nil), []string{"depreciation_expense"}},
		{"expected credit loss", covTaxAccount(masterdata.AccountTypeExpense, masterdata.SubClassExpectedCreditLossExpense, ifrsCategoryOperating, nil), []string{"depreciation_expense"}},
		{"other operating expense", covTaxAccount(masterdata.AccountTypeExpense, masterdata.SubClassOtherOperatingExpense, ifrsCategoryOperating, nil), []string{"other_operating_expense"}},
		{"finance income", covTaxAccount(masterdata.AccountTypeIncome, masterdata.SubClassFinanceIncome, "", nil), []string{"investing_revenue", "nonoperating_income"}},
		{"finance cost", covTaxAccount(masterdata.AccountTypeExpense, masterdata.SubClassFinanceCost, "", nil), []string{"finance_cost"}},
		{"tax expense", covTaxAccount(masterdata.AccountTypeExpense, masterdata.SubClassTaxExpense, "", nil), []string{"income_tax"}},
		{"income tax expense", covTaxAccount(masterdata.AccountTypeExpense, masterdata.SubClassIncomeTaxExpense, "", nil), []string{"income_tax"}},
		{"oci non recyclable", covTaxAccount(masterdata.AccountTypeIncome, subClassOCINonRecyclable, "", nil), []string{"oci_income", "oci"}},
		{"oci recyclable", covTaxAccount(masterdata.AccountTypeIncome, subClassOCIRecyclable, "", nil), []string{"oci_income", "oci"}},
		{"current asset", covTaxAccount(masterdata.AccountTypeAsset, masterdata.SubClassCashAndEquivalents, "", nil), []string{"current_asset"}},
		{"noncurrent asset", covTaxAccount(masterdata.AccountTypeAsset, masterdata.SubClassPropertyPlantEquipment, "", nil), []string{"noncurrent_asset"}},
		{"current liability", covTaxAccount(masterdata.AccountTypeLiability, masterdata.SubClassTradePayables, "", nil), []string{"current_liability"}},
		{"noncurrent liability", covTaxAccount(masterdata.AccountTypeLiability, masterdata.SubClassLongTermBorrowings, "", nil), []string{"noncurrent_liability"}},
		{"equity", covTaxAccount(masterdata.AccountTypeEquity, masterdata.SubClassRetainedEarnings, "", nil), []string{"equity"}},
		{"income operating category", covTaxAccount(masterdata.AccountTypeIncome, "", ifrsCategoryOperating, nil), []string{"revenue"}},
		{"income investing category", covTaxAccount(masterdata.AccountTypeIncome, "", ifrsCategoryInvesting, nil), []string{"investing_revenue", "nonoperating_income"}},
		{"income financing category", covTaxAccount(masterdata.AccountTypeIncome, "", ifrsCategoryFinancing, nil), []string{"financing_revenue", "nonoperating_income"}},
		{"income tax category", covTaxAccount(masterdata.AccountTypeIncome, "", ifrsCategoryTax, nil), []string{"income_tax"}},
		{"income income taxes category", covTaxAccount(masterdata.AccountTypeIncome, "", ifrsCategoryIncomeTaxes, nil), []string{"income_tax"}},
		{"income equity movements", covTaxAccount(masterdata.AccountTypeIncome, "", ifrsCategoryEquityMovements, nil), []string{"oci_income", "oci"}},
		{"income discontinued", covTaxAccount(masterdata.AccountTypeIncome, "", ifrsCategoryDiscontinued, nil), []string{"discontinued_income"}},
		{"income discontinued operations", covTaxAccount(masterdata.AccountTypeIncome, "", ifrsCategoryDiscontinuedOperations, nil), []string{"discontinued_income"}},
		{"expense tax category", covTaxAccount(masterdata.AccountTypeExpense, "", ifrsCategoryTax, nil), []string{"income_tax"}},
		{"expense income taxes category", covTaxAccount(masterdata.AccountTypeExpense, "", ifrsCategoryIncomeTaxes, nil), []string{"income_tax"}},
		{"expense investing category", covTaxAccount(masterdata.AccountTypeExpense, "", ifrsCategoryInvesting, nil), []string{"investing_expense"}},
		{"expense financing category", covTaxAccount(masterdata.AccountTypeExpense, "", ifrsCategoryFinancing, nil), []string{"finance_cost"}},
		{"expense equity movements", covTaxAccount(masterdata.AccountTypeExpense, "", ifrsCategoryEquityMovements, nil), []string{"oci_expense", "oci"}},
		{"expense discontinued", covTaxAccount(masterdata.AccountTypeExpense, "", ifrsCategoryDiscontinued, nil), []string{"discontinued_expense"}},
		{"expense discontinued operations", covTaxAccount(masterdata.AccountTypeExpense, "", ifrsCategoryDiscontinuedOperations, nil), []string{"discontinued_expense"}},
		{"expense operating default", covTaxAccount(masterdata.AccountTypeExpense, "", ifrsCategoryOperating, nil), []string{"other_operating_expense"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			covRequireTags(t, TagsForAccount(tc.account), tc.expected...)
		})
	}
	got := uniqueTagStrings([]string{" Asset ", "asset", "", " ", "LIABILITY"})
	if len(got) != 2 || got[0] != "asset" || got[1] != "liability" {
		t.Fatalf("uniqueTagStrings = %v", got)
	}
}

func TestCoverageTaxonomyCurrentClassificationHelpers(t *testing.T) {
	if taxonomyIsCurrentAsset(covTaxAccount(masterdata.AccountTypeLiability, masterdata.SubClassCashAndEquivalents, "", nil)) {
		t.Fatal("non-asset classified as current asset")
	}
	for _, sub := range []string{
		masterdata.SubClassCashAndEquivalents, masterdata.SubClassTradeReceivables,
		masterdata.SubClassInventories, masterdata.SubClassPrepaymentsOtherDebit,
		masterdata.SubClassCurrentAssets, "bank", "CASH", " accounts_receivable ",
	} {
		if !taxonomyIsCurrentAsset(covTaxAccount(masterdata.AccountTypeAsset, sub, "", nil)) {
			t.Fatalf("asset subclass %q should be current", sub)
		}
	}
	if taxonomyIsCurrentAsset(covTaxAccount(masterdata.AccountTypeAsset, masterdata.SubClassPropertyPlantEquipment, "", nil)) {
		t.Fatal("PPE classified as current asset")
	}

	if taxonomyIsCurrentLiability(covTaxAccount(masterdata.AccountTypeAsset, masterdata.SubClassTradePayables, "", nil)) {
		t.Fatal("non-liability classified as current liability")
	}
	for _, sub := range []string{
		masterdata.SubClassShortTermBorrowings, masterdata.SubClassCurrentLeaseLiabilities,
		masterdata.SubClassTradePayables, masterdata.SubClassAccrualsOtherCredit,
		masterdata.SubClassCurrentTaxLiabilities, masterdata.SubClassCurrentLiabilities,
		"accounts_payable", " PAYABLE ",
	} {
		if !taxonomyIsCurrentLiability(covTaxAccount(masterdata.AccountTypeLiability, sub, "", nil)) {
			t.Fatalf("liability subclass %q should be current", sub)
		}
	}
	if taxonomyIsCurrentLiability(covTaxAccount(masterdata.AccountTypeLiability, masterdata.SubClassLongTermBorrowings, "", nil)) {
		t.Fatal("long-term debt classified as current liability")
	}

	if !taxonomyIsCurrent(covTaxAccount(masterdata.AccountTypeAsset, masterdata.SubClassTradeReceivables, "", nil)) {
		t.Fatal("expected current asset")
	}
	if taxonomyIsCurrent(covTaxAccount(masterdata.AccountTypeLiability, masterdata.SubClassLongTermBorrowings, "", nil)) {
		t.Fatal("expected noncurrent liability")
	}
	if !taxonomyIsCurrent(covTaxAccount(masterdata.AccountTypeEquity, "", "", nil)) {
		t.Fatal("expected active equity account to be current")
	}
	past := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	if taxonomyIsCurrent(covTaxAccount(masterdata.AccountTypeEquity, "", "", &past)) {
		t.Fatal("expected expired equity account not to be current")
	}
}

func TestCoverageFormulaParserBranches(t *testing.T) {
	values := map[string]decimal.Decimal{
		"A": decimal.NewFromInt(10),
		"B": decimal.NewFromInt(4),
	}
	success := []struct {
		expression string
		want       decimal.Decimal
	}{
		{"1 + 2 * (3 - 4) / -2", decimal.NewFromInt(2)},
		{"+3", decimal.NewFromInt(3)},
		{"-(4 + 1)", decimal.NewFromInt(-5)},
		{"A + B", decimal.NewFromInt(14)},
		{"1.5 * 2", decimal.NewFromInt(3)},
	}
	for _, tc := range success {
		got, err := evaluateFormula(tc.expression, values)
		if err != nil || !got.Equal(tc.want) {
			t.Fatalf("evaluateFormula(%q) = %s, %v; want %s", tc.expression, got, err, tc.want)
		}
	}

	failures := []struct {
		expression string
		wantErr    error
		contains   string
	}{
		{"1 / 0", nil, "division by zero"},
		{"1..2", nil, "invalid decimal number"},
		{".", nil, "invalid decimal number"},
		{"1 +", nil, "expected a value"},
		{"(1 + 2", nil, "missing closing parenthesis"},
		{"1 2", nil, "unexpected character"},
		{"#", nil, "unexpected character"},
		{"UNKNOWN", ErrUnknownFormulaRef, ""},
		{strings.Repeat("(", 65) + "1" + strings.Repeat(")", 65), nil, "formula nesting exceeds 64 levels"},
		{strings.Repeat("+", 65) + "1", nil, "formula nesting exceeds 64 levels"},
		{strings.Repeat("1", 4097), nil, "formula exceeds 4096 characters"},
	}
	for _, tc := range failures {
		_, err := evaluateFormula(tc.expression, values)
		if err == nil {
			t.Fatalf("evaluateFormula(%q) unexpectedly succeeded", tc.expression)
		}
		if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
			t.Fatalf("evaluateFormula(%q) error = %v; want %v", tc.expression, err, tc.wantErr)
		}
		if tc.contains != "" && !strings.Contains(err.Error(), tc.contains) {
			t.Fatalf("evaluateFormula(%q) error = %v; want substring %q", tc.expression, err, tc.contains)
		}
	}

	refs := formulaReferences("alpha + B_2 - # 9")
	if len(refs) != 2 || refs[0] != "ALPHA" || refs[1] != "B_2" {
		t.Fatalf("formulaReferences = %v", refs)
	}
	parser := &formulaParser{input: []rune("#")}
	if _, ok := parser.readIdentifier(); ok {
		t.Fatal("readIdentifier unexpectedly consumed a non-identifier")
	}
}

func TestCoverageTaxonomyEvaluatorValidation(t *testing.T) {
	ctx := context.Background()
	companyID, periodID := uuid.New(), uuid.New()
	reader := covBalanceReaderFunc(func(context.Context, uuid.UUID, uuid.UUID, string) (PeriodBalances, error) {
		return PeriodBalances{}, nil
	})
	validDef := covDefinition(covReportNode("ASSET", NodeHeader))

	var nilEvaluator *TaxonomyEvaluator
	if _, err := nilEvaluator.EvaluateStatement(ctx, companyID, validDef, periodID, "USD"); err == nil {
		t.Fatal("expected error for nil evaluator")
	}
	if _, err := NewTaxonomyEvaluator(nil).EvaluateStatement(ctx, companyID, validDef, periodID, "USD"); err == nil {
		t.Fatal("expected error for missing balance reader")
	}
	if _, err := NewTaxonomyEvaluator(reader).EvaluateStatement(nil, companyID, validDef, periodID, "USD"); err == nil {
		t.Fatal("expected error for nil context")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := NewTaxonomyEvaluator(reader).EvaluateStatement(cancelled, companyID, validDef, periodID, "USD"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled context error = %v", err)
	}
	if _, err := NewTaxonomyEvaluator(reader).EvaluateStatement(ctx, uuid.Nil, validDef, periodID, "USD"); err == nil {
		t.Fatal("expected error for nil company")
	}
	if _, err := NewTaxonomyEvaluator(reader).EvaluateStatement(ctx, companyID, validDef, uuid.Nil, "USD"); err == nil {
		t.Fatal("expected error for nil period")
	}
	if _, err := NewTaxonomyEvaluator(reader).EvaluateStatement(ctx, companyID, validDef, periodID, " "); err == nil {
		t.Fatal("expected error for blank currency")
	}

	failingReader := covBalanceReaderFunc(func(context.Context, uuid.UUID, uuid.UUID, string) (PeriodBalances, error) {
		return PeriodBalances{}, errors.New("reader offline")
	})
	if _, err := NewTaxonomyEvaluator(failingReader).EvaluateStatement(ctx, companyID, validDef, periodID, "USD"); err == nil ||
		!strings.Contains(err.Error(), "reader offline") {
		t.Fatalf("reader error = %v", err)
	}

	formulaNode := covReportNode("TOTAL", NodeFormula)
	formulaNode.FormulaExpr = "MISSING + 1"
	if _, err := NewTaxonomyEvaluator(reader).EvaluateStatement(
		ctx, companyID, covDefinition(formulaNode), periodID, "USD",
	); !errors.Is(err, ErrUnknownFormulaRef) {
		t.Fatalf("formula evaluation error = %v", err)
	}

	evaluator := NewTaxonomyEvaluator(reader)
	evaluator.now = nil
	report, err := evaluator.EvaluateStatement(ctx, companyID, covDefinition(covReportNode("HEADER", NodeHeader)), periodID, "USD")
	if err != nil || report.GeneratedAt.IsZero() {
		t.Fatalf("evaluate with default clock: report=%+v err=%v", report, err)
	}
}

func TestCoverageValidateDefinitionBranches(t *testing.T) {
	baseNode := func() *ReportNode { return covReportNode("ROOT", NodeHeader) }
	cases := []struct {
		name   string
		mutate func(*FinancialReportDefinition)
	}{
		{"blank code", func(def *FinancialReportDefinition) { def.Code = " " }},
		{"blank name", func(def *FinancialReportDefinition) { def.Name = " " }},
		{"invalid framework", func(def *FinancialReportDefinition) { def.Framework = "OTHER" }},
		{"invalid statement type", func(def *FinancialReportDefinition) { def.Type = "OTHER" }},
		{"no roots", func(def *FinancialReportDefinition) { def.RootNodes = nil }},
		{"nil node", func(def *FinancialReportDefinition) { def.RootNodes = []*ReportNode{nil} }},
		{"missing titles", func(def *FinancialReportDefinition) { def.RootNodes[0].TitleAr = " " }},
		{"duplicate code", func(def *FinancialReportDefinition) {
			def.RootNodes = []*ReportNode{baseNode(), baseNode()}
		}},
		{"duplicate id", func(def *FinancialReportDefinition) {
			second := covReportNode("SECOND", NodeHeader)
			second.ID = def.RootNodes[0].ID
			def.RootNodes = []*ReportNode{def.RootNodes[0], second}
		}},
		{"invalid sign", func(def *FinancialReportDefinition) { def.RootNodes[0].SignMultiplier = 2 }},
		{"header with range", func(def *FinancialReportDefinition) { def.RootNodes[0].AccountFrom = "1000" }},
		{"header with tags", func(def *FinancialReportDefinition) { def.RootNodes[0].AccountTags = []string{"cash"} }},
		{"unbounded range", func(def *FinancialReportDefinition) {
			def.RootNodes[0].NodeType = NodeAccountRange
		}},
		{"reversed range", func(def *FinancialReportDefinition) {
			node := def.RootNodes[0]
			node.NodeType = NodeAccountRange
			node.AccountFrom = "Z"
			node.AccountTo = "A"
		}},
		{"tag node without tags", func(def *FinancialReportDefinition) {
			def.RootNodes[0].NodeType = NodeAccountTag
		}},
		{"tag node with blank tag", func(def *FinancialReportDefinition) {
			def.RootNodes[0].NodeType = NodeAccountTag
			def.RootNodes[0].AccountTags = []string{" "}
		}},
		{"formula without expression", func(def *FinancialReportDefinition) {
			def.RootNodes[0].NodeType = NodeFormula
		}},
		{"subtotal without children", func(def *FinancialReportDefinition) {
			def.RootNodes[0].NodeType = NodeSubtotal
		}},
		{"unsupported node type", func(def *FinancialReportDefinition) {
			def.RootNodes[0].NodeType = NodeType("UNKNOWN")
		}},
		{"excessive depth", func(def *FinancialReportDefinition) {
			parent := def.RootNodes[0]
			for i := 0; i <= maxTaxonomyNodeDepth; i++ {
				child := covReportNode(fmt.Sprintf("CHILD%d", i), NodeHeader)
				parent.Children = []*ReportNode{child}
				parent = child
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			def := covDefinition(baseNode())
			tc.mutate(&def)
			if err := ValidateDefinition(def); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
	if err := ValidateDefinition(covDefinition(baseNode())); err != nil {
		t.Fatalf("valid definition rejected: %v", err)
	}

	overLimit := make([]*ReportNode, 0, maxTaxonomyNodeCount+1)
	for i := 0; i <= maxTaxonomyNodeCount; i++ {
		overLimit = append(overLimit, covReportNode(fmt.Sprintf("N%d", i), NodeHeader))
	}
	if err := ValidateDefinition(covDefinition(overLimit...)); err == nil ||
		!strings.Contains(err.Error(), "tree exceeds") {
		t.Fatalf("node count error = %v", err)
	}
}

func TestCoverageTaxonomyEvaluatorUtilities(t *testing.T) {
	accounts := []AccountBalance{{
		AccountID:    uuid.New(),
		Code:         "1000",
		Amount:       decimal.NewFromInt(7),
		PeriodAmount: decimal.NewFromInt(20),
		EndingAmount: decimal.NewFromInt(30),
	}}
	income := accountsForStatement(accounts, StatementIncome)
	if !income[0].Amount.Equal(decimal.NewFromInt(20)) {
		t.Fatalf("income amount = %s", income[0].Amount)
	}
	balance := accountsForStatement(accounts, StatementBalanceSheet)
	if !balance[0].Amount.Equal(decimal.NewFromInt(30)) || accounts[0].Amount.Equal(decimal.NewFromInt(30)) {
		t.Fatalf("balance-sheet amount = %s, source = %s", balance[0].Amount, accounts[0].Amount)
	}

	id := uuid.New()
	ids := uniqueIDs([]uuid.UUID{uuid.Nil, id, id, uuid.Nil})
	if len(ids) != 1 || ids[0] != id {
		t.Fatalf("uniqueIDs = %v", ids)
	}
	ordered := sortedNodes([]*ReportNode{
		{Code: "B", DisplayOrder: 2},
		{Code: "Z", DisplayOrder: 1},
		{Code: "A", DisplayOrder: 1},
	})
	if ordered[0].Code != "A" || ordered[1].Code != "Z" || ordered[2].Code != "B" {
		t.Fatalf("sortedNodes = %q, %q, %q", ordered[0].Code, ordered[1].Code, ordered[2].Code)
	}
	if !validFramework(FrameworkSOCPA) || !validFramework(FrameworkUSGAAP) || validFramework("X") {
		t.Fatal("validFramework did not report all branches")
	}
	if !validStatementType(StatementCashFlow) || !validStatementType(StatementEquityMovement) || validStatementType("X") {
		t.Fatal("validStatementType did not report all branches")
	}
	if _, err := BuiltInTemplate("DOES_NOT_EXIST"); err == nil {
		t.Fatal("expected unknown built-in template error")
	}
	if len(BuiltInTemplates()) == 0 {
		t.Fatal("expected built-in templates")
	}
}
