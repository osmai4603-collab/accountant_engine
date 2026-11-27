package contract_parity

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	kernalposting "github.com/osmai4603-collab/accounting_engine/internal/kernal/posting"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/journal"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/osmai4603-collab/accounting_engine/pkg/sdk"
	publicapplication "github.com/osmai4603-collab/accounting_engine/pkg/spi/application"
	publicmasterdata "github.com/osmai4603-collab/accounting_engine/pkg/spi/masterdata"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/storage"
	"github.com/shopspring/decimal"
)

// g019g022Capabilities is the optional capability set introduced for G-019 and
// G-022. The tests deliberately assert against the public SPI interfaces, not
// against adapter-local method sets.
type g019g022Capabilities struct {
	versions       publicmasterdata.EffectiveAccountReader
	versionWriter  publicmasterdata.AccountVersionWriter
	costCenters    publicmasterdata.CostCenterReader
	costCenterWr   publicmasterdata.CostCenterWriter
	dimensions     publicmasterdata.AnalyticalDimensionReader
	dimensionWrite publicmasterdata.AnalyticalDimensionWriter
}

func g019g022RequireCapabilities(t *testing.T, h *StorageAdapterHarness) g019g022Capabilities {
	t.Helper()
	var caps g019g022Capabilities
	var ok bool
	if caps.versions, ok = h.MasterData.(publicmasterdata.EffectiveAccountReader); !ok {
		t.Fatalf("%s: MasterData does not implement EffectiveAccountReader", h.Name)
	}
	if caps.versionWriter, ok = h.MasterData.(publicmasterdata.AccountVersionWriter); !ok {
		t.Fatalf("%s: MasterData does not implement AccountVersionWriter", h.Name)
	}
	if caps.costCenters, ok = h.MasterData.(publicmasterdata.CostCenterReader); !ok {
		t.Fatalf("%s: MasterData does not implement CostCenterReader", h.Name)
	}
	if caps.costCenterWr, ok = h.MasterData.(publicmasterdata.CostCenterWriter); !ok {
		t.Fatalf("%s: MasterData does not implement CostCenterWriter", h.Name)
	}
	if caps.dimensions, ok = h.MasterData.(publicmasterdata.AnalyticalDimensionReader); !ok {
		t.Fatalf("%s: MasterData does not implement AnalyticalDimensionReader", h.Name)
	}
	if caps.dimensionWrite, ok = h.MasterData.(publicmasterdata.AnalyticalDimensionWriter); !ok {
		t.Fatalf("%s: MasterData does not implement AnalyticalDimensionWriter", h.Name)
	}
	return caps
}

func g019g022Date(day int) time.Time {
	return time.Date(2026, time.July, day, 0, 0, 0, 0, time.UTC)
}

func g019g022Version(base publicmasterdata.Account, name string, from time.Time, to *time.Time) publicmasterdata.Account {
	version := base
	version.Name = name
	version.EffectiveFrom = from
	version.EffectiveTo = to
	return version
}

func g019g022AssertVersion(t *testing.T, got, want publicmasterdata.Account) {
	t.Helper()
	if got.ID != want.ID || got.CompanyID != want.CompanyID || got.Code != want.Code ||
		got.Name != want.Name || got.Type != want.Type || got.NormalBalance != want.NormalBalance ||
		got.Currency != want.Currency || got.IsActive != want.IsActive ||
		got.RequiresCostCenter != want.RequiresCostCenter || got.IsCashEquivalent != want.IsCashEquivalent ||
		got.DisplayOrder != want.DisplayOrder {
		t.Fatalf("effective account = %+v, want full-field snapshot %+v", got, want)
	}
	// Adapters store UTC calendar-day boundaries; compare at that granularity.
	if got.EffectiveFrom.UTC().Format("2006-01-02") != want.EffectiveFrom.UTC().Format("2006-01-02") {
		t.Fatalf("effective from = %v, want %v", got.EffectiveFrom, want.EffectiveFrom)
	}
	if (got.EffectiveTo == nil) != (want.EffectiveTo == nil) {
		t.Fatalf("effective to = %v, want %v", got.EffectiveTo, want.EffectiveTo)
	}
	if want.EffectiveTo != nil && got.EffectiveTo.UTC().Format("2006-01-02") != want.EffectiveTo.UTC().Format("2006-01-02") {
		t.Fatalf("effective to = %v, want %v", got.EffectiveTo, want.EffectiveTo)
	}
}

// TestPhase11_G019_EffectiveAccountWindowIsDeterministic proves that the
// versioned reader returns full-field snapshots with inclusive boundaries and
// that adding a newer version cannot rewrite an older effective window.
func TestPhase11_G019_EffectiveAccountWindowIsDeterministic(t *testing.T) {
	ForEachStorageAdapter(t, "G019EffectiveAccountWindow", func(t *testing.T, h *StorageAdapterHarness, f UnifiedFixture) {
		caps := g019g022RequireCapabilities(t, h)
		ctx := context.Background()

		endJune := time.Date(2026, time.June, 30, 0, 0, 0, 0, time.UTC)
		endJuly := time.Date(2026, time.July, 31, 0, 0, 0, 0, time.UTC)
		v1 := g019g022Version(f.CashAccount, "Cash v1", time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC), &endJune)
		v2 := g019g022Version(f.CashAccount, "Cash v2", g019g022Date(1), &endJuly)
		v3 := g019g022Version(f.CashAccount, "Cash v3", time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC), nil)
		for _, version := range []publicmasterdata.Account{v1, v2, v3} {
			if err := caps.versionWriter.SaveAccountVersion(ctx, version); err != nil {
				t.Fatalf("SaveAccountVersion(%s): %v", version.Name, err)
			}
		}

		cases := []struct {
			name string
			asOf time.Time
			want publicmasterdata.Account
		}{
			{name: "middle of first window", asOf: time.Date(2026, time.March, 15, 9, 0, 0, 0, time.UTC), want: v1},
			{name: "inclusive first end", asOf: time.Date(2026, time.June, 30, 23, 0, 0, 0, time.UTC), want: v1},
			{name: "inclusive second start", asOf: g019g022Date(1), want: v2},
			{name: "inclusive second end", asOf: time.Date(2026, time.July, 31, 23, 0, 0, 0, time.UTC), want: v2},
			{name: "open-ended current", asOf: time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC), want: v3},
		}
		for _, tc := range cases {
			got, err := caps.versions.GetAccountAsOf(ctx, f.Company.ID, f.CashAccount.ID, tc.asOf)
			if err != nil {
				t.Fatalf("%s: GetAccountAsOf: %v", tc.name, err)
			}
			g019g022AssertVersion(t, got, tc.want)
		}

		// A later master-data edit (a newer version) must not reinterpret the
		// already stored past windows.
		edited := g019g022Version(f.CashAccount, "Cash v3 edited", time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC), nil)
		edited.NormalBalance = publicmasterdata.NormalBalanceCredit
		edited.IsActive = false
		if err := caps.versionWriter.SaveAccountVersion(ctx, edited); err == nil {
			t.Fatal("overlapping edit of the current version was accepted")
		}
		got, err := caps.versions.GetAccountAsOf(ctx, f.Company.ID, f.CashAccount.ID, time.Date(2026, time.March, 15, 0, 0, 0, 0, time.UTC))
		if err != nil {
			t.Fatalf("past version after edit: %v", err)
		}
		g019g022AssertVersion(t, got, v1)
	})
}

// TestPhase11_G019_VersionWritesFailClosed proves overlap, inverted-window and
// foreign-company version writes are rejected before any state is changed.
func TestPhase11_G019_VersionWritesFailClosed(t *testing.T) {
	ForEachStorageAdapter(t, "G019VersionWritesFailClosed", func(t *testing.T, h *StorageAdapterHarness, f UnifiedFixture) {
		caps := g019g022RequireCapabilities(t, h)
		ctx := context.Background()

		firstEnd := g019g022Date(31)
		first := g019g022Version(f.CashAccount, "First", time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC), &firstEnd)
		if err := caps.versionWriter.SaveAccountVersion(ctx, first); err != nil {
			t.Fatalf("save first version: %v", err)
		}
		touchingEnd := g019g022Date(31).AddDate(0, 0, 30)
		overlapping := g019g022Version(f.CashAccount, "Overlap", g019g022Date(31), &touchingEnd)
		if err := caps.versionWriter.SaveAccountVersion(ctx, overlapping); !errors.Is(err, publicmasterdata.ErrAccountVersionOverlap) {
			t.Fatalf("overlapping version error = %v, want ErrAccountVersionOverlap", err)
		}
		invertedEnd := g019g022Date(1)
		inverted := g019g022Version(f.CashAccount, "Inverted", g019g022Date(10), &invertedEnd)
		if err := caps.versionWriter.SaveAccountVersion(ctx, inverted); !errors.Is(err, publicmasterdata.ErrInvalidEffectiveRange) {
			t.Fatalf("inverted version error = %v, want ErrInvalidEffectiveRange", err)
		}

		other := phase8SecondCompany(t, ctx, h)
		foreign := g019g022Version(f.CashAccount, "Foreign", time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC), nil)
		foreign.CompanyID = other.Company.ID
		if err := caps.versionWriter.SaveAccountVersion(ctx, foreign); err == nil {
			t.Fatal("version write for another company was accepted")
		}
		if _, err := caps.versions.GetAccountAsOf(ctx, other.Company.ID, f.CashAccount.ID, g019g022Date(15)); err == nil {
			t.Fatal("effective read returned an account for another company")
		}
		got, err := caps.versions.GetAccountAsOf(ctx, f.Company.ID, f.CashAccount.ID, g019g022Date(15))
		if err != nil {
			t.Fatalf("past version after rejected writes: %v", err)
		}
		g019g022AssertVersion(t, got, first)
	})
}

// TestPhase11_G022_CostCenterAndDimensionScopes proves that the new public
// DTO-based readers and writers round-trip full records and reject foreign IDs
// in both directions.
func TestPhase11_G022_CostCenterAndDimensionScopes(t *testing.T) {
	ForEachStorageAdapter(t, "G022DimensionScopes", func(t *testing.T, h *StorageAdapterHarness, f UnifiedFixture) {
		caps := g019g022RequireCapabilities(t, h)
		ctx := context.Background()

		center := publicmasterdata.CostCenter{
			ID: types.NewCostCenterID(), CompanyID: f.Company.ID,
			Code: "CC-G019-G022", Name: "Operations", Active: true,
		}
		if err := caps.costCenterWr.CreateCostCenter(ctx, center); err != nil {
			t.Fatalf("CreateCostCenter: %v", err)
		}
		project := publicmasterdata.AnalyticalDimensionValue{
			ID: types.NewProjectID(), CompanyID: f.Company.ID, Kind: publicmasterdata.DimensionProject,
			Code: "PRJ-G019-G022", Name: "Parity Project", Active: true,
		}
		if err := caps.dimensionWrite.CreateAnalyticalDimensionValue(ctx, project); err != nil {
			t.Fatalf("CreateAnalyticalDimensionValue: %v", err)
		}

		gotCenter, err := caps.costCenters.GetCostCenterForCompany(ctx, f.Company.ID, center.ID)
		if err != nil {
			t.Fatalf("GetCostCenterForCompany: %v", err)
		}
		if gotCenter.ID != center.ID || gotCenter.CompanyID != center.CompanyID ||
			gotCenter.Code != center.Code || gotCenter.Name != center.Name || !gotCenter.Active {
			t.Fatalf("cost center = %+v, want %+v", gotCenter, center)
		}
		gotProject, err := caps.dimensions.GetAnalyticalDimensionForCompany(
			ctx, f.Company.ID, publicmasterdata.DimensionProject, project.ID.String())
		if err != nil {
			t.Fatalf("GetAnalyticalDimensionForCompany: %v", err)
		}
		if gotProject.ID != project.ID || gotProject.CompanyID != project.CompanyID ||
			gotProject.Kind != project.Kind || gotProject.Code != project.Code ||
			gotProject.Name != project.Name || !gotProject.Active {
			t.Fatalf("dimension = %+v, want %+v", gotProject, project)
		}

		other := phase8SecondCompany(t, ctx, h)
		foreignCenter := center
		foreignCenter.ID = types.NewCostCenterID()
		foreignCenter.CompanyID = other.Company.ID
		if err := caps.costCenterWr.CreateCostCenter(ctx, foreignCenter); err != nil {
			t.Fatalf("CreateCostCenter for second company: %v", err)
		}
		foreignProject := project
		foreignProject.ID = types.NewProjectID()
		foreignProject.CompanyID = other.Company.ID
		if err := caps.dimensionWrite.CreateAnalyticalDimensionValue(ctx, foreignProject); err != nil {
			t.Fatalf("CreateAnalyticalDimensionValue for second company: %v", err)
		}

		if _, err := caps.costCenters.GetCostCenterForCompany(ctx, f.Company.ID, foreignCenter.ID); !errors.Is(err, publicmasterdata.ErrInvalidCostCenter) {
			t.Fatalf("foreign cost center error = %v, want ErrInvalidCostCenter", err)
		}
		if _, err := caps.dimensions.GetAnalyticalDimensionForCompany(
			ctx, f.Company.ID, publicmasterdata.DimensionProject, foreignProject.ID.String()); !errors.Is(err, publicmasterdata.ErrInvalidAnalyticalDimension) {
			t.Fatalf("foreign dimension error = %v, want ErrInvalidAnalyticalDimension", err)
		}
		if _, err := caps.costCenters.GetCostCenterForCompany(ctx, other.Company.ID, center.ID); err == nil {
			t.Fatal("cost center was readable through another company scope")
		}
		if _, err := caps.dimensions.GetAnalyticalDimensionForCompany(
			ctx, other.Company.ID, publicmasterdata.DimensionProject, project.ID.String()); err == nil {
			t.Fatal("dimension was readable through another company scope")
		}
	})
}

// g019g022PlainMasterData hides every optional capability by exposing only the
// baseline MasterDataReader method set, even when the wrapped reader implements
// the optional interfaces.
type g019g022PlainMasterData struct {
	storage.MasterDataReader
}

func (a g019g022PlainMasterData) GetJournalByID(
	ctx context.Context,
	companyID types.CompanyID,
	journalID publicmasterdata.JournalID,
) (publicmasterdata.Journal, error) {
	if repo, ok := a.MasterDataReader.(publicmasterdata.JournalRepository); ok {
		return repo.GetJournalByID(ctx, companyID, journalID)
	}
	return publicmasterdata.Journal{}, publicmasterdata.ErrJournalNotFound
}

// g019g022PolicyMasterData keeps the adapter's real Phase 11 capabilities but
// presents distinct functional/presentation currencies to the SDK. The shared
// parity fixture deliberately uses USD for both, which makes the SDK's
// synthetic policy bootstrap invalid because it repeats USD in the allowed
// currency list. This wrapper isolates that unrelated policy fixture detail so
// the tests exercise G-019/G-022 capability detection and posting.
type g019g022PolicyMasterData struct {
	storage.MasterDataReader
	presentationCurrency string
	effective            publicmasterdata.EffectiveAccountReader
	costCenters          publicmasterdata.CostCenterReader
	dimensions           publicmasterdata.AnalyticalDimensionReader
}

func g019g022PolicyReader(reader storage.MasterDataReader) g019g022PolicyMasterData {
	effective, _ := reader.(publicmasterdata.EffectiveAccountReader)
	costCenters, _ := reader.(publicmasterdata.CostCenterReader)
	dimensions, _ := reader.(publicmasterdata.AnalyticalDimensionReader)
	return g019g022PolicyMasterData{
		MasterDataReader:     reader,
		presentationCurrency: "EUR",
		effective:            effective,
		costCenters:          costCenters,
		dimensions:           dimensions,
	}
}

func (a g019g022PolicyMasterData) GetCompany(
	ctx context.Context,
	companyID types.CompanyID,
) (publicmasterdata.Company, error) {
	company, err := a.MasterDataReader.GetCompany(ctx, companyID)
	if err != nil {
		return publicmasterdata.Company{}, err
	}
	company.PresentationCurrency = a.presentationCurrency
	return company, nil
}

func (a g019g022PolicyMasterData) GetAccountAsOf(
	ctx context.Context,
	companyID types.CompanyID,
	accountID types.AccountID,
	asOf time.Time,
) (publicmasterdata.Account, error) {
	return a.effective.GetAccountAsOf(ctx, companyID, accountID, asOf)
}

func (a g019g022PolicyMasterData) GetCostCenterForCompany(
	ctx context.Context,
	companyID types.CompanyID,
	costCenterID types.CostCenterID,
) (publicmasterdata.CostCenter, error) {
	return a.costCenters.GetCostCenterForCompany(ctx, companyID, costCenterID)
}

func (a g019g022PolicyMasterData) GetAnalyticalDimensionForCompany(
	ctx context.Context,
	companyID types.CompanyID,
	kind publicmasterdata.AnalyticalDimensionKind,
	id string,
) (publicmasterdata.AnalyticalDimensionValue, error) {
	return a.dimensions.GetAnalyticalDimensionForCompany(ctx, companyID, kind, id)
}

func (a g019g022PolicyMasterData) GetJournalByID(
	ctx context.Context,
	companyID types.CompanyID,
	journalID publicmasterdata.JournalID,
) (publicmasterdata.Journal, error) {
	if repo, ok := a.MasterDataReader.(publicmasterdata.JournalRepository); ok {
		return repo.GetJournalByID(ctx, companyID, journalID)
	}
	return publicmasterdata.Journal{}, publicmasterdata.ErrJournalNotFound
}

func g019g022DraftCommand(f UnifiedFixture, centerID types.CostCenterID, projectID types.ProjectID, key string) publicapplication.CreateDraftCommand {
	effective := f.OpenPeriod.StartDate.AddDate(0, 0, 14)
	amount := mustMoney("100.00", f.FunctionalCurrency)
	zero := mustMoney("0.00", f.FunctionalCurrency)
	return publicapplication.CreateDraftCommand{
		Context: publicapplication.Context{
			Principal:      publicapplication.Principal{ActorID: "g019-g022-tester"},
			CorrelationID:  "corr-" + key,
			IdempotencyKey: key,
			SourceSystem:   "contract-parity",
		},
		CompanyID:           f.Company.ID,
		JournalID:           f.Journal.ID,
		FiscalPeriodID:      f.OpenPeriod.ID,
		EffectiveDate:       effective,
		PostingDate:         effective,
		Description:         "G-019/G-022 dimensional posting",
		TransactionCurrency: f.FunctionalCurrency,
		ExchangeRate:        "1",
		Lines: []journal.EntryLine{
			{
				ID:                  types.NewJournalEntryLineID(),
				AccountID:           f.CashAccount.ID,
				TransactionAmount:   amount,
				TransactionCurrency: f.FunctionalCurrency,
				ExchangeRate:        decimal.NewFromInt(1),
				FunctionalDebit:     amount,
				FunctionalCredit:    zero,
			},
			{
				ID:                  types.NewJournalEntryLineID(),
				AccountID:           f.RevenueAccount.ID,
				TransactionAmount:   amount,
				TransactionCurrency: f.FunctionalCurrency,
				ExchangeRate:        decimal.NewFromInt(1),
				FunctionalDebit:     zero,
				FunctionalCredit:    amount,
				CostCenterID:        &centerID,
				ProjectID:           &projectID,
			},
		},
	}
}

func g019g022PostDraft(t *testing.T, h *StorageAdapterHarness, command publicapplication.CreateDraftCommand) (journal.JournalEntry, error) {
	t.Helper()
	ports := h.Ports
	ports.MasterData = g019g022PolicyReader(h.Ports.MasterData)
	app, err := sdk.NewApplicationService(ports)
	if err != nil {
		t.Fatalf("NewApplicationService: %v", err)
	}
	draft, err := app.CreateDraft(context.Background(), command)
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	result, err := app.Post(context.Background(), publicapplication.PostEntryCommand{
		Context:   command.Context,
		CompanyID: command.CompanyID,
		EntryID:   draft.Entry.ID,
	})
	if err != nil {
		return journal.JournalEntry{}, err
	}
	return result.Entry, nil
}

// TestPhase11_G019_G022_SDKPostingUsesDetectedCapabilities proves that the SDK
// detects both capability groups on the adapter ports and that a fully
// dimensioned entry succeeds and preserves its dimension IDs.
func TestPhase11_G019_G022_SDKPostingUsesDetectedCapabilities(t *testing.T) {
	ForEachStorageAdapter(t, "G019G022SDKPosting", func(t *testing.T, h *StorageAdapterHarness, f UnifiedFixture) {
		caps := g019g022RequireCapabilities(t, h)
		ctx := context.Background()
		start := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
		revenueVersion := g019g022Version(f.RevenueAccount, "Revenue with dimensions", start, nil)
		revenueVersion.RequiresCostCenter = true
		if err := caps.versionWriter.SaveAccountVersion(ctx, revenueVersion); err != nil {
			t.Fatalf("save revenue version: %v", err)
		}
		center := publicmasterdata.CostCenter{
			ID: types.NewCostCenterID(), CompanyID: f.Company.ID,
			Code: "CC-SDK-POST", Name: "SDK Post Center", Active: true,
		}
		if err := caps.costCenterWr.CreateCostCenter(ctx, center); err != nil {
			t.Fatalf("CreateCostCenter: %v", err)
		}
		project := publicmasterdata.AnalyticalDimensionValue{
			ID: types.NewProjectID(), CompanyID: f.Company.ID, Kind: publicmasterdata.DimensionProject,
			Code: "PRJ-SDK-POST", Name: "SDK Post Project", Active: true,
		}
		if err := caps.dimensionWrite.CreateAnalyticalDimensionValue(ctx, project); err != nil {
			t.Fatalf("CreateAnalyticalDimensionValue: %v", err)
		}

		command := g019g022DraftCommand(f, center.ID, project.ID, "g019-g022-post-key")
		entry, err := g019g022PostDraft(t, h, command)
		if err != nil {
			t.Fatalf("SDK post with dimensions: %v", err)
		}
		if entry.Status != journal.EntryPosted || len(entry.Lines) != 2 {
			t.Fatalf("posted entry = status %q, lines %d", entry.Status, len(entry.Lines))
		}
		stored, err := h.Ports.Entries.GetEntry(ctx, f.Company.ID, entry.ID)
		if err != nil {
			t.Fatalf("read posted entry: %v", err)
		}
		line := stored.Lines[1]
		if line.CostCenterID == nil || *line.CostCenterID != center.ID {
			t.Fatalf("stored cost center = %v, want %s", line.CostCenterID, center.ID.String())
		}
		if line.ProjectID == nil || *line.ProjectID != project.ID {
			t.Fatalf("stored project = %v, want %s", line.ProjectID, project.ID.String())
		}
	})
}

// TestPhase11_G022_SDKFailsClosedWithoutDimensionCapabilities proves that an
// SDK built over a port that lacks the optional readers still fails closed with
// ErrDimensionReaderRequired instead of silently accepting dimensions.
func TestPhase11_G022_SDKFailsClosedWithoutDimensionCapabilities(t *testing.T) {
	ForEachStorageAdapter(t, "G022SDKFailClosed", func(t *testing.T, h *StorageAdapterHarness, f UnifiedFixture) {
		caps := g019g022RequireCapabilities(t, h)
		ctx := context.Background()
		start := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
		revenueVersion := g019g022Version(f.RevenueAccount, "Revenue with dimensions", start, nil)
		revenueVersion.RequiresCostCenter = true
		if err := caps.versionWriter.SaveAccountVersion(ctx, revenueVersion); err != nil {
			t.Fatalf("save revenue version: %v", err)
		}
		centerID := types.NewCostCenterID()
		projectID := types.NewProjectID()
		command := g019g022DraftCommand(f, centerID, projectID, "g019-g022-no-cap-key")

		ports := h.Ports
		ports.MasterData = g019g022PlainMasterData{MasterDataReader: g019g022PolicyReader(h.Ports.MasterData)}
		app, err := sdk.NewApplicationService(ports)
		if err != nil {
			t.Fatalf("NewApplicationService: %v", err)
		}
		draft, err := app.CreateDraft(ctx, command)
		if err != nil {
			t.Fatalf("CreateDraft: %v", err)
		}
		_, err = app.Post(ctx, publicapplication.PostEntryCommand{
			Context:   command.Context,
			CompanyID: command.CompanyID,
			EntryID:   draft.Entry.ID,
		})
		if err == nil {
			t.Fatal("posting with dimensions succeeded without a dimension reader")
		}
		if !errors.Is(err, kernalposting.ErrDimensionReaderRequired) &&
			!strings.Contains(strings.ToLower(err.Error()), "dimension reader is required") {
			t.Fatalf("error = %v, want ErrDimensionReaderRequired", err)
		}
	})
}

// TestPhase11_G022_SDKRejectsForeignDimensionIDs proves that the SDK rejects a
// dimension belonging to another company even when the entry itself names the
// requesting company.
func TestPhase11_G022_SDKRejectsForeignDimensionIDs(t *testing.T) {
	ForEachStorageAdapter(t, "G022SDKForeignDimension", func(t *testing.T, h *StorageAdapterHarness, f UnifiedFixture) {
		caps := g019g022RequireCapabilities(t, h)
		ctx := context.Background()
		start := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
		revenueVersion := g019g022Version(f.RevenueAccount, "Revenue with dimensions", start, nil)
		revenueVersion.RequiresCostCenter = true
		if err := caps.versionWriter.SaveAccountVersion(ctx, revenueVersion); err != nil {
			t.Fatalf("save revenue version: %v", err)
		}
		other := phase8SecondCompany(t, ctx, h)
		foreignCenter := publicmasterdata.CostCenter{
			ID: types.NewCostCenterID(), CompanyID: other.Company.ID,
			Code: "CC-FOREIGN", Name: "Foreign Center", Active: true,
		}
		if err := caps.costCenterWr.CreateCostCenter(ctx, foreignCenter); err != nil {
			t.Fatalf("create foreign cost center: %v", err)
		}
		projectID := types.NewProjectID()
		command := g019g022DraftCommand(f, foreignCenter.ID, projectID, "g019-g022-foreign-key")
		_, err := g019g022PostDraft(t, h, command)
		if err == nil {
			t.Fatal("SDK accepted a cost center from another company")
		}
		if !strings.Contains(strings.ToLower(err.Error()), "cost center") {
			t.Fatalf("error = %v, want a cost center scope failure", err)
		}
	})
}
