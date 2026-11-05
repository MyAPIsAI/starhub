package relay

import (
	"encoding/json"
	"starhub/model"
	"starhub/relay/dto"
	"strconv"
	"time"
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
	entry := &model.Log{TokenPeriodStartedAt: task.TokenPeriodStartedAt, UserID: task.UserID, TokenID: task.TokenID, TokenName: task.TokenName,
		ChannelID: task.ChannelID, Model: task.Model, RequestedModel: original, Protocol: task.Protocol,
		Quota: task.PendingQuota, UsedChannels: strconv.FormatInt(task.ChannelID, 10), Status: status}
	entry.TraceID = task.TraceID
	entry.DurationMs = max(task.CompleteTime-task.SubmitTime, 0) * 1000
	callType := task.CallType
	if callType == "" {
		callType = task.Platform
	}
	model.FreezeLogLabels(entry, task.Group, callType)
	model.ApplyTaskRouting(task, entry)
	if status == 2 {
		entry.ErrorCode = "task_failed"
	}
	if status == 3 {
		entry.ErrorCode = "request_cancelled"
	}
	if task.Platform == "responses" {
		var result model.BackgroundResponseOutcome
		if json.Unmarshal([]byte(task.PendingResultMeta), &result) == nil {
			entry.UsageSource = result.UsageSource
			entry.BillingDetail = result.BillingDetail
			entry.CacheReadTokens, entry.CacheWriteTokens = result.CacheReadTokens, result.CacheWriteTokens
			entry.PromptTokens = result.PromptTokens
			entry.CompletionTokens = result.CompletionTokens
		}
	}
	if task.Platform != "responses" {
		var result dto.TaskResult
		if json.Unmarshal([]byte(task.PendingResultMeta), &result) == nil {
			entry.UsageSource = result.BillingUsageSource
			entry.BillingDetail = result.BillingDetail
			if result.Usage != nil {
				entry.PromptTokens = result.Usage.PromptTokens
				entry.CompletionTokens = result.Usage.CompletionTokens
				entry.CacheReadTokens = result.Usage.CachedTokens()
				entry.CacheWriteTokens = result.Usage.CacheWriteTokens()
			}
		}
	}
	if entry.DurationMs == 0 && task.SubmitTime > 0 {
		entry.DurationMs = max(time.Now().Unix()-task.SubmitTime, 0) * 1000
	}
	claimed, err := model.SettleTaskQuota(task, entry)
	if claimed && err == nil {
		applyAffiliateRebate(entry.ID)
	}
	return claimed, err
}
