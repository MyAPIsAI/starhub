package common

import (
	"log"
	"math"
)

// QuotaSaturation marks billing anomalies (overflow, non-finite or negative
// intermediate values, clamped multipliers) in the server log. Operations
// teams can grep for this marker to audit every request whose quota math had
// to be clamped.
const QuotaSaturation = "quota_saturation"

// MaxQuotaPerRequest caps the quota a single request may ever reserve or
// consume (~$2.2M). Anything above this is the result of misconfigured
// ratios or hostile input and is clamped (with an audit line) instead of
// overflowing int64, where wrap-around could turn the value negative and
// effectively credit the user money.
const MaxQuotaPerRequest = int64(1) << 40

// MaxRatioProduct bounds the product of all billing multipliers
// (group ratio x special ratios such as audio / image resolution / video
// seconds). It keeps the float math far away from int64 overflow territory
// even before QuotaFromFloat64 does its final saturation.
const MaxRatioProduct = 1e6

// QuotaFromFloat64 converts a quota amount in float64 to int64 with
// saturating semantics:
//
//	negative / NaN / Inf -> 0 (plus a quota_saturation audit line)
//	above MaxQuotaPerRequest -> MaxQuotaPerRequest (plus audit line)
//
// The caller keeps responsibility for any minimum-billing clamp (e.g. "any
// real usage bills at least 1").
func QuotaFromFloat64(v float64) int64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		log.Printf("[%s] non-finite quota value %v clamped to 0", QuotaSaturation, v)
		return 0
	}
	if v < 0 {
		log.Printf("[%s] negative quota value %v clamped to 0", QuotaSaturation, v)
		return 0
	}
	if v >= float64(MaxQuotaPerRequest) {
		log.Printf("[%s] quota value %v exceeds cap %d, clamped", QuotaSaturation, v, MaxQuotaPerRequest)
		return MaxQuotaPerRequest
	}
	return int64(math.Round(v))
}

// ClampRatio bounds a single billing multiplier to [0, MaxRatioProduct] and
// audits when the value had to be corrected. Invalid multipliers collapse to
// 0 — a request that cannot be priced sanely bills nothing rather than
// wrapping around and paying the user.
func ClampRatio(r float64, what string) float64 {
	if math.IsNaN(r) || math.IsInf(r, 0) {
		log.Printf("[%s] %s ratio %v is not finite, clamped to 0", QuotaSaturation, what, r)
		return 0
	}
	if r < 0 {
		log.Printf("[%s] %s ratio %v is negative, clamped to 0", QuotaSaturation, what, r)
		return 0
	}
	if r > MaxRatioProduct {
		log.Printf("[%s] %s ratio %v exceeds cap %v, clamped", QuotaSaturation, what, r, MaxRatioProduct)
		return MaxRatioProduct
	}
	return r
}
