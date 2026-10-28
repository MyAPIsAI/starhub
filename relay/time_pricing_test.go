package relay

import (
	"encoding/json"
	"testing"
	"time"

	"starhub/relay/channel"
	"starhub/setting"
)

func TestTimePricingIsFrozenAtRequestInstantAcrossBillingModes(t *testing.T) {
	setting.ResetAllToDefaults()
	t.Cleanup(setting.ResetAllToDefaults)
	rules := map[string]any{}
	for _, model := range []string{"token", "flat", "expression", "image", "task"} {
		rules[model] = map[string]any{"timezone": "UTC", "windows": []map[string]any{{"id": "night", "label": "Night", "days": []int{1}, "start": "23:00", "end": "06:00", "multiplier": 0.5}}}
	}
	raw, _ := json.Marshal(rules)
	if err := setting.ApplyOptions(map[string]string{"model_ratio": `{"token":2}`, "completion_ratio": `{"token":3}`, "model_price": `{"flat":0.02,"image":0.02,"task":0.02}`, "model_price_expr": `{"expression":"total_tokens * 0.000002"}`, setting.OptionKeyModelTimePricing: string(raw)}); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 10, 26, 23, 30, 0, 0, time.UTC)
	token := NewPriceDataAt("token", "default", "", at)
	if token.TimeAdjustment.Multiplier != 0.5 || token.TimeAdjustment.WindowID != "night" || !token.PricingTime.Equal(at) {
		t.Fatal("request instant or adjustment was not captured")
	}
	if got := ComputeUsageQuota(token, &Usage{PromptTokens: 1_000_000}); got != 500000 {
		t.Fatalf("timed token fee=%d", got)
	}
	if got := ComputeUsageQuota(NewPriceDataAt("flat", "default", "", at), &Usage{PromptTokens: 100}); got != 5000 {
		t.Fatalf("timed flat fee=%d", got)
	}
	if got := ComputeUsageQuota(NewPriceDataAt("expression", "default", "", at), &Usage{PromptTokens: 1000}); got != 500 {
		t.Fatalf("timed expression fee=%d", got)
	}
	image := &ImageBill{Model: "image", Group: "default", Count: 2, Size: "1024x1024", Quality: "standard", Pricing: NewPriceDataAt("image", "default", "", at)}
	if pre, actual := image.PreQuota(), image.SettleQuota(2, nil); pre != 10000 || actual != 10000 {
		t.Fatalf("timed image fees=%d/%d", pre, actual)
	}
	taskPrice := NewPriceDataAt("task", "default", "", at)
	accepted := channel.TaskPricing{PerCallUSD: 0.02, Units: 16}
	if got := TaskPricingQuotaWithPriceData(taskPrice, accepted); got != 80000 {
		t.Fatalf("timed task fee=%d", got)
	}
	encoded, err := json.Marshal(TaskBillingSnapshot{PriceData: *taskPrice, Estimate: accepted})
	if err != nil {
		t.Fatal(err)
	}
	if err := setting.UpdateOption(setting.OptionKeyModelTimePricing, "{}"); err != nil {
		t.Fatal(err)
	}
	if got := ComputeUsageQuota(token, &Usage{PromptTokens: 1_000_000}); got != 500000 {
		t.Fatal("existing request lost its time adjustment after rules changed")
	}
	var persisted TaskBillingSnapshot
	if err := json.Unmarshal(encoded, &persisted); err != nil {
		t.Fatal(err)
	}
	if got := TaskPricingQuotaWithPriceData(&persisted.PriceData, persisted.Estimate); got != 80000 || persisted.PriceData.TimeAdjustment.WindowID != "night" {
		t.Fatal("task snapshot lost its timed price")
	}
	if got := ComputeUsageQuota(NewPriceDataAt("token", "default", "", at), &Usage{PromptTokens: 1_000_000}); got != 1000000 {
		t.Fatal("new request did not use cleared time rules")
	}
	if err := setting.UpdateOption(setting.OptionKeyModelTimePricing, string(raw)); err != nil {
		t.Fatal(err)
	}
	outside := NewPriceDataAt("token", "default", "", time.Date(2026, 10, 27, 6, 0, 0, 0, time.UTC))
	if outside.TimeAdjustment.Multiplier != 1 || outside.TimeAdjustment.WindowID != "" {
		t.Fatal("exclusive time-window end was not respected")
	}
	groups := CapturePricingAt("token", []string{"default", "vip"}, "", at)
	for _, price := range groups {
		if !price.PricingTime.Equal(at) || price.TimeAdjustment.Multiplier != 0.5 {
			t.Fatal("group snapshots used different pricing instants")
		}
	}
	legacy := PriceData{ModelRatio: 2, CompletionRatio: 3, GroupRatio: 1, OtherRatio: 1}
	if got := ComputeUsageQuota(&legacy, &Usage{PromptTokens: 1_000_000}); got != 1000000 {
		t.Fatal("legacy snapshot without time fields was repriced")
	}
}
