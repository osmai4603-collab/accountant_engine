package standards

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/journal"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	publicapplication "github.com/osmai4603-collab/accounting_engine/pkg/spi/application"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/masterdata"
	"github.com/shopspring/decimal"
)

type coveragePostingApp struct {
	getEntryFn    func(context.Context, publicapplication.GetEntryQuery) (journal.JournalEntry, error)
	createDraftFn func(context.Context, publicapplication.CreateDraftCommand) (publicapplication.EntryResult, error)
	postFn        func(context.Context, publicapplication.PostEntryCommand) (publicapplication.EntryResult, error)
}

func (a *coveragePostingApp) CreateDraft(ctx context.Context, cmd publicapplication.CreateDraftCommand) (publicapplication.EntryResult, error) {
	if a.createDraftFn == nil {
		return publicapplication.EntryResult{}, errors.New("unexpected CreateDraft call")
	}
	return a.createDraftFn(ctx, cmd)
}

func (a *coveragePostingApp) Validate(context.Context, publicapplication.ValidateEntryCommand) error {
	return nil
}

func (a *coveragePostingApp) Post(ctx context.Context, cmd publicapplication.PostEntryCommand) (publicapplication.EntryResult, error) {
	if a.postFn == nil {
		return publicapplication.EntryResult{}, errors.New("unexpected Post call")
	}
	return a.postFn(ctx, cmd)
}

func (a *coveragePostingApp) Reverse(context.Context, publicapplication.ReverseEntryCommand) (publicapplication.EntryResult, error) {
	return publicapplication.EntryResult{}, nil
}

func (a *coveragePostingApp) GetEntry(ctx context.Context, query publicapplication.GetEntryQuery) (journal.JournalEntry, error) {
	if a.getEntryFn == nil {
		return journal.JournalEntry{}, errors.New("entry not found")
	}
	return a.getEntryFn(ctx, query)
}

type coverageIDPostingApp struct {
	coveragePostingApp
	createDraftWithIDFn func(context.Context, publicapplication.CreateDraftCommand, types.JournalEntryID) (publicapplication.EntryResult, error)
}

func (a *coverageIDPostingApp) CreateDraftWithID(
	ctx context.Context, cmd publicapplication.CreateDraftCommand, id types.JournalEntryID,
) (publicapplication.EntryResult, error) {
	if a.createDraftWithIDFn == nil {
		return publicapplication.EntryResult{}, errors.New("unexpected CreateDraftWithID call")
	}
	return a.createDraftWithIDFn(ctx, cmd, id)
}

type coverageAccountReader struct {
	coverageMasterDataReader
	err   error
	calls int
}

func (m *coverageAccountReader) GetAccount(_ context.Context, _ types.CompanyID, _ types.AccountID) (masterdata.Account, error) {
	m.calls++
	if m.err != nil {
		return masterdata.Account{}, m.err
	}
	return masterdata.Account{}, nil
}

func coverageMustMoney(t *testing.T, amount, currency string) types.Money {
	t.Helper()
	money, err := types.NewMoney(decimal.RequireFromString(amount), currency)
	if err != nil {
		t.Fatalf("money %s %s: %v", amount, currency, err)
	}
	return money
}

func coveragePostingRequest(t *testing.T) StandardPostingRequest {
	t.Helper()
	amount := coverageMustMoney(t, "100.00", "USD")
	zero := coverageMustMoney(t, "0.00", "USD")
	return StandardPostingRequest{
		StandardRef:    "IFRS-15-REV-REC",
		EntityID:       "contract-1",
		IdempotencyKey: "key-1",
		CompanyID:      types.NewCompanyID(),
		JournalID:      types.NewJournalID(),
		EffectiveDate:  time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC),
		Description:    "Revenue recognition",
		Lines: []journal.EntryLine{
			{
				AccountID:           types.NewAccountID(),
				TransactionCurrency: "USD",
				TransactionAmount:   amount,
				ExchangeRate:        decimal.NewFromInt(1),
				FunctionalDebit:     amount,
				FunctionalCredit:    zero,
			},
			{
				AccountID:           types.NewAccountID(),
				TransactionCurrency: "USD",
				TransactionAmount:   amount,
				ExchangeRate:        decimal.NewFromInt(1),
				FunctionalDebit:     zero,
				FunctionalCredit:    amount,
			},
		},
	}
}

func coveragePostingContext(companyID types.CompanyID) context.Context {
	ctx := publicapplication.WithPrincipal(context.Background(), publicapplication.Principal{
		ActorID:    "tester",
		TenantID:   "tenant-1",
		CompanyIDs: []types.CompanyID{companyID},
	})
	return publicapplication.WithRequestMetadata(ctx, publicapplication.RequestMetadata{
		CorrelationID:  "corr-1",
		IdempotencyKey: "key-1",
		SourceSystem:   "test",
	})
}

func coverageNotFindingApp() *coveragePostingApp {
	return &coveragePostingApp{
		getEntryFn: func(context.Context, publicapplication.GetEntryQuery) (journal.JournalEntry, error) {
			return journal.JournalEntry{}, errors.New("not found")
		},
	}
}

func TestCoverageStandardPostingValidationErrors(t *testing.T) {
	base := coveragePostingRequest(t)
	ctx := coveragePostingContext(base.CompanyID)

	tests := []struct {
		name string
		ctx  context.Context
		edit func(*StandardPostingRequest, *testing.T)
		app  publicapplication.Service
		md   MasterDataReader
		want error
	}{
		{"nil app", ctx, nil, nil, nil, ErrPostingServiceRequired},
		{"nil context", nil, nil, coverageNotFindingApp(), nil, ErrPrincipalRequired},
		{"cancelled context", func() context.Context {
			c, cancel := context.WithCancel(ctx)
			cancel()
			return c
		}(), nil, coverageNotFindingApp(), nil, context.Canceled},
		{"standard ref", ctx, func(r *StandardPostingRequest, _ *testing.T) { r.StandardRef = " " }, coverageNotFindingApp(), nil, ErrStandardRefRequired},
		{"entity id", ctx, func(r *StandardPostingRequest, _ *testing.T) { r.EntityID = "" }, coverageNotFindingApp(), nil, ErrEntityIDRequired},
		{"idempotency key", ctx, func(r *StandardPostingRequest, _ *testing.T) { r.IdempotencyKey = " " }, coverageNotFindingApp(), nil, ErrIdempotencyKeyRequired},
		{"company id", ctx, func(r *StandardPostingRequest, _ *testing.T) { r.CompanyID = types.CompanyID{} }, coverageNotFindingApp(), nil, ErrInvalidCompanyID},
		{"journal id", ctx, func(r *StandardPostingRequest, _ *testing.T) { r.JournalID = types.JournalID{} }, coverageNotFindingApp(), nil, ErrInvalidJournalID},
		{"effective date", ctx, func(r *StandardPostingRequest, _ *testing.T) { r.EffectiveDate = time.Time{} }, coverageNotFindingApp(), nil, ErrInvalidEffectiveDate},
		{"line count", ctx, func(r *StandardPostingRequest, _ *testing.T) { r.Lines = r.Lines[:1] }, coverageNotFindingApp(), nil, ErrLinesRequired},
		{"line account", ctx, func(r *StandardPostingRequest, _ *testing.T) { r.Lines[0].AccountID = types.AccountID{} }, coverageNotFindingApp(), nil, ErrLinesRequired},
		{"line currency missing", ctx, func(r *StandardPostingRequest, tt *testing.T) {
			for i := range r.Lines {
				r.Lines[i].TransactionCurrency = ""
				r.Lines[i].TransactionAmount = types.Money{}
				r.Lines[i].FunctionalDebit = types.Money{}
				r.Lines[i].FunctionalCredit = types.Money{}
			}
		}, coverageNotFindingApp(), nil, ErrCurrencyMismatch},
		{"line currency mismatch", ctx, func(r *StandardPostingRequest, tt *testing.T) {
			r.Lines[1].TransactionCurrency = "EUR"
		}, coverageNotFindingApp(), nil, ErrCurrencyMismatch},
		{"unbalanced", ctx, func(r *StandardPostingRequest, tt *testing.T) {
			r.Lines[1].FunctionalCredit = coverageMustMoney(tt, "50.00", "USD")
		}, coverageNotFindingApp(), nil, ErrUnbalancedLines},
		{"zero totals", ctx, func(r *StandardPostingRequest, tt *testing.T) {
			zero := coverageMustMoney(tt, "0.00", "USD")
			r.Lines[0].FunctionalDebit = zero
			r.Lines[1].FunctionalCredit = zero
		}, coverageNotFindingApp(), nil, ErrUnbalancedLines},
		{"account lookup failure", ctx, nil, coverageNotFindingApp(), &coverageAccountReader{err: errors.New("no account")}, ErrAccountNotFound},
		{"no principal", context.Background(), nil, coverageNotFindingApp(), nil, ErrPrincipalRequired},
		{"blank actor", publicapplication.WithPrincipal(context.Background(), publicapplication.Principal{
			CompanyIDs: []types.CompanyID{base.CompanyID},
		}), nil, coverageNotFindingApp(), nil, ErrPrincipalRequired},
		{"unauthorized company", publicapplication.WithPrincipal(context.Background(), publicapplication.Principal{
			ActorID:    "tester",
			CompanyIDs: []types.CompanyID{types.NewCompanyID()},
		}), nil, coverageNotFindingApp(), nil, ErrUnauthorizedCompany},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := base
			req.Lines = append([]journal.EntryLine(nil), base.Lines...)
			if tc.edit != nil {
				tc.edit(&req, t)
			}
			poster := NewStandardPoster(tc.app, tc.md)
			if _, err := poster.PostStandardEntry(tc.ctx, req); !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
		})
	}

	reader := &coverageAccountReader{}
	poster := NewStandardPoster(coverageNotFindingApp(), reader)
	if _, err := poster.PostStandardEntry(ctx, base); err == nil {
		t.Fatal("expected account lookup path to be exercised")
	}
	if reader.calls != len(base.Lines) {
		t.Fatalf("account lookups = %d, want %d", reader.calls, len(base.Lines))
	}

	var nilPoster *defaultStandardPoster
	if _, err := nilPoster.PostStandardEntry(ctx, base); !errors.Is(err, ErrPostingServiceRequired) {
		t.Fatalf("nil poster error = %v", err)
	}
	if _, err := NewStandardPostingCapability(nil).PostStandardEntry(ctx, base); !errors.Is(err, ErrPostingServiceRequired) {
		t.Fatalf("nil ports poster error = %v", err)
	}
	ports := NewModulePorts(nil, nil, nil, coverageNotFindingApp())
	if _, err := NewStandardPostingCapability(ports).PostStandardEntry(ctx, base); err == nil {
		t.Fatal("expected error from poster built from ports")
	}
}

func TestCoverageStandardPostingHappyPath(t *testing.T) {
	req := coveragePostingRequest(t)
	ctx := coveragePostingContext(req.CompanyID)
	app := coverageNotFindingApp()
	var draftCmd publicapplication.CreateDraftCommand
	var getQuery publicapplication.GetEntryQuery
	var postCmd publicapplication.PostEntryCommand
	draftID := types.NewJournalEntryID()
	app.createDraftFn = func(_ context.Context, cmd publicapplication.CreateDraftCommand) (publicapplication.EntryResult, error) {
		draftCmd = cmd
		return publicapplication.EntryResult{Entry: journal.JournalEntry{ID: draftID, Status: journal.EntryDraft}}, nil
	}
	app.postFn = func(_ context.Context, cmd publicapplication.PostEntryCommand) (publicapplication.EntryResult, error) {
		postCmd = cmd
		return publicapplication.EntryResult{
			Entry: journal.JournalEntry{ID: cmd.EntryID, Status: journal.EntryPosted},
		}, nil
	}
	app.getEntryFn = func(_ context.Context, query publicapplication.GetEntryQuery) (journal.JournalEntry, error) {
		getQuery = query
		return journal.JournalEntry{}, errors.New("not found")
	}

	result, err := NewStandardPoster(app, nil).PostStandardEntry(ctx, req)
	if err != nil {
		t.Fatalf("PostStandardEntry: %v", err)
	}
	if !result.Entry.Status.IsValid() || result.Entry.Status != journal.EntryPosted || result.IdempotentReplay {
		t.Fatalf("unexpected result: %+v", result)
	}
	if draftCmd.SourceModule != "ifrs15" || draftCmd.Description != "[IFRS-15-REV-REC] Revenue recognition" {
		t.Fatalf("unexpected draft metadata: %+v", draftCmd)
	}
	if draftCmd.TransactionCurrency != "USD" || draftCmd.ExchangeRate != "1" ||
		draftCmd.Classification != journal.EntryClassRegular || draftCmd.ReferenceID != "contract-1" {
		t.Fatalf("unexpected draft fields: %+v", draftCmd)
	}
	if !draftCmd.PostingDate.Equal(req.EffectiveDate) || !draftCmd.EffectiveDate.Equal(req.EffectiveDate) {
		t.Fatalf("unexpected draft dates: %+v", draftCmd)
	}
	if draftCmd.Context.CorrelationID != "corr-1" || draftCmd.Context.IdempotencyKey != "key-1" ||
		draftCmd.Context.SourceSystem != "ifrs-15-rev-rec" || draftCmd.Context.Principal.ActorID != "tester" {
		t.Fatalf("unexpected command context: %+v", draftCmd.Context)
	}
	if getQuery.EntryID == (types.JournalEntryID{}) || postCmd.EntryID != draftID {
		t.Fatalf("unexpected lookup/post IDs: get=%s post=%s", getQuery.EntryID, postCmd.EntryID)
	}

	// Same request must derive the same deterministic lookup ID.
	secondQuery := types.JournalEntryID{}
	app.getEntryFn = func(_ context.Context, query publicapplication.GetEntryQuery) (journal.JournalEntry, error) {
		secondQuery = query.EntryID
		return journal.JournalEntry{}, errors.New("not found")
	}
	if _, err := NewStandardPoster(app, nil).PostStandardEntry(ctx, req); err != nil {
		t.Fatalf("second post: %v", err)
	}
	if secondQuery != getQuery.EntryID {
		t.Fatalf("deterministic ID changed: %s vs %s", secondQuery, getQuery.EntryID)
	}
}

func TestCoverageStandardPostingDeterministicCreator(t *testing.T) {
	req := coveragePostingRequest(t)
	ctx := coveragePostingContext(req.CompanyID)
	expectedID := types.JournalEntryID(uuid.NewSHA1(uuid.NameSpaceOID,
		fmt.Appendf(nil, "%s:%s:%s", req.CompanyID, req.StandardRef, req.IdempotencyKey)))

	app := &coverageIDPostingApp{coveragePostingApp: *coverageNotFindingApp()}
	var passedID types.JournalEntryID
	app.createDraftWithIDFn = func(_ context.Context, _ publicapplication.CreateDraftCommand, id types.JournalEntryID) (publicapplication.EntryResult, error) {
		passedID = id
		return publicapplication.EntryResult{Entry: journal.JournalEntry{ID: id, Status: journal.EntryDraft}}, nil
	}
	app.postFn = func(_ context.Context, cmd publicapplication.PostEntryCommand) (publicapplication.EntryResult, error) {
		return publicapplication.EntryResult{Entry: journal.JournalEntry{ID: cmd.EntryID, Status: journal.EntryPosted}}, nil
	}
	if _, err := NewStandardPoster(app, nil).PostStandardEntry(ctx, req); err != nil {
		t.Fatalf("post with deterministic creator: %v", err)
	}
	if passedID != expectedID {
		t.Fatalf("derived ID = %s, want %s", passedID, expectedID)
	}

	explicitID := types.NewJournalEntryID()
	req.EntryID = explicitID
	if _, err := NewStandardPoster(app, nil).PostStandardEntry(ctx, req); err != nil {
		t.Fatalf("post with explicit ID: %v", err)
	}
	if passedID != explicitID {
		t.Fatalf("explicit ID not honoured: %s", passedID)
	}

	explicitPostingDate := time.Date(2026, 4, 2, 8, 0, 0, 0, time.UTC)
	req.Description = "IFRS-15-REV-REC already present"
	req.PostingDate = explicitPostingDate
	req.Classification = journal.EntryClassAdjusting
	req.TargetPeriodNumber = 14
	req.AdjustingEntryType = journal.AdjustingEntryAccrual
	req.AdjustmentReason = "audit reclass"
	req.AdjustmentApprovalReference = "approval-9"
	app.createDraftWithIDFn = func(_ context.Context, cmd publicapplication.CreateDraftCommand, _ types.JournalEntryID) (publicapplication.EntryResult, error) {
		if cmd.Description != req.Description || !cmd.PostingDate.Equal(explicitPostingDate) ||
			cmd.Classification != journal.EntryClassAdjusting || cmd.TargetPeriodNumber != 14 ||
			cmd.AdjustingEntryType != journal.AdjustingEntryAccrual || cmd.AdjustmentReason != "audit reclass" ||
			cmd.AdjustmentApprovalReference != "approval-9" {
			t.Fatalf("adjustment metadata was not propagated: %+v", cmd)
		}
		return publicapplication.EntryResult{Entry: journal.JournalEntry{ID: explicitID, Status: journal.EntryPosted}}, nil
	}
	// A draft returned as already posted short-circuits the Post call.
	app.postFn = nil
	if _, err := NewStandardPoster(app, nil).PostStandardEntry(ctx, req); err != nil {
		t.Fatalf("already-posted draft result: %v", err)
	}

	req.Description = ""
	app.createDraftWithIDFn = func(_ context.Context, cmd publicapplication.CreateDraftCommand, _ types.JournalEntryID) (publicapplication.EntryResult, error) {
		if cmd.Description != "[IFRS-15-REV-REC] contract-1" {
			t.Fatalf("unexpected generated description: %q", cmd.Description)
		}
		return publicapplication.EntryResult{Entry: journal.JournalEntry{ID: explicitID, Status: journal.EntryDraft}}, nil
	}
	app.postFn = func(_ context.Context, cmd publicapplication.PostEntryCommand) (publicapplication.EntryResult, error) {
		return publicapplication.EntryResult{Entry: journal.JournalEntry{ID: cmd.EntryID, Status: journal.EntryPosted}}, nil
	}
	if _, err := NewStandardPoster(app, nil).PostStandardEntry(ctx, req); err != nil {
		t.Fatalf("generated description path: %v", err)
	}
}

func TestCoverageStandardPostingIdempotencyAndErrors(t *testing.T) {
	req := coveragePostingRequest(t)
	ctx := coveragePostingContext(req.CompanyID)
	existingID := types.JournalEntryID(uuid.NewSHA1(uuid.NameSpaceOID,
		fmt.Appendf(nil, "%s:%s:%s", req.CompanyID, req.StandardRef, req.IdempotencyKey)))

	postedApp := coverageNotFindingApp()
	postedApp.getEntryFn = func(context.Context, publicapplication.GetEntryQuery) (journal.JournalEntry, error) {
		return journal.JournalEntry{ID: existingID, Status: journal.EntryPosted}, nil
	}
	result, err := NewStandardPoster(postedApp, nil).PostStandardEntry(ctx, req)
	if err != nil || !result.IdempotentReplay || result.Entry.ID != existingID {
		t.Fatalf("idempotent replay = %+v, %v", result, err)
	}

	draftApp := coverageNotFindingApp()
	var postedID types.JournalEntryID
	draftApp.getEntryFn = func(context.Context, publicapplication.GetEntryQuery) (journal.JournalEntry, error) {
		return journal.JournalEntry{ID: existingID, Status: journal.EntryDraft}, nil
	}
	draftApp.postFn = func(_ context.Context, cmd publicapplication.PostEntryCommand) (publicapplication.EntryResult, error) {
		postedID = cmd.EntryID
		return publicapplication.EntryResult{Entry: journal.JournalEntry{ID: cmd.EntryID, Status: journal.EntryPosted}}, nil
	}
	if _, err := NewStandardPoster(draftApp, nil).PostStandardEntry(ctx, req); err != nil {
		t.Fatalf("complete pending posting: %v", err)
	}
	if postedID != existingID {
		t.Fatalf("pending draft was not posted: %s", postedID)
	}

	wrongIDApp := coverageNotFindingApp()
	wrongIDApp.getEntryFn = func(context.Context, publicapplication.GetEntryQuery) (journal.JournalEntry, error) {
		return journal.JournalEntry{ID: types.NewJournalEntryID(), Status: journal.EntryPosted}, nil
	}
	wrongIDApp.createDraftFn = func(context.Context, publicapplication.CreateDraftCommand) (publicapplication.EntryResult, error) {
		return publicapplication.EntryResult{Entry: journal.JournalEntry{ID: types.NewJournalEntryID(), Status: journal.EntryDraft}}, nil
	}
	wrongIDApp.postFn = func(_ context.Context, cmd publicapplication.PostEntryCommand) (publicapplication.EntryResult, error) {
		return publicapplication.EntryResult{Entry: journal.JournalEntry{ID: cmd.EntryID, Status: journal.EntryPosted}}, nil
	}
	if _, err := NewStandardPoster(wrongIDApp, nil).PostStandardEntry(ctx, req); err != nil {
		t.Fatalf("unrelated existing entry must not replay: %v", err)
	}

	createErr := errors.New("create failed")
	failingCreate := coverageNotFindingApp()
	failingCreate.createDraftFn = func(context.Context, publicapplication.CreateDraftCommand) (publicapplication.EntryResult, error) {
		return publicapplication.EntryResult{}, createErr
	}
	if _, err := NewStandardPoster(failingCreate, nil).PostStandardEntry(ctx, req); !errors.Is(err, createErr) ||
		!strings.Contains(err.Error(), "create draft") {
		t.Fatalf("create draft error = %v", err)
	}

	postErr := errors.New("post failed")
	failingPost := coverageNotFindingApp()
	failingPost.createDraftFn = func(context.Context, publicapplication.CreateDraftCommand) (publicapplication.EntryResult, error) {
		return publicapplication.EntryResult{Entry: journal.JournalEntry{ID: types.NewJournalEntryID(), Status: journal.EntryDraft}}, nil
	}
	failingPost.postFn = func(context.Context, publicapplication.PostEntryCommand) (publicapplication.EntryResult, error) {
		return publicapplication.EntryResult{}, postErr
	}
	if _, err := NewStandardPoster(failingPost, nil).PostStandardEntry(ctx, req); !errors.Is(err, postErr) ||
		!strings.Contains(err.Error(), "post entry") {
		t.Fatalf("post entry error = %v", err)
	}
}

func TestCoverageDeriveSourceModule(t *testing.T) {
	cases := map[string]string{
		" IFRS-15-REV ":    "ifrs15",
		"IAS-12-TAX":       "ias12",
		"custom-extension": "custom",
		"lonely":           "lonely",
		"ifrs":             "ifrs",
	}
	for input, want := range cases {
		if got := deriveSourceModule(input); got != want {
			t.Fatalf("deriveSourceModule(%q) = %q, want %q", input, got, want)
		}
	}
}
