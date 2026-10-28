package dto

import (
	"encoding/json"
	"testing"
)

func TestGeminiUsageCountsDistinctCategories(t *testing.T) {
	body := []byte(`{"usageMetadata":{"promptTokenCount":20,"candidatesTokenCount":7,"toolUsePromptTokenCount":3,"thoughtsTokenCount":5,"cachedContentTokenCount":8,"totalTokenCount":35}}`)
	u, err := ParseGeminiUsage(body)
	if err != nil || u == nil || u.PromptTokens != 23 || u.CompletionTokens != 12 || u.TotalTokens() != 35 || u.CachedTokens() != 8 || u.CompletionTokensDetails.ReasoningTokens != 5 {
		t.Fatalf("usage = %+v, %v", u, err)
	}
	chat, err := ParseGeminiChatResponse(body)
	if err != nil || chat.Usage == nil || chat.Usage.TotalTokens() != 35 || chat.Usage.CachedTokens() != 8 {
		t.Fatalf("chat dropped normalized usage: %+v %v", chat, err)
	}
	stream, err := ParseGeminiStreamPayload(body)
	if err != nil || stream.Usage == nil || stream.Usage.TotalTokens() != 35 {
		t.Fatalf("stream dropped normalized usage: %+v %v", stream, err)
	}
}

func TestGeminiUsageMissingAndZero(t *testing.T) {
	for _, tc := range []struct {
		body string
		nil  bool
	}{
		{`{}`, true}, {`{"usageMetadata":null}`, true}, {`{"usageMetadata":{}}`, true},
		{`{"usageMetadata":{"totalTokenCount":12}}`, true},
		{`{"usageMetadata":{"promptTokenCount":0,"candidatesTokenCount":0}}`, false},
	} {
		u, err := ParseGeminiUsage([]byte(tc.body))
		if err != nil || (u == nil) != tc.nil || (u != nil && u.TotalTokens() != 0) {
			t.Fatalf("ParseGeminiUsage(%s) = %+v %v", tc.body, u, err)
		}
		ev, err := ParseGeminiStreamPayload([]byte(tc.body))
		if err != nil || (ev.Usage == nil) != tc.nil {
			t.Fatalf("stream zero/missing lost: %+v %v", ev, err)
		}
	}
}

func TestGeminiUsageRejectsInvalidCounts(t *testing.T) {
	for _, body := range []string{
		`null`, `[]`, `{"usageMetadata":[]}`, `{"usageMetadata":{"promptTokenCount":1.5}}`,
		`{"usageMetadata":{"promptTokenCount":-1}}`, `{"usageMetadata":{"candidatesTokenCount":-1}}`,
		`{"usageMetadata":{"toolUsePromptTokenCount":-1}}`, `{"usageMetadata":{"thoughtsTokenCount":-1}}`,
		`{"usageMetadata":{"cachedContentTokenCount":-1}}`, `{"usageMetadata":{"totalTokenCount":-1}}`,
		`{"usageMetadata":{"promptTokenCount":9223372036854775807,"toolUsePromptTokenCount":1}}`,
		`{"usageMetadata":{"candidatesTokenCount":9223372036854775807,"thoughtsTokenCount":1}}`,
		`{"usageMetadata":{"promptTokenCount":9223372036854775807,"candidatesTokenCount":1}}`,
	} {
		if u, err := ParseGeminiUsage([]byte(body)); err == nil {
			t.Fatalf("invalid report accepted: %s => %+v", body, u)
		}
	}
}

func TestGeminiUsageClampsCachedSubsetAndTrustsNoGatewayMetadata(t *testing.T) {
	u, err := ParseGeminiUsage([]byte(`{"usageMetadata":{"promptTokenCount":2,"toolUsePromptTokenCount":8,"cachedContentTokenCount":99,"candidatesTokenCount":1,"RequestProtocol":"untrusted","RequestStatus":3}}`))
	if err != nil || u.CachedTokens() != 2 || u.PromptTokens != 10 || u.RequestProtocol != "" || u.RequestStatus != 0 {
		t.Fatalf("invalid cache subset or gateway attribution: %+v %v", u, err)
	}
	if err := json.Unmarshal([]byte(`{"RequestStatus":3,"RequestProtocol":"untrusted"}`), u); err != nil || u.RequestStatus != 0 || u.RequestProtocol != "" {
		t.Fatalf("JSON set trusted metadata: %+v %v", u, err)
	}
}
