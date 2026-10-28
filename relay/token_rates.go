package relay

import (
	"log"
	"math/big"
	"strconv"

	"starhub/common"
)

// independentTokenQuota keeps decimal prices and token products exact through
// final rounding, including counts too large for int64 multiplication.
func independentTokenQuota(p *PriceData, counts [6]int64) int64 {
	nanos := p.TokenRates.NanoPrices()
	total := new(big.Int)
	for index, count := range counts {
		if count > 0 {
			total.Add(total, new(big.Int).Mul(big.NewInt(count), big.NewInt(nanos[index])))
		}
	}
	ratio, ok := new(big.Rat).SetString(strconv.FormatFloat(p.EffectiveRatio(), 'g', -1, 64))
	if !ok || ratio.Sign() == 0 || total.Sign() == 0 {
		return 0
	}
	quota := new(big.Rat).SetFrac(total, big.NewInt(1_000_000*1_000_000_000))
	quota.Mul(quota, new(big.Rat).SetInt64(common.QuotaPerUSD))
	quota.Mul(quota, ratio)
	if quota.Cmp(new(big.Rat).SetInt64(common.MaxQuotaPerRequest)) >= 0 {
		log.Printf("[%s] model %q independent token price exceeds quota cap %d, clamped", common.QuotaSaturation, p.ModelName, common.MaxQuotaPerRequest)
		return common.MaxQuotaPerRequest
	}
	// Positive tariffs retain the minimum paid charge. Explicitly free usage
	// stays free, and the reservation layer keeps its existing one-quota hold.
	whole, remainder := new(big.Int), new(big.Int)
	whole.QuoRem(quota.Num(), quota.Denom(), remainder)
	if remainder.Lsh(remainder, 1).Cmp(quota.Denom()) >= 0 {
		whole.Add(whole, big.NewInt(1))
	}
	return max64(whole.Int64(), 1)
}

func independentUsageQuota(p *PriceData, usage *Usage) int64 {
	prompt, completion := max64(usage.PromptTokens, 0), max64(usage.CompletionTokens, 0)
	read, write := usage.CachedTokens(), usage.CacheWriteTokens()
	audioIn, audioOut := usage.InputAudioTokens(), usage.OutputAudioTokens()
	return independentTokenQuota(p, [6]int64{prompt - read - write - audioIn, completion - audioOut, read, write, audioIn, audioOut})
}

// Token details are unknown before dispatch. Reserve at the most expensive
// input/output class rather than assuming cache reads always give a discount.
func tokenReserveQuota(p *PriceData, prompt, completion int64) int64 {
	if !p.TokenRates.Valid() {
		return tokenUsageQuota(p, &Usage{PromptTokens: prompt, CompletionTokens: completion})
	}
	prices := p.TokenRates.NanoPrices()
	input, output := 0, 1
	for _, index := range []int{2, 3, 4} {
		if prices[index] > prices[input] {
			input = index
		}
	}
	if prices[5] > prices[output] {
		output = 5
	}
	var counts [6]int64
	counts[input] = max64(prompt, 0)
	counts[output] = max64(completion, 0)
	return independentTokenQuota(p, counts)
}
