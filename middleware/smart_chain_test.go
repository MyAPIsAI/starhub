package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"starhub/common"
	"starhub/model"
	"starhub/relay/dto"
	"starhub/setting"
)

func TestHeaderlessSmartToolChainKeepsItsModel(t *testing.T) {
	for _, protocol := range []string{"openai", "anthropic", "legacy functions"} {
		t.Run(protocol, func(t *testing.T) {
			smartFixture(t)
			if err := model.UpsertModelMeta(&model.ModelMeta{Model: "cheap", Tags: "text,function_call", ContextLength: 4096, MaxOutput: 2048}); err != nil {
				t.Fatal(err)
			}
			path := "/v1/chat/completions"
			first := `{"model":"myapis-auto","messages":[{"role":"user","content":"review conversation"}],"tools":[{"type":"function","function":{"name":"lookup","parameters":{"type":"object"}}}],"max_tokens":8}`
			next := `{"model":"veyra-auto","messages":[{"role":"user","content":"review conversation"},{"role":"assistant","content":null,"tool_calls":[{"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{}"}}]},{"role":"tool","tool_call_id":"call_1","content":"result"}],"max_tokens":8}`
			if protocol == "anthropic" {
				path = "/v1/messages"
				first = `{"model":"myapis-auto","messages":[{"role":"user","content":[{"text":"review conversation","type":"text"}]}],"tools":[{"name":"lookup","input_schema":{"type":"object"}}],"max_tokens":8}`
				next = `{"model":"veyra-auto","messages":[{"role":"user","content":[ { "type":"text", "text":"review conversation" } ]},{"role":"assistant","content":[{"type":"tool_use","id":"call_1","name":"lookup","input":{}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"call_1","content":"result"}]}],"max_tokens":8}`
			}
			if protocol == "legacy functions" {
				first = `{"model":"myapis-auto","messages":[{"role":"user","content":"review conversation"}],"functions":[{"name":"lookup","parameters":{"type":"object"}}],"max_tokens":8}`
				next = `{"model":"veyra-auto","messages":[{"role":"user","content":"review conversation"},{"role":"assistant","content":null,"function_call":{"name":"lookup","arguments":"{}"}},{"role":"function","name":"lookup","content":"result"}],"max_tokens":8}`
			}
			if code, chosen := routeSmart(t, first, path, "", "", 1, "cheap,capable"); code != 204 || chosen != "cheap" {
				t.Fatalf("headerless tools rejected: %d %s", code, chosen)
			}
			if err := setting.UpdateRatio(setting.OptionKeyModelRatio, `{"cheap":100,"capable":0.1}`); err != nil {
				t.Fatal(err)
			}
			if code, chosen := routeSmart(t, next, path, "", "", 1, "cheap,capable"); code != 204 || chosen != "cheap" {
				t.Fatalf("tool continuation changed model: %d %s", code, chosen)
			}
			if code, chosen := routeSmart(t, next, path, "", "", 2, "cheap,capable"); code != 400 || chosen != "" {
				t.Fatalf("another token reused the inferred pin: %d %s", code, chosen)
			}
			if code, chosen := routeSmart(t, next, path, "", "", 1, "capable"); code != 403 || chosen != "" {
				t.Fatalf("revoked model was replaced or authorized: %d %s", code, chosen)
			}
			var chains []model.RoutingChain
			if err := model.DB.Find(&chains).Error; err != nil {
				t.Fatal(err)
			}
			if len(chains) != 1 || chains[0].Model != "cheap" || len(chains[0].ChainHash) != 64 {
				t.Fatalf("unexpected pins: %+v", chains)
			}
		})
	}
}

func TestHeaderlessToolContinuationNeedsExistingHistory(t *testing.T) {
	smartFixture(t)
	for _, body := range []string{
		`{"model":"myapis-auto","messages":[{"role":"tool","content":"result"}],"max_tokens":8}`,
		`{"model":"myapis-auto","messages":[{"role":"user","content":"a new unrecognized prefix"},{"role":"assistant","tool_calls":[{"id":"call_1"}]},{"role":"tool","content":"result"}],"max_tokens":8}`,
		`{"model":"myapis-auto","messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"call_1","content":"result"}]}],"max_tokens":8}`,
	} {
		r := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
		r = r.WithContext(common.WithValues(r.Context(), common.CtxUsingGroup, "default", common.CtxTokenID, int64(1)))
		rec := httptest.NewRecorder()
		Distribute(dto.InboundOpenAI)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Fatal("unknown continuation was dispatched") })).ServeHTTP(rec, r)
		if rec.Code != 400 || !strings.Contains(rec.Body.String(), "X-MyAPIs-Chain-Id") {
			t.Fatalf("missing actionable history error: %d %s", rec.Code, rec.Body)
		}
	}
	var count int64
	if err := model.DB.Model(&model.RoutingChain{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("unknown continuations created model bindings")
	}
}
