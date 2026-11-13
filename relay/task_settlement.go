package relay

import (
	"encoding/json"
	"starhub/common"
	"starhub/model"
	"starhub/relay/dto"
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
		Quota: task.PendingQuota, UsedChannels: strconv.FormatInt(task.ChannelID, 10), Status: status,
		ExactFeeMicroUSD: QuotaToMicroUSD(task.PendingQuota), ExactFeeNanoUSD: common.QuotaMoney(task.PendingQuota, 0).String()}
	if common.IsNanoReservationID(task.QuotaReservationID) {
		entry.AmountUnit = common.MoneyUnitNano
		entry.ExactFeeMicroUSD = task.PendingQuota / 1000
		entry.ExactFeeNanoUSD = strconv.FormatInt(task.PendingQuota, 10)
	}
	if task.Platform == "responses" {
		var result model.BackgroundResponseOutcome
		if json.Unmarshal([]byte(task.PendingResultMeta), &result) == nil {
			entry.UsageSource = result.UsageSource
			entry.PromptTokens = result.PromptTokens
			entry.CompletionTokens = result.CompletionTokens
			if result.BillingEvidence != "" {
				if err := ApplyBillingEvidence(entry, result.BillingEvidence); err != nil {
					return false, err
				}
			}
		}
	}
	if task.Platform != "responses" {
		var result dto.TaskResult
		if json.Unmarshal([]byte(task.PendingResultMeta), &result) == nil {
			entry.UsageSource = result.BillingUsageSource
			if result.Usage != nil {
				entry.PromptTokens = result.Usage.PromptTokens
				entry.CompletionTokens = result.Usage.CompletionTokens
			}
			if result.BillingEvidence != "" {
				if err := ApplyBillingEvidence(entry, result.BillingEvidence); err != nil {
					return false, err
				}
			}
		}
	}
	claimed, err := model.SettleTaskQuota(task, entry)
	if claimed && err == nil {
		applyAffiliateRebate(entry.ID)
	}
	return claimed, err
}
