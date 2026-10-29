package relay

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"sync"

	"starhub/model"
	"starhub/pkg/billingexpr"
	"starhub/pkg/pricingtime"
	"starhub/pkg/taskplugin"
	"starhub/relay/channel"
	"starhub/relay/dto"
)

var taskPluginCache sync.Map

// LoadTaskPlugin always checks enablement in storage; old tasks may load disabled versions.
func LoadTaskPlugin(ctx context.Context, id int64, submitting bool) (*taskplugin.Plugin, error) {
	stored, err := model.GetTaskPlugin(id)
	if err != nil {
		return nil, errors.New("task plugin version unavailable")
	}
	if submitting && !stored.Enabled {
		return nil, errors.New("task plugin version disabled")
	}
	if cached, ok := taskPluginCache.Load(stored.SourceHash); ok {
		return cached.(*taskplugin.Plugin), nil
	}
	compiled, err := taskplugin.Compile(ctx, stored.Source)
	if err != nil {
		return nil, err
	}
	taskPluginCache.Store(stored.SourceHash, compiled)
	return compiled, nil
}

// PluginTaskQuota accepts only host prices and bounded numeric usage facts.
func PluginTaskQuota(pd *PriceData, usage taskplugin.Usage, status string) (int64, error) {
	if err := usage.Validate(); err != nil {
		return 0, err
	}
	if pd.HasIndependentTaskPricing() {
		if usage.Tokens == nil {
			return 0, errors.New("independent task prices require explicit token usage")
		}
		return tokenUsageQuota(pd, usage.Tokens), nil
	}
	if status == "failed" || status == "cancelled" {
		return 0, nil
	}
	if usage.Seconds <= 0 || usage.N <= 0 {
		return 0, errors.New("duration prices require seconds and n")
	}
	vars := map[string]float64{"seconds": usage.Seconds, "n": usage.N, "task_status": 1}
	if pd.IsExpression() {
		usd, err := billingexpr.Eval(pd.PriceExpr, vars)
		if err != nil || math.IsNaN(usd) || math.IsInf(usd, 0) || usd < 0 {
			return 0, errors.New("plugin task pricing expression failed")
		}
		return priceUSDToQuota(pd, usd), nil
	}
	if !pd.HasModelPrice {
		return 0, errors.New("configure a model unit price or expression before submitting plugin tasks")
	}
	return priceUSDToQuota(pd, pd.ModelPriceUSD*usage.Seconds*usage.N), nil
}

type PluginTaskPricingSnapshot struct {
	Version   int                 `json:"version,omitempty"`
	Estimate  channel.TaskPricing `json:"token_estimate,omitempty"`
	PriceData PriceData           `json:"price_data"`
	Usage     taskplugin.Usage    `json:"usage"`
}

func PluginTaskFinalQuota(task *model.Task, result taskplugin.Result) (int64, error) {
	var snapshot PluginTaskPricingSnapshot
	if err := json.Unmarshal([]byte(task.PricingMeta), &snapshot); err != nil {
		return 0, errors.New("invalid plugin task pricing snapshot")
	}
	if snapshot.Version < 2 && !snapshot.PriceData.TokenRates.Valid() {
		snapshot.PriceData.TokenTimeAdjustment = pricingtime.TokenAdjustment{}
	}
	if snapshot.PriceData.HasIndependentTaskPricing() {
		if snapshot.Version != 2 {
			return 0, errors.New("unsupported plugin task pricing snapshot")
		}
		if result.Usage != nil && result.Usage.Tokens != nil {
			return PluginTaskQuota(&snapshot.PriceData, *result.Usage, result.Status)
		}
		estimate := snapshot.Estimate
		if estimate.PromptTokens < 0 || estimate.CompletionTokens < 0 {
			return 0, errors.New("invalid plugin task token estimate")
		}
		if result.Status == "failed" {
			return 0, nil
		}
		if result.Status == "cancelled" {
			estimate.PromptTokens /= 2
			estimate.CompletionTokens /= 2
		}
		return TaskPricingQuotaWithPriceData(&snapshot.PriceData, estimate), nil
	}
	if snapshot.Version < 0 || snapshot.Version > 2 {
		return 0, errors.New("unsupported plugin task pricing snapshot")
	}
	if result.Usage != nil {
		snapshot.Usage = *result.Usage
	}
	return PluginTaskQuota(&snapshot.PriceData, snapshot.Usage, result.Status)
}

// PluginTaskReservation prices host estimates without trusting a plugin price.
func PluginTaskReservation(pd *PriceData, request *dto.TaskRequest, usage taskplugin.Usage) (int64, error) {
	if !pd.HasIndependentTaskPricing() {
		return PluginTaskQuota(pd, usage, "succeeded")
	}
	if err := usage.Validate(); err != nil {
		return 0, err
	}
	estimate := TaskTokenEstimate(request)
	return TaskPricingQuotaWithPriceData(pd, estimate), nil
}
