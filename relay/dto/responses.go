package dto

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
)

type responsesUsage struct {
	InputTokens  *int64 `json:"input_tokens"`
	OutputTokens *int64 `json:"output_tokens"`
	InputDetails *struct {
		CachedTokens int64 `json:"cached_tokens"`
	} `json:"input_tokens_details"`
	OutputDetails *struct {
		ReasoningTokens int64 `json:"reasoning_tokens"`
	} `json:"output_tokens_details"`
}

// ParseResponsesUsage handles a native response or a terminal event envelope.
// Cached and reasoning tokens are subsets of the input/output totals, never
// additional billable tokens. A missing report stays distinct from valid zero.
func ParseResponsesUsage(body []byte) (*Usage, error) {
	if trimmed := bytes.TrimSpace(body); len(trimmed) == 0 || trimmed[0] != '{' {
		return nil, errors.New("invalid Responses payload")
	}
	var envelope struct {
		Usage    *responsesUsage `json:"usage"`
		Response json.RawMessage `json:"response"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, err
	}
	if len(envelope.Response) > 0 && !bytes.Equal(bytes.TrimSpace(envelope.Response), []byte("null")) {
		var response struct {
			Usage *responsesUsage `json:"usage"`
		}
		if err := json.Unmarshal(envelope.Response, &response); err != nil {
			return nil, err
		}
		envelope.Usage = response.Usage
	}
	usage := envelope.Usage
	if usage == nil {
		return nil, nil
	}
	if usage.InputTokens == nil || usage.OutputTokens == nil {
		return nil, errors.New("incomplete Responses usage report")
	}
	input, output := *usage.InputTokens, *usage.OutputTokens
	if input < 0 || output < 0 || output > math.MaxInt64-input {
		return nil, errors.New("invalid Responses token totals")
	}
	result := &Usage{PromptTokens: input, CompletionTokens: output}
	if usage.InputDetails != nil {
		result.PromptTokensDetails = &PromptTokensDetails{CachedTokens: clampNonNegative(usage.InputDetails.CachedTokens, input)}
	}
	if usage.OutputDetails != nil {
		result.CompletionTokensDetails = &CompletionTokensDetails{ReasoningTokens: clampNonNegative(usage.OutputDetails.ReasoningTokens, output)}
	}
	return result, nil
}
