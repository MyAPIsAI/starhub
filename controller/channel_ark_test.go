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
	"starhub/service"
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
	request.body = fmt.Sprintf(`{"channel_id":%d,"include_metadata":true}`, ch.ID)
	var catalog []struct {
		ID     string `json:"id"`
		Domain string `json:"domain"`
	}
	decodeData(t, call(FetchChannelModels, request), &catalog)
	if len(catalog) != 1 || catalog[0].ID != ch.Models || catalog[0].Domain != "ImageGeneration" {
		t.Fatal("Ark capability metadata was lost")
	}
	var channels int64
	model.DB.Model(&model.Channel{}).Count(&channels)
	if channels != 1 {
		t.Fatal("catalog discovery changed channel configuration")
	}
}

func TestArkDiscoveryAuthenticationDiagnostics(t *testing.T) {
	newTestDB(t)
	for _, status := range []int{401, 403} {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer fixture-key" {
				t.Error("credential changed")
			}
			w.Header().Set("X-Request-Id", "fixture-request-id")
			w.WriteHeader(status)
			fmt.Fprint(w, `{"error":{"code":"AuthenticationError.InvalidApiKey","message":"fixture-key must never be echoed"}}`)
		}))
		response := call(FetchChannelModels, testRequest{method: "POST", target: "/api/channel/fetch_models", body: fmt.Sprintf(`{"type":32,"base_url":%q,"key":"fixture-key"}`, upstream.URL)})
		code := "upstream_models_authentication_failed"
		if status == 403 {
			code = "upstream_models_permission_denied"
		}
		expectError(t, response, 502, code)
		var envelope struct {
			Data service.ChannelModelsDiagnostic `json:"data"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		if envelope.Data.UpstreamStatus != status || envelope.Data.ProviderCode != "AuthenticationError" || envelope.Data.RequestID != "fixture-request-id" || strings.Contains(response.Body.String(), "fixture-key") {
			t.Fatalf("unsafe or incomplete diagnostic: %s", response.Body.String())
		}
		upstream.Close()
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-Id", "fixture-key")
		w.WriteHeader(401)
		fmt.Fprint(w, `{"error":{"code":"fixture-key"}}`)
	}))
	defer upstream.Close()
	response := call(FetchChannelModels, testRequest{method: "POST", target: "/api/channel/fetch_models", body: fmt.Sprintf(`{"type":32,"base_url":%q,"key":"fixture-key"}`, upstream.URL)})
	if strings.Contains(response.Body.String(), "fixture-key") {
		t.Fatal("upstream diagnostic echoed a credential")
	}
	for _, key := range []string{"Bearer fixture-key", "fixture key"} {
		response := call(FetchChannelModels, testRequest{method: "POST", target: "/api/channel/fetch_models", body: fmt.Sprintf(`{"type":32,"base_url":%q,"key":%q}`, upstream.URL, key)})
		expectError(t, response, 400, "models_key_format_invalid")
	}
}

func TestArkChannelCapabilitiesPersist(t *testing.T) {
	newTestDB(t)
	response := call(CreateChannel, testRequest{method: "POST", target: "/api/channel", body: `{"name":"Ark","type":32,"key":"fixture-key","models":"exact-version","model_capabilities":{"exact-version":"ImageGeneration","unselected":"LLM"}}`})
	var created struct {
		ID                int64             `json:"id"`
		ModelCapabilities map[string]string `json:"model_capabilities"`
	}
	decodeData(t, response, &created)
	if !reflect.DeepEqual(created.ModelCapabilities, map[string]string{"exact-version": "ImageGeneration"}) {
		t.Fatal("unselected capabilities were persisted")
	}
	saved, err := model.GetChannelByID(created.ID)
	if err != nil || !reflect.DeepEqual(saved.ModelCapabilities, created.ModelCapabilities) {
		t.Fatal("capability metadata did not survive a database read")
	}
	response = call(UpdateChannel, testRequest{method: "PUT", target: "/api/channel", body: fmt.Sprintf(`{"id":%d,"models":"replacement"}`, created.ID)})
	created.ModelCapabilities = nil
	decodeData(t, response, &created)
	if len(created.ModelCapabilities) != 0 {
		t.Fatal("removed model capabilities were retained")
	}
}

func TestArkRejectsImageShortNames(t *testing.T) {
	newTestDB(t)
	for _, name := range []string{"doubao-seedream-4.5", "doubao-seedream-5.0"} {
		expectError(t, call(CreateChannel, testRequest{method: "POST", target: "/api/channel", body: fmt.Sprintf(`{"type":32,"models":%q}`, name)}), 400, "ark_model_version_required")
	}
	ch := &model.Channel{Type: channel.ChannelTypeArk, Models: "doubao-seedream-4.5", Group: "default", Status: 1}
	if err := model.InsertChannel(ch); err != nil {
		t.Fatal(err)
	}
	expectError(t, call(UpdateChannel, testRequest{method: "PUT", target: "/api/channel", body: fmt.Sprintf(`{"id":%d,"models":"doubao-seedream-4.5"}`, ch.ID)}), 400, "ark_model_version_required")
	expectStatus(t, call(UpdateChannel, testRequest{method: "PUT", target: "/api/channel", body: fmt.Sprintf(`{"id":%d,"status":2}`, ch.ID)}), 200)
	for _, name := range []string{"doubao-seedream-4-5-251128", "ep-fixture"} {
		expectStatus(t, call(UpdateChannel, testRequest{method: "PUT", target: "/api/channel", body: fmt.Sprintf(`{"id":%d,"models":%q}`, ch.ID, name)}), 200)
	}
	expectStatus(t, call(CreateChannel, testRequest{method: "POST", target: "/api/channel", body: `{"type":1,"models":"doubao-seedream-4.5"}`}), 200)
}

func TestArkImageRelay(t *testing.T) {
	newTestDB(t)
	const modelID = "doubao-seedream-4-5-251128"
	wantSize := ""
	hits := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.URL.Path != "/api/v3/images/generations" || r.Header.Get("Authorization") != "Bearer fixture-key" {
			t.Errorf("wrong Ark image path or credential: %s", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["model"] != modelID {
			t.Error("image model was changed")
		}
		if body["size"] != wantSize || body["watermark"] != false {
			t.Error("Ark image parameters were changed")
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
	for _, size := range []string{"2048x2048", "2K", "4K", "1024x1024"} {
		wantSize = size
		before := hits
		r := httptest.NewRequest("POST", "/v1/images/generations", strings.NewReader(fmt.Sprintf(`{"model":%q,"prompt":"a cat","size":%q,"watermark":false}`, modelID, size)))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Authorization", "Bearer "+token.PlaintextKey())
		w := httptest.NewRecorder()
		middleware.TokenAuth(middleware.Distribute(dto.InboundOpenAI)(http.HandlerFunc(RelayImageGenerations))).ServeHTTP(w, r)
		if size == "1024x1024" {
			if w.Code != 400 || hits != before {
				t.Fatal("invalid Seedream size reached upstream")
			}
		} else if w.Code != 200 || !strings.Contains(w.Body.String(), "image.png") || hits != before+1 {
			t.Fatalf("Ark image relay size %q failed: %d %s", size, w.Code, w.Body.String())
		}
	}
}

func TestArkCapabilityProbes(t *testing.T) {
	newTestDB(t)
	const modelID = "doubao-seedream-4-5-251128"
	images, chats, catalogs := 0, 0, 0
	domain := "ImageGeneration"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v3/models":
			catalogs++
			fmt.Fprintf(w, `{"data":[{"id":%q,"domain":%q}]}`, modelID, domain)
		case "/api/v3/images/generations":
			images++
			if r.Header.Get("Authorization") != "Bearer fixture-key" {
				t.Error("wrong image test credential")
			}
			fmt.Fprint(w, `{"data":[{"url":"https://example.test/probe.png"}]}`)
		case "/api/v3/chat/completions":
			chats++
			fmt.Fprint(w, `{"choices":[{"message":{"content":"pong"}}]}`)
		default:
			t.Errorf("unexpected probe path %s", r.URL.Path)
		}
	}))
	defer upstream.Close()
	ch := &model.Channel{Name: "Ark probes", Type: channel.ChannelTypeArk, Key: "fixture-key", BaseURL: upstream.URL, Models: modelID, Group: "default", Status: 1, Weight: 1}
	if err := model.InsertChannel(ch); err != nil {
		t.Fatal(err)
	}
	request := testRequest{method: "POST", target: fmt.Sprintf("/api/channel/test/%d", ch.ID), path: map[string]string{"id": fmt.Sprint(ch.ID)}}
	var result channelTestResult
	decodeData(t, call(TestChannel, request), &result)
	if !result.OK || result.Scope != "catalog" || images != 0 || chats != 0 {
		t.Fatal("default image probe submitted inference")
	}
	ch.Models = "doubao-seedream-4.5"
	if err := model.UpdateChannel(ch); err != nil {
		t.Fatal(err)
	}
	decodeData(t, call(TestChannel, request), &result)
	if result.OK || result.Code != "upstream_model_not_found" || result.Model != ch.Models || result.Scope != "catalog" || result.Status != 1 || images != 0 || chats != 0 {
		t.Fatal("legacy model mismatch was not diagnosed without inference")
	}
	ch.Models = modelID
	if err := model.UpdateChannel(ch); err != nil {
		t.Fatal(err)
	}
	if err := model.UpdateChannelStatus(ch.ID, common.ChannelStatusAutoDisabled); err != nil {
		t.Fatal(err)
	}
	service.RunChannelTestOnce(t.Context())
	after, _ := model.GetChannelByID(ch.ID)
	if after.Status != common.ChannelStatusAutoDisabled || images != 0 || chats != 0 {
		t.Fatal("catalog-only automatic probe changed inference health or submitted inference")
	}
	request.target += "?mode=image"
	decodeData(t, call(TestChannel, request), &result)
	if !result.OK || result.Scope != "image" || result.ImageURL != "https://example.test/probe.png" || images != 1 || chats != 0 {
		t.Fatal("explicit image probe did not perform exactly one generation")
	}
	domain = "LLM"
	request.target = fmt.Sprintf("/api/channel/test/%d", ch.ID)
	decodeData(t, call(TestChannel, request), &result)
	if !result.OK || chats != 1 || catalogs < 3 {
		t.Fatal("text capability did not use the chat probe")
	}
	request.target += "?mode=unsupported"
	expectStatus(t, call(TestChannel, request), http.StatusBadRequest)
	ch.Models = "ep-fixture"
	ch.ModelCapabilities = nil
	if err := model.UpdateChannel(ch); err != nil {
		t.Fatal(err)
	}
	request.target = fmt.Sprintf("/api/channel/test/%d?mode=image&model=ep-fixture", ch.ID)
	decodeData(t, call(TestChannel, request), &result)
	if !result.OK || result.Model != "ep-fixture" || images != 2 {
		t.Fatal("explicit endpoint image probe required catalog metadata")
	}
	request.target = fmt.Sprintf("/api/channel/test/%d?mode=image&model=unconfigured", ch.ID)
	expectStatus(t, call(TestChannel, request), http.StatusBadRequest)
	if images != 2 {
		t.Fatal("unconfigured model reached image generation")
	}
}
