package ifrs15

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/reporting"
	"github.com/shopspring/decimal"
)

// GetDisclosures implements standards.DisclosureProvider for IFRS 15.
// Produces structured disclosures for:
// 1. IFRS15-REV-DISAGG: Disaggregation of revenue (Point in Time vs Over Time).
// 2. IFRS15-CONTRACT-BALANCES: Contract assets and contract liabilities.
// 3. IFRS15-REMAINING-OBLIGATIONS: Transaction price allocated to remaining performance obligations (IFRS 15.120).
func (m *Module) GetDisclosures(
	ctx context.Context,
	query reporting.DisclosureQuery,
) ([]reporting.StructuredDisclosure, error) {
	companyUUID := uuid.UUID(query.CompanyID)
	if companyUUID == uuid.Nil {
		return nil, fmt.Errorf("reporting: invalid company ID")
	}
	if query.StandardReference != "" && query.StandardReference != "IFRS 15" {
		return nil, nil
	}

	requestedCurrency := strings.ToUpper(strings.TrimSpace(query.Currency))
	currency := requestedCurrency

	store := m.ContractStore()
	var contracts []Contract
	if store != nil {
		var err error
		contracts, err = store.ListContracts(ctx, companyUUID)
		if err != nil {
			return nil, fmt.Errorf("IFRS 15 disclosures: list contracts: %w", err)
		}
	}

	pointInTimeTotal := decimal.Zero
	overTimeTotal := decimal.Zero
	totalContractAssets := decimal.Zero
	totalContractLiabilities := decimal.Zero
	totalBilled := decimal.Zero
	totalRecognized := decimal.Zero
	totalRemainingPrice := decimal.Zero
	remainingObligationsCount := 0
	remainingBreakdown := make([]map[string]any, 0)

	for _, c := range contracts {
		if requestedCurrency != "" && !strings.EqualFold(c.Currency, requestedCurrency) {
			continue
		}
		if currency == "" {
			currency = strings.ToUpper(c.Currency)
		} else if !strings.EqualFold(currency, c.Currency) {
			return nil, fmt.Errorf(
				"IFRS 15 disclosures: contracts use multiple currencies; specify a currency in the query",
			)
		}

		asset, liability, err := c.ContractBalances()
		if err != nil {
			return nil, fmt.Errorf("IFRS 15 disclosures: contract %s balances: %w", c.ID, err)
		}

		totalBilled = totalBilled.Add(c.BilledAmount.Amount())
		totalRecognized = totalRecognized.Add(c.RecognizedAmount.Amount())
		totalContractAssets = totalContractAssets.Add(asset.Amount())
		totalContractLiabilities = totalContractLiabilities.Add(liability.Amount())

		for _, obl := range c.Obligations {
			allocated := c.AllocatedPrices[obl.ID].Amount()
			var recognized decimal.Decimal
			if c.RecognizedPrices != nil {
				recognized = c.RecognizedPrices[obl.ID].Amount()
			}
			if obl.RecognitionPattern == PointInTime {
				pointInTimeTotal = pointInTimeTotal.Add(recognized)
			} else {
				overTimeTotal = overTimeTotal.Add(recognized)
			}

			if allocated.GreaterThan(recognized) {
				remAmount := allocated.Sub(recognized)
				totalRemainingPrice = totalRemainingPrice.Add(remAmount)
				remainingObligationsCount++
				remainingBreakdown = append(remainingBreakdown, map[string]any{
					"contract_id":        c.ID.String(),
					"obligation_id":      obl.ID.String(),
					"description":        obl.Description,
					"pattern":            string(obl.RecognitionPattern),
					"allocated_amount":   allocated.StringFixed(2),
					"recognized_amount":  recognized.StringFixed(2),
					"unsatisfied_amount": remAmount.StringFixed(2),
				})
			}
		}
	}

	if currency == "" {
		currency = "USD"
	}

	totalRevenue := pointInTimeTotal.Add(overTimeTotal)
	source := []reporting.DisclosureSource{
		{Kind: "subledger", Reference: "ifrs15_revenue_contracts"},
	}

	disclosures := []reporting.StructuredDisclosure{
		{
			CompanyID:         query.CompanyID,
			Code:              "IFRS15-REV-DISAGG",
			Title:             "Disaggregation of Revenue from Contracts with Customers",
			StandardReference: "IFRS 15",
			Period:            query.Period,
			Currency:          currency,
			Version:           "1.0.0",
			Source:            source,
			Data: map[string]any{
				"point_in_time_revenue": pointInTimeTotal.StringFixed(2),
				"over_time_revenue":     overTimeTotal.StringFixed(2),
				"total_revenue":         totalRevenue.StringFixed(2),
				"contracts_count":       len(contracts),
			},
			Narrative: "Revenue from contracts with customers disaggregated by pattern of transfer in accordance with IFRS 15.114-115.",
		},
		{
			CompanyID:         query.CompanyID,
			Code:              "IFRS15-CONTRACT-BALANCES",
			Title:             "Contract Assets and Contract Liabilities",
			StandardReference: "IFRS 15",
			Period:            query.Period,
			Currency:          currency,
			Version:           "1.0.0",
			Source:            source,
			Data: map[string]any{
				"total_contract_assets":      totalContractAssets.StringFixed(2),
				"total_contract_liabilities": totalContractLiabilities.StringFixed(2),
				"total_billed":               totalBilled.StringFixed(2),
				"total_recognized":           totalRecognized.StringFixed(2),
				"contracts_count":            len(contracts),
			},
			Narrative: "Balances of contract assets and contract liabilities resulting from contracts with customers in accordance with IFRS 15.116-118.",
		},
		{
			CompanyID:         query.CompanyID,
			Code:              "IFRS15-REMAINING-OBLIGATIONS",
			Title:             "Transaction Price Allocated to Remaining Performance Obligations",
			StandardReference: "IFRS 15",
			Period:            query.Period,
			Currency:          currency,
			Version:           "1.0.0",
			Source:            source,
			Data: map[string]any{
				"unsatisfied_amount": totalRemainingPrice.StringFixed(2),
				"obligations_count":  remainingObligationsCount,
				"breakdown":          remainingBreakdown,
			},
			Narrative: "Aggregate transaction price allocated to performance obligations that are unsatisfied (or partially unsatisfied) at reporting period end in accordance with IFRS 15.120.",
		},
	}

	return disclosures, nil
}
