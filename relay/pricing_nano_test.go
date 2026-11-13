package relay

import (
	"encoding/json"
	"math"
	"testing"

	"starhub/common"
)

func TestNanoTariffsPriceSixClassesAndRoundOnlyAtCurrencyBoundary(t *testing.T) {
	p := independentPriceFixture(t, [6]string{"1", "2", "3", "4", "5", "6"})
	u := &Usage{PromptTokens: 4000000, CompletionTokens: 2000000, PromptTokensDetails: &PromptTokensDetails{CachedTokens: 1000000, CacheWriteTokens: 1000000, AudioTokens: 1000000}, CompletionTokensDetails: &CompletionTokensDetails{AudioTokens: 1000000}}
	if got := ComputeUsageNano(p, u); got != 21000000000 {
		t.Fatalf("six-class fee=%d", got)
	}
	p = independentPriceFixture(t, [6]string{"0.000000001", "0", "0", "0", "0", "0"})
	for _, sample := range []struct{ tokens, want int64 }{{0, 0}, {1, 1}, {1000000, 1}, {1500000, 2}, {1000000000, 1000}} {
		if got := ComputeUsageNano(p, &Usage{PromptTokens: sample.tokens}); got != sample.want {
			t.Fatalf("tokens=%d fee=%d want=%d", sample.tokens, got, sample.want)
		}
	}
	p = independentPriceFixture(t, [6]string{"1000000", "1000000", "0", "0", "0", "0"})
	if got := ComputeUsageNano(p, &Usage{PromptTokens: math.MaxInt64, CompletionTokens: math.MaxInt64}); got != common.MaxNanoPerRequest {
		t.Fatal("nano price overflow")
	}
}

func TestNanoTariffMultipliersSurviveValueSnapshotWithoutBinaryProductDrift(t *testing.T) {
	p := &PriceData{ModelPriceUSD: .1, GroupRatio: .1, OtherRatio: 1}
	p.AddOtherRatio(.1)
	p.AddOtherRatio(.2)
	if got := ComputeUsageNano(p, nil); got != 200000 {
		t.Fatalf("decimal multiplier drift: %d", got)
	}
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var restored PriceData
	if json.Unmarshal(raw, &restored) != nil || ComputeUsageNano(&restored, nil) != 200000 {
		t.Fatal("snapshot changed exact multipliers")
	}
	copy := *p
	copy.AddOtherRatio(.1)
	if ComputeUsageNano(p, nil) != 200000 || ComputeUsageNano(&copy, nil) != 20000 {
		t.Fatal("price snapshot mutation leaked")
	}
	p = &PriceData{ModelRatio: 1, CompletionRatio: 1, CacheRatio: 1, AudioRatio: 1, GroupRatio: 1, OtherRatio: 1}
	if ComputeUsageNano(p, &Usage{PromptTokens: 1}) != 1000 {
		t.Fatal("legacy ratio formula lost sub-quota precision")
	}
	p.PriceExpr = "prompt_tokens * 0.000000001"
	if ComputeUsageNano(p, &Usage{PromptTokens: 1}) != 1 {
		t.Fatal("nano expression fee lost")
	}
}

func TestNanoMicroProjectionDoesNotRecomputeWithLegacyRates(t *testing.T) {
	p := NewPriceData("projection", "default")
	p.AmountUnit = common.MoneyUnitNano
	p.HasModelPrice = true
	p.ModelPriceUSD = 0.001
	if ExactFeeMicroUSD(p, &Usage{}) != 1000 {
		t.Fatal("micro projection changed the canonical price")
	}
	if QuotaToMicroUSD(9_000_000_000_000) != 18_000_000_000_000 {
		t.Fatal("intermediate conversion overflowed")
	}
}
