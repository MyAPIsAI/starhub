package claude

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"starhub/relay/dto"
)

func cacheUsageStream() string {
	return "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"cached\",\"model\":\"m\",\"usage\":{\"input_tokens\":10,\"cache_read_input_tokens\":100,\"cache_creation_input_tokens\":50,\"output_tokens\":0}}}\n\nevent: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"local\"}}\n\nevent: message_delta\ndata: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":3}}\n\nevent: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":20}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
}

func TestCachedNativePassthroughPreservesBytesAndFullUsage(t *testing.T) {
	for _, stream := range []bool{false, true} {
		body := `{"id":"cached","model":"m","content":[{"type":"text","text":"local"}],"usage":{"input_tokens":10,"cache_read_input_tokens":100,"cache_creation_input_tokens":50,"output_tokens":20}}`
		if stream {
			body = cacheUsageStream()
		}
		response := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}
		recorder := httptest.NewRecorder()
		usage, text, err := New().HandlePassthroughResponse(recorder, response, stream)
		if err != nil || text != "local" || usage == nil || usage.PromptTokens != 160 || usage.CompletionTokens != 20 || usage.CachedTokens() != 100 || usage.CacheWriteTokens() != 50 {
			t.Fatalf("stream%t lost native usage: %+v %s %v", stream, usage, text, err)
		}
		if recorder.Body.String() != body {
			t.Fatal("native body rewritten")
		}
	}
}

func TestConvertedClaudeStreamKeepsCacheDetailsAcrossPartialOutputUpdates(t *testing.T) {
	response := &http.Response{Body: io.NopCloser(strings.NewReader(cacheUsageStream()))}
	reader, err := New().StreamChatEvents(response)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	var outputCounts []int64
	var snapshots []*dto.Usage
	text := ""
	for {
		event, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		text += event.TextDelta
		if event.Usage != nil {
			if event.Usage.PromptTokens != 160 || event.Usage.CachedTokens() != 100 || event.Usage.CacheWriteTokens() != 50 {
				t.Fatal("converted stream dropped input details")
			}
			outputCounts = append(outputCounts, event.Usage.CompletionTokens)
			snapshots = append(snapshots, event.Usage)
		}
	}
	if text != "local" || len(outputCounts) < 2 || outputCounts[0] != 3 || outputCounts[len(outputCounts)-1] != 20 {
		t.Fatalf("cumulative output counts: %v %s", outputCounts, text)
	}
	if snapshots[0].CompletionTokens != 3 || snapshots[len(snapshots)-1].CompletionTokens != 20 {
		t.Fatal("later stream report mutated prior snapshot")
	}
}

func TestClaudeStreamRetainsKnownInputAtEarlyEOF(t *testing.T) {
	body := strings.Split(cacheUsageStream(), "event: message_delta")[0]
	response := &http.Response{Body: io.NopCloser(strings.NewReader(body))}
	reader, err := New().StreamChatEvents(response)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	known := false
	for {
		event, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if event.Usage != nil {
			known = true
			if event.Usage.PromptTokens != 160 || event.Usage.CacheWriteTokens() != 50 {
				t.Fatal("early EOF lost known usage")
			}
		}
	}
	if !known {
		t.Fatal("known input omitted at EOF")
	}
}
