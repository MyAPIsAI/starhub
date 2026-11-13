package relay

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"starhub/common"
	"starhub/model"
	"starhub/setting"
)

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
