package ifrs15

import (
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/journal"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/shopspring/decimal"
)

var ErrContractNotFound = errors.New("revenue contract not found")

// zeroMoney returns a zero Money in the requested currency. A zero value is
// returned when the currency is invalid; callers only use it for balanced
// journal-line construction with otherwise validated amounts.
func zeroMoney(currency string) types.Money {
	m, err := types.NewMoney(decimal.Zero, currency)
	if err != nil {
		return types.Money{}
	}
	return m
}

func debitLine(accountID uuid.UUID, amount types.Money, partnerID *uuid.UUID) journal.EntryLine {
	line := journal.EntryLine{
		ID:                  types.NewJournalEntryLineID(),
		AccountID:           types.AccountID(accountID),
		TransactionAmount:   amount,
		TransactionCurrency: amount.Currency(),
		ExchangeRate:        decimal.NewFromInt(1),
		FunctionalDebit:     amount,
		FunctionalCredit:    zeroMoney(amount.Currency()),
	}
	if partnerID != nil {
		id := types.PartnerID(*partnerID)
		line.PartnerID = &id
	}
	return line
}

func creditLine(accountID uuid.UUID, amount types.Money, partnerID *uuid.UUID) journal.EntryLine {
	line := journal.EntryLine{
		ID:                  types.NewJournalEntryLineID(),
		AccountID:           types.AccountID(accountID),
		TransactionAmount:   amount,
		TransactionCurrency: amount.Currency(),
		ExchangeRate:        decimal.NewFromInt(1),
		FunctionalDebit:     zeroMoney(amount.Currency()),
		FunctionalCredit:    amount,
	}
	if partnerID != nil {
		id := types.PartnerID(*partnerID)
		line.PartnerID = &id
	}
	return line
}

func newJournalEntry(journalID uuid.UUID, date time.Time, description string, lines []journal.EntryLine) (journal.JournalEntry, error) {
	if journalID == uuid.Nil {
		return journal.JournalEntry{}, errors.New("journal ID is required")
	}
	if date.IsZero() {
		return journal.JournalEntry{}, errors.New("entry date is required")
	}
	if len(lines) < 2 {
		return journal.JournalEntry{}, errors.New("at least two lines are required")
	}
	debit, credit := decimal.Zero, decimal.Zero
	for _, line := range lines {
		debit = debit.Add(line.FunctionalDebit.Amount())
		credit = credit.Add(line.FunctionalCredit.Amount())
	}
	if !debit.Equal(credit) || debit.IsZero() {
		return journal.JournalEntry{}, errors.New("entry lines must be balanced")
	}
	return journal.JournalEntry{
		ID:                  types.NewJournalEntryID(),
		JournalID:           types.JournalID(journalID),
		EffectiveDate:       date.UTC(),
		PostingDate:         date.UTC(),
		Status:              journal.EntryDraft,
		Classification:      journal.EntryClassRegular,
		Description:         description,
		TransactionCurrency: lines[0].TransactionCurrency,
		ExchangeRate:        decimal.NewFromInt(1),
		Lines:               append([]journal.EntryLine(nil), lines...),
		CreatedAt:           time.Now().UTC(),
	}, nil
}

func partnerReference(partnerID uuid.UUID) *types.PartnerID {
	id := types.PartnerID(partnerID)
	return &id
}

// UnitConfig binds contract lifecycle events to the general-ledger accounts.
type UnitConfig struct {
	JournalID                uuid.UUID
	CashAccountID            uuid.UUID
	ReceivableAccountID      uuid.UUID
	ContractAssetAccountID   uuid.UUID
	ContractLiabilityAccount uuid.UUID
	RevenueAccountID         uuid.UUID
}

// Unit coordinates contract state and creates balanced journal drafts. Posting
// remains the responsibility of the central posting workflow.
type Unit struct {
	config         UnitConfig
	mu             sync.RWMutex
	contracts      map[uuid.UUID]Contract
	recognizedByPO map[uuid.UUID]map[uuid.UUID]types.Money
}

func NewUnit(config UnitConfig) (*Unit, error) {
	if config.JournalID == uuid.Nil || config.CashAccountID == uuid.Nil ||
		config.ContractAssetAccountID == uuid.Nil || config.ContractLiabilityAccount == uuid.Nil ||
		config.RevenueAccountID == uuid.Nil {
		return nil, ErrInvalidContract
	}
	return &Unit{
		config: config, contracts: make(map[uuid.UUID]Contract),
		recognizedByPO: make(map[uuid.UUID]map[uuid.UUID]types.Money),
	}, nil
}

func (u *Unit) AddContract(contract Contract) error {
	if u == nil {
		return ErrInvalidContract
	}
	if err := contract.Validate(); err != nil {
		return err
	}
	if !contract.RecognizedAmount.Amount().IsZero() {
		return ErrInvalidContract
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	if _, exists := u.contracts[contract.ID]; exists {
		return ErrInvalidContract
	}
	u.contracts[contract.ID] = contract
	u.recognizedByPO[contract.ID] = make(map[uuid.UUID]types.Money, len(contract.Obligations))
	zero, _ := types.Zero(contract.Currency)
	for _, obligation := range contract.Obligations {
		u.recognizedByPO[contract.ID][obligation.ID] = zero
	}
	return nil
}

func (u *Unit) Contract(id uuid.UUID) (Contract, bool) {
	if u == nil {
		return Contract{}, false
	}
	u.mu.RLock()
	defer u.mu.RUnlock()
	contract, ok := u.contracts[id]
	return contract, ok
}

// ReceiveAdvance records customer cash as a contract liability and returns
// the corresponding balanced journal draft.
func (u *Unit) ReceiveAdvance(contractID uuid.UUID, amount types.Money, date time.Time) (Contract, journal.JournalEntry, error) {
	if u == nil || date.IsZero() {
		return Contract{}, journal.JournalEntry{}, ErrInvalidContract
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	contract, ok := u.contracts[contractID]
	if !ok {
		return Contract{}, journal.JournalEntry{}, ErrContractNotFound
	}
	updated, err := contract.ApplyBilling(amount)
	if err != nil {
		return Contract{}, journal.JournalEntry{}, err
	}
	entry, err := buildAdvanceJournal(u.config, contract, amount, date)
	if err != nil {
		return Contract{}, journal.JournalEntry{}, err
	}
	u.contracts[contractID] = updated
	return updated, entry, nil
}

// BillCustomer records an invoice issued before satisfaction of the related
// performance obligation as a receivable and contract liability.
func (u *Unit) BillCustomer(contractID uuid.UUID, amount types.Money, date time.Time) (Contract, journal.JournalEntry, error) {
	if u == nil || date.IsZero() || u.config.ReceivableAccountID == uuid.Nil {
		return Contract{}, journal.JournalEntry{}, ErrInvalidContract
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	contract, ok := u.contracts[contractID]
	if !ok {
		return Contract{}, journal.JournalEntry{}, ErrContractNotFound
	}
	updated, err := contract.ApplyBilling(amount)
	if err != nil {
		return Contract{}, journal.JournalEntry{}, err
	}
	customerID := contract.CustomerID
	unbilledEarned := contract.RecognizedAmount.Amount().Sub(contract.BilledAmount.Amount())
	if unbilledEarned.IsNegative() {
		unbilledEarned = decimal.Zero
	}
	assetCredit := decimal.Min(amount.Amount(), unbilledEarned)
	liabilityCredit := amount.Amount().Sub(assetCredit)
	lines := []journal.EntryLine{debitLine(u.config.ReceivableAccountID, amount, &customerID)}
	if assetCredit.IsPositive() {
		assetAmount, err := types.NewMoney(assetCredit, contract.Currency)
		if err != nil {
			return Contract{}, journal.JournalEntry{}, err
		}
		lines = append(lines, creditLine(u.config.ContractAssetAccountID, assetAmount, &customerID))
	}
	if liabilityCredit.IsPositive() {
		liabilityAmount, err := types.NewMoney(liabilityCredit, contract.Currency)
		if err != nil {
			return Contract{}, journal.JournalEntry{}, err
		}
		lines = append(lines, creditLine(u.config.ContractLiabilityAccount, liabilityAmount, &customerID))
	}
	entry, err := newJournalEntry(u.config.JournalID, date, "ifrs15-billing", lines)
	if err != nil {
		return Contract{}, journal.JournalEntry{}, err
	}
	u.contracts[contractID] = updated
	return updated, entry, nil
}

// CollectReceivable applies customer cash to an invoice receivable. The
// receivable subledger remains responsible for allocation to specific invoices.
func (u *Unit) CollectReceivable(contractID uuid.UUID, amount types.Money, date time.Time) (journal.JournalEntry, error) {
	if u == nil || date.IsZero() || u.config.ReceivableAccountID == uuid.Nil ||
		amount.Currency() == "" || !amount.Amount().IsPositive() {
		return journal.JournalEntry{}, ErrInvalidContract
	}
	u.mu.RLock()
	contract, ok := u.contracts[contractID]
	u.mu.RUnlock()
	if !ok {
		return journal.JournalEntry{}, ErrContractNotFound
	}
	if amount.Currency() != contract.Currency {
		return journal.JournalEntry{}, ErrInvalidAmount
	}
	customerID := contract.CustomerID
	return newJournalEntry(u.config.JournalID, date, "ifrs15-receivable-collection", []journal.EntryLine{
		debitLine(u.config.CashAccountID, amount, &customerID),
		creditLine(u.config.ReceivableAccountID, amount, &customerID),
	})
}

// RecognizeObligation records a milestone or goods-delivery event against a
// specific performance obligation and prevents recognition beyond its
// allocated transaction price.
func (u *Unit) RecognizeObligation(
	contractID, obligationID uuid.UUID,
	amount types.Money,
	date time.Time,
) (Contract, journal.JournalEntry, error) {
	if u == nil {
		return Contract{}, journal.JournalEntry{}, ErrInvalidContract
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	contract, ok := u.contracts[contractID]
	if !ok {
		return Contract{}, journal.JournalEntry{}, ErrContractNotFound
	}
	allocated, ok := contract.AllocatedPrices[obligationID]
	if !ok {
		return Contract{}, journal.JournalEntry{}, ErrObligationNotFound
	}
	recognized := u.recognizedByPO[contractID][obligationID]
	next, err := recognized.Add(amount)
	if err != nil || next.GreaterThan(allocated) {
		return Contract{}, journal.JournalEntry{}, ErrRecognitionExceeded
	}
	entry, err := BuildRecognitionJournal(contract, amount, u.config.JournalID, date,
		u.config.RevenueAccountID, u.config.ContractLiabilityAccount, u.config.ContractAssetAccountID)
	if err != nil {
		return Contract{}, journal.JournalEntry{}, err
	}
	partner := partnerReference(contract.CustomerID)
	for i := range entry.Lines {
		entry.Lines[i].PartnerID = partner
	}
	updated, err := contract.ApplyRecognition(amount)
	if err != nil {
		return Contract{}, journal.JournalEntry{}, err
	}
	u.contracts[contractID] = updated
	u.recognizedByPO[contractID][obligationID] = next
	return updated, entry, nil
}

func (u *Unit) applyRecognitionToObligations(contract Contract, amount types.Money) {
	remaining := amount
	for _, obligation := range contract.Obligations {
		if !remaining.Amount().IsPositive() {
			return
		}
		recognized := u.recognizedByPO[contract.ID][obligation.ID]
		allocated := contract.AllocatedPrices[obligation.ID]
		available, _ := allocated.Subtract(recognized)
		applied := remaining
		if available.LessThan(applied) {
			applied = available
		}
		updated, _ := recognized.Add(applied)
		u.recognizedByPO[contract.ID][obligation.ID] = updated
		remaining, _ = remaining.Subtract(applied)
	}
}

// Recognize records revenue for a satisfied performance obligation. Previously
// billed consideration is released from deferred revenue; any unbilled earned
// amount is debited to the contract asset.
func (u *Unit) Recognize(contractID uuid.UUID, amount types.Money, date time.Time) (Contract, journal.JournalEntry, error) {
	if u == nil {
		return Contract{}, journal.JournalEntry{}, ErrInvalidContract
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	contract, ok := u.contracts[contractID]
	if !ok {
		return Contract{}, journal.JournalEntry{}, ErrContractNotFound
	}
	entry, err := BuildRecognitionJournal(contract, amount, u.config.JournalID, date,
		u.config.RevenueAccountID, u.config.ContractLiabilityAccount, u.config.ContractAssetAccountID)
	if err != nil {
		return Contract{}, journal.JournalEntry{}, err
	}
	partner := partnerReference(contract.CustomerID)
	for i := range entry.Lines {
		entry.Lines[i].PartnerID = partner
	}
	updated, err := contract.ApplyRecognition(amount)
	if err != nil {
		return Contract{}, journal.JournalEntry{}, err
	}
	u.contracts[contractID] = updated
	u.applyRecognitionToObligations(contract, amount)
	return updated, entry, nil
}

func buildAdvanceJournal(config UnitConfig, contract Contract, amount types.Money, date time.Time) (journal.JournalEntry, error) {
	if amount.Currency() != contract.Currency || !amount.Amount().IsPositive() {
		return journal.JournalEntry{}, ErrInvalidAmount
	}
	customerID := contract.CustomerID
	debit := debitLine(config.CashAccountID, amount, &customerID)
	credit := creditLine(config.ContractLiabilityAccount, amount, &customerID)
	return newJournalEntry(config.JournalID, date, "ifrs15-advance", []journal.EntryLine{debit, credit})
}
