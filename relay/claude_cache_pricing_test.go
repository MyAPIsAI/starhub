package relay

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"starhub/relay/dto"
)

func TestClaudeCacheUsageBillsDisjointInputsAtFrozenPlatformRates(t *testing.T) {
	response, err := dto.ParseClaudeChatResponse([]byte(`{"usage":{"input_tokens":10,"cache_read_input_tokens":100,"cache_creation_input_tokens":50,"output_tokens":20}}`))
	if err != nil {
		t.Fatal(err)
	}
	price := &PriceData{ModelRatio: 2, CompletionRatio: 3, CacheRatio: 0.1, AudioRatio: 2, GroupRatio: 1, OtherRatio: 1}
	if got := ComputeUsageQuota(price, response.Usage); got != 130 {
		t.Fatalf("cache billed twice or omitted: %d", got)
	}
	price.GroupRatio = .8
	price.TimeAdjustment.Multiplier = .5
	if got := ComputeUsageQuota(price, response.Usage); got != 52 {
		t.Fatalf("timed/group cache bill: %d", got)
	}
}

func TestClaudeStreamWriterReturnsNativeCacheSplitAndDetachedSnapshots(t *testing.T) {
	recorder := httptest.NewRecorder()
	emitter := NewChatStreamEmitter(recorder, dto.InboundClaude, "cached")
	usage := &Usage{PromptTokens: 160, CompletionTokens: 20, PromptTokensDetails: &dto.PromptTokensDetails{CachedTokens: 100, CacheWriteTokens: 50}}
	if err := emitter.Emit(&dto.ChatStreamEvent{TextDelta: "local", Usage: usage}); err != nil {
		t.Fatal(err)
	}
	usage.PromptTokens = 999
	usage.PromptTokensDetails.CachedTokens = 0
	if err := emitter.Finish(nil, "stop"); err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, line := range strings.Split(recorder.Body.String(), "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var event struct {
			Type  string                   `json:"type"`
			Usage *dto.ClaudeUsageResponse `json:"usage"`
		}
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event); err != nil {
			t.Fatal(err)
		}
		if event.Type == "message_delta" {
			seen++
			if event.Usage == nil || event.Usage.InputTokens != 10 || event.Usage.CacheReadInputTokens != 100 || event.Usage.CacheCreationInputTokens != 50 || event.Usage.OutputTokens != 20 {
				t.Fatalf("converted native split: %+v", event.Usage)
			}
		}
	}
	if seen != 1 {
		t.Fatal("missing terminal native usage")
	}
}
