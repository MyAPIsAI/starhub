package pricingtime

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTokenRatesSnapshotIsExactDetachedAndStrict(t *testing.T) {
	prices := tokenPriceFixture()["model"].Windows[0].Prices
	rates, err := NewTokenRates(prices)
	if err != nil || !rates.Valid() {
		t.Fatalf("compile: %v", err)
	}
	copy := rates
	prices.InputPerM = "999"
	returned := rates.Prices()
	returned.InputPerM = "999"
	nanos := rates.NanoPrices()
	nanos[0] = 999
	if copy.Prices().InputPerM != "1.23" || rates.NanoPrices()[0] != 1_230_000_000 {
		t.Fatal("snapshot shares mutable prices")
	}
	raw, err := json.Marshal(rates)
	if err != nil || !strings.Contains(string(raw), `"audio_input_per_m":"0.000000001"`) {
		t.Fatalf("exact wire prices: %s %v", raw, err)
	}
	var decoded TokenRates
	if err := json.Unmarshal(raw, &decoded); err != nil || decoded != rates {
		t.Fatalf("roundtrip: %+v %v", decoded, err)
	}
	for _, invalid := range []string{
		`{}`, `[]`, `"1"`, `true`,
		strings.Replace(string(raw), `"input_per_m":"1.23"`, `"input_per_m":1.23`, 1),
		strings.Replace(string(raw), `"input_per_m":"1.23"`, `"input_per_m":null`, 1),
		strings.Replace(string(raw), `"input_per_m":"1.23"`, `"input_per_m":"-1"`, 1),
		strings.Replace(string(raw), `"input_per_m":"1.23"`, `"input_per_m":"1","input_per_m":"2"`, 1),
		strings.Replace(string(raw), `{`, `{"unknown":1,`, 1),
		string(raw) + `{}`,
		`{"input_per_m":"` + strings.Repeat("0", 2100) + `"}`,
	} {
		decoded = rates
		if err := json.Unmarshal([]byte(invalid), &decoded); err == nil || decoded != rates {
			t.Fatalf("invalid snapshot accepted or partially mutated: %s", invalid)
		}
	}
	if err := json.Unmarshal([]byte(`null`), &decoded); err != nil || decoded.Valid() {
		t.Fatal("null must clear the optional override")
	}
	zeroJSON, err := json.Marshal(TokenRates{})
	if err != nil || string(zeroJSON) != "null" {
		t.Fatal("legacy absent table must stay distinct from free pricing")
	}
	free, err := NewTokenRates(TokenPrices{"0", "0", "0", "0", "0", "0"})
	if err != nil || !free.Valid() || free == (TokenRates{}) {
		t.Fatal("explicit free tariff lost presence")
	}
}
