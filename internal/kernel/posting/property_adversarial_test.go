package posting

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/osmai4603-collab/accounting_engine/internal/kernel/journal"
	"github.com/osmai4603-collab/accounting_engine/internal/kernel/masterdata"
	"github.com/osmai4603-collab/accounting_engine/internal/kernel/policy"
	"github.com/osmai4603-collab/accounting_engine/internal/kernel/types"
	"github.com/shopspring/decimal"
)

func FuzzBalancedEntryProperties(f *testing.F) {
	for _, seed := range [][2]int64{
		{100, 100},
		{1004, 1004},
		{1, 1},
		{999999, 999999},
	} {
		f.Add(seed[0], seed[1])
	}
	f.Fuzz(func(t *testing.T, debitMinor, creditMinor int64) {
		if debitMinor <= 0 || creditMinor <= 0 || debitMinor > 1_000_000_000 || creditMinor > 1_000_000_000 {
			return
		}
		debit, err := types.NewMoney(decimal.NewFromInt(debitMinor).Shift(-2), "USD")
		if err != nil {
			return
		}
		credit, err := types.NewMoney(decimal.NewFromInt(creditMinor).Shift(-2), "USD")
		if err != nil {
			return
		}
		zero, err := types.Zero("USD")
		if err != nil {
			t.Fatal(err)
		}
		debitAccountID, creditAccountID := types.NewAccountID(), types.NewAccountID()
		entry := journal.JournalEntry{
			ID: types.NewJournalEntryID(), JournalID: types.NewJournalID(),
			EffectiveDate: time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC),
			Status:        journal.EntryPosted, Classification: journal.EntryClassRegular,
			Lines: []journal.EntryLine{
				{ID: types.NewJournalEntryLineID(), AccountID: debitAccountID, TransactionAmount: debit, TransactionCurrency: "USD", ExchangeRate: decimal.NewFromInt(1), FunctionalDebit: debit, FunctionalCredit: zero},
				{ID: types.NewJournalEntryLineID(), AccountID: creditAccountID, TransactionAmount: credit, TransactionCurrency: "USD", ExchangeRate: decimal.NewFromInt(1), FunctionalDebit: zero, FunctionalCredit: credit},
			},
		}
		err = ValidateEntryDoubleEntry(&entry, nil)
		if debitMinor == creditMinor {
			if err != nil {
				t.Fatalf("balanced entry rejected: %v", err)
			}
		} else if !errors.Is(err, ErrUnbalancedEntry) {
			t.Fatalf("unbalanced entry error = %v, want ErrUnbalancedEntry", err)
		} else {
			return
		}
		reversal, err := BuildReversalEntry(entry, entry.EffectiveDate, "actor", "property reversal")
		if err != nil {
			t.Fatalf("valid posted entry reversal: %v", err)
		}
		reversal.Status = journal.EntryPosted
		if err := ValidateEntryDoubleEntry(&reversal, nil); err != nil {
			t.Fatalf("reversal is unbalanced: %v", err)
		}
		netByAccount := make(map[types.AccountID]decimal.Decimal)
		for _, current := range []journal.JournalEntry{entry, reversal} {
			for _, line := range current.Lines {
				netByAccount[line.AccountID] = netByAccount[line.AccountID].
					Add(line.FunctionalDebit.Amount()).
					Sub(line.FunctionalCredit.Amount())
			}
		}
		for accountID, net := range netByAccount {
			if !net.IsZero() {
				t.Fatalf("entry and reversal net for account %s = %s, want zero", accountID, net)
			}
		}
	})
}

func FuzzRoundingResidualRequiresExplicitSettlement(f *testing.F) {
	f.Add(int64(1000))
	f.Add(int64(1))
	f.Add(int64(999_999))
	f.Fuzz(func(t *testing.T, cents int64) {
		if cents <= 0 || cents > 1_000_000 {
			return
		}
		debit, err := types.NewMoney(
			decimal.NewFromInt(cents).Shift(-2).Add(decimal.NewFromInt(5).Shift(-3)),
			"USD",
		)
		if err != nil {
			t.Fatal(err)
		}
		credit, err := types.NewMoney(debit.Amount().Mul(decimal.NewFromInt(2)), "USD")
		if err != nil {
			t.Fatal(err)
		}
		zero, err := types.Zero("USD")
		if err != nil {
			t.Fatal(err)
		}
		entry := journal.JournalEntry{
			ID: types.NewJournalEntryID(), JournalID: types.NewJournalID(),
			EffectiveDate:  time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC),
			Classification: journal.EntryClassRegular,
			Lines: []journal.EntryLine{
				{ID: types.NewJournalEntryLineID(), AccountID: types.NewAccountID(), TransactionAmount: debit, TransactionCurrency: "USD", ExchangeRate: decimal.NewFromInt(1), FunctionalDebit: debit, FunctionalCredit: zero},
				{ID: types.NewJournalEntryLineID(), AccountID: types.NewAccountID(), TransactionAmount: debit, TransactionCurrency: "USD", ExchangeRate: decimal.NewFromInt(1), FunctionalDebit: debit, FunctionalCredit: zero},
				{ID: types.NewJournalEntryLineID(), AccountID: types.NewAccountID(), TransactionAmount: credit, TransactionCurrency: "USD", ExchangeRate: decimal.NewFromInt(1), FunctionalDebit: zero, FunctionalCredit: credit},
			},
		}
		if err := ValidateEntryDoubleEntry(&entry, nil); err != nil {
			t.Fatalf("exact input entry is not balanced: %v", err)
		}
		original := entry.Clone()
		if err := RoundEntryToMinorUnits(&entry, types.AccountID{}); !errors.Is(err, ErrRoundingAccountRequired) {
			t.Fatalf("rounding without settlement account error = %v, want ErrRoundingAccountRequired", err)
		}
		if len(entry.Lines) != len(original.Lines) {
			t.Fatal("failed rounding mutated the entry")
		}
		roundingAccountID := types.NewAccountID()
		if err := RoundEntryToMinorUnits(&entry, roundingAccountID); err != nil {
			t.Fatalf("rounding with settlement account: %v", err)
		}
		if len(entry.Lines) != len(original.Lines)+1 {
			t.Fatalf("settled line count = %d, want %d", len(entry.Lines), len(original.Lines)+1)
		}
		if settlement := entry.Lines[len(entry.Lines)-1]; settlement.AccountID != roundingAccountID ||
			(settlement.FunctionalDebit.Amount().IsZero() && settlement.FunctionalCredit.Amount().IsZero()) {
			t.Fatalf("rounding residual was not explicitly settled: %+v", settlement)
		}
		if err := ValidateEntryDoubleEntry(&entry, nil); err != nil {
			t.Fatalf("settled entry is unbalanced: %v", err)
		}
	})
}

type failingPostingReader struct {
	masterdata.Reader
	companyErr error
	accountErr error
	journalErr error
	periodErr  error
}

func (r failingPostingReader) GetCompany(context.Context, types.CompanyID) (masterdata.Company, error) {
	if r.companyErr != nil {
		return masterdata.Company{}, r.companyErr
	}
	return r.Reader.GetCompany(context.Background(), types.NewCompanyID())
}

func (r failingPostingReader) GetAccount(context.Context, types.CompanyID, types.AccountID) (masterdata.Account, error) {
	if r.accountErr != nil {
		return masterdata.Account{}, r.accountErr
	}
	return r.Reader.GetAccount(context.Background(), types.NewCompanyID(), types.NewAccountID())
}

func (r failingPostingReader) GetJournalByID(context.Context, types.CompanyID, types.JournalID) (masterdata.Journal, error) {
	if r.journalErr != nil {
		return masterdata.Journal{}, r.journalErr
	}
	return r.Reader.(masterdata.JournalRepository).GetJournalByID(context.Background(), types.NewCompanyID(), types.NewJournalID())
}

func (r failingPostingReader) GetPeriodForDate(context.Context, types.CompanyID, time.Time) (masterdata.FiscalPeriod, error) {
	if r.periodErr != nil {
		return masterdata.FiscalPeriod{}, r.periodErr
	}
	return r.Reader.GetPeriodForDate(context.Background(), types.NewCompanyID(), time.Now().UTC())
}

func TestPostingAdversarialRepositoryFailures(t *testing.T) {
	entry := newPostingTestEntry(t, "creator")
	base := &mockMasterReaderForPosting{compActive: true, periodOpen: true}
	failures := []struct {
		name   string
		reader failingPostingReader
		want   error
	}{
		{name: "company", reader: failingPostingReader{Reader: base, companyErr: errors.New("company unavailable")}, want: errors.New("company unavailable")},
		{name: "journal", reader: failingPostingReader{Reader: base, journalErr: errors.New("journal unavailable")}, want: errors.New("journal unavailable")},
		{name: "period", reader: failingPostingReader{Reader: base, periodErr: errors.New("period unavailable")}, want: errors.New("period unavailable")},
		{name: "account", reader: failingPostingReader{Reader: base, accountErr: errors.New("account unavailable")}, want: errors.New("account unavailable")},
	}
	for _, test := range failures {
		t.Run(test.name, func(t *testing.T) {
			pipeline := NewPostingPipeline(test.reader,
				func(context.Context, journal.JournalEntry, PostOptions) (journal.JournalEntry, error) {
					t.Fatal("executor ran after repository failure")
					return journal.JournalEntry{}, nil
				},
				WithPolicyEngine(testPostingPolicyEngine(t, entry.CompanyID, "USD")))
			_, err := pipeline.PostEntry(context.Background(), entry, PostOptions{ActorID: "poster", IdempotencyKey: "test-idem-key"})
			if err == nil {
				t.Fatalf("%s repository failure unexpectedly passed", test.name)
			}
		})
	}
}

func TestPostingRoundingResidualPositiveAndNegative(t *testing.T) {
	roundingAccount := types.NewAccountID()
	for _, test := range []struct {
		name        string
		debitLines  []string
		creditLines []string
	}{
		{name: "positive residual", debitLines: []string{"10.015", "10.015"}, creditLines: []string{"20.000"}},
		{name: "negative residual", debitLines: []string{"10.004"}, creditLines: []string{"10.004", "10.004"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			zero, err := types.Zero("USD")
			if err != nil {
				t.Fatal(err)
			}
			entry := journal.JournalEntry{
				ID: types.NewJournalEntryID(), JournalID: types.NewJournalID(),
				EffectiveDate: time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC),
				Lines:         make([]journal.EntryLine, 0, len(test.debitLines)+len(test.creditLines)),
			}
			for _, value := range test.debitLines {
				amount, err := types.NewMoneyFromString(value, "USD")
				if err != nil {
					t.Fatal(err)
				}
				entry.Lines = append(entry.Lines, journal.EntryLine{
					ID: types.NewJournalEntryLineID(), AccountID: types.NewAccountID(),
					TransactionAmount: amount, TransactionCurrency: "USD",
					ExchangeRate: decimal.NewFromInt(1), FunctionalDebit: amount, FunctionalCredit: zero,
				})
			}
			for _, value := range test.creditLines {
				amount, err := types.NewMoneyFromString(value, "USD")
				if err != nil {
					t.Fatal(err)
				}
				entry.Lines = append(entry.Lines, journal.EntryLine{
					ID: types.NewJournalEntryLineID(), AccountID: types.NewAccountID(),
					TransactionAmount: amount, TransactionCurrency: "USD",
					ExchangeRate: decimal.NewFromInt(1), FunctionalDebit: zero, FunctionalCredit: amount,
				})
			}
			round := func(amount types.Money) (types.Money, error) {
				return amount.Rounded()
			}
			if err := RoundEntryWithRoundingPolicy(&entry, roundingAccount, round); err == nil {
				originalCount := len(test.debitLines) + len(test.creditLines)
				if len(entry.Lines) != originalCount+1 {
					t.Fatalf("settlement line count = %d, want %d", len(entry.Lines), originalCount+1)
				}
				settlement := entry.Lines[len(entry.Lines)-1]
				if settlement.AccountID != roundingAccount {
					t.Fatalf("settlement account = %s, want %s", settlement.AccountID, roundingAccount)
				}
			}
		})
	}
}

func TestPostingPolicyAndClockAdversarialBranches(t *testing.T) {
	companyID := types.NewCompanyID()
	entry := newPostingTestEntry(t, "creator")
	entry.CompanyID = companyID
	engine := policy.NewPolicyEngine()
	pipeline := NewPostingPipeline(
		&mockMasterReaderForPosting{compActive: true, periodOpen: true},
		func(context.Context, journal.JournalEntry, PostOptions) (journal.JournalEntry, error) {
			t.Fatal("executor ran with missing policies")
			return journal.JournalEntry{}, nil
		},
		WithPolicyEngine(engine),
	)
	if _, err := pipeline.PostEntry(context.Background(), entry, PostOptions{ActorID: "poster", IdempotencyKey: "test-idem-key"}); err == nil {
		t.Fatal("missing posting policy unexpectedly passed")
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	pipeline = NewPostingPipeline(
		&mockMasterReaderForPosting{compActive: true, periodOpen: true},
		func(ctx context.Context, _ journal.JournalEntry, _ PostOptions) (journal.JournalEntry, error) {
			return journal.JournalEntry{}, ctx.Err()
		},
		WithPolicyEngine(testPostingPolicyEngine(t, companyID, "USD")),
	)
	if _, err := pipeline.PostEntry(cancelled, entry, PostOptions{ActorID: "poster", IdempotencyKey: "test-idem-key"}); err == nil {
		t.Fatal("cancelled posting context unexpectedly passed")
	}
}
