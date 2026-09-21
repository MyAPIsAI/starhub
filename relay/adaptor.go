package relay

import (
	"fmt"
	"net/http"
)

// Usage mirrors the OpenAI usage report. The detail fields are subsets of
// the totals (OpenAI counts cached / audio tokens inside prompt_tokens and
// completion_tokens) and drive the fine-grained settlement pricing.
type Usage struct {
	PromptTokens            int64                    `json:"prompt_tokens"`
	CompletionTokens        int64                    `json:"completion_tokens"`
	PromptTokensDetails     *PromptTokensDetails     `json:"prompt_tokens_details,omitempty"`
	CompletionTokensDetails *CompletionTokensDetails `json:"completion_tokens_details,omitempty"`
}

// PromptTokensDetails breaks down prompt tokens (all subsets of
// PromptTokens).
type PromptTokensDetails struct {
	CachedTokens int64 `json:"cached_tokens"`
	AudioTokens  int64 `json:"audio_tokens"`
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

// InputAudioTokens returns the audio prompt token count, clamped so that
// cached + audio never exceeds the prompt total.
func (u *Usage) InputAudioTokens() int64 {
	if u == nil || u.PromptTokensDetails == nil {
		return 0
	}
	remaining := u.PromptTokens - clampNonNegative(u.PromptTokensDetails.CachedTokens, u.PromptTokens)
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

const ChannelTypeOpenAI = 1

type Adaptor interface {
	Name() string
	ConvertRequestBody(body []byte, stream bool) ([]byte, error)
	RequestURL(baseURL string) string
	SetupRequestHeader(req *http.Request, channelKey string)
	HandleResponse(w http.ResponseWriter, resp *http.Response, stream bool) (*Usage, string, error)
}

func GetAdaptor(channelType int) (Adaptor, error) {
	switch channelType {
	case ChannelTypeOpenAI:
		return openaiAdaptor{}, nil
	default:
		return nil, fmt.Errorf("unsupported channel type: %d", channelType)
	}
}
