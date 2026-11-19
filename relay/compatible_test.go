package relay

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"starhub/relay/channel"
	"starhub/relay/dto"
)

func TestCompatibleProviderRegistrationAndRequests(t *testing.T) {
	for _, tc := range []struct {
		typeID             int
		name, root, prefix string
	}{
		{channel.ChannelTypeZhipu, "zhipu", "https://open.bigmodel.cn", "/api/paas/v4"},
		{channel.ChannelTypeMoonshot, "moonshot", "https://api.moonshot.cn", "/v1"},
		{channel.ChannelTypeDeepSeek, "deepseek", "https://api.deepseek.com", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, err := GetChatAdaptor(tc.typeID)
			if err != nil || a.Name() != tc.name || SupportedChannelTypes()[tc.typeID] != tc.name {
				t.Fatalf("provider registration: %v %v", a, err)
			}
			if got := a.ChatURL("", "", "fixture", false); got != tc.root+tc.prefix+"/chat/completions" {
				t.Fatal(got)
			}
			for _, base := range []string{"http://localhost:1234/proxy", "http://localhost:1234/proxy" + tc.prefix + "///"} {
				if got := a.ChatURL(base, "", "fixture", true); got != "http://localhost:1234/proxy"+tc.prefix+"/chat/completions" {
					t.Fatal(got)
				}
			}
			req := httptest.NewRequest("POST", "/", nil)
			if err := a.PrepareRequest(req, "fixture-key", nil); err != nil || req.Header.Get("Authorization") != "Bearer fixture-key" {
				t.Fatal("invalid authentication", err)
			}
			for _, stream := range []bool{false, true} {
				body, err := a.ConvertChatRequest(&dto.ChatRequest{Model: "fixture", Stream: stream, Messages: []dto.ChatMessage{{Role: "user", Text: "hello"}}})
				if err != nil || !json.Valid(body) || strings.Contains(string(body), "include_usage") != (stream && tc.name != "zhipu") {
					t.Fatalf("provider stream options: %s %v", body, err)
				}
				original := []byte(`{"model":"fixture","messages":[],"thinking":{"type":"enabled"},"stream_options":{"custom":true}}`)
				prepared, err := a.PreparePassthroughBody(original, stream)
				if err != nil || !strings.Contains(string(prepared), `"thinking"`) || !strings.Contains(string(prepared), `"custom":true`) {
					t.Fatalf("provider extensions lost: %s %v", prepared, err)
				}
			}
			for mode := channel.RelayModeEmbeddings; mode <= channel.RelayModeResponses; mode++ {
				if _, err := a.ModeURL("", mode); !errors.Is(err, channel.ErrUnsupportedMode) {
					t.Fatalf("unverified endpoint mode %d was enabled", mode)
				}
			}
			if _, err := a.RealtimeURL("", "fixture"); !errors.Is(err, channel.ErrUnsupportedMode) {
				t.Fatal(err)
			}
			if _, err := GetTaskAdaptor(tc.typeID); err == nil {
				t.Fatal("unverified task support")
			}
		})
	}
}
