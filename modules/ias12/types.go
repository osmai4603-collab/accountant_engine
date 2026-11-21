package ias12

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/journal"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/reporting"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/shopspring/decimal"
)

var (
	ErrInvalidInput             = errors.New("invalid IAS 12 input")
	ErrInvalidRate              = errors.New("invalid enacted tax rate")
	ErrRateNotEnacted           = errors.New("tax rate was not enacted by the measurement date")
	ErrCurrencyMismatch         = errors.New("IAS 12 amounts use different currencies")
	ErrNoBalances               = errors.New("at least one tax balance is required")
	ErrNoDeferredTaxMovement    = errors.New("no deferred tax movement to post")
	ErrInvalidMeasurementPeriod = errors.New("measurement period must be quarterly or annual with an end date")

	ErrInvalidTaxProfile    = errors.New("invalid IAS 12 tax profile")
	ErrMissingTaxProvenance = errors.New("IAS 12 tax data provenance is required")
	ErrMissingDTAEvidence   = errors.New("probable-profit evidence is required for deductible differences")
	ErrRateUnavailable      = errors.New("no enacted tax rate is effective on the required date")
	ErrPostingNotConfigured = errors.New("IAS 12 posting requires a configured application and journal mapping")

	ErrInvalidUncertainTaxTreatment = errors.New("invalid uncertain tax treatment")
	ErrInvalidTaxOutcomes           = errors.New("invalid uncertain tax outcomes")
	ErrReassessmentNotRequired      = errors.New("uncertain tax treatment has no reassessment trigger")

	ErrInvalidTaxReconciliationDisclosure = errors.New("invalid IAS 12 tax reconciliation disclosure")
)

const (
	TaxReconciliationDisclosureCode    = "IAS12-TAX-RATE-RECONCILIATION"
	TaxReconciliationDisclosureVersion = "1.0.0"
	TaxReconciliationStandardReference = "IAS 12"
)

type BalanceKind string

const (
	Asset     BalanceKind = "asset"
	Liability BalanceKind = "liability"
)

type PeriodKind string

const (
	Quarterly PeriodKind = "quarterly"
	Annual    PeriodKind = "annual"
)

type MeasurementPeriod struct {
	Kind PeriodKind
	AsOf time.Time
}

// TaxRate.Value is a decimal fraction (0.25 means 25%); zero represents an enacted 0% rate.
type TaxRate struct {
	Value           decimal.Decimal
	Enacted         bool
	EnactedOn       time.Time
	EffectiveFrom   time.Time
	SourceReference string
}

type TaxBalance struct {
	ID                   string
	Kind                 BalanceKind
	CarryingAmount       types.Money
	TaxBase              types.Money
	Rate                 TaxRate
	CarryingAmountSource string
	TaxBaseSource        string
	ExpectedReversalDate time.Time
	RecognitionEvidence  *DTARecognitionEvidence
	// Controls recognition of a DTA for this deductible temporary difference
	// based on probable future taxable profit. Nil does not establish
	// probability, so the DTA is not recognized.
	FutureTaxableProfitProbable *bool
}

type DTARecognitionEvidence struct {
	Reference                 string
	AssessedOn                time.Time
	Probable                  bool
	SupportedDeductibleAmount types.Money
}

type MeasuredBalance struct {
	ID                            string
	Kind                          BalanceKind
	TaxableTemporaryDifference    types.Money
	DeductibleTemporaryDifference types.Money
	DTA                           types.Money
	DTL                           types.Money
	Rate                          TaxRate
	ExpectedReversalDate          time.Time
	CarryingAmountSource          string
	TaxBaseSource                 string
	RecognitionEvidenceReference  string
}

type Measurement struct {
	Period                 MeasurementPeriod
	Currency               string
	Balances               []MeasuredBalance
	DTA                    types.Money
	DTL                    types.Money
	TaxProfileID           string
	CompanyID              uuid.UUID
	Jurisdiction           string
	ProfileVersion         string
	ProfileSourceReference string
}

type PermanentDifference struct {
	Description string
	// Positive amounts increase taxable profit; negative amounts reduce it.
	Amount types.Money
}

// TaxReconciliationInput expects CurrentTaxExpense and DeferredTaxExpense to
// include the recognized effects relevant to the period. UncertainTaxTreatments
// are supplementary disclosure detail and are not added to those totals.
type TaxReconciliationInput struct {
	Period                    MeasurementPeriod
	AccountingProfitBeforeTax types.Money
	StatutoryRate             TaxRate
	PermanentDifferences      []PermanentDifference
	CurrentTaxExpense         types.Money
	DeferredTaxExpense        types.Money
	UncertainTaxTreatments    []UncertainTaxMeasurement
}

type TaxReconciliationReport struct {
	Period                     MeasurementPeriod
	AccountingProfitBeforeTax  types.Money
	StatutoryTaxRate           decimal.Decimal
	ExpectedTaxExpense         types.Money
	PermanentDifferencesTax    types.Money
	PermanentDifferences       []TaxReconciliationLine
	CurrentTaxExpense          types.Money
	DeferredTaxExpense         types.Money
	UncertainTaxTreatments     []UncertainTaxMeasurement
	TemporaryDifferencesChange types.Money
	EffectiveTaxExpense        types.Money
	EffectiveTaxRate           decimal.Decimal
	EffectiveTaxRateAvailable  bool
}

type TaxReconciliationLine struct {
	Description string
	Amount      types.Money
	TaxEffect   types.Money
}

type DeferredTaxPosition struct {
	DTA types.Money
	DTL types.Money
}

type DeferredTaxAccounts struct {
	Expense uuid.UUID
	DTA     uuid.UUID
	DTL     uuid.UUID
	OCI     uuid.UUID
}

type DeferredTaxEntryInput struct {
	CompanyID uuid.UUID
	JournalID uuid.UUID
	EntryDate time.Time
	CreatedBy string
	Previous  DeferredTaxPosition
	Closing   DeferredTaxPosition
	OCIChange DeferredTaxPosition
	Accounts  DeferredTaxAccounts
}

type CurrentTaxEntryInput struct {
	CompanyID, JournalID, ExpenseAccountID, PayableAccountID uuid.UUID
	EntryDate                                                time.Time
	CreatedBy                                                string
	Amount                                                   types.Money
}

// TaxProfile is an explicitly scoped, versioned set of company/jurisdiction tax
// rates. It is not a tax-law engine; callers remain responsible for selecting
// the applicable local law and classifying tax bases.
type TaxProfile struct {
	ID              string
	CompanyID       uuid.UUID
	Jurisdiction    string
	Currency        string
	Version         string
	EffectiveFrom   time.Time
	SourceReference string
	Rates           []TaxRate
}

type CurrentTaxInput struct {
	Period          MeasurementPeriod
	TaxableProfit   types.Money
	SourceReference string
}

type CurrentTaxMeasurement struct {
	ProfileID              string
	CompanyID              uuid.UUID
	Jurisdiction           string
	ProfileVersion         string
	ProfileSourceReference string
	Period                 MeasurementPeriod
	TaxableProfit          types.Money
	Rate                   TaxRate
	TaxExpense             types.Money
	SourceReference        string
}

// ConfiguredPosting is host-supplied evidence that a real journal mapping and
// application posting workflow are configured. This package creates drafts
// only; it does not authorize or persist entries.
type ConfiguredPosting struct {
	CompanyID                         uuid.UUID
	JournalMappingReference           string
	ApplicationConfigurationReference string
	CurrentExpenseAccountID           uuid.UUID
	CurrentPayableAccountID           uuid.UUID
	DeferredAccounts                  DeferredTaxAccounts
}

type TaxUnitOfAccount string

const (
	IndividualTaxUnit TaxUnitOfAccount = "individual"
	GroupTaxUnit      TaxUnitOfAccount = "group"
)

type UncertaintyMeasurementMethod string

const (
	MostLikelyAmount UncertaintyMeasurementMethod = "most_likely_amount"
	ExpectedValue    UncertaintyMeasurementMethod = "expected_value"
)

type TaxOutcome struct {
	ID string
	// TaxAmount is signed: positive is a liability and negative is a tax asset.
	TaxAmount       types.Money
	Probability     decimal.Decimal
	SourceReference string
}

// UncertainTaxTreatment records the host's tax analysis and the evidence
// needed to apply IFRIC 23. It is not a tax-law or legal-opinion engine.
type UncertainTaxTreatment struct {
	ID                                string
	Description                       string
	UnitOfAccount                     TaxUnitOfAccount
	UnitOfAccountReference            string
	UnitOfAccountRationale            string
	ExaminationAssumptionAcknowledged bool
	AcceptanceProbable                bool
	AssessmentDate                    time.Time
	AssessmentReference               string
	ReportedTaxAmount                 types.Money
	Method                            UncertaintyMeasurementMethod
	MethodRationale                   string
	Outcomes                          []TaxOutcome
	FactsAndCircumstancesReference    string
	LimitationDate                    time.Time
	ReassessmentReason                string
}

type UncertainTaxMeasurement struct {
	TreatmentID                    string
	Description                    string
	UnitOfAccount                  TaxUnitOfAccount
	UnitOfAccountReference         string
	AcceptanceProbable             bool
	Method                         UncertaintyMeasurementMethod
	ReportedTaxAmount              types.Money
	RecognizedTaxAmount            types.Money
	UncertaintyAdjustment          types.Money
	LiabilityIncrease              types.Money
	LiabilityDecrease              types.Money
	TaxAssetIncrease               types.Money
	TaxAssetDecrease               types.Money
	SelectedOutcomeID              string
	AssessmentDate                 time.Time
	AssessmentReference            string
	FactsAndCircumstancesReference string
	LimitationDate                 time.Time
	ReassessmentDue                bool
}

type TaxTreatmentReassessment struct {
	PreviousTreatment   UncertainTaxTreatment
	UpdatedTreatment    UncertainTaxTreatment
	PreviousMeasurement UncertainTaxMeasurement
	UpdatedMeasurement  UncertainTaxMeasurement
	Reason              string
}

type TaxReconciliationDisclosureInput struct {
	CompanyID        types.CompanyID
	PeriodStart      time.Time
	PeriodEnd        time.Time
	Report           TaxReconciliationReport
	SourceReferences []reporting.DisclosureSource
	Narrative        string
}

type TaxReconciliationDisclosureProvider struct {
	disclosures []reporting.StructuredDisclosure
}

// Unit exposes IAS 12 measurement, disclosure reconciliation, and journal
// generation as one optional standards service.
type Unit struct{}

func NewUnit() *Unit { return &Unit{} }

func (u *Unit) Measure(balances []TaxBalance, period MeasurementPeriod) (Measurement, error) {
	if u == nil {
		return Measurement{}, ErrInvalidInput
	}
	return Measure(balances, period)
}

func (u *Unit) MeasureWithProfile(profile TaxProfile, balances []TaxBalance, period MeasurementPeriod) (Measurement, error) {
	if u == nil {
		return Measurement{}, ErrInvalidInput
	}
	return MeasureWithProfile(profile, balances, period)
}

func (u *Unit) MeasureCurrentTax(profile TaxProfile, input CurrentTaxInput) (CurrentTaxMeasurement, error) {
	if u == nil {
		return CurrentTaxMeasurement{}, ErrInvalidInput
	}
	return MeasureCurrentTax(profile, input)
}

func (u *Unit) Reconcile(input TaxReconciliationInput) (TaxReconciliationReport, error) {
	if u == nil {
		return TaxReconciliationReport{}, ErrInvalidInput
	}
	return Reconcile(input)
}

func (u *Unit) MeasureUncertainTaxTreatment(
	treatment UncertainTaxTreatment,
	asOf time.Time,
) (UncertainTaxMeasurement, error) {
	if u == nil {
		return UncertainTaxMeasurement{}, ErrInvalidInput
	}
	return MeasureUncertainTaxTreatment(treatment, asOf)
}

func (u *Unit) ReassessUncertainTaxTreatment(
	previous UncertainTaxTreatment,
	updated UncertainTaxTreatment,
	asOf time.Time,
	reason string,
) (TaxTreatmentReassessment, error) {
	if u == nil {
		return TaxTreatmentReassessment{}, ErrInvalidInput
	}
	return ReassessUncertainTaxTreatment(previous, updated, asOf, reason)
}

func (u *Unit) GenerateConfiguredDeferredTaxEntry(
	measurement Measurement,
	prior DeferredTaxPosition,
	config ConfiguredPosting,
	journalID uuid.UUID,
	createdBy string,
) (journal.JournalEntry, error) {
	if u == nil {
		return journal.JournalEntry{}, ErrInvalidInput
	}
	return GenerateConfiguredDeferredTaxEntry(measurement, prior, config, journalID, createdBy)
}

func (u *Unit) GenerateConfiguredCurrentTaxEntry(
	measurement CurrentTaxMeasurement,
	config ConfiguredPosting,
	journalID uuid.UUID,
	createdBy string,
) (journal.JournalEntry, error) {
	if u == nil {
		return journal.JournalEntry{}, ErrInvalidInput
	}
	return GenerateConfiguredCurrentTaxEntry(measurement, config, journalID, createdBy)
}

func (u *Unit) GenerateDeferredTaxEntry(input DeferredTaxEntryInput) (journal.JournalEntry, error) {
	if u == nil {
		return journal.JournalEntry{}, ErrInvalidInput
	}
	return GenerateDeferredTaxEntry(input)
}

func (u *Unit) GenerateCurrentTaxEntry(input CurrentTaxEntryInput) (journal.JournalEntry, error) {
	if u == nil {
		return journal.JournalEntry{}, ErrInvalidInput
	}
	return GenerateCurrentTaxEntry(input)
}

// ClosePeriod measures balances, generates a deferred-tax journal draft, and
// returns both outputs for validation before central posting.
func (u *Unit) ClosePeriod(
	balances []TaxBalance,
	period MeasurementPeriod,
	prior DeferredTaxPosition,
	accounts DeferredTaxAccounts,
	companyID, journalID uuid.UUID,
	createdBy string,
) (Measurement, journal.JournalEntry, error) {
	if u == nil {
		return Measurement{}, journal.JournalEntry{}, ErrInvalidInput
	}
	measurement, err := Measure(balances, period)
	if err != nil {
		return Measurement{}, journal.JournalEntry{}, err
	}
	if prior.DTA.Currency() == "" || prior.DTL.Currency() == "" {
		zero, zeroErr := types.Zero(measurement.Currency)
		if zeroErr != nil {
			return Measurement{}, journal.JournalEntry{}, zeroErr
		}
		if prior.DTA.Currency() == "" {
			prior.DTA = zero
		}
		if prior.DTL.Currency() == "" {
			prior.DTL = zero
		}
	}
	entry, err := GenerateDeferredTaxEntry(DeferredTaxEntryInput{
		Previous: prior, Closing: DeferredTaxPosition{DTA: measurement.DTA, DTL: measurement.DTL},
		Accounts: accounts, CompanyID: companyID, JournalID: journalID,
		EntryDate: period.AsOf, CreatedBy: createdBy,
	})
	if err != nil {
		return Measurement{}, journal.JournalEntry{}, err
	}
	return measurement, entry, nil
}

// zeroMoney returns a zero Money in the supplied currency.
func zeroMoney(currency string) types.Money {
	m, err := types.NewMoney(decimal.Zero, currency)
	if err != nil {
		return types.Money{}
	}
	return m
}

func debitLine(accountID uuid.UUID, amount types.Money, partnerID *uuid.UUID) journal.EntryLine {
	line := journal.EntryLine{
		ID: types.NewJournalEntryLineID(), AccountID: types.AccountID(accountID),
		TransactionAmount: amount, TransactionCurrency: amount.Currency(),
		ExchangeRate: decimal.NewFromInt(1), FunctionalDebit: amount,
		FunctionalCredit: zeroMoney(amount.Currency()),
	}
	if partnerID != nil {
		id := types.PartnerID(*partnerID)
		line.PartnerID = &id
	}
	return line
}

func creditLine(accountID uuid.UUID, amount types.Money, partnerID *uuid.UUID) journal.EntryLine {
	line := journal.EntryLine{
		ID: types.NewJournalEntryLineID(), AccountID: types.AccountID(accountID),
		TransactionAmount: amount, TransactionCurrency: amount.Currency(),
		ExchangeRate: decimal.NewFromInt(1), FunctionalDebit: zeroMoney(amount.Currency()),
		FunctionalCredit: amount,
	}
	if partnerID != nil {
		id := types.PartnerID(*partnerID)
		line.PartnerID = &id
	}
	return line
}

// newJournalEntry builds a draft journal.JournalEntry directly from balanced
// public journal lines. It replaces the internal posting factory in the
// reference implementation.
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
	for _, l := range lines {
		debit = debit.Add(l.FunctionalDebit.Amount())
		credit = credit.Add(l.FunctionalCredit.Amount())
	}
	if !debit.Equal(credit) || debit.IsZero() {
		return journal.JournalEntry{}, errors.New("entry lines must be balanced")
	}
	return journal.JournalEntry{
		ID: types.NewJournalEntryID(), JournalID: types.JournalID(journalID),
		EffectiveDate: date.UTC(), PostingDate: date.UTC(), Status: journal.EntryDraft,
		Classification: journal.EntryClassRegular, Description: description,
		TransactionCurrency: lines[0].TransactionCurrency, ExchangeRate: decimal.NewFromInt(1),
		Lines: append([]journal.EntryLine(nil), lines...), CreatedAt: time.Now().UTC(),
	}, nil
}
