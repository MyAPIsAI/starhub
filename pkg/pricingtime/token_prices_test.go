package pricingtime

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func tokenPriceFixture() map[string]TokenSchedule {
	return map[string]TokenSchedule{"model": {Timezone: "UTC", Windows: []TokenWindow{{ID: "night", Label: "Night", Days: []int{0}, Start: "23:00", End: "06:00", Prices: TokenPrices{InputPerM: "1.230000000", OutputPerM: "4.56", CacheReadPerM: "0", CacheWritePerM: "2.50", AudioInputPerM: "0.000000001", AudioOutputPerM: "1000000"}}}}}
}
func tokenCatalog(t *testing.T, rules map[string]TokenSchedule) *TokenCatalog {
	t.Helper()
	raw, err := json.Marshal(rules)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := DecodeTokenPrices(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}

func TestTokenPriceDecimalsAreExactCanonicalAndBounded(t *testing.T) {
	for value, want := range map[string]int64{"0": 0, "0.000000001": 1, "1.230000000": 1_230_000_000, "999999.999999999": 999_999_999_999_999, "1000000": 1_000_000_000_000_000} {
		got, err := TokenPriceNano(value)
		if err != nil || got != want {
			t.Fatalf("%s: %d %v", value, got, err)
		}
	}
	for _, value := range []string{"", "-1", "+1", ".5", "1.", "01", "1e2", "NaN", "Infinity", " 1", "1 ", "0.0000000001", "1000000.000000001", "9999999", "10000000", "1\n"} {
		if _, err := TokenPriceNano(value); err == nil {
			t.Fatalf("invalid decimal accepted: %q", value)
		}
	}
	catalog := tokenCatalog(t, tokenPriceFixture())
	if !strings.Contains(catalog.JSON(), `"input_per_m":"1.23"`) || strings.Contains(catalog.JSON(), "1.230000000") {
		t.Fatal("decimal canonicalization lost precision")
	}
	again, err := DecodeTokenPrices(catalog.JSON())
	if err != nil || again.JSON() != catalog.JSON() {
		t.Fatal("canonical payload is unstable")
	}
}

func TestTokenPriceWindowMatchesExactOvernightBoundariesAndSnapshots(t *testing.T) {
	catalog := tokenCatalog(t, tokenPriceFixture())
	for _, test := range []struct {
		at    string
		match bool
	}{{"2026-10-25T22:59:59Z", false}, {"2026-10-25T23:00:00Z", true}, {"2026-10-26T05:59:59Z", true}, {"2026-10-26T06:00:00Z", false}, {"2026-10-26T23:30:00Z", false}} {
		at, _ := time.Parse(time.RFC3339, test.at)
		got, ok := catalog.Match("model", at)
		if ok != test.match {
			t.Fatalf("boundary %s: %+v %v", test.at, got, ok)
		}
		if ok && (got.WindowID != "night" || got.Timezone != "UTC" || got.Prices.CacheReadPerM != "0" || got.Prices.CacheWritePerM != "2.5" || got.Prices.AudioInputPerM != "0.000000001") {
			t.Fatal("matched snapshot lost declared prices")
		}
		if !ok && got.Prices.InputPerM != "" {
			t.Fatal("outside-window price invented")
		}
	}
	at, _ := time.Parse(time.RFC3339, "2026-10-26T01:00:00Z")
	snapshot, _ := catalog.Match("model", at)
	snapshot.Prices.InputPerM = "999"
	snapshot.WindowID = "mutated"
	again, _ := catalog.Match("model", at)
	if again.Prices.InputPerM != "1.23" || again.WindowID != "night" {
		t.Fatal("caller mutated immutable catalog")
	}
	if _, ok := catalog.Match("unknown", at); ok {
		t.Fatal("unknown model got an invented price")
	}
	var absent *TokenCatalog
	if _, ok := absent.Match("model", at); ok {
		t.Fatal("nil catalog matched")
	}
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				got, ok := catalog.Match("model", at)
				if !ok || got.Prices.InputPerM != "1.23" {
					t.Error("concurrent snapshot changed")
				}
			}
		}()
	}
	wg.Wait()
}

func TestTokenPriceWindowsUseIANARepeatedAndSkippedLocalMinutes(t *testing.T) {
	rules := tokenPriceFixture()
	schedule := rules["model"]
	schedule.Timezone = "America/New_York"
	schedule.Windows[0].Start = "01:00"
	schedule.Windows[0].End = "02:00"
	rules["model"] = schedule
	catalog := tokenCatalog(t, rules)
	for _, value := range []string{"2026-11-01T05:30:00Z", "2026-11-01T06:30:00Z"} {
		at, _ := time.Parse(time.RFC3339, value)
		if _, ok := catalog.Match("model", at); !ok {
			t.Fatalf("repeated local minute excluded: %s", value)
		}
	}
	schedule.Windows[0].Start = "02:00"
	schedule.Windows[0].End = "03:00"
	rules["model"] = schedule
	catalog = tokenCatalog(t, rules)
	for _, value := range []string{"2026-03-08T06:30:00Z", "2026-03-08T07:30:00Z"} {
		at, _ := time.Parse(time.RFC3339, value)
		if _, ok := catalog.Match("model", at); ok {
			t.Fatal("nonexistent spring-forward hour invented")
		}
	}
}

func TestTokenPriceRulesRejectAmbiguousOrUnsupportedContracts(t *testing.T) {
	raw, _ := json.Marshal(tokenPriceFixture())
	for _, mutate := range []func(string) string{
		func(s string) string { return "null" }, func(s string) string { return s + " {}" }, func(s string) string {
			return strings.Replace(s, `"input_per_m":"1.230000000"`, `"input_per_m":1.23`, 1)
		}, func(s string) string {
			return strings.Replace(s, `"input_per_m":"1.230000000"`, `"input_per_m":null`, 1)
		}, func(s string) string { return strings.Replace(s, `"cache_read_per_m":"0",`, "", 1) }, func(s string) string { return strings.Replace(s, `"prices":{`, `"multiplier":1,"prices":{`, 1) }, func(s string) string {
			return strings.Replace(s, `"prices":{`, `"prices":{"reasoning_per_m":"1",`, 1)
		}, func(s string) string {
			return strings.Replace(s, `"timezone":"UTC"`, `"timezone":"UTC","timezone":"UTC"`, 1)
		}, func(s string) string { return strings.Replace(s, `"timezone":"UTC"`, `"timezone":"Local"`, 1) }, func(s string) string { return strings.Replace(s, `"model":`, `"myapis-auto":`, 1) }, func(s string) string { return strings.Replace(s, `"model":`, `"veyra-auto":`, 1) }, func(s string) string { return strings.Replace(s, `"days":[0]`, `"days":[0,0]`, 1) }, func(s string) string { return strings.Replace(s, `"end":"06:00"`, `"end":"23:00"`, 1) }, func(s string) string { return strings.Replace(s, `"model":`, `" model":`, 1) }, func(s string) string { return strings.Repeat(" ", MaxCatalogBytes) + s },
	} {
		if _, err := DecodeTokenPrices(mutate(string(raw))); err == nil {
			t.Fatal("unsupported token-price rules accepted")
		}
	}
	if _, err := Decode(string(raw)); err == nil {
		t.Fatal("serving multiplier decoder silently accepted inactive absolute prices")
	}
	for _, change := range []string{"overlap", "duplicate-id", "too-many-windows", "nil-windows", "too-many-models"} {
		rules := tokenPriceFixture()
		schedule := rules["model"]
		switch change {
		case "overlap":
			extra := schedule.Windows[0]
			extra.ID = "overlap"
			schedule.Windows = append(schedule.Windows, extra)
		case "duplicate-id":
			schedule.Windows = append(schedule.Windows, schedule.Windows[0])
		case "too-many-windows":
			for i := 0; i < MaxWindows; i++ {
				extra := schedule.Windows[0]
				extra.ID = fmt.Sprintf("w%d", i)
				schedule.Windows = append(schedule.Windows, extra)
			}
		case "nil-windows":
			schedule.Windows = nil
		case "too-many-models":
			for i := 0; i < MaxModels; i++ {
				rules[fmt.Sprintf("m%d", i)] = schedule
			}
		}
		rules["model"] = schedule
		encoded, _ := json.Marshal(rules)
		if _, err := DecodeTokenPrices(string(encoded)); err == nil {
			t.Fatalf("invalid %s accepted", change)
		}
	}
}

func TestTokenPriceRulesRequireEveryExplicitPriceAndAcceptWindowCapacity(t *testing.T) {
	encoded, _ := json.Marshal(tokenPriceFixture())
	for _, field := range []string{"input_per_m", "output_per_m", "cache_read_per_m", "cache_write_per_m", "audio_input_per_m", "audio_output_per_m"} {
		var rules map[string]any
		if err := json.Unmarshal(encoded, &rules); err != nil {
			t.Fatal(err)
		}
		prices := rules["model"].(map[string]any)["windows"].([]any)[0].(map[string]any)["prices"].(map[string]any)
		delete(prices, field)
		raw, _ := json.Marshal(rules)
		if _, err := DecodeTokenPrices(string(raw)); err == nil {
			t.Fatalf("missing %s inherited a price", field)
		}
	}
	rules := tokenPriceFixture()
	schedule := rules["model"]
	base := schedule.Windows[0]
	schedule.Windows = nil
	for i := 0; i < MaxWindows; i++ {
		window := base
		window.ID = fmt.Sprintf("w%02d", i)
		start, end := i*30, (i+1)*30
		window.Start = fmt.Sprintf("%02d:%02d", start/60, start%60)
		window.End = fmt.Sprintf("%02d:%02d", end/60, end%60)
		schedule.Windows = append(schedule.Windows, window)
	}
	rules["model"] = schedule
	catalog := tokenCatalog(t, rules)
	at, _ := time.Parse(time.RFC3339, "2026-10-25T15:59:59Z")
	if got, ok := catalog.Match("model", at); !ok || got.WindowID != "w31" {
		t.Fatal("valid maximum window count rejected")
	}
}
