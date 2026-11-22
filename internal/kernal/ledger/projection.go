package ledger

import (
	"fmt"
	"sort"
	"time"

	"github.com/osmai4603-collab/accounting_engine/internal/kernal/journal"
	"github.com/osmai4603-collab/accounting_engine/internal/kernal/types"
	"github.com/shopspring/decimal"
)

// PostedMovement is a deterministic read projection derived from a posted
// journal entry line. It is not a stored independent entity and must not be
// treated as a second source of truth. The canonical accounting fact remains
// the posted JournalEntry with its EntryLines (ADR P1-02).
type PostedMovement struct {
	CompanyID           types.CompanyID
	JournalID           types.JournalID
	FiscalPeriodID      types.FiscalPeriodID
	EntryID             types.JournalEntryID
	LineID              types.JournalEntryLineID
	EntryNumber         string
	SequenceNumber      int64
	EffectiveDate       time.Time
	PostingDate         time.Time
	AccountID           types.AccountID
	PartnerID           *types.PartnerID
	CostCenterID        *types.CostCenterID
	ProjectID           *types.ProjectID
	SegmentID           *types.SegmentID
	BusinessUnitID      *types.BusinessUnitID
	TransactionAmount   types.Money
	TransactionCurrency string
	ExchangeRate        decimal.Decimal
	FunctionalDebit     types.Money
	FunctionalCredit    types.Money
	FunctionalCurrency  string
	Classification      journal.EntryClassification
	SourceModule        string
	ReferenceID         string
	Status              journal.EntryStatus
	ReversalOfID        *types.JournalEntryID
	ReversedByID        *types.JournalEntryID
}

// AccountBalanceKey identifies a deterministic rebuild balance bucket.
type AccountBalanceKey struct {
	CompanyID          types.CompanyID
	AccountID          types.AccountID
	FunctionalCurrency string
}

// AccountBalance is a deterministic projection rebuilt from posted movements.
// It is not a materialized balance store.
type AccountBalance struct {
	CompanyID          types.CompanyID
	AccountID          types.AccountID
	FunctionalCurrency string
	Debit              types.Money
	Credit             types.Money
	Net                types.Money
	MovementCount      int
}

// IsPostedAccountingFact reports whether the entry status participates in the
// canonical posted ledger projection.
func IsPostedAccountingFact(status journal.EntryStatus) bool {
	return status == journal.EntryPosted || status == journal.EntryReversed
}

// ComparePostedEntries defines the canonical deterministic ordering for
// rebuilding GL projections: EffectiveDate UTC, SequenceNumber,
// JournalEntryID.String(), then caller may sort lines by EntryLine.ID.String().
func ComparePostedEntries(a, b journal.JournalEntry) int {
	aDate := a.EffectiveDate.UTC()
	bDate := b.EffectiveDate.UTC()
	if !aDate.Equal(bDate) {
		if aDate.Before(bDate) {
			return -1
		}
		return 1
	}
	if a.SequenceNumber != b.SequenceNumber {
		if a.SequenceNumber < b.SequenceNumber {
			return -1
		}
		return 1
	}
	aID := a.ID.String()
	bID := b.ID.String()
	switch {
	case aID < bID:
		return -1
	case aID > bID:
		return 1
	default:
		return 0
	}
}

// CompareEntryLines defines deterministic line ordering within a posted entry.
func CompareEntryLines(a, b journal.EntryLine) int {
	aID := a.ID.String()
	bID := b.ID.String()
	switch {
	case aID < bID:
		return -1
	case aID > bID:
		return 1
	default:
		return 0
	}
}

// ProjectPostedMovements returns deterministic posted movement projections
// derived from canonical posted journal entries. The input slice is not mutated.
func ProjectPostedMovements(entries []journal.JournalEntry) ([]PostedMovement, error) {
	posted := make([]journal.JournalEntry, 0, len(entries))
	for _, entry := range entries {
		if !IsPostedAccountingFact(entry.Status) {
			continue
		}
		if entry.CompanyID == (types.CompanyID{}) || entry.ID == (types.JournalEntryID{}) {
			return nil, fmt.Errorf("posted journal entry missing company or entry id")
		}
		posted = append(posted, entry.Clone())
	}

	sort.Slice(posted, func(i, j int) bool {
		return ComparePostedEntries(posted[i], posted[j]) < 0
	})

	movements := make([]PostedMovement, 0)
	for _, entry := range posted {
		lines := append([]journal.EntryLine(nil), entry.Lines...)
		sort.Slice(lines, func(i, j int) bool {
			return CompareEntryLines(lines[i], lines[j]) < 0
		})
		for _, line := range lines {
			if line.ID == (types.JournalEntryLineID{}) {
				return nil, fmt.Errorf("posted journal entry %s contains line without id", entry.ID)
			}
			if line.AccountID == (types.AccountID{}) {
				return nil, fmt.Errorf("posted journal entry %s line %s missing account id", entry.ID, line.ID)
			}
			functionalCurrency := line.FunctionalDebit.Currency()
			if functionalCurrency == "" {
				functionalCurrency = line.FunctionalCredit.Currency()
			}
			if functionalCurrency == "" {
				return nil, fmt.Errorf("posted journal entry %s line %s missing functional currency", entry.ID, line.ID)
			}
			movements = append(movements, PostedMovement{
				CompanyID:           entry.CompanyID,
				JournalID:           entry.JournalID,
				FiscalPeriodID:      entry.FiscalPeriodID,
				EntryID:             entry.ID,
				LineID:              line.ID,
				EntryNumber:         entry.EntryNumber,
				SequenceNumber:      entry.SequenceNumber,
				EffectiveDate:       entry.EffectiveDate.UTC(),
				PostingDate:         entry.PostingDate.UTC(),
				AccountID:           line.AccountID,
				PartnerID:           clonePartnerID(line.PartnerID),
				CostCenterID:        cloneCostCenterID(line.CostCenterID),
				ProjectID:           cloneProjectID(line.ProjectID),
				SegmentID:           cloneSegmentID(line.SegmentID),
				BusinessUnitID:      cloneBusinessUnitID(line.BusinessUnitID),
				TransactionAmount:   line.TransactionAmount,
				TransactionCurrency: line.TransactionCurrency,
				ExchangeRate:        line.ExchangeRate,
				FunctionalDebit:     line.FunctionalDebit,
				FunctionalCredit:    line.FunctionalCredit,
				FunctionalCurrency:  functionalCurrency,
				Classification:      entry.Classification,
				SourceModule:        entry.SourceModule,
				ReferenceID:         entry.ReferenceID,
				Status:              entry.Status,
				ReversalOfID:        cloneJournalEntryID(entry.ReversalOfID),
				ReversedByID:        cloneJournalEntryID(entry.ReversedByID),
			})
		}
	}
	return movements, nil
}

// RebuildAccountBalances deterministically rebuilds account balances from
// canonical posted journal entries. It stores no materialized state and returns
// a new map on every call.
func RebuildAccountBalances(entries []journal.JournalEntry) (map[AccountBalanceKey]AccountBalance, error) {
	movements, err := ProjectPostedMovements(entries)
	if err != nil {
		return nil, err
	}
	balances := make(map[AccountBalanceKey]AccountBalance)
	for _, mv := range movements {
		key := AccountBalanceKey{
			CompanyID:          mv.CompanyID,
			AccountID:          mv.AccountID,
			FunctionalCurrency: mv.FunctionalCurrency,
		}
		balance, ok := balances[key]
		if !ok {
			zero, err := types.Zero(mv.FunctionalCurrency)
			if err != nil {
				return nil, err
			}
			balance = AccountBalance{
				CompanyID:          mv.CompanyID,
				AccountID:          mv.AccountID,
				FunctionalCurrency: mv.FunctionalCurrency,
				Debit:              zero,
				Credit:             zero,
				Net:                zero,
			}
		}
		debit, err := balance.Debit.Add(mv.FunctionalDebit)
		if err != nil {
			return nil, err
		}
		credit, err := balance.Credit.Add(mv.FunctionalCredit)
		if err != nil {
			return nil, err
		}
		net, err := debit.Subtract(credit)
		if err != nil {
			return nil, err
		}
		balance.Debit = debit
		balance.Credit = credit
		balance.Net = net
		balance.MovementCount++
		balances[key] = balance
	}
	return balances, nil
}

func cloneJournalEntryID(v *types.JournalEntryID) *types.JournalEntryID {
	if v == nil {
		return nil
	}
	clone := *v
	return &clone
}

func clonePartnerID(v *types.PartnerID) *types.PartnerID {
	if v == nil {
		return nil
	}
	clone := *v
	return &clone
}

func cloneCostCenterID(v *types.CostCenterID) *types.CostCenterID {
	if v == nil {
		return nil
	}
	clone := *v
	return &clone
}

func cloneProjectID(v *types.ProjectID) *types.ProjectID {
	if v == nil {
		return nil
	}
	clone := *v
	return &clone
}

func cloneSegmentID(v *types.SegmentID) *types.SegmentID {
	if v == nil {
		return nil
	}
	clone := *v
	return &clone
}

func cloneBusinessUnitID(v *types.BusinessUnitID) *types.BusinessUnitID {
	if v == nil {
		return nil
	}
	clone := *v
	return &clone
}
