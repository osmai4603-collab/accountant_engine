package masterdata

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/osmai4603-collab/accounting_engine/internal/kernal/ledger"
	"github.com/osmai4603-collab/accounting_engine/internal/kernal/types"
)

// zeroScopeReader would happily return records owned by the zero company. It
// records every access so the tests can prove that fail-closed paths never
// reach the repository with an untrusted (zero) company scope.
type zeroScopeReader struct {
	calls   int
	company Company
	account Account
	period  FiscalPeriod
	year    FiscalYear
}

func (r *zeroScopeReader) GetCompany(context.Context, types.CompanyID) (Company, error) {
	r.calls++
	return r.company, nil
}

func (r *zeroScopeReader) GetAccount(context.Context, types.CompanyID, types.AccountID) (Account, error) {
	r.calls++
	return r.account, nil
}

func (r *zeroScopeReader) GetAccountByCode(context.Context, types.CompanyID, string) (Account, error) {
	r.calls++
	return r.account, nil
}

func (r *zeroScopeReader) GetJournalByCode(context.Context, types.CompanyID, string) (Journal, error) {
	r.calls++
	return Journal{}, nil
}

func (r *zeroScopeReader) HasAccountChildren(context.Context, types.CompanyID, types.AccountID) (bool, error) {
	r.calls++
	return false, nil
}

func (r *zeroScopeReader) GetFiscalYear(context.Context, types.CompanyID, types.FiscalYearID) (FiscalYear, error) {
	r.calls++
	return r.year, nil
}

func (r *zeroScopeReader) GetPeriodForDate(context.Context, types.CompanyID, time.Time) (FiscalPeriod, error) {
	r.calls++
	return r.period, nil
}

func (r *zeroScopeReader) GetPeriodByNumber(
	context.Context, types.CompanyID, types.FiscalYearID, int,
) (FiscalPeriod, error) {
	r.calls++
	return r.period, nil
}

func TestCompanyScopeFailsClosedWithoutRepositoryAccess(t *testing.T) {
	ctx := context.Background()
	zeroCompany := types.CompanyID{}
	accountID := types.NewAccountID()
	date := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)

	period := FiscalPeriod{
		ID: types.NewFiscalPeriodID(), CompanyID: zeroCompany,
		FiscalYearID: types.NewFiscalYearID(), Number: 6,
		Name: "June 2026", Type: FiscalPeriodTypeNormal, State: PeriodOpen,
		StartDate: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, 6, 30, 23, 59, 59, 999999999, time.UTC),
	}
	year := FiscalYear{
		ID: period.FiscalYearID, CompanyID: zeroCompany, Name: "FY2026",
		StartDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, 12, 31, 23, 59, 59, 999999999, time.UTC),
		State:     FiscalYearOpen,
	}
	adjustment := period
	adjustment.Number = 13
	adjustment.Type = FiscalPeriodTypeYearEndAdjustment
	adjustment.StartDate, adjustment.EndDate = year.EndDate, year.EndDate

	cases := []struct {
		name string
		run  func(*zeroScopeReader) error
	}{
		{"company", func(r *zeroScopeReader) error {
			_, err := ValidateCompanyScope(ctx, r, zeroCompany)
			return err
		}},
		{"account scope", func(r *zeroScopeReader) error {
			return ValidateAccountScope(r.account, zeroCompany, accountID)
		}},
		{"account posting", func(r *zeroScopeReader) error {
			return ValidateCanPostToAccountAsOf(ctx, r, zeroCompany, accountID, date)
		}},
		{"period posting", func(r *zeroScopeReader) error {
			_, err := ValidatePeriodForPostingWithSoftClose(ctx, r, zeroCompany, date, false)
			return err
		}},
		{"adjustment posting", func(r *zeroScopeReader) error {
			_, err := ValidateAdjustmentPeriodForPostingWithSoftClose(ctx, r, zeroCompany, date, 13, false)
			return err
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &zeroScopeReader{
				company: Company{ID: zeroCompany, Active: true},
				account: Account{ID: accountID, CompanyID: zeroCompany, IsActive: true},
				period:  period,
				year:    year,
			}
			if tc.name == "adjustment posting" {
				r.period = adjustment
			}
			err := tc.run(r)
			if !errors.Is(err, ErrCompanyScopeRequired) {
				t.Fatalf("zero company scope error = %v, want ErrCompanyScopeRequired", err)
			}
			if r.calls != 0 {
				t.Fatalf("zero company scope reached the repository %d time(s)", r.calls)
			}
		})
	}
}

func TestAccountHierarchyRejectsCrossCompanyLinks(t *testing.T) {
	companyA, companyB := types.NewCompanyID(), types.NewCompanyID()
	parentID, childID := types.NewAccountID(), types.NewAccountID()
	parent := Account{ID: parentID, CompanyID: companyA, Code: "1000", Name: "Parent"}
	child := Account{ID: childID, CompanyID: companyB, Code: "1100", Name: "Child", ParentID: &parentID}

	for _, tc := range []struct {
		name     string
		accounts []Account
	}{
		{"cross-company parent", []Account{parent, child}},
		{"mixed companies without parent links", []Account{parent, Account{ID: childID, CompanyID: companyB, Code: "1100", Name: "Other"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := DetectCyclicHierarchy(tc.accounts); !errors.Is(err, ErrAccountIdentityMismatch) {
				t.Fatalf("DetectCyclicHierarchy error = %v, want ErrAccountIdentityMismatch", err)
			}
			if _, err := BuildHierarchyTree(tc.accounts); !errors.Is(err, ErrAccountIdentityMismatch) {
				t.Fatalf("BuildHierarchyTree error = %v, want ErrAccountIdentityMismatch", err)
			}
		})
	}

	t.Run("same company remains valid", func(t *testing.T) {
		sameCompanyChild := child
		sameCompanyChild.CompanyID = companyA
		if err := DetectCyclicHierarchy([]Account{parent, sameCompanyChild}); err != nil {
			t.Fatalf("same-company hierarchy: %v", err)
		}
		roots, err := BuildHierarchyTree([]Account{parent, sameCompanyChild})
		if err != nil || len(roots) != 1 || len(roots[0].Children) != 1 {
			t.Fatalf("same-company tree = %v, err = %v", roots, err)
		}
	})
}

func TestAccountValidateRejectsReversedEffectiveRange(t *testing.T) {
	from := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, time.December, 31, 0, 0, 0, 0, time.UTC)
	account := Account{
		ID: types.NewAccountID(), Code: "1000", Name: "Cash", Type: AccountTypeAsset,
		EffectiveFrom: from, EffectiveTo: &to,
	}
	if err := account.Validate(); err != nil {
		t.Fatalf("valid effective range: %v", err)
	}

	reversed := account
	reversed.EffectiveFrom, reversed.EffectiveTo = to, &from
	if err := reversed.Validate(); !errors.Is(err, ErrInvalidEffectiveRange) {
		t.Fatalf("reversed effective range error = %v, want ErrInvalidEffectiveRange", err)
	}
	if err := reversed.IsValid(); !errors.Is(err, ErrInvalidEffectiveRange) {
		t.Fatalf("IsValid reversed range error = %v, want ErrInvalidEffectiveRange", err)
	}
}

// fixedClock lets the test control what "today" means for the compatibility
// wrapper without ever calling the wall clock from domain logic.
type fixedClock struct{ at time.Time }

func (c fixedClock) Now() time.Time { return c.at }

type versionedAccountReader struct {
	*mockMasterDataReader
	versions []Account
}

func (r *versionedAccountReader) GetAccountAsOf(
	_ context.Context, companyID types.CompanyID, accountID types.AccountID, asOf time.Time,
) (Account, error) {
	for _, version := range r.versions {
		if version.ID == accountID && version.CompanyID == companyID && version.IsCurrent(asOf) {
			return version, nil
		}
	}
	return Account{}, ErrAccountNotFound
}

func TestEffectiveDatingUsesInjectedClockAndVersionedReader(t *testing.T) {
	ctx := context.Background()
	companyID, accountID := types.NewCompanyID(), types.NewAccountID()
	effectiveFrom := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	effectiveTo := time.Date(2026, time.June, 30, 0, 0, 0, 0, time.UTC)
	historical := Account{
		ID: accountID, CompanyID: companyID, Code: "1000", Name: "Cash (old)",
		IsActive: true, RequiresCostCenter: true,
		EffectiveFrom: effectiveFrom, EffectiveTo: &effectiveTo,
	}
	current := Account{
		ID: accountID, CompanyID: companyID, Code: "1000", Name: "Cash (new)",
		IsActive: false, RequiresCostCenter: false,
		EffectiveFrom: time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC),
	}

	base := newMockReader()
	base.accounts[accountID] = historical
	reader := &versionedAccountReader{mockMasterDataReader: base, versions: []Account{historical, current}}

	clock := fixedClock{at: time.Date(2026, time.March, 15, 0, 0, 0, 0, time.UTC)}
	if err := ValidateCanPostToAccountWithClock(ctx, reader, companyID, accountID, clock); err != nil {
		t.Fatalf("historical posting with injected clock: %v", err)
	}

	// Changing the current master-data version must not reinterpret the past:
	// the effective version is resolved by the as-of port, not by wall time.
	reader.versions[1].Name = "Cash (renamed)"
	reader.versions[1].Code = "9999"
	reader.versions[1].IsActive = true
	if err := ValidateCanPostToAccountWithClock(ctx, reader, companyID, accountID, clock); err != nil {
		t.Fatalf("historical result changed after current master-data edit: %v", err)
	}

	before := fixedClock{at: effectiveFrom.Add(-time.Nanosecond)}
	if err := ValidateCanPostToAccountWithClock(ctx, reader, companyID, accountID, before); err == nil {
		t.Fatal("posting before the first effective version must be rejected")
	}
	after := fixedClock{at: effectiveTo.Add(time.Nanosecond)}
	if err := ValidateCanPostToAccountWithClock(ctx, reader, companyID, accountID, after); !errors.Is(err, ErrAccountNotFound) {
		t.Fatalf("after effective-to error = %v, want account-not-found from versioned reader", err)
	}

	// A reader without the as-of port falls back to a single record and fails
	// closed when that record is not current on the posting date.
	legacy := newMockReader()
	legacy.accounts[accountID] = historical
	if err := ValidateCanPostToAccountAsOf(ctx, legacy, companyID, accountID, after.at); !errors.Is(err, ErrAccountNotCurrent) {
		t.Fatalf("legacy reader with expired record error = %v, want ErrAccountNotCurrent", err)
	}
}

func TestJournalReaderScopeIsValidated(t *testing.T) {
	companyID, journalID := types.NewCompanyID(), types.NewJournalID()
	journal := Journal{
		ID: journalID, CompanyID: companyID, Code: "GEN", Name: "General",
		EffectiveFrom: time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC),
	}
	if err := ValidateJournalScope(journal, companyID, journalID); err != nil {
		t.Fatalf("matching journal scope: %v", err)
	}
	for _, tc := range []struct {
		name      string
		companyID types.CompanyID
		journalID types.JournalID
	}{
		{"foreign company", types.NewCompanyID(), journalID},
		{"foreign journal", companyID, types.NewJournalID()},
		{"zero company", types.CompanyID{}, journalID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateJournalScope(journal, tc.companyID, tc.journalID)
			if tc.name == "zero company" {
				if !errors.Is(err, ErrCompanyScopeRequired) {
					t.Fatalf("error = %v, want ErrCompanyScopeRequired", err)
				}
				return
			}
			if !errors.Is(err, ErrJournalIdentityMismatch) {
				t.Fatalf("error = %v, want ErrJournalIdentityMismatch", err)
			}
		})
	}
}

func TestFiscalCalendarBoundaryMatrix(t *testing.T) {
	companyID := types.NewCompanyID()
	for _, tc := range []struct {
		name  string
		start time.Time
	}{
		{"january month end", time.Date(2026, time.January, 31, 18, 45, 0, 0, time.FixedZone("UTC+3", 3*3600))},
		{"leap day", time.Date(2024, time.February, 29, 0, 0, 0, 0, time.UTC)},
		{"non-leap february", time.Date(2026, time.February, 28, 0, 0, 0, 0, time.UTC)},
		{"december year end", time.Date(2026, time.December, 31, 0, 0, 0, 0, time.UTC)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			year, periods := GenerateFiscalYear(companyID, tc.start)
			if !year.StartDate.Equal(tc.start.UTC().Truncate(24 * time.Hour)) {
				t.Fatalf("year start = %v, want UTC midnight of %v", year.StartDate, tc.start)
			}
			if err := ValidateFiscalYear(year, periods); err != nil {
				t.Fatalf("generated fiscal year rejected: %v", err)
			}
			// Adjustment periods intentionally share the year-end instant; they
			// are not date-searchable. Continuity applies to periods 1..12.
			for i := 1; i < 12; i++ {
				if !periods[i-1].EndDate.Add(time.Nanosecond).Equal(periods[i].StartDate) {
					t.Fatalf("gap/overlap between periods %d and %d", i, i+1)
				}
			}
			if periods[12].Type != FiscalPeriodTypeYearEndAdjustment ||
				periods[13].Type != FiscalPeriodTypeAuditAdjustment {
				t.Fatal("periods 13 and 14 must have explicit adjustment types")
			}
			for _, adjustment := range []FiscalPeriod{periods[12], periods[13]} {
				if adjustment.Contains(adjustment.StartDate) || adjustment.Contains(year.EndDate) {
					t.Fatal("adjustment periods must not be discovered by normal date search")
				}
			}
		})
	}
}

func TestCloseStateMatrixForPeriodAndYear(t *testing.T) {
	ctx := context.Background()
	companyID := types.NewCompanyID()
	yearID := types.NewFiscalYearID()
	date := time.Date(2026, time.June, 15, 0, 0, 0, 0, time.UTC)
	period := FiscalPeriod{
		ID: types.NewFiscalPeriodID(), CompanyID: companyID, FiscalYearID: yearID,
		Number: 6, Name: "June 2026", Type: FiscalPeriodTypeNormal,
		StartDate: time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, time.June, 30, 23, 59, 59, 999999999, time.UTC),
	}

	for _, tc := range []struct {
		name        string
		yearState   FiscalYearState
		periodState PeriodState
		allowSoft   bool
		wantErr     error
	}{
		{"both open", FiscalYearOpen, PeriodOpen, false, nil},
		{"period soft closed without policy", FiscalYearOpen, PeriodSoftClosed, false, ErrClosedPeriod},
		{"period soft closed with policy", FiscalYearOpen, PeriodSoftClosed, true, nil},
		{"period hard closed with policy", FiscalYearOpen, PeriodHardClosed, true, ErrClosedPeriod},
		{"year soft closed without policy", FiscalYearSoftClosed, PeriodOpen, false, ErrClosedFiscalYear},
		{"year soft closed with policy", FiscalYearSoftClosed, PeriodOpen, true, nil},
		{"year hard closed with policy", FiscalYearHardClosed, PeriodOpen, true, ErrClosedFiscalYear},
		{"both soft closed with policy", FiscalYearSoftClosed, PeriodSoftClosed, true, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			year := FiscalYear{
				ID: yearID, CompanyID: companyID, Name: "FY2026", State: tc.yearState,
				StartDate: time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC),
				EndDate:   time.Date(2026, time.December, 31, 23, 59, 59, 999999999, time.UTC),
			}
			currentPeriod := period
			currentPeriod.State = tc.periodState
			reader := overrideFiscalReader{year: year, regular: currentPeriod}
			_, err := ValidatePeriodForPostingWithSoftClose(ctx, reader, companyID, date, tc.allowSoft)
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("posting unexpectedly rejected: %v", err)
				}
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

// TestReferenceModelsDoNotDriftSilently is the kernel-side half of G-016: any
// new authority field that is not reflected in the reference DTO must be added
// to the documented drop set explicitly, and any DTO field without an authority
// counterpart fails immediately.
func TestReferenceModelsDoNotDriftSilently(t *testing.T) {
	assertFields := func(authority, dto reflect.Type, dtoOnly map[string]bool, dropped map[string]bool) {
		t.Helper()
		authorityFields := map[string]reflect.Kind{}
		for i := 0; i < authority.NumField(); i++ {
			field := authority.Field(i)
			authorityFields[field.Name] = field.Type.Kind()
		}
		dtoFields := map[string]reflect.Kind{}
		for i := 0; i < dto.NumField(); i++ {
			field := dto.Field(i)
			dtoFields[field.Name] = field.Type.Kind()
		}
		for name, kind := range dtoFields {
			if dtoOnly[name] {
				continue
			}
			authorityKind, exists := authorityFields[name]
			if !exists {
				t.Errorf("%s has no authority field in %s", name, authority.Name())
				continue
			}
			if authorityKind != kind {
				t.Errorf("field %s kind drift: %s vs %s", name, authorityKind, kind)
			}
		}
		documented := map[string]bool{}
		for name := range dropped {
			documented[name] = true
		}
		for name := range dtoFields {
			documented[name] = true
		}
		for name := range authorityFields {
			if !documented[name] {
				t.Errorf("authority field %s is silently dropped from %s; update ADR P1-03 and this set", name, dto.Name())
			}
		}
		for name := range dropped {
			if _, exists := authorityFields[name]; !exists {
				t.Errorf("documented dropped field %s no longer exists in %s", name, authority.Name())
			}
		}
	}

	assertFields(
		reflect.TypeOf(Account{}), reflect.TypeOf(ledger.Account{}), nil,
		map[string]bool{
			"IsControlAccount": true, "IsContraAccount": true, "RequiresCostCenter": true,
			"IsMonetary": true, "IsCashEquivalent": true, "IsNonCashAdjustment": true, "CashFlowActivity": true,
			"IFRSCategory": true, "DisplayOrder": true, "EffectiveFrom": true,
			"EffectiveTo": true, "CreatedAt": true, "UpdatedAt": true,
		},
	)
	assertFields(reflect.TypeOf(FiscalPeriod{}), reflect.TypeOf(ledger.FiscalPeriod{}), nil, nil)
	assertFields(reflect.TypeOf(FiscalYear{}), reflect.TypeOf(ledger.FiscalYear{}), map[string]bool{"Periods": true}, nil)
}

func FuzzEffectiveRangeValidationNeverPanics(f *testing.F) {
	for _, seed := range [][3]int64{
		{1767225600, 1798761600, 1}, // 2026-01-01 .. 2027-01-01
		{1798761600, 1767225600, 1}, // inverted
		{1767225600, 0, 0},          // open ended
	} {
		f.Add(seed[0], seed[1], seed[2])
	}
	f.Fuzz(func(t *testing.T, fromUnix, toUnix int64, hasTo int64) {
		from := time.Unix(fromUnix, 0).UTC()
		account := Account{Code: "1000", Name: "Cash", Type: AccountTypeAsset, EffectiveFrom: from}
		if hasTo != 0 {
			to := time.Unix(toUnix, 0).UTC()
			account.EffectiveTo = &to
		}

		err := account.Validate()
		if err != nil && !errors.Is(err, ErrInvalidEffectiveRange) {
			t.Fatalf("unexpected validation error: %v", err)
		}
		if err != nil {
			return
		}
		if account.IsCurrent(from.Add(-time.Second)) {
			t.Fatal("account became current before effective-from")
		}
		if account.EffectiveTo != nil && account.IsCurrent(account.EffectiveTo.Add(time.Second)) {
			t.Fatal("account remained current after effective-to")
		}
	})
}

func TestMasterDataAdversarialInputsFailClosed(t *testing.T) {
	companyID := types.NewCompanyID()
	ctx := context.Background()

	journal := Journal{ID: types.NewJournalID(), CompanyID: companyID, Code: "GEN", Name: "General"}
	if err := journal.ValidateWithPolicy([]string{"USD ", "usd"}, nil); err == nil {
		t.Fatal("duplicate normalized journal currencies must be rejected")
	}
	if err := journal.ValidateWithPolicy([]string{string(types.XXX)}, nil); err == nil {
		t.Fatal("XXX must not be an allowed journal currency")
	}

	account := Account{ID: types.NewAccountID(), CompanyID: companyID, Code: "1000", Name: "Cash", Type: AccountTypeAsset}
	if err := account.Validate(); err != nil {
		t.Fatalf("baseline account: %v", err)
	}
	control := account
	control.IsControlAccount = true
	if err := control.Validate(); !errors.Is(err, ErrInvalidControlAccount) {
		t.Fatalf("unreconcilable control account error = %v", err)
	}

	reader := newMockReader()
	accountID := types.NewAccountID()
	reader.accounts[accountID] = Account{ID: accountID, CompanyID: types.NewCompanyID(), IsActive: true}
	postingDate := time.Date(2026, time.March, 15, 0, 0, 0, 0, time.UTC)
	if err := ValidateCanPostToAccountAsOf(ctx, reader, companyID, accountID, postingDate); !errors.Is(err, ErrAccountNotFound) {
		t.Fatalf("foreign-company account must not be reachable: %v", err)
	}
}

func TestAccountIsMonetaryAndAnalyticalDimensionHelpers(t *testing.T) {
	yes, no := true, false
	cash := Account{ID: types.NewAccountID(), CompanyID: types.NewCompanyID(), Code: "1010", Name: "FX Cash", Type: AccountTypeAsset, IsMonetary: &yes}
	if err := cash.Validate(); err != nil {
		t.Fatalf("monetary asset Validate = %v", err)
	}
	if isMon, explicit := cash.MonetaryClassification(); !isMon || !explicit {
		t.Fatalf("cash MonetaryClassification = (%v, %v), want (true, true)", isMon, explicit)
	}
	inventory := Account{ID: types.NewAccountID(), CompanyID: types.NewCompanyID(), Code: "1200", Name: "Inventory", Type: AccountTypeAsset, IsMonetary: &no}
	if isMon, explicit := inventory.MonetaryClassification(); isMon || !explicit {
		t.Fatalf("inventory MonetaryClassification = (%v, %v), want (false, true)", isMon, explicit)
	}
	unspecifiedAsset := Account{ID: types.NewAccountID(), CompanyID: types.NewCompanyID(), Code: "1300", Name: "Prepaid", Type: AccountTypeAsset}
	if isMon, explicit := unspecifiedAsset.MonetaryClassification(); isMon || explicit {
		t.Fatalf("unspecified asset MonetaryClassification = (%v, %v), want (false, false)", isMon, explicit)
	}
	for _, nonBS := range []AccountType{AccountTypeEquity, AccountTypeIncome, AccountTypeExpense} {
		invalid := Account{ID: types.NewAccountID(), CompanyID: types.NewCompanyID(), Code: "9000", Name: "Invalid", Type: nonBS, IsMonetary: &yes}
		if err := invalid.Validate(); !errors.Is(err, ErrInvalidAccountType) {
			t.Fatalf("%s with IsMonetary=true error = %v, want ErrInvalidAccountType", nonBS, err)
		}
		if isMon, explicit := (Account{Type: nonBS}).MonetaryClassification(); isMon || !explicit {
			t.Fatalf("%s MonetaryClassification = (%v, %v), want (false, true)", nonBS, isMon, explicit)
		}
	}

	companyID := types.NewCompanyID()
	projID := types.NewProjectID()
	segID := types.NewSegmentID()
	buID := types.NewBusinessUnitID()

	projDim := NewProjectDimensionValue(projID, companyID, "PRJ-1", "Project One", true)
	segDim := NewSegmentDimensionValue(segID, companyID, "SEG-1", "Segment One", true)
	buDim := NewBusinessUnitDimensionValue(buID, companyID, "BU-1", "Business Unit One", true)

	if projDim.DimensionID() != projID.String() || segDim.DimensionID() != segID.String() || buDim.DimensionID() != buID.String() {
		t.Fatal("DimensionID mismatch on analytical dimension constructors")
	}
	if got, ok := projDim.AsProjectID(); !ok || got != projID {
		t.Fatalf("AsProjectID = (%v, %v)", got, ok)
	}
	if _, ok := segDim.AsProjectID(); ok {
		t.Fatal("segment dim must not convert AsProjectID")
	}
	if got, ok := segDim.AsSegmentID(); !ok || got != segID {
		t.Fatalf("AsSegmentID = (%v, %v)", got, ok)
	}
	if _, ok := projDim.AsSegmentID(); ok {
		t.Fatal("project dim must not convert AsSegmentID")
	}
	if got, ok := buDim.AsBusinessUnitID(); !ok || got != buID {
		t.Fatalf("AsBusinessUnitID = (%v, %v)", got, ok)
	}
	if _, ok := segDim.AsBusinessUnitID(); ok {
		t.Fatal("segment dim must not convert AsBusinessUnitID")
	}
}
