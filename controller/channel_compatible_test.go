package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"starhub/common"
	"starhub/model"
	"starhub/relay/channel"
	"starhub/service"
)

func TestCompatibleChannelCreateAndUpdate(t *testing.T) {
	newTestDB(t)
	for _, typeID := range []int{channel.ChannelTypeZhipu, channel.ChannelTypeMoonshot, channel.ChannelTypeDeepSeek} {
		body, _ := json.Marshal(map[string]any{"name": "compatible fixture", "type": typeID, "key": "fixture-key", "models": "fixture-model"})
		response := call(CreateChannel, testRequest{method: "POST", target: "/api/channel", body: string(body), userID: 1, role: common.RoleAdmin})
		var created struct {
			ID   int64 `json:"id"`
			Type int   `json:"type"`
		}
		decodeData(t, response, &created)
		saved, err := model.GetChannelByID(created.ID)
		if err != nil || saved.Type != typeID || saved.BaseURL != "" || saved.Key != "fixture-key" {
			t.Fatalf("create: %+v %v", saved, err)
		}
		body, _ = json.Marshal(map[string]any{"id": created.ID, "name": "edited fixture", "type": typeID, "key": "", "models": "fixture-model", "base_url": "http://localhost:1234/proxy"})
		response = call(UpdateChannel, testRequest{method: "PUT", target: "/api/channel", body: string(body), userID: 1, role: common.RoleAdmin})
		decodeData(t, response, &created)
		saved, err = model.GetChannelByID(created.ID)
		if err != nil || saved.Type != typeID || saved.Key != "fixture-key" || saved.BaseURL != "http://localhost:1234/proxy" {
			t.Fatalf("update: %+v %v", saved, err)
		}
	}
}

func TestCompatibleChannelModelDiscovery(t *testing.T) {
	newTestDB(t)
	for _, tc := range []struct {
		typeID            int
		path, defaultBase string
	}{
		{channel.ChannelTypeMoonshot, "/v1/models", "https://api.moonshot.cn/v1"},
		{channel.ChannelTypeDeepSeek, "/models", "https://api.deepseek.com"},
	} {
		t.Run(tc.defaultBase, func(t *testing.T) {
			requests, expectedKey := 0, "Bearer fixture-key"
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if r.Method != "GET" || r.URL.Path != tc.path || r.Header.Get("Authorization") != expectedKey {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				fmt.Fprint(w, `{"data":[{"id":"fixture-model"},{"id":"fixture-model"}]}`)
			}))
			defer upstream.Close()
			fetch := func(input any) *httptest.ResponseRecorder {
				body, _ := json.Marshal(input)
				return call(FetchChannelModels, testRequest{method: "POST", target: "/api/channel/fetch_models", body: string(body), userID: 1, role: common.RoleAdmin})
			}
			var ids []string
			decodeData(t, fetch(map[string]any{"type": tc.typeID, "base_url": upstream.URL, "key": "fixture-key"}), &ids)
			if !reflect.DeepEqual(ids, []string{"fixture-model"}) {
				t.Fatal(ids)
			}
			base, err := service.NormalizeCompatibleModelsBaseURL(tc.typeID, "")
			if err != nil || base != tc.defaultBase {
				t.Fatalf("default: %q %v", base, err)
			}
			saved := &model.Channel{Name: "compatible discovery", Type: tc.typeID, Key: "saved-key", BaseURL: upstream.URL, Models: "fixture-model", Group: "default", Status: common.ChannelStatusEnabled, Weight: 1}
			if err := model.InsertChannel(saved); err != nil {
				t.Fatal(err)
			}
			expectedKey = "Bearer saved-key"
			decodeData(t, fetch(map[string]any{"channel_id": saved.ID, "type": tc.typeID, "base_url": upstream.URL}), &ids)
			before := requests
			for _, base := range []string{upstream.URL + "/changed", upstream.URL + "?key=secret", "http://user:secret@localhost"} {
				response := fetch(map[string]any{"channel_id": saved.ID, "type": tc.typeID, "base_url": base})
				if response.Code != 400 {
					t.Fatalf("unsafe credential reuse: %d", response.Code)
				}
			}
			otherType := channel.ChannelTypeMoonshot
			if tc.typeID == otherType {
				otherType = channel.ChannelTypeDeepSeek
			}
			expectError(t, fetch(map[string]any{"channel_id": saved.ID, "type": otherType, "base_url": upstream.URL}), 400, "models_changed_connection_requires_key")
			expectError(t, fetch(map[string]any{"type": channel.ChannelTypeZhipu, "base_url": upstream.URL, "key": "fixture-key"}), 400, "models_channel_type_unsupported")
			if requests != before {
				t.Fatal("rejected discovery contacted upstream")
			}
		})
	}
}
