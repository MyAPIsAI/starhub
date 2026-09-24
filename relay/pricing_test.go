package relay

import (
	"testing"

	"starhub/common"
	"starhub/setting"
)

func resetPricingForTests() {
	_ = setting.UpdatePricing(setting.OptionKeyModelPrice, `{}`)
	_ = setting.UpdatePricing(setting.OptionKeyModelPriceExpr, `{}`)
	_ = setting.UpdatePricing(setting.OptionKeyCacheRatio, `0.5`)
	_ = setting.UpdatePricing(setting.OptionKeyAudioRatio, `40`)
}

func TestComputeUsageQuotaCacheTokenDiscount(t *testing.T) {
	resetPricingForTests()

	base := ComputeQuota("gpt-4o-mini", "default", 100, 50)
	usage := &Usage{
		PromptTokens:        100,
		CompletionTokens:    50,
		PromptTokensDetails: &PromptTokensDetails{CachedTokens: 80},
	}
	got := ComputeUsageQuota(NewPriceData("gpt-4o-mini", "default"), usage)
	// 80 cached tokens at 0.5x instead of 1x saves 80*0.5 input units:
	// (20 + 40 + 200) * 0.15 / 1e6 * 500000 = 19.5 -> 20
	if got != 20 {
		t.Fatalf("cached-token billing = %d, want 20 (base %d)", got, base)
	}
	if got >= base {
		t.Fatalf("cached tokens must discount the bill: %d vs %d", got, base)
	}

	// cache_ratio 1 bills cached tokens like plain input
	setting.UpdatePricing(setting.OptionKeyCacheRatio, `1`)
	usage = &Usage{
		PromptTokens:        100,
		CompletionTokens:    50,
		PromptTokensDetails: &PromptTokensDetails{CachedTokens: 80},
	}
	got = ComputeUsageQuota(NewPriceData("gpt-4o-mini", "default"), usage)
	if got != base {
		t.Fatalf("cache_ratio 1 should bill like plain tokens: %d vs %d", got, base)
	}
	resetPricingForTests()
}

func TestComputeUsageQuotaAudioTokenPremium(t *testing.T) {
	resetPricingForTests()

	usage := &Usage{
		PromptTokens:            100,
		CompletionTokens:        50,
		PromptTokensDetails:     &PromptTokensDetails{AudioTokens: 10},
		CompletionTokensDetails: &CompletionTokensDetails{AudioTokens: 5},
	}
	pd := NewPriceData("gpt-4o-mini", "default")
	got := ComputeUsageQuota(pd, usage)
	// input: 90 text + 10*40 audio = 490; output: 45*4 + 5*40 = 380
	// (490 + 380) * 0.15 / 1e6 * 500000 = 65.25 -> 65
	if got != 65 {
		t.Fatalf("audio billing = %d, want 65", got)
	}
}

func TestUsageDetailClampingAgainstLyingUpstream(t *testing.T) {
	usage := &Usage{
		PromptTokens:            100,
		CompletionTokens:        50,
		PromptTokensDetails:     &PromptTokensDetails{CachedTokens: 500, AudioTokens: 500},
		CompletionTokensDetails: &CompletionTokensDetails{AudioTokens: 999},
	}
	if got := usage.CachedTokens(); got != 100 {
		t.Fatalf("cached clamp = %d, want 100", got)
	}
	if got := usage.InputAudioTokens(); got != 0 {
		t.Fatalf("input audio clamp = %d, want 0 (cached already eats the total)", got)
	}
	if got := usage.OutputAudioTokens(); got != 50 {
		t.Fatalf("output audio clamp = %d, want 50", got)
	}

	negative := &Usage{
		PromptTokens:            -5,
		CompletionTokens:        -5,
		PromptTokensDetails:     &PromptTokensDetails{CachedTokens: -1},
		CompletionTokensDetails: &CompletionTokensDetails{AudioTokens: -1},
	}
	pd := NewPriceData("gpt-4o-mini", "default")
	if got := ComputeUsageQuota(pd, negative); got != 0 {
		t.Fatalf("negative usage must bill nothing, got %d", got)
	}
}

func TestComputeUsageQuotaPerCallPrice(t *testing.T) {
	resetPricingForTests()
	defer resetPricingForTests()

	if err := setting.UpdatePricing(setting.OptionKeyModelPrice, `{"gpt-4o-mini":0.01}`); err != nil {
		t.Fatalf("set model price failed: %v", err)
	}
	pd := NewPriceData("gpt-4o-mini", "default")
	if !pd.IsPerCall() {
		t.Fatal("model should switch to per-call billing")
	}
	// fixed price ignores the token counts
	usage := &Usage{PromptTokens: 100, CompletionTokens: 50}
	if got := ComputeUsageQuota(pd, usage); got != 5000 {
		t.Fatalf("per-call price 0.01 USD = %d quota, want 5000", got)
	}
	if got := ComputeUsageQuota(pd, &Usage{}); got != 5000 {
		t.Fatalf("per-call price applies even without usage, got %d", got)
	}

	// tiny prices never round down to free
	setting.UpdatePricing(setting.OptionKeyModelPrice, `{"gpt-4o-mini":0.0000001}`)
	if got := ComputeUsageQuota(NewPriceData("gpt-4o-mini", "default"), usage); got != 1 {
		t.Fatalf("tiny per-call price must bill at least 1, got %d", got)
	}
}

func TestComputeUsageQuotaExpressionPricing(t *testing.T) {
	resetPricingForTests()
	defer resetPricingForTests()

	expr := "completion_tokens <= 100 ? 0.001 : 0.01"
	if err := setting.UpdatePricing(setting.OptionKeyModelPriceExpr,
		`{"tiered":"`+expr+`"}`); err != nil {
		t.Fatalf("set expr failed: %v", err)
	}
	pd := NewPriceData("tiered", "default")
	if !pd.IsExpression() {
		t.Fatal("model should switch to expression billing")
	}
	if got := ComputeUsageQuota(pd, &Usage{PromptTokens: 10, CompletionTokens: 100}); got != 500 {
		t.Fatalf("expr tier1 = %d, want 500", got)
	}
	if got := ComputeUsageQuota(pd, &Usage{PromptTokens: 10, CompletionTokens: 101}); got != 5000 {
		t.Fatalf("expr tier2 = %d, want 5000", got)
	}

	// the expression wins even when a per-call price is also configured
	setting.UpdatePricing(setting.OptionKeyModelPrice, `{"tiered":0.99}`)
	if got := ComputeUsageQuota(NewPriceData("tiered", "default"),
		&Usage{PromptTokens: 10, CompletionTokens: 100}); got != 500 {
		t.Fatalf("expr must beat the per-call price, got %d", got)
	}
	setting.UpdatePricing(setting.OptionKeyModelPrice, `{}`)

	// group ratio multiplies expression prices too
	setting.UpdateRatio(setting.OptionKeyGroupRatio, `{"default":1,"vip":2}`)
	if got := ComputeUsageQuota(NewPriceData("tiered", "vip"),
		&Usage{PromptTokens: 10, CompletionTokens: 100}); got != 1000 {
		t.Fatalf("expr with vip group = %d, want 1000", got)
	}
	setting.UpdateRatio(setting.OptionKeyGroupRatio, `{"default":1}`)
}

func TestPriceDataAddOtherRatioClamp(t *testing.T) {
	pd := NewPriceData("gpt-4o-mini", "default")
	pd.AddOtherRatio(2)
	pd.AddOtherRatio(1.5)
	if pd.OtherRatio != 3 {
		t.Fatalf("other ratio = %v, want 3", pd.OtherRatio)
	}
	pd.AddOtherRatio(common.MaxRatioProduct) // clamped at the cap
	if pd.OtherRatio != common.MaxRatioProduct {
		t.Fatalf("other ratio must clamp at %v, got %v", common.MaxRatioProduct, pd.OtherRatio)
	}
	// clamping keeps the final quota finite instead of overflowing
	usage := &Usage{PromptTokens: 5_000_000, CompletionTokens: 5_000_000}
	if got := ComputeUsageQuota(pd, usage); got != common.MaxQuotaPerRequest {
		t.Fatalf("saturated bill = %d, want cap %d", got, common.MaxQuotaPerRequest)
	}
}

func TestPriceDataKeepsGroupMatrixAtAcceptance(t *testing.T) {
	if err := setting.UpdateOption(setting.OptionKeyGroupGroupRatio, `{"vip":{"default":0.5}}`); err != nil {
		t.Fatal(err)
	}
	defer setting.UpdateOption(setting.OptionKeyGroupGroupRatio, `{}`)
	accepted := NewPriceDataWithUserGroup("gpt-4o", "default", "vip")
	if err := setting.UpdateOption(setting.OptionKeyGroupGroupRatio, `{"vip":{"default":0.25}}`); err != nil {
		t.Fatal(err)
	}
	if got := accepted.EffectiveRatio(); got != 0.5 {
		t.Fatalf("accepted request ratio = %v, want 0.5", got)
	}
	if got := NewPriceDataWithUserGroup("gpt-4o", "default", "vip").EffectiveRatio(); got != 0.25 {
		t.Fatalf("new request ratio = %v, want 0.25", got)
	}
}

func TestEstimatePreConsumeQuotaUsesMaxTokens(t *testing.T) {
	resetPricingForTests()
	defer resetPricingForTests()

	plain := []byte(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hi"}]}`)
	withMax := []byte(`{"model":"gpt-4o-mini","max_tokens":1000,"messages":[{"role":"user","content":"hi"}]}`)
	withMaxAndN := []byte(`{"model":"gpt-4o-mini","max_tokens":1000,"n":4,"messages":[{"role":"user","content":"hi"}]}`)
	maxCompletion := []byte(`{"model":"gpt-4o-mini","max_completion_tokens":1000,"messages":[{"role":"user","content":"hi"}]}`)

	base := EstimatePreConsumeQuota("gpt-4o-mini", "default", plain)
	// default budget: prompt estimate (1 + 100 reserve) + 1024 completions
	want := ComputeQuota("gpt-4o-mini", "default", 101, 1024)
	if base != want {
		t.Fatalf("default pre-charge = %d, want %d", base, want)
	}

	maxTok := EstimatePreConsumeQuota("gpt-4o-mini", "default", withMax)
	if maxTok != ComputeQuota("gpt-4o-mini", "default", 101, 1000) {
		t.Fatalf("max_tokens pre-charge = %d", maxTok)
	}
	if got := EstimatePreConsumeQuota("gpt-4o-mini", "default", maxCompletion); got != maxTok {
		t.Fatalf("max_completion_tokens should match max_tokens budget, got %d", got)
	}
	scaled := EstimatePreConsumeQuota("gpt-4o-mini", "default", withMaxAndN)
	if scaled != ComputeQuota("gpt-4o-mini", "default", 101, 4000) {
		t.Fatalf("n=4 pre-charge = %d", scaled)
	}
	if scaled <= maxTok {
		t.Fatal("n choices must scale the completion budget up")
	}
	// pinning a budget below the default reserves less — the estimate always
	// follows what the caller actually asked for
	if maxTok >= base {
		t.Fatalf("max_tokens=1000 must reserve less than the 1024 default, got %d vs %d", maxTok, base)
	}
}

func TestEstimatePreConsumeQuotaPerCallAndExpr(t *testing.T) {
	resetPricingForTests()
	defer resetPricingForTests()

	body := []byte(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hi"}]}`)
	setting.UpdatePricing(setting.OptionKeyModelPrice, `{"gpt-4o-mini":0.02}`)
	if got := EstimatePreConsumeQuota("gpt-4o-mini", "default", body); got != 10000 {
		t.Fatalf("per-call pre-charge = %d, want 10000", got)
	}

	// expression pre-charge prices the worst case (prompt estimate over the
	// tier boundary), never less than the settle price of the same tier
	setting.UpdatePricing(setting.OptionKeyModelPriceExpr,
		`{"gpt-4o-mini":"prompt_tokens <= 100 ? 0.001 : 0.01"}`)
	got := EstimatePreConsumeQuota("gpt-4o-mini", "default", body)
	if got != 5000 {
		t.Fatalf("expr pre-charge = %d, want 5000 (est prompt 101 > 100 tier)", got)
	}
}

func TestValidateChatRequest(t *testing.T) {
	valid := []string{
		`{"model":"m","messages":[]}`,
		`{"model":"m","messages":[],"n":1}`,
		`{"model":"m","messages":[],"n":10}`,
		`{"model":"m","messages":[],"max_tokens":0}`,
		`{"model":"m","messages":[],"max_tokens":100000}`,
		`{"model":"m","messages":[],"max_completion_tokens":99999}`,
		`{"model":"m","messages":[],"n":3,"max_tokens":50}`,
		`not json`, // malformed bodies degrade to defaults upstream will reject
	}
	for _, body := range valid {
		if err := ValidateChatRequest([]byte(body)); err != nil {
			t.Errorf("ValidateChatRequest(%s) should pass, got %v", body, err)
		}
	}
	invalid := []string{
		`{"model":"m","n":11}`,
		`{"model":"m","n":-1}`,
		`{"model":"m","max_tokens":100001}`,
		`{"model":"m","max_tokens":-5}`,
		`{"model":"m","max_completion_tokens":100000000}`,
	}
	for _, body := range invalid {
		if err := ValidateChatRequest([]byte(body)); err == nil {
			t.Errorf("ValidateChatRequest(%s) should be rejected", body)
		}
	}
}
