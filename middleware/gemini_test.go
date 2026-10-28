package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"starhub/common"
	"starhub/model"
	"starhub/relay/channel"
	"starhub/relay/dto"
	"starhub/setting"
)

func routeNativeGemini(t *testing.T, requested, body, limits string, tokenID int64) (int, string) {
	t.Helper()
	r := httptest.NewRequest("POST", "/v1beta/models/"+requested+":generateContent", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r = r.WithContext(common.WithValues(r.Context(), common.CtxUsingGroup, "default", common.CtxTokenID, tokenID, common.CtxTokenModelLimits, limits))
	selected := ""
	rec := httptest.NewRecorder()
	Distribute(dto.InboundGemini)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

func TestSmartGeminiCapabilities(t *testing.T) {
	for _, tc := range []struct {
		tags, extra string
		kind, want  int
	}{
		{"", "", channel.ChannelTypeGemini, 503},
		{"text", "", channel.ChannelTypeGemini, 503},
		{"gemini", "", channel.ChannelTypeGemini, 204},
		{"gemini", "", channel.ChannelTypeOpenAI, 503},
		{"gemini", `,"tools":[{"functionDeclarations":[{"name":"lookup"}]}]`, channel.ChannelTypeGemini, 503},
		{"gemini,function_call", `,"tools":[{"functionDeclarations":[{"name":"lookup"}]}]`, channel.ChannelTypeGemini, 204},
		{"gemini", `,"generationConfig":{"maxOutputTokens":64}`, channel.ChannelTypeGemini, 503},
		{"gemini", `,"generationConfig":{"maxOutputTokens":8,"thinkingConfig":{}}`, channel.ChannelTypeGemini, 503},
		{"gemini,reasoning", `,"generationConfig":{"maxOutputTokens":8,"thinkingConfig":{}}`, channel.ChannelTypeGemini, 204},
	} {
		t.Run(tc.tags+tc.extra, func(t *testing.T) {
			setupAutoRoutingDB(t)
			if err := model.InsertChannel(&model.Channel{Name: "native", Type: tc.kind, Models: "native", Group: "default", Status: common.ChannelStatusEnabled}); err != nil {
				t.Fatal(err)
			}
			if tc.tags != "" {
				if err := model.UpsertModelMeta(&model.ModelMeta{Model: "native", Tags: tc.tags, ContextLength: 128, MaxOutput: 16}); err != nil {
					t.Fatal(err)
				}
			}
			body := `{"contents":[{"parts":[{"text":"hello"}]}],"generationConfig":{"maxOutputTokens":8}` + tc.extra + `}`
			code, selected := routeNativeGemini(t, "myapis-auto", body, "", 1)
			if code != tc.want || (code == 204 && selected != "native") {
				t.Fatalf("capability result = %d %s, want %d", code, selected, tc.want)
			}
		})
	}
}

func TestGeminiHistoryPinsAcrossPathAliases(t *testing.T) {
	setupAutoRoutingDB(t)
	for _, name := range []string{"cheap", "capable"} {
		if err := model.InsertChannel(&model.Channel{Name: name, Type: channel.ChannelTypeGemini, Models: name, Group: "default", Status: common.ChannelStatusEnabled}); err != nil {
			t.Fatal(err)
		}
		if err := model.UpsertModelMeta(&model.ModelMeta{Model: name, Tags: "gemini,function_call", ContextLength: 4096, MaxOutput: 2048}); err != nil {
			t.Fatal(err)
		}
	}
	if err := setting.UpdateRatio(setting.OptionKeyModelRatio, `{"cheap":0.1,"capable":1}`); err != nil {
		t.Fatal(err)
	}
	first := `{"contents":[{"role":"user","parts":[{"text":"history"}]}],"generationConfig":{"maxOutputTokens":8}}`
	if code, chosen := routeNativeGemini(t, "myapis-auto", first, "", 1); code != 204 || chosen != "cheap" {
		t.Fatalf("first request = %d %s", code, chosen)
	}
	if err := setting.UpdateRatio(setting.OptionKeyModelRatio, `{"cheap":100,"capable":0.1}`); err != nil {
		t.Fatal(err)
	}
	next := `{"contents":[{"role":"user","parts":[{"text":"history"}]},{"role":"model","parts":[{"functionCall":{"name":"lookup","args":{}}}]},{"role":"user","parts":[{"functionResponse":{"name":"lookup","response":{"result":"done"}}}]}],"generationConfig":{"maxOutputTokens":8}}`
	if code, chosen := routeNativeGemini(t, "veyra-auto", next, "", 1); code != 204 || chosen != "cheap" {
		t.Fatalf("history switched model = %d %s", code, chosen)
	}
	if code, _ := routeNativeGemini(t, "myapis-auto", next, "", 2); code != 400 {
		t.Fatalf("other token reused pin: %d", code)
	}
	if code, _ := routeNativeGemini(t, "myapis-auto", next, "capable", 1); code != 403 {
		t.Fatalf("revoked model reused pin: %d", code)
	}
}
