// Package dto holds the unified intermediate representation (IR) that the
// relay core speaks, plus the wire-format DTOs of every supported protocol.
//
// The IR is what makes "any inbound protocol x any outbound channel" work:
// an inbound request is parsed into IR once, every channel adaptor serializes
// IR into its own wire format, and upstream responses are parsed back into
// IR before being re-serialized for the client (P3.2 format matrix).
package dto

// Usage mirrors the OpenAI usage report. The detail fields are subsets of
// the totals (OpenAI counts cached / audio tokens inside prompt_tokens and
// completion_tokens) and drive the fine-grained settlement pricing.
type Usage struct {
	// RequestProtocol is gateway-owned metadata; upstream JSON cannot set it.
	RequestProtocol string `json:"-"`
	// RequestStatus is a gateway-owned outcome, defaulting to success.
	RequestStatus           int                      `json:"-"`
	PromptTokens            int64                    `json:"prompt_tokens"`
	CompletionTokens        int64                    `json:"completion_tokens"`
	PromptTokensDetails     *PromptTokensDetails     `json:"prompt_tokens_details,omitempty"`
	CompletionTokensDetails *CompletionTokensDetails `json:"completion_tokens_details,omitempty"`
}

// PromptTokensDetails breaks down prompt tokens (all subsets of
// PromptTokens).
type PromptTokensDetails struct {
	CachedTokens int64 `json:"cached_tokens"`
	// Cache writes are an optional gateway extension for native providers.
	CacheWriteTokens int64 `json:"cache_write_tokens,omitempty"`
	AudioTokens      int64 `json:"audio_tokens"`
}

// CacheWriteTokens is disjoint from cache reads and bounded by the prompt total.
func (u *Usage) CacheWriteTokens() int64 {
	if u == nil || u.PromptTokensDetails == nil {
		return 0
	}
	return clampNonNegative(u.PromptTokensDetails.CacheWriteTokens, u.PromptTokens-u.CachedTokens())
}

// CompletionTokensDetails breaks down completion tokens (all subsets of
// CompletionTokens).
type CompletionTokensDetails struct {
	AudioTokens     int64 `json:"audio_tokens"`
	ReasoningTokens int64 `json:"reasoning_tokens"`
}

// CachedTokens returns the cached prompt token count, clamped into range.
func (u *Usage) CachedTokens() int64 {
	if u == nil || u.PromptTokensDetails == nil {
		return 0
	}
	return clampNonNegative(u.PromptTokensDetails.CachedTokens, u.PromptTokens)
}

// InputAudioTokens is clamped after cache reads and writes, so the disjoint
// input classes never exceed the prompt total.
func (u *Usage) InputAudioTokens() int64 {
	if u == nil || u.PromptTokensDetails == nil {
		return 0
	}
	remaining := u.PromptTokens - u.CachedTokens() - u.CacheWriteTokens()
	return clampNonNegative(u.PromptTokensDetails.AudioTokens, remaining)
}

// OutputAudioTokens returns the audio completion token count, clamped into
// range.
func (u *Usage) OutputAudioTokens() int64 {
	if u == nil || u.CompletionTokensDetails == nil {
		return 0
	}
	return clampNonNegative(u.CompletionTokensDetails.AudioTokens, u.CompletionTokens)
}

// TotalTokens returns the prompt + completion total.
func (u *Usage) TotalTokens() int64 {
	if u == nil {
		return 0
	}
	return u.PromptTokens + u.CompletionTokens
}

// Clone detaches all mutable detail objects for frozen stream snapshots.
func (u *Usage) Clone() *Usage {
	if u == nil {
		return nil
	}
	copied := *u
	if u.PromptTokensDetails != nil {
		details := *u.PromptTokensDetails
		copied.PromptTokensDetails = &details
	}
	if u.CompletionTokensDetails != nil {
		details := *u.CompletionTokensDetails
		copied.CompletionTokensDetails = &details
	}
	return &copied
}

func clampNonNegative(v, upper int64) int64 {
	if v < 0 {
		return 0
	}
	if upper < 0 {
		return 0
	}
	if v > upper {
		return upper
	}
	return v
}
