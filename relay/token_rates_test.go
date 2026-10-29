package relay

import (
	"encoding/json"
	"math"
	"testing"

	"starhub/common"
	"starhub/pkg/pricingtime"
)

func independentPriceFixture(t *testing.T, values [6]string) *PriceData {
	t.Helper()
	rates, err := pricingtime.NewTokenRates(pricingtime.TokenPrices{InputPerM: values[0], OutputPerM: values[1], CacheReadPerM: values[2], CacheWritePerM: values[3], AudioInputPerM: values[4], AudioOutputPerM: values[5]})
	if err != nil {
		t.Fatal(err)
	}
	return &PriceData{ModelName: "independent", ModelRatio: 99, CompletionRatio: 99, CacheRatio: 99, AudioRatio: 99, GroupRatio: 1, GroupGroupRatio: 1, OtherRatio: 1, TokenRates: rates}
}

func TestIndependentTokenRatesPriceAllSixDisjointClasses(t *testing.T) {
	p := independentPriceFixture(t, [6]string{"1", "2", "3", "4", "5", "6"})
	u := &Usage{PromptTokens: 4_000_000, CompletionTokens: 2_000_000,
		PromptTokensDetails:     &PromptTokensDetails{CachedTokens: 1_000_000, CacheWriteTokens: 1_000_000, AudioTokens: 1_000_000},
		CompletionTokensDetails: &CompletionTokensDetails{AudioTokens: 1_000_000}}
	if got := ComputeUsageQuota(p, u); got != 10_500_000 {
		t.Fatalf("six-class price = %d, want 10500000", got)
	}
	p.GroupRatio, p.OtherRatio = 0.8, 2
	p.TimeAdjustment.Multiplier = 0.5
	p.UserGroup, p.UsingGroup, p.GroupGroupRatio = "owner", "serving", 0.25
	if got := ComputeUsageQuota(p, u); got != 2_100_000 {
		t.Fatalf("frozen group/time/other multipliers = %d", got)
	}
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var restored PriceData
	if err := json.Unmarshal(raw, &restored); err != nil || ComputeUsageQuota(&restored, u) != 2_100_000 {
		t.Fatalf("exact value snapshot roundtrip: %v", err)
	}
	copy := *p
	copy.TokenRates = independentPriceFixture(t, [6]string{"0", "0", "0", "0", "0", "0"}).TokenRates
	if ComputeUsageQuota(&copy, u) != 0 || ComputeUsageQuota(p, u) != 2_100_000 {
		t.Fatal("copied tariff changed another request")
	}
}

func TestIndependentTokenRatesClampOverlappingAndNegativeDetails(t *testing.T) {
	p := independentPriceFixture(t, [6]string{"1", "2", "3", "4", "5", "6"})
	u := &Usage{PromptTokens: 100, CompletionTokens: 50,
		PromptTokensDetails:     &PromptTokensDetails{CachedTokens: 30, CacheWriteTokens: 100, AudioTokens: 100},
		CompletionTokensDetails: &CompletionTokensDetails{AudioTokens: 100}}
	if got := ComputeUsageQuota(p, u); got != 335 {
		t.Fatalf("read30/write70/audioOut50 charged once: %d", got)
	}
	u.PromptTokensDetails.CachedTokens = 999
	if got := ComputeUsageQuota(p, u); got != 300 {
		t.Fatalf("cached subset must exhaust prompt: %d", got)
	}
	u.PromptTokens, u.CompletionTokens = -5, -9
	if got := ComputeUsageQuota(p, u); got != 0 {
		t.Fatalf("negative totals: %d", got)
	}
	if got := ComputeUsageQuota(p, nil); got != 0 {
		t.Fatalf("absent usage: %d", got)
	}
}

func TestIndependentTokenRatesRoundExactlyAndSaturateSafely(t *testing.T) {
	p := independentPriceFixture(t, [6]string{"0.000000001", "0", "0", "0", "0", "0"})
	for _, tc := range []struct{ count, want int64 }{{0, 0}, {1, 1}, {2_999_999_999, 1}, {3_000_000_000, 2}, {3_000_000_001, 2}} {
		if got := ComputeUsageQuota(p, &Usage{PromptTokens: tc.count}); got != tc.want {
			t.Fatalf("nano price count%d = %d, want%d", tc.count, got, tc.want)
		}
	}
	p = independentPriceFixture(t, [6]string{"1000000", "1000000", "0", "0", "0", "0"})
	if got := ComputeUsageQuota(p, &Usage{PromptTokens: math.MaxInt64, CompletionTokens: math.MaxInt64}); got != common.MaxQuotaPerRequest {
		t.Fatalf("large token products must saturate without overflowing: %d", got)
	}
	p = independentPriceFixture(t, [6]string{"0", "0", "3", "7", "0", "0"})
	if ComputeUsageQuota(p, &Usage{PromptTokens: 100}) != 0 {
		t.Fatal("zero input price must not discard independent cache prices")
	}
	if got := ComputeUsageQuota(p, &Usage{PromptTokens: 100, PromptTokensDetails: &PromptTokensDetails{CacheWriteTokens: 100}}); got != 350 {
		t.Fatalf("independently priced cache writes: %d", got)
	}
	p.GroupRatio = 0
	if got := ComputeUsageQuota(p, &Usage{PromptTokens: 100, PromptTokensDetails: &PromptTokensDetails{CachedTokens: 100}}); got != 0 {
		t.Fatalf("zero effective tariff must remain free: %d", got)
	}
}

func TestIndependentTokenRatesPreserveModePrecedenceAndLegacyFallback(t *testing.T) {
	p := independentPriceFixture(t, [6]string{"1", "2", "3", "4", "5", "6"})
	u := &Usage{PromptTokens: 100, CompletionTokens: 50}
	p.ModelPriceUSD = 0.01
	if got := ComputeUsageQuota(p, u); got != 5000 {
		t.Fatalf("flat pricing precedence: %d", got)
	}
	p.PriceExpr = "0.02"
	if got := ComputeUsageQuota(p, u); got != 10000 {
		t.Fatalf("expression pricing precedence: %d", got)
	}
	p.PriceExpr, p.ModelPriceUSD = "invalid(", 0
	if got := ComputeUsageQuota(p, u); got != 100 {
		t.Fatalf("expression failure must use captured token tariff: %d", got)
	}
	var legacy PriceData
	if err := json.Unmarshal([]byte(`{"ModelRatio":2,"CompletionRatio":3,"GroupRatio":1,"OtherRatio":1,"CacheRatio":0.5,"AudioRatio":40}`), &legacy); err != nil {
		t.Fatal(err)
	}
	if legacy.TokenRates.Valid() || ComputeUsageQuota(&legacy, u) != 250 {
		t.Fatal("old snapshots must retain their legacy token formula")
	}
}

func TestIndependentTokenReserveCoversEveryUnknownUsageClass(t *testing.T) {
	p := independentPriceFixture(t, [6]string{"1", "2", "9", "12", "3", "8"})
	reserve := estimatePreConsume(p, 100, 50)
	if reserve != 800 {
		t.Fatalf("reserve must cover input12/output8, got %d", reserve)
	}
	for input := 0; input < 4; input++ {
		for output := 0; output < 2; output++ {
			u := &Usage{PromptTokens: 100, CompletionTokens: 50, PromptTokensDetails: &PromptTokensDetails{}, CompletionTokensDetails: &CompletionTokensDetails{}}
			switch input {
			case 1:
				u.PromptTokensDetails.CachedTokens = 100
			case 2:
				u.PromptTokensDetails.CacheWriteTokens = 100
			case 3:
				u.PromptTokensDetails.AudioTokens = 100
			}
			if output == 1 {
				u.CompletionTokensDetails.AudioTokens = 50
			}
			if got := ComputeUsageQuota(p, u); got > reserve {
				t.Fatalf("class%d/%d exceeds reserve: %d > %d", input, output, got, reserve)
			}
		}
	}
	bill := ImageBill{Prompt: "", Pricing: p, Count: 1}
	if got := bill.PreQuota(); got != 600 {
		t.Fatalf("image prompt reservation must use max input class: %d", got)
	}
	if got := bill.SettleQuota(1, nil); got != 50 {
		t.Fatalf("image fallback settles estimated text at its actual class: %d", got)
	}
}

func TestIndependentTokenTaskSnapshotRequiresVersionTwo(t *testing.T) {
	p := independentPriceFixture(t, [6]string{"1", "2", "3", "4", "5", "6"})
	raw, err := json.Marshal(TaskBillingSnapshot{PriceData: *p})
	if err != nil {
		t.Fatal(err)
	}
	var saved TaskBillingSnapshot
	if err := json.Unmarshal(raw, &saved); err != nil || saved.Version != 2 || !saved.PriceData.TokenRates.Valid() {
		t.Fatal("independent task snapshot lost its version or prices")
	}
	var invalid map[string]json.RawMessage
	_ = json.Unmarshal(raw, &invalid)
	delete(invalid, "version")
	raw, _ = json.Marshal(invalid)
	if json.Unmarshal(raw, &saved) == nil {
		t.Fatal("unversioned independent task snapshot accepted")
	}
	p.TokenRates = pricingtime.TokenRates{}
	if _, err := json.Marshal(TaskBillingSnapshot{PriceData: *p}); err != nil {
		t.Fatalf("legacy task snapshot must remain writable: %v", err)
	}
}

func TestIndependentTaskBaseWindowReservationIncludesExpensiveDetails(t *testing.T) {
	p := &PriceData{ModelRatio: 1, CompletionRatio: 2, CacheRatio: 9, AudioRatio: 7, GroupRatio: 1, OtherRatio: 1, TokenTimeAdjustment: pricingtime.TokenAdjustment{Timezone: "UTC"}}
	if got := taskTokenReserveQuota(p, 4_000_000, 2_000_000); got != 25_000_000 {
		t.Fatalf("outside-window reservation missed expensive base classes: %d", got)
	}
}
