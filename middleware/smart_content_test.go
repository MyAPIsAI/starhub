package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"starhub/common"
	"starhub/model"
	"starhub/relay/dto"
)

func TestSmartRoutingNestedContentCapabilities(t *testing.T) {
	smartFixture(t)
	if err := model.UpsertModelMeta(&model.ModelMeta{Model: "cheap", Tags: "text,function_call", ContextLength: 4096, MaxOutput: 2048}); err != nil {
		t.Fatal(err)
	}
	for _, alias := range []string{"myapis-auto", "veyra-auto"} {
		for index, tc := range []struct{ name, content, want string }{
			{"image result", `[{"type":"tool_result","tool_use_id":"call_1","content":[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"AA=="}}]}]`, "capable"},
			{"image URL result", `[{"type":"tool_result","tool_use_id":"call_1","content":[{"type":"image_url","image_url":{"url":"https://example.invalid/review.png"}}]}]`, "capable"},
			{"nested result", `[{"type":"tool_result","tool_use_id":"call_1","content":[{"type":"tool_result","tool_use_id":"call_2","content":[{"type":"image","source":{"type":"base64","data":"AA=="}}]}]}]`, "capable"},
			{"top-level image", `[{"type":"image","source":{"type":"base64","data":"AA=="}}]`, "capable"},
			{"text result", `[{"type":"tool_result","tool_use_id":"call_1","content":"an image_url is ordinary text"}]`, "cheap"},
			{"JSON text result", `[{"type":"tool_result","tool_use_id":"call_1","content":"{\"type\":\"image\"}"}]`, "cheap"},
			{"text block result", `[{"type":"tool_result","tool_use_id":"call_1","content":[{"type":"text","text":"{\"type\":\"image\"}"}]}]`, "cheap"},
			{"tool arguments", `[{"type":"tool_use","id":"call_1","name":"lookup","input":{"type":"image","content":[{"type":"image"}]}}]`, "cheap"},
		} {
			t.Run(alias+"/"+tc.name, func(t *testing.T) {
				body := fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"review"},{"role":"user","content":%s}],"tools":[{"name":"lookup","input_schema":{"type":"object"}}],"max_tokens":8}`, alias, tc.content)
				chain := fmt.Sprintf("content-%s-%d", alias, index)
				if _, err := model.PinRoutingChain(1, "default", chain, "cheap"); err != nil {
					t.Fatal(err)
				}
				if tc.want == "capable" {
					if code, _ := routeSmart(t, body, "/v1/messages", "X-MyAPIs-Chain-Id", chain, 1, ""); code != 503 {
						t.Fatalf("vision requirement ignored for pinned text model: %d", code)
					}
					if _, err := model.PinRoutingChain(2, "default", chain, "capable"); err != nil {
						t.Fatal(err)
					}
				}
				tokenID := int64(1)
				if tc.want == "capable" {
					tokenID = 2
				}
				code, actual := routeSmart(t, body, "/v1/messages", "X-MyAPIs-Chain-Id", chain, tokenID, "cheap,capable")
				if code != 204 || actual != tc.want {
					t.Fatalf("status=%d model=%q want=%q", code, actual, tc.want)
				}
			})
		}
	}
}

func TestNestedImagesDoNotExpandSmartAuthorization(t *testing.T) {
	smartFixture(t)
	if err := model.UpsertModelMeta(&model.ModelMeta{Model: "cheap", Tags: "text,function_call", ContextLength: 4096, MaxOutput: 2048}); err != nil {
		t.Fatal(err)
	}
	for _, alias := range []string{"myapis-auto", "veyra-auto"} {
		body := `{"model":"` + alias + `","messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"call_1","content":[{"type":"image","source":{"type":"base64","data":"AA=="}}]}]}],"max_tokens":8}`
		if _, err := model.PinRoutingChain(1, "default", "limited-image-"+alias, "capable"); err != nil {
			t.Fatal(err)
		}
		if code, actual := routeSmart(t, body, "/v1/messages", "X-MyAPIs-Chain-Id", "limited-image-"+alias, 1, "cheap"); code != 403 || actual != "" {
			t.Fatalf("nested image escaped model whitelist: %d %s", code, actual)
		}
	}
}

func TestNestedImagesDoNotReplacePinnedSmartModel(t *testing.T) {
	smartFixture(t)
	if err := model.UpsertModelMeta(&model.ModelMeta{Model: "cheap", Tags: "text,function_call", ContextLength: 4096, MaxOutput: 2048}); err != nil {
		t.Fatal(err)
	}
	first := `{"model":"myapis-auto","messages":[{"role":"user","content":"review"}],"tools":[{"name":"lookup","input_schema":{"type":"object"}}],"max_tokens":8}`
	if code, actual := routeSmart(t, first, "/v1/messages", "X-MyAPIs-Chain-Id", "pinned-image", 1, ""); code != 204 || actual != "cheap" {
		t.Fatalf("unexpected initial model: %d %s", code, actual)
	}
	next := `{"model":"veyra-auto","messages":[{"role":"user","content":"review"},{"role":"user","content":[{"type":"tool_result","tool_use_id":"call_1","content":[{"type":"image","source":{"type":"base64","data":"AA=="}}]}]}],"max_tokens":8}`
	if code, actual := routeSmart(t, next, "/v1/messages", "X-MyAPIs-Chain-Id", "pinned-image", 1, ""); code != 503 || actual != "" {
		t.Fatalf("incompatible pin was reused or replaced: %d %s", code, actual)
	}
}

func TestExplicitModelWithNestedImageIsNotRewritten(t *testing.T) {
	smartFixture(t)
	body := `{"model":"cheap","messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"call_1","content":[{"type":"image","source":{"type":"base64","data":"AA=="}}]}]}],"max_tokens":8}`
	r := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(body))
	r = r.WithContext(common.WithValues(r.Context(), common.CtxUsingGroup, "default", common.CtxTokenModelLimits, "cheap"))
	called := false
	Distribute(dto.InboundClaude)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.Context().Value(common.CtxRequestModel) != "cheap" || string(r.Context().Value(common.CtxRequestBody).([]byte)) != body {
			t.Fatal("explicit request was rewritten")
		}
	})).ServeHTTP(httptest.NewRecorder(), r)
	if !called {
		t.Fatal("explicit request was blocked by smart-only scanning")
	}
}
