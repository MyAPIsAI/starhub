package controller

import (
	"math"
	"testing"

	"starhub/common"
	"starhub/relay"
	"starhub/setting"
)

// The plaza quotes prices; the relay charges them. If the two ever disagree,
// the catalog is lying to the customer, which is the one failure a pricing
// page cannot recover from.
//
// These tests pin the relationship rather than the numbers: they recompute the
// plaza's per-1M-token quote from the ratio, run the relay's own per-token
// billing formula over exactly one million tokens of a single kind, and assert
// the two land on the same USD figure.

// TestPlazaInputPriceMatchesRelayBilling is the AC-01 equivalent for input
// tokens: 1M prompt tokens at the plaza's quoted input price must cost the
// relay's billed quota, converted back to USD.
func TestPlazaInputPriceMatchesRelayBilling(t *testing.T) {
	for _, model := range []string{"gpt-4o", "claude-sonnet-4-6", "deepseek-chat"} {
		// Plaza side: the number a shopper reads.
		quoted := resolvePlazaPrice(model, common.DefaultGroup, "")
		if quoted.InputPerM == nil {
			t.Fatalf("%s: plaza quoted no input price", model)
		}

		// Relay side: bill exactly 1M prompt tokens, no completion, no cache,
		// through the same production entry point the relay uses.
		pd := relay.NewPriceData(model, common.DefaultGroup)
		quota := relay.ComputeUsageQuota(pd, &relay.Usage{
			PromptTokens:     1_000_000,
			CompletionTokens: 0,
		})

		billedUSD := float64(quota) / float64(common.QuotaPerUSD)

		// One quota unit of rounding slack: ComputeUsageQuota rounds to an
		// integer quota, which at 500k quota/USD is a sub-cent difference.
		if math.Abs(billedUSD-*quoted.InputPerM) > 1.0/float64(common.QuotaPerUSD) {
			t.Errorf("%s: plaza quotes $%.6f/1M input but relay bills $%.6f/1M",
				model, *quoted.InputPerM, billedUSD)
		}
	}
}

// TestPlazaOutputPriceMatchesRelayBilling checks the completion axis, which is
// where a completion_ratio mistake would hide: the plaza multiplies the input
// price by the completion ratio, exactly as the relay multiplies output tokens.
func TestPlazaOutputPriceMatchesRelayBilling(t *testing.T) {
	for _, model := range []string{"gpt-4o", "claude-sonnet-4-6"} {
		quoted := resolvePlazaPrice(model, common.DefaultGroup, "")
		if quoted.OutputPerM == nil {
			t.Fatalf("%s: plaza quoted no output price", model)
		}

		pd := relay.NewPriceData(model, common.DefaultGroup)
		quota := relay.ComputeUsageQuota(pd, &relay.Usage{
			PromptTokens:     0,
			CompletionTokens: 1_000_000,
		})
		billedUSD := float64(quota) / float64(common.QuotaPerUSD)

		if math.Abs(billedUSD-*quoted.OutputPerM) > 1.0/float64(common.QuotaPerUSD) {
			t.Errorf("%s: plaza quotes $%.6f/1M output but relay bills $%.6f/1M",
				model, *quoted.OutputPerM, billedUSD)
		}
	}
}

// TestPlazaGroupRatioMatchesRelayBilling verifies that the group multiplier the
// plaza bakes into its quote is the same one the relay applies. A VIP browsing
// the plaza must see the price they will actually pay.
func TestPlazaGroupRatioMatchesRelayBilling(t *testing.T) {
	if err := setting.UpdateRatio("group_ratio", `{"default":1,"vip":0.6}`); err != nil {
		t.Fatalf("configure group ratio: %v", err)
	}
	t.Cleanup(func() { setting.ResetRatiosToDefaults() })

	const model = "gpt-4o"

	base := resolvePlazaPrice(model, common.DefaultGroup, "")
	vip := resolvePlazaPrice(model, "vip", "")
	if base.InputPerM == nil || vip.InputPerM == nil {
		t.Fatal("missing quoted input price")
	}
	if math.Abs(*vip.InputPerM-*base.InputPerM*0.6) > 1e-9 {
		t.Errorf("vip quote $%.6f is not 0.6x the default quote $%.6f",
			*vip.InputPerM, *base.InputPerM)
	}

	pd := relay.NewPriceData(model, "vip")
	quota := relay.ComputeUsageQuota(pd, &relay.Usage{PromptTokens: 1_000_000})
	billedUSD := float64(quota) / float64(common.QuotaPerUSD)

	if math.Abs(billedUSD-*vip.InputPerM) > 1.0/float64(common.QuotaPerUSD) {
		t.Errorf("vip plaza quote $%.6f/1M but relay bills $%.6f/1M", *vip.InputPerM, billedUSD)
	}
}

// TestPlazaCachePriceMatchesRelayBilling covers the cache axis. The plaza
// derives the cache-read price from cache_ratio the same way the relay
// discounts cached prompt tokens.
func TestPlazaCachePriceMatchesRelayBilling(t *testing.T) {
	const model = "gpt-4o"
	quoted := resolvePlazaPrice(model, common.DefaultGroup, "")
	if quoted.CacheReadPerM == nil {
		t.Fatal("plaza quoted no cache read price")
	}

	pd := relay.NewPriceData(model, common.DefaultGroup)
	// CachedTokens is a subset of prompt tokens, so bill 1M prompt of which
	// all 1M are cache hits.
	quota := relay.ComputeUsageQuota(pd, &relay.Usage{
		PromptTokens:        1_000_000,
		PromptTokensDetails: &relay.PromptTokensDetails{CachedTokens: 1_000_000},
	})
	billedUSD := float64(quota) / float64(common.QuotaPerUSD)

	if math.Abs(billedUSD-*quoted.CacheReadPerM) > 1.0/float64(common.QuotaPerUSD) {
		t.Errorf("plaza quotes $%.6f/1M cache read but relay bills $%.6f/1M",
			*quoted.CacheReadPerM, billedUSD)
	}
}

// TestPlazaPerCallPriceMatchesRelayBilling covers the per-call axis, where the
// plaza must quote the same flat price the relay charges per request.
func TestPlazaPerCallPriceMatchesRelayBilling(t *testing.T) {
	if err := setting.UpdatePricing("model_price", `{"dall-e":0.04}`); err != nil {
		t.Fatalf("configure model price: %v", err)
	}
	t.Cleanup(func() { setting.ResetPricingToDefaults() })

	quoted := resolvePlazaPrice("dall-e", common.DefaultGroup, "")
	if quoted.PerRequest == nil {
		t.Fatal("plaza quoted no per-call price")
	}
	if quoted.Mode != "per_request" {
		t.Fatalf("expected per_request mode, got %q", quoted.Mode)
	}

	pd := relay.NewPriceData("dall-e", common.DefaultGroup)
	quota := relay.ComputeUsageQuota(pd, &relay.Usage{PromptTokens: 100, CompletionTokens: 100})
	billedUSD := float64(quota) / float64(common.QuotaPerUSD)

	if math.Abs(billedUSD-*quoted.PerRequest) > 1.0/float64(common.QuotaPerUSD) {
		t.Errorf("plaza quotes $%.6f/call but relay bills $%.6f/call", *quoted.PerRequest, billedUSD)
	}
}

// TestDiscountRatioThreshold pins the 5% floor. Discounts smaller than that are
// noise that devalues the badges that matter, so both the sort and the UI drop
// them; this test keeps the server and client thresholds from drifting apart.
func TestDiscountRatioThreshold(t *testing.T) {
	p := func(v float64) *float64 { return &v }

	cases := []struct {
		name     string
		platform float64
		official float64
		want     float64 // 0 means "below the badge threshold"
	}{
		{"deep discount", 3, 5, 0.4},
		{"just over threshold", 9, 10, 0.1},
		{"at threshold", 9.5, 10, 0},
		{"below threshold", 9.8, 10, 0},
		{"markup", 6, 5, 0},
		{"no official price", 5, 0, 0},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			row := plazaRow{Pricing: plazaPricingJSON{
				Mode:               "per_token",
				InputPerM:          p(c.platform),
				OfficialInputPerM:  p(c.official),
			}}
			got := discountRatio(&row)
			if math.Abs(got-c.want) > 1e-9 {
				t.Errorf("discountRatio = %v, want %v", got, c.want)
			}
		})
	}
}
