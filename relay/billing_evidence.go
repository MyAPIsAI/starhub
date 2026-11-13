package relay

import (
	"encoding/json"
	"errors"
	"strconv"

	"starhub/common"
	"starhub/model"
	"starhub/pkg/taskplugin"
	"starhub/relay/channel"
	"starhub/relay/dto"
)

// BillingEvidence contains prices and classified meter facts, never content.
// The whole document is frozen inside the durable settlement intent.
type BillingEvidence struct {
	Version int                  `json:"version"`
	Kind    string               `json:"kind"`
	Price   PriceData            `json:"price"`
	Usage   *dto.Usage           `json:"usage,omitempty"`
	Native  *channel.TaskPricing `json:"native,omitempty"`
	Plugin  *PluginBillingFacts  `json:"plugin,omitempty"`
}

type PluginBillingFacts struct {
	Version        int                 `json:"version"`
	Estimate       channel.TaskPricing `json:"estimate"`
	AdmissionUsage taskplugin.Usage    `json:"admission_usage"`
	ActualUsage    *taskplugin.Usage   `json:"actual_usage,omitempty"`
	Status         string              `json:"status"`
}

func EncodeTokenBillingEvidence(price *PriceData, usage *Usage) (string, error) {
	entry := &model.Log{}
	if err := attachTokenBillingEvidence(entry, price, usage); err != nil {
		return "", err
	}
	return entry.BillingEvidence, nil
}

func EncodeNativeBillingEvidence(price *PriceData, pricing channel.TaskPricing) (string, error) {
	entry := &model.Log{}
	usage := pricing.Usage
	if usage == nil {
		usage = &dto.Usage{PromptTokens: pricing.PromptTokens, CompletionTokens: pricing.CompletionTokens}
	}
	if err := setBillingEvidence(entry, BillingEvidence{Version: 1, Kind: "native", Price: *price, Native: &pricing, Usage: usage}); err != nil {
		return "", err
	}
	return entry.BillingEvidence, nil
}

func EncodePluginBillingEvidence(snapshot PluginTaskPricingSnapshot, result taskplugin.Result) (string, error) {
	entry := &model.Log{}
	usage := result.Usage
	var tokens *dto.Usage
	if usage != nil {
		tokens = usage.Tokens
	}
	if tokens == nil && snapshot.PriceData.HasIndependentTaskPricing() {
		tokens = &dto.Usage{PromptTokens: snapshot.Estimate.PromptTokens, CompletionTokens: snapshot.Estimate.CompletionTokens}
	}
	evidence := BillingEvidence{Version: 1, Kind: "plugin", Price: snapshot.PriceData, Usage: tokens, Plugin: &PluginBillingFacts{Version: snapshot.Version, Estimate: snapshot.Estimate, AdmissionUsage: snapshot.Usage, ActualUsage: usage, Status: result.Status}}
	if err := setBillingEvidence(entry, evidence); err != nil {
		return "", err
	}
	return entry.BillingEvidence, nil
}

func ApplyBillingEvidence(entry *model.Log, raw string) error {
	var evidence BillingEvidence
	if json.Unmarshal([]byte(raw), &evidence) != nil || evidence.Version != 1 || evidence.Price.ModelName != entry.Model || evidence.Price.AmountUnit != entry.AmountUnit {
		return errors.New("billing evidence does not match settlement")
	}
	entry.PriceVersion = evidence.Price.PriceVersion
	if evidence.Usage != nil {
		entry.PromptTokens, entry.CompletionTokens = evidence.Usage.PromptTokens, evidence.Usage.CompletionTokens
	}
	entry.BillingEvidence = raw
	return nil
}

func attachTokenBillingEvidence(entry *model.Log, price *PriceData, usage *Usage) error {
	if price == nil {
		return errors.New("billing price snapshot is required")
	}
	copy := &dto.Usage{PromptTokens: max(usage.PromptTokens, 0), CompletionTokens: max(usage.CompletionTokens, 0)}
	copy.PromptTokensDetails = &dto.PromptTokensDetails{CachedTokens: usage.CachedTokens(), CacheWriteTokens: usage.CacheWriteTokens(), AudioTokens: usage.InputAudioTokens()}
	copy.CompletionTokensDetails = &dto.CompletionTokensDetails{AudioTokens: usage.OutputAudioTokens()}
	if usage.CompletionTokensDetails != nil {
		copy.CompletionTokensDetails.ReasoningTokens = min(max(usage.CompletionTokensDetails.ReasoningTokens, 0), copy.CompletionTokens)
	}
	entry.PriceVersion = price.PriceVersion
	entry.PromptTokens, entry.CompletionTokens = copy.PromptTokens, copy.CompletionTokens
	frozen := *price
	if frozen.ModelName == "" {
		frozen.ModelName = entry.Model
	}
	return setBillingEvidence(entry, BillingEvidence{Version: 1, Kind: "tokens", Price: frozen, Usage: copy})
}

func setBillingEvidence(entry *model.Log, evidence BillingEvidence) error {
	encoded, err := json.Marshal(evidence)
	if err != nil {
		return errors.New("billing evidence could not be encoded")
	}
	entry.BillingEvidence = string(encoded)
	return nil
}

// RecomputeBilledNano applies the frozen monetary contract. Legacy rounding is
// preserved; the returned decimal string avoids overflowing old quota values.
func RecomputeBilledNano(entry *model.Log) (string, error) {
	var evidence BillingEvidence
	if json.Unmarshal([]byte(entry.BillingEvidence), &evidence) != nil || evidence.Version != 1 {
		return "", errors.New("billing evidence unavailable or unsupported")
	}
	if evidence.Price.PriceVersion != entry.PriceVersion || evidence.Price.ModelName != entry.Model || evidence.Price.AmountUnit != entry.AmountUnit || (evidence.Usage != nil && (evidence.Usage.PromptTokens != entry.PromptTokens || evidence.Usage.CompletionTokens != entry.CompletionTokens)) {
		return "", errors.New("billing evidence does not match the metering record")
	}
	var quota int64
	switch evidence.Kind {
	case "tokens", "image_per_call":
		if evidence.Usage == nil {
			return "", errors.New("missing classified token facts")
		}
		quota = ComputeUsageQuota(&evidence.Price, evidence.Usage)
	case "image_tokens":
		if evidence.Usage == nil {
			return "", errors.New("missing image token facts")
		}
		quota = tokenUsageQuota(&evidence.Price, evidence.Usage)
	case "native", "image_expression":
		if evidence.Native == nil {
			return "", errors.New("missing task pricing facts")
		}
		quota = TaskPricingQuotaWithPriceData(&evidence.Price, *evidence.Native)
	case "plugin":
		if evidence.Plugin == nil {
			return "", errors.New("missing plugin pricing facts")
		}
		facts := evidence.Plugin
		if facts.Status != dto.TaskStatusSucceeded && facts.Status != dto.TaskStatusFailed && facts.Status != dto.TaskStatusCancelled {
			return "", errors.New("invalid plugin billing outcome")
		}
		snapshot := PluginTaskPricingSnapshot{Version: facts.Version, PriceData: evidence.Price, Estimate: facts.Estimate, Usage: facts.AdmissionUsage}
		raw, err := json.Marshal(snapshot)
		if err != nil {
			return "", err
		}
		task := &model.Task{PricingMeta: string(raw), QuotaReservationID: entry.QuotaReservationID}
		quota, err = PluginTaskFinalQuota(task, taskplugin.Result{Usage: facts.ActualUsage, Status: facts.Status})
		if err != nil {
			return "", err
		}
	default:
		return "", errors.New("unsupported billing evidence kind")
	}
	if evidence.Price.AmountUnit == common.MoneyUnitNano {
		return strconv.FormatInt(quota, 10), nil
	}
	return common.QuotaMoney(quota, 0).String(), nil
}
