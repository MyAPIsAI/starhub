package middleware

import (
	"encoding/json"
	"strings"
	"testing"

	"starhub/common"
	"starhub/model"
	"starhub/relay/channel"
	"starhub/setting"
)

func TestSmartResponsesCapabilities(t *testing.T) {
	for _, alias := range []string{"myapis-auto", "veyra-auto"} {
		for _, tc := range []struct {
			name, tags, extra string
			kind, want        int
		}{
			{"unknown", "", "", channel.ChannelTypeOpenAI, 503},
			{"text only", "text", "", channel.ChannelTypeOpenAI, 503},
			{"native", "responses", "", channel.ChannelTypeOpenAI, 204},
			{"wrong adapter", "responses", "", channel.ChannelTypeClaude, 503},
			{"tools unknown", "responses", `,"tools":[{"type":"function","name":"lookup"}]`, channel.ChannelTypeOpenAI, 503},
			{"tools", "responses,function_call", `,"tools":[{"type":"function","name":"lookup"}]`, channel.ChannelTypeOpenAI, 204},
			{"vision unknown", "responses", `,"input":[{"role":"user","content":[{"type":"input_image","image_url":"data:image/png;base64,AAAA"}]}]`, channel.ChannelTypeOpenAI, 503},
			{"vision", "responses,vision", `,"input":[{"role":"user","content":[{"type":"input_image","image_url":"data:image/png;base64,AAAA"}]}]`, channel.ChannelTypeOpenAI, 204},
			{"reasoning unknown", "responses", `,"reasoning":{"effort":"high"}`, channel.ChannelTypeOpenAI, 503},
			{"reasoning", "responses,reasoning", `,"reasoning":{"effort":"high"}`, channel.ChannelTypeOpenAI, 204},
			{"context bound", "responses", `,"input":"` + strings.Repeat("a", 4096) + `"`, channel.ChannelTypeOpenAI, 503},
			{"output bound", "responses", `,"max_output_tokens":100`, channel.ChannelTypeOpenAI, 503},
		} {
			t.Run(alias+"/"+tc.name, func(t *testing.T) {
				setupAutoRoutingDB(t)
				if err := model.InsertChannel(&model.Channel{Name: "responses", Type: tc.kind, Models: "native", Group: "default", Status: common.ChannelStatusEnabled}); err != nil {
					t.Fatal(err)
				}
				if tc.tags != "" {
					if err := model.UpsertModelMeta(&model.ModelMeta{Model: "native", Tags: tc.tags, ContextLength: 128, MaxOutput: 32}); err != nil {
						t.Fatal(err)
					}
				}
				fields := map[string]any{"model": alias, "input": "hello", "max_output_tokens": 8}
				if tc.extra != "" {
					if err := json.Unmarshal([]byte("{"+strings.TrimPrefix(tc.extra, ",")+"}"), &fields); err != nil {
						t.Fatal(err)
					}
				}
				body, err := json.Marshal(fields)
				if err != nil {
					t.Fatal(err)
				}
				code, selected := routeSmart(t, string(body), "/v1/responses", "", "", 1, "")
				if code != tc.want || (code == 204 && selected != "native") || (code != 204 && selected != "") {
					t.Fatalf("status=%d model=%q, want %d", code, selected, tc.want)
				}
			})
		}
	}
}

func TestResponsesHistoryKeepsOriginalModel(t *testing.T) {
	smartFixture(t)
	for _, name := range []string{"cheap", "capable"} {
		if err := model.UpsertModelMeta(&model.ModelMeta{Model: name, Tags: "responses,function_call,reasoning", ContextLength: 4096, MaxOutput: 2048}); err != nil {
			t.Fatal(err)
		}
	}
	first := `{"model":"myapis-auto","input":[{"role":"user","content":"history"}],"max_output_tokens":8}`
	if code, selected := routeSmart(t, first, "/v1/responses", "", "", 1, "cheap,capable"); code != 204 || selected != "cheap" {
		t.Fatalf("first turn = %d %s", code, selected)
	}
	if err := setting.UpdateRatio(setting.OptionKeyModelRatio, `{"cheap":100,"capable":0.1}`); err != nil {
		t.Fatal(err)
	}
	next := `{"model":"veyra-auto","input":[{"role":"user","content":"history"},{"type":"reasoning","encrypted_content":"opaque","summary":[]},{"type":"function_call","name":"lookup","call_id":"call_1","arguments":"{}"},{"type":"function_call_output","call_id":"call_1","output":"done"}],"max_output_tokens":8}`
	if code, selected := routeSmart(t, next, "/v1/responses", "", "", 1, "cheap,capable"); code != 204 || selected != "cheap" {
		t.Fatalf("continuation changed model = %d %s", code, selected)
	}
	if code, _ := routeSmart(t, next, "/v1/responses", "", "", 2, "cheap,capable"); code != 400 {
		t.Fatalf("another token reused chain: %d", code)
	}
	if code, _ := routeSmart(t, next, "/v1/responses", "", "", 1, "capable"); code != 403 {
		t.Fatalf("revoked model reused: %d", code)
	}
	trimmed := `{"model":"myapis-auto","input":[{"type":"function_call_output","call_id":"call_1","output":"done"}]}`
	if code, _ := routeSmart(t, trimmed, "/v1/responses", "", "", 1, ""); code != 400 {
		t.Fatalf("trimmed history silently repinned: %d", code)
	}
	if err := model.DB.Model(&model.RoutingChain{}).Where("token_id = ?", 1).Update("expires_at", 1).Error; err != nil {
		t.Fatal(err)
	}
	if code, _ := routeSmart(t, next, "/v1/responses", "", "", 1, "cheap,capable"); code != 400 {
		t.Fatalf("expired chain silently repinned: %d", code)
	}
}
