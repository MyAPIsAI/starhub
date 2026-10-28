package relay

import (
	"testing"

	"starhub/common"
	"starhub/relay/channel"
	"starhub/relay/channel/openai"
	"starhub/relay/dto"
)

func TestTaskPricingQuotaExpression(t *testing.T) {
	resetPricingForTests()
	defer resetPricingForTests()
	setOptionTest(t, "model_price_expr", `{"sora-2":"(min(seconds, 10) * 0.1 + max(0, seconds - 10) * 0.08) * n"}`)

	pd := NewPriceData("sora-2", "default")
	// 8 seconds, 1 video: min(8,10)*0.1 + max(0,-2)*0.08 = 0.8; *1 = 0.8 USD
	pricing := channel.TaskPricing{
		ExprVars: map[string]float64{
			"seconds":     8,
			"n":           1,
			"task_status": 1,
		},
	}
	want := common.QuotaFromFloat64(0.8 * float64(common.QuotaPerUSD) * pd.EffectiveRatio())
	if got := TaskPricingQuotaWithPriceData(pd, pricing); got != want {
		t.Fatalf("expr quota = %d, want %d", got, want)
	}
}

func TestTaskPricingQuotaExpressionTaskStatus(t *testing.T) {
	resetPricingForTests()
	defer resetPricingForTests()
	// Expression that discounts by terminal status.
	expr := "task_status == 1 ? seconds * 0.1 * n : task_status == 0.5 ? 0.5 * seconds * 0.1 * n : 0"
	setOptionTest(t, "model_price_expr", `{"sora-2":"`+expr+`"}`)

	pd := NewPriceData("sora-2", "default")
	base := channel.TaskPricing{
		ExprVars: map[string]float64{"seconds": 10, "n": 2},
	}

	// success (task_status=1): 10 * 0.1 * 2 = 2.0 USD
	success := base
	success.ExprVars = map[string]float64{"seconds": 10, "n": 2, "task_status": 1}
	wantSuccess := common.QuotaFromFloat64(2.0 * float64(common.QuotaPerUSD) * pd.EffectiveRatio())
	if got := TaskPricingQuotaWithPriceData(pd, success); got != wantSuccess {
		t.Fatalf("expr success = %d, want %d", got, wantSuccess)
	}

	// cancelled (task_status=0.5): 0.5 * 10 * 0.1 * 2 = 1.0 USD
	cancelled := base
	cancelled.ExprVars = map[string]float64{"seconds": 10, "n": 2, "task_status": 0.5}
	wantCancelled := common.QuotaFromFloat64(1.0 * float64(common.QuotaPerUSD) * pd.EffectiveRatio())
	if got := TaskPricingQuotaWithPriceData(pd, cancelled); got != wantCancelled {
		t.Fatalf("expr cancelled = %d, want %d", got, wantCancelled)
	}

	// failed (task_status=0): 0 USD → 0 quota
	failed := base
	failed.ExprVars = map[string]float64{"seconds": 10, "n": 2, "task_status": 0}
	if got := TaskPricingQuotaWithPriceData(pd, failed); got != 0 {
		t.Fatalf("expr failed = %d, want 0", got)
	}
}

func TestTaskPricingQuotaExpressionEmptyVarsFallsThrough(t *testing.T) {
	resetPricingForTests()
	defer resetPricingForTests()
	setOptionTest(t, "model_price_expr", `{"sora-2":"seconds * 0.1 * n"}`)
	setOptionTest(t, "model_price", `{"sora-2":0.1}`)

	pd := NewPriceData("sora-2", "default")
	// Expression configured but no ExprVars → falls to per-call path.
	// PerCallUSD=0.1, Units=10 (seconds*n=10*1): 0.1 * 10 = 1.0 USD
	pricing := channel.TaskPricing{
		PerCallUSD: 0.1,
		Units:      10,
	}
	want := common.QuotaFromFloat64(0.1 * 10 * float64(common.QuotaPerUSD) * pd.EffectiveRatio())
	if got := TaskPricingQuotaWithPriceData(pd, pricing); got != want {
		t.Fatalf("empty ExprVars fallback = %d, want %d", got, want)
	}
}

func TestTaskPricingQuotaExpressionGroupRatio(t *testing.T) {
	resetPricingForTests()
	defer resetPricingForTests()
	setOptionTest(t, "model_price_expr", `{"sora-2":"seconds * 0.1 * n"}`)
	setOptionTest(t, "group_ratio", `{"default":1,"vip":2}`)

	pd := NewPriceData("sora-2", "vip")
	pricing := channel.TaskPricing{
		ExprVars: map[string]float64{"seconds": 5, "n": 1, "task_status": 1},
	}
	// 5 * 0.1 * 1 = 0.5 USD, group ratio 2 → effective 1.0 USD
	want := common.QuotaFromFloat64(0.5 * float64(common.QuotaPerUSD) * pd.EffectiveRatio())
	if got := TaskPricingQuotaWithPriceData(pd, pricing); got != want {
		t.Fatalf("expr group ratio = %d, want %d", got, want)
	}
}

func TestTaskPricingQuotaPerCallUnchanged(t *testing.T) {
	resetPricingForTests()
	defer resetPricingForTests()
	setOptionTest(t, "model_price", `{"sora-2":0.1}`)

	pd := NewPriceData("sora-2", "default")
	// No expression → per-call path unchanged.
	pricing := channel.TaskPricing{
		PerCallUSD: 0.1,
		Units:      10, // 10 seconds * 1 n
	}
	want := common.QuotaFromFloat64(0.1 * 10 * float64(common.QuotaPerUSD) * pd.EffectiveRatio())
	if got := TaskPricingQuotaWithPriceData(pd, pricing); got != want {
		t.Fatalf("per-call unchanged = %d, want %d", got, want)
	}
}

// --- End-to-end: VideoTaskAdaptor → TaskPricingQuotaWithPriceData ---

func TestVideoTaskExpressionEndToEnd(t *testing.T) {
	resetPricingForTests()
	defer resetPricingForTests()
	expr := "task_status == 1 ? seconds * 0.1 * n : task_status == 0.5 ? 0.5 * seconds * 0.1 * n : 0"
	setOptionTest(t, "model_price_expr", `{"sora-2":"`+expr+`"}`)

	adaptor := openai.VideoTaskAdaptor{}
	req := &dto.TaskRequest{Action: "video", Model: "sora-2", Prompt: "cat", Seconds: 10, N: 2}
	priceData := NewPriceData("sora-2", "default")

	// Pre-charge: EstimateBilling fills ExprVars with task_status=1.
	pricing := adaptor.EstimateBilling("sora-2", "default", req)
	if len(pricing.ExprVars) == 0 {
		t.Fatalf("EstimateBilling must fill ExprVars when expression is configured")
	}
	if pricing.ExprVars["task_status"] != 1 {
		t.Fatalf("pre-charge task_status must be 1, got %v", pricing.ExprVars["task_status"])
	}
	if pricing.ExprVars["seconds"] != 10 {
		t.Fatalf("pre-charge seconds must be 10, got %v", pricing.ExprVars["seconds"])
	}
	if pricing.ExprVars["n"] != 2 {
		t.Fatalf("pre-charge n must be 2, got %v", pricing.ExprVars["n"])
	}
	pre := TaskPricingQuotaWithPriceData(priceData, pricing)
	// success: 10 * 0.1 * 2 = 2.0 USD
	wantPre := common.QuotaFromFloat64(2.0 * float64(common.QuotaPerUSD) * priceData.EffectiveRatio())
	if pre != wantPre {
		t.Fatalf("pre-charge = %d, want %d", pre, wantPre)
	}

	// Settle: success → same price.
	result := &dto.TaskResult{Status: dto.TaskStatusSucceeded}
	terminalPricing := adaptor.AdjustBillingOnComplete("sora-2", "default", req, result, pricing)
	if terminalPricing.ExprVars["task_status"] != 1 {
		t.Fatalf("success task_status must be 1, got %v", terminalPricing.ExprVars["task_status"])
	}
	actual := TaskPricingQuotaWithPriceData(priceData, terminalPricing)
	if actual != pre {
		t.Fatalf("success settle = %d, want %d (same as pre)", actual, pre)
	}

	// Settle: cancelled → half price.
	cancelResult := &dto.TaskResult{Status: dto.TaskStatusCancelled}
	cancelPricing := adaptor.AdjustBillingOnComplete("sora-2", "default", req, cancelResult, pricing)
	if cancelPricing.ExprVars["task_status"] != 0.5 {
		t.Fatalf("cancelled task_status must be 0.5, got %v", cancelPricing.ExprVars["task_status"])
	}
	cancelActual := TaskPricingQuotaWithPriceData(priceData, cancelPricing)
	wantCancel := common.QuotaFromFloat64(1.0 * float64(common.QuotaPerUSD) * priceData.EffectiveRatio())
	if cancelActual != wantCancel {
		t.Fatalf("cancelled settle = %d, want %d", cancelActual, wantCancel)
	}

	// Settle: failed → free.
	failResult := &dto.TaskResult{Status: dto.TaskStatusFailed}
	failPricing := adaptor.AdjustBillingOnComplete("sora-2", "default", req, failResult, pricing)
	if failPricing.ExprVars["task_status"] != 0 {
		t.Fatalf("failed task_status must be 0, got %v", failPricing.ExprVars["task_status"])
	}
	failActual := TaskPricingQuotaWithPriceData(priceData, failPricing)
	if failActual != 0 {
		t.Fatalf("failed settle = %d, want 0", failActual)
	}
}

func TestVideoTaskExpressionPreChargeUpperBound(t *testing.T) {
	resetPricingForTests()
	defer resetPricingForTests()
	// Pre-charge must use the request seconds (upper bound), not 0.
	setOptionTest(t, "model_price_expr", `{"sora-2":"seconds * 0.1 * n"}`)

	adaptor := openai.VideoTaskAdaptor{}
	req := &dto.TaskRequest{Action: "video", Model: "sora-2", Prompt: "cat", Seconds: 20, N: 3}
	priceData := NewPriceData("sora-2", "default")

	pricing := adaptor.EstimateBilling("sora-2", "default", req)
	pre := TaskPricingQuotaWithPriceData(priceData, pricing)
	// 20 * 0.1 * 3 = 6.0 USD
	want := common.QuotaFromFloat64(6.0 * float64(common.QuotaPerUSD) * priceData.EffectiveRatio())
	if pre != want {
		t.Fatalf("pre-charge upper bound = %d, want %d", pre, want)
	}
}

func TestVideoTaskPerCallUnchanged(t *testing.T) {
	resetPricingForTests()
	defer resetPricingForTests()
	setOptionTest(t, "model_price", `{"sora-2":0.1}`)

	adaptor := openai.VideoTaskAdaptor{}
	req := &dto.TaskRequest{Action: "video", Model: "sora-2", Prompt: "cat", Seconds: 10, N: 1}

	// No expression → per-call path, no ExprVars.
	pricing := adaptor.EstimateBilling("sora-2", "default", req)
	if len(pricing.ExprVars) != 0 {
		t.Fatalf("per-call path must not fill ExprVars, got %v", pricing.ExprVars)
	}
	if pricing.PerCallUSD != 0.1 {
		t.Fatalf("PerCallUSD = %v, want 0.1", pricing.PerCallUSD)
	}
	if pricing.Units != 10 {
		t.Fatalf("Units = %v, want 10", pricing.Units)
	}

	// Cancel still uses the hardcoded half-rate fallback.
	cancelResult := &dto.TaskResult{Status: dto.TaskStatusCancelled}
	cancelPricing := adaptor.AdjustBillingOnComplete("sora-2", "default", req, cancelResult, pricing)
	if cancelPricing.Units != 5 {
		t.Fatalf("per-call cancel Units = %v, want 5 (half of 10)", cancelPricing.Units)
	}

	// Failed → full refund.
	failResult := &dto.TaskResult{Status: dto.TaskStatusFailed}
	failPricing := adaptor.AdjustBillingOnComplete("sora-2", "default", req, failResult, pricing)
	if failPricing.PerCallUSD != 0 || failPricing.Units != 0 {
		t.Fatalf("per-call failed must zero out, got PerCallUSD=%v Units=%v",
			failPricing.PerCallUSD, failPricing.Units)
	}
}

func TestVideoTaskTokenFallback(t *testing.T) {
	resetPricingForTests()
	defer resetPricingForTests()
	// No expression, no model_price → token fallback (sora default price
	// still applies in the adaptor, so use a non-sora model name).
	adaptor := openai.VideoTaskAdaptor{}
	req := &dto.TaskRequest{Action: "video", Model: "unpriced-video", Prompt: "cat", Seconds: 5, N: 1}

	pricing := adaptor.EstimateBilling("unpriced-video", "default", req)
	if len(pricing.ExprVars) != 0 {
		t.Fatalf("token fallback must not fill ExprVars, got %v", pricing.ExprVars)
	}
	if pricing.PerCallUSD != 0 {
		t.Fatalf("token fallback PerCallUSD must be 0, got %v", pricing.PerCallUSD)
	}
	if pricing.PromptTokens <= 0 {
		t.Fatalf("token fallback must set PromptTokens, got %d", pricing.PromptTokens)
	}
}
