package relay

import (
	"encoding/json"
	"math/big"
	"strconv"
	"strings"
	"time"

	"starhub/model"
	"starhub/relay/dto"
)

// BillingSnapshot records effective tariffs and numeric metering only.
// Rounding and collection remain exclusively in the existing settlement path.
func BillingSnapshot(pd *PriceData, u *Usage) string {
	if pd == nil {
		return ""
	}
	if u == nil {
		u = &Usage{}
	}
	at := pd.PricingTime
	if at.IsZero() {
		at = time.Now()
	}
	detail := model.BillingDetail{Version: 1, Mode: "tokens", PricedAt: at.Format(time.RFC3339), GroupMultiplier: strconv.FormatFloat(pd.EffectiveRatio(), 'g', -1, 64), Parts: []model.BillingPart{}}
	add := func(name string, count int64, unit, price string) {
		detail.Parts = append(detail.Parts, model.BillingPart{Name: name, Quantity: strconv.FormatInt(count, 10), Unit: unit, UnitPriceUSD: price})
	}
	format := func(rate float64) string { return formatUnitRate(rate) }
	vars := map[string]float64{"prompt_tokens": float64(u.PromptTokens), "completion_tokens": float64(u.CompletionTokens), "total_tokens": float64(u.PromptTokens + u.CompletionTokens)}
	expressionValid := false
	if pd.IsExpression() {
		_, expressionValid = evalExpression(pd, vars)
	}
	if expressionValid {
		detail.Mode = "expression"
		detail.Formula = pd.PriceExpr
		add("input_tokens", u.PromptTokens, "tokens", "")
		add("output_tokens", u.CompletionTokens, "tokens", "")
	} else if pd.IsPerCall() {
		detail.Mode = "request"
		add("requests", 1, "request", format(pd.ModelPriceUSD*pd.EffectiveRatio()))
	} else {
		counts := [6]int64{max64(u.PromptTokens, 0) - u.CachedTokens() - u.CacheWriteTokens() - u.InputAudioTokens(), max64(u.CompletionTokens, 0) - u.OutputAudioTokens(), u.CachedTokens(), u.CacheWriteTokens(), u.InputAudioTokens(), u.OutputAudioTokens()}
		names := [6]string{"input_tokens", "output_tokens", "cache_read_tokens", "cache_write_tokens", "input_audio_tokens", "output_audio_tokens"}
		rates := [6]float64{pd.ModelRatio, pd.ModelRatio * pd.CompletionRatio, pd.ModelRatio * pd.CacheRatio, pd.ModelRatio, pd.ModelRatio * pd.AudioRatio, pd.ModelRatio * pd.AudioRatio}
		for i, count := range counts {
			price := format(rates[i] * pd.EffectiveRatio())
			if pd.TokenRates.Valid() {
				ratio, _ := new(big.Rat).SetString(strconv.FormatFloat(pd.EffectiveRatio(), 'g', -1, 64))
				rate := new(big.Rat).SetFrac(big.NewInt(pd.TokenRates.NanoPrices()[i]), big.NewInt(1_000_000_000))
				if ratio != nil {
					rate.Mul(rate, ratio)
				}
				price = strings.TrimRight(strings.TrimRight(rate.FloatString(18), "0"), ".")
			}
			add(names[i], max64(count, 0), "million_tokens", price)
		}
	}
	data, err := json.Marshal(detail)
	if err != nil {
		return ""
	}
	return string(data)
}

func formatUnitRate(value float64) string {
	rate, ok := new(big.Rat).SetString(strconv.FormatFloat(value, 'g', -1, 64))
	if !ok {
		return ""
	}
	return strings.TrimRight(strings.TrimRight(rate.FloatString(18), "0"), ".")
}

func (b *ImageBill) rememberBilling(count int, usage *dto.ImageUsage, countKnown bool) {
	u := &Usage{}
	b.UsageSource = "admission_estimate"
	if usage != nil && (usage.InputTokens > 0 || usage.OutputTokens > 0) {
		u.PromptTokens, u.CompletionTokens = usage.InputTokens, usage.OutputTokens
		b.UsageSource = "upstream"
	} else {
		u.PromptTokens = dto.EstimateTokens(b.Prompt) + estimateReserveTokens
	}
	b.SettledUsage = u
	pd := b.priceData()

	detail := BillingSnapshot(pd, u)
	if pd.IsExpression() {
		vars := map[string]float64{"count": float64(count), "size": imageSizeValue(b.Size), "quality": imageQualityValue(b.Quality), "prompt_tokens": float64(u.PromptTokens), "completion_tokens": float64(u.CompletionTokens), "total_tokens": float64(u.PromptTokens + u.CompletionTokens)}
		if _, ok := evalExpression(pd, vars); ok {
			var frozen model.BillingDetail
			_ = json.Unmarshal([]byte(detail), &frozen)
			frozen.Mode, frozen.Formula = "expression", pd.PriceExpr
			frozen.Parts = nil
			for _, name := range []string{"count", "size", "quality", "prompt_tokens", "completion_tokens", "total_tokens"} {
				frozen.Parts = append(frozen.Parts, model.BillingPart{Name: name, Quantity: strconv.FormatFloat(vars[name], 'g', -1, 64), Unit: "variable"})
			}
			raw, _ := json.Marshal(frozen)
			b.BillingDetail = string(raw)
			return
		}
	}
	if pd.IsPerCall() && usage == nil {
		u.PromptTokens = 0
		if countKnown {
			b.UsageSource = "upstream"
		}
	}
	if pd.IsPerCall() {
		var frozen model.BillingDetail
		if json.Unmarshal([]byte(detail), &frozen) == nil {
			frozen.Mode = "images"
			frozen.Parts = []model.BillingPart{{Name: "images", Quantity: strconv.Itoa(count), Unit: "image", UnitPriceUSD: formatUnitRate(pd.ModelPriceUSD * pd.EffectiveRatio() * ImagePriceRatio(b.Model, b.Size, b.Quality))}}
			raw, _ := json.Marshal(frozen)
			detail = string(raw)
		}
	}
	b.BillingDetail = detail
}

// PostConsumeRecordedQuota uses the same durable accounting with a richer log.
func PostConsumeRecordedQuota(entry *model.Log, unlimited bool, actual, pre int64) error {
	return settleDurableQuota(entry.UserID, entry.TokenID, unlimited, actual-pre, actual, entry)
}
