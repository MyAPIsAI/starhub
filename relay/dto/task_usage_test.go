package dto

import (
	"encoding/json"
	"testing"
)

func TestTaskTokenUsageRequiresActualSafeTotals(t *testing.T) {
	for _, raw := range []string{`{}`, `{"prompt_tokens":1}`, `{"input_tokens":-1,"output_tokens":0}`, `{"input_tokens":9007199254740991,"output_tokens":1}`, `{"input_tokens":1.5,"output_tokens":0}`, `{"input_tokens":1,"prompt_tokens":2,"output_tokens":0}`, `{"input_tokens":1,"output_tokens":0,"input_tokens_details":{"audio_tokens":-1}}`} {
		if _, err := ParseTaskUsage(json.RawMessage(raw)); err == nil {
			t.Fatalf("invalid token facts accepted %s", raw)
		}
	}
	usage, err := ParseTaskUsage(json.RawMessage(`{"input_tokens":0,"output_tokens":0}`))
	if err != nil || usage == nil || usage.PromptTokens != 0 {
		t.Fatal("explicit zero became missing")
	}
}
