package ifrs9

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/journal"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/shopspring/decimal"
)

var (
	ErrInvalidConfiguration           = errors.New("invalid IFRS 9 ECL configuration")
	ErrInvalidExposure                = errors.New("invalid receivable exposure")
	ErrMissingRate                    = errors.New("missing provision matrix rate")
	ErrInvalidRate                    = errors.New("invalid provision matrix rate")
	ErrInvalidAllowance               = errors.New("invalid existing allowance")
	ErrInvalidDate                    = errors.New("invalid ECL measurement date")
	ErrDependenciesRequired           = errors.New("ECL data sources are required")
	ErrHistoricalAllowanceUnavailable = errors.New("historical allowance is unavailable")
)

type Stage string

const (
	Stage1 Stage = "stage_1"
	Stage2 Stage = "stage_2"
	Stage3 Stage = "stage_3"
)

// AgingBucket is the local, public aging classification used by the simplified
// provision matrix. The reference project used internal/subledgers buckets;
// standard modules cannot import that package.
type AgingBucket string

const (
	AgingCurrent AgingBucket = "current"
	Aging1To30   AgingBucket = "1_to_30"
	Aging31To60  AgingBucket = "31_to_60"
	Aging61To90  AgingBucket = "61_to_90"
	AgingOver90  AgingBucket = "over_90"
)

// Receivable is the host-supplied open receivable used by the simplified ECL
// calculation. Domain identifiers remain uuid.UUID; journal and company IDs
// are converted at the posting boundary.
type Receivable struct {
	ID             uuid.UUID
	CompanyID      uuid.UUID
	PartnerID      *uuid.UUID
	OriginalAmount types.Money
	ResidualAmount types.Money
	DueDate        time.Time
	Closed         bool
}

type Exposure struct {
	Receivable Receivable
	SICR       bool
	Defaulted  bool
}

type Config struct {
	CompanyID           uuid.UUID
	JournalID           uuid.UUID
	ImpairmentAccountID uuid.UUID
	AllowanceAccountID  uuid.UUID
	Currency            string
	ProvisionMatrix     map[AgingBucket]decimal.Decimal
}

type PortfolioSource interface {
	ListReceivables(context.Context, uuid.UUID) ([]Exposure, error)
}

type AllowanceSource interface {
	ExistingAllowance(context.Context, uuid.UUID, uuid.UUID, string) (types.Money, error)
}

type AsOfAllowanceSource interface {
	ExistingAllowanceAsOf(context.Context, uuid.UUID, uuid.UUID, string, time.Time) (types.Money, error)
}

type Engine struct {
	config    Config
	portfolio PortfolioSource
	allowance AllowanceSource
}

type ReceivableResult struct {
	ReceivableID uuid.UUID
	Stage        Stage
	DaysPastDue  int
	AgingBucket  AgingBucket
	ECL          types.Money
}

type CalculationResult struct {
	AsOf              time.Time
	Receivables       []ReceivableResult
	RequiredAllowance types.Money
	ExistingAllowance types.Money
	Adjustment        types.Money
	JournalEntry      *journal.JournalEntry
}

func NewEngine(config Config, portfolio PortfolioSource, allowance AllowanceSource) (*Engine, error) {
	var err error
	config, err = normalizeConfig(config)
	if err != nil {
		return nil, err
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if portfolio == nil || allowance == nil {
		return nil, ErrDependenciesRequired
	}
	return &Engine{config: config, portfolio: portfolio, allowance: allowance}, nil
}

func (c Config) Validate() error {
	if c.CompanyID == uuid.Nil || c.JournalID == uuid.Nil ||
		c.ImpairmentAccountID == uuid.Nil || c.AllowanceAccountID == uuid.Nil ||
		c.ImpairmentAccountID == c.AllowanceAccountID ||
		strings.TrimSpace(c.Currency) == "" {
		return ErrInvalidConfiguration
	}
	if _, err := types.Zero(c.Currency); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidConfiguration, err)
	}
	for _, bucket := range allAgingBuckets {
		rate, ok := c.ProvisionMatrix[bucket]
		if !ok {
			return fmt.Errorf("%w: %s", ErrMissingRate, bucket)
		}
		if rate.IsNegative() || rate.GreaterThan(decimal.NewFromInt(1)) {
			return fmt.Errorf("%w: %s", ErrInvalidRate, bucket)
		}
	}
	for bucket := range c.ProvisionMatrix {
		if !validAgingBucket(bucket) {
			return fmt.Errorf("%w: unknown aging bucket %q", ErrInvalidConfiguration, bucket)
		}
	}
	return nil
}

func normalizeConfig(config Config) (Config, error) {
	currency, err := types.NewMoney(decimal.Zero, config.Currency)
	if err != nil {
		return Config{}, fmt.Errorf("%w: %v", ErrInvalidConfiguration, err)
	}
	config.Currency = currency.Currency()
	return config, nil
}

func (e *Engine) Calculate(ctx context.Context, asOf time.Time) (CalculationResult, error) {
	if e == nil || e.portfolio == nil || e.allowance == nil {
		return CalculationResult{}, ErrDependenciesRequired
	}
	if asOf.IsZero() {
		return CalculationResult{}, ErrInvalidDate
	}
	if err := ctx.Err(); err != nil {
		return CalculationResult{}, err
	}
	exposures, err := e.portfolio.ListReceivables(ctx, e.config.CompanyID)
	if err != nil {
		return CalculationResult{}, err
	}
	var existing types.Money
	if source, ok := e.allowance.(AsOfAllowanceSource); ok {
		existing, err = source.ExistingAllowanceAsOf(
			ctx, e.config.CompanyID, e.config.AllowanceAccountID, e.config.Currency, asOf,
		)
	} else if dateOnlyUTC(asOf).Before(dateOnlyUTC(time.Now())) {
		return CalculationResult{}, ErrHistoricalAllowanceUnavailable
	} else {
		existing, err = e.allowance.ExistingAllowance(
			ctx, e.config.CompanyID, e.config.AllowanceAccountID, e.config.Currency,
		)
	}
	if err != nil {
		return CalculationResult{}, err
	}
	return Calculate(asOf, e.config, exposures, existing)
}

// Unit is the simplified IFRS 9 provision-matrix unit for trade receivables.
// The general PD/LGD/EAD method is exposed separately by CalculateGeneralECL.
type Unit struct{ engine *Engine }

func NewUnit(config Config, portfolio PortfolioSource, allowance AllowanceSource) (*Unit, error) {
	engine, err := NewEngine(config, portfolio, allowance)
	if err != nil {
		return nil, err
	}
	return &Unit{engine: engine}, nil
}

// NewGeneralUnit creates a unit instance for caller-supplied PD/LGD models.
func NewGeneralUnit() *Unit { return &Unit{} }

func (u *Unit) CalculateGeneralECL(
	asOf time.Time,
	exposures []GeneralExposure,
	pdModel PDModel,
	lgdModel LGDModel,
	eirDiscount EIRDiscount,
) (GeneralCalculationResult, error) {
	if u == nil {
		return GeneralCalculationResult{}, ErrDependenciesRequired
	}
	return CalculateGeneralECL(asOf, exposures, pdModel, lgdModel, eirDiscount)
}

func (u *Unit) Calculate(ctx context.Context, asOf time.Time) (CalculationResult, error) {
	if u == nil || u.engine == nil {
		return CalculationResult{}, ErrDependenciesRequired
	}
	return u.engine.Calculate(ctx, asOf)
}

// CalculateSimplifiedECL calculates trade receivable losses using the
// lifetime provision matrix required by the simplified approach.
func CalculateSimplifiedECL(asOf time.Time, matrix map[AgingBucket]decimal.Decimal, receivables []Exposure) (CalculationResult, error) {
	if asOf.IsZero() {
		return CalculationResult{}, ErrInvalidDate
	}
	if len(receivables) == 0 {
		return CalculationResult{AsOf: asOf.UTC()}, nil
	}
	currency := ""
	var companyID uuid.UUID
	for _, exposure := range receivables {
		line := exposure.Receivable
		if currency == "" {
			currency, companyID = line.ResidualAmount.Currency(), line.CompanyID
		} else if currency != line.ResidualAmount.Currency() || companyID != line.CompanyID {
			return CalculationResult{}, ErrInvalidExposure
		}
	}
	if currency == "" {
		return CalculationResult{}, ErrInvalidConfiguration
	}
	config := Config{CompanyID: companyID, JournalID: uuid.New(), ImpairmentAccountID: uuid.New(),
		AllowanceAccountID: uuid.New(), Currency: currency, ProvisionMatrix: matrix}
	zero, err := types.Zero(currency)
	if err != nil {
		return CalculationResult{}, err
	}
	result, err := Calculate(asOf, config, receivables, zero)
	if err != nil {
		return CalculationResult{}, err
	}
	// This convenience API has no posting-account configuration; only the
	// configured Unit can produce a journal draft suitable for posting.
	result.JournalEntry = nil
	return result, nil
}

func Calculate(asOf time.Time, config Config, exposures []Exposure, existingAllowance types.Money) (CalculationResult, error) {
	var err error
	config, err = normalizeConfig(config)
	if err != nil {
		return CalculationResult{}, err
	}
	if err := config.Validate(); err != nil {
		return CalculationResult{}, err
	}
	if asOf.IsZero() {
		return CalculationResult{}, ErrInvalidDate
	}
	if existingAllowance.Currency() != config.Currency || existingAllowance.IsNegative() {
		return CalculationResult{}, ErrInvalidAllowance
	}
	required, err := types.Zero(config.Currency)
	if err != nil {
		return CalculationResult{}, err
	}
	results := make([]ReceivableResult, 0, len(exposures))
	seen := make(map[uuid.UUID]struct{}, len(exposures))

	sorted := append([]Exposure(nil), exposures...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Receivable.ID.String() < sorted[j].Receivable.ID.String()
	})
	for _, exposure := range sorted {
		line := exposure.Receivable
		if line.ID == uuid.Nil || line.CompanyID != config.CompanyID ||
			line.DueDate.IsZero() ||
			line.ResidualAmount.Currency() != config.Currency ||
			line.OriginalAmount.Currency() != config.Currency ||
			line.ResidualAmount.IsNegative() || line.OriginalAmount.IsNegative() {
			return CalculationResult{}, fmt.Errorf("%w: %s", ErrInvalidExposure, line.ID)
		}
		if _, exists := seen[line.ID]; exists {
			return CalculationResult{}, fmt.Errorf("%w: duplicate receivable %s", ErrInvalidExposure, line.ID)
		}
		seen[line.ID] = struct{}{}
		if line.Closed || line.ResidualAmount.Amount().IsZero() {
			continue
		}

		daysPastDue := daysPastDue(asOf, line.DueDate)
		bucket := agingBucket(daysPastDue)
		stage := classifyStage(daysPastDue, exposure.SICR, exposure.Defaulted)
		expected, err := line.ResidualAmount.Multiply(config.ProvisionMatrix[bucket])
		if err != nil {
			return CalculationResult{}, err
		}
		expected = expected.RoundForPosting()
		required, err = required.Add(expected)
		if err != nil {
			return CalculationResult{}, err
		}
		results = append(results, ReceivableResult{
			ReceivableID: line.ID,
			Stage:        stage,
			DaysPastDue:  daysPastDue,
			AgingBucket:  bucket,
			ECL:          expected,
		})
	}

	adjustment, err := required.Subtract(existingAllowance)
	if err != nil {
		return CalculationResult{}, err
	}
	adjustment = adjustment.RoundForPosting()
	result := CalculationResult{
		AsOf:              asOf.UTC(),
		Receivables:       results,
		RequiredAllowance: required,
		ExistingAllowance: existingAllowance,
		Adjustment:        adjustment,
	}
	if !adjustment.Amount().IsZero() {
		entry, err := makeAdjustmentEntry(config, asOf, adjustment)
		if err != nil {
			return CalculationResult{}, err
		}
		result.JournalEntry = &entry
	}
	return result, nil
}

func classifyStage(days int, sicr, defaulted bool) Stage {
	if defaulted || days > 90 {
		return Stage3
	}
	if sicr || days > 30 {
		return Stage2
	}
	return Stage1
}

func daysPastDue(asOf, dueDate time.Time) int {
	asOf = dateOnlyUTC(asOf)
	dueDate = dateOnlyUTC(dueDate)
	if !dueDate.Before(asOf) {
		return 0
	}
	return int(asOf.Sub(dueDate) / (24 * time.Hour))
}

func dateOnlyUTC(value time.Time) time.Time {
	year, month, day := value.UTC().Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

func agingBucket(days int) AgingBucket {
	switch {
	case days <= 0:
		return AgingCurrent
	case days <= 30:
		return Aging1To30
	case days <= 60:
		return Aging31To60
	case days <= 90:
		return Aging61To90
	default:
		return AgingOver90
	}
}

func makeAdjustmentEntry(config Config, asOf time.Time, adjustment types.Money) (journal.JournalEntry, error) {
	amount := adjustment
	if amount.IsNegative() {
		var err error
		amount, err = types.NewMoney(amount.Amount().Neg(), amount.Currency())
		if err != nil {
			return journal.JournalEntry{}, err
		}
	}
	var impairment, allowance journal.EntryLine
	if adjustment.Amount().IsPositive() {
		impairment = debitLine(config.ImpairmentAccountID, amount, nil)
		allowance = creditLine(config.AllowanceAccountID, amount, nil)
	} else {
		allowance = debitLine(config.AllowanceAccountID, amount, nil)
		impairment = creditLine(config.ImpairmentAccountID, amount, nil)
	}

	seed := strings.Join([]string{
		config.CompanyID.String(), config.JournalID.String(),
		config.ImpairmentAccountID.String(), config.AllowanceAccountID.String(),
		asOf.UTC().Format(time.RFC3339Nano), config.Currency, adjustment.Amount().String(),
	}, ":")
	entryID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(seed))
	impairment.ID = types.JournalEntryLineID(uuid.NewSHA1(entryID, []byte("impairment")))
	allowance.ID = types.JournalEntryLineID(uuid.NewSHA1(entryID, []byte("allowance")))
	date := asOf.UTC()
	lines := []journal.EntryLine{impairment, allowance}
	if !adjustment.Amount().IsPositive() {
		lines = []journal.EntryLine{allowance, impairment}
	}
	entry, err := newJournalEntry(config.JournalID, date, "IFRS 9 ECL allowance adjustment", lines)
	if err != nil {
		return journal.JournalEntry{}, err
	}
	entry.ID = types.JournalEntryID(entryID)
	entry.CompanyID = types.CompanyID(config.CompanyID)
	entry.Classification = journal.EntryClassAdjusting
	entry.CreatedAt = date
	entry.ReferenceID = "ifrs9-ecl-allowance-adjustment"
	return entry, nil
}

var allAgingBuckets = [...]AgingBucket{
	AgingCurrent,
	Aging1To30,
	Aging31To60,
	Aging61To90,
	AgingOver90,
}

func validAgingBucket(bucket AgingBucket) bool {
	for _, valid := range allAgingBuckets {
		if bucket == valid {
			return true
		}
	}
	return false
}
