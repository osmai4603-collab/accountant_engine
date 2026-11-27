package contract_parity

import (
	"reflect"
	"testing"

	publicledger "github.com/osmai4603-collab/accounting_engine/pkg/api/v1/ledger"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
)

// TestPhase11_PublicLedgerBridgePreservesReferenceFieldContract is the public
// half of G-016. It proves that the sanctioned pkg/api/v1 bridge exposes the
// same authority/reference field relationship already asserted inside the
// kernel, including the explicitly documented drop set.
func TestPhase11_PublicLedgerBridgePreservesReferenceFieldContract(t *testing.T) {
	assertFields := func(authority, dto reflect.Type, dtoOnly, dropped map[string]bool) {
		t.Helper()
		authorityFields := map[string]reflect.Kind{}
		for i := 0; i < authority.NumField(); i++ {
			field := authority.Field(i)
			authorityFields[field.Name] = field.Type.Kind()
		}
		dtoFields := map[string]reflect.Kind{}
		for i := 0; i < dto.NumField(); i++ {
			field := dto.Field(i)
			dtoFields[field.Name] = field.Type.Kind()
		}
		for name, kind := range dtoFields {
			if dtoOnly[name] {
				continue
			}
			authorityKind, exists := authorityFields[name]
			if !exists {
				t.Errorf("%s has no authority field in %s", name, authority.Name())
				continue
			}
			if authorityKind != kind {
				t.Errorf("field %s kind drift: %s vs %s", name, authorityKind, kind)
			}
		}
		documented := map[string]bool{}
		for name := range dropped {
			documented[name] = true
		}
		for name := range dtoFields {
			documented[name] = true
		}
		for name := range authorityFields {
			if !documented[name] {
				t.Errorf("authority field %s is silently dropped from %s", name, dto.Name())
			}
		}
		for name := range dropped {
			if _, exists := authorityFields[name]; !exists {
				t.Errorf("documented dropped field %s no longer exists in %s", name, authority.Name())
			}
		}
	}

	assertFields(
		reflect.TypeOf(types.Account{}), reflect.TypeOf(publicledger.Account{}), nil,
		map[string]bool{
			"IsControlAccount": true, "IsContraAccount": true, "RequiresCostCenter": true,
			"IsMonetary": true, "IsCashEquivalent": true, "IsNonCashAdjustment": true, "CashFlowActivity": true,
			"IFRSCategory": true, "DisplayOrder": true, "EffectiveFrom": true,
			"EffectiveTo": true, "CreatedAt": true, "UpdatedAt": true,
		},
	)
	assertFields(reflect.TypeOf(types.FiscalPeriod{}), reflect.TypeOf(publicledger.FiscalPeriod{}), nil, nil)
	assertFields(reflect.TypeOf(types.FiscalYear{}), reflect.TypeOf(publicledger.FiscalYear{}), map[string]bool{"Periods": true}, nil)
}
