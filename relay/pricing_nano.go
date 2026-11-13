package relay

import (
	"log"
	"math/big"
	"strconv"

	"starhub/common"
	"starhub/pkg/billingexpr"
)

func decimalRatio(value float64, name string) *big.Rat {
	value = common.ClampRatio(value, name)
	ratio, ok := new(big.Rat).SetString(strconv.FormatFloat(value, 'g', -1, 64))
	if !ok {
		return new(big.Rat)
	}
	return ratio
}

func nanoEffectiveRatio(p *PriceData) *big.Rat {
	ratio := decimalRatio(p.GroupRatio, "group")
	other := decimalRatio(p.OtherRatio, "other")
	if p.OtherRatioExact != "" {
		if exact, ok := new(big.Rat).SetString(p.OtherRatioExact); ok && exact.Sign() >= 0 && exact.Cmp(new(big.Rat).SetInt64(int64(common.MaxRatioProduct))) <= 0 {
			other = exact
		}
	}
	ratio.Mul(ratio, other)
	if p.TimeAdjustment.Multiplier != 0 {
		ratio.Mul(ratio, decimalRatio(p.TimeAdjustment.Multiplier, "time"))
	}
	if p.UserGroup != "" && p.UsingGroup != "" {
		ratio.Mul(ratio, decimalRatio(p.GroupGroupRatio, "group_group"))
	}
	cap := new(big.Rat).SetInt64(int64(common.MaxRatioProduct))
	if ratio.Cmp(cap) > 0 {
		log.Printf("[%s] effective multiplier exceeds cap", common.QuotaSaturation)
		return cap
	}
	return ratio
}

func roundNanoPrice(nano *big.Rat) int64 {
	if nano.Sign() <= 0 {
		return 0
	}
	if nano.Cmp(new(big.Rat).SetInt64(common.MaxNanoPerRequest)) >= 0 {
		log.Printf("[%s] nano price exceeds request cap", common.QuotaSaturation)
		return common.MaxNanoPerRequest
	}
	whole, remainder := new(big.Int), new(big.Int)
	whole.QuoRem(nano.Num(), nano.Denom(), remainder)
	if remainder.Lsh(remainder, 1).Cmp(nano.Denom()) >= 0 {
		whole.Add(whole, big.NewInt(1))
	}
	return max(int64(1), whole.Int64())
}

func priceUSDNano(p *PriceData, usd float64) int64 {
	value, ok := new(big.Rat).SetString(strconv.FormatFloat(usd, 'g', -1, 64))
	if !ok || value.Sign() <= 0 {
		return 0
	}
	value.Mul(value, new(big.Rat).SetInt64(common.NanoPerUSD))
	value.Mul(value, nanoEffectiveRatio(p))
	return roundNanoPrice(value)
}

// ComputeUsageNano keeps token products, decimal tariffs and individual
// multipliers rational until one final half-up rounding at the nano-USD boundary.
// A positive price has a one-nano minimum; explicitly free tariffs remain free.
func ComputeUsageNano(p *PriceData, usage *Usage) int64 {
	if usage == nil {
		usage = &Usage{}
	}
	if p.IsExpression() {
		vars := map[string]float64{"prompt_tokens": float64(usage.PromptTokens), "completion_tokens": float64(usage.CompletionTokens), "total_tokens": float64(usage.PromptTokens) + float64(usage.CompletionTokens)}
		if usd, err := billingexpr.Eval(p.PriceExpr, vars); err == nil {
			return priceUSDNano(p, usd)
		}
	}
	if p.IsPerCall() {
		return priceUSDNano(p, p.ModelPriceUSD)
	}
	return tokenUsageNano(p, usage)
}

func tokenUsageNano(p *PriceData, usage *Usage) int64 {
	prompt, completion := max(usage.PromptTokens, 0), max(usage.CompletionTokens, 0)
	read, write, audioIn, audioOut := usage.CachedTokens(), usage.CacheWriteTokens(), usage.InputAudioTokens(), usage.OutputAudioTokens()
	if p.TokenRates.Valid() {
		return independentTokenNano(p, [6]int64{prompt - read - write - audioIn, completion - audioOut, read, write, audioIn, audioOut})
	}
	input := new(big.Rat).SetInt64(max(int64(0), prompt-read-audioIn))
	input.Add(input, new(big.Rat).Mul(new(big.Rat).SetInt64(read), decimalRatio(p.CacheRatio, "cache")))
	input.Add(input, new(big.Rat).Mul(new(big.Rat).SetInt64(audioIn), decimalRatio(p.AudioRatio, "audio")))
	output := new(big.Rat).Mul(new(big.Rat).SetInt64(max(int64(0), completion-audioOut)), decimalRatio(p.CompletionRatio, "completion"))
	output.Add(output, new(big.Rat).Mul(new(big.Rat).SetInt64(audioOut), decimalRatio(p.AudioRatio, "audio")))
	input.Add(input, output)
	input.Mul(input, decimalRatio(p.ModelRatio, "model"))
	input.Mul(input, new(big.Rat).SetInt64(1000))
	input.Mul(input, nanoEffectiveRatio(p))
	return roundNanoPrice(input)
}

func independentTokenNano(p *PriceData, counts [6]int64) int64 {
	total := new(big.Int)
	prices := p.TokenRates.NanoPrices()
	for i, count := range counts {
		if count > 0 {
			total.Add(total, new(big.Int).Mul(big.NewInt(count), big.NewInt(prices[i])))
		}
	}
	value := new(big.Rat).SetFrac(total, big.NewInt(1_000_000))
	value.Mul(value, nanoEffectiveRatio(p))
	return roundNanoPrice(value)
}
