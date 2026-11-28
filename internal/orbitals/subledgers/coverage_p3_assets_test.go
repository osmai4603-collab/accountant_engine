package subledgers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	financialtypes "github.com/osmai4603-collab/accounting_engine/internal/kernel/types"
	posting "github.com/osmai4603-collab/accounting_engine/internal/orbitals/postingcompat"
	apisubledger "github.com/osmai4603-collab/accounting_engine/pkg/api/v1/subledger"
	apitypes "github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/shopspring/decimal"
)

type apisubledgerFixedAssetType = apisubledger.FixedAsset

func publicCompanyID(id uuid.UUID) apitypes.CompanyID { return apitypes.CompanyID(id) }

func covPublicMoney(t *testing.T, value, currency string) apitypes.Money {
	t.Helper()
	amount, err := apitypes.NewMoneyFromString(value, currency)
	if err != nil {
		t.Fatal(err)
	}
	return amount
}

func apisubledgerFixedAsset(t *testing.T, companyID, assetID uuid.UUID) apisubledger.FixedAsset {
	t.Helper()
	return apisubledger.FixedAsset{
		ID:                 assetID.String(),
		CompanyID:          publicCompanyID(companyID),
		AcquisitionDate:    covDate(1),
		AcquisitionCost:    covPublicMoney(t, "1000", "USD"),
		UsefulLifeMonths:   24,
		SalvageValue:       covPublicMoney(t, "0", "USD"),
		DepreciationMethod: string(StraightLine),
		NetBookValue:       covPublicMoney(t, "1000", "USD"),
		Status:             "active",
	}
}

type covAssetState struct {
	*MemoryRepository
	updateErr error
}

func (s *covAssetState) UpdateDepreciatedAsset(ctx context.Context, expected, updated FixedAsset) error {
	if s.updateErr != nil {
		return s.updateErr
	}
	return s.SaveFixedAsset(ctx, updated)
}

type covOperationAssetState struct {
	*MemoryRepository
	applyErr error
}

func (s *covOperationAssetState) ApplyFixedAssetOperation(
	ctx context.Context, expected, updated FixedAsset, entry posting.JournalEntry,
) error {
	if s.applyErr != nil {
		return s.applyErr
	}
	return s.ConfirmFixedAssetMutation(ctx, expected, updated, entry)
}

type covFailingAssetRepo struct {
	*MemoryRepository
	saveErr error
}

func (r *covFailingAssetRepo) SaveFixedAsset(ctx context.Context, asset FixedAsset) error {
	if r.saveErr != nil {
		return r.saveErr
	}
	return r.MemoryRepository.SaveFixedAsset(ctx, asset)
}

func covAssetFixture(t *testing.T, companyID, assetID uuid.UUID) FixedAsset {
	t.Helper()
	return FixedAsset{
		CompanyID:                      companyID,
		ID:                             assetID,
		HistoricalCost:                 covMoney(t, "1000", "USD"),
		SalvageValue:                   covMoney(t, "0", "USD"),
		UsefulLifeMonths:               24,
		RemainingUsefulLifeMonths:      24,
		Method:                         StraightLine,
		InServiceDate:                  covDate(1),
		AccumulatedDepreciation:        covMoney(t, "0", "USD"),
		AccumulatedImpairment:          covMoney(t, "0", "USD"),
		CurrentBookValue:               covMoney(t, "1000", "USD"),
		ExpenseAccountID:               uuid.New(),
		AccumulatedAccountID:           uuid.New(),
		ImpairmentAccountID:            uuid.New(),
		AccumulatedImpairmentAccountID: uuid.New(),
		AssetAccountID:                 uuid.New(),
		DisposalCashAccountID:          uuid.New(),
		DisposalGainAccountID:          uuid.New(),
		DisposalLossAccountID:          uuid.New(),
		RevaluationAdjustment:          covMoney(t, "0", "USD"),
		RevaluationSurplus:             covMoney(t, "0", "USD"),
		RevaluationSurplusAccountID:    uuid.New(),
		RevaluationLossAccountID:       uuid.New(),
	}
}

func covSaveAsset(t *testing.T, repo *MemoryRepository, asset FixedAsset) {
	t.Helper()
	if err := repo.SaveFixedAsset(context.Background(), asset); err != nil {
		t.Fatal(err)
	}
}

func TestCovRegisterAssetContextValidation(t *testing.T) {
	companyID := uuid.New()
	ctx := context.Background()
	repo := NewMemoryRepository()
	ledger := NewFixedAssetLedgerForCompany(repo, nil, companyID)
	asset := covAssetFixture(t, companyID, uuid.New())

	if err := ledger.RegisterAssetContext(ctx, asset); err != nil {
		t.Fatalf("valid asset: %v", err)
	}
	withoutID := asset
	withoutID.ID = uuid.Nil
	if err := ledger.RegisterAssetContext(ctx, withoutID); err != nil {
		t.Fatalf("asset without id: %v", err)
	}
	assets, err := repo.ListFixedAssets(ctx)
	if err != nil || len(assets) != 2 {
		t.Fatalf("saved assets = %d err=%v", len(assets), err)
	}
	for _, saved := range assets {
		if saved.ID == uuid.Nil {
			t.Fatal("generated asset id was not persisted")
		}
	}

	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	invalid := []struct {
		name  string
		ctx   context.Context
		asset FixedAsset
		want  error
	}{
		{"cancelled", cancelled, asset, context.Canceled},
		{"nil company", ctx, func() FixedAsset { a := asset; a.CompanyID = uuid.Nil; return a }(), ErrInvalidAsset},
		{"company mismatch", ctx, func() FixedAsset { a := asset; a.CompanyID = uuid.New(); return a }(), ErrCompanyMismatch},
		{"missing cost currency", ctx, func() FixedAsset { a := asset; a.HistoricalCost = financialtypes.Money{}; return a }(), ErrInvalidAsset},
		{"salvage above cost", ctx, func() FixedAsset { a := asset; a.SalvageValue = covMoney(t, "2000", "USD"); return a }(), ErrInvalidAsset},
		{"life zero", ctx, func() FixedAsset { a := asset; a.UsefulLifeMonths = 0; return a }(), ErrInvalidAsset},
		{"remaining above life", ctx, func() FixedAsset { a := asset; a.RemainingUsefulLifeMonths = 25; return a }(), ErrInvalidAsset},
		{"invalid method", ctx, func() FixedAsset { a := asset; a.Method = DepreciationMethod("other"); return a }(), ErrInvalidAsset},
		{"declining rate too high", ctx, func() FixedAsset {
			a := asset
			a.Method = DecliningBalance
			a.DecliningRate = decimal.NewFromInt(2)
			return a
		}(), ErrInvalidAsset},
		{"cwip missing account", ctx, func() FixedAsset { a := asset; a.IsCWIP = true; return a }(), ErrInvalidAsset},
		{"negative surplus", ctx, func() FixedAsset { a := asset; a.RevaluationSurplus = covMoney(t, "-1", "USD"); return a }(), ErrInvalidAsset},
		{"accumulated currency mismatch", ctx, func() FixedAsset {
			a := asset
			a.AccumulatedDepreciation = covMoney(t, "0", "EUR")
			return a
		}(), ErrInvalidAsset},
		{"book value mismatch", ctx, func() FixedAsset { a := asset; a.CurrentBookValue = covMoney(t, "999", "USD"); return a }(), ErrInvalidAsset},
		{"pending operation mismatch", ctx, func() FixedAsset { a := asset; a.PendingOperation = "DISPOSAL"; return a }(), ErrInvalidAsset},
	}
	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			if err := ledger.RegisterAssetContext(tc.ctx, tc.asset); !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
		})
	}

	failing := &covFailingAssetRepo{MemoryRepository: NewMemoryRepository(), saveErr: errors.New("disk full")}
	failingLedger := NewFixedAssetLedgerForCompany(failing, nil, companyID)
	if err := failingLedger.RegisterAssetContext(ctx, asset); !errors.Is(err, failing.saveErr) {
		t.Fatalf("save error = %v", err)
	}
}

func TestCovCalculateDepreciationAmountBranches(t *testing.T) {
	base := covAssetFixture(t, uuid.New(), uuid.New())
	beforeService := base.InServiceDate.AddDate(0, -1, 0)
	if amount, err := calculateDepreciationAmount(base, beforeService); err != nil || !amount.Amount().IsZero() {
		t.Fatalf("before service amount = %v err=%v", amount, err)
	}

	scheduled, err := calculateDepreciationAmount(base, covDate(15))
	if err != nil || !scheduled.Amount().IsPositive() {
		t.Fatalf("scheduled amount = %v err=%v", scheduled, err)
	}

	withRevaluation := base
	withRevaluation.RevaluationAdjustment = covMoney(t, "100", "USD")
	withRevaluation.CurrentBookValue = covMoney(t, "1100", "USD")
	straightLine, err := calculateDepreciationAmount(withRevaluation, covDate(15))
	if err != nil || !straightLine.Amount().IsPositive() {
		t.Fatalf("straight-line amount = %v err=%v", straightLine, err)
	}

	revaluedInMonth := withRevaluation
	revaluedInMonth.LastRevaluationDate = covDate(10)
	if amount, err := calculateDepreciationAmount(revaluedInMonth, covDate(15)); err != nil || !amount.Amount().IsPositive() {
		t.Fatalf("revalued in month amount = %v err=%v", amount, err)
	}

	afterLife := withRevaluation
	afterLife.InServiceDate = covDate(1).AddDate(0, -1, 0)
	afterLife.UsefulLifeMonths = 1
	if amount, err := calculateDepreciationAmount(afterLife, covDate(15).AddDate(0, 1, 0)); err != nil || !amount.Amount().IsZero() {
		t.Fatalf("after life amount = %v err=%v", amount, err)
	}

	zeroDepreciable := withRevaluation
	zeroDepreciable.SalvageValue = covMoney(t, "1100", "USD")
	zeroDepreciable.CurrentBookValue = covMoney(t, "1100", "USD")
	if amount, err := calculateDepreciationAmount(zeroDepreciable, covDate(15)); err != nil || !amount.Amount().IsZero() {
		t.Fatalf("zero depreciable amount = %v err=%v", amount, err)
	}

	declining := withRevaluation
	declining.Method = DecliningBalance
	declining.DecliningRate = decimal.Zero
	if amount, err := calculateDepreciationAmount(declining, covDate(15)); err != nil || !amount.Amount().IsPositive() {
		t.Fatalf("declining balance amount = %v err=%v", amount, err)
	}

	capped := declining
	capped.UsefulLifeMonths = 1
	if amount, err := calculateDepreciationAmount(capped, covDate(15)); err != nil || !amount.Equal(covMoney(t, "1100", "USD")) {
		t.Fatalf("capped declining amount = %v err=%v", amount, err)
	}

	unknownMethod := withRevaluation
	unknownMethod.Method = DepreciationMethod("other")
	if _, err := calculateDepreciationAmount(unknownMethod, covDate(15)); !errors.Is(err, ErrInvalidAsset) {
		t.Fatalf("unknown method error = %v", err)
	}
}

func TestCovRevalueAssetAtomicAndAttachment(t *testing.T) {
	companyID, assetID, journalID := uuid.New(), uuid.New(), uuid.New()
	ctx := context.Background()
	asset := covAssetFixture(t, companyID, assetID)
	repo := &covAssetState{MemoryRepository: NewMemoryRepository()}
	covSaveAsset(t, repo.MemoryRepository, asset)
	ledger := NewFixedAssetLedgerForCompany(repo, nil, companyID)
	engine := posting.NewEngineWithStore(nil, repo)
	fairValue := covMoney(t, "1200", "USD")
	asOf := covDate(5)

	entry, err := ledger.RevalueAssetAtomic(ctx, engine, assetID, journalID, fairValue, asOf, "poster")
	if !errors.Is(err, ErrInvalidAsset) || entry.Status != posting.EntryDraft {
		t.Fatalf("atomic revaluation: %+v err=%v", entry, err)
	}
	unchanged, err := repo.GetFixedAsset(ctx, companyID, assetID)
	if err != nil || !unchanged.CurrentBookValue.Equal(covMoney(t, "1000", "USD")) {
		t.Fatalf("asset should remain unchanged after failed post: %+v err=%v", unchanged, err)
	}

	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := ledger.RevalueAssetAtomic(cancelled, engine, assetID, journalID, fairValue, asOf, "poster"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled error = %v", err)
	}
	if _, err := ledger.RevalueAssetAtomic(ctx, engine, uuid.Nil, journalID, fairValue, asOf, "poster"); !errors.Is(err, ErrInvalidAsset) {
		t.Fatalf("nil asset error = %v", err)
	}
	if _, err := ledger.RevalueAssetAtomic(ctx, engine, uuid.New(), journalID, fairValue, asOf, "poster"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown asset error = %v", err)
	}
	if _, err := ledger.RevalueAssetAtomic(ctx, engine, assetID, journalID, covMoney(t, "1000", "USD"), asOf, "poster"); !errors.Is(err, ErrInvalidAsset) {
		t.Fatalf("zero movement error = %v", err)
	}
	if _, err := ledger.RevalueAssetAtomic(ctx, nil, assetID, journalID, fairValue, asOf, "poster"); err == nil {
		t.Fatal("expected nil engine failure")
	}
	if _, err := ledger.RevalueAssetContext(cancelled, assetID, journalID, fairValue, asOf); !errors.Is(err, context.Canceled) {
		t.Fatalf("context revaluation cancelled error = %v", err)
	}
	if _, err := ledger.RevalueAssetContext(ctx, assetID, uuid.Nil, fairValue, asOf); !errors.Is(err, ErrInvalidAsset) {
		t.Fatalf("context revaluation nil journal error = %v", err)
	}
	if _, err := ledger.RevalueAssetContext(ctx, uuid.New(), journalID, fairValue, asOf); !errors.Is(err, ErrNotFound) {
		t.Fatalf("context revaluation unknown asset error = %v", err)
	}
	if err := ledger.ConfirmRevaluationContext(cancelled, assetID, entry.ID, fairValue, asOf); !errors.Is(err, context.Canceled) {
		t.Fatalf("confirm cancelled error = %v", err)
	}
	if err := ledger.ConfirmRevaluationContext(ctx, uuid.New(), entry.ID, fairValue, asOf); !errors.Is(err, ErrNotFound) {
		t.Fatalf("confirm unknown asset error = %v", err)
	}

	_, expectedUpdated, lines, err := revaluationUpdate(asset, fairValue, asOf)
	if err != nil {
		t.Fatal(err)
	}
	expectedUpdated.AppliedEntryIDs = append(append([]uuid.UUID(nil), asset.AppliedEntryIDs...), entry.ID)
	attachment := &RevaluationAttachment{
		AssetID: assetID, CompanyID: companyID, FairValue: fairValue, AsOf: asOf,
		EntryID: entry.ID, expectedAsset: asset, UpdatedAsset: expectedUpdated,
	}
	draft := posting.JournalEntry{
		ID: entry.ID, CompanyID: companyID, Status: posting.EntryDraft,
		EntryDate: asOf, Date: asOf, DocumentReference: revaluationReference(assetID, asOf), Lines: lines,
	}
	if attachment.AttachmentKind() != "fixed_asset_revaluation" {
		t.Fatalf("attachment kind = %s", attachment.AttachmentKind())
	}
	if err := attachment.ValidateAttachment(ctx, draft); err != nil {
		t.Fatalf("valid attachment: %v", err)
	}
	invalidDraft := draft
	invalidDraft.Status = posting.EntryPosted
	if err := attachment.ValidateAttachment(ctx, invalidDraft); !errors.Is(err, ErrInvalidAsset) {
		t.Fatalf("posted draft error = %v", err)
	}
	invalidDraft = draft
	invalidDraft.Lines = append([]posting.JournalEntryLine(nil), draft.Lines...)
	invalidDraft.Lines[0].Debit = covMoney(t, "1", "USD")
	if err := attachment.ValidateAttachment(ctx, invalidDraft); !errors.Is(err, ErrInvalidAsset) {
		t.Fatalf("line mismatch error = %v", err)
	}
	if err := attachment.ValidateAttachment(cancelled, draft); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled attachment error = %v", err)
	}

	if !fixedAssetDraftLinesMatch(draft.Lines, lines, companyID) {
		t.Fatal("expected matching draft lines")
	}
	if fixedAssetDraftLinesMatch(draft.Lines[:1], lines, companyID) {
		t.Fatal("expected length mismatch")
	}
	wrongCompany := append([]posting.JournalEntryLine(nil), draft.Lines...)
	wrongCompany[0].CompanyID = uuid.New()
	if fixedAssetDraftLinesMatch(wrongCompany, lines, companyID) {
		t.Fatal("expected company mismatch")
	}

	posted := draft
	posted.Status = posting.EntryPosted
	if err := attachment.ExecuteAtomicMutation(ctx, &covAtomicTx{state: repo}, posted); err != nil {
		t.Fatalf("memory mutation: %v", err)
	}
	if err := attachment.ExecuteAtomicMutation(ctx, &covAtomicTx{state: struct{}{}}, posted); err != nil {
		t.Fatalf("SQL fallback: %v", err)
	}
	queryErr := errors.New("query failed")
	if err := attachment.ExecuteAtomicMutation(ctx, &covAtomicTx{state: struct{}{}, queryErr: queryErr}, posted); !errors.Is(err, ErrConcurrentAssetUpdate) || !errors.Is(err, queryErr) {
		t.Fatalf("SQL error = %v", err)
	}
	mutationErr := errors.New("mutation failed")
	if err := attachment.ExecuteAtomicMutation(ctx, &covAtomicTx{state: repo, mutationErr: mutationErr}, posted); !errors.Is(err, mutationErr) {
		t.Fatalf("mutation error = %v", err)
	}
	badEntry := posted
	badEntry.Status = posting.EntryDraft
	if err := attachment.ExecuteAtomicMutation(ctx, &covAtomicTx{state: repo}, badEntry); !errors.Is(err, ErrInvalidAsset) {
		t.Fatalf("bad entry error = %v", err)
	}
}

func TestCovRevaluationUpdateBranches(t *testing.T) {
	companyID, assetID := uuid.New(), uuid.New()
	asOf := covDate(5)
	base := covAssetFixture(t, companyID, assetID)
	disposedAt := covDate(4)
	cases := []struct {
		name  string
		asset FixedAsset
		value string
	}{
		{"disposed", func() FixedAsset { a := base; a.DisposedAt = &disposedAt; return a }(), "1200"},
		{"pending", func() FixedAsset { a := base; a.PendingOperation = "IMPAIRMENT"; return a }(), "1200"},
		{"nil id", func() FixedAsset { a := base; a.ID = uuid.Nil; return a }(), "1200"},
		{"zero date", base, "1200"},
		{"currency mismatch", func() FixedAsset { a := base; a.CurrentBookValue = covMoney(t, "1000", "EUR"); return a }(), "1200"},
		{"negative", base, "-1"},
		{"same revaluation date", func() FixedAsset { a := base; a.LastRevaluationDate = asOf; return a }(), "1200"},
		{"future revaluation", func() FixedAsset { a := base; a.LastRevaluationDate = covDate(9); return a }(), "1200"},
		{"future depreciation", func() FixedAsset { a := base; a.LastDepreciationDate = covDate(9); return a }(), "1200"},
		{"nil asset account", func() FixedAsset { a := base; a.AssetAccountID = uuid.Nil; return a }(), "1200"},
		{"zero movement", base, "1000"},
		{"increase without surplus account", func() FixedAsset { a := base; a.RevaluationSurplusAccountID = uuid.Nil; return a }(), "1200"},
		{"decrease without surplus account", func() FixedAsset {
			a := base
			a.RevaluationSurplus = covMoney(t, "50", "USD")
			a.RevaluationSurplusAccountID = uuid.Nil
			return a
		}(), "900"},
		{"decrease without loss account", func() FixedAsset {
			a := base
			a.RevaluationSurplus = covMoney(t, "0", "USD")
			a.RevaluationLossAccountID = uuid.Nil
			return a
		}(), "900"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			value := covMoney(t, tc.value, "USD")
			testAsOf := asOf
			if tc.name == "zero date" {
				testAsOf = time.Time{}
			}
			if _, _, _, err := revaluationUpdate(tc.asset, value, testAsOf); !errors.Is(err, ErrInvalidAsset) {
				t.Fatalf("error = %v, want invalid asset", err)
			}
		})
	}

	increase, updated, lines, err := revaluationUpdate(base, covMoney(t, "1200", "USD"), asOf)
	if err != nil || !increase.Equal(covMoney(t, "200", "USD")) || len(lines) != 2 ||
		!updated.RevaluationSurplus.Equal(covMoney(t, "200", "USD")) {
		t.Fatalf("increase = %v updated=%+v lines=%d err=%v", increase, updated, len(lines), err)
	}
	decreaseAsset := base
	decreaseAsset.RevaluationSurplus = covMoney(t, "50", "USD")
	decrease, updated, lines, err := revaluationUpdate(decreaseAsset, covMoney(t, "900", "USD"), asOf)
	if err != nil || !decrease.Equal(covMoney(t, "-100", "USD")) || len(lines) != 4 ||
		!updated.RevaluationSurplus.Amount().IsZero() {
		t.Fatalf("decrease = %v updated=%+v lines=%d err=%v", decrease, updated, len(lines), err)
	}
}

func TestCovDepreciateAssetAtomicAndAttachment(t *testing.T) {
	companyID, assetID, journalID := uuid.New(), uuid.New(), uuid.New()
	ctx := context.Background()
	asset := covAssetFixture(t, companyID, assetID)
	repo := &covAssetState{MemoryRepository: NewMemoryRepository()}
	covSaveAsset(t, repo.MemoryRepository, asset)
	ledger := NewFixedAssetLedgerForCompany(repo, nil, companyID)
	engine := posting.NewEngineWithStore(nil, repo)
	asOf := covDate(5)

	entry, err := ledger.DepreciateAssetAtomic(ctx, engine, assetID, journalID, asOf, "poster")
	if err != nil || entry.Status != posting.EntryPosted {
		t.Fatalf("atomic depreciation: %+v err=%v", entry, err)
	}

	if _, err := ledger.DepreciateAssetAtomic(ctx, engine, assetID, uuid.Nil, asOf, "poster"); !errors.Is(err, ErrInvalidAsset) {
		t.Fatalf("nil journal error = %v", err)
	}
	if _, err := ledger.DepreciateAssetAtomic(ctx, engine, uuid.New(), journalID, asOf, "poster"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown asset error = %v", err)
	}
	if _, err := ledger.DepreciateAssetAtomic(ctx, engine, assetID, journalID, asOf, "poster"); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("same month error = %v", err)
	}
	earlierMonth := covDate(1).AddDate(0, -1, 5)
	if _, err := ledger.DepreciateAssetAtomic(ctx, engine, assetID, journalID, earlierMonth, "poster"); !errors.Is(err, ErrInvalidAsset) {
		t.Fatalf("earlier as-of error = %v", err)
	}
	nextMonth := covDate(1).AddDate(0, 1, 5)
	if _, err := ledger.DepreciateAssetAtomic(ctx, nil, assetID, journalID, nextMonth, "poster"); err == nil {
		t.Fatal("expected nil engine failure")
	}
	sqlEngine := posting.NewEngineWithStore(nil, struct{}{})
	if _, err := ledger.DepreciateAssetAtomic(ctx, sqlEngine, assetID, journalID, nextMonth, "poster"); err == nil {
		t.Fatal("expected SQL fallback failure")
	}
	disposed := asset
	disposedAt := covDate(4)
	disposed.DisposedAt = &disposedAt
	covSaveAsset(t, repo.MemoryRepository, disposed)
	if _, err := ledger.DepreciateAssetAtomic(ctx, engine, assetID, journalID, covDate(6), "poster"); !errors.Is(err, ErrInvalidAsset) {
		t.Fatalf("disposed asset error = %v", err)
	}
	cwip := asset
	cwip.IsCWIP = true
	covSaveAsset(t, repo.MemoryRepository, cwip)
	if _, err := ledger.DepreciateAssetAtomic(ctx, engine, assetID, journalID, covDate(6), "poster"); !errors.Is(err, ErrInvalidAsset) {
		t.Fatalf("cwip asset error = %v", err)
	}

	invalidApprovals := []struct {
		name     string
		approval FixedAssetAdjustmentApproval
		poster   string
	}{
		{"unauthorized", FixedAssetAdjustmentApproval{}, "poster"},
		{"missing reason", FixedAssetAdjustmentApproval{Authorized: true, ApprovalReference: "APR", ApprovedBy: "CFO", ApprovedAt: covDate(1)}, "poster"},
		{"missing reference", FixedAssetAdjustmentApproval{Authorized: true, Reason: "why", ApprovedBy: "CFO", ApprovedAt: covDate(1)}, "poster"},
		{"missing approver", FixedAssetAdjustmentApproval{Authorized: true, Reason: "why", ApprovalReference: "APR", ApprovedAt: covDate(1)}, "poster"},
		{"missing approval date", FixedAssetAdjustmentApproval{Authorized: true, Reason: "why", ApprovalReference: "APR", ApprovedBy: "CFO"}, "poster"},
		{"missing poster", FixedAssetAdjustmentApproval{Authorized: true, Reason: "why", ApprovalReference: "APR", ApprovedBy: "CFO", ApprovedAt: covDate(1)}, ""},
	}
	for _, tc := range invalidApprovals {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ledger.DepreciateAssetAdjustingAtomic(ctx, engine, assetID, journalID, covDate(6), tc.poster, tc.approval); !errors.Is(err, posting.ErrAdjustingEntryMetadataRequired) {
				t.Fatalf("error = %v", err)
			}
		})
	}

	adjustingRepo := &covAssetState{MemoryRepository: NewMemoryRepository()}
	adjustingAsset := covAssetFixture(t, companyID, uuid.New())
	covSaveAsset(t, adjustingRepo.MemoryRepository, adjustingAsset)
	adjustingLedger := NewFixedAssetLedgerForCompany(adjustingRepo, nil, companyID)
	approval := FixedAssetAdjustmentApproval{
		Authorized: true, Reason: " revision ", ApprovalReference: " APR-1 ",
		ApprovedBy: " CFO ", ApprovedAt: covDate(4), TargetPeriodNumber: 1,
	}
	adjustingEntry, err := adjustingLedger.DepreciateAssetAdjustingAtomic(ctx, posting.NewEngineWithStore(nil, adjustingRepo),
		adjustingAsset.ID, uuid.New(), covDate(6), "poster", approval)
	if err != nil || adjustingEntry.Classification != posting.EntryClassAdjusting || adjustingEntry.AdjustmentReason != "revision" {
		t.Fatalf("adjusting entry: %+v err=%v", adjustingEntry, err)
	}

	expectedAmount, expectedUpdated, err := depreciationUpdate(asset, asOf)
	if err != nil {
		t.Fatal(err)
	}
	lines, err := depreciationJournalLines(asset, asOf, expectedAmount)
	if err != nil {
		t.Fatal(err)
	}
	attachment := &DepreciationAttachment{
		AssetID: assetID, CompanyID: companyID, Amount: expectedAmount, AsOf: asOf,
		EntryID: entry.ID, expectedAsset: asset, UpdatedAsset: expectedUpdated,
	}
	if attachment.AttachmentKind() != "fixed_asset_depreciation" {
		t.Fatalf("attachment kind = %s", attachment.AttachmentKind())
	}
	draft := posting.JournalEntry{
		ID: entry.ID, CompanyID: companyID, Status: posting.EntryDraft,
		EntryDate: asOf, Date: asOf, DocumentReference: depreciationReference(assetID, asOf), Lines: lines,
	}
	if err := attachment.ValidateAttachment(ctx, draft); err != nil {
		t.Fatalf("valid depreciation attachment: %v", err)
	}
	invalid := draft
	invalid.CompanyID = uuid.New()
	if err := attachment.ValidateAttachment(ctx, invalid); !errors.Is(err, ErrCompanyMismatch) {
		t.Fatalf("company mismatch error = %v", err)
	}
	invalid = draft
	invalid.EntryDate = covDate(9)
	if err := attachment.ValidateAttachment(ctx, invalid); !errors.Is(err, ErrInvalidAsset) {
		t.Fatalf("date mismatch error = %v", err)
	}
	invalid = draft
	invalid.Lines = append([]posting.JournalEntryLine(nil), draft.Lines...)
	invalid.Lines[0].AccountID = uuid.New()
	if err := attachment.ValidateAttachment(ctx, invalid); !errors.Is(err, ErrInvalidAsset) {
		t.Fatalf("line mismatch error = %v", err)
	}
	if err := attachment.ValidateAttachment(cancelledContext(), draft); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled attachment error = %v", err)
	}

	posted := draft
	posted.Status = posting.EntryPosted
	if err := attachment.ExecuteAtomicMutation(ctx, &covAtomicTx{state: repo}, posted); err != nil {
		t.Fatalf("depreciation memory mutation: %v", err)
	}
	if err := attachment.ExecuteAtomicMutation(ctx, &covAtomicTx{state: struct{}{}}, posted); err != nil {
		t.Fatalf("depreciation SQL fallback: %v", err)
	}
	queryErr := errors.New("query failed")
	if err := attachment.ExecuteAtomicMutation(ctx, &covAtomicTx{state: struct{}{}, queryErr: queryErr}, posted); !errors.Is(err, ErrConcurrentAssetUpdate) {
		t.Fatalf("depreciation SQL error = %v", err)
	}
	mutationErr := errors.New("mutation failed")
	if err := attachment.ExecuteAtomicMutation(ctx, &covAtomicTx{state: repo, mutationErr: mutationErr}, posted); !errors.Is(err, mutationErr) {
		t.Fatalf("depreciation mutation error = %v", err)
	}
}

func cancelledContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func TestCovFixedAssetOperationAtomicAndValidation(t *testing.T) {
	companyID, assetID, entryID := uuid.New(), uuid.New(), uuid.New()
	expected := covAssetFixture(t, companyID, assetID)
	expected.PendingOperation = "DISPOSAL"
	expected.PendingOperationEntryID = entryID
	disposedAt := covDate(5)
	updated := expected
	updated.DisposedAt = &disposedAt
	clearPendingAssetOperation(&updated)
	updated.AppliedEntryIDs = append(updated.AppliedEntryIDs, entryID)
	zero := covMoney(t, "0", "USD")
	amount := covMoney(t, "100", "USD")
	lines := []posting.JournalEntryLine{
		{CompanyID: companyID, AccountID: expected.AccumulatedAccountID, Debit: amount, Credit: zero},
		{CompanyID: companyID, AccountID: expected.AssetAccountID, Debit: zero, Credit: amount},
	}
	posted := posting.JournalEntry{
		ID: entryID, CompanyID: companyID, Status: posting.EntryPosted,
		EntryDate: covDate(5), Date: covDate(5),
		DocumentReference: disposalReference(assetID, covDate(5)), Lines: lines,
	}
	draft := posted
	draft.Status = posting.EntryDraft

	if err := ValidateFixedAssetOperationMutation(expected, updated, draft); err != nil {
		t.Fatalf("valid operation mutation: %v", err)
	}
	if err := ValidateFixedAssetOperationMutation(expected, updated, posted); err != nil {
		t.Fatalf("valid posted operation mutation: %v", err)
	}

	mutationCases := []struct {
		name     string
		expected FixedAsset
		updated  FixedAsset
		entry    posting.JournalEntry
	}{
		{"nil expected id", func() FixedAsset { a := expected; a.ID = uuid.Nil; return a }(), updated, draft},
		{"wrong updated id", expected, func() FixedAsset { a := updated; a.ID = uuid.New(); return a }(), draft},
		{"nil entry id", expected, updated, func() posting.JournalEntry { e := draft; e.ID = uuid.Nil; return e }()},
		{"wrong entry company", expected, updated, func() posting.JournalEntry { e := draft; e.CompanyID = uuid.New(); return e }()},
		{"bad status", expected, updated, func() posting.JournalEntry { e := draft; e.Status = posting.EntryReversed; return e }()},
		{"wrong pending id", func() FixedAsset { a := expected; a.PendingOperationEntryID = uuid.New(); return a }(), updated, draft},
		{"updated still pending", expected, func() FixedAsset { a := updated; a.PendingOperationEntryID = entryID; return a }(), draft},
		{"bad disposal metadata", expected, updated, func() posting.JournalEntry { e := draft; e.DocumentReference = "other"; return e }()},
		{"impairment disposed", func() FixedAsset { a := expected; a.PendingOperation = "IMPAIRMENT"; return a }(), updated, func() posting.JournalEntry {
			e := draft
			e.DocumentReference = impairmentReference(assetID, covDate(5))
			return e
		}()},
		{"cwip metadata", func() FixedAsset {
			a := expected
			a.PendingOperation = "CWIP_CAPITALIZATION"
			a.IsCWIP = true
			return a
		}(), updated, draft},
		{"unknown operation", func() FixedAsset { a := expected; a.PendingOperation = "other"; return a }(), updated, draft},
	}
	for _, tc := range mutationCases {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateFixedAssetOperationMutation(tc.expected, tc.updated, tc.entry); !errors.Is(err, ErrInvalidAsset) {
				t.Fatalf("error = %v, want invalid asset", err)
			}
		})
	}

	attachment := &fixedAssetOperationAttachment{Expected: expected, Updated: updated, Entry: draft}
	if attachment.AttachmentKind() != "fixed_asset_operation" {
		t.Fatalf("attachment kind = %s", attachment.AttachmentKind())
	}
	if err := attachment.ValidateAttachment(ctxOrBackground(), posted); err != nil {
		t.Fatalf("valid operation attachment: %v", err)
	}
	state := &covOperationAssetState{MemoryRepository: NewMemoryRepository()}
	covSaveAsset(t, state.MemoryRepository, expected)
	state.drafts[entryID] = posted
	if err := attachment.ExecuteAtomicMutation(ctxOrBackground(), &covAtomicTx{state: state}, posted); err != nil {
		t.Fatalf("operation memory mutation: %v", err)
	}
	if err := attachment.ExecuteAtomicMutation(ctxOrBackground(), &covAtomicTx{state: struct{}{}}, posted); err != nil {
		t.Fatalf("operation SQL fallback: %v", err)
	}
	queryErr := errors.New("query failed")
	if err := attachment.ExecuteAtomicMutation(ctxOrBackground(), &covAtomicTx{state: struct{}{}, queryErr: queryErr}, posted); !errors.Is(err, ErrConcurrentAssetUpdate) {
		t.Fatalf("operation SQL error = %v", err)
	}
	mutationErr := errors.New("mutation failed")
	if err := attachment.ExecuteAtomicMutation(ctxOrBackground(), &covAtomicTx{state: state, mutationErr: mutationErr}, posted); !errors.Is(err, mutationErr) {
		t.Fatalf("operation mutation error = %v", err)
	}
	badStatus := posted
	badStatus.Status = posting.EntryDraft
	if err := attachment.ExecuteAtomicMutation(ctxOrBackground(), &covAtomicTx{state: state}, badStatus); !errors.Is(err, ErrInvalidAsset) {
		t.Fatalf("bad status error = %v", err)
	}
	mismatched := posted
	mismatched.DocumentReference = "other"
	if err := attachment.ExecuteAtomicMutation(ctxOrBackground(), &covAtomicTx{state: state}, mismatched); !errors.Is(err, ErrInvalidAsset) {
		t.Fatalf("mismatched entry error = %v", err)
	}
	if err := postFixedAssetOperation(ctxOrBackground(), nil, &draft, expected, updated, "poster"); !errors.Is(err, posting.ErrInvalidEntry) {
		t.Fatalf("nil engine error = %v", err)
	}

	cleared := updated
	cleared.AppliedEntryIDs = nil
	finishFixedAssetOperation(&cleared, entryID)
	if cleared.PendingOperation != "" || cleared.PendingOperationEntryID != uuid.Nil ||
		len(cleared.AppliedEntryIDs) != 1 || cleared.AppliedEntryIDs[0] != entryID {
		t.Fatalf("finished asset = %+v", cleared)
	}

	disposalRepo := &covOperationAssetState{MemoryRepository: NewMemoryRepository()}
	disposalAsset := covAssetFixture(t, companyID, uuid.New())
	disposalAsset.AccumulatedDepreciation = covMoney(t, "100", "USD")
	disposalAsset.CurrentBookValue = covMoney(t, "900", "USD")
	covSaveAsset(t, disposalRepo.MemoryRepository, disposalAsset)
	disposalLedger := NewFixedAssetLedgerForCompany(disposalRepo, nil, companyID)
	disposalEngine := posting.NewEngineWithStore(nil, disposalRepo)
	disposalEntry, err := disposalLedger.DisposeAssetAtomic(ctxOrBackground(), disposalEngine, disposalAsset.ID,
		covDate(5), covMoney(t, "900", "USD"), uuid.New(), "poster")
	if !errors.Is(err, ErrInvalidAsset) || disposalEntry.Status != posting.EntryDraft {
		t.Fatalf("atomic disposal: %+v err=%v", disposalEntry, err)
	}
	disposed, err := disposalRepo.GetFixedAsset(ctxOrBackground(), companyID, disposalAsset.ID)
	if err != nil || disposed.DisposedAt != nil {
		t.Fatalf("asset should remain undisposed after failed post: %+v err=%v", disposed, err)
	}

	impairRepo := &covOperationAssetState{MemoryRepository: NewMemoryRepository()}
	impairAsset := covAssetFixture(t, companyID, uuid.New())
	covSaveAsset(t, impairRepo.MemoryRepository, impairAsset)
	impairLedger := NewFixedAssetLedgerForCompany(impairRepo, nil, companyID)
	impairEngine := posting.NewEngineWithStore(nil, impairRepo)
	impairEntry, err := impairLedger.ImpairAssetAtomic(ctxOrBackground(), impairEngine, impairAsset.ID,
		covMoney(t, "200", "USD"), uuid.New(), covDate(5), "poster")
	if !errors.Is(err, ErrInvalidAsset) || impairEntry.Status != posting.EntryDraft {
		t.Fatalf("atomic impairment: %+v err=%v", impairEntry, err)
	}
	impaired, err := impairRepo.GetFixedAsset(ctxOrBackground(), companyID, impairAsset.ID)
	if err != nil || !impaired.AccumulatedImpairment.Amount().IsZero() ||
		!impaired.CurrentBookValue.Equal(covMoney(t, "1000", "USD")) {
		t.Fatalf("asset should remain unimpaired after failed post: %+v err=%v", impaired, err)
	}

	postImpairRepo := &covOperationAssetState{MemoryRepository: NewMemoryRepository()}
	postImpairAsset := covAssetFixture(t, companyID, uuid.New())
	covSaveAsset(t, postImpairRepo.MemoryRepository, postImpairAsset)
	postImpairLedger := NewFixedAssetLedgerForCompany(postImpairRepo, nil, companyID)
	postImpairEngine := posting.NewEngineWithStore(nil, postImpairRepo)
	if _, err := postImpairLedger.PostImpairmentAtomic(ctxOrBackground(), postImpairEngine, postImpairAsset.ID,
		covMoney(t, "800", "USD"), uuid.New(), covDate(5), "poster"); !errors.Is(err, ErrInvalidAsset) {
		t.Fatalf("atomic post impairment error = %v", err)
	}

	invalidRecovery := []struct {
		name    string
		assetID uuid.UUID
		amount  financialtypes.Money
	}{
		{"unknown", uuid.New(), covMoney(t, "800", "USD")},
		{"currency", postImpairAsset.ID, covMoney(t, "800", "EUR")},
		{"negative", postImpairAsset.ID, covMoney(t, "-1", "USD")},
		{"above book", postImpairAsset.ID, covMoney(t, "2000", "USD")},
		{"no loss", postImpairAsset.ID, covMoney(t, "1000", "USD")},
	}
	for _, tc := range invalidRecovery {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := postImpairLedger.PostImpairmentAtomic(ctxOrBackground(), postImpairEngine, tc.assetID,
				tc.amount, uuid.New(), covDate(5), "poster"); err == nil {
				t.Fatal("expected post impairment failure")
			}
		})
	}
	if err := postImpairLedger.Impair(postImpairAsset.ID, covMoney(t, "1", "USD")); !errors.Is(err, ErrInvalidAsset) {
		t.Fatalf("legacy impairment error = %v", err)
	}
	legacyRepo := NewMemoryRepository()
	legacyAsset := covAssetFixture(t, companyID, uuid.New())
	covSaveAsset(t, legacyRepo, legacyAsset)
	legacyLedger := NewFixedAssetLedgerForCompany(legacyRepo, nil, companyID)
	if _, err := legacyLedger.PostImpairment(legacyAsset.ID, covMoney(t, "1", "USD"), uuid.New(), covDate(5)); err != nil {
		t.Fatalf("legacy post impairment: %v", err)
	}
	if _, err := legacyLedger.PostImpairmentContext(cancelledContext(), legacyAsset.ID,
		covMoney(t, "1", "USD"), uuid.New(), covDate(5)); !errors.Is(err, context.Canceled) {
		t.Fatalf("post impairment cancelled error = %v", err)
	}
	if _, err := legacyLedger.PostImpairmentContext(ctxOrBackground(), legacyAsset.ID,
		covMoney(t, "1", "EUR"), uuid.New(), covDate(5)); !errors.Is(err, ErrInvalidAsset) {
		t.Fatalf("post impairment currency error = %v", err)
	}
}

func TestCovAssetSchedulePublicValidation(t *testing.T) {
	companyID := uuid.New()
	repo := NewMemoryRepository()
	ledger := NewFixedAssetLedgerForCompany(repo, nil, companyID)
	ctx := context.Background()
	valid := apisubledgerFixedAsset(t, companyID, uuid.New())

	if err := ledger.SaveAsset(ctx, publicCompanyID(companyID), valid); err != nil {
		t.Fatalf("valid public asset: %v", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	cases := []struct {
		name      string
		ctx       context.Context
		companyID uuid.UUID
		asset     apisubledgerFixedAssetType
		want      error
	}{
		{"cancelled", cancelled, companyID, valid, context.Canceled},
		{"nil company", ctx, uuid.Nil, valid, ErrCompanyMismatch},
		{"public company mismatch", ctx, companyID, func() apisubledgerFixedAssetType {
			a := valid
			a.CompanyID = publicCompanyID(uuid.New())
			return a
		}(), ErrCompanyMismatch},
		{"empty id", ctx, companyID, func() apisubledgerFixedAssetType { a := valid; a.ID = ""; return a }(), ErrInvalidAsset},
		{"malformed id", ctx, companyID, func() apisubledgerFixedAssetType { a := valid; a.ID = "bad"; return a }(), ErrInvalidAsset},
		{"zero acquisition date", ctx, companyID, func() apisubledgerFixedAssetType {
			a := valid
			a.AcquisitionDate = time.Time{}
			return a
		}(), ErrInvalidAsset},
		{"bad method", ctx, companyID, func() apisubledgerFixedAssetType { a := valid; a.DepreciationMethod = "other"; return a }(), ErrInvalidAsset},
		{"cwip status", ctx, companyID, func() apisubledgerFixedAssetType { a := valid; a.Status = "cwip"; return a }(), ErrInvalidAsset},
		{"bad status", ctx, companyID, func() apisubledgerFixedAssetType { a := valid; a.Status = "other"; return a }(), ErrInvalidAsset},
		{"duplicate", ctx, companyID, valid, ErrAlreadyExists},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := ledger.SaveAsset(tc.ctx, publicCompanyID(tc.companyID), tc.asset); !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
		})
	}

	if _, err := ledger.ComputeMonthlyDepreciation(apisubledgerFixedAssetType{}); !errors.Is(err, ErrInvalidAsset) {
		t.Fatalf("empty monthly depreciation error = %v", err)
	}
	monthly := valid
	monthly.AccumulatedDep = covPublicMoney(t, "-1", "USD")
	if _, err := ledger.ComputeMonthlyDepreciation(monthly); !errors.Is(err, ErrInvalidAsset) {
		t.Fatalf("negative accumulated error = %v", err)
	}
	monthly = valid
	monthly.NetBookValue = covPublicMoney(t, "-1", "USD")
	if _, err := ledger.ComputeMonthlyDepreciation(monthly); !errors.Is(err, ErrInvalidAsset) {
		t.Fatalf("negative book value error = %v", err)
	}
	monthly = valid
	monthly.DepreciationMethod = "other"
	if _, err := ledger.ComputeMonthlyDepreciation(monthly); !errors.Is(err, ErrInvalidAsset) {
		t.Fatalf("unknown method error = %v", err)
	}

	if _, err := CalculateProrataDepreciation(decimal.NewFromInt(-1), decimal.Zero, 12, covDate(1), covDate(2)); !errors.Is(err, ErrInvalidAsset) {
		t.Fatalf("negative cost error = %v", err)
	}
	if _, err := CalculateProrataDepreciation(decimal.NewFromInt(100), decimal.Zero, 12, time.Time{}, covDate(2)); !errors.Is(err, ErrInvalidAsset) {
		t.Fatalf("zero start error = %v", err)
	}
	if _, err := GenerateDepreciationSchedule(FixedAsset{}); !errors.Is(err, ErrInvalidAsset) {
		t.Fatalf("empty schedule error = %v", err)
	}
	if _, err := depreciationComponents(FixedAsset{IsCWIP: true}); !errors.Is(err, ErrInvalidAsset) {
		t.Fatalf("cwip components error = %v", err)
	}
	badComponent := covAssetFixture(t, companyID, uuid.New())
	badComponent.Components = []AssetComponent{{ID: uuid.New(), Name: "part", Cost: covMoney(t, "100", "USD")}}
	if _, err := depreciationComponents(badComponent); !errors.Is(err, ErrInvalidAsset) {
		t.Fatalf("invalid component error = %v", err)
	}
	invalidScheduleMethod := covAssetFixture(t, companyID, uuid.New())
	invalidScheduleMethod.Method = DepreciationMethod("other")
	if _, err := GenerateDepreciationSchedule(invalidScheduleMethod); !errors.Is(err, ErrInvalidAsset) {
		t.Fatalf("invalid schedule method error = %v", err)
	}
	declining := AssetComponent{
		ID: uuid.New(), Name: "part", Cost: covMoney(t, "100", "USD"), SalvageValue: covMoney(t, "0", "USD"),
		UsefulLifeMonths: 12, Method: DecliningBalance, InServiceDate: covDate(1), DecliningRate: decimal.NewFromInt(2),
	}
	if _, err := generateComponentSchedule(uuid.New(), declining); !errors.Is(err, ErrInvalidAsset) {
		t.Fatalf("invalid declining rate error = %v", err)
	}
	unknownComponentMethod := declining
	unknownComponentMethod.Method = DepreciationMethod("other")
	unknownComponentMethod.DecliningRate = decimal.Zero
	if _, err := generateComponentSchedule(uuid.New(), unknownComponentMethod); !errors.Is(err, ErrInvalidAsset) {
		t.Fatalf("unknown component method error = %v", err)
	}
}
