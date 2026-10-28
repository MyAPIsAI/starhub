package relay

import (
	"testing"

	"starhub/relay/dto"
	"starhub/setting"
)

func TestImagePriceRatio(t *testing.T) {
	cases := []struct {
		model, size, quality string
		want                 float64
	}{
		{"dall-e-3", "1024x1024", "standard", 1},
		{"dall-e-3", "1792x1024", "standard", 2.025},
		{"dall-e-3", "1024x1024", "hd", 2},
		{"dall-e-3", "", "", 1},
		{"gpt-image-1", "1536x1024", "high", 1.5 * 1.5},
		{"gpt-image-1", "unknown-size", "auto", 1},
		{"mystery", "1024x1024", "hd", 2},
	}
	for _, c := range cases {
		if got := ImagePriceRatio(c.model, c.size, c.quality); got != c.want {
			t.Fatalf("ImagePriceRatio(%q,%q,%q) = %v, want %v", c.model, c.size, c.quality, got, c.want)
		}
	}
}

func TestImageCapturedBaselineIsCopiedForEachCountCalculation(t *testing.T) {
	setting.ResetAllToDefaults()
	t.Cleanup(setting.ResetAllToDefaults)
	if err := setting.UpdatePricing("model_price", `{"captured-image":0.01}`); err != nil {
		t.Fatal(err)
	}
	baseline := NewPriceData("captured-image", "default")
	bill := &ImageBill{Model: "captured-image", Group: "default", Size: "1024x1024", Quality: "standard", Count: 2, Pricing: baseline}
	if err := setting.UpdatePricing("model_price", `{"captured-image":0.2}`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if quota := bill.PreQuota(); quota != 10000 {
			t.Fatalf("pre-charge accumulated multipliers or read live price: %d", quota)
		}
		if quota := bill.SettleQuota(1, nil); quota != 5000 {
			t.Fatalf("actual-count price did not use baseline: %d", quota)
		}
	}
	if baseline.OtherRatio != 1 || baseline.ModelPriceUSD != 0.01 {
		t.Fatal("image calculation mutated captured baseline")
	}
}

func TestImageBillPerCallPreAndSettle(t *testing.T) {
	setOptionTest(t, "model_price", `{"gpt-image-1":0.04}`)
	// pre: 0.04 x 1.0 (size) x 1.0 (quality) x n=3 x group 1
	bill := &ImageBill{Model: "gpt-image-1", Group: "default", Size: "1024x1024", Prompt: "cat", Count: 3}
	if got := bill.PreQuota(); got != 0.04*3*500000 {
		t.Fatalf("pre = %d, want %v", got, 0.04*3*500000)
	}
	// settle with the actual count of 2 (upstream under-delivered): bills 2
	if got := bill.SettleQuota(2, nil); got != 0.04*2*500000 {
		t.Fatalf("settle(2) = %d, want %v", got, 0.04*2*500000)
	}
	// settle with MORE completed images than requested: bills the actual
	// (upstream over-delivery must not be free, P3.1 invariant 2)
	if got := bill.SettleQuota(4, nil); got != 0.04*4*500000 {
		t.Fatalf("settle(4) = %d, want %v", got, 0.04*4*500000)
	}
	// a count outside [1, MaxImageN] is ignored: the requested n stays the
	// bill of record (guard against poisoned upstream payloads)
	for _, hostile := range []int{0, -1, 999999} {
		if got := bill.SettleQuota(hostile, nil); got != 0.04*3*500000 {
			t.Fatalf("hostile count %d must fall back to the request: %d", hostile, got)
		}
	}
}

func TestImageBillSizeAndQualityMultipliers(t *testing.T) {
	setOptionTest(t, "model_price", `{"dall-e-3":0.04}`)
	bill := &ImageBill{Model: "dall-e-3", Group: "default", Size: "1024x1792", Quality: "hd", Prompt: "cat", Count: 1}
	// 0.04 x 2.025 (size) x 2 (hd) = 0.162 USD
	if got := bill.PreQuota(); got != 0.04*2.025*2*500000 {
		t.Fatalf("pre = %d, want %v", got, 0.04*2.025*2*500000)
	}
}

func TestImageBillTokenFallback(t *testing.T) {
	// token-billed image model: no per-call price, usage from the response
	// (input_tokens -> prompt, output_tokens -> completion; the overwrite
	// mapping is image-path-only).
	bill := &ImageBill{Model: "gpt-image-1", Group: "default", Prompt: "cat", Count: 2}
	pre := bill.PreQuota()
	if pre <= 0 {
		t.Fatalf("token pre must be positive, got %d", pre)
	}
	actual := bill.SettleQuota(2, &dto.ImageUsage{InputTokens: 500, OutputTokens: 3000})
	want := ComputeUsageQuota(NewPriceData("gpt-image-1", "default"),
		&Usage{PromptTokens: 500, CompletionTokens: 3000})
	if actual != want {
		t.Fatalf("token settle = %d, want %d", actual, want)
	}
	// no usage report: the estimate stays the bill
	if got := bill.SettleQuota(2, nil); got != pre {
		t.Fatalf("usage-less settle = %d, want %d (the estimate)", got, pre)
	}
}

func TestImageBillGroupRatio(t *testing.T) {
	setOptionTest(t, "model_price", `{"gpt-image-1":0.04}`)
	setOptionTest(t, "group_ratio", `{"default":1,"vip":0.5}`)
	bill := &ImageBill{Model: "gpt-image-1", Group: "vip", Size: "1024x1024", Prompt: "cat", Count: 1}
	if got := bill.PreQuota(); got != 0.04*0.5*500000 {
		t.Fatalf("group ratio must apply: %d", got)
	}
}

func setOptionTest(t *testing.T, key, value string) {
	t.Helper()
	if err := setting.UpdateOption(key, value); err != nil {
		t.Fatalf("set option %s: %v", key, err)
	}
	t.Cleanup(func() {
		_ = setting.UpdateOption(key, `{}`)
	})
}

func TestImageSizeValue(t *testing.T) {
	cases := []struct {
		size string
		want float64
	}{
		{"1024x1024", 1024},
		{"1024x1792", 1792},
		{"1792x1024", 1792},
		{"1536x1024", 1536},
		{"1024x1536", 1536},
		{"256x256", 256},
		{"512x512", 512},
		{"auto", 1024},
		{"", 1024},
		{"bogus", 1024},
	}
	for _, c := range cases {
		if got := imageSizeValue(c.size); got != c.want {
			t.Fatalf("imageSizeValue(%q) = %v, want %v", c.size, got, c.want)
		}
	}
}

func TestImageQualityValue(t *testing.T) {
	cases := []struct {
		quality string
		want    float64
	}{
		{"standard", 1},
		{"hd", 2},
		{"low", 0.5},
		{"medium", 1},
		{"high", 1.5},
		{"auto", 1},
		{"", 1},
		{"unknown", 1},
	}
	for _, c := range cases {
		if got := imageQualityValue(c.quality); got != c.want {
			t.Fatalf("imageQualityValue(%q) = %v, want %v", c.quality, got, c.want)
		}
	}
}

func TestImageBillExpressionCount(t *testing.T) {
	resetPricingForTests()
	defer resetPricingForTests()
	setOptionTest(t, "model_price_expr", `{"gpt-image-1":"0.04 * count"}`)
	// Expression with count: pre uses requested n, settle uses actual.
	bill := &ImageBill{Model: "gpt-image-1", Group: "default", Size: "1024x1024", Prompt: "cat", Count: 3}
	// pre: 0.04 * 3 = 0.12 USD = 60000 quota (group 1, no OtherRatio)
	if got := bill.PreQuota(); got != 0.04*3*500000 {
		t.Fatalf("expr pre = %d, want %v", got, 0.04*3*500000)
	}
	// settle with actual count 2: 0.04 * 2 = 0.08 USD = 40000
	if got := bill.SettleQuota(2, nil); got != 0.04*2*500000 {
		t.Fatalf("expr settle(2) = %d, want %v", got, 0.04*2*500000)
	}
	// over-delivery: settle with 4 must bill 4 (no free images)
	if got := bill.SettleQuota(4, nil); got != 0.04*4*500000 {
		t.Fatalf("expr settle(4) = %d, want %v", got, 0.04*4*500000)
	}
	// hostile count falls back to requested n
	for _, hostile := range []int{0, -1, 999999} {
		if got := bill.SettleQuota(hostile, nil); got != 0.04*3*500000 {
			t.Fatalf("hostile count %d must fall back to request: %d", hostile, got)
		}
	}
}

func TestImageBillExpressionSizeAndQuality(t *testing.T) {
	resetPricingForTests()
	defer resetPricingForTests()
	// Expression that uses size and quality explicitly: the expression
	// result is the complete USD price, NOT multiplied by OtherRatio.
	expr := "0.04 * count * (size == 1792 ? 2.025 : 1) * quality"
	setOptionTest(t, "model_price_expr", `{"dall-e-3":"`+expr+`"}`)

	// 1024x1792, hd (quality=2): 0.04 * 1 * 2.025 * 2 = 0.162
	bill := &ImageBill{Model: "dall-e-3", Group: "default", Size: "1024x1792", Quality: "hd", Prompt: "cat", Count: 1}
	if got := bill.PreQuota(); got != int64(0.04*2.025*2*500000) {
		t.Fatalf("expr size+quality pre = %d, want %v", got, 0.04*2.025*2*500000)
	}

	// 1024x1024, standard (quality=1): 0.04 * 1 * 1 * 1 = 0.04
	bill2 := &ImageBill{Model: "dall-e-3", Group: "default", Size: "1024x1024", Quality: "standard", Prompt: "cat", Count: 1}
	if got := bill2.PreQuota(); got != 0.04*500000 {
		t.Fatalf("expr base pre = %d, want %v", got, 0.04*500000)
	}

	// 1536x1024, high (quality=1.5): 0.04 * 1 * 1 * 1.5 = 0.06
	bill3 := &ImageBill{Model: "dall-e-3", Group: "default", Size: "1536x1024", Quality: "high", Prompt: "cat", Count: 1}
	if got := bill3.PreQuota(); got != int64(0.04*1.5*500000) {
		t.Fatalf("expr 1536+high pre = %d, want %v", got, 0.04*1.5*500000)
	}
}

func TestImageBillExpressionNoOtherRatio(t *testing.T) {
	resetPricingForTests()
	defer resetPricingForTests()
	// A constant expression: the result must NOT be multiplied by
	// ImagePriceRatio or count (the key invariant of the expression path).
	setOptionTest(t, "model_price_expr", `{"gpt-image-1":"0.10"}`)
	bill := &ImageBill{Model: "gpt-image-1", Group: "default", Size: "1024x1792", Quality: "hd", Prompt: "cat", Count: 5}
	// 0.10 USD flat, regardless of size/quality/count
	if got := bill.PreQuota(); got != 0.10*500000 {
		t.Fatalf("constant expr must not multiply OtherRatio: %d, want %v", got, 0.10*500000)
	}
}

func TestImageBillExpressionWithTokens(t *testing.T) {
	resetPricingForTests()
	defer resetPricingForTests()
	// Expression that uses prompt_tokens: settle uses real usage.
	setOptionTest(t, "model_price_expr", `{"gpt-image-1":"0.001 * prompt_tokens"}`)
	bill := &ImageBill{Model: "gpt-image-1", Group: "default", Size: "1024x1024", Prompt: "cat", Count: 2}
	pre := bill.PreQuota()
	if pre <= 0 {
		t.Fatalf("expr with tokens pre must be positive, got %d", pre)
	}
	// settle with real usage: 0.001 * 500 = 0.5 USD = 250000
	actual := bill.SettleQuota(2, &dto.ImageUsage{InputTokens: 500, OutputTokens: 0})
	if actual != 0.001*500*500000 {
		t.Fatalf("expr token settle = %d, want %v", actual, 0.001*500*500000)
	}
}

func TestImageBillExpressionGroupRatio(t *testing.T) {
	resetPricingForTests()
	defer resetPricingForTests()
	setOptionTest(t, "model_price_expr", `{"gpt-image-1":"0.04 * count"}`)
	setOptionTest(t, "group_ratio", `{"default":1,"vip":0.5}`)
	bill := &ImageBill{Model: "gpt-image-1", Group: "vip", Size: "1024x1024", Prompt: "cat", Count: 1}
	// 0.04 * 0.5 (group) = 0.02 USD = 10000
	if got := bill.PreQuota(); got != 0.04*0.5*500000 {
		t.Fatalf("expr group ratio = %d, want %v", got, 0.04*0.5*500000)
	}
}

func TestImageBillExpressionFailureFallback(t *testing.T) {
	resetPricingForTests()
	defer resetPricingForTests()
	// A syntactically valid but eval-time-failing expression (division by
	// zero at runtime when count is a variable... actually the engine
	// rejects div by zero). Use a valid expression that references an
	// unknown variable to trigger eval failure.
	setOptionTest(t, "model_price_expr", `{"gpt-image-1":"0.04 * nonexistent_var"}`)
	setOptionTest(t, "model_price", `{"gpt-image-1":0.04}`)
	bill := &ImageBill{Model: "gpt-image-1", Group: "default", Size: "1024x1024", Prompt: "cat", Count: 2}
	// Expression fails (unknown var) → falls back to per-call path:
	// 0.04 * 1 (size) * 1 (quality) * 2 (count) = 0.08
	if got := bill.PreQuota(); got != 0.04*2*500000 {
		t.Fatalf("expr failure fallback to per-call = %d, want %v", got, 0.04*2*500000)
	}
}
