package dto

import (
	"fmt"
	"math"
)

// Claude input_tokens excludes cache reads and writes. Stream deltas contain
// cumulative counters and may omit unchanged fields, including valid zeroes.
type ClaudeUsageFields struct {
	InputTokens              *int64 `json:"input_tokens"`
	OutputTokens             *int64 `json:"output_tokens"`
	CacheReadInputTokens     *int64 `json:"cache_read_input_tokens"`
	CacheCreationInputTokens *int64 `json:"cache_creation_input_tokens"`
}

func (fields *ClaudeUsageFields) normalized() (*Usage, error) {
	if fields == nil {
		return nil, nil
	}
	if fields.InputTokens == nil && fields.OutputTokens == nil && fields.CacheReadInputTokens == nil && fields.CacheCreationInputTokens == nil {
		return nil, nil
	}
	value := func(field *int64) int64 {
		if field == nil {
			return 0
		}
		return *field
	}
	input, output, read, write := value(fields.InputTokens), value(fields.OutputTokens), value(fields.CacheReadInputTokens), value(fields.CacheCreationInputTokens)
	if input < 0 || output < 0 || read < 0 || write < 0 || read > math.MaxInt64-input || write > math.MaxInt64-input-read {
		return nil, fmt.Errorf("invalid Claude input token totals")
	}
	prompt := input + read + write
	if output > math.MaxInt64-prompt {
		return nil, fmt.Errorf("invalid Claude total token count")
	}
	usage := &Usage{PromptTokens: prompt, CompletionTokens: output}
	if fields.CacheReadInputTokens != nil || fields.CacheCreationInputTokens != nil {
		usage.PromptTokensDetails = &PromptTokensDetails{CachedTokens: read, CacheWriteTokens: write}
	}
	return usage, nil
}

// ClaudeUsageAccumulator merges by field presence, never by addition or >0.
// Returned snapshots own their detail objects and failed updates are atomic.
type ClaudeUsageAccumulator struct{ fields ClaudeUsageFields }

func (acc *ClaudeUsageAccumulator) Merge(update *ClaudeUsageFields) (*Usage, error) {
	if update == nil {
		return acc.Snapshot(), nil
	}
	next := acc.fields
	assign := func(target **int64, source *int64) {
		if source != nil {
			value := *source
			*target = &value
		}
	}
	assign(&next.InputTokens, update.InputTokens)
	assign(&next.OutputTokens, update.OutputTokens)
	assign(&next.CacheReadInputTokens, update.CacheReadInputTokens)
	assign(&next.CacheCreationInputTokens, update.CacheCreationInputTokens)
	snapshot, err := next.normalized()
	if err != nil {
		return nil, err
	}
	acc.fields = next
	return snapshot, nil
}

func (acc *ClaudeUsageAccumulator) Snapshot() *Usage {
	usage, _ := acc.fields.normalized()
	return usage
}

type ClaudeUsageResponse struct {
	InputTokens              int64 `json:"input_tokens"`
	OutputTokens             int64 `json:"output_tokens"`
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens,omitempty"`
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens,omitempty"`
}

// ClaudeUsageFromIR reverses the native input split without counting cache
// tokens twice. Cache writes retain the platform input rate unless overridden.
func ClaudeUsageFromIR(usage *Usage) ClaudeUsageResponse {
	if usage == nil {
		return ClaudeUsageResponse{}
	}
	read, write := usage.CachedTokens(), usage.CacheWriteTokens()
	return ClaudeUsageResponse{InputTokens: clampNonNegative(usage.PromptTokens-read-write, usage.PromptTokens), OutputTokens: clampNonNegative(usage.CompletionTokens, usage.CompletionTokens), CacheReadInputTokens: read, CacheCreationInputTokens: write}
}
