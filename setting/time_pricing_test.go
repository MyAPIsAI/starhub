package setting

import (
	"testing"
	"time"
)

const timePricingFixture = `{"gpt-4o":{"timezone":"UTC","windows":[{"id":"all","label":"All week","days":[6,5,4,3,2,1,0],"start":"00:00","end":"24:00","multiplier":0.8}]}}`

func TestTimePricingOptionRegistryAndValidatedLoading(t *testing.T) {
	ResetAllToDefaults()
	t.Cleanup(ResetAllToDefaults)
	if !IsOptionKey(OptionKeyModelTimePricing) {
		t.Fatal("time pricing is not registered")
	}
	found := 0
	for _, key := range OptionKeys() {
		if key == OptionKeyModelTimePricing {
			found++
		}
	}
	if found != 1 {
		t.Fatal("time pricing key must be seeded once")
	}
	if value, err := SnapshotOption(OptionKeyModelTimePricing); err != nil || value != "{}" {
		t.Fatalf("default snapshot: %s %v", value, err)
	}
	canonical, err := NormalizeOption(OptionKeyModelTimePricing, timePricingFixture)
	if err != nil {
		t.Fatal(err)
	}
	if err := UpdateOption(OptionKeyModelTimePricing, timePricingFixture); err != nil {
		t.Fatal(err)
	}
	if value, err := SnapshotOption(OptionKeyModelTimePricing); err != nil || value != canonical {
		t.Fatal("loaded schedule is not canonical")
	}
	for _, raw := range []string{`null`, `{"m":{"timezone":"Local","windows":[]}}`, `{"m":{"timezone":"UTC","windows":null}}`} {
		if err := UpdateOption(OptionKeyModelTimePricing, raw); err == nil {
			t.Fatal("invalid schedule loaded")
		}
		if value, _ := SnapshotOption(OptionKeyModelTimePricing); value != canonical {
			t.Fatal("rejected schedule changed current rules")
		}
	}
	if got := TimePricingAt("gpt-4o", time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)); got.Multiplier != 0.8 || got.WindowID != "all" {
		t.Fatalf("loaded schedule does not match: %+v", got)
	}
	if err := ApplyOptions(map[string]string{OptionKeyModelTimePricing: "{}", OptionKeyCacheRatio: "invalid"}); err == nil {
		t.Fatal("invalid option batch applied")
	}
	if value, _ := SnapshotOption(OptionKeyModelTimePricing); value != canonical {
		t.Fatal("partial option apply erased time rules")
	}
	ResetPricingToDefaults()
	if got := TimePricingAt("gpt-4o", time.Time{}); got.Multiplier != 1 || got.WindowID != "" {
		t.Fatal("reset retained time adjustment")
	}
}
