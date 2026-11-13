package relay

import (
	"encoding/json"
	"errors"
	"strconv"

	"starhub/common"
	"starhub/model"
	"starhub/relay/dto"
)

// BillingEvidence contains prices and classified meter facts, never content.
// The whole document is frozen inside the durable settlement intent.
type BillingEvidence struct {
	Version int        `json:"version"`
	Kind    string     `json:"kind"`
	Price   PriceData  `json:"price"`
	Usage   *dto.Usage `json:"usage,omitempty"`
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
	if json.Unmarshal([]byte(entry.BillingEvidence), &evidence) != nil || evidence.Version != 1 || evidence.Kind != "tokens" || evidence.Usage == nil {
		return "", errors.New("billing evidence unavailable or unsupported")
	}
	if evidence.Price.PriceVersion != entry.PriceVersion || evidence.Price.ModelName != entry.Model || evidence.Price.AmountUnit != entry.AmountUnit || evidence.Usage.PromptTokens != entry.PromptTokens || evidence.Usage.CompletionTokens != entry.CompletionTokens {
		return "", errors.New("billing evidence does not match the metering record")
	}
	quota := ComputeUsageQuota(&evidence.Price, evidence.Usage)
	if evidence.Price.AmountUnit == common.MoneyUnitNano {
		return strconv.FormatInt(quota, 10), nil
	}
	return common.QuotaMoney(quota, 0).String(), nil
}
