package middleware

import (
	"testing"
)

func TestChatPrefixHashStable(t *testing.T) {
	mkBody := func(system, userContent string) []byte {
		return []byte(`{"model":"gpt-4o","system":"` + system + `","messages":[{"role":"user","content":"` + userContent + `"},{"role":"assistant","content":"hi"}]}`)
	}

	a := chatPrefixHashFromBody(mkBody("you are a helpful assistant", "hello"))
	b := chatPrefixHashFromBody(mkBody("you are a helpful assistant", "hello"))
	if a == "" {
		t.Fatal("hash should not be empty for a valid prompt")
	}
	if a != b {
		t.Fatalf("hash should be stable: %q != %q", a, b)
	}

	// A different first user message must change the hash.
	if chatPrefixHashFromBody(mkBody("you are a helpful assistant", "goodbye")) == a {
		t.Fatal("different prompts should hash differently")
	}

	// Later turns do not affect the hash (stable conversation key).
	later := []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"hello"},{"role":"user","content":"and then?"}]}`)
	first := []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}]}`)
	if chatPrefixHashFromBody(later) != chatPrefixHashFromBody(first) {
		t.Fatal("hash must be scoped to the leading prompt, not the last turn")
	}
}

func TestChatPrefixHashEdgeCases(t *testing.T) {
	// No system and no user message: nothing to hash.
	if got := chatPrefixHashFromBody([]byte(`{"model":"gpt-4o","messages":[{"role":"assistant","content":"hi"}]}`)); got != "" {
		t.Fatalf("empty prompt hash = %q, want \"\"", got)
	}
	// System only is still a usable prefix.
	if got := chatPrefixHashFromBody([]byte(`{"model":"gpt-4o","system":"sys"}`)); got == "" {
		t.Fatal("system-only prompt should produce a hash")
	}
	// Multimodal array content hashes deterministically.
	arrBody := []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":[{"type":"text","text":"describe"},{"type":"image_url","image_url":{"url":"x"}}]}]}`)
	if chatPrefixHashFromBody(arrBody) == "" {
		t.Fatal("array content should produce a hash")
	}
	if chatPrefixHashFromBody(arrBody) != chatPrefixHashFromBody(arrBody) {
		t.Fatal("array content hash should be stable")
	}
	// Invalid JSON returns empty.
	if got := chatPrefixHashFromBody([]byte(`not json`)); got != "" {
		t.Fatalf("invalid json hash = %q, want \"\"", got)
	}
}
