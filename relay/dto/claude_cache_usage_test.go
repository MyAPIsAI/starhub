package dto

import (
	"encoding/json"
	"math"
	"reflect"
	"testing"
)

func TestClaudeResponseCountsCacheReadsAndWritesAsInput(t *testing.T) {
	response, err := ParseClaudeChatResponse([]byte(`{"id":"cached","model":"m","content":[{"type":"text","text":"local"}],"stop_reason":"end_turn","usage":{"input_tokens":10,"cache_read_input_tokens":100,"cache_creation_input_tokens":50,"output_tokens":20}}`))
	if err != nil || response.Usage == nil || response.Usage.PromptTokens != 160 || response.Usage.CachedTokens() != 100 {
		t.Fatalf("native cache input omitted: %+v %v", response, err)
	}
}

func TestClaudeUsageMergesCumulativePresentFieldsAndPreservesSnapshots(t *testing.T) {
	parse := func(raw string) *ClaudeUsageFields {
		t.Helper()
		event, err := ParseClaudeStreamEvent([]byte(raw))
		if err != nil {
			t.Fatal(err)
		}
		return event.UsageFields
	}
	var acc ClaudeUsageAccumulator
	first, err := acc.Merge(parse(`{"type":"message_start","message":{"usage":{"input_tokens":10,"cache_read_input_tokens":100,"cache_creation_input_tokens":50,"output_tokens":0}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if first.PromptTokens != 160 || first.CacheWriteTokens() != 50 {
		t.Fatal("input classes lost")
	}
	intermediate, err := acc.Merge(parse(`{"type":"message_delta","usage":{"output_tokens":3}}`))
	if err != nil || intermediate.PromptTokens != 160 || intermediate.CompletionTokens != 3 {
		t.Fatal("partial output removed input")
	}
	final, err := acc.Merge(parse(`{"type":"message_delta","usage":{"output_tokens":20}}`))
	if err != nil || final.CompletionTokens != 20 || final.TotalTokens() != 180 {
		t.Fatal("cumulative output added twice")
	}
	raw := parse(`{"type":"message_delta","usage":{"input_tokens":0,"cache_read_input_tokens":0,"cache_creation_input_tokens":15,"output_tokens":0}}`)
	cleared, err := acc.Merge(raw)
	if err != nil || cleared.PromptTokens != 15 || cleared.CompletionTokens != 0 || cleared.CachedTokens() != 0 || cleared.CacheWriteTokens() != 15 {
		t.Fatal("explicit zero ignored")
	}
	*raw.CacheCreationInputTokens = 999
	cleared.PromptTokensDetails.CacheWriteTokens = 777
	if got := acc.Snapshot(); got.CacheWriteTokens() != 15 {
		t.Fatal("caller mutated accumulator")
	}
	if first.PromptTokens != 160 || first.CompletionTokens != 0 || intermediate.CompletionTokens != 3 || final.CompletionTokens != 20 {
		t.Fatal("later update mutated prior snapshots")
	}
	before := acc.Snapshot()
	negative := int64(-1)
	if _, err := acc.Merge(&ClaudeUsageFields{OutputTokens: &negative}); err == nil {
		t.Fatal("negative cumulative report accepted")
	}
	overflow := int64(math.MaxInt64)
	if _, err := acc.Merge(&ClaudeUsageFields{CacheReadInputTokens: &overflow}); err == nil {
		t.Fatal("merged input overflow accepted")
	}
	if !reflect.DeepEqual(before, acc.Snapshot()) {
		t.Fatal("rejected update changed known usage")
	}
}

func TestClaudeCacheUsageRoundTripsAndClampsDisjointIRDetails(t *testing.T) {
	usage := &Usage{PromptTokens: 160, CompletionTokens: 20, PromptTokensDetails: &PromptTokensDetails{CachedTokens: 100, CacheWriteTokens: 50}}
	body, err := BuildClaudeChatResponse(&ChatResponse{ID: "cached", Model: "m", Text: "local", FinishReason: "stop", Usage: usage})
	if err != nil {
		t.Fatal(err)
	}
	var wire struct {
		Usage ClaudeUsageResponse `json:"usage"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		t.Fatal(err)
	}
	if wire.Usage.InputTokens != 10 || wire.Usage.OutputTokens != 20 || wire.Usage.CacheReadInputTokens != 100 || wire.Usage.CacheCreationInputTokens != 50 {
		t.Fatalf("native output double-counted cache: %+v", wire.Usage)
	}
	parsed, err := ParseClaudeChatResponse(body)
	if err != nil || !reflect.DeepEqual(parsed.Usage, usage) {
		t.Fatalf("usage round trip: %+v %v", parsed, err)
	}
	malformed := &Usage{PromptTokens: 100, CompletionTokens: 5, PromptTokensDetails: &PromptTokensDetails{CachedTokens: 80, CacheWriteTokens: 50, AudioTokens: 30}, CompletionTokensDetails: &CompletionTokensDetails{AudioTokens: 10}}
	if malformed.CachedTokens() != 80 || malformed.CacheWriteTokens() != 20 || malformed.InputAudioTokens() != 0 || malformed.OutputAudioTokens() != 5 {
		t.Fatal("input classes exceeded total")
	}
	copied := malformed.Clone()
	copied.PromptTokensDetails.CachedTokens = 0
	copied.CompletionTokensDetails.AudioTokens = 0
	if malformed.PromptTokensDetails.CachedTokens != 80 || malformed.CompletionTokensDetails.AudioTokens != 10 {
		t.Fatal("cloned detail snapshots alias source")
	}
}

func TestClaudeCacheUsageRejectsNegativeOverflowAndMalformedCounts(t *testing.T) {
	for _, usage := range []string{`{"input_tokens":-1,"output_tokens":1}`, `{"input_tokens":1,"cache_read_input_tokens":-1,"output_tokens":1}`, `{"input_tokens":1,"cache_creation_input_tokens":-1,"output_tokens":1}`, `{"input_tokens":1,"output_tokens":-1}`, `{"input_tokens":9223372036854775807,"cache_read_input_tokens":1}`, `{"input_tokens":9223372036854775807,"cache_creation_input_tokens":1}`, `{"input_tokens":9223372036854775807,"output_tokens":1}`, `{"input_tokens":1,"cache_read_input_tokens":1.5}`, `{"input_tokens":1,"cache_creation_input_tokens":"50"}`} {
		if _, err := ParseClaudeChatResponse([]byte(`{"content":[],"usage":` + usage + `}`)); err == nil {
			t.Fatalf("invalid native usage accepted: %s", usage)
		}
	}
	for _, raw := range []string{`{"content":[]}`, `{"content":[],"usage":null}`, `{"content":[],"usage":{}}`} {
		if response, err := ParseClaudeChatResponse([]byte(raw)); err != nil || response.Usage != nil {
			t.Fatal("missing report invented counters")
		}
	}
}

func TestClaudeStreamStartCountsAllInputClasses(t *testing.T) {
	event, err := ParseClaudeStreamEvent([]byte(`{"type":"message_start","message":{"id":"cached","model":"m","usage":{"input_tokens":10,"cache_read_input_tokens":100,"cache_creation_input_tokens":50,"output_tokens":0}}}`))
	if err != nil || event.Usage == nil || event.Usage.PromptTokens != 160 || event.Usage.CachedTokens() != 100 {
		t.Fatalf("stream cache input omitted: %+v %v", event, err)
	}
}
