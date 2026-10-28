package dto

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
)

// ParseGeminiUsage normalizes native usageMetadata for shared billing.
// Gemini reports thoughts and tool prompts separately from candidate and
// prompt tokens. Cached tokens remain a subset of the original prompt.
func ParseGeminiUsage(body []byte) (*Usage, error) {
	if trimmed := bytes.TrimSpace(body); len(trimmed) == 0 || trimmed[0] != '{' {
		return nil, errors.New("invalid Gemini payload")
	}
	var envelope struct {
		Usage *struct {
			Prompt     *int64 `json:"promptTokenCount"`
			Candidates *int64 `json:"candidatesTokenCount"`
			Tools      *int64 `json:"toolUsePromptTokenCount"`
			Thoughts   *int64 `json:"thoughtsTokenCount"`
			Cached     *int64 `json:"cachedContentTokenCount"`
			Total      *int64 `json:"totalTokenCount"`
		} `json:"usageMetadata"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, err
	}
	u := envelope.Usage
	if u == nil {
		return nil, nil
	}
	value := func(n *int64) int64 {
		if n == nil {
			return 0
		}
		return *n
	}
	for _, count := range []*int64{u.Prompt, u.Candidates, u.Tools, u.Thoughts, u.Cached, u.Total} {
		if value(count) < 0 {
			return nil, errors.New("negative Gemini token count")
		}
	}
	if u.Prompt == nil && u.Candidates == nil && u.Tools == nil && u.Thoughts == nil {
		return nil, nil
	}
	input, output := value(u.Prompt), value(u.Candidates)
	if value(u.Tools) > math.MaxInt64-input || value(u.Thoughts) > math.MaxInt64-output {
		return nil, errors.New("overflowing Gemini token count")
	}
	input += value(u.Tools)
	output += value(u.Thoughts)
	if output > math.MaxInt64-input {
		return nil, errors.New("overflowing Gemini token total")
	}
	// totalTokenCount is a consistency summary, never an extra charge or
	// a substitute from which unknown categories are guessed.
	result := &Usage{PromptTokens: input, CompletionTokens: output}
	if u.Cached != nil {
		result.PromptTokensDetails = &PromptTokensDetails{CachedTokens: clampNonNegative(*u.Cached, value(u.Prompt))}
	}
	if u.Thoughts != nil {
		result.CompletionTokensDetails = &CompletionTokensDetails{ReasoningTokens: *u.Thoughts}
	}
	return result, nil
}
