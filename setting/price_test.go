package setting

import (
	"strings"
	"testing"
)

func resetPricing(t *testing.T) {
	t.Helper()
	updates := []struct{ key, value string }{
		{OptionKeyModelPrice, `{}`},
		{OptionKeyModelPriceExpr, `{}`},
		{OptionKeyCacheRatio, `0.5`},
		{OptionKeyAudioRatio, `40`},
	}
	for _, u := range updates {
		if err := UpdatePricing(u.key, u.value); err != nil {
			t.Fatalf("reset %s failed: %v", u.key, err)
		}
	}
}

func TestPricingOptionDispatcher(t *testing.T) {
	resetPricing(t)

	if !IsOptionKey(OptionKeyModelPrice) || !IsOptionKey(OptionKeyGroupRatio) {
		t.Fatal("pricing and ratio keys should be recognized")
	}
	if IsOptionKey("bogus") {
		t.Fatal("unknown key must be rejected")
	}
	keys := OptionKeys()
	if len(keys) != 12 {
		t.Fatalf("OptionKeys = %v, want 12 keys", keys)
	}
	if err := UpdateOption("no_such_key", "1"); err == nil {
		t.Fatal("UpdateOption should reject unknown keys")
	}
	if err := UpdateOption(OptionKeyGroupRatio, `{"default":1}`); err != nil {
		t.Fatalf("UpdateOption should dispatch ratio keys: %v", err)
	}
	if err := UpdateOption(OptionKeyAudioRatio, `12.5`); err != nil {
		t.Fatalf("UpdateOption should dispatch pricing keys: %v", err)
	}
}

func TestModelPriceUpdateAndRead(t *testing.T) {
	resetPricing(t)

	if err := UpdatePricing(OptionKeyModelPrice, `{"gpt-4o":0.002,"dall-e-3":0.04}`); err != nil {
		t.Fatalf("UpdatePricing failed: %v", err)
	}
	v, ok := ModelPriceValue("dall-e-3")
	if !ok || v != 0.04 {
		t.Fatalf("dall-e-3 price = %v %v", v, ok)
	}
	if _, ok := ModelPriceValue("unknown"); ok {
		t.Fatal("unknown model should have no fixed price")
	}
	snapshot, err := SnapshotPricing(OptionKeyModelPrice)
	if err != nil {
		t.Fatalf("SnapshotPricing failed: %v", err)
	}
	want := `{"dall-e-3":0.04,"gpt-4o":0.002}`
	if snapshot != want {
		t.Fatalf("snapshot = %q, want %q", snapshot, want)
	}

	if err := UpdatePricing(OptionKeyModelPrice, `{"m":-1}`); err == nil {
		t.Fatal("negative price must be rejected")
	}
	if err := UpdatePricing(OptionKeyModelPrice, `not json`); err == nil {
		t.Fatal("invalid JSON must be rejected")
	}
}

func TestModelPriceExprValidation(t *testing.T) {
	resetPricing(t)

	good := `{"tiered":"completion_tokens <= 1000 ? 0.001 : 0.01"}`
	if err := UpdatePricing(OptionKeyModelPriceExpr, good); err != nil {
		t.Fatalf("valid expression must be accepted: %v", err)
	}
	expr, ok := ModelPriceExpr("tiered")
	if !ok || expr != "completion_tokens <= 1000 ? 0.001 : 0.01" {
		t.Fatalf("tiered expr = %q %v", expr, ok)
	}

	bad := []string{
		`{"m":"1 +"}`,     // syntax error
		`{"m":"hack(1)"}`, // non-whitelisted function
		`{"m":"min()"}`,   // arity error
	}
	for _, value := range bad {
		if err := UpdatePricing(OptionKeyModelPriceExpr, value); err == nil {
			t.Fatalf("expression %s must be rejected", value)
		}
	}
	// over-length expressions are rejected before they can ever run
	longExpr := `{"m":"` + strings.Repeat("1+", 300) + `1"}`
	if err := UpdatePricing(OptionKeyModelPriceExpr, longExpr); err == nil {
		t.Fatal("over-length expression must be rejected")
	}
}

func TestDetailRatios(t *testing.T) {
	resetPricing(t)

	if CacheRatioValue() != 0.5 {
		t.Fatalf("default cache ratio = %v", CacheRatioValue())
	}
	if AudioRatioValue() != 40 {
		t.Fatalf("default audio ratio = %v", AudioRatioValue())
	}
	if err := UpdatePricing(OptionKeyCacheRatio, `0.25`); err != nil {
		t.Fatalf("cache ratio update failed: %v", err)
	}
	if got := CacheRatioValue(); got != 0.25 {
		t.Fatalf("cache ratio = %v, want 0.25", got)
	}
	snapshot, err := SnapshotPricing(OptionKeyCacheRatio)
	if err != nil || snapshot != "0.25" {
		t.Fatalf("cache ratio snapshot = %q %v", snapshot, err)
	}
	for _, bad := range []string{`-1`, `notjson`, `1001`, `{"a":1}`} {
		if err := UpdatePricing(OptionKeyCacheRatio, bad); err == nil {
			t.Fatalf("cache ratio %q must be rejected", bad)
		}
	}
}
