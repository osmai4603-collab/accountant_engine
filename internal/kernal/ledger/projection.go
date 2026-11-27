package ledger

import (
	"fmt"
	"sort"
	"strings"
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

// ForeignCurrencyBalanceFilter narrows RebuildForeignCurrencyBalances by tenant,
// account, transaction currency, posting cutoff, and analytical dimensions (LDG-003).
type ForeignCurrencyBalanceFilter struct {
	CompanyID           types.CompanyID
	AccountID           types.AccountID
	TransactionCurrency string
	AsOf                time.Time
	PartnerID           *types.PartnerID
	CostCenterID        *types.CostCenterID
	ProjectID           *types.ProjectID
	SegmentID           *types.SegmentID
	BusinessUnitID      *types.BusinessUnitID
	GroupByDimensions   bool
}

// ForeignCurrencyBalanceKey identifies a deterministic foreign-currency rebuild bucket.
type ForeignCurrencyBalanceKey struct {
	CompanyID           types.CompanyID
	AccountID           types.AccountID
	TransactionCurrency string
	FunctionalCurrency  string
	PartnerID           types.PartnerID
	CostCenterID        types.CostCenterID
	ProjectID           types.ProjectID
	SegmentID           types.SegmentID
	BusinessUnitID      types.BusinessUnitID
}

// ForeignCurrencyBalance is a deterministic read projection pairing functional
// carrying-book totals with the signed transaction-currency balance required
// for IAS 21 end-of-period FX revaluation (LDG-003 / DEC-LDG-001).
type ForeignCurrencyBalance struct {
	CompanyID           types.CompanyID
	AccountID           types.AccountID
	TransactionCurrency string
	FunctionalCurrency  string
	PartnerID           *types.PartnerID
	CostCenterID        *types.CostCenterID
	ProjectID           *types.ProjectID
	SegmentID           *types.SegmentID
	BusinessUnitID      *types.BusinessUnitID
	TransactionDebit    decimal.Decimal
	TransactionCredit   decimal.Decimal
	TransactionNet      decimal.Decimal
	FunctionalDebit     types.Money
	FunctionalCredit    types.Money
	FunctionalNet       types.Money
	MovementCount       int
}

// UnsignedTransactionPosition converts the signed TransactionNet read balance
// into a strictly non-negative magnitude and explicit normal-side indicator
// so FX revaluation generators preserve JRN-003 (positive EntryLine amounts).
func (b ForeignCurrencyBalance) UnsignedTransactionPosition() (magnitude decimal.Decimal, isCredit bool) {
	if b.TransactionNet.IsNegative() {
		return b.TransactionNet.Abs(), true
	}
	return b.TransactionNet, false
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
				PartnerID:           clonePointer(line.PartnerID),
				CostCenterID:        clonePointer(line.CostCenterID),
				ProjectID:           clonePointer(line.ProjectID),
				SegmentID:           clonePointer(line.SegmentID),
				BusinessUnitID:      clonePointer(line.BusinessUnitID),
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
				ReversalOfID:        clonePointer(entry.ReversalOfID),
				ReversedByID:        clonePointer(entry.ReversedByID),
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

// RebuildForeignCurrencyBalances deterministically projects functional carrying
// balances alongside transaction-currency balances filtered by asOf, transaction
// currency, and optional analytical dimensions in total canonical order (LDG-003).
func RebuildForeignCurrencyBalances(
	entries []journal.JournalEntry,
	filter ForeignCurrencyBalanceFilter,
) (map[ForeignCurrencyBalanceKey]ForeignCurrencyBalance, error) {
	cutoffUTC := filter.AsOf.UTC()
	hasCutoff := !filter.AsOf.IsZero()
	filteredEntries := make([]journal.JournalEntry, 0, len(entries))
	for _, entry := range entries {
		if hasCutoff {
			ts := entry.PostingDate.UTC()
			if ts.IsZero() {
				ts = entry.EffectiveDate.UTC()
			}
			if ts.After(cutoffUTC) {
				continue
			}
		}
		filteredEntries = append(filteredEntries, entry)
	}

	movements, err := ProjectPostedMovements(filteredEntries)
	if err != nil {
		return nil, err
	}

	wantCurrency := strings.ToUpper(strings.TrimSpace(filter.TransactionCurrency))
	balances := make(map[ForeignCurrencyBalanceKey]ForeignCurrencyBalance)
	for _, mv := range movements {
		txCurrency := strings.ToUpper(strings.TrimSpace(mv.TransactionCurrency))
		if txCurrency == "" {
			txCurrency = mv.FunctionalCurrency
		}
		if !movementMatchesFilter(mv, filter, wantCurrency, txCurrency) {
			continue
		}

		key := ForeignCurrencyBalanceKey{
			CompanyID:           mv.CompanyID,
			AccountID:           mv.AccountID,
			TransactionCurrency: txCurrency,
			FunctionalCurrency:  mv.FunctionalCurrency,
		}
		if filter.GroupByDimensions {
			key.PartnerID = derefOrZero(mv.PartnerID)
			key.CostCenterID = derefOrZero(mv.CostCenterID)
			key.ProjectID = derefOrZero(mv.ProjectID)
			key.SegmentID = derefOrZero(mv.SegmentID)
			key.BusinessUnitID = derefOrZero(mv.BusinessUnitID)
		}

		balance, ok := balances[key]
		if !ok {
			zeroFunc, err := types.Zero(mv.FunctionalCurrency)
			if err != nil {
				return nil, err
			}
			balance = ForeignCurrencyBalance{
				CompanyID:           mv.CompanyID,
				AccountID:           mv.AccountID,
				TransactionCurrency: txCurrency,
				FunctionalCurrency:  mv.FunctionalCurrency,
				FunctionalDebit:     zeroFunc,
				FunctionalCredit:    zeroFunc,
				FunctionalNet:       zeroFunc,
			}
			if filter.GroupByDimensions {
				balance.PartnerID = clonePointer(mv.PartnerID)
				balance.CostCenterID = clonePointer(mv.CostCenterID)
				balance.ProjectID = clonePointer(mv.ProjectID)
				balance.SegmentID = clonePointer(mv.SegmentID)
				balance.BusinessUnitID = clonePointer(mv.BusinessUnitID)
			}
		}

		funcDebit, err := balance.FunctionalDebit.Add(mv.FunctionalDebit)
		if err != nil {
			return nil, err
		}
		funcCredit, err := balance.FunctionalCredit.Add(mv.FunctionalCredit)
		if err != nil {
			return nil, err
		}
		funcNet, err := funcDebit.Subtract(funcCredit)
		if err != nil {
			return nil, err
		}

		txAbs := mv.TransactionAmount.Amount().Abs()
		if mv.FunctionalDebit.Amount().IsPositive() {
			balance.TransactionDebit = balance.TransactionDebit.Add(txAbs)
		}
		if mv.FunctionalCredit.Amount().IsPositive() {
			balance.TransactionCredit = balance.TransactionCredit.Add(txAbs)
		}
		balance.TransactionNet = balance.TransactionDebit.Sub(balance.TransactionCredit)
		balance.FunctionalDebit = funcDebit
		balance.FunctionalCredit = funcCredit
		balance.FunctionalNet = funcNet
		balance.MovementCount++
		balances[key] = balance
	}
	return balances, nil
}

// SortedForeignCurrencyBalances returns a deterministically ordered slice from a
// foreign-currency balance rebuild map.
func SortedForeignCurrencyBalances(balances map[ForeignCurrencyBalanceKey]ForeignCurrencyBalance) []ForeignCurrencyBalance {
	keys := make([]ForeignCurrencyBalanceKey, 0, len(balances))
	for k := range balances {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := keys[i], keys[j]
		if a.CompanyID != b.CompanyID {
			return a.CompanyID.String() < b.CompanyID.String()
		}
		if a.AccountID != b.AccountID {
			return a.AccountID.String() < b.AccountID.String()
		}
		if a.TransactionCurrency != b.TransactionCurrency {
			return a.TransactionCurrency < b.TransactionCurrency
		}
		if a.FunctionalCurrency != b.FunctionalCurrency {
			return a.FunctionalCurrency < b.FunctionalCurrency
		}
		if a.PartnerID != b.PartnerID {
			return a.PartnerID.String() < b.PartnerID.String()
		}
		if a.CostCenterID != b.CostCenterID {
			return a.CostCenterID.String() < b.CostCenterID.String()
		}
		if a.ProjectID != b.ProjectID {
			return a.ProjectID.String() < b.ProjectID.String()
		}
		if a.SegmentID != b.SegmentID {
			return a.SegmentID.String() < b.SegmentID.String()
		}
		return a.BusinessUnitID.String() < b.BusinessUnitID.String()
	})
	out := make([]ForeignCurrencyBalance, len(keys))
	for i, k := range keys {
		out[i] = balances[k]
	}
	return out
}

func movementMatchesFilter(
	mv PostedMovement,
	filter ForeignCurrencyBalanceFilter,
	wantCurrency string,
	txCurrency string,
) bool {
	if filter.CompanyID != (types.CompanyID{}) && mv.CompanyID != filter.CompanyID {
		return false
	}
	if filter.AccountID != (types.AccountID{}) && mv.AccountID != filter.AccountID {
		return false
	}
	if wantCurrency != "" && txCurrency != wantCurrency {
		return false
	}
	if !matchOptionalID(mv.PartnerID, filter.PartnerID) ||
		!matchOptionalID(mv.CostCenterID, filter.CostCenterID) ||
		!matchOptionalID(mv.ProjectID, filter.ProjectID) ||
		!matchOptionalID(mv.SegmentID, filter.SegmentID) ||
		!matchOptionalID(mv.BusinessUnitID, filter.BusinessUnitID) {
		return false
	}
	return true
}

func matchOptionalID[T comparable](actual, filter *T) bool {
	if filter == nil {
		return true
	}
	if actual == nil {
		return false
	}
	return *actual == *filter
}

func derefOrZero[T comparable](v *T) T {
	if v == nil {
		var zero T
		return zero
	}
	return *v
}

func clonePointer[T any](v *T) *T {
	if v == nil {
		return nil
	}
	clone := *v
	return &clone
}
