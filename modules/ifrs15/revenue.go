package ifrs15

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/journal"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/shopspring/decimal"
)

var (
	ErrInvalidContract     = errors.New("invalid revenue contract")
	ErrInvalidObligation   = errors.New("invalid performance obligation")
	ErrInvalidAmount       = errors.New("invalid revenue amount")
	ErrInvalidPeriod       = errors.New("invalid recognition period")
	ErrObligationNotFound  = errors.New("performance obligation not found")
	ErrRecognitionExceeded = errors.New("recognized revenue exceeds transaction price")
)

type RecognitionPattern string

const (
	PointInTime RecognitionPattern = "point_in_time"
	OverTime    RecognitionPattern = "over_time"
)

type ScheduleStatus string

const (
	SchedulePending    ScheduleStatus = "pending"
	ScheduleRecognized ScheduleStatus = "recognized"
	ScheduleDeferred   ScheduleStatus = "deferred"
)

type ScheduleFrequency string

const (
	Daily   ScheduleFrequency = "daily"
	Monthly ScheduleFrequency = "monthly"
)

type PerformanceObligation struct {
	ID                 uuid.UUID
	Description        string
	StandalonePrice    types.Money
	RecognitionPattern RecognitionPattern
	StartDate          time.Time
	EndDate            time.Time
}

type Contract struct {
	ID               uuid.UUID                 `json:"id"`
	CustomerID       uuid.UUID                 `json:"customer_id"`
	Currency         string                    `json:"currency"`
	TransactionPrice types.Money               `json:"transaction_price"`
	Obligations      []PerformanceObligation   `json:"obligations"`
	AllocatedPrices  map[uuid.UUID]types.Money `json:"allocated_prices"`
	BilledAmount     types.Money               `json:"billed_amount"`
	RecognizedAmount types.Money               `json:"recognized_amount"`
	Status           ContractStatus            `json:"status,omitempty"`
	RecognizedPrices map[uuid.UUID]types.Money `json:"recognized_prices,omitempty"`
}

type RevenueScheduleEntry struct {
	ContractID              uuid.UUID
	PerformanceObligationID uuid.UUID
	RecognitionDate         time.Time
	AmountToRecognize       types.Money
	Status                  ScheduleStatus
}

// NewContract validates the five-step inputs and allocates transaction price
// using the relative standalone selling prices of its obligations.
func NewContract(id, customerID uuid.UUID, transactionPrice types.Money, obligations []PerformanceObligation) (Contract, error) {
	if id == uuid.Nil || customerID == uuid.Nil || transactionPrice.Currency() == "" || !transactionPrice.Amount().IsPositive() {
		return Contract{}, ErrInvalidContract
	}
	var err error
	transactionPrice, err = transactionPrice.Rounded()
	if err != nil || !transactionPrice.Amount().IsPositive() {
		return Contract{}, ErrInvalidAmount
	}
	if len(obligations) == 0 {
		return Contract{}, ErrInvalidObligation
	}
	currency := transactionPrice.Currency()
	seen := make(map[uuid.UUID]struct{}, len(obligations))
	standaloneTotal := decimal.Zero
	for _, obligation := range obligations {
		if obligation.ID == uuid.Nil || obligation.Description == "" {
			return Contract{}, ErrInvalidObligation
		}
		if _, exists := seen[obligation.ID]; exists {
			return Contract{}, fmt.Errorf("%w: duplicate obligation ID", ErrInvalidObligation)
		}
		seen[obligation.ID] = struct{}{}
		if obligation.StandalonePrice.Currency() != currency ||
			!obligation.StandalonePrice.Amount().IsPositive() {
			return Contract{}, ErrInvalidObligation
		}
		if obligation.RecognitionPattern != PointInTime && obligation.RecognitionPattern != OverTime {
			return Contract{}, ErrInvalidObligation
		}
		if obligation.RecognitionPattern == OverTime &&
			(dateOnly(obligation.StartDate).After(dateOnly(obligation.EndDate)) ||
				obligation.StartDate.IsZero() || obligation.EndDate.IsZero()) {
			return Contract{}, ErrInvalidPeriod
		}
		if obligation.RecognitionPattern == PointInTime && obligation.StartDate.IsZero() {
			return Contract{}, ErrInvalidPeriod
		}
		standaloneTotal = standaloneTotal.Add(obligation.StandalonePrice.Amount())
	}

	allocations, err := allocate(transactionPrice, obligations, standaloneTotal)
	if err != nil {
		return Contract{}, err
	}
	zero, err := types.Zero(currency)
	if err != nil {
		return Contract{}, err
	}
	copied := append([]PerformanceObligation(nil), obligations...)
	recognizedPrices := make(map[uuid.UUID]types.Money, len(obligations))
	for _, obligation := range obligations {
		recognizedPrices[obligation.ID] = zero
	}
	return Contract{
		ID:               id,
		CustomerID:       customerID,
		Currency:         currency,
		TransactionPrice: transactionPrice,
		Obligations:      copied,
		AllocatedPrices:  allocations,
		BilledAmount:     zero,
		RecognizedAmount: zero,
		Status:           ContractStatusActive,
		RecognizedPrices: recognizedPrices,
	}, nil
}

func (c Contract) Validate() error {
	if c.ID == uuid.Nil || c.CustomerID == uuid.Nil || c.Currency == "" ||
		c.TransactionPrice.Currency() != c.Currency || !c.TransactionPrice.Amount().IsPositive() ||
		len(c.Obligations) == 0 || len(c.AllocatedPrices) != len(c.Obligations) {
		return ErrInvalidContract
	}
	if c.BilledAmount.Currency() != c.Currency || c.RecognizedAmount.Currency() != c.Currency ||
		c.BilledAmount.IsNegative() || c.RecognizedAmount.IsNegative() {
		return ErrInvalidContract
	}
	total := decimal.Zero
	seen := make(map[uuid.UUID]struct{}, len(c.Obligations))
	for _, obligation := range c.Obligations {
		if obligation.ID == uuid.Nil || obligation.Description == "" ||
			obligation.StandalonePrice.Currency() != c.Currency ||
			!obligation.StandalonePrice.Amount().IsPositive() {
			return ErrInvalidObligation
		}
		if obligation.RecognitionPattern != PointInTime && obligation.RecognitionPattern != OverTime {
			return ErrInvalidObligation
		}
		if obligation.RecognitionPattern == OverTime &&
			(obligation.StartDate.IsZero() || obligation.EndDate.IsZero() ||
				dateOnly(obligation.StartDate).After(dateOnly(obligation.EndDate))) {
			return ErrInvalidPeriod
		}
		if obligation.RecognitionPattern == PointInTime && obligation.StartDate.IsZero() {
			return ErrInvalidPeriod
		}
		if _, exists := seen[obligation.ID]; exists {
			return ErrInvalidObligation
		}
		seen[obligation.ID] = struct{}{}
		allocated, ok := c.AllocatedPrices[obligation.ID]
		if !ok || allocated.Currency() != c.Currency || allocated.IsNegative() {
			return ErrInvalidContract
		}
		total = total.Add(allocated.Amount())
	}
	if !total.Equal(c.TransactionPrice.Amount().RoundBank(types.CurrencyMinorUnit(c.Currency))) {
		return ErrInvalidContract
	}
	if c.BilledAmount.GreaterThan(c.TransactionPrice) ||
		c.RecognizedAmount.GreaterThan(c.TransactionPrice) {
		return ErrRecognitionExceeded
	}
	if c.Status != "" && c.Status != ContractStatusDraft &&
		c.Status != ContractStatusActive && c.Status != ContractStatusModified &&
		c.Status != ContractStatusSatisfied {
		return ErrInvalidContract
	}
	return nil
}

// AllocateTransactionPrice exposes relative-SSP allocation for callers that
// need to preview the five-step allocation before creating a contract.
func AllocateTransactionPrice(transactionPrice types.Money, obligations []PerformanceObligation) (map[uuid.UUID]types.Money, error) {
	if transactionPrice.Currency() == "" || !transactionPrice.Amount().IsPositive() || len(obligations) == 0 {
		return nil, ErrInvalidAmount
	}
	var err error
	transactionPrice, err = transactionPrice.Rounded()
	if err != nil || !transactionPrice.Amount().IsPositive() {
		return nil, ErrInvalidAmount
	}
	total := decimal.Zero
	seen := make(map[uuid.UUID]struct{}, len(obligations))
	for _, obligation := range obligations {
		if obligation.ID == uuid.Nil || obligation.StandalonePrice.Currency() != transactionPrice.Currency() ||
			!obligation.StandalonePrice.Amount().IsPositive() {
			return nil, ErrInvalidObligation
		}
		if _, ok := seen[obligation.ID]; ok {
			return nil, ErrInvalidObligation
		}
		seen[obligation.ID] = struct{}{}
		total = total.Add(obligation.StandalonePrice.Amount())
	}
	return allocate(transactionPrice, obligations, total)
}

type allocationRemainder struct {
	index    int
	fraction decimal.Decimal
}

func allocate(price types.Money, obligations []PerformanceObligation, sspTotal decimal.Decimal) (map[uuid.UUID]types.Money, error) {
	unit := decimal.New(1, -types.CurrencyMinorUnit(price.Currency()))
	totalUnits := price.Amount().Div(unit)
	if !totalUnits.IsPositive() || sspTotal.IsZero() {
		return nil, ErrInvalidAmount
	}
	units := make([]decimal.Decimal, len(obligations))
	remainders := make([]allocationRemainder, len(obligations))
	allocatedUnits := decimal.Zero
	for i, obligation := range obligations {
		exact := totalUnits.Mul(obligation.StandalonePrice.Amount()).Div(sspTotal)
		units[i] = exact.Floor()
		allocatedUnits = allocatedUnits.Add(units[i])
		remainders[i] = allocationRemainder{index: i, fraction: exact.Sub(units[i])}
	}
	sort.SliceStable(remainders, func(i, j int) bool {
		return remainders[i].fraction.GreaterThan(remainders[j].fraction)
	})
	left := totalUnits.Sub(allocatedUnits).IntPart()
	for i := int64(0); i < left; i++ {
		units[remainders[i%int64(len(remainders))].index] =
			units[remainders[i%int64(len(remainders))].index].Add(decimal.NewFromInt(1))
	}

	result := make(map[uuid.UUID]types.Money, len(obligations))
	for i, obligation := range obligations {
		amount, err := types.NewMoney(units[i].Mul(unit), price.Currency())
		if err != nil {
			return nil, err
		}
		result[obligation.ID] = amount
	}
	return result, nil
}

// Schedule builds a dated recognition schedule for one obligation of the
// contract. The exported BuildRecognitionSchedule in schedule.go carries the
// same algorithm for callers that already hold the obligation.
func (c Contract) Schedule(obligationID uuid.UUID, frequency ScheduleFrequency) ([]RevenueScheduleEntry, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	var obligation *PerformanceObligation
	for i := range c.Obligations {
		if c.Obligations[i].ID == obligationID {
			obligation = &c.Obligations[i]
			break
		}
	}
	if obligation == nil {
		return nil, ErrObligationNotFound
	}
	return BuildRecognitionSchedule(c, *obligation, frequency)
}

func UpdateScheduleStatus(entries []RevenueScheduleEntry, throughDate time.Time) []RevenueScheduleEntry {
	result := append([]RevenueScheduleEntry(nil), entries...)
	throughDate = dateOnly(throughDate)
	for i := range result {
		if !dateOnly(result[i].RecognitionDate).After(throughDate) {
			result[i].Status = ScheduleRecognized
		} else {
			result[i].Status = ScheduleDeferred
		}
	}
	return result
}

func (c Contract) ApplyBilling(amount types.Money) (Contract, error) {
	if err := c.validateAmount(amount); err != nil {
		return Contract{}, err
	}
	billed, err := c.BilledAmount.Add(amount)
	if err != nil {
		return Contract{}, err
	}
	if billed.GreaterThan(c.TransactionPrice) {
		return Contract{}, ErrInvalidAmount
	}
	c.BilledAmount = billed
	return c, nil
}

func (c Contract) ApplyRecognition(amount types.Money) (Contract, error) {
	if err := c.validateAmount(amount); err != nil {
		return Contract{}, err
	}
	recognized, err := c.RecognizedAmount.Add(amount)
	if err != nil {
		return Contract{}, err
	}
	if recognized.GreaterThan(c.TransactionPrice) {
		return Contract{}, ErrRecognitionExceeded
	}
	c.RecognizedAmount = recognized
	if c.RecognizedAmount.Equal(c.TransactionPrice) {
		c.Status = ContractStatusSatisfied
	}
	return c, nil
}

func (c Contract) ContractBalances() (asset, liability types.Money, err error) {
	if err = c.Validate(); err != nil {
		return types.Money{}, types.Money{}, err
	}
	difference, err := c.RecognizedAmount.Subtract(c.BilledAmount)
	if err != nil {
		return types.Money{}, types.Money{}, err
	}
	zero, err := types.Zero(c.Currency)
	if err != nil {
		return types.Money{}, types.Money{}, err
	}
	if difference.IsNegative() {
		liability, multiplyErr := difference.Multiply(decimal.NewFromInt(-1))
		return zero, liability, multiplyErr
	}
	return difference, zero, nil
}

func (c Contract) validateAmount(amount types.Money) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if amount.Currency() != c.Currency || !amount.Amount().IsPositive() {
		return ErrInvalidAmount
	}
	return nil
}

// BuildRecognitionJournal debits deferred revenue first and records any
// unbilled earned portion as a contract asset, with earned revenue credited.
func BuildRecognitionJournal(c Contract, amount types.Money, journalID uuid.UUID, date time.Time, revenueAccount, contractLiabilityAccount, contractAssetAccount uuid.UUID) (journal.JournalEntry, error) {
	if err := c.validateAmount(amount); err != nil {
		return journal.JournalEntry{}, err
	}
	if _, err := c.ApplyRecognition(amount); err != nil {
		return journal.JournalEntry{}, err
	}
	if journalID == uuid.Nil || revenueAccount == uuid.Nil || contractLiabilityAccount == uuid.Nil ||
		contractAssetAccount == uuid.Nil || date.IsZero() {
		return journal.JournalEntry{}, ErrInvalidContract
	}
	liability := c.BilledAmount.Amount().Sub(c.RecognizedAmount.Amount())
	if liability.IsNegative() {
		liability = decimal.Zero
	}
	deferredDebit := decimal.Min(liability, amount.Amount())
	assetDebit := amount.Amount().Sub(deferredDebit)
	lines := make([]journal.EntryLine, 0, 3)
	if deferredDebit.IsPositive() {
		debit, err := types.NewMoney(deferredDebit, c.Currency)
		if err != nil {
			return journal.JournalEntry{}, err
		}
		lines = append(lines, debitLine(contractLiabilityAccount, debit, nil))
	}
	if assetDebit.IsPositive() {
		debit, err := types.NewMoney(assetDebit, c.Currency)
		if err != nil {
			return journal.JournalEntry{}, err
		}
		lines = append(lines, debitLine(contractAssetAccount, debit, nil))
	}
	credit, err := types.NewMoney(amount.Amount(), c.Currency)
	if err != nil {
		return journal.JournalEntry{}, err
	}
	lines = append(lines, creditLine(revenueAccount, credit, nil))
	return newJournalEntry(journalID, date, "ifrs15", lines)
}

func dateOnly(value time.Time) time.Time {
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)
}

func cloneContract(contract Contract) Contract {
	contract.Obligations = append([]PerformanceObligation(nil), contract.Obligations...)
	allocations := make(map[uuid.UUID]types.Money, len(contract.AllocatedPrices))
	for id, amount := range contract.AllocatedPrices {
		allocations[id] = amount
	}
	contract.AllocatedPrices = allocations
	if contract.RecognizedPrices != nil {
		rec := make(map[uuid.UUID]types.Money, len(contract.RecognizedPrices))
		for id, amount := range contract.RecognizedPrices {
			rec[id] = amount
		}
		contract.RecognizedPrices = rec
	}
	return contract
}
