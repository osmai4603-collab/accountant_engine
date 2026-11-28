package ifrs15

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/journal"
	publictypes "github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/shopspring/decimal"
)

func p2IFRS15Contract(t *testing.T) Contract {
	t.Helper()
	price, err := publictypes.NewMoney(decimal.NewFromInt(100), "USD")
	if err != nil {
		t.Fatalf("price fixture: %v", err)
	}
	standalone, err := publictypes.NewMoney(decimal.NewFromInt(100), "USD")
	if err != nil {
		t.Fatalf("standalone fixture: %v", err)
	}
	contract, err := NewContract(uuid.New(), uuid.New(), price, []PerformanceObligation{{
		ID: uuid.New(), Description: "delivery", StandalonePrice: standalone,
		RecognitionPattern: PointInTime, StartDate: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
	}})
	if err != nil {
		t.Fatalf("contract fixture: %v", err)
	}
	return contract
}

func p2IFRS15Unit(t *testing.T) *Unit {
	t.Helper()
	if _, err := NewUnit(UnitConfig{}); !errors.Is(err, ErrInvalidContract) {
		t.Fatalf("invalid unit config = %v", err)
	}
	u, err := NewUnit(UnitConfig{
		JournalID: uuid.New(), CashAccountID: uuid.New(), ReceivableAccountID: uuid.New(),
		ContractAssetAccountID: uuid.New(), ContractLiabilityAccount: uuid.New(), RevenueAccountID: uuid.New(),
	})
	if err != nil {
		t.Fatalf("unit fixture: %v", err)
	}
	return u
}

func TestP2IFRS15UnitRecognitionAndCollection(t *testing.T) {
	ctx := context.Background()
	date := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	amount, err := publictypes.NewMoney(decimal.NewFromInt(10), "USD")
	if err != nil {
		t.Fatalf("amount fixture: %v", err)
	}
	contract := p2IFRS15Contract(t)
	u := p2IFRS15Unit(t)
	var nilUnit *Unit
	if err := nilUnit.AddContract(contract); !errors.Is(err, ErrInvalidContract) {
		t.Fatalf("nil unit add contract = %v", err)
	}
	if err := u.AddContract(Contract{}); err == nil {
		t.Fatal("expected invalid contract add error")
	}
	alreadyRecognized := contract
	alreadyRecognized.RecognizedAmount = contract.TransactionPrice
	if err := u.AddContract(alreadyRecognized); !errors.Is(err, ErrInvalidContract) {
		t.Fatalf("already recognized contract = %v", err)
	}
	if err := u.AddContract(contract); err != nil {
		t.Fatalf("add contract: %v", err)
	}
	if err := u.AddContract(contract); !errors.Is(err, ErrInvalidContract) {
		t.Fatalf("duplicate contract = %v", err)
	}

	if _, _, err := nilUnit.ReceiveAdvance(contract.ID, amount, date); !errors.Is(err, ErrInvalidContract) {
		t.Fatalf("nil unit advance = %v", err)
	}
	if _, _, err := u.ReceiveAdvance(contract.ID, amount, time.Time{}); !errors.Is(err, ErrInvalidContract) {
		t.Fatalf("zero date advance = %v", err)
	}
	if _, _, err := u.ReceiveAdvance(uuid.New(), amount, date); !errors.Is(err, ErrContractNotFound) {
		t.Fatalf("missing contract advance = %v", err)
	}

	if _, ok := nilUnit.Contract(contract.ID); ok {
		t.Fatal("nil unit returned a contract")
	}
	if _, ok := u.Contract(uuid.New()); ok {
		t.Fatal("unexpected contract lookup hit")
	}
	if got, ok := u.Contract(contract.ID); !ok || got.ID != contract.ID {
		t.Fatalf("contract lookup = %+v %v", got, ok)
	}

	if _, err := nilUnit.CollectReceivable(contract.ID, amount, date); !errors.Is(err, ErrInvalidContract) {
		t.Fatalf("nil unit collection = %v", err)
	}
	if _, err := u.CollectReceivable(contract.ID, amount, time.Time{}); !errors.Is(err, ErrInvalidContract) {
		t.Fatalf("zero date collection = %v", err)
	}
	if _, err := u.CollectReceivable(uuid.New(), amount, date); !errors.Is(err, ErrContractNotFound) {
		t.Fatalf("missing contract collection = %v", err)
	}
	euro, err := publictypes.NewMoney(decimal.NewFromInt(10), "EUR")
	if err != nil {
		t.Fatalf("EUR fixture: %v", err)
	}
	if _, err := u.CollectReceivable(contract.ID, euro, date); !errors.Is(err, ErrInvalidAmount) {
		t.Fatalf("currency mismatch collection = %v", err)
	}
	if entry, err := u.CollectReceivable(contract.ID, amount, date); err != nil || len(entry.Lines) != 2 {
		t.Fatalf("collection entry = %+v %v", entry, err)
	}

	if _, _, err := nilUnit.Recognize(contract.ID, amount, date); !errors.Is(err, ErrInvalidContract) {
		t.Fatalf("nil unit recognition = %v", err)
	}
	if _, _, err := u.Recognize(uuid.New(), amount, date); !errors.Is(err, ErrContractNotFound) {
		t.Fatalf("missing contract recognition = %v", err)
	}
	updated, entry, err := u.Recognize(contract.ID, amount, date)
	if err != nil || len(entry.Lines) == 0 || !updated.RecognizedAmount.Amount().Equal(decimal.NewFromInt(10)) {
		t.Fatalf("recognition = %+v %+v %v", updated, entry, err)
	}
	if recognized := u.recognizedByPO[contract.ID][contract.Obligations[0].ID]; !recognized.Amount().Equal(decimal.NewFromInt(10)) {
		t.Fatalf("obligation recognition = %+v", recognized)
	}
	_ = ctx
}

func TestP2IFRS15StoreDeletePaths(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryContractStore()
	contract := p2IFRS15Contract(t)
	companyID := uuid.New()

	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := store.DeleteContract(canceled, companyID, contract.ID); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled delete = %v", err)
	}
	if err := store.DeleteContract(ctx, uuid.Nil, contract.ID); !errors.Is(err, ErrStoreCompanyNil) {
		t.Fatalf("nil company delete = %v", err)
	}
	if err := store.DeleteContract(ctx, companyID, contract.ID); !errors.Is(err, ErrContractNotFound) {
		t.Fatalf("missing company delete = %v", err)
	}
	if err := store.SaveContract(ctx, companyID, contract); err != nil {
		t.Fatalf("save contract: %v", err)
	}
	if err := store.DeleteContract(ctx, companyID, uuid.New()); !errors.Is(err, ErrContractNotFound) {
		t.Fatalf("missing contract delete = %v", err)
	}
	if err := store.DeleteContract(ctx, companyID, contract.ID); err != nil {
		t.Fatalf("delete contract: %v", err)
	}
	if _, err := store.GetContract(ctx, companyID, contract.ID); !errors.Is(err, ErrContractNotFound) {
		t.Fatalf("deleted contract lookup = %v", err)
	}
}

func TestP2IFRS15CompoundFactorAndEnterpriseBranches(t *testing.T) {
	if got := decimalCompoundFactor(decimal.RequireFromString("0.05"), 400); !got.GreaterThan(decimal.NewFromInt(1)) {
		t.Fatalf("compound factor = %s", got)
	}
	if got := decimalCompoundFactor(decimal.Zero, 365); !got.Equal(decimal.NewFromInt(1)) {
		t.Fatalf("zero-rate factor = %s", got)
	}

	if err := (ControlTransferInput{}).Validate(); err == nil {
		t.Fatal("expected missing identifiers error")
	}
	if err := (ControlTransferInput{
		ContractID: uuid.New(), ObligationID: uuid.New(), SatisfactionDate: time.Now().UTC(),
	}).Validate(); err == nil {
		t.Fatal("expected missing evidence error")
	}
	if err := (ControlTransferInput{
		ContractID: uuid.New(), ObligationID: uuid.New(), SatisfactionDate: time.Now().UTC(),
		HostEvidenceRef: "ref", MeasurementMethod: "output", ProgressPercentage: decimal.NewFromInt(101),
	}).Validate(); err == nil {
		t.Fatal("expected out-of-range progress error")
	}

	for _, in := range []PrincipalAgentInput{
		{},
		{ContractID: uuid.New(), Currency: "USD", AssessmentRef: "ref", GrossConsideration: decimal.NewFromInt(-1)},
	} {
		if _, err := EvaluatePrincipalVsAgent(in); err == nil {
			t.Fatalf("principal/agent validation error for %+v", in)
		}
	}
	if result, err := EvaluatePrincipalVsAgent(PrincipalAgentInput{
		ContractID: uuid.New(), Currency: "USD", AssessmentRef: "ref",
		ControlsBeforeTrans: true, GrossConsideration: decimal.NewFromInt(100), SupplierCost: decimal.NewFromInt(60),
	}); err != nil || result.Role != RolePrincipal || !result.RecognizedRevenue.Equal(decimal.NewFromInt(100)) {
		t.Fatalf("principal result = %+v %v", result, err)
	}

	for _, in := range []IPLicenseInput{
		{},
		{ContractID: uuid.New(), Currency: "USD", FixedFee: decimal.NewFromInt(-1)},
		{ContractID: uuid.New(), Currency: "USD", LicenseType: "unsupported"},
	} {
		if _, err := EvaluateIPLicense(in); err == nil {
			t.Fatalf("IP license validation error for %+v", in)
		}
	}
	if result, err := EvaluateIPLicense(IPLicenseInput{
		ContractID: uuid.New(), Currency: "USD", LicenseType: IPLicenseRightToUse,
		FixedFee: decimal.NewFromInt(100),
	}); err != nil || result.RecognitionPattern != "point_in_time" {
		t.Fatalf("right-to-use result = %+v %v", result, err)
	}
}

func TestP2IFRS15WorkflowContractLookup(t *testing.T) {
	contract := p2IFRS15Contract(t)
	companyID := uuid.New()
	store := NewInMemoryContractStore()
	if err := store.SaveContract(context.Background(), companyID, contract); err != nil {
		t.Fatalf("save contract: %v", err)
	}
	var nilWorkflow *RevenueWorkflow
	if _, ok := nilWorkflow.Contract(contract.ID); ok {
		t.Fatal("nil workflow returned a contract")
	}
	workflow := &RevenueWorkflow{
		mapping:   RevenueJournalMapping{CompanyID: companyID},
		contracts: make(map[uuid.UUID]Contract),
		store:     store,
	}
	if got, ok := workflow.Contract(contract.ID); !ok || got.ID != contract.ID {
		t.Fatalf("stored contract lookup = %+v %v", got, ok)
	}
	if got, ok := workflow.Contract(contract.ID); !ok || got.ID != contract.ID {
		t.Fatalf("cached contract lookup = %+v %v", got, ok)
	}
	if _, ok := workflow.Contract(uuid.New()); ok {
		t.Fatal("unexpected missing contract lookup hit")
	}
	workflow.contracts[contract.ID] = contract
	if got, ok := workflow.Contract(contract.ID); !ok || got.ID != contract.ID {
		t.Fatalf("in-memory contract lookup = %+v %v", got, ok)
	}
}

func TestP2IFRS15ValidationAndReassessmentBranches(t *testing.T) {
	contract := p2IFRS15Contract(t)
	zero, err := publictypes.Zero("USD")
	if err != nil {
		t.Fatalf("zero money fixture: %v", err)
	}
	if err := (Contract{}).Validate(); !errors.Is(err, ErrInvalidContract) {
		t.Fatalf("empty contract validation = %v", err)
	}
	wrongCurrency := contract
	invalidZero, err := publictypes.Zero("EUR")
	if err != nil {
		t.Fatalf("EUR zero fixture: %v", err)
	}
	wrongCurrency.BilledAmount = invalidZero
	if err := wrongCurrency.Validate(); !errors.Is(err, ErrInvalidContract) {
		t.Fatalf("wrong billed currency = %v", err)
	}
	badObligation := contract
	badObligation.Obligations = append([]PerformanceObligation(nil), contract.Obligations...)
	badObligation.Obligations[0].Description = ""
	if err := badObligation.Validate(); !errors.Is(err, ErrInvalidObligation) {
		t.Fatalf("empty obligation description = %v", err)
	}
	badPattern := contract
	badPattern.Obligations = append([]PerformanceObligation(nil), contract.Obligations...)
	badPattern.Obligations[0].RecognitionPattern = "unknown"
	if err := badPattern.Validate(); !errors.Is(err, ErrInvalidObligation) {
		t.Fatalf("invalid recognition pattern = %v", err)
	}
	wrongAllocation, err := publictypes.NewMoney(decimal.NewFromInt(50), "USD")
	if err != nil {
		t.Fatalf("wrong allocation fixture: %v", err)
	}
	wrongTotal := contract
	wrongTotal.AllocatedPrices = map[uuid.UUID]publictypes.Money{
		contract.Obligations[0].ID: wrongAllocation,
	}
	if err := wrongTotal.Validate(); !errors.Is(err, ErrInvalidContract) {
		t.Fatalf("allocation total mismatch = %v", err)
	}
	badStatus := contract
	badStatus.Status = "unknown"
	if err := badStatus.Validate(); !errors.Is(err, ErrInvalidContract) {
		t.Fatalf("invalid status = %v", err)
	}

	price, err := publictypes.NewMoney(decimal.NewFromInt(100), "USD")
	if err != nil {
		t.Fatalf("price fixture: %v", err)
	}
	if _, err := NewContract(uuid.Nil, uuid.New(), price, nil); !errors.Is(err, ErrInvalidContract) {
		t.Fatalf("nil contract id = %v", err)
	}
	if _, err := NewContract(uuid.New(), uuid.New(), price, nil); !errors.Is(err, ErrInvalidObligation) {
		t.Fatalf("missing obligations = %v", err)
	}
	invalidStandalone, err := publictypes.NewMoney(decimal.NewFromInt(100), "EUR")
	if err != nil {
		t.Fatalf("EUR fixture: %v", err)
	}
	if _, err := NewContract(uuid.New(), uuid.New(), price, []PerformanceObligation{{
		ID: uuid.New(), Description: "delivery", StandalonePrice: invalidStandalone,
		RecognitionPattern: PointInTime, StartDate: time.Now().UTC(),
	}}); !errors.Is(err, ErrInvalidObligation) {
		t.Fatalf("invalid standalone currency = %v", err)
	}

	fixed, err := publictypes.NewMoney(decimal.NewFromInt(60), "USD")
	if err != nil {
		t.Fatalf("fixed fixture: %v", err)
	}
	variable, err := publictypes.NewMoney(decimal.NewFromInt(50), "USD")
	if err != nil {
		t.Fatalf("variable fixture: %v", err)
	}
	if _, err := ReassessVariableConsideration(Contract{}, fixed, VariableConsiderationEstimate{}); err == nil {
		t.Fatal("expected invalid contract reassessment error")
	}
	invalidFixed, err := publictypes.NewMoney(decimal.NewFromInt(60), "EUR")
	if err != nil {
		t.Fatalf("EUR fixed fixture: %v", err)
	}
	if _, err := ReassessVariableConsideration(contract, invalidFixed, VariableConsiderationEstimate{}); err == nil {
		t.Fatal("expected currency mismatch reassessment error")
	}
	if _, err := ReassessVariableConsideration(contract, zero, VariableConsiderationEstimate{FinalAmount: zero}); err == nil {
		t.Fatal("expected zero total reassessment error")
	}
	reassessed, err := ReassessVariableConsideration(contract, fixed, VariableConsiderationEstimate{FinalAmount: variable})
	if err != nil || !reassessed.TransactionPrice.Amount().Equal(decimal.NewFromInt(110)) {
		t.Fatalf("reassessment = %+v %v", reassessed, err)
	}
	recognized := contract
	recognized.RecognizedAmount = price
	if _, err := ReassessVariableConsideration(recognized, zero, VariableConsiderationEstimate{FinalAmount: variable}); !errors.Is(err, ErrRecognitionExceeded) {
		t.Fatalf("recognized above updated price = %v", err)
	}
}

func TestP2IFRS15JournalEntryHelpers(t *testing.T) {
	date := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	if _, err := newJournalEntry(uuid.Nil, date, "draft", nil); err == nil {
		t.Fatal("expected missing journal id error")
	}
	if _, err := newJournalEntry(uuid.New(), time.Time{}, "draft", nil); err == nil {
		t.Fatal("expected missing entry date error")
	}
	if _, err := newJournalEntry(uuid.New(), date, "draft", nil); err == nil {
		t.Fatal("expected minimum lines error")
	}
	unbalanced := make([]journal.EntryLine, 2)
	if _, err := newJournalEntry(uuid.New(), date, "draft", unbalanced); err == nil {
		t.Fatal("expected unbalanced lines error")
	}
	if got := zeroMoney(""); got.Currency() != "" {
		t.Fatalf("zero money with invalid currency = %+v", got)
	}
}

func TestP2IFRS15StoreLookupBranches(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryContractStore()
	contract := p2IFRS15Contract(t)
	companyID := uuid.New()

	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := store.SaveContract(canceled, companyID, contract); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled save = %v", err)
	}
	if err := store.SaveContract(ctx, uuid.Nil, contract); !errors.Is(err, ErrStoreCompanyNil) {
		t.Fatalf("nil company save = %v", err)
	}
	if err := store.SaveContract(ctx, companyID, Contract{}); !errors.Is(err, ErrInvalidContract) {
		t.Fatalf("invalid contract save = %v", err)
	}
	if _, err := store.GetContract(canceled, companyID, contract.ID); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled get = %v", err)
	}
	if _, err := store.GetContract(ctx, companyID, contract.ID); !errors.Is(err, ErrContractNotFound) {
		t.Fatalf("missing company get = %v", err)
	}
	if _, err := store.GetContract(ctx, uuid.Nil, contract.ID); !errors.Is(err, ErrStoreCompanyNil) {
		t.Fatalf("nil company get = %v", err)
	}
	if _, err := store.ListContracts(canceled, companyID); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled list = %v", err)
	}
	if _, err := store.ListContracts(ctx, uuid.Nil); !errors.Is(err, ErrStoreCompanyNil) {
		t.Fatalf("nil company list = %v", err)
	}
	if contracts, err := store.ListContracts(ctx, uuid.New()); err != nil || len(contracts) != 0 {
		t.Fatalf("unknown company list = %+v %v", contracts, err)
	}
}

func TestP2IFRS15ModuleHelpers(t *testing.T) {
	mod := NewModule()
	if err := mod.RunScheduledJobs(context.Background(), nil); err != nil {
		t.Fatalf("scheduled jobs = %v", err)
	}
	if err := mod.Close(context.Background()); err != nil {
		t.Fatalf("close = %v", err)
	}
	store := NewInMemoryContractStore()
	mod.SetContractStore(store)
	if mod.ContractStore() != store {
		t.Fatal("contract store was not installed")
	}
}
