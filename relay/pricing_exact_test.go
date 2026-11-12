package relay

import (
	"testing"

	"starhub/common"
)

func TestQuotaToMicroUSD(t *testing.T) {
	// 1 USD = 500000 quota; 1 USD = 1,000,000 micro-USD.
	// So 500000 quota -> 1,000,000 micro-USD; 1 quota -> 2 micro-USD.
	got := QuotaToMicroUSD(common.QuotaPerUSD)
	if got != 1_000_000 {
		t.Fatalf("QuotaToMicroUSD(%d) = %d, want 1000000", common.QuotaPerUSD, got)
	}
	got = QuotaToMicroUSD(1)
	if got != 1_000_000/common.QuotaPerUSD {
		t.Fatalf("QuotaToMicroUSD(1) = %d, want %d", got, 1_000_000/common.QuotaPerUSD)
	}
}

func TestExactFeeMicroUSDTokenBilling(t *testing.T) {
	// ModelRatio=2 means 2 USD per 1M tokens. GroupRatio=1, CompletionRatio=3.
	pd := &PriceData{ModelRatio: 2, CompletionRatio: 3, CacheRatio: 0, AudioRatio: 0, GroupRatio: 1, OtherRatio: 1}
	usage := &Usage{PromptTokens: 100, CompletionTokens: 50}
	quota := ComputeUsageQuota(pd, usage)
	micro := ExactFeeMicroUSD(pd, usage)
	if micro <= 0 {
		t.Fatalf("ExactFeeMicroUSD should be positive, got %d", micro)
	}
	// Verify consistency: micro-USD converted back to quota should be
	// within 1 of the rounded quota (rounding tolerance).
	recomputed := micro * common.QuotaPerUSD / 1_000_000
	if abs(recomputed-quota) > 1 {
		t.Fatalf("recomputed quota %d differs from original %d by more than 1", recomputed, quota)
	}
}

func TestExactFeeMicroUSDPerCall(t *testing.T) {
	pd := &PriceData{ModelPriceUSD: 0.01, HasModelPrice: true, GroupRatio: 1, OtherRatio: 1}
	usage := &Usage{}
	micro := ExactFeeMicroUSD(pd, usage)
	// 0.01 USD * 1e6 = 10000 micro-USD (GroupRatio=1, EffectiveRatio=1).
	if micro != 10000 {
		t.Fatalf("ExactFeeMicroUSD for $0.01 per-call = %d, want 10000", micro)
	}
}

func TestExactFeeMicroUSDExpression(t *testing.T) {
	pd := &PriceData{PriceExpr: "prompt_tokens * 0.000001", GroupRatio: 1, OtherRatio: 1}
	usage := &Usage{PromptTokens: 1000}
	micro := ExactFeeMicroUSD(pd, usage)
	// 1000 * 0.000001 = 0.001 USD -> 1000 micro-USD.
	if micro != 1000 {
		t.Fatalf("ExactFeeMicroUSD for expr 1000*0.000001 = %d, want 1000", micro)
	}
}

func abs(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}
