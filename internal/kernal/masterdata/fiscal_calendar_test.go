package masterdata

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/osmai4603-collab/accounting_engine/internal/kernal/types"
)

type mockMasterDataReader struct {
	companies   map[types.CompanyID]Company
	accounts    map[types.AccountID]Account
	children    map[types.AccountID]bool
	periods     map[types.CompanyID][]FiscalPeriod
	fiscalYears map[types.FiscalYearID]FiscalYear
}

type overrideFiscalReader struct {
	Reader
	year        FiscalYear
	regular     FiscalPeriod
	adjustments map[int]FiscalPeriod
}

func (r overrideFiscalReader) GetPeriodForDate(context.Context, types.CompanyID, time.Time) (FiscalPeriod, error) {
	return r.regular, nil
}

func (r overrideFiscalReader) GetFiscalYear(context.Context, types.CompanyID, types.FiscalYearID) (FiscalYear, error) {
	return r.year, nil
}

func (r overrideFiscalReader) GetPeriodByNumber(
	_ context.Context,
	_ types.CompanyID,
	_ types.FiscalYearID,
	number int,
) (FiscalPeriod, error) {
	period, ok := r.adjustments[number]
	if !ok {
		return FiscalPeriod{}, types.ErrFiscalPeriodNotFound
	}
	return period, nil
}

func newMockReader() *mockMasterDataReader {
	return &mockMasterDataReader{
		companies:   make(map[types.CompanyID]Company),
		accounts:    make(map[types.AccountID]Account),
		children:    make(map[types.AccountID]bool),
		periods:     make(map[types.CompanyID][]FiscalPeriod),
		fiscalYears: make(map[types.FiscalYearID]FiscalYear),
	}
}

func (m *mockMasterDataReader) GetCompany(ctx context.Context, companyID types.CompanyID) (Company, error) {
	c, ok := m.companies[companyID]
	if !ok {
		return Company{}, ErrCompanyNotFound
	}
	return c, nil
}

func (m *mockMasterDataReader) GetAccount(ctx context.Context, companyID types.CompanyID, accountID types.AccountID) (Account, error) {
	a, ok := m.accounts[accountID]
	if !ok || a.CompanyID != companyID {
		return Account{}, types.ErrAccountNotFound
	}
	return a, nil
}

func (m *mockMasterDataReader) GetAccountByCode(ctx context.Context, companyID types.CompanyID, code string) (Account, error) {
	for _, a := range m.accounts {
		if a.CompanyID == companyID && a.Code == code {
			return a, nil
		}
	}
	return Account{}, types.ErrAccountNotFound
}

func (m *mockMasterDataReader) GetJournalByCode(ctx context.Context, companyID types.CompanyID, code string) (Journal, error) {
	return Journal{}, ErrJournalNotFound
}

func (m *mockMasterDataReader) HasAccountChildren(ctx context.Context, companyID types.CompanyID, accountID types.AccountID) (bool, error) {
	return m.children[accountID], nil
}

func (m *mockMasterDataReader) GetFiscalYear(ctx context.Context, companyID types.CompanyID, fiscalYearID types.FiscalYearID) (FiscalYear, error) {
	fy, ok := m.fiscalYears[fiscalYearID]
	if !ok || fy.CompanyID != companyID {
		return FiscalYear{}, errors.New("fiscal year not found")
	}
	return fy, nil
}

func (m *mockMasterDataReader) GetPeriodForDate(ctx context.Context, companyID types.CompanyID, date time.Time) (FiscalPeriod, error) {
	for _, p := range m.periods[companyID] {
		if (date.Equal(p.StartDate) || date.After(p.StartDate)) && (date.Equal(p.EndDate) || date.Before(p.EndDate)) {
			return p, nil
		}
	}
	return FiscalPeriod{}, types.ErrFiscalPeriodNotFound
}

func TestCOAAndPostingValidation(t *testing.T) {
	ctx := context.Background()
	reader := newMockReader()
	compID := types.NewCompanyID()
	reader.companies[compID] = Company{ID: compID, Active: true}

	// 5 level hierarchy test
	level1ID := types.NewAccountID()
	level2ID := types.NewAccountID()
	level3ID := types.NewAccountID()
	level4ID := types.NewAccountID()
	level5ID := types.NewAccountID()

	accs := []Account{
		{ID: level1ID, CompanyID: compID, Code: "1000", Name: "Assets", IsActive: true, ParentID: nil},
		{ID: level2ID, CompanyID: compID, Code: "1100", Name: "Current Assets", IsActive: true, ParentID: &level1ID},
		{ID: level3ID, CompanyID: compID, Code: "1110", Name: "Cash and Cash Equivalents", IsActive: true, ParentID: &level2ID},
		{ID: level4ID, CompanyID: compID, Code: "1111", Name: "Bank Accounts", IsActive: true, ParentID: &level3ID},
		{ID: level5ID, CompanyID: compID, Code: "111101", Name: "Main Operating Bank Account", IsActive: true, ParentID: &level4ID},
	}

	for _, a := range accs {
		reader.accounts[a.ID] = a
	}
	reader.children[level1ID] = true
	reader.children[level2ID] = true
	reader.children[level3ID] = true
	reader.children[level4ID] = true
	reader.children[level5ID] = false

	// Build hierarchy tree
	tree, err := BuildHierarchyTree(accs)
	if err != nil {
		t.Fatalf("BuildHierarchyTree failed: %v", err)
	}
	if len(tree) != 1 {
		t.Fatalf("expected 1 root, got %d", len(tree))
	}
	if len(tree[0].Children) != 1 || len(tree[0].Children[0].Children) != 1 {
		t.Fatalf("hierarchy depth check failed")
	}

	// Reject posting to header / parent account
	if err := ValidateCanPostToAccount(ctx, reader, compID, level1ID); !errors.Is(err, ErrHeaderAccountPosting) {
		t.Fatalf("expected ErrHeaderAccountPosting on level 1, got %v", err)
	}
	if err := ValidateCanPostToAccount(ctx, reader, compID, level4ID); !errors.Is(err, ErrHeaderAccountPosting) {
		t.Fatalf("expected ErrHeaderAccountPosting on level 4, got %v", err)
	}

	// Allow posting to leaf account
	if err := ValidateCanPostToAccount(ctx, reader, compID, level5ID); err != nil {
		t.Fatalf("expected posting to leaf account to succeed, got %v", err)
	}

	// Cyclic hierarchy detection
	cyclicAccs := []Account{
		{ID: level1ID, Code: "1000", ParentID: &level2ID},
		{ID: level2ID, Code: "1100", ParentID: &level1ID},
	}
	if err := DetectCyclicHierarchy(cyclicAccs); !errors.Is(err, ErrAccountCycle) {
		t.Fatalf("expected ErrAccountCycle, got %v", err)
	}
}

func TestFiscalCalendarAndPeriodValidation(t *testing.T) {
	ctx := context.Background()
	reader := newMockReader()
	compID := types.NewCompanyID()
	reader.companies[compID] = Company{ID: compID, Active: true}

	fy, periods := GenerateFiscalYear(compID, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if len(periods) != 14 {
		t.Fatalf("expected 14 periods, got %d", len(periods))
	}
	reader.fiscalYears[fy.ID] = fy
	reader.periods[compID] = periods

	// Post during open period
	janDate := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	p, err := ValidatePeriodForPosting(ctx, reader, compID, janDate)
	if err != nil {
		t.Fatalf("expected open period for posting, got %v", err)
	}
	if p.Number != 1 {
		t.Fatalf("expected period 1, got %d", p.Number)
	}

	// Close period 1
	for i := range reader.periods[compID] {
		if reader.periods[compID][i].Number == 1 {
			reader.periods[compID][i].State = PeriodHardClosed
		}
	}

	// Re-attempt posting in closed period
	_, err = ValidatePeriodForPosting(ctx, reader, compID, janDate)
	if !errors.Is(err, ErrClosedPeriod) {
		t.Fatalf("expected ErrClosedPeriod, got %v", err)
	}

	// Date outside fiscal year
	outDate := time.Date(2027, 5, 1, 0, 0, 0, 0, time.UTC)
	_, err = ValidatePeriodForPosting(ctx, reader, compID, outDate)
	if !errors.Is(err, types.ErrFiscalPeriodNotFound) {
		t.Fatalf("expected ErrFiscalPeriodNotFound, got %v", err)
	}
}

func TestPostingPeriodRequiresFiscalYearAndExplicitSoftClosePolicy(t *testing.T) {
	ctx := context.Background()
	reader := newMockReader()
	companyID, yearID := types.NewCompanyID(), types.NewFiscalYearID()
	reader.fiscalYears[yearID] = FiscalYear{
		ID: yearID, CompanyID: companyID, State: FiscalYearSoftClosed,
		StartDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, 12, 31, 23, 59, 59, 999999999, time.UTC),
	}
	date := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	reader.periods[companyID] = []FiscalPeriod{{
		ID: types.NewFiscalPeriodID(), CompanyID: companyID, FiscalYearID: yearID,
		State: PeriodSoftClosed, Type: FiscalPeriodTypeNormal, Number: 6,
		StartDate: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, 6, 30, 23, 59, 59, 0, time.UTC),
	}}

	if _, err := ValidatePeriodForPosting(ctx, reader, companyID, date); !errors.Is(err, ErrClosedFiscalYear) {
		t.Fatalf("posting in soft-closed year without policy error = %v, want closed fiscal year", err)
	}
	if _, err := ValidatePeriodForPostingWithSoftClose(ctx, reader, companyID, date, true); err != nil {
		t.Fatalf("posting in soft-closed period/year with explicit policy: %v", err)
	}

	reader.periods[companyID][0].FiscalYearID = types.FiscalYearID{}
	if _, err := ValidatePeriodForPostingWithSoftClose(ctx, reader, companyID, date, true); !errors.Is(err, ErrInvalidFiscalPeriod) {
		t.Fatalf("period without fiscal year error = %v, want incomplete fiscal period", err)
	}
}

func TestFiscalPeriodRepositoryDataIsValidatedDefensively(t *testing.T) {
	companyID := types.NewCompanyID()
	year, periods := GenerateFiscalYear(companyID, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	date := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)

	reader := overrideFiscalReader{year: year, regular: periods[1]}
	if _, err := ValidatePeriodForPosting(context.Background(), reader, companyID, date); !errors.Is(err, ErrInvalidFiscalPeriod) {
		t.Fatalf("repository period not containing the posting date error = %v, want invalid fiscal period", err)
	}

	periodOutsideYear := periods[11]
	periodOutsideYear.EndDate = year.EndDate.Add(time.Nanosecond)
	reader.regular = periodOutsideYear
	if _, err := ValidatePeriodForPosting(
		context.Background(), reader, companyID, time.Date(2026, 12, 15, 0, 0, 0, 0, time.UTC),
	); !errors.Is(err, ErrInvalidFiscalPeriod) {
		t.Fatalf("period extending beyond fiscal year error = %v, want invalid fiscal period", err)
	}

	invalidYear := year
	invalidYear.StartDate = time.Time{}
	reader.year = invalidYear
	reader.regular = periods[0]
	if _, err := ValidatePeriodForPosting(
		context.Background(), reader, companyID, time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
	); !errors.Is(err, ErrInvalidFiscalPeriod) {
		t.Fatalf("fiscal year with missing bounds error = %v, want invalid fiscal period", err)
	}
}

func TestAdjustmentPeriodMustMatchFiscalYearEnd(t *testing.T) {
	companyID := types.NewCompanyID()
	year, periods := GenerateFiscalYear(companyID, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	regularPeriod := periods[11]
	adjustmentPeriod := periods[12]
	adjustmentPeriod.StartDate = year.StartDate
	reader := overrideFiscalReader{
		year: year, regular: regularPeriod,
		adjustments: map[int]FiscalPeriod{13: adjustmentPeriod},
	}

	_, err := ValidateAdjustmentPeriodForPosting(
		context.Background(),
		reader,
		companyID,
		time.Date(2026, 12, 15, 0, 0, 0, 0, time.UTC),
		13,
	)
	if !errors.Is(err, ErrInvalidAdjustmentPeriod) {
		t.Fatalf("adjustment period with inconsistent year-end dates error = %v, want invalid adjustment period", err)
	}
}

func TestPeriodTransitions(t *testing.T) {
	if err := ValidatePeriodTransition(PeriodOpen, PeriodSoftClosed); err != nil {
		t.Fatalf("transition open -> soft_closed failed: %v", err)
	}
	if err := ValidatePeriodTransition(PeriodSoftClosed, PeriodHardClosed); err != nil {
		t.Fatalf("transition soft_closed -> hard_closed failed: %v", err)
	}
	if err := ValidatePeriodTransition(PeriodOpen, PeriodHardClosed); err == nil {
		t.Fatalf("direct transition open -> hard_closed should fail")
	}

	if err := ValidatePeriodReopen(PeriodHardClosed, PeriodOpen, "actor1", "audited reopen"); err != nil {
		t.Fatalf("valid reopen failed: %v", err)
	}
	if err := ValidatePeriodReopen(PeriodHardClosed, PeriodOpen, "", ""); err == nil {
		t.Fatalf("reopen without audit actor should fail")
	}
}

func TestCompanyScopeValidation(t *testing.T) {
	ctx := context.Background()
	reader := newMockReader()
	activeCompID := types.NewCompanyID()
	inactiveCompID := types.NewCompanyID()

	reader.companies[activeCompID] = Company{ID: activeCompID, Active: true}
	reader.companies[inactiveCompID] = Company{ID: inactiveCompID, Active: false}

	c, err := ValidateCompanyScope(ctx, reader, activeCompID)
	if err != nil || !c.Active {
		t.Fatalf("expected active company, got %v, err: %v", c, err)
	}

	_, err = ValidateCompanyScope(ctx, reader, inactiveCompID)
	if !errors.Is(err, ErrCompanyInactive) {
		t.Fatalf("expected ErrCompanyInactive, got %v", err)
	}

	otherCompanyID := types.NewCompanyID()
	reader.companies[activeCompID] = Company{ID: otherCompanyID, Active: true}
	if _, err := ValidateCompanyScope(ctx, reader, activeCompID); !errors.Is(err, ErrCompanyIdentityMismatch) {
		t.Fatalf("mismatched company identity error = %v, want identity mismatch", err)
	}
}

func TestValidateAccountScopeRejectsMismatchedRepositoryIdentity(t *testing.T) {
	companyID, accountID := types.NewCompanyID(), types.NewAccountID()
	if err := ValidateAccountScope(Account{ID: accountID, CompanyID: companyID}, companyID, accountID); err != nil {
		t.Fatalf("matching account scope: %v", err)
	}
	if err := ValidateAccountScope(Account{ID: types.NewAccountID(), CompanyID: companyID}, companyID, accountID); !errors.Is(err, ErrAccountIdentityMismatch) {
		t.Fatalf("mismatched account ID error = %v, want identity mismatch", err)
	}
	if err := ValidateAccountScope(Account{ID: accountID, CompanyID: types.NewCompanyID()}, companyID, accountID); !errors.Is(err, ErrAccountIdentityMismatch) {
		t.Fatalf("mismatched account company error = %v, want identity mismatch", err)
	}
}

func TestMasterDataEdgeCases(t *testing.T) {
	ctx := context.Background()
	reader := newMockReader()
	compID := types.NewCompanyID()

	// Inactive account check
	accID := types.NewAccountID()
	reader.accounts[accID] = Account{ID: accID, CompanyID: compID, Code: "9999", IsActive: false}
	err := ValidateCanPostToAccount(ctx, reader, compID, accID)
	if err == nil || !strings.Contains(err.Error(), "inactive") {
		t.Fatalf("expected inactive error, got %v", err)
	}

	// Account not found check
	nonExistentAccID := types.NewAccountID()
	err = ValidateCanPostToAccount(ctx, reader, compID, nonExistentAccID)
	if !errors.Is(err, types.ErrAccountNotFound) {
		t.Fatalf("expected ErrAccountNotFound, got %v", err)
	}

	// Company not found check
	_, err = ValidateCompanyScope(ctx, reader, types.NewCompanyID())
	if !errors.Is(err, ErrCompanyNotFound) {
		t.Fatalf("expected ErrCompanyNotFound, got %v", err)
	}

	// A complete hierarchy snapshot cannot silently promote a child with a missing parent.
	detachedParentID := types.NewAccountID()
	accs := []Account{
		{ID: types.NewAccountID(), Code: "2000", ParentID: &detachedParentID},
	}
	if _, err := BuildHierarchyTree(accs); !errors.Is(err, ErrInvalidParent) {
		t.Fatalf("missing hierarchy parent error = %v, want ErrInvalidParent", err)
	}

	// Test FiscalPeriod.Contains for normal vs adjustment periods
	normalPeriod := FiscalPeriod{
		StartDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, 1, 31, 23, 59, 59, 999999999, time.UTC),
		Type:      FiscalPeriodTypeNormal,
	}
	if !normalPeriod.Contains(time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("normal period should contain date within range")
	}
	if normalPeriod.Contains(time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("normal period should not contain date outside range")
	}

	adjPeriod13 := FiscalPeriod{
		StartDate: time.Date(2026, 12, 31, 23, 59, 59, 999999999, time.UTC),
		EndDate:   time.Date(2026, 12, 31, 23, 59, 59, 999999999, time.UTC),
		Type:      FiscalPeriodTypeYearEndAdjustment,
	}
	if adjPeriod13.Contains(time.Date(2026, 12, 31, 23, 59, 59, 999999999, time.UTC)) {
		t.Fatalf("adjustment period 13 must never contain date in search")
	}

	// Test ValidatePeriodForPosting rejecting closed fiscal year
	fyID := types.NewFiscalYearID()
	closedYear := FiscalYear{
		ID: fyID, CompanyID: compID, State: FiscalYearHardClosed,
		StartDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, 12, 31, 23, 59, 59, 999999999, time.UTC),
	}
	reader.fiscalYears[fyID] = closedYear
	openPeriodInClosedYear := FiscalPeriod{
		ID:           types.NewFiscalPeriodID(),
		CompanyID:    compID,
		FiscalYearID: fyID,
		State:        PeriodOpen,
		Number:       1,
		Type:         FiscalPeriodTypeNormal,
		StartDate:    time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		EndDate:      time.Date(2026, 1, 31, 23, 59, 59, 0, time.UTC),
	}
	reader.periods[compID] = []FiscalPeriod{openPeriodInClosedYear}

	_, err = ValidatePeriodForPosting(ctx, reader, compID, time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC))
	if !errors.Is(err, ErrClosedFiscalYear) {
		t.Fatalf("expected ErrClosedFiscalYear when fiscal year is closed, got %v", err)
	}

	// Test transitions and audit requirements
	if err := ValidateFiscalYearTransition(FiscalYearOpen, FiscalYearSoftClosed); err != nil {
		t.Fatalf("valid FY transition failed: %v", err)
	}
	if err := ValidateFiscalYearTransition(FiscalYearOpen, FiscalYearOpen); err == nil {
		t.Fatalf("invalid FY transition should fail")
	}
	if err := ValidateFiscalYearReopen(FiscalYearHardClosed, FiscalYearOpen, "admin", "audit reason"); err != nil {
		t.Fatalf("valid FY reopen failed: %v", err)
	}
	if err := ValidateFiscalYearReopen(FiscalYearHardClosed, FiscalYearOpen, "", ""); err == nil {
		t.Fatalf("FY reopen without actor should fail")
	}
}

func TestGenerateFiscalYearPreservesCustomStartDay(t *testing.T) {
	companyID := types.NewCompanyID()

	for _, start := range []time.Time{
		time.Date(2026, time.March, 15, 12, 30, 0, 0, time.FixedZone("test", 3*60*60)),
		time.Date(2026, time.January, 31, 0, 0, 0, 0, time.UTC),
		time.Date(2024, time.February, 29, 0, 0, 0, 0, time.UTC),
	} {
		year, periods := GenerateFiscalYear(companyID, start)
		if year.StartDate != periods[0].StartDate {
			t.Fatalf("start mismatch: year=%v period=%v", year.StartDate, periods[0].StartDate)
		}
		if len(periods) != 14 {
			t.Fatalf("period count = %d, want 14", len(periods))
		}
		if year.StartDate.Day() != start.UTC().Day() {
			t.Fatalf("year start day = %d, want %d", year.StartDate.Day(), start.UTC().Day())
		}
		for i := 0; i < 11; i++ {
			if !periods[i].EndDate.Before(periods[i+1].StartDate) {
				t.Fatalf("period %d overlaps or gaps before period %d", i+1, i+2)
			}
		}
		if !year.EndDate.Before(year.StartDate.AddDate(1, 0, 0)) {
			t.Fatalf("fiscal year end %v is not before next anniversary", year.EndDate)
		}
		if err := ValidateFiscalYear(year, periods); err != nil {
			t.Fatalf("ValidateFiscalYear failed: %v", err)
		}
	}
}

func TestValidateFiscalYear_RejectsInvalidStructures(t *testing.T) {
	companyID := types.NewCompanyID()
	year, periods := GenerateFiscalYear(companyID, time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC))

	// Success case
	if err := ValidateFiscalYear(year, periods); err != nil {
		t.Fatalf("expected valid fiscal year, got %v", err)
	}

	// Gap introduced
	tamperedPeriods := make([]FiscalPeriod, len(periods))
	copy(tamperedPeriods, periods)
	tamperedPeriods[1].StartDate = tamperedPeriods[1].StartDate.Add(time.Hour)
	if err := ValidateFiscalYear(year, tamperedPeriods); !errors.Is(err, ErrInvalidFiscalPeriod) {
		t.Fatalf("expected ErrInvalidFiscalPeriod on gap, got %v", err)
	}

	// Overlap introduced
	copy(tamperedPeriods, periods)
	tamperedPeriods[1].StartDate = tamperedPeriods[0].EndDate.Add(-time.Hour)
	if err := ValidateFiscalYear(year, tamperedPeriods); !errors.Is(err, ErrInvalidFiscalPeriod) {
		t.Fatalf("expected ErrInvalidFiscalPeriod on overlap, got %v", err)
	}

	// Wrong company ID on period
	copy(tamperedPeriods, periods)
	tamperedPeriods[2].CompanyID = types.NewCompanyID()
	if err := ValidateFiscalYear(year, tamperedPeriods); !errors.Is(err, ErrInvalidFiscalPeriod) {
		t.Fatalf("expected ErrInvalidFiscalPeriod on company mismatch, got %v", err)
	}

	// Wrong period count
	if err := ValidateFiscalYear(year, periods[:10]); !errors.Is(err, ErrInvalidFiscalPeriod) {
		t.Fatalf("expected ErrInvalidFiscalPeriod on wrong count, got %v", err)
	}
}

func TestValidateFiscalYearRejectsDuplicatePeriodIDsAndUnknownStates(t *testing.T) {
	companyID := types.NewCompanyID()
	year, periods := GenerateFiscalYear(companyID, time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC))

	for _, test := range []struct {
		name   string
		mutate func(*FiscalYear, []FiscalPeriod)
	}{
		{
			name: "duplicate normal period ids",
			mutate: func(_ *FiscalYear, values []FiscalPeriod) {
				values[1].ID = values[0].ID
			},
		},
		{
			name: "duplicate adjustment period ids",
			mutate: func(_ *FiscalYear, values []FiscalPeriod) {
				values[13].ID = values[12].ID
			},
		},
		{
			name: "unknown period state",
			mutate: func(_ *FiscalYear, values []FiscalPeriod) {
				values[13].State = "unknown"
			},
		},
		{
			name: "unknown fiscal year state",
			mutate: func(value *FiscalYear, _ []FiscalPeriod) {
				value.State = "unknown"
			},
		},
		{
			name: "period 0 start does not match year start",
			mutate: func(_ *FiscalYear, values []FiscalPeriod) {
				values[0].StartDate = values[0].StartDate.Add(time.Hour)
			},
		},
		{
			name: "period 11 end does not match year end",
			mutate: func(_ *FiscalYear, values []FiscalPeriod) {
				values[11].EndDate = values[11].EndDate.Add(-time.Hour)
			},
		},
		{
			name: "period start after end",
			mutate: func(_ *FiscalYear, values []FiscalPeriod) {
				values[0].EndDate = values[0].StartDate.Add(-time.Hour)
			},
		},
		{
			name: "period start before year start",
			mutate: func(_ *FiscalYear, values []FiscalPeriod) {
				values[0].StartDate = values[0].StartDate.Add(-24 * time.Hour)
			},
		},
		{
			name: "period 13 dates do not match year end",
			mutate: func(_ *FiscalYear, values []FiscalPeriod) {
				values[12].StartDate = values[12].StartDate.Add(-time.Hour)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			changedYear := year
			changedPeriods := append([]FiscalPeriod(nil), periods...)
			test.mutate(&changedYear, changedPeriods)
			if err := ValidateFiscalYear(changedYear, changedPeriods); !errors.Is(err, ErrInvalidFiscalPeriod) {
				t.Fatalf("ValidateFiscalYear error = %v, want ErrInvalidFiscalPeriod", err)
			}
		})
	}
}

func TestAdjustmentPeriodSoftCloseAndIdentityBranches(t *testing.T) {
	ctx := context.Background()
	companyID := types.NewCompanyID()
	yearID := types.NewFiscalYearID()
	year := FiscalYear{
		ID: yearID, CompanyID: companyID, State: FiscalYearSoftClosed,
		StartDate: time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, time.December, 31, 23, 59, 59, 999999999, time.UTC),
	}
	regular := FiscalPeriod{
		ID: types.NewFiscalPeriodID(), CompanyID: companyID, FiscalYearID: yearID,
		Number: 12, State: PeriodSoftClosed, Type: FiscalPeriodTypeNormal,
		StartDate: time.Date(2026, time.December, 1, 0, 0, 0, 0, time.UTC),
		EndDate:   year.EndDate,
	}
	adjustment13 := FiscalPeriod{
		ID: types.NewFiscalPeriodID(), CompanyID: companyID, FiscalYearID: yearID,
		Number: 13, State: PeriodSoftClosed, Type: FiscalPeriodTypeYearEndAdjustment,
		StartDate: year.EndDate, EndDate: year.EndDate,
	}
	reader := overrideFiscalReader{
		year: year, regular: regular,
		adjustments: map[int]FiscalPeriod{13: adjustment13},
	}

	if _, err := ValidateAdjustmentPeriodForPosting(ctx, reader, companyID, time.Date(2026, time.December, 31, 0, 0, 0, 0, time.UTC), 13); !errors.Is(err, ErrClosedFiscalYear) {
		t.Fatalf("closed/soft-close policy error = %v, want closed fiscal year", err)
	}
	got, err := ValidateAdjustmentPeriodForPostingWithSoftClose(
		ctx, reader, companyID, time.Date(2026, time.December, 31, 0, 0, 0, 0, time.UTC), 13, true,
	)
	if err != nil {
		t.Fatalf("valid soft-close adjustment period: %v", err)
	}
	if got.ID != adjustment13.ID {
		t.Fatalf("adjustment period ID = %s, want %s", got.ID, adjustment13.ID)
	}

	for _, test := range []struct {
		number int
		date   time.Time
	}{
		{12, time.Date(2026, time.December, 31, 0, 0, 0, 0, time.UTC)},
		{13, time.Time{}},
		{15, time.Date(2026, time.December, 31, 0, 0, 0, 0, time.UTC)},
	} {
		if _, err := ValidateAdjustmentPeriodForPosting(ctx, reader, companyID, test.date, test.number); err == nil {
			t.Errorf("invalid adjustment request (%d, %v) unexpectedly passed", test.number, test.date)
		}
	}

	// Period 14 adjustment
	adjustment14 := FiscalPeriod{
		ID: types.NewFiscalPeriodID(), CompanyID: companyID, FiscalYearID: yearID,
		Number: 14, State: PeriodOpen, Type: FiscalPeriodTypeAuditAdjustment,
		StartDate: year.EndDate, EndDate: year.EndDate,
	}
	yearOpen := year
	yearOpen.State = FiscalYearOpen
	reader14 := overrideFiscalReader{
		year: yearOpen, regular: regular,
		adjustments: map[int]FiscalPeriod{14: adjustment14},
	}
	p14Got, err := ValidateAdjustmentPeriodForPosting(ctx, reader14, companyID, time.Date(2026, time.December, 31, 0, 0, 0, 0, time.UTC), 14)
	if err != nil {
		t.Fatalf("period 14 adjustment posting: %v", err)
	}
	if p14Got.ID != adjustment14.ID {
		t.Fatalf("p14 ID = %s, want %s", p14Got.ID, adjustment14.ID)
	}
}

func TestPeriodAndFiscalYearReopenValidation(t *testing.T) {
	if err := ValidatePeriodReopen(PeriodSoftClosed, PeriodOpen, "actor", "reason"); err != nil {
		t.Fatalf("valid period reopen: %v", err)
	}
	for _, test := range []struct {
		from, to      PeriodState
		actor, reason string
	}{
		{PeriodOpen, PeriodOpen, "actor", "reason"},
		{PeriodSoftClosed, PeriodOpen, "", "reason"},
		{PeriodHardClosed, PeriodOpen, "actor", ""},
	} {
		if err := ValidatePeriodReopen(test.from, test.to, test.actor, test.reason); err == nil {
			t.Errorf("invalid period reopen unexpectedly passed: %+v", test)
		}
	}
	if err := ValidateFiscalYearReopen(FiscalYearSoftClosed, FiscalYearOpen, "actor", "reason"); err != nil {
		t.Fatalf("valid fiscal-year reopen: %v", err)
	}
	for _, test := range []struct {
		from, to      FiscalYearState
		actor, reason string
	}{
		{FiscalYearOpen, FiscalYearOpen, "actor", "reason"},
		{FiscalYearHardClosed, FiscalYearOpen, "", "reason"},
		{FiscalYearSoftClosed, FiscalYearOpen, "actor", ""},
	} {
		if err := ValidateFiscalYearReopen(test.from, test.to, test.actor, test.reason); err == nil {
			t.Errorf("invalid fiscal-year reopen unexpectedly passed: %+v", test)
		}
	}
}
