package service

import (
	"encoding/json"
	"starhub/model"
	"starhub/pkg/pricingtime"
	"starhub/pkg/taskplugin"
	"starhub/relay"
	"starhub/relay/channel"
	"starhub/relay/channel/openai"
	"starhub/relay/dto"
	"strings"
	"testing"
)

func TestTaskTokenEstimateFallbackAndSnapshotVersion(t *testing.T) {
	rates, err := pricingtime.NewTokenRates(pricingtime.TokenPrices{InputPerM: "1", OutputPerM: "2", CacheReadPerM: "3", CacheWritePerM: "4", AudioInputPerM: "5", AudioOutputPerM: "6"})
	if err != nil {
		t.Fatal(err)
	}
	pd := relay.PriceData{ModelName: "sora-2", GroupRatio: 1, GroupGroupRatio: 1, OtherRatio: 1, TokenRates: rates}
	estimate := channel.TaskPricing{PromptTokens: 400, CompletionTokens: 200}
	encoded, err := json.Marshal(relay.TaskBillingSnapshot{PriceData: pd, Estimate: estimate})
	if err != nil {
		t.Fatal(err)
	}
	task := &model.Task{Model: "sora-2", Group: "default", RequestMeta: `{"model":"sora-2","prompt":"fixture","seconds":4,"n":1}`, PricingMeta: string(encoded)}
	for _, tc := range []struct {
		status string
		want   int64
	}{{"succeeded", 1600}, {"cancelled", 800}, {"failed", 0}} {
		result := &dto.TaskResult{Status: tc.status}
		if got, err := taskFinalQuota(task, openai.VideoTaskAdaptor{}, result); err != nil || got != tc.want || result.BillingUsageSource != "admission_estimate" {
			t.Fatalf("fallback %s %d %v", tc.status, got, err)
		}
		pluginSnapshot, _ := json.Marshal(relay.PluginTaskPricingSnapshot{Version: 2, PriceData: pd, Estimate: estimate, Usage: taskplugin.Usage{Seconds: 4, N: 1}})
		pluginTask := &model.Task{PricingMeta: string(pluginSnapshot)}
		if got, err := relay.PluginTaskFinalQuota(pluginTask, taskplugin.Result{Status: tc.status}); err != nil || got != tc.want {
			t.Fatalf("plugin fallback %s %d %v", tc.status, got, err)
		}
	}
	result := &dto.TaskResult{Status: "succeeded", Usage: &dto.Usage{PromptTokens: -1}}
	if _, err := taskFinalQuota(task, openai.VideoTaskAdaptor{}, result); err == nil {
		t.Fatal("negative actual usage was charged")
	}
	task.PricingMeta = strings.Replace(string(encoded), `"version":2`, `"version":3`, 1)
	if _, err := taskFinalQuota(task, openai.VideoTaskAdaptor{}, &dto.TaskResult{Status: "succeeded"}); err == nil {
		t.Fatal("future snapshot silently interpreted")
	}
}

func TestLegacyOutsideWindowSnapshotRetainsItsOriginalTaskContract(t *testing.T) {
	pd := relay.PriceData{ModelName: "sora-2", GroupRatio: 1, OtherRatio: 1, TokenTimeAdjustment: pricingtime.TokenAdjustment{Timezone: "UTC"}}
	raw, err := json.Marshal(map[string]any{"price_data": pd, "estimate": channel.TaskPricing{PerCallUSD: 0.01, Units: 4}})
	if err != nil {
		t.Fatal(err)
	}
	task := &model.Task{Model: "sora-2", Group: "default", RequestMeta: `{"model":"sora-2","prompt":"fixture","seconds":4,"n":1}`, PricingMeta: string(raw)}
	if got, err := taskFinalQuota(task, openai.VideoTaskAdaptor{}, &dto.TaskResult{Status: "succeeded"}); err != nil || got != 20000 {
		t.Fatalf("legacy inactive-window task changed contract: %d %v", got, err)
	}
}
