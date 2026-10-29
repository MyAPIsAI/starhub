package relay

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"sync"

	"starhub/model"
	"starhub/pkg/billingexpr"
	"starhub/pkg/taskplugin"
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
	if pd.TokenRates.Valid() {
		return 0, errors.New("independent token prices are not supported for tasks")
	}
	if status == "failed" || status == "cancelled" {
		return 0, nil
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
	PriceData PriceData        `json:"price_data"`
	Usage     taskplugin.Usage `json:"usage"`
}

func PluginTaskFinalQuota(task *model.Task, result taskplugin.Result) (int64, error) {
	var snapshot PluginTaskPricingSnapshot
	if err := json.Unmarshal([]byte(task.PricingMeta), &snapshot); err != nil {
		return 0, errors.New("invalid plugin task pricing snapshot")
	}
	if result.Usage != nil {
		snapshot.Usage = *result.Usage
	}
	return PluginTaskQuota(&snapshot.PriceData, snapshot.Usage, result.Status)
}
