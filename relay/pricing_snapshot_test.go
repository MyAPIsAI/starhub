package relay

import (
	"sync"
	"testing"

	"starhub/setting"
)

func TestCapturedPricingRetainsEveryEligibleGroupAndAllPriceInputs(t *testing.T) {
	setting.ResetAllToDefaults()
	t.Cleanup(setting.ResetAllToDefaults)
	first := map[string]string{"model_ratio": `{"captured":2}`, "completion_ratio": `{"captured":3}`, "group_ratio": `{"default":1,"vip":0.5}`, "group_group_ratio": `{"vip":{"default":0.8,"vip":0.9}}`, "model_price": `{"captured":0.01}`, "model_price_expr": `{"captured":"total_tokens * 0.001"}`, "cache_ratio": "0.25", "audio_ratio": "12"}
	if err := setting.ApplyOptions(first); err != nil {
		t.Fatal(err)
	}
	prices := CapturePricing("captured", []string{"default", "vip"}, "vip")
	if err := setting.ApplyOptions(map[string]string{"model_ratio": `{"captured":20}`, "completion_ratio": `{"captured":30}`, "group_ratio": `{"default":2,"vip":3}`, "cache_ratio": "1", "audio_ratio": "40", "model_price": "{}", "model_price_expr": "{}"}); err != nil {
		t.Fatal(err)
	}
	for group, price := range prices {
		if price.ModelRatio != 2 || price.CompletionRatio != 3 || price.CacheRatio != 0.25 || price.AudioRatio != 12 || price.ModelPriceUSD != 0.01 || price.PriceExpr != "total_tokens * 0.001" {
			t.Fatalf("captured %s prices changed: %+v", group, price)
		}
	}
	if prices["default"].GroupRatio != 1 || prices["default"].GroupGroupRatio != 0.8 || prices["vip"].GroupRatio != 0.5 || prices["vip"].GroupGroupRatio != 0.9 {
		t.Fatal("group or account/group prices were not captured")
	}
	copy := prices["default"]
	copy.AddOtherRatio(7)
	if prices["default"].OtherRatio != 1 {
		t.Fatal("per-attempt multiplier mutated captured baseline")
	}
}

func TestCapturedGroupPricesShareOneAtomicOptionVersion(t *testing.T) {
	setting.ResetAllToDefaults()
	t.Cleanup(setting.ResetAllToDefaults)
	values := []map[string]string{{"model_ratio": `{"m":2}`, "completion_ratio": `{"m":3}`, "group_ratio": `{"default":1,"vip":0.5}`}, {"model_ratio": `{"m":20}`, "completion_ratio": `{"m":30}`, "group_ratio": `{"default":2,"vip":3}`}}
	if err := setting.ApplyOptions(values[0]); err != nil {
		t.Fatal(err)
	}
	var writer sync.WaitGroup
	writer.Go(func() {
		for i := 0; i < 100; i++ {
			if err := setting.ApplyOptions(values[i%2]); err != nil {
				t.Error(err)
			}
		}
	})
	defer writer.Wait()
	for i := 0; i < 100; i++ {
		prices := CapturePricing("m", []string{"default", "vip"}, "")
		a, b := prices["default"], prices["vip"]
		if a.ModelRatio == 2 {
			if a.CompletionRatio != 3 || a.GroupRatio != 1 || b.ModelRatio != 2 || b.CompletionRatio != 3 || b.GroupRatio != 0.5 {
				t.Fatal("mixed first pricing version")
			}
		} else if a.ModelRatio == 20 {
			if a.CompletionRatio != 30 || a.GroupRatio != 2 || b.ModelRatio != 20 || b.CompletionRatio != 30 || b.GroupRatio != 3 {
				t.Fatal("mixed second pricing version")
			}
		} else {
			t.Fatal("unknown price version")
		}
	}
	writer.Wait()
}
