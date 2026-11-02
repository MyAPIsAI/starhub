package relay

import (
	"testing"

	"starhub/setting"
)

func TestFreeGroupSurvivesPricingSnapshotResolution(t *testing.T) {
	before, err := setting.SnapshotRatio(setting.OptionKeyGroupRatio)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = setting.UpdateRatio(setting.OptionKeyGroupRatio, before) })
	if err := setting.UpdateRatio(setting.OptionKeyGroupRatio, `{"default":1,"free":0}`); err != nil {
		t.Fatal(err)
	}
	if got := NewPriceData("gpt-4o", "free").GroupRatio; got != 0 {
		t.Fatalf("free group resolved as ratio %v", got)
	}
	if got := NewPriceData("gpt-4o", "missing").GroupRatio; got != 1 {
		t.Fatalf("missing ratio lost its compatibility fallback: %v", got)
	}
}

func TestZeroGroupMultiplierStaysFreeAcrossPricingModes(t *testing.T) {
	for _, price := range []PriceData{
		{ModelRatio: 1, CompletionRatio: 1, GroupRatio: 0, OtherRatio: 1},
		{ModelPriceUSD: 0.05, GroupRatio: 0, OtherRatio: 1},
		{PriceExpr: "total_tokens * 0.01", GroupRatio: 0, OtherRatio: 1},
	} {
		if got := ComputeUsageQuota(&price, &Usage{PromptTokens: 100, CompletionTokens: 50}); got != 0 {
			t.Errorf("free group settled %d quota: %+v", got, price)
		}
		if got := EstimatePreConsumeQuotaWithPriceData(&price, []byte(`{"max_tokens":64}`)); got != 0 {
			t.Errorf("free group estimated %d quota: %+v", got, price)
		}
	}
	paid := &PriceData{ModelPriceUSD: 0.00000001, GroupRatio: 1, OtherRatio: 1}
	if got := ComputeUsageQuota(paid, &Usage{}); got != 1 {
		t.Fatalf("positive tiny prices lost their minimum charge: %d", got)
	}
}
