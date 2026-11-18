package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"starhub/common"
	"starhub/middleware"
	"starhub/model"
	"starhub/relay/channel"
	"starhub/relay/dto"
)

func TestArkModelDiscovery(t *testing.T) {
	newTestDB(t)
	hits := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.URL.Path != "/prefix/api/v3/models" || r.Header.Get("Authorization") != "Bearer fixture-key" {
			t.Errorf("wrong Ark discovery path or credential: %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"data":[{"id":"doubao-seedream-4-5-251128","domain":"ImageGeneration"}]}`)
	}))
	defer upstream.Close()
	ch := &model.Channel{Name: "Ark", Type: channel.ChannelTypeArk, Key: "fixture-key", BaseURL: upstream.URL + "/prefix", Models: "doubao-seedream-4-5-251128", Group: "default", Status: 1, Weight: 1}
	if err := model.InsertChannel(ch); err != nil {
		t.Fatal(err)
	}
	request := testRequest{method: "POST", target: "/api/channel/fetch_models", body: fmt.Sprintf(`{"channel_id":%d,"type":32,"base_url":%q}`, ch.ID, ch.BaseURL+"/api/v3/")}
	var ids []string
	decodeData(t, call(FetchChannelModels, request), &ids)
	if !reflect.DeepEqual(ids, []string{ch.Models}) {
		t.Fatal(ids)
	}
	request.body = fmt.Sprintf(`{"channel_id":%d,"type":32,"base_url":%q}`, ch.ID, upstream.URL+"/other")
	expectError(t, call(FetchChannelModels, request), 400, "models_changed_connection_requires_key")
	if hits != 1 {
		t.Fatal("changed connection leaked a saved credential")
	}
}

func TestArkImageRelay(t *testing.T) {
	newTestDB(t)
	const modelID = "doubao-seedream-4-5-251128"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/images/generations" || r.Header.Get("Authorization") != "Bearer fixture-key" {
			t.Errorf("wrong Ark image path or credential: %s", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["model"] != modelID {
			t.Error("image model was changed")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"data":[{"url":"https://example.test/image.png"}]}`)
	}))
	defer upstream.Close()
	ch := &model.Channel{Name: "Ark image", Type: channel.ChannelTypeArk, Key: "fixture-key", BaseURL: upstream.URL + "/api/v3", Models: modelID, Group: "default", Status: 1, Weight: 1}
	if err := model.InsertChannel(ch); err != nil {
		t.Fatal(err)
	}
	if err := model.UpsertModelMeta(&model.ModelMeta{Model: modelID, Tags: "image"}); err != nil {
		t.Fatal(err)
	}
	u := newUser(t, "ark-image", common.RoleUser, 1000000000)
	token := mustToken(t, u.ID, 1000000000)
	r := httptest.NewRequest("POST", "/v1/images/generations", strings.NewReader(`{"model":"`+modelID+`","prompt":"a cat","size":"2048x2048"}`))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer "+token.PlaintextKey())
	w := httptest.NewRecorder()
	middleware.TokenAuth(middleware.Distribute(dto.InboundOpenAI)(http.HandlerFunc(RelayImageGenerations))).ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "image.png") {
		t.Fatalf("Ark image relay failed: %d %s", w.Code, w.Body.String())
	}
}
