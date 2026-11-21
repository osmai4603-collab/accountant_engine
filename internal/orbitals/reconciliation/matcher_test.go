package reconciliation

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

const mt940Fixture = `:20:STATEMENT-1
:25:SA0380000000608010167519
:60F:C261109SAR1000,00
:61:261110D100,50NTRFNONREF//PAY-44
:86:Supplier payment
:62F:C261110SAR899,50`

const camt053Fixture = `<?xml version="1.0" encoding="UTF-8"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:camt.053.001.02">
  <BkToCstmrStmt><Stmt><CreDtTm>2026-11-10T09:30:00Z</CreDtTm>
    <Acct><Id><IBAN>SA0380000000608010167519</IBAN></Id></Acct>
    <Bal><Tp><CdOrPrtry><Cd>OPBD</Cd></CdOrPrtry></Tp><Amt Ccy="SAR">1000.00</Amt><CdtDbtInd>CRDT</CdtDbtInd></Bal>
    <Bal><Tp><CdOrPrtry><Cd>CLBD</Cd></CdOrPrtry></Tp><Amt Ccy="SAR">899.50</Amt><CdtDbtInd>CRDT</CdtDbtInd><Dt><Dt>2026-11-10</Dt></Dt></Bal>
    <Ntry><Amt Ccy="SAR">100.50</Amt><CdtDbtInd>DBIT</CdtDbtInd>
      <ValDt><Dt>2026-11-10</Dt></ValDt><NtryRef>ENTRY-1</NtryRef>
      <NtryDtls><TxDtls><Refs><EndToEndId>PAY-44</EndToEndId></Refs><RmtInf><Ustrd>Supplier payment</Ustrd></RmtInf></TxDtls></NtryDtls>
    </Ntry>
  </Stmt></BkToCstmrStmt>
</Document>`

type cashSource struct {
	records []SourceRecord
	err     error
}

func (s cashSource) LoadCashRecords(
	_ context.Context, _ uuid.UUID, _, _ time.Time, _ string,
) ([]SourceRecord, error) {
	return s.records, s.err
}

func matcherRequirements() Requirements {
	day := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	return Requirements{
		Left:               SourceRequirement{Name: "bank_statement", Currency: "SAR", Start: day, End: day.Add(24*time.Hour - time.Nanosecond)},
		Right:              SourceRequirement{Name: "cash_ledger", Currency: "SAR", Start: day, End: day.Add(24*time.Hour - time.Nanosecond)},
		MatchKeys:          []string{"reference"},
		Tolerance:          decimal.Zero,
		ExceptionTreatment: ExceptionReject,
	}
}

func TestParseStatementAutoDetectsBothFormats(t *testing.T) {
	mt940, err := ParseStatement([]byte(mt940Fixture), StatementFormatAuto)
	if err != nil || len(mt940.Transactions) != 1 || mt940.Currency != "SAR" {
		t.Fatalf("MT940 auto-detection failed: stmt=%+v err=%v", mt940, err)
	}
	camt, err := ParseStatement([]byte(camt053Fixture), StatementFormatAuto)
	if err != nil || len(camt.Transactions) != 1 || camt.Currency != "SAR" {
		t.Fatalf("CAMT.053 auto-detection failed: stmt=%+v err=%v", camt, err)
	}
	if _, err := ParseStatement([]byte("not a statement"), StatementFormatAuto); err == nil {
		t.Fatal("expected unsupported format error")
	}
}

func TestMatcherReconcilesMT940AgainstCashLedger(t *testing.T) {
	matcher, err := NewMatcher(matcherRequirements())
	if err != nil {
		t.Fatal(err)
	}
	day := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	cash := cashSource{records: []SourceRecord{{
		ID: "cash-1", MatchKey: "PAY-44",
		Amount: decimal.RequireFromString("-100.50"),
		Date:   day, Reference: "Supplier payment",
	}}}
	report, err := matcher.ReconcileStatement(
		context.Background(), []byte(mt940Fixture), StatementFormatMT940,
		uuid.New(), uuid.New(), cash,
	)
	if err != nil {
		t.Fatalf("expected successful reconciliation, got %v", err)
	}
	if !report.Reconciled || !report.StatementBalancesTieOut {
		t.Fatalf("expected reconciled report, got %+v", report)
	}
	if len(report.MatchedTransactions) != 1 || len(report.UnmatchedStatement) != 0 || len(report.UnmatchedLedger) != 0 {
		t.Fatalf("expected one matched transaction and no exceptions, got %+v", report)
	}
	if !report.VarianceTotal.IsZero() {
		t.Fatalf("expected zero variance, got %s", report.VarianceTotal)
	}
	if !report.ClosingBalance.Equal(decimal.RequireFromString("899.50")) {
		t.Fatalf("unexpected closing balance: %s", report.ClosingBalance)
	}
}

func TestMatcherReportsOutstandingBankItems(t *testing.T) {
	matcher, err := NewMatcher(matcherRequirements())
	if err != nil {
		t.Fatal(err)
	}
	report, err := matcher.ReconcileStatement(
		context.Background(), []byte(mt940Fixture), StatementFormatAuto,
		uuid.New(), uuid.New(), cashSource{},
	)
	if !errors.Is(err, ErrBankReconciliationOutstanding) {
		t.Fatalf("expected outstanding-items error, got %v", err)
	}
	if report.Reconciled || len(report.UnmatchedStatement) != 1 {
		t.Fatalf("expected one unmatched statement line, got %+v", report)
	}
	if len(report.Comparison.Exceptions) == 0 || !strings.Contains(report.Comparison.Exceptions[0], "unmatched") {
		t.Fatalf("expected exception detail, got %+v", report.Comparison.Exceptions)
	}
}

func TestMatcherRejectsStatementThatDoesNotTieOut(t *testing.T) {
	matcher, err := NewMatcher(matcherRequirements())
	if err != nil {
		t.Fatal(err)
	}
	broken := strings.Replace(mt940Fixture, "899,50", "898,50", 1)
	_, err = matcher.ReconcileStatement(
		context.Background(), []byte(broken), StatementFormatMT940,
		uuid.New(), uuid.New(), cashSource{},
	)
	if !errors.Is(err, ErrStatementBalanceMismatch) {
		t.Fatalf("expected balance tie-out error, got %v", err)
	}
}
