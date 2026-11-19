package main

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"starhub/common"
	"starhub/model"
)

func TestPublicStatusOverviewUsesOneAnonymousScopeForEveryIdentity(t *testing.T) {
	env := setupRelayApp(t)
	id := createChannelID(t, env, map[string]any{"name": "Public status fixture", "models": "public-model", "key": "private-upstream-key"})
	createChannelID(t, env, map[string]any{"name": "Private fixture", "models": "hidden-model", "group": "private", "key": "hidden-key"})
	if err := model.DB.Create(&model.Log{ChannelID: id, Model: "public-model", Status: 2, DurationMs: 25}).Error; err != nil {
		t.Fatal(err)
	}
	user := &model.User{Username: "public-status-viewer", Password: "unused-password", Group: "default", Role: common.RoleUser, Status: 1, AffCode: common.GenerateAffCode()}
	if err := model.CreateUser(user); err != nil {
		t.Fatal(err)
	}
	var models []model.PublicStatusModel
	for _, token := range []string{"", env.session, fixtureSession(t, user)} {
		response := doRequest(env.base, http.MethodGet, "/api/status/overview?user_id=999&group=private&days=99999", token, nil)
		var decoded struct {
			Data model.PublicStatusOverview `json:"data"`
		}
		if response.Status != 200 || json.Unmarshal([]byte(response.Body), &decoded) != nil || len(decoded.Data.Models) != 1 || decoded.Data.Models[0].Recent.SuccessRate == nil || *decoded.Data.Models[0].Recent.SuccessRate != 0 {
			t.Fatalf("public status: %d %s", response.Status, response.Body)
		}
		if models == nil {
			models = decoded.Data.Models
		} else if !reflect.DeepEqual(models, decoded.Data.Models) {
			t.Fatal("public observations changed with the viewer's identity")
		}
		for _, secret := range []string{"hidden-model", "private-upstream-key", "hidden-key", "channel_id", "user_id", "quota", "token_id"} {
			if strings.Contains(response.Body, secret) {
				t.Fatal("public response exposed private fields", secret)
			}
		}
	}
	if response := doRequest(env.base, http.MethodGet, "/api/service-quality", "", nil); response.Status != 401 {
		t.Fatal("personal metrics lost authentication")
	}
}
