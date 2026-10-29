package relay

import (
	"encoding/json"
	"starhub/model"
	"strconv"
)

// FinalizeTaskQuota uses the durable result and preserves outcome independently
// of whether the platform charges for a failed or cancelled task.
func FinalizeTaskQuota(task *model.Task) (bool, error) {
	status := 1
	if task.PendingStatus == "failed" {
		status = 2
	}
	if task.PendingStatus == "cancelled" {
		status = 3
	}
	original := task.RequestedModel
	if original == "" {
		original = task.Model
	}
	entry := &model.Log{UserID: task.UserID, TokenID: task.TokenID, TokenName: task.TokenName,
		ChannelID: task.ChannelID, Model: task.Model, RequestedModel: original, Protocol: task.Protocol,
		Quota: task.PendingQuota, UsedChannels: strconv.FormatInt(task.ChannelID, 10), Status: status}
	if task.Platform == "responses" {
		var result model.BackgroundResponseOutcome
		if json.Unmarshal([]byte(task.PendingResultMeta), &result) == nil {
			entry.PromptTokens = result.PromptTokens
			entry.CompletionTokens = result.CompletionTokens
		}
	}
	claimed, err := model.SettleTaskQuota(task, entry)
	if claimed && err == nil {
		applyAffiliateRebate(entry.ID)
	}
	return claimed, err
}
