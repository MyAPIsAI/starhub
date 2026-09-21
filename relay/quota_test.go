package relay

import (
	"testing"

	"starhub/setting"
)

func TestEstimateTokens(t *testing.T) {
	cases := []struct {
		text string
		want int64
	}{
		{"", 0},
		{"a", 1},
		{"abcd", 1},
		{"abcde", 2},
		{"你好世界测试", 2},
	}
	for _, c := range cases {
		if got := EstimateTokens(c.text); got != c.want {
			t.Errorf("EstimateTokens(%q) = %d, want %d", c.text, got, c.want)
		}
	}
}

func TestExtractPromptText(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4o-mini",
		"messages": [
			{"role": "system", "content": "be brief"},
			{"role": "user", "content": "hello there"},
			{"role": "user", "content": [
				{"type": "text", "text": "multi part"},
				{"type": "image_url", "text": ""}
			]}
		]
	}`)
	got := ExtractPromptText(body)
	if got != "be brief hello there multi part" {
		t.Fatalf("ExtractPromptText = %q", got)
	}
	if ExtractPromptText([]byte("not json")) != "" {
		t.Fatal("non-JSON body should yield empty text")
	}
}

func TestComputeQuotaKnownModel(t *testing.T) {
	setting.UpdateRatio(setting.OptionKeyGroupRatio, `{"default":1,"vip":2}`)

	quota := ComputeQuota("gpt-4o-mini", "default", 100, 50)
	if quota < 22 || quota > 24 {
		t.Fatalf("gpt-4o-mini 100/50 quota = %d, want 23 (±1)", quota)
	}

	if ComputeQuota("gpt-4o-mini", "default", 0, 0) != 0 {
		t.Fatal("zero usage should cost nothing")
	}

	if got := ComputeQuota("gpt-4o-mini", "default", 1, 0); got != 1 {
		t.Fatalf("tiny usage should be clamped to 1, got %d", got)
	}

	unknown := ComputeQuota("mystery-model", "default", 1000, 1000)
	if unknown != 2500 {
		t.Fatalf("unknown model 1000/1000 quota = %d, want 2500", unknown)
	}

	vip := ComputeQuota("mystery-model", "vip", 1000, 1000)
	if vip != 5000 {
		t.Fatalf("vip group should double the quota: got %d, want 5000", vip)
	}
}
