package service

import (
	"encoding/json"
	"testing"

	"starhub/model"
	"starhub/relay"
	"starhub/relay/channel"
	"starhub/relay/channel/openai"
	"starhub/relay/dto"
	"starhub/setting"
)

func TestTimePricingDoesNotApplyNewSchedulesToLegacyTaskSnapshots(t *testing.T) {
	setting.ResetAllToDefaults()
	t.Cleanup(setting.ResetAllToDefaults)
	if err := setting.ApplyOptions(map[string]string{"model_price": `{"sora-2":0.01}`, setting.OptionKeyModelTimePricing: `{"sora-2":{"timezone":"UTC","windows":[{"id":"all","label":"All","days":[0,1,2,3,4,5,6],"start":"00:00","end":"24:00","multiplier":10}]}}`}); err != nil {
		t.Fatal(err)
	}
	request, _ := json.Marshal(dto.TaskRequest{Model: "sora-2", Prompt: "a shape", Seconds: 8, N: 2})
	task := &model.Task{Model: "sora-2", Group: "default", RequestMeta: string(request)}
	adaptor := openai.VideoTaskAdaptor{}
	result := &dto.TaskResult{Status: dto.TaskStatusSucceeded}
	if got, err := taskFinalQuota(task, adaptor, result); err != nil || got != 80000 {
		t.Fatalf("legacy task without pricing metadata received a new time multiplier: %d", got)
	}
	snapshot := relay.TaskBillingSnapshot{PriceData: relay.PriceData{ModelName: "sora-2", GroupRatio: 1, OtherRatio: 1}, Estimate: channel.TaskPricing{PerCallUSD: 0.01, Units: 16}}
	encoded, _ := json.Marshal(snapshot)
	task.PricingMeta = string(encoded)
	if got, err := taskFinalQuota(task, adaptor, result); err != nil || got != 80000 {
		t.Fatalf("old persisted pricing snapshot received a time multiplier: %d", got)
	}
	snapshot.PriceData = *relay.NewPriceData("sora-2", "default")
	encoded, _ = json.Marshal(snapshot)
	task.PricingMeta = string(encoded)
	if err := setting.UpdateOption(setting.OptionKeyModelTimePricing, "{}"); err != nil {
		t.Fatal(err)
	}
	if got, err := taskFinalQuota(task, adaptor, result); err != nil || got != 800000 {
		t.Fatalf("time-aware task lost its original adjustment: %d", got)
	}
}
