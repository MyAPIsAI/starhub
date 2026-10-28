package middleware

import (
	"net/http"
	"net/http/httptest"
	"starhub/common"
	"starhub/model"
	"starhub/relay/channel"
	"starhub/relay/dto"
	"starhub/setting"
	"strings"
	"testing"
)

func smartFixture(t *testing.T) {
	setupAutoRoutingDB(t)
	for _, ch := range []model.Channel{
		{Name: "chat", Type: channel.ChannelTypeOpenAI, Models: "cheap,capable,video-only", Group: "default", Status: common.ChannelStatusEnabled},
		{Name: "unsupported audio", Type: channel.ChannelTypeClaude, Models: "audio-cheap", Group: "default", Status: common.ChannelStatusEnabled},
		{Name: "audio", Type: channel.ChannelTypeOpenAI, Models: "audio-capable", Group: "default", Status: common.ChannelStatusEnabled},
	} {
		if err := model.InsertChannel(&ch); err != nil {
			t.Fatal(err)
		}
	}
	for _, meta := range []model.ModelMeta{
		{Model: "cheap", Tags: "text", ContextLength: 64, MaxOutput: 16},
		{Model: "capable", Tags: "text,vision,code,reasoning,function_call", ContextLength: 4096, MaxOutput: 2048},
		{Model: "video-only", Tags: "video"}, {Model: "audio-cheap", Tags: "tts"}, {Model: "audio-capable", Tags: "tts"},
	} {
		if err := model.UpsertModelMeta(&meta); err != nil {
			t.Fatal(err)
		}
	}
	if err := setting.UpdateRatio(setting.OptionKeyModelRatio, `{"cheap":0.1,"capable":1,"video-only":0.01,"audio-cheap":0.01,"audio-capable":1}`); err != nil {
		t.Fatal(err)
	}
}
func routeSmart(t *testing.T, body, path, header, value string, tokenID int64, limits string) (int, string) {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if header != "" {
		r.Header.Set(header, value)
	}
	r = r.WithContext(common.WithValues(r.Context(), common.CtxUsingGroup, "default", common.CtxTokenID, tokenID, common.CtxTokenModelLimits, limits))
	selected := ""
	rec := httptest.NewRecorder()
	Distribute(dto.InboundOpenAI)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if admit, ok := r.Context().Value(common.CtxSmartChainAdmission).(func() error); ok {
			if err := admit(); err != nil {
				t.Fatal(err)
			}
		}
		selected, _ = r.Context().Value(common.CtxRequestModel).(string)
		w.WriteHeader(204)
	})).ServeHTTP(rec, r)
	return rec.Code, selected
}
func TestSmartRoutingRequirements(t *testing.T) {
	smartFixture(t)
	for _, tc := range []struct {
		name, body, path, header, value, want string
		code                                  int
	}{
		{"text", `{"model":"myapis-auto","messages":[{"role":"user","content":"hello"}],"max_tokens":8}`, "/v1/chat/completions", "", "", "cheap", 204},
		{"vision", `{"model":"myapis-auto","messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"review"}}]}],"max_tokens":8}`, "/v1/chat/completions", "", "", "capable", 204},
		{"code", `{"model":"veyra-auto","max_tokens":8}`, "/v1/messages", "X-MyAPIs-Capability", "code", "capable", 204},
		{"legacy capability", `{"model":"veyra-auto","max_tokens":8}`, "/v1/chat/completions", "X-Veyra-Capability", "reasoning", "capable", 204},
		{"tools", `{"model":"myapis-auto","tools":[{"type":"function"}],"max_tokens":8}`, "/v1/chat/completions", "X-MyAPIs-Chain-Id", "initial-tools", "capable", 204},
		{"unscoped initial tools", `{"model":"myapis-auto","tools":[{"type":"function"}],"max_tokens":8}`, "/v1/chat/completions", "", "", "", 400},
		{"unscoped legacy function", `{"model":"myapis-auto","messages":[{"role":"function","content":"result"}]}`, "/v1/chat/completions", "", "", "", 400},
		{"context and output", `{"model":"myapis-auto","max_tokens":128}`, "/v1/chat/completions", "", "", "capable", 204},
		{"no context candidate", `{"model":"myapis-auto","max_tokens":8000}`, "/v1/chat/completions", "", "", "", 503},
		{"unknown capability", `{"model":"myapis-auto"}`, "/v1/chat/completions", "X-MyAPIs-Capability", "unknown", "", 400},
		{"invalid chain", `{"model":"myapis-auto"}`, "/v1/chat/completions", "X-MyAPIs-Chain-Id", "../bad", "", 400},
		{"unscoped continuation", `{"model":"myapis-auto","messages":[{"role":"tool","content":"result"}]}`, "/v1/chat/completions", "", "", "", 400},
		{"text is not a capability", `{"model":"myapis-auto","messages":[{"role":"user","content":"image_url tool_calls"}],"max_tokens":8}`, "/v1/chat/completions", "", "", "cheap", 204},
		{"audio mode", `{"model":"myapis-auto","input":"review"}`, "/v1/audio/speech", "", "", "audio-capable", 204},
		{"explicit model unchanged", `{"model":"cheap","max_tokens":128}`, "/v1/chat/completions", "X-MyAPIs-Capability", "vision", "cheap", 204},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, model := routeSmart(t, tc.body, tc.path, tc.header, tc.value, 1, "")
			if code != tc.code || model != tc.want {
				t.Fatalf("status=%d model=%q want=%d/%q", code, model, tc.code, tc.want)
			}
		})
	}
}

func TestSmartRoutingCarriesRetryConstraints(t *testing.T) {
	smartFixture(t)
	r := httptest.NewRequest(http.MethodPost, "/v1/audio/speech", strings.NewReader(`{"model":"myapis-auto","input":"review"}`))
	r.Header.Set("Content-Type", "application/json")
	r = r.WithContext(common.WithValues(r.Context(), common.CtxUsingGroup, "default"))
	called := false
	Distribute(dto.InboundOpenAI)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		check, ok := r.Context().Value(common.CtxSmartChannelCheck).(func(*model.Channel, string) (bool, error))
		if !ok {
			t.Fatal("smart constraints were not passed to relay retries")
		}
		for _, tc := range []struct {
			kind int
			want bool
		}{{channel.ChannelTypeClaude, false}, {channel.ChannelTypeOpenAI, true}} {
			allowed, err := check(&model.Channel{Type: tc.kind}, "default")
			if err != nil || allowed != tc.want {
				t.Fatalf("channel type %d: allowed=%v error=%v", tc.kind, allowed, err)
			}
		}
	})).ServeHTTP(httptest.NewRecorder(), r)
	if !called {
		t.Fatal("smart request did not reach relay")
	}
}
func TestSmartToolChainRemainsAuthorizedAndPinned(t *testing.T) {
	smartFixture(t)
	body := `{"model":"myapis-auto","max_tokens":8}`
	code, actual := routeSmart(t, body, "/v1/chat/completions", "X-MyAPIs-Chain-Id", "review-chain", 1, "")
	if code != 204 || actual != "cheap" {
		t.Fatalf("initial chain: %d %s", code, actual)
	}
	if err := setting.UpdateRatio(setting.OptionKeyModelRatio, `{"cheap":10,"capable":0.1}`); err != nil {
		t.Fatal(err)
	}
	code, actual = routeSmart(t, body, "/v1/chat/completions", "X-Veyra-Chain-Id", "review-chain", 1, "")
	if code != 204 || actual != "cheap" {
		t.Fatalf("price change broke pin: %d %s", code, actual)
	}
	code, actual = routeSmart(t, body, "/v1/chat/completions", "X-MyAPIs-Chain-Id", "review-chain", 2, "")
	if code != 204 || actual != "capable" {
		t.Fatalf("token chains leaked: %d %s", code, actual)
	}
	code, actual = routeSmart(t, body, "/v1/chat/completions", "X-MyAPIs-Chain-Id", "review-chain", 1, "capable")
	if code == 204 || actual != "" {
		t.Fatal("pinned model escaped updated allowlist")
	}
	body = `{"model":"myapis-auto","messages":[{"role":"user","content":[{"type":"image_url"}]}],"max_tokens":8}`
	code, actual = routeSmart(t, body, "/v1/chat/completions", "X-MyAPIs-Chain-Id", "review-chain", 1, "")
	if code == 204 || actual != "" {
		t.Fatal("incompatible continuation silently changed models")
	}
}

func TestSmartRoutingUsesOnlyAuthorizedAutoGroups(t *testing.T) {
	smartFixture(t)
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"myapis-auto","max_tokens":8}`))
	r = r.WithContext(common.WithValues(r.Context(), common.CtxUsingGroup, common.AutoGroup, common.CtxAutoGroups, []string{"missing", "default"}, common.CtxTokenModelLimits, "capable"))
	rec := httptest.NewRecorder()
	selected := ""
	Distribute(dto.InboundOpenAI)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		selected, _ = r.Context().Value(common.CtxRequestModel).(string)
		w.WriteHeader(204)
	})).ServeHTTP(rec, r)
	if rec.Code != 204 || selected != "capable" {
		t.Fatalf("auto-group whitelist lost: %d %s", rec.Code, selected)
	}
}

func TestSmartRoutingMetadataFailureStopsDispatch(t *testing.T) {
	smartFixture(t)
	if err := model.DB.Migrator().DropTable(&model.ModelMeta{}); err != nil {
		t.Fatal(err)
	}
	code, selected := routeSmart(t, `{"model":"myapis-auto","max_tokens":8}`, "/v1/chat/completions", "", "", 1, "")
	if code != 500 || selected != "" {
		t.Fatalf("metadata failure bypassed routing: %d %s", code, selected)
	}
}
