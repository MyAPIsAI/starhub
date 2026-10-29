package controller

import (
	"math"
	"testing"
	"time"

	"starhub/common"
	"starhub/relay"
	"starhub/relay/dto"
	"starhub/setting"
)

func TestIndependentTokenQuoteMatchesAllSixBilledClassesAndExpiresOutsideWindows(t *testing.T) {
	setting.ResetAllToDefaults()
	t.Cleanup(setting.ResetAllToDefaults)
	if err := setting.ApplyOptions(map[string]string{
		setting.OptionKeyModelTimeTokenPrices: `{"m":{"timezone":"UTC","windows":[{"id":"absolute","label":"Absolute","days":[4],"start":"09:00","end":"10:00","prices":{"input_per_m":"1.23","output_per_m":"4.56","cache_read_per_m":"0","cache_write_per_m":"2.5","audio_input_per_m":"8","audio_output_per_m":"16"}}]}}`,
		setting.OptionKeyModelTimePricing:     `{"m":{"timezone":"Asia/Shanghai","windows":[{"id":"boost","label":"Boost","days":[4],"start":"17:00","end":"19:00","multiplier":2}]}}`,
		setting.OptionKeyGroupRatio:           `{"default":1,"vip":0.6}`,
		setting.OptionKeyGroupGroupRatio:      `{"owner":{"vip":0.5}}`,
	}); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 10, 29, 9, 30, 40, 0, time.UTC)
	inputs := setting.ResolvePricingAt("m", "vip", "owner", at)
	quote := plazaPriceFromInputs(inputs, "vip", "owner")
	if quote.Mode != "per_token" || !quote.Explicit || quote.GroupRatio != 0.3 || quote.TimePricing == nil || quote.TimePricing.TokenWindow == nil || quote.TimePricing.TokenWindow.WindowID != "absolute" || quote.TimePricing.TokenWindow.Timezone != "UTC" || quote.TimePricing.Timezone != "Asia/Shanghai" || quote.TimePricing.ValidUntil.Sub(at) != 20*time.Second {
		t.Fatalf("quote metadata: %+v", quote)
	}
	prices := []*float64{quote.InputPerM, quote.OutputPerM, quote.CacheReadPerM, quote.CacheWritePerM, quote.AudioInputPerM, quote.AudioOutputPerM}
	for index, price := range prices {
		u := &relay.Usage{PromptTokens: 1_000_000}
		switch index {
		case 1:
			u.PromptTokens, u.CompletionTokens = 0, 1_000_000
		case 2:
			u.PromptTokensDetails = &dto.PromptTokensDetails{CachedTokens: 1_000_000}
		case 3:
			u.PromptTokensDetails = &dto.PromptTokensDetails{CacheWriteTokens: 1_000_000}
		case 4:
			u.PromptTokensDetails = &dto.PromptTokensDetails{AudioTokens: 1_000_000}
		case 5:
			u.PromptTokens, u.CompletionTokens = 0, 1_000_000
			u.CompletionTokensDetails = &dto.CompletionTokensDetails{AudioTokens: 1_000_000}
		}
		charged := relay.ComputeUsageQuota(relay.NewPriceDataAt("m", "vip", "owner", at), u)
		if price == nil || math.Abs(*price-float64(charged)/float64(common.QuotaPerUSD)) > 1.0/float64(common.QuotaPerUSD) {
			t.Fatalf("class%d quote%v charge%d", index, price, charged)
		}
	}
	outside := plazaPriceFromInputs(setting.ResolvePricingAt("m", "vip", "owner", at.Add(30*time.Minute)), "vip", "owner")
	if outside.TimePricing == nil || outside.TimePricing.TokenWindow == nil || outside.TimePricing.TokenWindow.WindowID != "" || outside.TimePricing.Multiplier != 2 {
		t.Fatal("outside absolute windows must still refresh base-price quotes")
	}
}
