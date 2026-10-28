package openai

import (
	"testing"

	"starhub/relay/channel"
)

func TestResponsesURL(t *testing.T) {
	for _, base := range []string{"https://example.com", "https://example.com/", "https://example.com/v1", " https://example.com/v1/ "} {
		got, err := New().ModeURL(base, channel.RelayModeResponses)
		if err != nil || got != "https://example.com/v1/responses" {
			t.Fatalf("ModeURL(%q) = %q, %v", base, got, err)
		}
	}
}
