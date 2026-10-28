package middleware

import (
	"strings"
	"testing"
)

func TestManagedTextBodyAcceptsOnlyExplicitBasicTextShapes(t *testing.T) {
	for _, body := range []string{
		`{"model":"alpha","messages":[{"role":"user","content":"text containing {\"image_url\":\"literal\"}"}],"max_tokens":10}`,
		`{"model":"alpha","messages":[{"role":"developer","content":"system"},{"role":"user","content":[{"type":"text","text":"plain"}]}],"stream":true,"stream_options":{"include_usage":true}}`,
	} {
		if !managedTextBody("/v1/chat/completions", []byte(body)) {
			t.Fatalf("plain text rejected: %s", body)
		}
	}
	if !managedTextBody("/v1/messages", []byte(`{"model":"alpha","system":[{"type":"text","text":"system"}],"messages":[{"role":"user","content":[{"type":"text","text":"plain"}]}],"max_tokens":10}`)) {
		t.Fatal("Anthropic text rejected")
	}
	for _, body := range []string{
		`{"model":"alpha","messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"synthetic"}}]}]}`,
		`{"model":"alpha","messages":[{"role":"user","content":[{"type":"input_audio","input_audio":{"data":"synthetic"}}]}]}`,
		`{"model":"alpha","messages":[{"role":"user","content":[{"type":"text","text":"plain","image_url":"hidden"}]}]}`,
		`{"model":"alpha","messages":[{"role":"user","content":"plain","audio":{"data":"hidden"}}]}`,
		`{"model":"alpha","messages":[{"role":"user","content":"plain"}],"modalities":["audio"]}`,
		`{"model":"alpha","messages":[{"role":"user","content":"plain"}],"tools":[]}`,
		`{"model":"alpha","messages":[{"role":"user","content":"plain"}],"extra_body":{"audio":"hidden"}}`,
		`{"model":"alpha","messages":[{"role":"user","content":"plain"}],"Messages":[]}`,
		`{"model":"alpha","messages":[],"messages":[{"role":"user","content":"plain"}]}`,
		`{"model":"alpha","messages":[{"role":"user","content":"plain","content":"duplicate"}]}`,
		`{"model":"alpha","messages":[{"role":"user","content":null}]}`,
		`{"model":"alpha","messages":[{"role":"user","content":[{"type":"text","text":null}]}]}`,
		`{"model":"alpha","messages":[{"role":"tool","content":"result"}]}`,
		`{"model":"alpha","messages":[{"role":"user","content":"plain"}]} {}`,
		`null`,
	} {
		if managedTextBody("/v1/chat/completions", []byte(body)) {
			t.Fatalf("non-basic or ambiguous input accepted: %s", body)
		}
	}
	deep := `{"model":"alpha","messages":[{"role":"user","content":"plain"}],"metadata":` + strings.Repeat("[", 20) + "0" + strings.Repeat("]", 20) + "}"
	if managedTextBody("/v1/chat/completions", []byte(deep)) {
		t.Fatal("deep JSON accepted")
	}
	if managedTextBody("/v1/responses", []byte(`{"input":"plain"}`)) {
		t.Fatal("unsupported endpoint accepted")
	}
}
