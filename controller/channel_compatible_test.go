package controller

import (
	"encoding/json"
	"testing"

	"starhub/common"
	"starhub/model"
	"starhub/relay/channel"
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
