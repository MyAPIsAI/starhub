package dto

import (
	"encoding/json"
	"errors"
)

const maxTaskTokens int64 = 1<<53 - 1

func ValidateTaskUsage(usage *Usage) error {
	if usage == nil {
		return errors.New("task token usage is missing")
	}
	if usage.PromptTokens < 0 || usage.CompletionTokens < 0 || usage.PromptTokens > maxTaskTokens || usage.CompletionTokens > maxTaskTokens-usage.PromptTokens {
		return errors.New("task token totals are outside safe bounds")
	}
	if d := usage.PromptTokensDetails; d != nil && (d.CachedTokens < 0 || d.CacheWriteTokens < 0 || d.AudioTokens < 0 || d.CachedTokens > maxTaskTokens || d.CacheWriteTokens > maxTaskTokens || d.AudioTokens > maxTaskTokens) {
		return errors.New("task input token details are outside safe bounds")
	}
	if d := usage.CompletionTokensDetails; d != nil && (d.AudioTokens < 0 || d.ReasoningTokens < 0 || d.AudioTokens > maxTaskTokens || d.ReasoningTokens > maxTaskTokens) {
		return errors.New("task output token details are outside safe bounds")
	}
	return nil
}

// ParseTaskUsage distinguishes missing facts from an explicit zero measurement.
func ParseTaskUsage(raw json.RawMessage) (*Usage, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var fields struct {
		Prompt            *int64                   `json:"prompt_tokens"`
		Input             *int64                   `json:"input_tokens"`
		Completion        *int64                   `json:"completion_tokens"`
		Output            *int64                   `json:"output_tokens"`
		PromptDetails     *PromptTokensDetails     `json:"prompt_tokens_details"`
		InputDetails      *PromptTokensDetails     `json:"input_tokens_details"`
		CompletionDetails *CompletionTokensDetails `json:"completion_tokens_details"`
		OutputDetails     *CompletionTokensDetails `json:"output_tokens_details"`
	}
	if json.Unmarshal(raw, &fields) != nil {
		return nil, errors.New("invalid task token usage")
	}
	input, output := fields.Prompt, fields.Completion
	if input == nil {
		input = fields.Input
	} else if fields.Input != nil && *fields.Input != *input {
		return nil, errors.New("conflicting task token totals")
	}
	if output == nil {
		output = fields.Output
	} else if fields.Output != nil && *fields.Output != *output {
		return nil, errors.New("conflicting task token totals")
	}
	if input == nil || output == nil {
		return nil, errors.New("task token usage requires explicit input and output totals")
	}
	usage := &Usage{PromptTokens: *input, CompletionTokens: *output, PromptTokensDetails: fields.PromptDetails, CompletionTokensDetails: fields.CompletionDetails}
	if usage.PromptTokensDetails == nil {
		usage.PromptTokensDetails = fields.InputDetails
	}
	if usage.CompletionTokensDetails == nil {
		usage.CompletionTokensDetails = fields.OutputDetails
	}
	return usage, ValidateTaskUsage(usage)
}
