package relay

import (
	"encoding/json"
	"sort"
	"strconv"

	"starhub/model"
	"starhub/pkg/taskplugin"
	"starhub/relay/channel"
)

func TaskBillingDetails(pd *PriceData, p channel.TaskPricing) string {
	if pd == nil {
		return ""
	}
	u := p.Usage
	if u == nil {
		u = &Usage{PromptTokens: p.PromptTokens, CompletionTokens: p.CompletionTokens}
	}
	var detail model.BillingDetail
	_ = json.Unmarshal([]byte(BillingSnapshot(pd, u)), &detail)
	if pd.HasIndependentTaskPricing() {
		// Missing provider details are billed using the captured maximum-rate classes.
		if p.Usage == nil && pd.TokenRates.Valid() {
			counts := [6]int64{}
			prices := pd.TokenRates.NanoPrices()
			in, out := 0, 1
			for _, i := range []int{2, 3, 4} {
				if prices[i] > prices[in] {
					in = i
				}
			}
			if prices[5] > prices[out] {
				out = 5
			}
			counts[in], counts[out] = p.PromptTokens, p.CompletionTokens
			for i := range detail.Parts {
				detail.Parts[i].Quantity = strconv.FormatInt(counts[i], 10)
			}
		}
	} else {
		// Task expression variables differ from synchronous token variables.
		validExpression := false
		if pd.IsExpression() && len(p.ExprVars) > 0 {
			_, validExpression = evalExpression(pd, p.ExprVars)
		}
		if validExpression {
			detail.Mode = "expression"
			detail.Formula = pd.PriceExpr
			detail.Parts = nil
			names := make([]string, 0, len(p.ExprVars))
			for name := range p.ExprVars {
				names = append(names, name)
			}
			sort.Strings(names)
			for _, name := range names {
				value := p.ExprVars[name]
				detail.Parts = append(detail.Parts, model.BillingPart{Name: name, Quantity: strconv.FormatFloat(value, 'g', -1, 64), Unit: "variable"})
			}
		} else if p.PerCallUSD > 0 {
			detail.Mode = "unit"
			detail.Formula = ""
			detail.Parts = []model.BillingPart{{Name: "billable_units", Quantity: strconv.FormatFloat(p.Units, 'g', -1, 64), Unit: "unit", UnitPriceUSD: strconv.FormatFloat(p.PerCallUSD*pd.EffectiveRatio(), 'g', -1, 64)}}
		} else {
			fallback := *pd
			fallback.PriceExpr = ""
			fallback.HasModelPrice = false
			fallback.ModelPriceUSD = 0
			_ = json.Unmarshal([]byte(BillingSnapshot(&fallback, &Usage{PromptTokens: p.PromptTokens, CompletionTokens: p.CompletionTokens})), &detail)
		}
	}
	raw, _ := json.Marshal(detail)
	return string(raw)
}

func PluginBillingDetails(snapshot PluginTaskPricingSnapshot, result taskplugin.Result) string {
	pd := snapshot.PriceData
	if pd.HasIndependentTaskPricing() {
		p := snapshot.Estimate
		if result.Usage != nil && result.Usage.Tokens != nil {
			p.Usage = result.Usage.Tokens
		} else if result.Status == "failed" {
			p.PromptTokens, p.CompletionTokens = 0, 0
		} else if result.Status == "cancelled" {
			p.PromptTokens /= 2
			p.CompletionTokens /= 2
		}
		return TaskBillingDetails(&pd, p)
	}
	u := snapshot.Usage
	if result.Usage != nil {
		u = *result.Usage
	}
	units := u.Seconds * u.N
	if result.Status == "failed" || result.Status == "cancelled" {
		units = 0
	}
	p := channel.TaskPricing{PerCallUSD: pd.ModelPriceUSD, Units: units}
	if pd.IsExpression() && result.Status != "failed" && result.Status != "cancelled" {
		p.ExprVars = map[string]float64{"seconds": u.Seconds, "n": u.N, "task_status": 1}
	}
	data := TaskBillingDetails(&pd, p)
	var detail model.BillingDetail
	if (result.Status == "failed" || result.Status == "cancelled") && json.Unmarshal([]byte(data), &detail) == nil {
		detail.Mode, detail.Formula, detail.Parts = "not_charged", "", []model.BillingPart{}
		raw, _ := json.Marshal(detail)
		return string(raw)
	}
	if json.Unmarshal([]byte(data), &detail) == nil && detail.Mode == "unit" {
		detail.Parts[0].Name = "seconds"
		detail.Parts[0].Unit = "second"
		raw, _ := json.Marshal(detail)
		return string(raw)
	}
	return data
}
