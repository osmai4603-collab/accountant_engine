package sdk

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/osmai4603-collab/accounting_engine/internal/orbitals/consolidation"
	publicconsolidation "github.com/osmai4603-collab/accounting_engine/pkg/spi/consolidation"
	publicreconciliation "github.com/osmai4603-collab/accounting_engine/pkg/spi/reconciliation"
	"github.com/shopspring/decimal"
)

type defaultConsolidationService struct{}

func (s *defaultConsolidationService) Consolidate(ctx context.Context, req publicconsolidation.ConsolidationRequest) (publicconsolidation.ConsolidationResult, error) {
	if ctx == nil {
		return publicconsolidation.ConsolidationResult{}, errors.New("consolidation: context is required")
	}
	if err := ctx.Err(); err != nil {
		return publicconsolidation.ConsolidationResult{}, err
	}

	rateLookup := make(map[string]decimal.Decimal, len(req.Rates))
	for _, r := range req.Rates {
		key := strings.ToUpper(r.FromCurrency) + ":" + strings.ToUpper(r.ToCurrency) + ":" + string(r.RateClass)
		rateLookup[key] = r.Rate
	}
	provider := staticRateProvider{rates: rateLookup}

	entities := make([]consolidation.Entity, 0, len(req.Entities))
	for _, e := range req.Entities {
		entities = append(entities, consolidation.Entity{
			ID:                 e.ID,
			FunctionalCurrency: e.FunctionalCurrency,
		})
	}
	accounts := make([]consolidation.GroupAccount, 0, len(req.GroupAccounts))
	for _, a := range req.GroupAccounts {
		accounts = append(accounts, consolidation.GroupAccount{
			ID:   a.ID,
			Name: a.Name,
			Type: consolidation.AccountType(a.Type),
		})
	}
	mappings := make([]consolidation.AccountMapping, 0, len(req.AccountMappings))
	for _, m := range req.AccountMappings {
		mappings = append(mappings, consolidation.AccountMapping{
			EntityID:        m.EntityID,
			SourceAccountID: m.SourceAccountID,
			GroupAccountID:  m.GroupAccountID,
			RateClass:       consolidation.RateClass(m.RateClass),
			HistoricalDate:  m.HistoricalDate,
		})
	}
	ownerships := make([]consolidation.Ownership, 0, len(req.Ownerships))
	for _, o := range req.Ownerships {
		ownerships = append(ownerships, consolidation.Ownership{
			ParentEntityID:           o.ParentEntityID,
			SubsidiaryEntityID:       o.SubsidiaryEntityID,
			Percentage:               o.Percentage,
			ControlBasis:             o.ControlBasis,
			ControlEvidenceReference: o.ControlEvidenceReference,
		})
	}
	tbs := make([]consolidation.TrialBalance, 0, len(req.TrialBalances))
	for _, tb := range req.TrialBalances {
		lines := make([]consolidation.TrialBalanceLine, 0, len(tb.Lines))
		for _, l := range tb.Lines {
			var debit, credit decimal.Decimal
			if l.Balance.IsNegative() {
				credit = l.Balance.Abs()
			} else {
				debit = l.Balance
			}
			lines = append(lines, consolidation.TrialBalanceLine{
				AccountID:    l.AccountID,
				EndingDebit:  debit,
				EndingCredit: credit,
			})
		}
		tbs = append(tbs, consolidation.TrialBalance{
			EntityID:  tb.EntityID,
			Currency:  tb.Currency,
			StartDate: tb.StartDate,
			EndDate:   tb.EndDate,
			Lines:     lines,
		})
	}

	group := consolidation.Group{
		ID:                req.GroupID,
		ParentEntityID:    req.ParentEntityID,
		ReportingCurrency: req.GroupCurrency,
		Period:            consolidation.ReportingPeriod{StartDate: req.StartDate, EndDate: req.EndDate},
		Entities:          entities,
		Accounts:          accounts,
		Mappings:          mappings,
		Ownerships:        ownerships,
		Policies: consolidation.Policies{
			AccountingPolicyID:          "sdk-default-accounting",
			ControlPolicyID:             "sdk-default-control",
			CurrencyTranslationPolicyID: "sdk-default-fx",
			EliminationPolicyID:         "sdk-default-elim",
			IntercompanyPolicyID:        "sdk-default-ic",
			EliminationScope:            consolidation.PolicyScopeNone,
			IntercompanyScope:           consolidation.PolicyScopeNone,
		},
	}

	eng := consolidation.NewEngine(provider)
	res, err := eng.Consolidate(group, tbs)
	if err != nil {
		return publicconsolidation.ConsolidationResult{}, err
	}
	balances := make(map[string]decimal.Decimal, len(res.Accounts))
	for _, acc := range res.Accounts {
		balances[acc.AccountID] = acc.Amount
	}
	return publicconsolidation.ConsolidationResult{
		GroupCurrency: req.GroupCurrency,
		Balances:      balances,
		Totals: publicconsolidation.FinancialTotals{
			Assets:      res.Totals.Assets,
			Liabilities: res.Totals.Liabilities,
			Equity:      res.Totals.Equity,
			Revenue:     res.Totals.Revenue,
			Expenses:    res.Totals.Expenses,
			NetIncome:   res.Totals.NetIncome,
		},
		AdjustmentCount: len(res.Entries),
	}, nil
}

type staticRateProvider struct {
	rates map[string]decimal.Decimal
}

func (p staticRateProvider) Rate(req consolidation.RateRequest) (decimal.Decimal, error) {
	if strings.EqualFold(req.FromCurrency, req.ToCurrency) {
		return decimal.NewFromInt(1), nil
	}
	key := strings.ToUpper(req.FromCurrency) + ":" + strings.ToUpper(req.ToCurrency) + ":" + string(req.Class)
	if rate, ok := p.rates[key]; ok {
		return rate, nil
	}
	return decimal.Zero, fmt.Errorf("%w: %s->%s (%s)", consolidation.ErrMissingExchangeRate, req.FromCurrency, req.ToCurrency, req.Class)
}

type defaultReconciliationService struct{}

func (s *defaultReconciliationService) ReconcileRecords(ctx context.Context, req publicreconciliation.ReconciliationRequest) (publicreconciliation.ReconciliationResult, error) {
	if ctx == nil {
		return publicreconciliation.ReconciliationResult{}, errors.New("reconciliation: context is required")
	}
	if err := ctx.Err(); err != nil {
		return publicreconciliation.ReconciliationResult{}, err
	}
	if strings.TrimSpace(req.LeftSourceName) == "" || strings.TrimSpace(req.RightSourceName) == "" || strings.TrimSpace(req.Currency) == "" {
		return publicreconciliation.ReconciliationResult{}, errors.New("reconciliation: source names and currency are required")
	}
	if req.Tolerance.IsNegative() {
		return publicreconciliation.ReconciliationResult{}, errors.New("reconciliation: tolerance must be non-negative")
	}

	rightByKey := make(map[string]publicreconciliation.Record, len(req.RightRecords))
	matchedRight := make(map[string]bool, len(req.RightRecords))
	for _, r := range req.RightRecords {
		rightByKey[r.Key] = r
	}

	var result publicreconciliation.ReconciliationResult
	for _, l := range req.LeftRecords {
		r, ok := rightByKey[l.Key]
		if !ok {
			result.Exceptions = append(result.Exceptions, publicreconciliation.Exception{
				Side:   "left_only",
				Key:    l.Key,
				Amount: l.Amount,
				Reason: "missing in " + req.RightSourceName,
			})
			continue
		}
		matchedRight[l.Key] = true
		diff := l.Amount.Sub(r.Amount)
		if diff.Abs().GreaterThan(req.Tolerance) {
			result.Exceptions = append(result.Exceptions, publicreconciliation.Exception{
				Side:   "amount_mismatch",
				Key:    l.Key,
				Amount: diff,
				Reason: fmt.Sprintf("difference %s exceeds tolerance %s", diff.String(), req.Tolerance.String()),
			})
			continue
		}
		result.Matched = append(result.Matched, publicreconciliation.Match{
			Key:         l.Key,
			LeftAmount:  l.Amount,
			RightAmount: r.Amount,
			Difference:  diff,
		})
	}
	for _, r := range req.RightRecords {
		if !matchedRight[r.Key] {
			result.Exceptions = append(result.Exceptions, publicreconciliation.Exception{
				Side:   "right_only",
				Key:    r.Key,
				Amount: r.Amount,
				Reason: "missing in " + req.LeftSourceName,
			})
		}
	}
	result.Balanced = len(result.Exceptions) == 0
	return result, nil
}
