package model

import (
	"encoding/json"
	"testing"

	"starhub/setting"
)

func TestPlatformEffectivePricing(t *testing.T) {
	p := setting.PricingInputs{ModelRatio: 1, CompletionRatio: 4, CacheRatio: .5, AudioRatio: 40}
	quote := func(p setting.PricingInputs) (map[string]any, map[string]any) {
		t.Helper()
		result := platformCatalogPricingInputs("glm-5.2", p)
		return result, result["cost_quotes"].([]any)[0].(map[string]any)
	}
	r, q := quote(p)
	if q["cost"] != "2.500000000000" || q["error"] != "" || r["price_source"] != "default_fallback" {
		t.Fatalf("invalid fallback quote: %+v", r)
	}
	rates := r["unit_prices"].(map[string]any)
	if rates["input"] != "1.000000000000" || rates["output"] != "4.000000000000" || rates["cache_read"] != "0.500000000000" {
		t.Fatalf("invalid unit prices: %+v", rates)
	}
	_, repeated := quote(p)
	if q["version"] != repeated["version"] {
		t.Fatal("unstable version")
	}
	p.CacheRatio = .25
	_, changed := quote(p)
	if q["version"] == changed["version"] {
		t.Fatal("cache price did not change version")
	}
	p.HasModelRatio, p.HasCompletion = true, true
	r, _ = quote(p)
	if r["price_source"] != "configured" {
		t.Fatal("configured provenance lost")
	}
	p.HasModelPrice, p.ModelPriceUSD = true, 0
	r, _ = quote(p)
	if r["billing_mode"] != "token" {
		t.Fatal("zero flat option changed runtime billing mode")
	}
	p.ModelRatio = 0
	_, q = quote(p)
	if q["cost"] != "0.000000000000" || q["error"] != "" {
		t.Fatal("free tariff lost")
	}
	p.ModelPriceUSD = .05
	r, q = quote(p)
	if r["billing_mode"] != "per_call" || q["cost"] != "0.050000000000" {
		t.Fatal("per-call mode changed")
	}
	p.PriceExpr = "prompt_tokens"
	r, q = quote(p)
	if r["billing_mode"] != "tiered_expr" || q["error"] != "context_required" {
		t.Fatal("expression fell back")
	}
	if _, err := json.Marshal(r); err != nil {
		t.Fatal(err)
	}
}
