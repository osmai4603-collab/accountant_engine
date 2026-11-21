package reconciliation

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/osmai4603-collab/accounting_engine/internal/orbitals/reconciliation/bank_parsers"
	"github.com/shopspring/decimal"
)

var (
	// ErrUnsupportedStatementFormat indicates an unrecognised bank statement format.
	ErrUnsupportedStatementFormat = errors.New("reconciliation: unsupported bank statement format")
	// ErrStatementBalanceMismatch means the statement lines do not tie to its closing balance.
	ErrStatementBalanceMismatch = errors.New("reconciliation: bank statement balances do not tie out")
	// ErrBankReconciliationOutstanding means unmatched items remain after matching.
	ErrBankReconciliationOutstanding = errors.New("reconciliation: outstanding bank items remain")
)

// StatementFormat identifies a supported bank statement representation.
type StatementFormat string

const (
	StatementFormatAuto    StatementFormat = "auto"
	StatementFormatCAMT053 StatementFormat = "camt053"
	StatementFormatMT940   StatementFormat = "mt940"
)

// CashRecordSource provides already-posted cash movements from the treasury
// ledger for the same account, window and currency as the bank statement.
type CashRecordSource interface {
	LoadCashRecords(ctx context.Context, accountID uuid.UUID, start, end time.Time, currency string) ([]SourceRecord, error)
}

// MatchedBankTransaction is a statement line paired with a posted cash record.
type MatchedBankTransaction struct {
	StatementTx    bank_parsers.ParsedBankTx
	LedgerRecord   SourceRecord
	AmountVariance decimal.Decimal
}

// BankReconciliationReport is the audit-ready outcome of one bank reconciliation.
type BankReconciliationReport struct {
	CompanyID               uuid.UUID
	AccountID               uuid.UUID
	AccountIBAN             string
	Currency                string
	PeriodStart             time.Time
	PeriodEnd               time.Time
	OpeningBalance          decimal.Decimal
	ClosingBalance          decimal.Decimal
	StatementLineTotal      decimal.Decimal
	StatementBalancesTieOut bool
	Comparison              ComparisonResult
	MatchedTransactions     []MatchedBankTransaction
	UnmatchedStatement      []bank_parsers.ParsedBankTx
	UnmatchedLedger         []SourceRecord
	VarianceTotal           decimal.Decimal
	Reconciled              bool
	GeneratedAt             time.Time
}

// ParseStatement parses a CAMT.053 XML or MT940 statement. Auto-detection
// inspects the payload prefix so callers do not need to know the bank format.
func ParseStatement(data []byte, format StatementFormat) (*bank_parsers.BankStatement, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, fmt.Errorf("%w: empty payload", bank_parsers.ErrInvalidStatement)
	}
	switch format {
	case StatementFormatAuto, "":
		trimmed := bytes.TrimSpace(data)
		switch {
		case bytes.HasPrefix(trimmed, []byte("<")):
			format = StatementFormatCAMT053
		case bytes.HasPrefix(trimmed, []byte(":")) || bytes.HasPrefix(trimmed, []byte(":20")):
			format = StatementFormatMT940
		default:
			return nil, fmt.Errorf("%w: cannot auto-detect format", ErrUnsupportedStatementFormat)
		}
	}
	var parser bank_parsers.StatementParser
	switch format {
	case StatementFormatCAMT053:
		parser = bank_parsers.CAMT053Parser{}
	case StatementFormatMT940:
		parser = bank_parsers.MT940Parser{}
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedStatementFormat, format)
	}
	return parser.Parse(data)
}

// Matcher joins statement parsing, cash-record loading and deterministic
// comparison into a single bank reconciliation path.
type Matcher struct {
	requirements Requirements
}

// NewMatcher validates the comparison contract before any work is performed.
func NewMatcher(requirements Requirements) (*Matcher, error) {
	if err := requirements.Validate(); err != nil {
		return nil, err
	}
	requirements.MatchKeys = append([]string(nil), requirements.MatchKeys...)
	return &Matcher{requirements: requirements}, nil
}

// Requirements returns a defensive copy of the active comparison contract.
func (m *Matcher) Requirements() Requirements {
	if m == nil {
		return Requirements{}
	}
	result := m.requirements
	result.MatchKeys = append([]string(nil), result.MatchKeys...)
	return result
}

type statementSource struct {
	statement *bank_parsers.BankStatement
	format    string
}

func (s statementSource) LoadRecords(
	_ context.Context, start, end time.Time, currency string,
) ([]SourceRecord, error) {
	if s.statement == nil {
		return nil, fmt.Errorf("%w: nil statement", ErrUnsupportedStatementFormat)
	}
	records := make([]SourceRecord, 0, len(s.statement.Transactions))
	for _, tx := range s.statement.Transactions {
		if !tx.ValueDate.IsZero() && (tx.ValueDate.Before(start) || tx.ValueDate.After(end)) {
			continue
		}
		if currency != "" && tx.Currency != "" && !strings.EqualFold(tx.Currency, currency) {
			continue
		}
		records = append(records, SourceRecord{
			ID:        tx.ReferenceNumber,
			MatchKey:  tx.ReferenceNumber,
			Amount:    tx.Amount.Add(tx.BankFee),
			Date:      tx.ValueDate,
			Reference: tx.Description,
		})
	}
	return records, nil
}

type ledgerSource struct {
	records []SourceRecord
}

func (s ledgerSource) LoadRecords(
	_ context.Context, _, _ time.Time, _ string,
) ([]SourceRecord, error) {
	return s.records, nil
}

// ReconcileStatement parses the statement, loads matching cash records and
// produces a full reconciliation report. A non-nil error is returned only for
// infrastructure/contract failures; outstanding items are reported in the
// result together with ErrBankReconciliationOutstanding so operators can both
// fail fast and inspect the exception detail.
func (m *Matcher) ReconcileStatement(
	ctx context.Context,
	statementData []byte,
	format StatementFormat,
	companyID, accountID uuid.UUID,
	cash CashRecordSource,
) (BankReconciliationReport, error) {
	report := BankReconciliationReport{
		CompanyID:   companyID,
		AccountID:   accountID,
		GeneratedAt: time.Now().UTC(),
	}
	if m == nil {
		return report, fmt.Errorf("%w: matcher is nil", ErrCapabilityDependencyMissing)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return report, err
	}
	if cash == nil {
		return report, fmt.Errorf("%w: cash record source", ErrCapabilityDependencyMissing)
	}
	if accountID == uuid.Nil {
		return report, fmt.Errorf("%w: account id is required", ErrInvalidRequirements)
	}

	statement, err := ParseStatement(statementData, format)
	if err != nil {
		return report, err
	}
	report.AccountIBAN = statement.AccountIBAN
	report.Currency = statement.Currency
	report.OpeningBalance = statement.OpeningBalance
	report.ClosingBalance = statement.ClosingBalance
	if statement.Currency == "" {
		report.Currency = m.requirements.Left.Currency
	}
	report.PeriodStart = m.requirements.Left.Start
	report.PeriodEnd = m.requirements.Left.End

	lineTotal := statement.OpeningBalance
	statementLineSum := decimal.Zero
	for _, tx := range statement.Transactions {
		statementLineSum = statementLineSum.Add(tx.Amount).Add(tx.BankFee)
		lineTotal = lineTotal.Add(tx.Amount).Add(tx.BankFee)
		report.StatementLineTotal = lineTotal
	}
	report.StatementBalancesTieOut = lineTotal.Equal(statement.ClosingBalance)

	records, err := cash.LoadCashRecords(ctx, accountID, report.PeriodStart, report.PeriodEnd, report.Currency)
	if err != nil {
		return report, fmt.Errorf("load cash records: %w", err)
	}
	capability := &Capability{repository: nil, requirements: m.requirements}
	comparison, err := capability.CompareSources(
		ctx,
		statementSource{statement: statement},
		ledgerSource{records: records},
	)
	if err != nil {
		return report, err
	}
	report.Comparison = comparison
	report.MatchedTransactions = make([]MatchedBankTransaction, 0, len(comparison.Matched))
	for _, pair := range comparison.Matched {
		report.MatchedTransactions = append(report.MatchedTransactions, MatchedBankTransaction{
			StatementTx:    transactionByReference(statement, pair.Left.ID),
			LedgerRecord:   pair.Right,
			AmountVariance: pair.Difference,
		})
	}
	report.UnmatchedStatement = make([]bank_parsers.ParsedBankTx, 0, len(comparison.UnmatchedLeft))
	for _, record := range comparison.UnmatchedLeft {
		report.UnmatchedStatement = append(report.UnmatchedStatement, transactionByReference(statement, record.ID))
	}
	report.UnmatchedLedger = comparison.UnmatchedRight
	report.VarianceTotal = comparison.DifferenceTotal
	report.Reconciled = comparison.WithinTolerance && report.StatementBalancesTieOut

	if !report.StatementBalancesTieOut {
		return report, fmt.Errorf("%w: opening %s + lines %s != closing %s",
			ErrStatementBalanceMismatch, statement.OpeningBalance, statementLineSum, statement.ClosingBalance)
	}
	if !report.Reconciled {
		return report, fmt.Errorf("%w: %d statement and %d ledger items unmatched (variance %s)",
			ErrBankReconciliationOutstanding,
			len(report.UnmatchedStatement), len(report.UnmatchedLedger), report.VarianceTotal)
	}
	return report, nil
}

func transactionByReference(statement *bank_parsers.BankStatement, reference string) bank_parsers.ParsedBankTx {
	if statement == nil {
		return bank_parsers.ParsedBankTx{}
	}
	for _, tx := range statement.Transactions {
		if tx.ReferenceNumber == reference {
			return tx
		}
	}
	return bank_parsers.ParsedBankTx{ReferenceNumber: reference}
}
