package taskplugin

import (
	"encoding/json"
	"testing"
)

func TestTaskTokenUsageJSONDoesNotInventMissingFacts(t *testing.T) {
	for _, raw := range []string{`{"tokens":{}}`, `{"tokens":{"prompt_tokens":1}}`, `{"tokens":{"prompt_tokens":-1,"completion_tokens":0}}`} {
		var usage Usage
		if json.Unmarshal([]byte(raw), &usage) == nil {
			t.Fatalf("invalid plugin token facts accepted: %s", raw)
		}
	}
	var usage Usage
	if err := json.Unmarshal([]byte(`{"tokens":{"prompt_tokens":0,"completion_tokens":0}}`), &usage); err != nil || usage.Validate() != nil || usage.Tokens == nil {
		t.Fatal("explicit zero token measurement was discarded")
	}
}
