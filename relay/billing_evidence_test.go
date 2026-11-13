package relay

import (
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"starhub/common"
	"starhub/model"
	"starhub/pkg/pricingtime"
	"starhub/pkg/taskplugin"
	"starhub/relay/channel"
	"starhub/relay/dto"
	"starhub/setting"
)

func TestImageBillingEvidenceReplaysEverySettlementModeWithoutPrompt(t *testing.T) {
	for _, mode := range []string{"per_call", "tokens", "expression"} {
		t.Run(mode, func(t *testing.T) {
			price := &PriceData{ModelName: "image-evidence", PriceVersion: 41, AmountUnit: common.MoneyUnitNano, ModelRatio: 1, CompletionRatio: 2, GroupRatio: 1, OtherRatio: 1}
			want := int64(14000)
			if mode == "per_call" {
				price.ModelPriceUSD = 0.000000001
				price.HasModelPrice = true
				want = 2
			}
			if mode == "expression" {
				price.PriceExpr = "count * 0.000000003"
				want = 6
			}
			bill := &ImageBill{Model: price.ModelName, Group: "default", Size: "1024x1024", Quality: "standard", Prompt: "private prompt must not reach evidence", Count: 2, Pricing: price}
			actual := bill.SettleQuota(2, &dto.ImageUsage{InputTokens: 10, OutputTokens: 2})
			if actual != want || bill.BillingEvidenceError != nil {
				t.Fatalf("image mode %s: %d %v", mode, actual, bill.BillingEvidenceError)
			}
			if strings.Contains(bill.BillingEvidence, bill.Prompt) {
				t.Fatal("prompt leaked into billing evidence")
			}
			entry := &model.Log{Model: price.ModelName, AmountUnit: price.AmountUnit}
			if err := ApplyBillingEvidence(entry, bill.BillingEvidence); err != nil {
				t.Fatal(err)
			}
			if replay, err := RecomputeBilledNano(entry); err != nil || replay != strconv.FormatInt(want, 10) {
				t.Fatalf("image replay: %s %v", replay, err)
			}
		})
	}
}

func TestFrozenBillingEvidenceMatchesIndependentClassifiedGoldenAmounts(t *testing.T) {
	setting.ResetAllToDefaults()
	t.Cleanup(setting.ResetAllToDefaults)
	rates, err := pricingtime.NewTokenRates(pricingtime.TokenPrices{InputPerM: "2", OutputPerM: "6", CacheReadPerM: "0.2", CacheWritePerM: "3", AudioInputPerM: "8", AudioOutputPerM: "10"})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name        string
		independent bool
		want        string
	}{{"ratio", false, "756000"}, {"six-explicit-rates", true, "786000"}} {
		t.Run(tc.name, func(t *testing.T) {
			price := &PriceData{PriceVersion: 71, AmountUnit: common.MoneyUnitNano, ModelName: "classified-golden", ModelRatio: 2, CompletionRatio: 3, CacheRatio: 0.1, AudioRatio: 4, GroupRatio: 0.5, GroupGroupRatio: 1.5, OtherRatio: 2, UserGroup: "customer-tier", UsingGroup: "sale-tier"}
			if tc.independent {
				price.TokenRates = rates
			}
			usage := &Usage{PromptTokens: 100, CompletionTokens: 40, PromptTokensDetails: &PromptTokensDetails{CachedTokens: 20, CacheWriteTokens: 10, AudioTokens: 15}, CompletionTokensDetails: &CompletionTokensDetails{AudioTokens: 5, ReasoningTokens: 7}}
			entry := &model.Log{Model: price.ModelName, AmountUnit: price.AmountUnit}
			if err := attachTokenBillingEvidence(entry, price, usage); err != nil {
				t.Fatal(err)
			}
			// Golden amounts are calculated from the six disjoint classes and the
			// 0.5 x 1.5 x 2 group multiplier, independently of production helpers.
			if err := setting.ApplyVersionedOptions(map[string]string{"model_ratio": `{"classified-golden":999}`, "group_ratio": `{"sale-tier":99}`}, 72); err != nil {
				t.Fatal(err)
			}
			price.ModelRatio = 999
			usage.PromptTokensDetails.CachedTokens = 0
			got, err := RecomputeBilledNano(entry)
			if err != nil || got != tc.want {
				t.Fatalf("got %q want %q: %v", got, tc.want, err)
			}
		})
	}
}

func TestBillingEvidenceRejectsCorruptUnsupportedAndMismatchedRecords(t *testing.T) {
	price := &PriceData{PriceVersion: 81, AmountUnit: common.MoneyUnitNano, ModelName: "replay-validation", ModelRatio: 1, CompletionRatio: 1, GroupRatio: 1, OtherRatio: 1}
	base := &model.Log{Model: price.ModelName, AmountUnit: price.AmountUnit}
	if err := attachTokenBillingEvidence(base, price, &Usage{PromptTokens: 2}); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"truncated", "unsupported-version", "unsupported-kind", "missing-usage", "different-model", "different-unit", "different-price", "different-usage"} {
		t.Run(mode, func(t *testing.T) {
			entry := *base
			var evidence BillingEvidence
			if err := json.Unmarshal([]byte(base.BillingEvidence), &evidence); err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "unsupported-version":
				evidence.Version = 2
			case "unsupported-kind":
				evidence.Kind = "future-kind"
			case "missing-usage":
				evidence.Usage = nil
			case "different-model":
				entry.Model = "other-model"
			case "different-unit":
				entry.AmountUnit = "unsupported-unit"
			case "different-price":
				entry.PriceVersion++
			case "different-usage":
				entry.PromptTokens++
			}
			raw, err := json.Marshal(evidence)
			if err != nil {
				t.Fatal(err)
			}
			entry.BillingEvidence = string(raw)
			if mode == "truncated" {
				entry.BillingEvidence = "{"
			}
			if amount, err := RecomputeBilledNano(&entry); err == nil || amount != "" {
				t.Fatalf("unverifiable record returned %q: %v", amount, err)
			}
		})
	}
}

func TestBillingEvidenceReplaysNativeAndPluginMeterFacts(t *testing.T) {
	price := &PriceData{ModelName: "task-evidence", PriceVersion: 31, AmountUnit: common.MoneyUnitNano, ModelPriceUSD: 0.000000001, HasModelPrice: true, GroupRatio: 1, OtherRatio: 1}
	pricing := channel.TaskPricing{PerCallUSD: 0.000000001, Units: 3}
	raw, err := EncodeNativeBillingEvidence(price, pricing)
	if err != nil {
		t.Fatal(err)
	}
	entry := &model.Log{Model: price.ModelName, AmountUnit: price.AmountUnit}
	if err := ApplyBillingEvidence(entry, raw); err != nil {
		t.Fatal(err)
	}
	if got, err := RecomputeBilledNano(entry); err != nil || got != "3" {
		t.Fatalf("native replay: %s %v", got, err)
	}
	snapshot := PluginTaskPricingSnapshot{Version: 3, PriceData: *price, Usage: taskplugin.Usage{Seconds: 2, N: 1}}
	result := taskplugin.Result{Status: "succeeded", Usage: &taskplugin.Usage{Seconds: 4, N: 2}}
	raw, err = EncodePluginBillingEvidence(snapshot, result)
	if err != nil {
		t.Fatal(err)
	}
	entry.QuotaReservationID = common.NewNanoReservationID()
	if err := ApplyBillingEvidence(entry, raw); err != nil {
		t.Fatal(err)
	}
	if got, err := RecomputeBilledNano(entry); err != nil || got != "8" {
		t.Fatalf("plugin replay: %s %v", got, err)
	}
}

func TestBillingEvidenceSurvivesDurableSettlementSerialization(t *testing.T) {
	if err := model.InitDB(filepath.Join(t.TempDir(), "evidence.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = model.CloseDB() })
	price := &PriceData{ModelName: "frozen", PriceVersion: 23, ModelRatio: 1, CompletionRatio: 2, GroupRatio: 1, OtherRatio: 1}
	usage := &Usage{PromptTokens: 10, CompletionTokens: 5, PromptTokensDetails: &PromptTokensDetails{CachedTokens: 3}}
	entry := &model.Log{Model: "frozen"}
	if err := attachTokenBillingEvidence(entry, price, usage); err != nil {
		t.Fatal(err)
	}
	actual := ComputeUsageQuota(price, usage)
	intent, err := model.RecordQuotaSettlement(1, 1, true, actual, actual, entry)
	if err != nil {
		t.Fatal(err)
	}
	entry.BillingEvidence = "changed after freeze"
	var recovered model.Log
	if err := json.Unmarshal([]byte(intent.Payload), &recovered); err != nil {
		t.Fatal(err)
	}
	value, err := RecomputeBilledNano(&recovered)
	if err != nil || value != common.QuotaMoney(actual, 0).String() || recovered.PriceVersion != 23 {
		t.Fatalf("durable evidence lost: %s %v", value, err)
	}
}

func TestBillingEvidenceReplaysFrozenPriceAndClassifiedUsage(t *testing.T) {
	setting.ResetAllToDefaults()
	t.Cleanup(setting.ResetAllToDefaults)
	if err := setting.ApplyVersionedOptions(map[string]string{"model_ratio": `{"evidence-model":2}`, "completion_ratio": `{"evidence-model":3}`, "cache_ratio": "0.1", "audio_ratio": "4"}, 17); err != nil {
		t.Fatal(err)
	}
	price := NewPriceData("evidence-model", "default")
	price.AmountUnit = common.MoneyUnitNano
	usage := &Usage{PromptTokens: 100, CompletionTokens: 40, PromptTokensDetails: &PromptTokensDetails{CachedTokens: 20, CacheWriteTokens: 10, AudioTokens: 15}, CompletionTokensDetails: &CompletionTokensDetails{AudioTokens: 5, ReasoningTokens: 7}}
	entry := &model.Log{Model: price.ModelName, AmountUnit: price.AmountUnit, PromptTokens: 100, CompletionTokens: 40}
	if err := attachTokenBillingEvidence(entry, price, usage); err != nil {
		t.Fatal(err)
	}
	want := ComputeUsageQuota(price, usage)
	if err := setting.ApplyVersionedOptions(map[string]string{"model_ratio": `{"evidence-model":200}`, "cache_ratio": "1"}, 18); err != nil {
		t.Fatal(err)
	}
	usage.PromptTokensDetails.CachedTokens = 0
	got, err := RecomputeBilledNano(entry)
	if err != nil || got != common.QuotaMoney(0, want).String() || entry.PriceVersion != 17 {
		t.Fatalf("frozen replay: %s %v version %d", got, err, entry.PriceVersion)
	}
	entry.PriceVersion = 18
	if _, err := RecomputeBilledNano(entry); err == nil {
		t.Fatal("mismatched price version accepted")
	}
}
