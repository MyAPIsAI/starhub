package middleware

import (
	"fmt"
	"net/http/httptest"
	"starhub/model"
	"starhub/relay/dto"
	"strings"
	"testing"
)

func TestSmartContextIncludesToolPayloads(t *testing.T) {
	smartFixture(t)
	if err := model.UpsertModelMeta(&model.ModelMeta{Model: "cheap", Tags: "text,function_call", ContextLength: 64, MaxOutput: 16}); err != nil {
		t.Fatal(err)
	}
	large := strings.Repeat("a", 1000)
	for i, message := range []string{
		fmt.Sprintf(`{"role":"assistant","content":null,"tool_calls":[{"id":"call_1","type":"function","function":{"name":"lookup","arguments":%q}}]}`, large),
		fmt.Sprintf(`{"role":"assistant","content":null,"function_call":{"name":"lookup","arguments":%q}}`, large),
		fmt.Sprintf(`{"role":"assistant","content":[{"type":"tool_use","id":"call_1","name":"lookup","input":{"value":%q}}]}`, large),
		fmt.Sprintf(`{"role":"user","content":[{"type":"tool_result","tool_use_id":"call_1","content":%q}]}`, large),
		fmt.Sprintf(`{"role":"user","content":[{"type":"tool_result","tool_use_id":"call_1","content":[{"type":"text","text":%q}]}]}`, large),
	} {
		body := `{"model":"myapis-auto","messages":[{"role":"user","content":"review"},` + message + `],"max_tokens":8}`
		r := httptest.NewRequest("POST", "/v1/chat/completions", nil)
		r.Header.Set("X-MyAPIs-Chain-Id", fmt.Sprintf("context-%d", i))
		req, err := readSmartRequirements(r, []byte(body), false)
		if err != nil || req.input <= 64 {
			t.Fatalf("tool payload missing from estimate: input=%d err=%v", req.input, err)
		}
		if _, err := model.PinRoutingChain(1, "default", fmt.Sprintf("context-%d", i), "cheap"); err != nil {
			t.Fatal(err)
		}
		if code, _ := routeSmart(t, body, "/v1/chat/completions", "X-MyAPIs-Chain-Id", fmt.Sprintf("context-%d", i), 1, ""); code != 503 {
			t.Fatalf("oversized continuation accepted by small pinned model: %d", code)
		}
		if _, err := model.PinRoutingChain(2, "default", fmt.Sprintf("context-%d", i), "capable"); err != nil {
			t.Fatal(err)
		}
		code, actual := routeSmart(t, body, "/v1/chat/completions", "X-MyAPIs-Chain-Id", fmt.Sprintf("context-%d", i), 2, "")
		if code != 204 || actual != "capable" {
			t.Fatalf("tool payload not counted: %d %s", code, actual)
		}
	}
	body := fmt.Sprintf(`{"model":"veyra-auto","messages":[{"role":"assistant","tool_calls":[{"id":"call_1","function":{"name":"lookup","arguments":%q}}]}],"max_tokens":8}`, strings.Repeat("a", 100000))
	if _, err := model.PinRoutingChain(1, "default", "oversized-tool", "capable"); err != nil {
		t.Fatal(err)
	}
	if code, actual := routeSmart(t, body, "/v1/chat/completions", "X-MyAPIs-Chain-Id", "oversized-tool", 1, ""); code != 503 || actual != "" {
		t.Fatalf("oversized tool input admitted: %d %s", code, actual)
	}
}

func TestSmartToolTokenEstimateDoesNotDoubleCountTextOrImageBytes(t *testing.T) {
	body := []byte(`{"model":"myapis-auto","messages":[{"role":"user","content":"hello"}],"max_tokens":8}`)
	req, err := readSmartRequirements(httptest.NewRequest("POST", "/v1/chat/completions", nil), body, false)
	if err != nil || req.input != dto.EstimateTokens("hello") {
		t.Fatalf("ordinary text estimate changed: %+v %v", req, err)
	}
	content := fmt.Sprintf(`[{"type":"tool_result","tool_use_id":"x","content":[{"type":"text","text":"hello"},{"type":"image","source":{"type":"base64","data":%q}}]}]`, strings.Repeat("A", 100000))
	if got := smartToolBlockTokens([]byte(content)); got != dto.EstimateTokens("x")+dto.EstimateTokens("hello") {
		t.Fatalf("image bytes treated as text or text duplicated: %d", got)
	}
}
