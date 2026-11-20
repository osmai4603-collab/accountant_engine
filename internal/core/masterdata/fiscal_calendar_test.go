package masterdata

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/masterdata"
)

type mockMasterDataReader struct {
	companies   map[types.CompanyID]masterdata.Company
	accounts    map[types.AccountID]masterdata.Account
	children    map[types.AccountID]bool
	periods     map[types.CompanyID][]masterdata.FiscalPeriod
	fiscalYears map[types.FiscalYearID]masterdata.FiscalYear
}

func newMockReader() *mockMasterDataReader {
	return &mockMasterDataReader{
		companies:   make(map[types.CompanyID]masterdata.Company),
		accounts:    make(map[types.AccountID]masterdata.Account),
		children:    make(map[types.AccountID]bool),
		periods:     make(map[types.CompanyID][]masterdata.FiscalPeriod),
		fiscalYears: make(map[types.FiscalYearID]masterdata.FiscalYear),
	}
}

func (m *mockMasterDataReader) GetCompany(ctx context.Context, companyID types.CompanyID) (masterdata.Company, error) {
	c, ok := m.companies[companyID]
	if !ok {
		return masterdata.Company{}, masterdata.ErrCompanyNotFound
	}
	return c, nil
}

func (m *mockMasterDataReader) GetAccount(ctx context.Context, companyID types.CompanyID, accountID types.AccountID) (masterdata.Account, error) {
	a, ok := m.accounts[accountID]
	if !ok || a.CompanyID != companyID {
		return masterdata.Account{}, types.ErrAccountNotFound
	}
	return a, nil
}

func (m *mockMasterDataReader) GetAccountByCode(ctx context.Context, companyID types.CompanyID, code string) (masterdata.Account, error) {
	for _, a := range m.accounts {
		if a.CompanyID == companyID && a.Code == code {
			return a, nil
		}
	}
	return masterdata.Account{}, types.ErrAccountNotFound
}

func (m *mockMasterDataReader) GetJournalByCode(ctx context.Context, companyID types.CompanyID, code string) (masterdata.Journal, error) {
	return masterdata.Journal{}, masterdata.ErrJournalNotFound
}

func (m *mockMasterDataReader) HasAccountChildren(ctx context.Context, companyID types.CompanyID, accountID types.AccountID) (bool, error) {
	return m.children[accountID], nil
}

func (m *mockMasterDataReader) GetFiscalYear(ctx context.Context, companyID types.CompanyID, fiscalYearID types.FiscalYearID) (masterdata.FiscalYear, error) {
	fy, ok := m.fiscalYears[fiscalYearID]
	if !ok || fy.CompanyID != companyID {
		return masterdata.FiscalYear{}, errors.New("fiscal year not found")
	}
	return fy, nil
}

func (m *mockMasterDataReader) GetPeriodForDate(ctx context.Context, companyID types.CompanyID, date time.Time) (masterdata.FiscalPeriod, error) {
	for _, p := range m.periods[companyID] {
		if (date.Equal(p.StartDate) || date.After(p.StartDate)) && (date.Equal(p.EndDate) || date.Before(p.EndDate)) {
			return p, nil
		}
	}
	return masterdata.FiscalPeriod{}, types.ErrFiscalPeriodNotFound
}

func TestCOAAndPostingValidation(t *testing.T) {
	ctx := context.Background()
	reader := newMockReader()
	compID := types.NewCompanyID()
	reader.companies[compID] = masterdata.Company{ID: compID, Active: true}

	// 5 level hierarchy test
	level1ID := types.NewAccountID()
	level2ID := types.NewAccountID()
	level3ID := types.NewAccountID()
	level4ID := types.NewAccountID()
	level5ID := types.NewAccountID()

	accs := []masterdata.Account{
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
	cyclicAccs := []masterdata.Account{
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
	reader.companies[compID] = masterdata.Company{ID: compID, Active: true}

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
			reader.periods[compID][i].State = masterdata.PeriodHardClosed
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

func TestPeriodTransitions(t *testing.T) {
	if err := ValidatePeriodTransition(masterdata.PeriodOpen, masterdata.PeriodSoftClosed); err != nil {
		t.Fatalf("transition open -> soft_closed failed: %v", err)
	}
	if err := ValidatePeriodTransition(masterdata.PeriodSoftClosed, masterdata.PeriodHardClosed); err != nil {
		t.Fatalf("transition soft_closed -> hard_closed failed: %v", err)
	}
	if err := ValidatePeriodTransition(masterdata.PeriodOpen, masterdata.PeriodHardClosed); err == nil {
		t.Fatalf("direct transition open -> hard_closed should fail")
	}

	if err := ValidatePeriodReopen(masterdata.PeriodHardClosed, masterdata.PeriodOpen, "actor1", "audited reopen"); err != nil {
		t.Fatalf("valid reopen failed: %v", err)
	}
	if err := ValidatePeriodReopen(masterdata.PeriodHardClosed, masterdata.PeriodOpen, "", ""); err == nil {
		t.Fatalf("reopen without audit actor should fail")
	}
}

func TestCompanyScopeValidation(t *testing.T) {
	ctx := context.Background()
	reader := newMockReader()
	activeCompID := types.NewCompanyID()
	inactiveCompID := types.NewCompanyID()

	reader.companies[activeCompID] = masterdata.Company{ID: activeCompID, Active: true}
	reader.companies[inactiveCompID] = masterdata.Company{ID: inactiveCompID, Active: false}

	c, err := ValidateCompanyScope(ctx, reader, activeCompID)
	if err != nil || !c.Active {
		t.Fatalf("expected active company, got %v, err: %v", c, err)
	}

	_, err = ValidateCompanyScope(ctx, reader, inactiveCompID)
	if !errors.Is(err, ErrCompanyInactive) {
		t.Fatalf("expected ErrCompanyInactive, got %v", err)
	}
}


func TestMasterDataEdgeCases(t *testing.T) {
	ctx := context.Background()
	reader := newMockReader()
	compID := types.NewCompanyID()

	// Inactive account check
	accID := types.NewAccountID()
	reader.accounts[accID] = masterdata.Account{ID: accID, CompanyID: compID, Code: "9999", IsActive: false}
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
	if !errors.Is(err, masterdata.ErrCompanyNotFound) {
		t.Fatalf("expected ErrCompanyNotFound, got %v", err)
	}

	// Tree with detached parent falling back to root
	detachedParentID := types.NewAccountID()
	accs := []masterdata.Account{
		{ID: types.NewAccountID(), Code: "2000", ParentID: &detachedParentID},
	}
	tree, err := BuildHierarchyTree(accs)
	if err != nil || len(tree) != 1 {
		t.Fatalf("expected fallback to root for missing parent in batch, got tree=%v, err=%v", tree, err)
	}
}
