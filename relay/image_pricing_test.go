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
