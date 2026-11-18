package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"starhub/common"
	"starhub/middleware"
	"starhub/model"
	"starhub/relay/channel"
	"starhub/relay/dto"
	"starhub/service"
	"starhub/setting"
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

func TestArkVideoTaskLifecycle(t *testing.T) {
	const modelID = "doubao-seedance-2-0-260128"
	for _, tc := range []struct {
		status          string
		tokens, recover bool
		want            int64
	}{
		{"succeeded", false, false, 40000000}, {"succeeded", true, true, 246000},
		{"failed", false, false, 0}, {"expired", false, false, 0}, {"cancelled", false, false, 0},
	} {
		t.Run(fmt.Sprintf("%s/tokens=%v", tc.status, tc.tokens), func(t *testing.T) {
			newTestDB(t)
			prices, rules := `{"`+modelID+`":0.01}`, "{}"
			if tc.tokens {
				prices, rules = "{}", strings.ReplaceAll(taskAbsoluteRates, "sora-2", modelID)
			}
			if err := setting.ApplyOptions(map[string]string{setting.OptionKeyModelPrice: prices, setting.OptionKeyModelTimeTokenPrices: rules}); err != nil {
				t.Fatal(err)
			}
			var posts, gets atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer video-key" {
					t.Error("video credential changed")
				}
				switch r.Method + " " + r.URL.Path {
				case "POST /prefix/api/v3/contents/generations/tasks":
					posts.Add(1)
					var body struct {
						Model    string           `json:"model"`
						Duration int              `json:"duration"`
						Content  []map[string]any `json:"content"`
					}
					if json.NewDecoder(r.Body).Decode(&body) != nil || body.Model != modelID || body.Duration != 5 || len(body.Content) != 1 || body.Content[0]["text"] != "a cat" {
						t.Error("video request changed")
					}
					fmt.Fprint(w, `{"id":"cgt-fixture"}`)
				case "GET /prefix/api/v3/contents/generations/tasks/cgt-fixture":
					count := gets.Add(1)
					if count == 1 {
						fmt.Fprint(w, `{"status":"queued"}`)
						return
					}
					if count == 2 {
						fmt.Fprint(w, `{"status":"running"}`)
						return
					}
					if tc.tokens && count == 3 {
						fmt.Fprint(w, `{"status":"succeeded","content":{"video_url":"https://example.test/video.mp4"},"usage":{"completion_tokens":123,"total_tokens":124}}`)
						return
					}
					usage := ""
					if tc.tokens {
						usage = `,"usage":{"completion_tokens":123,"total_tokens":123}`
					}
					fmt.Fprintf(w, `{"status":%q,"duration":4,"content":{"video_url":"https://example.test/video.mp4"}%s}`, tc.status, usage)
				default:
					t.Error("unexpected video path")
					w.WriteHeader(404)
				}
			}))
			defer upstream.Close()
			ch := &model.Channel{Name: "Ark video", Type: channel.ChannelTypeArk, BaseURL: upstream.URL + "/prefix/api/v3", Key: "video-key", Models: modelID, Group: "default", Status: 1, Weight: 1}
			if err := model.InsertChannel(ch); err != nil {
				t.Fatal(err)
			}
			if err := model.UpsertModelMeta(&model.ModelMeta{Model: modelID, Tags: "video"}); err != nil {
				t.Fatal(err)
			}
			u := newUser(t, "ark-video", common.RoleUser, 1000000)
			token := mustToken(t, u.ID, 1000000)
			r := httptest.NewRequest("POST", "/v1/video/generations", strings.NewReader(`{"model":"`+modelID+`","prompt":"a cat","seconds":5,"n":1}`))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Authorization", "Bearer "+token.PlaintextKey())
			w := httptest.NewRecorder()
			middleware.TokenAuth(middleware.Distribute(dto.InboundOpenAI)(http.HandlerFunc(RelayTaskSubmitVideo))).ServeHTTP(w, r)
			var submitted dto.TaskSubmitResponse
			if json.Unmarshal(w.Body.Bytes(), &submitted) != nil || w.Code != 200 || submitted.ID == "" {
				t.Fatalf("video admission: %d %s", w.Code, w.Body)
			}
			id, err := strconv.ParseInt(submitted.ID, 10, 64)
			if err != nil {
				t.Fatal(err)
			}
			task, err := model.GetTaskByID(id)
			if err != nil {
				t.Fatal(err)
			}
			if task.Platform != "ark-video" || task.Model != modelID || task.Quota <= tc.want || task.KeyIndex != 0 {
				t.Fatalf("invalid admission: %+v", task)
			}
			changed := *ch
			changed.Key = "replacement-key"
			if err := model.UpdateChannel(&changed); !errors.Is(err, model.ErrTaskPluginChannelBusy) {
				t.Fatalf("active task lost its credential: %v", err)
			}
			if err := model.DeleteChannel(ch.ID); !errors.Is(err, model.ErrTaskPluginChannelBusy) {
				t.Fatalf("active video channel was deleted: %v", err)
			}
			for range 2 {
				if _, err := service.PollTask(context.Background(), task); err != nil {
					t.Fatal(err)
				}
			}
			if task.Status != dto.TaskStatusInProgress {
				t.Fatalf("running status not mapped: %+v", task)
			}
			if tc.tokens {
				if _, err := service.PollTask(context.Background(), task); err == nil {
					t.Fatal("malformed usage was accepted")
				}
				if task.Status != dto.TaskStatusInProgress || task.PendingStatus != "" {
					t.Fatal("malformed usage finalized the task")
				}
				if count, _ := model.CountLogs(u.ID); count != 0 {
					t.Fatal("malformed usage created a zero bill")
				}
			}
			if err := setting.ApplyOptions(map[string]string{setting.OptionKeyModelPrice: `{"` + modelID + `":9}`, setting.OptionKeyModelTimeTokenPrices: "{}"}); err != nil {
				t.Fatal(err)
			}
			if tc.recover {
				if err := model.DB.Exec(`CREATE TRIGGER reject_ark_video_log BEFORE INSERT ON logs BEGIN SELECT RAISE(ABORT,'injected'); END`).Error; err != nil {
					t.Fatal(err)
				}
				if _, err := service.PollTask(context.Background(), task); err == nil {
					t.Fatal("injected settlement failure ignored")
				}
				stored, _ := model.GetTaskByID(task.ID)
				if stored.PendingQuota != tc.want || stored.PendingStatus != "succeeded" {
					t.Fatal("settlement intent lost")
				}
				if err := model.DB.Exec(`DROP TRIGGER reject_ark_video_log`).Error; err != nil {
					t.Fatal(err)
				}
			}
			for range 2 {
				if _, err := service.PollTask(context.Background(), task); err != nil {
					t.Fatal(err)
				}
			}
			wantStatus := tc.status
			if tc.status == "expired" {
				wantStatus = "failed"
			}
			wantGets := int32(3)
			if tc.tokens {
				wantGets++
			}
			if task.Status != wantStatus || posts.Load() != 1 || gets.Load() != wantGets {
				t.Fatalf("lifecycle duplicated or lost: status=%s posts=%d gets=%d", task.Status, posts.Load(), gets.Load())
			}
			logs, total, err := model.ListLogs(u.ID, 1, 10, modelID)
			if err != nil || total != 1 || common.QuotaMoney(logs[0].Quota, logs[0].QuotaRemainderNano).Int64() != tc.want {
				t.Fatalf("invalid settlement: %+v %v", logs, err)
			}
			if tc.tokens && (logs[0].PromptTokens != 0 || logs[0].CompletionTokens != 123 || logs[0].UsageSource != "upstream") {
				t.Fatal("video usage was billed as text")
			}
			after, _ := model.GetUserByID(u.ID)
			if common.QuotaMoney(after.Quota, after.QuotaRemainderNano).Int64() != common.QuotaMoney(u.Quota, u.QuotaRemainderNano).Int64()-tc.want {
				t.Fatal("reservation did not reconcile")
			}
			if err := model.VerifyMoneyLedger(u.ID); err != nil {
				t.Fatal(err)
			}
			if err := model.UpdateChannel(&changed); err != nil {
				t.Fatalf("terminal task still blocks credential maintenance: %v", err)
			}
		})
	}
}

func TestArkVideoSubmissionRejectsInvalidAndUncertainRequests(t *testing.T) {
	for _, invalid := range []bool{true, false} {
		t.Run(fmt.Sprint(invalid), func(t *testing.T) {
			newTestDB(t)
			const modelID = "doubao-seedance-2-0-260128"
			if err := setting.UpdatePricing(setting.OptionKeyModelPrice, `{"`+modelID+`":0.01}`); err != nil {
				t.Fatal(err)
			}
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				fmt.Fprint(w, `{"status":"accepted-without-id"}`)
			}))
			defer upstream.Close()
			for i := range 2 {
				if err := model.InsertChannel(&model.Channel{Name: fmt.Sprint(i), Type: channel.ChannelTypeArk, Key: "video-key", BaseURL: upstream.URL, Models: modelID, Group: "default", Status: 1, Weight: 1}); err != nil {
					t.Fatal(err)
				}
			}
			if err := model.UpsertModelMeta(&model.ModelMeta{Model: modelID, Tags: "video"}); err != nil {
				t.Fatal(err)
			}
			u := newUser(t, "ark-submit", common.RoleUser, 1000000)
			token := mustToken(t, u.ID, 1000000)
			n := 1
			if invalid {
				n = 2
			}
			r := httptest.NewRequest("POST", "/v1/video/generations", strings.NewReader(fmt.Sprintf(`{"model":%q,"prompt":"cat","seconds":5,"n":%d}`, modelID, n)))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Authorization", "Bearer "+token.PlaintextKey())
			w := httptest.NewRecorder()
			middleware.TokenAuth(middleware.Distribute(dto.InboundOpenAI)(http.HandlerFunc(RelayTaskSubmitVideo))).ServeHTTP(w, r)
			wantStatus, wantCalls := 502, int32(1)
			if invalid {
				wantStatus, wantCalls = 400, 0
			}
			if w.Code != wantStatus || calls.Load() != wantCalls {
				t.Fatalf("unsafe submit: status=%d calls=%d body=%s", w.Code, calls.Load(), w.Body)
			}
			after, _ := model.GetUserByID(u.ID)
			if after.Quota != u.Quota || after.QuotaRemainderNano != u.QuotaRemainderNano || after.UsedQuota != 0 {
				t.Fatal("failed admission left a charge")
			}
		})
	}
}

func TestArkVideoAdmissionRejectsChangedConnection(t *testing.T) {
	newTestDB(t)
	ch := &model.Channel{Name: "Ark changed", Type: channel.ChannelTypeArk, Key: "old-key", BaseURL: "https://example.test", Models: "ep-fixture", Group: "default", Status: 1}
	if err := model.InsertChannel(ch); err != nil {
		t.Fatal(err)
	}
	expected := *ch
	ch.Key = "new-key"
	if err := model.UpdateChannel(ch); err != nil {
		t.Fatal(err)
	}
	task := &model.Task{ChannelID: ch.ID, Model: "ep-fixture", Group: "default", Status: model.TaskStatusSubmitting}
	if err := model.InsertTaskForChannel(task, &expected); err == nil {
		t.Fatal("stale connection admitted a video task")
	}
	var count int64
	if err := model.DB.Model(&model.Task{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("stale admission left a durable task")
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

func TestArkRejectsShortNames(t *testing.T) {
	newTestDB(t)
	for _, name := range []string{"doubao-seedream-4.5", "doubao-seedream-5.0", "doubao-seedance-2.0", "doubao-seedance-2.0-fast"} {
		expectError(t, call(CreateChannel, testRequest{method: "POST", target: "/api/channel", body: fmt.Sprintf(`{"type":32,"models":%q}`, name)}), 400, "ark_model_version_required")
	}
	ch := &model.Channel{Type: channel.ChannelTypeArk, Models: "doubao-seedream-4.5", Group: "default", Status: 1}
	if err := model.InsertChannel(ch); err != nil {
		t.Fatal(err)
	}
	expectError(t, call(UpdateChannel, testRequest{method: "PUT", target: "/api/channel", body: fmt.Sprintf(`{"id":%d,"models":"doubao-seedream-4.5"}`, ch.ID)}), 400, "ark_model_version_required")
	expectStatus(t, call(UpdateChannel, testRequest{method: "PUT", target: "/api/channel", body: fmt.Sprintf(`{"id":%d,"status":2}`, ch.ID)}), 200)
	for _, name := range []string{"doubao-seedream-4-5-251128", "doubao-seedance-2-0-260128", "doubao-seedance-2-0-fast-260128", "ep-fixture"} {
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
