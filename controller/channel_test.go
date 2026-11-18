package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"starhub/common"
	"starhub/model"
	"starhub/pkg/taskplugin"
	"starhub/relay/channel"
	"starhub/service"
)

func TestFetchChannelModelsDraftAndSavedCredentials(t *testing.T) {
	newTestDB(t)
	var receivedKey string
	requests := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		receivedKey = r.Header.Get("Authorization")
		if r.Method != "GET" || r.URL.Path != "/v1/models" {
			t.Errorf("unexpected discovery request: %s %s", r.Method, r.URL.Path)
		}
		fmt.Fprint(w, `{"data":[{"id":" model-a "},{"id":"model-a"},{"id":"Model-B"},{"id":""}]}`)
	}))
	defer upstream.Close()
	fetch := func(input any) *httptest.ResponseRecorder {
		body, _ := json.Marshal(input)
		return call(FetchChannelModels, testRequest{method: "POST", target: "/api/channel/fetch_models", body: string(body), userID: 1, role: common.RoleAdmin})
	}
	response := fetch(map[string]any{"type": 1, "base_url": upstream.URL + "///", "key": "draft-key\nother-key"})
	var ids []string
	decodeData(t, response, &ids)
	if !reflect.DeepEqual(ids, []string{"model-a", "Model-B"}) || receivedKey != "Bearer draft-key" {
		t.Fatal("draft discovery failed to normalize models or select the explicit key")
	}
	saved := mustChannel(t, "discovery", "existing-model", "disabled-key\nstored-key", common.ChannelStatusManuallyDisabled)
	saved.BaseURL = upstream.URL
	saved.MultiKeyStatus = "2,1"
	if err := model.UpdateChannel(saved); err != nil {
		t.Fatal(err)
	}
	var revision model.ChannelConfigurationState
	model.DB.First(&revision, 1)
	var abilities []model.Ability
	model.DB.Where("channel_id = ?", saved.ID).Find(&abilities)
	before, err := model.GetChannelByID(saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	response = fetch(map[string]any{"channel_id": saved.ID, "type": 1, "base_url": upstream.URL + "/", "key": ""})
	decodeData(t, response, &ids)
	if receivedKey != "Bearer stored-key" {
		t.Fatal("saved discovery did not select an enabled stored key")
	}
	response = fetch(map[string]any{"channel_id": saved.ID, "type": 1, "base_url": upstream.URL, "key": "replacement-key"})
	decodeData(t, response, &ids)
	if receivedKey != "Bearer replacement-key" {
		t.Fatal("draft key did not override the stored key")
	}
	after, _ := model.GetChannelByID(saved.ID)
	var afterRevision model.ChannelConfigurationState
	model.DB.First(&afterRevision, 1)
	var afterAbilities []model.Ability
	model.DB.Where("channel_id = ?", saved.ID).Find(&afterAbilities)
	if !reflect.DeepEqual(before, after) || revision != afterRevision || !reflect.DeepEqual(abilities, afterAbilities) {
		t.Fatal("discovery changed channel configuration, revision or abilities")
	}
	beforeRequests := requests
	for _, input := range []map[string]any{
		{"channel_id": saved.ID, "type": 1, "base_url": upstream.URL + "/different"},
		{"channel_id": saved.ID, "type": 1, "base_url": ""},
	} {
		expectError(t, fetch(input), 400, "models_changed_connection_requires_key")
	}
	saved.Type = 14
	if err := model.UpdateChannel(saved); err != nil {
		t.Fatal(err)
	}
	expectError(t, fetch(map[string]any{"channel_id": saved.ID, "type": 1, "base_url": upstream.URL}), 400, "models_changed_connection_requires_key")
	if requests != beforeRequests {
		t.Fatal("a rejected connection change sent a stored credential upstream")
	}
	for _, tc := range []struct {
		input  map[string]any
		status int
		code   string
	}{
		{map[string]any{"type": 1, "base_url": upstream.URL}, 400, "models_key_required"},
		{map[string]any{"type": 14, "base_url": upstream.URL, "key": "key"}, 400, "models_channel_type_unsupported"},
		{map[string]any{"type": 1, "base_url": upstream.URL + "/v1", "key": "key"}, 400, "models_base_url_contains_v1"},
		{map[string]any{"type": 1, "base_url": "http://user:password@localhost", "key": "key"}, 400, "invalid_models_base_url"},
		{map[string]any{"type": 1, "base_url": "file:///etc/passwd", "key": "key"}, 400, "invalid_models_base_url"},
		{map[string]any{"type": 1, "base_url": upstream.URL + "?key=hidden", "key": "key"}, 400, "invalid_models_base_url"},
		{map[string]any{"channel_id": saved.ID + 100, "type": 1}, 404, "models_channel_unavailable"},
		{map[string]any{"channel_id": -1, "type": 1}, 400, "invalid_models_channel_id"},
	} {
		expectError(t, fetch(tc.input), tc.status, tc.code)
	}
	saved.Type = 1
	saved.MultiKeyStatus = "2,2"
	if err := model.UpdateChannel(saved); err != nil {
		t.Fatal(err)
	}
	expectError(t, fetch(map[string]any{"channel_id": saved.ID, "type": 1}), 400, "models_key_required")
	if requests != beforeRequests {
		t.Fatal("invalid discovery requests reached upstream")
	}
	defaultBase, err := service.NormalizeChannelModelsBaseURL("")
	if err != nil || defaultBase != "https://api.openai.com" {
		t.Fatal("default upstream root was not resolved")
	}
}

func TestFetchChannelModelsExplicitKeySelection(t *testing.T) {
	newTestDB(t)
	var receivedKey string
	requests := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		receivedKey = r.Header.Get("Authorization")
		if receivedKey == "Bearer rejected-key" {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"error":{"code":"AuthenticationError"}}`)
			return
		}
		fmt.Fprint(w, `{"data":[{"id":"selected-model"}]}`)
	}))
	defer upstream.Close()
	fetch := func(input map[string]any) *httptest.ResponseRecorder {
		body, _ := json.Marshal(input)
		return call(FetchChannelModels, testRequest{method: "POST", target: "/api/channel/fetch_models", body: string(body)})
	}
	for _, channelType := range []int{channel.ChannelTypeOpenAI, channel.ChannelTypeArk} {
		saved := mustChannel(t, fmt.Sprintf("selected-key-%d", channelType), "existing-model", "disabled-key\nrejected-key\naccepted-key", common.ChannelStatusManuallyDisabled)
		saved.Type, saved.BaseURL, saved.MultiKeyStatus = channelType, upstream.URL, "2,1,1"
		if err := model.UpdateChannel(saved); err != nil {
			t.Fatal(err)
		}
		before, _ := model.GetChannelByID(saved.ID)
		input := map[string]any{"channel_id": saved.ID, "type": channelType}
		count := requests
		expectError(t, fetch(input), 502, "upstream_models_authentication_failed")
		if requests != count+1 || receivedKey != "Bearer rejected-key" {
			t.Fatal("catalog authentication failure must not retry other keys")
		}
		input["key_index"] = 2
		var ids []string
		decodeData(t, fetch(input), &ids)
		if receivedKey != "Bearer accepted-key" || !reflect.DeepEqual(ids, []string{"selected-model"}) {
			t.Fatal("explicit saved key selection was ignored")
		}
		count = requests
		for _, index := range []int{-1, 0, 3} {
			input["key_index"] = index
			expectError(t, fetch(input), 400, "models_key_selection_invalid")
		}
		input["key_index"], input["base_url"] = 2, upstream.URL+"/changed"
		expectError(t, fetch(input), 400, "models_changed_connection_requires_key")
		if requests != count {
			t.Fatal("invalid selections or connection changes sent saved credentials upstream")
		}
		input["key"], input["key_index"] = "\n rejected-key\r\n\n draft-accepted-key \n", 1
		decodeData(t, fetch(input), &ids)
		if receivedKey != "Bearer draft-accepted-key" {
			t.Fatal("explicit draft key selection did not override saved credentials")
		}
		count = requests
		input["key_index"] = 2
		expectError(t, fetch(input), 400, "models_key_selection_invalid")
		input["key"], input["key_index"] = "single-draft-key", 1
		expectError(t, fetch(input), 400, "models_key_selection_invalid")
		if requests != count {
			t.Fatal("out-of-range draft selections reached upstream")
		}
		after, _ := model.GetChannelByID(saved.ID)
		if !reflect.DeepEqual(before, after) {
			t.Fatal("key discovery changed saved channel configuration or key status")
		}
	}
}

func TestFetchTaskPluginChannelModels(t *testing.T) {
	newTestDB(t)
	stored := saveExamplePlugin(t, "")
	requests := 0
	var authorization, apiKey string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		authorization, apiKey = r.Header.Get("Authorization"), r.Header.Get("x-api-key")
		if r.Method != "GET" || r.URL.Path != "/v1/models" {
			t.Errorf("unexpected discovery request: %s %s", r.Method, r.URL.Path)
		}
		fmt.Fprint(w, `{"data":[{"id":"chat-model"},{"id":"sora-2-pro"},{"id":"sora-2"}]}`)
	}))
	defer upstream.Close()
	fetch := func(input map[string]any) *httptest.ResponseRecorder {
		body, _ := json.Marshal(input)
		return call(FetchChannelModels, testRequest{method: "POST", target: "/api/channel/fetch_models", body: string(body), userID: 1, role: common.RoleAdmin})
	}
	input := map[string]any{"type": channel.ChannelTypeTaskPlugin, "task_plugin_id": stored.ID, "base_url": upstream.URL, "key": "draft-key"}
	var ids []string
	decodeData(t, fetch(input), &ids)
	if !reflect.DeepEqual(ids, []string{"sora-2-pro", "sora-2"}) || authorization != "Bearer draft-key" || apiKey != "" {
		t.Fatal("plugin discovery did not filter models or use bearer authentication")
	}
	var count int64
	model.DB.Model(&model.Channel{}).Count(&count)
	if count != 0 {
		t.Fatal("draft discovery created a channel")
	}
	otherSource := strings.ReplaceAll(stored.Source, `version: "1.1.0"`, `version: "1.2.0"`)
	otherSource = strings.ReplaceAll(otherSource, `auth: "bearer"`, `auth: "x-api-key"`)
	compiled, err := taskplugin.Compile(context.Background(), otherSource)
	if err != nil {
		t.Fatal(err)
	}
	other, err := model.SaveTaskPlugin(otherSource, compiled.Meta)
	if err != nil {
		t.Fatal(err)
	}
	input["task_plugin_id"] = other.ID
	decodeData(t, fetch(input), &ids)
	if apiKey != "draft-key" || authorization != "" {
		t.Fatal("plugin discovery ignored x-api-key authentication")
	}
	saved := &model.Channel{Name: "plugin discovery", Type: channel.ChannelTypeTaskPlugin, TaskPluginID: stored.ID, BaseURL: upstream.URL, Key: "saved-key", Models: "sora-2", Group: "default", Status: common.ChannelStatusEnabled, Weight: 1}
	if err := model.InsertChannel(saved); err != nil {
		t.Fatal(err)
	}
	before, _ := model.GetChannelByID(saved.ID)
	decodeData(t, fetch(map[string]any{"channel_id": saved.ID}), &ids)
	if authorization != "Bearer saved-key" {
		t.Fatal("saved plugin discovery did not use the stored key")
	}
	beforeRequests := requests
	delete(input, "key")
	input["channel_id"] = saved.ID
	expectError(t, fetch(input), 400, "models_changed_connection_requires_key")
	input["key"] = "replacement-key"
	decodeData(t, fetch(input), &ids)
	if apiKey != "replacement-key" {
		t.Fatal("explicit key was not used for the changed plugin")
	}
	input["task_plugin_id"] = stored.ID + other.ID + 999
	expectError(t, fetch(input), 400, "models_plugin_unavailable")
	input["task_plugin_id"] = stored.ID
	input["base_url"] = ""
	expectError(t, fetch(input), 400, "invalid_models_base_url")
	if requests != beforeRequests+1 {
		t.Fatal("rejected plugin configuration reached upstream")
	}
	after, _ := model.GetChannelByID(saved.ID)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("plugin discovery changed saved channel configuration")
	}
	saved.BaseURL = upstream.URL + "/v1"
	if err := model.UpdateChannel(saved); err != nil {
		t.Fatal(err)
	}
	decodeData(t, fetch(map[string]any{"channel_id": saved.ID}), &ids)
	if authorization != "Bearer saved-key" || !reflect.DeepEqual(ids, []string{"sora-2-pro", "sora-2"}) {
		t.Fatal("version-prefixed plugin discovery did not preserve its endpoint or credential")
	}
	expectError(t, fetch(map[string]any{"channel_id": saved.ID, "base_url": upstream.URL}), 400, "models_changed_connection_requires_key")
}

func TestFetchTaskPluginModelsCustomPath(t *testing.T) {
	newTestDB(t)
	plugin := saveExamplePlugin(t, `meta.modelsPath="/catalog/models?limit=50"; meta.auth="x-api-key";`)
	invalid := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/prefix/catalog/models" || r.URL.RawQuery != "limit=50" || r.Header.Get("x-api-key") != "fixture-key" || r.Header.Get("Authorization") != "" {
			t.Errorf("custom discovery path or credential mismatch: %s", r.URL.RequestURI())
		}
		if invalid {
			fmt.Fprint(w, `{"models":["sora-2"]}`)
		} else {
			fmt.Fprint(w, `{"data":[{"id":"sora-2"},{"id":"chat-model"}]}`)
		}
	}))
	defer upstream.Close()
	ch := &model.Channel{Name: "custom catalog", Type: channel.ChannelTypeTaskPlugin, TaskPluginID: plugin.ID, Key: "fixture-key", BaseURL: upstream.URL + "/prefix", Models: "sora-2", Group: "default", Status: common.ChannelStatusEnabled, Weight: 1}
	if err := model.InsertChannel(ch); err != nil {
		t.Fatal(err)
	}
	request := testRequest{method: "POST", target: "/api/channel/fetch_models", body: fmt.Sprintf(`{"channel_id":%d}`, ch.ID)}
	var ids []string
	decodeData(t, call(FetchChannelModels, request), &ids)
	if !reflect.DeepEqual(ids, []string{"sora-2"}) {
		t.Fatalf("unexpected custom catalog models: %v", ids)
	}
	invalid = true
	expectError(t, call(FetchChannelModels, request), http.StatusBadGateway, "upstream_models_invalid_response")
}

func TestFetchChannelModelsUpstreamFailuresAndLimits(t *testing.T) {
	redirected := false
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirected = true }))
	defer destination.Close()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch strings.Split(r.URL.Path, "/")[1] {
		case "auth":
			w.WriteHeader(401)
			fmt.Fprint(w, "sensitive-key")
		case "forbidden":
			w.WriteHeader(403)
		case "missing":
			w.WriteHeader(404)
		case "limited":
			w.WriteHeader(429)
		case "redirect":
			http.Redirect(w, r, destination.URL, 302)
		case "large":
			fmt.Fprint(w, strings.Repeat(" ", (2<<20)+1))
		case "many":
			fmt.Fprint(w, `{"data":[`+strings.TrimSuffix(strings.Repeat(`{"id":"a"},`, 5001), ",")+`]}`)
		case "invalid":
			fmt.Fprint(w, `{"unexpected":[]}`)
		case "badid":
			fmt.Fprint(w, `{"data":[{"id":"a,b"}]}`)
		case "control":
			fmt.Fprint(w, `{"data":[{"id":"a\nb"}]}`)
		case "paged":
			fmt.Fprint(w, `{"data":[{"id":"a"}],"has_more":true}`)
		case "empty":
			fmt.Fprint(w, `{"data":[]}`)
		case "timeout":
			<-r.Context().Done()
		}
	}))
	defer upstream.Close()
	for _, tc := range []struct{ path, code string }{
		{"auth", "upstream_models_authentication_failed"}, {"forbidden", "upstream_models_permission_denied"},
		{"missing", "upstream_models_not_found"}, {"limited", "upstream_models_rate_limited"},
		{"redirect", "upstream_models_redirect_refused"}, {"large", "upstream_models_too_large"},
		{"many", "upstream_models_too_large"}, {"invalid", "upstream_models_invalid_response"},
		{"badid", "upstream_models_invalid_id"}, {"control", "upstream_models_invalid_id"},
		{"paged", "upstream_models_pagination_unsupported"},
	} {
		body, _ := json.Marshal(map[string]any{"type": 1, "base_url": upstream.URL + "/" + tc.path, "key": "sensitive-key"})
		response := call(FetchChannelModels, testRequest{method: "POST", target: "/api/channel/fetch_models", body: string(body)})
		expectError(t, response, 502, tc.code)
		if strings.Contains(response.Body.String(), "sensitive-key") {
			t.Fatal("upstream secret leaked in a discovery response")
		}
	}
	if redirected {
		t.Fatal("discovery followed a credential-bearing redirect")
	}
	ids, err := service.FetchChannelModels(context.Background(), upstream.URL+"/empty", "test-key")
	if err != nil || ids == nil || len(ids) != 0 {
		t.Fatal("valid empty catalog was not returned as an empty array")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err = service.FetchChannelModels(ctx, upstream.URL+"/timeout", "test-key")
	if err == nil || err.Error() != "upstream_models_timeout" {
		t.Fatal("discovery deadline was not classified safely")
	}
}

func mustChannel(t *testing.T, name, models, key string, status int) *model.Channel {
	t.Helper()
	ch := &model.Channel{
		Name:   name,
		Type:   common.DefaultChannelType,
		Key:    key,
		Models: models,
		Group:  common.DefaultGroup,
		Status: status,
		Weight: 1,
	}
	if err := model.InsertChannel(ch); err != nil {
		t.Fatalf("InsertChannel(%s) failed: %v", name, err)
	}
	return ch
}

func admin() (int64, int) { return 1, common.RoleAdmin }

func TestCreateChannelDefaults(t *testing.T) {
	newTestDB(t)
	uid, role := admin()

	rr := call(CreateChannel, testRequest{
		method: http.MethodPost,
		target: "/api/channel",
		body:   `{"name":"main","key":"sk-up","models":"gpt-4o"}`,
		userID: uid,
		role:   role,
	})

	var got map[string]any
	decodeData(t, rr, &got)
	if int(got["type"].(float64)) != common.DefaultChannelType {
		t.Fatalf("type = %v, want the default %d", got["type"], common.DefaultChannelType)
	}
	if int(got["status"].(float64)) != common.ChannelStatusEnabled {
		t.Fatalf("status = %v, want enabled", got["status"])
	}
	if got["weight"].(float64) != 1 {
		t.Fatalf("weight = %v, want 1", got["weight"])
	}
	if got["group"] != common.DefaultGroup {
		t.Fatalf("group = %v, want %q", got["group"], common.DefaultGroup)
	}
	if got["key_count"].(float64) != 1 {
		t.Fatalf("key_count = %v, want 1", got["key_count"])
	}

	// The channel must be discoverable through the ability table.
	if _, err := model.GetSatisfiedChannel(common.DefaultGroup, "gpt-4o"); err != nil {
		t.Fatalf("created channel is not selectable: %v", err)
	}
}

func TestCreateChannelRejectsUnsupportedType(t *testing.T) {
	newTestDB(t)
	uid, role := admin()

	rr := call(CreateChannel, testRequest{
		method: http.MethodPost,
		target: "/api/channel",
		body:   `{"name":"main","key":"sk-up","models":"gpt-4o","type":9999}`,
		userID: uid,
		role:   role,
	})
	// The message enumerates the supported types so the caller can fix it.
	expectError(t, rr, http.StatusBadRequest, "unsupported channel type 9999")
}

func TestCreateChannelRejectsEmptyModels(t *testing.T) {
	newTestDB(t)
	uid, role := admin()

	for _, models := range []string{`""`, `" , "`} {
		rr := call(CreateChannel, testRequest{
			method: http.MethodPost,
			target: "/api/channel",
			body:   fmt.Sprintf(`{"name":"main","key":"sk-up","models":%s}`, models),
			userID: uid,
			role:   role,
		})
		expectError(t, rr, http.StatusBadRequest, "models must be a non-empty")
	}
}

func TestCreateChannelNormalizesBaseURL(t *testing.T) {
	newTestDB(t)
	uid, role := admin()

	rr := call(CreateChannel, testRequest{
		method: http.MethodPost,
		target: "/api/channel",
		body:   `{"name":"main","key":"sk-up","models":"gpt-4o","base_url":"https://api.example.com/v1///"}`,
		userID: uid,
		role:   role,
	})
	var got map[string]any
	decodeData(t, rr, &got)
	if got["base_url"] != "https://api.example.com/v1" {
		t.Fatalf("base_url = %v, want the trailing slashes trimmed", got["base_url"])
	}

	rr = call(CreateChannel, testRequest{
		method: http.MethodPost,
		target: "/api/channel",
		body:   `{"name":"main","key":"sk-up","models":"gpt-4o","base_url":"ftp://api.example.com"}`,
		userID: uid,
		role:   role,
	})
	expectError(t, rr, http.StatusBadRequest, "base_url must start with http")
}

func TestCreateChannelRejectsBadKeyMode(t *testing.T) {
	newTestDB(t)
	uid, role := admin()

	rr := call(CreateChannel, testRequest{
		method: http.MethodPost,
		target: "/api/channel",
		body:   `{"name":"main","key":"sk-up","models":"gpt-4o","key_mode":"sticky"}`,
		userID: uid,
		role:   role,
	})
	expectError(t, rr, http.StatusBadRequest, "key_mode must be one of")

	// The documented modes are accepted and persisted.
	for _, mode := range []string{model.KeyModeRandom, model.KeyModeRoundRobin, ""} {
		rr = call(CreateChannel, testRequest{
			method: http.MethodPost,
			target: "/api/channel",
			body: fmt.Sprintf(`{"name":"m-%s","key":"sk-up","models":"gpt-4o","key_mode":%q}`,
				mode, mode),
			userID: uid,
			role:   role,
		})
		var got map[string]any
		decodeData(t, rr, &got)
		if got["key_mode"] != mode {
			t.Fatalf("key_mode = %v, want %q", got["key_mode"], mode)
		}
	}
}

func TestCreateChannelRejectsInvalidStatus(t *testing.T) {
	newTestDB(t)
	uid, role := admin()

	// Auto-disabled (3) is not settable through the API: only enabled and
	// manually-disabled are.
	rr := call(CreateChannel, testRequest{
		method: http.MethodPost,
		target: "/api/channel",
		body: fmt.Sprintf(`{"name":"main","key":"sk-up","models":"gpt-4o","status":%d}`,
			common.ChannelStatusAutoDisabled),
		userID: uid,
		role:   role,
	})
	expectError(t, rr, http.StatusBadRequest, "invalid status")
}

func TestCreateChannelNormalizesGroupList(t *testing.T) {
	newTestDB(t)
	uid, role := admin()

	rr := call(CreateChannel, testRequest{
		method: http.MethodPost,
		target: "/api/channel",
		body:   `{"name":"main","key":"sk-up","models":"a, b ,,c","group":" x , y "}`,
		userID: uid,
		role:   role,
	})
	var got map[string]any
	decodeData(t, rr, &got)
	if got["models"] != "a,b,c" {
		t.Fatalf("models = %v, want normalized a,b,c", got["models"])
	}
	if got["group"] != "x,y" {
		t.Fatalf("group = %v, want normalized x,y", got["group"])
	}
}

func TestGetChannelInvalidPathID(t *testing.T) {
	newTestDB(t)
	uid, role := admin()

	for _, raw := range []string{"", "abc", "0", "-1"} {
		rr := call(GetChannel, testRequest{
			target: "/api/channel/" + raw,
			userID: uid,
			role:   role,
			path:   map[string]string{"id": raw},
		})
		expectError(t, rr, http.StatusBadRequest, "invalid id in path")
	}
}

func TestGetChannelNotFound(t *testing.T) {
	newTestDB(t)
	uid, role := admin()

	rr := call(GetChannel, testRequest{
		target: "/api/channel/9999",
		userID: uid,
		role:   role,
		path:   map[string]string{"id": "9999"},
	})
	expectError(t, rr, http.StatusNotFound, "channel not found")
}

// A partial update must leave every field the caller omitted alone. The
// notable exception is `key`: replacing the keys clears the per-key disable
// state, because those statuses refer to keys that no longer exist.
func TestUpdateChannelPartialSemantics(t *testing.T) {
	newTestDB(t)
	uid, role := admin()
	ch := mustChannel(t, "main", "gpt-4o", "k1\nk2", common.ChannelStatusEnabled)
	ch.MultiKeyStatus = "2,1"
	if err := model.UpdateChannel(ch); err != nil {
		t.Fatalf("UpdateChannel failed: %v", err)
	}

	// Renaming only.
	rr := call(UpdateChannel, testRequest{
		method: http.MethodPut,
		target: "/api/channel",
		body:   fmt.Sprintf(`{"id":%d,"name":"renamed"}`, ch.ID),
		userID: uid,
		role:   role,
	})
	expectStatus(t, rr, http.StatusOK)
	got, err := model.GetChannelByID(ch.ID)
	if err != nil {
		t.Fatalf("GetChannelByID failed: %v", err)
	}
	if got.Name != "renamed" {
		t.Fatalf("name = %q, want renamed", got.Name)
	}
	if got.Models != "gpt-4o" {
		t.Fatalf("models = %q, want untouched gpt-4o", got.Models)
	}
	if got.Key != "k1\nk2" {
		t.Fatalf("key = %q, want untouched", got.Key)
	}
	if got.MultiKeyStatus != "2,1" {
		t.Fatalf("multi_key_status = %q, want untouched when the key is unchanged", got.MultiKeyStatus)
	}

	// Replacing the keys resets the stale per-key statuses.
	rr = call(UpdateChannel, testRequest{
		method: http.MethodPut,
		target: "/api/channel",
		body:   fmt.Sprintf(`{"id":%d,"key":"fresh-1\nfresh-2"}`, ch.ID),
		userID: uid,
		role:   role,
	})
	expectStatus(t, rr, http.StatusOK)
	got, _ = model.GetChannelByID(ch.ID)
	if got.Key != "fresh-1\nfresh-2" {
		t.Fatalf("key = %q, want the replacement", got.Key)
	}
	if got.MultiKeyStatus != "" {
		t.Fatalf("multi_key_status = %q, want cleared after a key swap", got.MultiKeyStatus)
	}
	var disabled int
	for _, s := range got.KeyStatuses() {
		if s != common.StatusEnabled {
			disabled++
		}
	}
	if disabled != 0 {
		t.Fatalf("%d fresh keys still carry a disabled status", disabled)
	}
}

func TestUpdateChannelValidationErrors(t *testing.T) {
	newTestDB(t)
	uid, role := admin()
	ch := mustChannel(t, "main", "gpt-4o", "sk-up", common.ChannelStatusEnabled)
	id := itoa(ch.ID)

	cases := []struct {
		name string
		body string
		want string
	}{
		{"missing id", `{"name":"x"}`, "channel id is required"},
		{"unknown id", `{"id":9999,"name":"x"}`, "channel not found"},
		{"bad key_mode", fmt.Sprintf(`{"id":%s,"key_mode":"nope"}`, id), "key_mode must be one of"},
		{"bad base_url", fmt.Sprintf(`{"id":%s,"base_url":"ftp://x"}`, id), "base_url must start with http"},
		{"empty models", fmt.Sprintf(`{"id":%s,"models":" , "}`, id), "models must be a non-empty"},
		{"empty group", fmt.Sprintf(`{"id":%s,"group":" , "}`, id), "invalid group"},
		{"bad status", fmt.Sprintf(`{"id":%s,"status":%d}`, id, common.ChannelStatusAutoDisabled), "invalid status"},
	}
	for _, tc := range cases {
		rr := call(UpdateChannel, testRequest{
			method: http.MethodPut,
			target: "/api/channel",
			body:   tc.body,
			userID: uid,
			role:   role,
		})
		status := http.StatusBadRequest
		if tc.name == "unknown id" {
			status = http.StatusNotFound
		}
		expectError(t, rr, status, tc.want)
	}

	// None of the rejected updates may have touched the row.
	got, err := model.GetChannelByID(ch.ID)
	if err != nil {
		t.Fatalf("GetChannelByID failed: %v", err)
	}
	if got.KeyMode != "" || got.BaseURL != "" || got.Models != "gpt-4o" || got.Status != common.ChannelStatusEnabled {
		t.Fatalf("a rejected update mutated the channel: %+v", got)
	}
}

func TestDeleteChannelNotFoundHandler(t *testing.T) {
	newTestDB(t)
	uid, role := admin()

	rr := call(DeleteChannel, testRequest{
		method: http.MethodDelete,
		target: "/api/channel/9999",
		userID: uid,
		role:   role,
		path:   map[string]string{"id": "9999"},
	})
	expectError(t, rr, http.StatusNotFound, "channel not found")
}

func TestDeleteChannelRemovesFromSelection(t *testing.T) {
	newTestDB(t)
	uid, role := admin()
	ch := mustChannel(t, "main", "gpt-4o", "sk-up", common.ChannelStatusEnabled)

	rr := call(DeleteChannel, testRequest{
		method: http.MethodDelete,
		target: "/api/channel/" + itoa(ch.ID),
		userID: uid,
		role:   role,
		path:   map[string]string{"id": itoa(ch.ID)},
	})
	expectStatus(t, rr, http.StatusOK)
	if _, err := model.GetSatisfiedChannel(common.DefaultGroup, "gpt-4o"); err != model.ErrNoChannel {
		t.Fatalf("deleted channel is still selectable: %v", err)
	}
}

// TestChannel only validates the request; the probe itself needs a live
// upstream, so these tests assert the guards that run before any network I/O.
func TestTestChannelGuards(t *testing.T) {
	newTestDB(t)
	uid, role := admin()
	ch := mustChannel(t, "main", "gpt-4o,claude-3", "sk-up", common.ChannelStatusEnabled)
	id := itoa(ch.ID)

	rr := call(TestChannel, testRequest{
		method: http.MethodPost,
		target: "/api/channel/test/9999",
		userID: uid,
		role:   role,
		path:   map[string]string{"id": "9999"},
	})
	expectError(t, rr, http.StatusNotFound, "channel not found")

	rr = call(TestChannel, testRequest{
		method: http.MethodPost,
		target: "/api/channel/test/" + id + "?model=not-in-list",
		userID: uid,
		role:   role,
		path:   map[string]string{"id": id},
	})
	expectError(t, rr, http.StatusBadRequest, "does not belong to this channel")

	// A channel with no models cannot be probed at all.
	empty := mustChannel(t, "empty", "gpt-4o", "sk-up", common.ChannelStatusEnabled)
	if err := model.DB.Model(&model.Channel{}).Where("id = ?", empty.ID).
		Update("models", "").Error; err != nil {
		t.Fatalf("clearing models failed: %v", err)
	}
	rr = call(TestChannel, testRequest{
		method: http.MethodPost,
		target: "/api/channel/test/" + itoa(empty.ID),
		userID: uid,
		role:   role,
		path:   map[string]string{"id": itoa(empty.ID)},
	})
	expectError(t, rr, http.StatusBadRequest, "has no models")
}

func TestMaskedChannelHidesKeyAndCountsKeys(t *testing.T) {
	newTestDB(t)
	uid, role := admin()
	ch := mustChannel(t, "multi", "gpt-4o", "secret-1\nsecret-2\nsecret-3", common.ChannelStatusEnabled)

	rr := call(GetChannel, testRequest{
		target: "/api/channel/" + itoa(ch.ID),
		userID: uid,
		role:   role,
		path:   map[string]string{"id": itoa(ch.ID)},
	})

	var got map[string]any
	decodeData(t, rr, &got)
	masked := got["key"].(string)
	for _, key := range ch.Keys() {
		if strings.Contains(masked, key) {
			t.Fatalf("the raw upstream key %q was returned in %q", key, masked)
		}
	}
	if got["key_count"].(float64) != 3 {
		t.Fatalf("key_count = %v, want 3", got["key_count"])
	}
	statuses := got["multi_key_status"].([]any)
	if len(statuses) != 3 {
		t.Fatalf("multi_key_status has %d entries, want 3 (aligned with the keys)", len(statuses))
	}
	for _, s := range statuses {
		if int(s.(float64)) != common.StatusEnabled {
			t.Fatalf("fresh multi-key channel reports a disabled key: %v", statuses)
		}
	}
}

func TestFormatChannelTypesIsSortedAndStable(t *testing.T) {
	out := formatChannelTypes()
	if out == "" {
		t.Fatal("formatChannelTypes returned an empty string")
	}
	// Stable ordering matters: the string is echoed back in API errors, so a
	// map iteration order would make the message flap.
	if again := formatChannelTypes(); again != out {
		t.Fatalf("formatChannelTypes is not deterministic:\n%s\n%s", out, again)
	}
	previous := -1
	for _, entry := range strings.Split(out, ", ") {
		idText, _, ok := strings.Cut(entry, " (")
		if !ok {
			t.Fatalf("channel type entry has no ID: %q", entry)
		}
		id, err := strconv.Atoi(idText)
		if err != nil {
			t.Fatalf("channel type entry has invalid ID %q: %v", idText, err)
		}
		if id <= previous {
			t.Fatalf("channel type IDs are not strictly ascending: %q", out)
		}
		previous = id
	}
}

func TestNormalizeKeyModeAndBaseURLHelpers(t *testing.T) {
	rr := &testRecorder{}
	if mode, ok := normalizeKeyMode(rr, "  "); !ok || mode != "" {
		t.Fatalf("blank key_mode = %q, %v; want empty and accepted", mode, ok)
	}
	if _, ok := normalizeKeyMode(rr, "sticky"); ok {
		t.Fatal("unknown key_mode should be rejected")
	}
	if rr.status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.status)
	}

	rr = &testRecorder{}
	if base, ok := normalizeBaseURL(rr, "  https://x/y/  "); !ok || base != "https://x/y" {
		t.Fatalf("normalizeBaseURL = %q, %v", base, ok)
	}
	if base, ok := normalizeBaseURL(rr, ""); !ok || base != "" {
		t.Fatalf("empty base_url should be accepted as-is, got %q %v", base, ok)
	}
}

func TestModelInList(t *testing.T) {
	list := []string{"gpt-4o", "claude-3"}
	if !modelInList("gpt-4o", list) {
		t.Fatal("exact match should be found")
	}
	if modelInList("gpt-4", list) || modelInList("", list) {
		t.Fatal("partial or empty names must not match")
	}
}

func TestGetChannelUpstreamUsageChannelNotFound(t *testing.T) {
	newTestDB(t)
	uid, role := admin()
	rr := call(GetChannelUpstreamUsage, testRequest{
		target: "/api/channel/9999/upstream-usage",
		userID: uid,
		role:   role,
		path:   map[string]string{"id": "9999"},
	})
	expectError(t, rr, http.StatusNotFound, "channel not found")
}

func TestGetChannelUpstreamUsageUnsupportedUpstream(t *testing.T) {
	newTestDB(t)
	uid, role := admin()
	ch := mustChannel(t, "test", "gpt-4o", "sk-upstream12345", common.ChannelStatusEnabled)
	// The channel has no real upstream, so the service will report
	// unsupported. The controller must still return 200 with supported=false.
	rr := call(GetChannelUpstreamUsage, testRequest{
		target: "/api/channel/" + itoa(ch.ID) + "/upstream-usage",
		userID: uid,
		role:   role,
		path:   map[string]string{"id": itoa(ch.ID)},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var got map[string]any
	decodeData(t, rr, &got)
	if got["supported"] != false {
		t.Fatalf("expected supported=false, got %v", got["supported"])
	}
	if got["channel_id"].(float64) != float64(ch.ID) {
		t.Fatalf("expected channel_id=%d, got %v", ch.ID, got["channel_id"])
	}
}
