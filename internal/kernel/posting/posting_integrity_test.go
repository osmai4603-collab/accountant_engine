package posting

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/osmai4603-collab/accounting_engine/internal/kernel/journal"
	"github.com/osmai4603-collab/accounting_engine/internal/kernel/masterdata"
	"github.com/osmai4603-collab/accounting_engine/internal/kernel/policy"
	"github.com/osmai4603-collab/accounting_engine/internal/kernel/types"
)

// effectiveSnapshotReader models an adapter whose current-state read and whose
// effective-dated read disagree. ADR P3-01 makes the effective-dated snapshot
// authoritative for a posting date; the posting pipeline must therefore use a
// single resolved snapshot for both validation and eligibility.
type effectiveSnapshotReader struct {
	masterdata.Reader
	effective masterdata.Account
	current   masterdata.Account
	reads     int
}

func (r *effectiveSnapshotReader) GetAccountAsOf(
	_ context.Context,
	companyID types.CompanyID,
	accountID types.AccountID,
	_ time.Time,
) (masterdata.Account, error) {
	account := r.effective
	account.ID, account.CompanyID = accountID, companyID
	return account, nil
}

func (r *effectiveSnapshotReader) GetAccount(
	_ context.Context,
	companyID types.CompanyID,
	accountID types.AccountID,
) (masterdata.Account, error) {
	r.reads++
	account := r.current
	account.ID, account.CompanyID = accountID, companyID
	return account, nil
}

func (r *effectiveSnapshotReader) GetJournalByID(
	ctx context.Context,
	companyID types.CompanyID,
	journalID types.JournalID,
) (masterdata.Journal, error) {
	repository, ok := r.Reader.(masterdata.JournalRepository)
	if !ok {
		return masterdata.Journal{}, masterdata.ErrJournalReaderRequired
	}
	return repository.GetJournalByID(ctx, companyID, journalID)
}

func TestPostEntryUsesEffectiveAccountSnapshotForEligibility(t *testing.T) {
	entry := newPostingTestEntry(t, "creator")
	accountID := entry.Lines[0].AccountID
	reader := &effectiveSnapshotReader{
		Reader: &mockMasterReaderForPosting{compActive: true, periodOpen: true},
		effective: masterdata.Account{
			ID: accountID, CompanyID: entry.CompanyID, Code: "1000",
			IsActive: true, Type: masterdata.AccountTypeAsset,
		},
		// The current-state row became a control account after the posting date.
		current: masterdata.Account{
			ID: accountID, CompanyID: entry.CompanyID, Code: "1000",
			IsActive: true, IsControlAccount: true, Type: masterdata.AccountTypeAsset,
		},
	}
	executorCalls := 0
	pipeline := NewPostingPipeline(reader,
		func(_ context.Context, posted journal.JournalEntry, _ PostOptions) (journal.JournalEntry, error) {
			executorCalls++
			return posted, nil
		},
		WithPolicyEngine(testPostingPolicyEngine(t, entry.CompanyID, "USD")),
	)

	if _, err := pipeline.PostEntry(context.Background(), entry, PostOptions{
		ActorID: "poster", IdempotencyKey: "phase6-effective-snapshot",
	}); err != nil {
		t.Fatalf("PostEntry must use the effective account snapshot, got: %v", err)
	}
	if executorCalls != 1 {
		t.Fatalf("executor calls = %d, want 1", executorCalls)
	}
	if reader.reads != 0 {
		t.Fatalf("current-state GetAccount reads = %d, want 0: the posting must not re-read a second, non-effective snapshot", reader.reads)
	}
}

// mutatingJournalReader injects an adversarial caller-side mutation at a point
// where PostEntry has already cloned the entry but has not reached the executor.
type mutatingJournalReader struct {
	*mockMasterReaderForPosting
	mutate func()
}

func (r *mutatingJournalReader) GetJournalByID(
	ctx context.Context,
	companyID types.CompanyID,
	journalID types.JournalID,
) (masterdata.Journal, error) {
	if r.mutate != nil {
		r.mutate()
	}
	return r.mockMasterReaderForPosting.GetJournalByID(ctx, companyID, journalID)
}

func TestPostEntryExecutesFrozenValidatedSnapshot(t *testing.T) {
	entry := newPostingTestEntry(t, "creator")
	expected := entry.Clone()
	mutatedAccount := types.NewAccountID()
	mutatedAmount := makeMoney(t, "999.00", "EUR")
	mutatedZero := makeMoney(t, "0", "EUR")

	reader := &mutatingJournalReader{
		mockMasterReaderForPosting: &mockMasterReaderForPosting{compActive: true, periodOpen: true},
		mutate: func() {
			entry.CompanyID = types.CompanyID{}
			entry.TargetPeriodNumber = 14
			entry.FiscalPeriodID = types.NewFiscalPeriodID()
			entry.Lines[0].AccountID = mutatedAccount
			entry.Lines[0].TransactionCurrency = "EUR"
			entry.Lines[0].TransactionAmount = mutatedAmount
			entry.Lines[0].FunctionalDebit = mutatedAmount
			entry.Lines[0].FunctionalCredit = mutatedZero
		},
	}

	executorCalls := 0
	var executed journal.JournalEntry
	pipeline := NewPostingPipeline(reader,
		func(_ context.Context, posted journal.JournalEntry, _ PostOptions) (journal.JournalEntry, error) {
			executorCalls++
			executed = posted.Clone()
			return posted, nil
		},
		WithPolicyEngine(testPostingPolicyEngine(t, expected.CompanyID, "USD")),
	)
	if _, err := pipeline.PostEntry(context.Background(), entry, PostOptions{
		ActorID: "poster", IdempotencyKey: "phase6-frozen-snapshot",
	}); err != nil {
		t.Fatalf("PostEntry with adversarial caller mutation: %v", err)
	}
	if executorCalls != 1 {
		t.Fatalf("executor calls = %d, want 1", executorCalls)
	}
	if entry.CompanyID != (types.CompanyID{}) || entry.Lines[0].AccountID != mutatedAccount ||
		entry.Lines[0].TransactionCurrency != "EUR" || entry.TargetPeriodNumber != 14 {
		t.Fatal("adversarial mutation did not reach the caller entry; the TOCTOU simulation is invalid")
	}
	if executed.CompanyID != expected.CompanyID {
		t.Errorf("executor company = %s, want validated company %s", executed.CompanyID, expected.CompanyID)
	}
	if executed.TargetPeriodNumber != expected.TargetPeriodNumber {
		t.Errorf("executor target period = %d, want validated period %d", executed.TargetPeriodNumber, expected.TargetPeriodNumber)
	}
	if executed.Lines[0].AccountID != expected.Lines[0].AccountID {
		t.Errorf("executor account = %s, want validated account %s", executed.Lines[0].AccountID, expected.Lines[0].AccountID)
	}
	if executed.Lines[0].TransactionCurrency != expected.Lines[0].TransactionCurrency ||
		!executed.Lines[0].TransactionAmount.Equal(expected.Lines[0].TransactionAmount) ||
		!executed.Lines[0].FunctionalDebit.Equal(expected.Lines[0].FunctionalDebit) ||
		!executed.Lines[0].FunctionalCredit.Equal(expected.Lines[0].FunctionalCredit) {
		t.Error("executor did not receive the validated currency and amount snapshot")
	}
}

func TestPostEntryRejectsDraftWithPreexistingReversalLink(t *testing.T) {
	base := newPostingTestEntry(t, "creator")
	reversalID := types.NewJournalEntryID()

	for _, tc := range []struct {
		name  string
		entry journal.JournalEntry
	}{
		{name: "regular draft", entry: base},
		{name: "reversal draft", entry: func() journal.JournalEntry {
			posted := base
			posted.Status = journal.EntryPosted
			rev, err := BuildReversalEntry(posted, base.EffectiveDate, "poster", "phase6 link test")
			if err != nil {
				t.Fatal(err)
			}
			return rev
		}()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entry := tc.entry
			entry.ReversedByID = &reversalID
			executorCalls, reversalCalls := 0, 0
			pipeline := NewPostingPipeline(
				&mockMasterReaderForPosting{compActive: true, periodOpen: true},
				func(_ context.Context, posted journal.JournalEntry, _ PostOptions) (journal.JournalEntry, error) {
					executorCalls++
					return posted, nil
				},
				WithPolicyEngine(testPostingPolicyEngine(t, entry.CompanyID, "USD")),
				WithReversalExecutor(func(_ context.Context, _ types.JournalEntryID, posted journal.JournalEntry, _ PostOptions) (journal.JournalEntry, error) {
					reversalCalls++
					return posted, nil
				}),
			)
			if _, err := pipeline.PostEntry(context.Background(), entry, PostOptions{
				ActorID: "poster", IdempotencyKey: "phase6-reversal-link",
			}); !errors.Is(err, ErrInvalidReversal) {
				t.Fatalf("PostEntry error = %v, want ErrInvalidReversal", err)
			}
			if executorCalls != 0 || reversalCalls != 0 {
				t.Fatalf("executor calls = %d, reversal calls = %d, want 0/0 before a linked draft is rejected", executorCalls, reversalCalls)
			}
		})
	}
}

func TestPostEntryExecutorNeverRunsForRejectedInputs(t *testing.T) {
	base := newPostingTestEntry(t, "creator")
	engine := testPostingPolicyEngine(t, base.CompanyID, "USD")

	for _, tc := range []struct {
		name    string
		mutate  func(*journal.JournalEntry, *mockMasterReaderForPosting)
		wantErr error
	}{
		{
			name: "company scope removed",
			mutate: func(e *journal.JournalEntry, _ *mockMasterReaderForPosting) {
				e.CompanyID = types.CompanyID{}
			},
			wantErr: masterdata.ErrCompanyScopeRequired,
		},
		{
			name: "foreign account",
			mutate: func(_ *journal.JournalEntry, r *mockMasterReaderForPosting) {
				r.foreignAccount = true
			},
			wantErr: masterdata.ErrAccountIdentityMismatch,
		},
		{
			name: "transaction currency mismatch",
			mutate: func(e *journal.JournalEntry, _ *mockMasterReaderForPosting) {
				e.Lines[0].TransactionCurrency = "EUR"
			},
			wantErr: ErrTransactionCurrencyMismatch,
		},
		{
			name: "functional currency mismatch",
			mutate: func(e *journal.JournalEntry, _ *mockMasterReaderForPosting) {
				e.Lines[0].FunctionalDebit = makeMoney(t, "100.00", "EUR")
			},
			wantErr: ErrFunctionalCurrencyMismatch,
		},
		{
			name: "hard closed period",
			mutate: func(_ *journal.JournalEntry, r *mockMasterReaderForPosting) {
				r.periodOpen = false
			},
			wantErr: masterdata.ErrClosedPeriod,
		},
		{
			name: "period 13 with regular classification",
			mutate: func(e *journal.JournalEntry, _ *mockMasterReaderForPosting) {
				e.TargetPeriodNumber = 13
			},
			wantErr: masterdata.ErrInvalidAdjustmentPeriod,
		},
		{
			// A functional residual after normalization may never reach the
			// executor without an explicitly configured rounding settlement.
			name: "unbalanced functional totals require policy settlement",
			mutate: func(e *journal.JournalEntry, _ *mockMasterReaderForPosting) {
				e.Lines[1].TransactionAmount = makeMoney(t, "90.00", "USD")
				e.Lines[1].FunctionalCredit = makeMoney(t, "90.00", "USD")
			},
			wantErr: ErrRoundingPolicyRequired,
		},
		{
			name: "control account rejected by eligibility",
			mutate: func(_ *journal.JournalEntry, r *mockMasterReaderForPosting) {
				r.controlAccount = true
			},
			wantErr: policy.ErrControlAccountNotPostable,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entry := base.Clone()
			reader := &mockMasterReaderForPosting{compActive: true, periodOpen: true}
			tc.mutate(&entry, reader)
			executorCalls, reversalCalls := 0, 0
			pipeline := NewPostingPipeline(reader,
				func(_ context.Context, posted journal.JournalEntry, _ PostOptions) (journal.JournalEntry, error) {
					executorCalls++
					return posted, nil
				},
				WithPolicyEngine(engine),
				WithReversalExecutor(func(_ context.Context, _ types.JournalEntryID, posted journal.JournalEntry, _ PostOptions) (journal.JournalEntry, error) {
					reversalCalls++
					return posted, nil
				}),
			)
			if _, err := pipeline.PostEntry(context.Background(), entry, PostOptions{
				ActorID: "poster", IdempotencyKey: "phase6-rejection-matrix",
			}); !errors.Is(err, tc.wantErr) {
				t.Fatalf("PostEntry error = %v, want %v", err, tc.wantErr)
			}
			if executorCalls != 0 || reversalCalls != 0 {
				t.Fatalf("executor calls = %d, reversal calls = %d, want 0/0 on rejected input", executorCalls, reversalCalls)
			}
		})
	}
}

func TestReversalBuilderLeavesOriginalImmutable(t *testing.T) {
	entry := newPostingTestEntry(t, "creator")
	entry.Status = journal.EntryPosted
	postedAt := entry.EffectiveDate
	entry.PostedAt = &postedAt
	entry.PostedBy = "poster"
	partnerID := types.NewPartnerID()
	costCenterID := types.NewCostCenterID()
	projectID := types.NewProjectID()
	segmentID := types.NewSegmentID()
	businessUnitID := types.NewBusinessUnitID()
	entry.Lines[0].PartnerID = &partnerID
	entry.Lines[0].CostCenterID = &costCenterID
	entry.Lines[0].ProjectID = &projectID
	entry.Lines[0].SegmentID = &segmentID
	entry.Lines[0].BusinessUnitID = &businessUnitID
	entry.Lines[0].MatchingNumber = "MATCH-PHASE6"
	before := entry.Clone()

	reversal, err := BuildReversalEntryAt(
		entry, entry.EffectiveDate.AddDate(0, 1, 0), "poster", "immutability",
		time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := journal.ValidateImmutability(before, entry); err != nil {
		t.Fatalf("reversal builder mutated the original posted entry: %v", err)
	}
	if reversal.ReversalOfID == nil || *reversal.ReversalOfID != entry.ID {
		t.Fatalf("reversal link = %v, want original id", reversal.ReversalOfID)
	}
	if reversal.FiscalPeriodID != (types.FiscalPeriodID{}) {
		t.Fatal("reversal builder must not inherit the original fiscal period; posting derives it from reversal date")
	}
	if len(reversal.Lines) != len(before.Lines) {
		t.Fatalf("reversal line count = %d, want %d", len(reversal.Lines), len(before.Lines))
	}
	for i, want := range before.Lines {
		got := reversal.Lines[i]
		name := fmt.Sprintf("line %d", i)
		if got.AccountID != want.AccountID {
			t.Errorf("%s account = %s, want %s", name, got.AccountID, want.AccountID)
		}
		assertPointerEqual(t, name+" partner", got.PartnerID, want.PartnerID)
		assertPointerEqual(t, name+" cost center", got.CostCenterID, want.CostCenterID)
		assertPointerEqual(t, name+" project", got.ProjectID, want.ProjectID)
		assertPointerEqual(t, name+" segment", got.SegmentID, want.SegmentID)
		assertPointerEqual(t, name+" business unit", got.BusinessUnitID, want.BusinessUnitID)
		if got.MatchingNumber != want.MatchingNumber {
			t.Errorf("%s matching number = %q, want %q", name, got.MatchingNumber, want.MatchingNumber)
		}
		if got.TransactionCurrency != want.TransactionCurrency ||
			!got.TransactionAmount.Equal(want.TransactionAmount) {
			t.Errorf("%s transaction currency/amount = %s/%s, want %s/%s",
				name, got.TransactionCurrency, got.TransactionAmount,
				want.TransactionCurrency, want.TransactionAmount)
		}
		if !got.ExchangeRate.Equal(want.ExchangeRate) {
			t.Errorf("%s exchange rate = %s, want %s", name, got.ExchangeRate, want.ExchangeRate)
		}
		if !got.FunctionalDebit.Equal(want.FunctionalCredit) ||
			!got.FunctionalCredit.Equal(want.FunctionalDebit) {
			t.Errorf("%s did not swap the debit and credit sides", name)
		}
	}
}

func assertPointerEqual[T comparable](t *testing.T, name string, got, want *T) {
	t.Helper()
	if got == nil && want == nil {
		return
	}
	if got == nil || want == nil || *got != *want {
		t.Errorf("%s = %v, want %v", name, got, want)
	}
}

func TestPostingProductionCodeUsesOnlyInjectedClocks(t *testing.T) {
	files, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".go") || strings.HasSuffix(file.Name(), "_test.go") {
			continue
		}
		path := file.Name()
		fset := token.NewFileSet()
		parsed, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		allowed := (*ast.FuncDecl)(nil)
		if path == "pipeline.go" {
			allowed, err = systemClockNowRange(fset, parsed)
			if err != nil {
				t.Fatal(err)
			}
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := selector.X.(*ast.Ident)
			if !ok || pkg.Name != "time" || selector.Sel.Name != "Now" {
				return true
			}
			if allowed != nil && call.Pos() >= allowed.Pos() && call.End() <= allowed.End() {
				return true
			}
			t.Errorf("%s calls time.Now directly at %s; use the Clock boundary instead", path, fset.Position(call.Pos()))
			return true
		})
	}
}

func systemClockNowRange(fset *token.FileSet, parsed *ast.File) (*ast.FuncDecl, error) {
	_ = fset
	for _, decl := range parsed.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || fn.Name.Name != "Now" || len(fn.Recv.List) != 1 {
			continue
		}
		if ident, ok := fn.Recv.List[0].Type.(*ast.Ident); ok && ident.Name == "systemClock" {
			return fn, nil
		}
	}
	return nil, nil
}
