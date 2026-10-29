package setting

import (
	"errors"
	"testing"
	"time"
)

const independentRules = `{"independent":{"timezone":"UTC","windows":[{"id":"morning","label":"Morning","days":[4],"start":"09:00","end":"10:00","prices":{"input_per_m":"1.23","output_per_m":"4.56","cache_read_per_m":"0","cache_write_per_m":"2.5","audio_input_per_m":"8","audio_output_per_m":"16"}}]}}`

func TestIndependentTimePricingAtomicSettingsAndSnapshots(t *testing.T) {
	ResetAllToDefaults()
	t.Cleanup(ResetAllToDefaults)
	if !IsOptionKey(OptionKeyModelTimeTokenPrices) {
		t.Fatal("independent prices missing from registry")
	}
	if err := ApplyOptions(map[string]string{OptionKeyModelTimeTokenPrices: independentRules, OptionKeyGroupRatio: `{"default":1,"vip":0.5}`, OptionKeyModelTimePricing: `{"independent":{"timezone":"UTC","windows":[{"id":"boost","label":"Boost","days":[4],"start":"09:00","end":"11:00","multiplier":2}]}}`}); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 10, 29, 9, 30, 0, 0, time.UTC)
	frozen := ResolvePricingGroupsAt("independent", []string{"default", "vip"}, "", at)
	if !frozen["default"].TokenRates.Valid() || frozen["default"].TokenRates.Prices().InputPerM != "1.23" || frozen["default"].TimeAdjustment.Multiplier != 2 || frozen["vip"].GroupRatio != 0.5 {
		t.Fatal("absolute rates and stacked group/time multipliers were not captured")
	}
	outside := ResolvePricingAt("independent", "default", "", at.Add(30*time.Minute))
	if outside.TokenRates.Valid() || outside.TokenAdjustment.WindowID != "" || outside.TokenAdjustment.Timezone != "UTC" || outside.TimeAdjustment.Multiplier != 2 {
		t.Fatal("outside token window must retain only multiplier and refresh metadata")
	}
	for _, values := range []map[string]string{
		{OptionKeyModelPrice: `{"independent":0.01}`},
		{OptionKeyModelPriceExpr: `{"independent":"0.01"}`},
	} {
		if err := ApplyOptions(values); !errors.Is(err, ErrTokenPriceMode) {
			t.Fatalf("incompatible mode accepted: %v", err)
		}
		for key, value := range values {
			if err := UpdatePricing(key, value); !errors.Is(err, ErrTokenPriceMode) {
				t.Fatalf("single-option update bypassed mode guard: %v", err)
			}
		}
	}
	if err := ApplyOptions(map[string]string{OptionKeyModelTimeTokenPrices: "{}", OptionKeyCacheRatio: "invalid"}); err == nil {
		t.Fatal("invalid atomic replacement accepted")
	}
	if !ResolvePricingAt("independent", "default", "", at).TokenRates.Valid() {
		t.Fatal("failed replacement leaked cleared prices")
	}
	if err := ApplyOptions(map[string]string{OptionKeyModelTimeTokenPrices: "{}", OptionKeyModelPrice: `{"independent":0.01}`}); err != nil {
		t.Fatalf("atomic retirement and mode change rejected: %v", err)
	}
	if err := UpdateOption(OptionKeyModelTimeTokenPrices, independentRules); !errors.Is(err, ErrTokenPriceMode) {
		t.Fatalf("independent windows hid a flat price: %v", err)
	}
	if !frozen["default"].TokenRates.Valid() || frozen["default"].TokenRates.Prices().InputPerM != "1.23" {
		t.Fatal("later mode change mutated frozen request prices")
	}
}
