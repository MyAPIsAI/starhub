package relay

import (
	"strconv"
	"strings"

	"starhub/relay/dto"
)

// Image billing model (P3.1): per-call priced image models bill
//
//	pre   = model_price x sizeRatio x qualityRatio x n(requested)
//	settle= model_price x sizeRatio x qualityRatio x count(actual)
//
// with the actual count read from the response data array (guarded into
// [1, MaxImageN]); token-billed models keep the usage-based path. Size x
// quality and count are deliberately two independent multipliers.
//
// When model_price_expr is configured, the expression receives count, size
// (long-edge pixels), quality (numeric ratio), and token variables; its
// result is the complete USD price — size/quality/count are NOT applied as
// OtherRatio on top. This lets operators express tiered / volume-discounted
// pricing that the fixed-multiplier per-call path cannot.

// imageSizeRatio scales the per-call price by resolution, relative to each
// model's base size (list prices of the public price sheets).
var imageSizeRatio = map[string]float64{
	// dall-e-3 (base 1024x1024, $0.04)
	"1024x1024": 1,
	"1024x1792": 2.025,
	"1792x1024": 2.025,
	// dall-e-2 (base 256x256, $0.016)
	"256x256":     1,
	"512x512":     1.75,
	"1024x1024d2": 2.1875, // internal marker, see sizeRatio()
	// gpt-image-1 (base 1024x1024)
	"1536x1024": 1.5,
	"1024x1536": 1.5,
	"auto":      1,
}

// imageQualityRatio scales the per-call price by quality.
var imageQualityRatio = map[string]float64{
	"standard": 1,
	"hd":       2,
	"low":      0.5,
	"medium":   1,
	"high":     1.5,
	"auto":     1,
}

// ImagePriceRatio returns the size x quality multiplier of the request
// (1 for unknown values: unpriceable combinations must not zero the bill).
func ImagePriceRatio(model, size, quality string) float64 {
	ratio := sizeRatio(model, size)
	if q, ok := imageQualityRatio[strings.ToLower(quality)]; ok && quality != "" {
		ratio *= q
	}
	return ratio
}

func sizeRatio(model, size string) float64 {
	if size == "" {
		return 1
	}
	if strings.Contains(model, "dall-e-2") && size == "1024x1024" {
		if v, ok := imageSizeRatio["1024x1024d2"]; ok {
			return v
		}
	}
	if v, ok := imageSizeRatio[size]; ok {
		return v
	}
	return 1
}

// ImageBill carries the billing inputs of one image request.
type ImageBill struct {
	Model, Group  string
	UserGroup     string // user's own group (for the group x group matrix)
	Size, Quality string
	Prompt        string
	Count         int // requested n (already validated [1, MaxImageN])
}

// priceData builds the PriceData with the user group so the group x group
// matrix multiplier applies.
func (b *ImageBill) priceData() *PriceData {
	if b.UserGroup != "" {
		return NewPriceDataWithUserGroup(b.Model, b.Group, b.UserGroup)
	}
	return NewPriceData(b.Model, b.Group)
}

// imageSizeValue extracts the long-edge pixel count from a "<w>x<h>" size
// string (e.g. "1024x1792" -> 1792). "auto" and unparseable sizes map to
// 1024 (the base resolution) so expression authors get a stable default.
// The long edge is used (not the short edge) because it distinguishes every
// preset tier: 1024x1024=1024, 1024x1792=1792, 1536x1024=1536, etc.
func imageSizeValue(size string) float64 {
	parts := strings.SplitN(size, "x", 2)
	if len(parts) != 2 {
		return 1024
	}
	w, err1 := strconv.Atoi(parts[0])
	h, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return 1024
	}
	if w > h {
		return float64(w)
	}
	return float64(h)
}

// imageQualityValue maps a quality string to its numeric ratio for
// expression evaluation: low=0.5 standard=1 medium=1 high=1.5 hd=2 auto=1.
// Unknown / empty quality maps to 1 (standard) so an unpriceable
// combination never zeroes the bill.
func imageQualityValue(quality string) float64 {
	if q, ok := imageQualityRatio[strings.ToLower(quality)]; ok && quality != "" {
		return q
	}
	return 1
}

// evalImageExpr evaluates the pricing expression with image-specific
// variables (count, size, quality, prompt_tokens, completion_tokens).
// The expression result is the complete USD price — size/quality/count are
// NOT applied as OtherRatio on top. Returns (quota, true) on success and
// (0, false) on failure (caller falls back to per-call / token billing).
func (b *ImageBill) evalImageExpr(pd *PriceData, count int, promptTokens, completionTokens int64) (int64, bool) {
	vars := map[string]float64{
		"count":             float64(count),
		"size":              imageSizeValue(b.Size),
		"quality":           imageQualityValue(b.Quality),
		"prompt_tokens":     float64(promptTokens),
		"completion_tokens": float64(completionTokens),
		"total_tokens":      float64(promptTokens + completionTokens),
	}
	return evalExpression(pd, vars)
}

// PreQuota prices the pre-charge of the request.
func (b *ImageBill) PreQuota() int64 {
	pd := b.priceData()
	promptEst := dto.EstimateTokens(b.Prompt) + estimateReserveTokens
	if pd.IsExpression() {
		// Expression path: count/size/quality are variables in the
		// expression, NOT OtherRatio multipliers. Pre-charge uses the
		// requested n (already an upper bound) and prompt estimate.
		if q, ok := b.evalImageExpr(pd, b.Count, promptEst, 0); ok {
			return q
		}
		// expression failed: fall through to per-call / token fallback
	}
	if pd.IsPerCall() {
		return b.quotaForCount(b.Count)
	}
	// Token-billed image models: the prompt drives the estimate.
	return tokenUsageQuota(pd, &Usage{PromptTokens: promptEst})
}

// SettleQuota prices the terminal bill. actualCount is the image count the
// upstream actually delivered; values outside [1, MaxImageN] are ignored so
// a broken upstream payload cannot poison the books (the requested count
// stays the bill of record, P3.1 guardrail).
func (b *ImageBill) SettleQuota(actualCount int, usage *dto.ImageUsage) int64 {
	pd := b.priceData()
	count := actualCount
	if count <= 0 || count > dto.MaxImageN {
		count = b.Count
	}
	if pd.IsExpression() {
		// Expression path: settle uses the actual delivered count and
		// the real usage (if the upstream reports tokens).
		var promptTok, completionTok int64
		if usage != nil && (usage.InputTokens > 0 || usage.OutputTokens > 0) {
			promptTok = usage.InputTokens
			completionTok = usage.OutputTokens
		} else {
			promptTok = dto.EstimateTokens(b.Prompt) + estimateReserveTokens
		}
		if q, ok := b.evalImageExpr(pd, count, promptTok, completionTok); ok {
			return q
		}
		// expression failed: fall through to per-call / token fallback
	}
	if pd.IsPerCall() {
		return b.quotaForCount(count)
	}
	// Token-billed image models: usage from the response (input_tokens ->
	// prompt, output_tokens -> completion; the overwrite mapping is
	// image-path-only and must not leak into chat / embedding settlement).
	if usage != nil && (usage.InputTokens > 0 || usage.OutputTokens > 0) {
		return tokenUsageQuota(pd, &Usage{
			PromptTokens:     usage.InputTokens,
			CompletionTokens: usage.OutputTokens,
		})
	}
	// No usage report: keep the prompt estimate (settled against the
	// same estimate as the pre-charge).
	return tokenUsageQuota(pd, &Usage{PromptTokens: dto.EstimateTokens(b.Prompt) + estimateReserveTokens})
}

func (b *ImageBill) quotaForCount(count int) int64 {
	pd := b.priceData()
	// Fold size x quality x count into the special-multiplier chain: the
	// per-call price is multiplied by the full chain (group ratio included).
	pd.AddOtherRatio(ImagePriceRatio(b.Model, b.Size, b.Quality))
	pd.AddOtherRatio(float64(count))
	return ComputeUsageQuota(pd, &Usage{})
}
