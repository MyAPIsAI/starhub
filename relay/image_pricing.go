package relay

import (
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
	Size, Quality string
	Prompt        string
	Count         int // requested n (already validated [1, MaxImageN])
}

// PreQuota prices the pre-charge of the request.
func (b *ImageBill) PreQuota() int64 {
	pd := NewPriceData(b.Model, b.Group)
	if !pd.IsPerCall() && !pd.IsExpression() {
		// Token-billed image models: the prompt drives the estimate.
		return estimatePreConsume(pd, dto.EstimateTokens(b.Prompt)+estimateReserveTokens, 0)
	}
	return b.quotaForCount(b.Count)
}

// SettleQuota prices the terminal bill. actualCount is the image count the
// upstream actually delivered; values outside [1, MaxImageN] are ignored so
// a broken upstream payload cannot poison the books (the requested count
// stays the bill of record, P3.1 guardrail).
func (b *ImageBill) SettleQuota(actualCount int, usage *dto.ImageUsage) int64 {
	pd := NewPriceData(b.Model, b.Group)
	if !pd.IsPerCall() && !pd.IsExpression() {
		if usage != nil && (usage.InputTokens > 0 || usage.OutputTokens > 0) {
			// Image APIs report usage as input_tokens/output_tokens; the
			// overwrite mapping is image-path-only and must not leak into
			// chat / embedding settlement (P3.1 warning).
			return ComputeUsageQuota(pd, &Usage{
				PromptTokens:     usage.InputTokens,
				CompletionTokens: usage.OutputTokens,
			})
		}
		// No usage report: keep the prompt estimate (settled against the
		// same estimate as the pre-charge).
		return estimatePreConsume(pd, dto.EstimateTokens(b.Prompt)+estimateReserveTokens, 0)
	}
	count := actualCount
	if count <= 0 || count > dto.MaxImageN {
		count = b.Count
	}
	return b.quotaForCount(count)
}

func (b *ImageBill) quotaForCount(count int) int64 {
	pd := NewPriceData(b.Model, b.Group)
	// Fold size x quality x count into the special-multiplier chain: the
	// per-call price is multiplied by the full chain (group ratio included).
	pd.AddOtherRatio(ImagePriceRatio(b.Model, b.Size, b.Quality))
	pd.AddOtherRatio(float64(count))
	return ComputeUsageQuota(pd, &Usage{})
}
