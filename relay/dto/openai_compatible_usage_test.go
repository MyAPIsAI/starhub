package dto

import (
	"fmt"
	"testing"
)

func TestCompatibleOpenAIUsageFields(t *testing.T) {
	for _, tc := range []struct {
		name, fields string
		cached       int64
	}{
		{"legacy cache", `"prompt_cache_hit_tokens":60,"prompt_cache_miss_tokens":40`, 60},
		{"standard priority", `"prompt_cache_hit_tokens":60,"prompt_tokens_details":{"cached_tokens":20}`, 20},
		{"explicit zero", `"prompt_cache_hit_tokens":60,"prompt_tokens_details":{"cached_tokens":0}`, 0},
		{"bounded cache", `"prompt_cache_hit_tokens":200`, 100},
		{"negative cache", `"prompt_cache_hit_tokens":-1`, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			usage := fmt.Sprintf(`{"prompt_tokens":100,"completion_tokens":50,"completion_tokens_details":{"reasoning_tokens":30},"RequestProtocol":"forged",%s}`, tc.fields)
			response, err := ParseOpenAIChatResponse([]byte(`{"choices":[{"message":{"content":"result"}}],"usage":` + usage + `}`))
			if err != nil || response.Usage == nil || response.Usage.CachedTokens() != tc.cached || response.Usage.TotalTokens() != 150 || response.Usage.CompletionTokensDetails.ReasoningTokens != 30 || response.Usage.RequestProtocol != "" {
				t.Fatalf("usage: %+v %v", response, err)
			}
			for _, placement := range []string{
				`{"choices":[],"usage":` + usage + `}`,
				`{"choices":[{"delta":{},"usage":` + usage + `}]}`,
			} {
				_, got, _, err := ParseOpenAIStreamPayload([]byte(placement))
				if err != nil || got == nil || got.CachedTokens() != tc.cached || got.TotalTokens() != 150 {
					t.Fatalf("stream usage: %+v %v", got, err)
				}
			}
		})
	}
	_, usage, _, err := ParseOpenAIStreamPayload([]byte(`{"usage":{"prompt_tokens":1,"completion_tokens":2},"choices":[{"usage":{"prompt_tokens":999,"completion_tokens":999}}]}`))
	if err != nil || usage.TotalTokens() != 3 {
		t.Fatal("choice usage overrode top-level usage")
	}
}
