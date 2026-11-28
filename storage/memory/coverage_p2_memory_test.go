package memory

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	publicgovernance "github.com/osmai4603-collab/accounting_engine/pkg/api/v1/governance"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/journal"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/ledger"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	publicevents "github.com/osmai4603-collab/accounting_engine/pkg/spi/events"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/masterdata"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/storage"
	"github.com/shopspring/decimal"
)

func p2MemoryDriver(t *testing.T) *Driver {
	t.Helper()
	d := newDriver()
	t.Cleanup(func() { _ = d.Close() })
	return d
}

func p2MemoryEntry(companyID types.CompanyID, journalID types.JournalID, accountID types.AccountID, id types.JournalEntryID) journal.JournalEntry {
	amount, _ := types.NewMoney(decimal.NewFromInt(42), "USD")
	zero, _ := types.Zero("USD")
	return journal.JournalEntry{
		ID:             id,
		CompanyID:      companyID,
		JournalID:      journalID,
		EffectiveDate:  time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
		PostingDate:    time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
		Status:         journal.EntryDraft,
		Classification: journal.EntryClassRegular,
		Lines: []journal.EntryLine{{
			ID:                  types.NewJournalEntryLineID(),
			AccountID:           accountID,
			TransactionAmount:   amount,
			TransactionCurrency: "USD",
			ExchangeRate:        decimal.NewFromInt(1),
			FunctionalDebit:     amount,
			FunctionalCredit:    zero,
		}},
	}
}

// Exercise every UnitOfWork accessor, the nested transaction reuse path and
// the read-only / rollback / panic branches of ExecuteWithOptions.
func TestCoverageP2MemoryUnitOfWorkPaths(t *testing.T) {
	ctx := context.Background()
	d := p2MemoryDriver(t)
	companyID := types.NewCompanyID()
	journalID := types.NewJournalID()
	accountID := types.NewAccountID()
	yearID := types.NewFiscalYearID()
	periodID := types.NewFiscalPeriodID()
	ctx = storage.WithCompanyContext(ctx, companyID)

	if err := d.CreateCompany(ctx, masterdata.Company{
		ID: companyID, Code: "P2", LegalName: "Coverage Co",
		FunctionalCurrency: "USD", PresentationCurrency: "USD", Active: true,
	}); err != nil {
		t.Fatalf("create company: %v", err)
	}
	if err := d.CreateJournal(ctx, masterdata.Journal{ID: journalID, CompanyID: companyID, Code: "GEN", Name: "General"}); err != nil {
		t.Fatalf("create journal: %v", err)
	}
	if err := d.CreateAccount(ctx, masterdata.Account{ID: accountID, CompanyID: companyID, Code: "1000", Name: "Cash", Type: masterdata.AccountTypeAsset, NormalBalance: masterdata.NormalBalanceDebit, Currency: "USD", IsActive: true}); err != nil {
		t.Fatalf("create account: %v", err)
	}
	if err := d.CreateFiscalYear(ctx, masterdata.FiscalYear{ID: yearID, CompanyID: companyID, Name: "FY26", StartDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), EndDate: time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC), State: masterdata.FiscalYearOpen}); err != nil {
		t.Fatalf("create fiscal year: err")
	}
	if err := d.CreateFiscalPeriod(ctx, masterdata.FiscalPeriod{ID: periodID, CompanyID: companyID, FiscalYearID: yearID, Number: 1, Name: "Jan", StartDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), EndDate: time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC), State: masterdata.PeriodOpen, Type: masterdata.FiscalPeriodTypeNormal}); err != nil {
		t.Fatalf("create fiscal period: err")
	}

	entryID := types.NewJournalEntryID()
	if err := d.Execute(ctx, func(txCtx context.Context, uow storage.UnitOfWork) error {
		if uow.Drafts() == nil || uow.Entries() == nil || uow.Posting() == nil ||
			uow.Sequences() == nil || uow.Audit() == nil || uow.Idempotency() == nil || uow.Balance() == nil {
			return errors.New("missing uow port")
		}
		entry := p2MemoryEntry(companyID, journalID, accountID, entryID)
		if err := uow.Drafts().SaveDraft(txCtx, entry); err != nil {
			return err
		}
		if _, err := uow.Entries().GetEntry(txCtx, companyID, entryID); err != nil {
			return err
		}
		if _, err := uow.Sequences().NextSequence(txCtx, companyID, journalID); err != nil {
			return err
		}
		if err := uow.Audit().AppendAudit(txCtx, storage.AuditRecord{CompanyID: companyID, EntityType: "test", EntityID: entryID.String(), Action: "draft"}); err != nil {
			return err
		}
		if _, _, err := uow.Idempotency().Claim(txCtx, storage.IdempotencyRecord{CompanyID: companyID, Key: "p2-uow", Operation: "post", PayloadHash: "h"}); err != nil {
			return err
		}
		if _, err := uow.Balance().GetAccountBalance(txCtx, companyID, accountID, time.Time{}, "USD"); err != nil {
			return err
		}
		// A nested unit of work reuses the already active transaction scope.
		return d.Execute(txCtx, func(nestedCtx context.Context, nested storage.UnitOfWork) error {
			entry.Status = journal.EntryDraft
			return nested.Drafts().SaveDraft(nestedCtx, entry)
		})
	}); err != nil {
		t.Fatalf("execute unit of work: %v", err)
	}
	if _, err := d.GetEntry(ctx, companyID, entryID); err != nil {
		t.Fatalf("committed entry missing: %v", err)
	}

	// Explicit error rolls the whole snapshot back.
	rolledBackID := types.NewJournalEntryID()
	err := d.Execute(ctx, func(txCtx context.Context, uow storage.UnitOfWork) error {
		if err := uow.Drafts().SaveDraft(txCtx, p2MemoryEntry(companyID, journalID, accountID, rolledBackID)); err != nil {
			return err
		}
		return errors.New("rollback requested")
	})
	if err == nil {
		t.Fatal("expected rollback error")
	}
	if _, err := d.GetEntry(ctx, companyID, rolledBackID); err == nil {
		t.Fatal("rolled back entry is visible")
	}

	// A panic inside the callback is propagated and discards the snapshot.
	panickedID := types.NewJournalEntryID()
	func() {
		defer func() {
			if recover() == nil {
				t.Error("expected panic to propagate")
			}
		}()
		_ = d.Execute(ctx, func(txCtx context.Context, uow storage.UnitOfWork) error {
			_ = uow.Drafts().SaveDraft(txCtx, p2MemoryEntry(companyID, journalID, accountID, panickedID))
			panic("p2 boom")
		})
	}()
	if _, err := d.GetEntry(ctx, companyID, panickedID); err == nil {
		t.Fatal("panicked entry is visible")
	}

	// A read-only unit of work rejects every write path.
	readOnlyID := types.NewJournalEntryID()
	if err := d.ExecuteWithOptions(ctx, storage.TxOptions{ReadOnly: true}, func(txCtx context.Context, uow storage.UnitOfWork) error {
		if err := uow.Drafts().SaveDraft(txCtx, p2MemoryEntry(companyID, journalID, accountID, readOnlyID)); !errors.Is(err, ErrReadOnlyTransaction) {
			t.Errorf("read-only SaveDraft error = %v", err)
		}
		if err := uow.Audit().AppendAudit(txCtx, storage.AuditRecord{CompanyID: companyID}); !errors.Is(err, ErrReadOnlyTransaction) {
			t.Errorf("read-only AppendAudit error = %v", err)
		}
		if _, _, err := uow.Idempotency().Claim(txCtx, storage.IdempotencyRecord{CompanyID: companyID, Key: "ro", Operation: "post"}); !errors.Is(err, ErrReadOnlyTransaction) {
			t.Errorf("read-only Claim error = %v", err)
		}
		return nil
	}); err != nil {
		t.Fatalf("read-only transaction: %v", err)
	}
	if _, err := d.GetEntry(ctx, companyID, readOnlyID); err == nil {
		t.Fatal("read-only snapshot wrote an entry")
	}

	// Invalid options, cancelled contexts, nil callbacks and the thin wrappers.
	if err := d.ExecuteWithOptions(ctx, storage.TxOptions{Isolation: "bogus"}, func(context.Context, storage.UnitOfWork) error { return nil }); err == nil {
		t.Fatal("expected invalid isolation error")
	}
	if err := d.Execute(ctx, nil); err == nil {
		t.Fatal("expected nil callback error")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := d.Execute(cancelled, func(context.Context, storage.UnitOfWork) error { return nil }); err == nil {
		t.Fatal("expected cancelled context error")
	}
	if err := d.WithinTransaction(ctx, func(txCtx context.Context) error {
		if txCtx == nil {
			return errors.New("nil transaction context")
		}
		return nil
	}); err != nil {
		t.Fatalf("WithinTransaction: %v", err)
	}
	if err := d.WithinTransactionWithOptions(ctx, storage.TxOptions{Isolation: storage.IsolationSerializable}, func(context.Context) error { return nil }); err != nil {
		t.Fatalf("WithinTransactionWithOptions: %v", err)
	}
	if transactionDriver(nil, d) != d {
		t.Fatal("transactionDriver with nil context must return root driver")
	}
}

// Call every transaction-aware port method outside a transaction context so
// each delegate's direct-dispatch branch is executed as well.
func TestCoverageP2MemoryPortDelegatesOutsideTransaction(t *testing.T) {
	ctx := context.Background()
	adapter := New()
	t.Cleanup(func() { _ = adapter.Close() })
	ports := adapter.Ports()
	d := adapter.Driver()
	companyID := types.NewCompanyID()
	journalID := types.NewJournalID()
	accountID := types.NewAccountID()
	yearID := types.NewFiscalYearID()
	periodID := types.NewFiscalPeriodID()
	entryID := types.NewJournalEntryID()
	ctx = storage.WithCompanyContext(ctx, companyID)

	company := masterdata.Company{ID: companyID, Code: "P2P", LegalName: "Ports", FunctionalCurrency: "USD", PresentationCurrency: "USD", Active: true}
	journal := masterdata.Journal{ID: journalID, CompanyID: companyID, Code: "GEN", Name: "General"}
	account := masterdata.Account{ID: accountID, CompanyID: companyID, Code: "1000", Name: "Cash", Type: masterdata.AccountTypeAsset, NormalBalance: masterdata.NormalBalanceDebit, Currency: "USD", IsActive: true}
	year := masterdata.FiscalYear{ID: yearID, CompanyID: companyID, Name: "FY26", StartDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), EndDate: time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC), State: masterdata.FiscalYearOpen}
	period := masterdata.FiscalPeriod{ID: periodID, CompanyID: companyID, FiscalYearID: yearID, Number: 1, Name: "Jan", StartDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), EndDate: time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC), State: masterdata.PeriodOpen, Type: masterdata.FiscalPeriodTypeNormal}
	entry := p2MemoryEntry(companyID, journalID, accountID, entryID)

	txDriver := transactionAwareDriver{driver: d}
	_ = ports.Drafts.SaveDraft(ctx, entry)
	_ = txDriver.CancelDraft(ctx, entry)
	_, _ = ports.Entries.GetEntry(ctx, companyID, entryID)
	_, _ = ports.Posting.CommitPosting(ctx, storage.PostingCommit{Entry: entry, Operation: storage.OperationPost})
	_, _ = ports.Sequences.NextSequence(ctx, companyID, journalID)
	_ = txDriver.CreateCompany(ctx, company)
	_ = txDriver.CreateJournal(ctx, journal)
	_ = txDriver.CreateAccount(ctx, account)
	_ = txDriver.CreateFiscalYear(ctx, year)
	_ = txDriver.CreateFiscalPeriod(ctx, period)
	_, _ = ports.MasterData.GetCompany(ctx, companyID)
	_, _ = txDriver.GetAccount(ctx, companyID, accountID)
	_, _ = txDriver.GetAccountByCode(ctx, companyID, "1000")
	_, _ = txDriver.GetJournalByCode(ctx, companyID, "GEN")
	_, _ = txDriver.GetJournalByID(ctx, companyID, journalID)
	_, _ = txDriver.HasAccountChildren(ctx, companyID, accountID)
	_, _ = txDriver.GetFiscalYear(ctx, companyID, yearID)
	_, _ = txDriver.GetFiscalPeriod(ctx, companyID, periodID)
	_, _ = txDriver.GetPeriodForDate(ctx, companyID, entry.EffectiveDate)
	_, _ = txDriver.GetPeriodByNumber(ctx, companyID, yearID, 1)
	_, _ = txDriver.ListAccounts(ctx, companyID)
	_, _ = txDriver.ListFiscalYears(ctx, companyID)
	_, _ = txDriver.ListFiscalPeriods(ctx, companyID, yearID)
	_, _ = txDriver.ListEntriesByDateRange(ctx, companyID, time.Time{}, time.Time{})
	_ = txDriver.UpdateFiscalPeriodState(ctx, companyID, periodID, masterdata.PeriodSoftClosed, "actor", "p2")
	_ = txDriver.UpdateFiscalYearState(ctx, companyID, yearID, masterdata.FiscalYearSoftClosed, "actor", "p2")
	_, _ = txDriver.GetAccountAsOf(ctx, companyID, accountID, entry.EffectiveDate)
	_ = txDriver.SaveAccountVersion(ctx, account)
	centerID := types.NewCostCenterID()
	center := masterdata.CostCenter{ID: centerID, CompanyID: companyID, Code: "CC1", Name: "Cost Center", Active: true}
	_ = txDriver.CreateCostCenter(ctx, center)
	_, _ = txDriver.GetCostCenterForCompany(ctx, companyID, centerID)
	dimensionID := types.NewProjectID()
	dimension := masterdata.AnalyticalDimensionValue{ID: dimensionID, CompanyID: companyID, Kind: masterdata.DimensionProject, Code: "PRJ", Name: "Project", Active: true}
	_ = txDriver.CreateAnalyticalDimensionValue(ctx, dimension)
	_, _ = txDriver.GetAnalyticalDimensionForCompany(ctx, companyID, masterdata.DimensionProject, dimensionID.String())
	policyID := uuid.New()
	version := storage.PolicyVersion{CompanyID: companyID, PolicyID: policyID, Kind: storage.PolicyKindPosting, Version: 1, EffectiveFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), CreatedAt: time.Now().UTC(), Payload: []byte(`{"max_backdating_days":1}`)}
	_ = txDriver.AppendVersion(ctx, version)
	_, _ = txDriver.ListVersions(ctx, companyID, storage.PolicyKindPosting)
	_, _ = ports.Balance.GetAccountBalance(ctx, companyID, accountID, time.Time{}, "USD")
	_, _ = txDriver.GetTransactionBalances(ctx, ledger.ForeignCurrencyBalanceFilter{CompanyID: companyID})
	_ = ports.Audit.AppendAudit(ctx, storage.AuditRecord{CompanyID: companyID, EntityType: "test", EntityID: entryID.String(), Action: "p2"})
	_, _, _ = ports.Idempotency.Claim(ctx, storage.IdempotencyRecord{CompanyID: companyID, Key: "p2-claim", Operation: "post", PayloadHash: "hash"})
	lookup, ok := ports.Idempotency.(storage.IdempotencyLookup)
	if ok {
		_, _, _ = lookup.Lookup(ctx, companyID, "p2-claim")
	}
	_ = ports.Idempotency.Complete(ctx, companyID, "p2-claim", "hash", entryID, []byte("ok"))
	_ = ports.Idempotency.Fail(ctx, companyID, "p2-claim", "hash", storage.IdempotencyRetryable)
	record := storage.YearEndCloseRecord{CompanyID: companyID, FiscalYearID: yearID, ClosedBy: "actor", ClosedAt: time.Now().UTC()}
	_ = txDriver.SaveYearEndCloseRecord(ctx, record)
	_, _, _ = txDriver.GetYearEndCloseRecord(ctx, companyID, yearID)
	events := ports.Events.Idempotency
	_, _, _ = events.GetPublicEventIngestion(ctx, uuid.UUID(companyID), "evt")
	scoped, ok := events.(publicevents.ScopedIdempotencyStore)
	if ok {
		_, _, _ = scoped.GetPublicEventIngestionForSource(ctx, uuid.UUID(companyID), "src", "evt")
	}
	_, _, _ = events.ClaimPublicEventIngestion(ctx, publicevents.Ingestion{CompanyID: uuid.UUID(companyID), SourceSystem: "src", IdempotencyKey: "evt", EventType: "test", PayloadHash: "h"})
	_ = events.CompletePublicEventIngestion(ctx, uuid.UUID(companyID), "evt", entry)
	if ok {
		_ = scoped.CompletePublicEventIngestionForSource(ctx, uuid.UUID(companyID), "src", "evt", "h", entry)
	}
	_ = events.FailPublicEventIngestion(ctx, uuid.UUID(companyID), "evt", "failed")
	if ok {
		_ = scoped.FailPublicEventIngestionForSource(ctx, uuid.UUID(companyID), "src", "evt", publicevents.FailureClassTerminal, "failed")
	}
	mapping := publicevents.AccountMapping{CompanyID: uuid.UUID(companyID), EventType: "test", LedgerID: "ledger", DebitAccountCode: "1000", CreditAccountCode: "2000"}
	_ = ports.Events.Mappings.SavePublicEventAccountMapping(ctx, mapping)
	_, _, _ = ports.Events.Mappings.GetPublicEventAccountMapping(ctx, uuid.UUID(companyID), "test", "ledger")
	if d == nil {
		t.Fatal("adapter driver is nil")
	}
}

// Exercise clone/copyFrom with every state map populated, plus the global
// closing/ledger-owner map helpers.
func TestCoverageP2MemoryCloneAndCopyState(t *testing.T) {
	src := p2MemoryDriver(t)
	dst := p2MemoryDriver(t)
	companyID := types.NewCompanyID()
	accountID := types.NewAccountID()
	journalID := types.NewJournalID()
	yearID := types.NewFiscalYearID()
	periodID := types.NewFiscalPeriodID()
	entryID := types.NewJournalEntryID()
	centerID := types.NewCostCenterID()
	dimensionID := types.NewProjectID()
	policyID := uuid.New()
	reopened := time.Now().UTC().Truncate(time.Second)

	parentID := types.NewAccountID()
	monetary := true
	effectiveTo := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	src.companies[companyID] = masterdata.Company{ID: companyID, Code: "CLN", FunctionalCurrency: "USD"}
	src.accounts[companyID] = map[types.AccountID]masterdata.Account{
		accountID: {ID: accountID, CompanyID: companyID, Code: "1000", Name: "Cash", ParentID: &parentID, EffectiveTo: &effectiveTo, IsMonetary: &monetary},
	}
	src.accountCodes[companyID] = map[string]types.AccountID{"1000": accountID}
	src.journals[companyID] = map[string]masterdata.Journal{"GEN": {ID: journalID, CompanyID: companyID, Code: "GEN"}}
	src.years[companyID] = map[types.FiscalYearID]masterdata.FiscalYear{yearID: {ID: yearID, CompanyID: companyID}}
	src.periods[companyID] = map[types.FiscalPeriodID]masterdata.FiscalPeriod{periodID: {ID: periodID, CompanyID: companyID}}
	src.primaryCompanyID = companyID
	versionKey := accountVersionKey{companyID: companyID, accountID: accountID}
	src.accountVersions[versionKey] = []masterdata.Account{{ID: accountID, ParentID: &parentID, EffectiveTo: &effectiveTo, IsMonetary: &monetary}}
	src.costCenters[companyID] = map[types.CostCenterID]masterdata.CostCenter{centerID: {ID: centerID, ParentID: &centerID}}
	dimKey := dimensionKey{kind: types.AnalyticalDimensionKind(masterdata.DimensionProject), id: dimensionID.String()}
	src.dimensionValues[companyID] = map[dimensionKey]masterdata.AnalyticalDimensionValue{dimKey: {ID: dimensionID}}
	policyKey := phase11PolicyVersionKey{companyID: companyID, policyID: policyID, kind: storage.PolicyKindPosting, version: 1}
	src.policyVersions[policyKey] = storage.PolicyVersion{EffectiveTo: &effectiveTo, Payload: []byte(`{"a":1}`)}
	src.journalSequences["seq"] = 7
	src.entries[companyID] = map[types.JournalEntryID]journal.JournalEntry{entryID: p2MemoryEntry(companyID, journalID, accountID, entryID)}
	src.balances[accountBalanceKey{companyID: companyID, accountID: accountID, currency: "USD"}] = decimal.NewFromInt(12)
	src.auditRecords = []storage.AuditRecord{{CompanyID: companyID, EntityID: entryID.String()}}
	src.auditLastHash = "hash"
	src.governanceAuditLogs[companyID] = []publicgovernance.AuditLog{{ID: 1, SequenceNumber: 1, Payload: []byte(`{}`)}}
	src.governanceCheckpoints[companyID] = publicgovernance.AuditCheckpoint{CompanyID: companyID, SequenceNumber: 1}
	idKey := idempotencyKey{companyID: companyID, key: "k", scope: "s"}
	src.idempotency[idKey] = storage.IdempotencyRecord{CompanyID: companyID, Key: "k"}
	src.eventIngestions["evt"] = publicevents.Ingestion{IdempotencyKey: "evt"}
	src.eventMappings["map"] = publicevents.AccountMapping{EventType: "test"}

	closings := getClosingMap(src, true)
	closings[companyID] = map[types.FiscalYearID]storage.YearEndCloseRecord{yearID: {
		CompanyID: companyID, FiscalYearID: yearID, ClosingEntryIDs: []types.JournalEntryID{entryID}, ReopenedAt: &reopened,
	}}
	src.setEntryLedger(context.Background(), entryID)
	customLedgerCtx := storage.WithLedgerContext(context.Background(), "STATUTORY")
	src.setEntryLedger(customLedgerCtx, types.NewJournalEntryID())

	cloned := src.clone()
	if cloned.primaryCompanyID != companyID || cloned.auditLastHash != "hash" {
		t.Fatal("clone lost scalar state")
	}
	if cloned.journalSequences["seq"] != 7 {
		t.Fatal("clone lost journal sequence")
	}
	if got := cloned.governanceAuditLogs[companyID][0].Payload; string(got) != "{}" {
		t.Fatalf("clone audit payload = %s", got)
	}
	if got := getClosingMap(cloned, false)[companyID][yearID].ClosingEntryIDs; len(got) != 1 || got[0] != entryID {
		t.Fatal("clone lost year end closing")
	}
	if !cloned.entryVisibleInLedger(customLedgerCtx, types.JournalEntryID{}) == false {
		t.Fatal("unexpected ledger visibility")
	}

	dst.copyFrom(cloned)
	if dst.primaryCompanyID != companyID || dst.journalSequences["seq"] != 7 {
		t.Fatal("copyFrom did not install cloned state")
	}
	if got := getClosingMap(dst, false)[companyID][yearID].ReopenedAt; got == nil || !got.Equal(reopened) {
		t.Fatal("copyFrom lost closing record")
	}
	if len(dst.entries[companyID]) != 1 {
		t.Fatal("copyFrom lost entries")
	}

	// Both branches of the global closing-map and ledger-owner copy helpers.
	empty := p2MemoryDriver(t)
	copyYearEndClosings(empty, dst)
	if getClosingMap(dst, false) != nil {
		t.Fatal("copyYearEndClosings should remove destination map when source is empty")
	}
	emptyID := types.NewJournalEntryID()
	empty.setEntryLedger(customLedgerCtx, emptyID)
	copyMultiLedgerOwners(empty, dst)
	if !dst.entryVisibleInLedger(customLedgerCtx, emptyID) {
		t.Fatal("copyMultiLedgerOwners did not transfer ownership")
	}
	cloneMultiLedgerOwners(dst, empty)
	if !empty.entryVisibleInLedger(customLedgerCtx, emptyID) {
		t.Fatal("cloneMultiLedgerOwners lost ownership")
	}
}

func TestCoverageP2MemoryPhase11Helpers(t *testing.T) {
	end := phase11EndOfUTCDay(time.Date(2026, 3, 4, 13, 20, 0, 0, time.FixedZone("x", 3600)))
	want := time.Date(2026, 3, 4, 23, 59, 59, 999999999, time.UTC)
	if !end.Equal(want) {
		t.Fatalf("phase11EndOfUTCDay = %v, want %v", end, want)
	}
	if _, err := normalizeAccountVersion(masterdata.Account{}); err == nil {
		t.Fatal("expected invalid effective range")
	}
	if _, err := normalizeAccountVersion(masterdata.Account{
		EffectiveFrom: time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC),
		EffectiveTo:   timePtr(time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)),
	}); err == nil {
		t.Fatal("expected reversed effective range error")
	}
	v, err := normalizeAccountVersion(masterdata.Account{
		Code:          "1000",
		EffectiveFrom: time.Date(2026, 5, 2, 13, 0, 0, 0, time.UTC),
		EffectiveTo:   timePtr(time.Date(2026, 5, 4, 1, 0, 0, 0, time.UTC)),
	})
	if err != nil {
		t.Fatalf("normalizeAccountVersion: %v", err)
	}
	if v.EffectiveTo == nil || !v.EffectiveTo.Equal(phase11EndOfUTCDay(time.Date(2026, 5, 4, 0, 0, 0, 0, time.UTC))) {
		t.Fatalf("normalized EffectiveTo = %v", v.EffectiveTo)
	}

	first := masterdata.Account{EffectiveFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), EffectiveTo: timePtr(time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC))}
	disjoint := masterdata.Account{EffectiveFrom: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), EffectiveTo: timePtr(time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC))}
	overlapping := masterdata.Account{EffectiveFrom: time.Date(2026, 1, 20, 0, 0, 0, 0, time.UTC), EffectiveTo: timePtr(time.Date(2026, 2, 20, 0, 0, 0, 0, time.UTC))}
	if accountWindowsOverlap(first, disjoint) || !accountWindowsOverlap(first, overlapping) || accountWindowsOverlap(disjoint, first) {
		t.Fatal("accountWindowsOverlap returned an unexpected result")
	}

	if got := activeLedgerID(nil); got != storage.DefaultLedgerID {
		t.Fatalf("activeLedgerID(nil) = %q", got)
	}
	if got := activeLedgerID(storage.WithLedgerContext(context.Background(), "  STATUTORY  ")); got != "STATUTORY" {
		t.Fatalf("activeLedgerID = %q", got)
	}
	if got := ledgerScopedIdempotencyKey(context.Background(), "key"); got != "key" {
		t.Fatalf("default ledger key = %q", got)
	}
	if got := ledgerScopedIdempotencyKey(storage.WithLedgerContext(context.Background(), "OTHER"), "key"); got != "ledger:OTHER:key" {
		t.Fatalf("scoped idempotency key = %q", got)
	}

	d := p2MemoryDriver(t)
	target := storage.LockTarget{Kind: storage.LockKindEntry, CompanyID: types.NewCompanyID(), ID: "x"}
	release, err := d.acquireNamedLock(context.Background(), target)
	if err != nil {
		t.Fatalf("acquireNamedLock: %v", err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := d.acquireNamedLock(cancelled, target); err == nil {
		t.Fatal("expected cancelled lock acquisition to fail")
	}
	release()
}

func timePtr(t time.Time) *time.Time { return &t }

// Cover retryable event-ingestion claims, error paths and posting fault
// injection around the atomic commit boundary.
func TestCoverageP2MemoryEventAndPostingFaultBranches(t *testing.T) {
	ctx := context.Background()
	d := p2MemoryDriver(t)
	companyID := types.NewCompanyID()
	journalID := types.NewJournalID()
	accountID := types.NewAccountID()
	id := uuid.UUID(companyID)

	if err := d.CreateCompany(ctx, masterdata.Company{ID: companyID, Code: "FLT", LegalName: "Faults", FunctionalCurrency: "USD", PresentationCurrency: "USD", Active: true}); err != nil {
		t.Fatalf("create company: %v", err)
	}

	first := publicevents.Ingestion{CompanyID: id, IdempotencyKey: "retry", EventType: "test", PayloadHash: "one", Status: publicevents.StatusStarted}
	if _, claimed, err := d.ClaimPublicEventIngestion(ctx, first); err != nil || !claimed {
		t.Fatalf("first claim = %v, %v", claimed, err)
	}
	if err := d.FailPublicEventIngestionForSource(ctx, id, publicevents.DefaultSourceSystem, "retry", publicevents.FailureClassTransient, "again"); err != nil {
		t.Fatalf("fail ingestion: %v", err)
	}
	second := publicevents.Ingestion{CompanyID: id, IdempotencyKey: "retry", EventType: "test", PayloadHash: "two"}
	if rec, claimed, err := d.ClaimPublicEventIngestion(ctx, second); err != nil || !claimed || rec.Status != publicevents.StatusStarted {
		t.Fatalf("reclaim = %+v, %v, %v", rec, claimed, err)
	}
	if _, claimed, err := d.ClaimPublicEventIngestion(ctx, second); err != nil || claimed {
		t.Fatalf("duplicate claim = %v, %v", claimed, err)
	}
	if err := d.CompletePublicEventIngestion(ctx, id, "missing", journal.JournalEntry{}); !errors.Is(err, publicevents.ErrRecordNotFound) {
		t.Fatalf("complete missing = %v", err)
	}
	if err := d.FailPublicEventIngestion(ctx, id, "missing", "x"); !errors.Is(err, publicevents.ErrRecordNotFound) {
		t.Fatalf("fail missing = %v", err)
	}

	entry := p2MemoryEntry(companyID, journalID, accountID, types.NewJournalEntryID())
	entry.SequenceNumber = 1
	fault := errors.New("p2 injected fault")
	for _, point := range []storage.FaultPoint{storage.FaultBeforeIdempotencyClaim, storage.FaultBeforeAudit, storage.FaultBeforeCommit} {
		testEntry := entry
		testEntry.ID = types.NewJournalEntryID()
		testEntry.SequenceNumber++
		faultCtx := storage.WithFaultInjector(ctx, func(got storage.FaultPoint) error {
			if got == point {
				return fault
			}
			return nil
		})
		_, err := d.CommitPosting(faultCtx, storage.PostingCommit{
			Entry: testEntry, Operation: storage.OperationPost, ActorID: "p2",
			Idempotency: &storage.PostingIdempotency{Key: "fault-" + string(point), Scope: p2Scope(companyID, storage.OperationPost, "fault-"+string(point)), PayloadHash: "h-" + string(point)},
		})
		if !errors.Is(err, fault) {
			t.Fatalf("fault %s: got %v", point, err)
		}
		if _, err := d.GetEntry(ctx, companyID, testEntry.ID); err == nil {
			t.Fatalf("fault %s left an entry behind", point)
		}
	}

	afterEntry := entry
	afterEntry.ID = types.NewJournalEntryID()
	key := "fault-after"
	afterCtx := storage.WithFaultInjector(ctx, func(got storage.FaultPoint) error {
		if got == storage.FaultAfterCommit {
			return fault
		}
		return nil
	})
	if _, err := d.CommitPosting(afterCtx, storage.PostingCommit{
		Entry: afterEntry, Operation: storage.OperationPost, ActorID: "p2",
		Idempotency: &storage.PostingIdempotency{Key: key, Scope: p2Scope(companyID, storage.OperationPost, key), PayloadHash: "after"},
	}); !errors.Is(err, fault) {
		t.Fatalf("after-commit fault: %v", err)
	}
	replayed, err := d.GetEntry(ctx, companyID, afterEntry.ID)
	if err != nil || replayed.ID != afterEntry.ID {
		t.Fatalf("after-commit entry missing: %v", err)
	}

	// Reversal failure branches leave the original untouched.
	reverseID := types.NewJournalEntryID()
	post, err := d.CommitPosting(ctx, storage.PostingCommit{Entry: entry, Operation: storage.OperationPost, ActorID: "p2"})
	if err != nil {
		t.Fatalf("post entry: %v", err)
	}
	for _, tc := range []struct {
		name       string
		originalID *types.JournalEntryID
	}{
		{name: "missing-original"},
		{name: "unknown-original", originalID: &reverseID},
	} {
		rev := p2MemoryEntry(companyID, journalID, accountID, types.NewJournalEntryID())
		_, err := d.CommitPosting(ctx, storage.PostingCommit{Entry: rev, Operation: storage.OperationReverse, OriginalEntryID: tc.originalID, ActorID: "p2"})
		if err == nil {
			t.Fatalf("%s: expected reversal error", tc.name)
		}
	}
	if post.Status != journal.EntryPosted {
		t.Fatalf("failed reversals modified original: %s", post.Status)
	}
}

func p2Scope(companyID types.CompanyID, operation storage.PostingOperation, key string) string {
	return "company=" + companyID.String() + "|operation=" + string(operation) + "|key=" + key
}
