package policy

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Fingerprint returns a stable digest of the *resolved* policy pair used for a
// posting. It is recorded on the journal entry as audit evidence so a later
// policy version cannot change the interpretation of an already posted entry
// without that change becoming visible.
//
// Only deterministic, semantic fields take part in the digest: map/slice order
// is normalized, timestamps are UTC, and an empty rounding method is resolved
// to the documented banker's default.
func Fingerprint(posting PostingPolicy, currency CurrencyPolicy) string {
	var canonical strings.Builder
	canonical.WriteString("policy-fingerprint-v1|")

	appendField(&canonical, "company=%s|", posting.CompanyID.String())
	appendField(&canonical, "posting.start=%s|", fingerprintTime(posting.EffectiveDate))
	appendField(&canonical, "posting.end=%s|", fingerprintEnd(posting.EffectiveTo))
	appendField(&canonical, "posting.max_backdating_days=%d|", posting.MaxBackdatingDays)
	appendField(&canonical, "posting.backdating_exception=%t|", posting.AllowBackdatingException)
	appendField(&canonical, "posting.soft_close=%t|", posting.AllowSoftClosedPosting)
	appendField(&canonical, "posting.cost_center=%t|", posting.RequireCostCenterForIncomeExpense)
	appendField(&canonical, "posting.dimension=%t|", posting.RequireAnalyticalDimensionForIncomeExpense)

	appendField(&canonical, "currency.start=%s|", fingerprintTime(currency.EffectiveDate))
	appendField(&canonical, "currency.end=%s|", fingerprintEnd(currency.EffectiveTo))
	appendField(&canonical, "currency.functional=%s|", strings.ToUpper(strings.TrimSpace(currency.FunctionalCurrency)))
	appendField(&canonical, "currency.presentation=%s|", strings.ToUpper(strings.TrimSpace(currency.PresentationCurrency)))
	method := currency.RoundingMethod
	if method == "" {
		method = RoundingBankers
	}
	appendField(&canonical, "currency.rounding=%s|", method)
	appendField(&canonical, "currency.auto_settle=%t|", currency.AutoSettleRoundingDifferences)
	appendField(&canonical, "currency.rounding_account=%s|", currency.RoundingDifferenceAccountID.String())

	allowed := append([]string(nil), currency.AllowedCurrencies...)
	for i := range allowed {
		allowed[i] = strings.ToUpper(strings.TrimSpace(allowed[i]))
	}
	sort.Strings(allowed)
	appendField(&canonical, "currency.allowed=%s|", strings.Join(allowed, ","))

	digest := sha256.Sum256([]byte(canonical.String()))
	return "sha256:" + fmt.Sprintf("%x", digest[:])
}

func appendField(builder *strings.Builder, format string, args ...any) {
	builder.WriteString(fmt.Sprintf(format, args...))
}

func fingerprintTime(value time.Time) string {
	if value.IsZero() {
		return "-"
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func fingerprintEnd(value *time.Time) string {
	if value == nil || normalizedPolicyTime(*value).IsZero() {
		return "-"
	}
	return fingerprintTime(*value)
}
