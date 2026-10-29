package controller

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"starhub/common"
	"starhub/middleware"
	"starhub/model"
	"starhub/relay"
	"starhub/relay/channel"
	"starhub/relay/dto"
	"starhub/setting"
)

type responseReadFixture struct {
	owner                              *model.User
	token                              *model.Token
	channel                            *model.Channel
	body, wire, request                string
	posts, gets                        atomic.Int64
	postKey, getKey, getQuery, getBody atomic.Value
	getStatus                          atomic.Int64
}

func newResponseReadFixture(t *testing.T, stream bool, requestStore, providerStore string) *responseReadFixture {
	t.Helper()
	newTestDB(t)
	f := &responseReadFixture{}
	f.body = fmt.Sprintf(`{"id":"resp_scoped","status":"completed","store":%s,"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"private-owner-output"}]},{"type":"function_call","call_id":"call_1","name":"lookup","arguments":"{}"},{"type":"reasoning","summary":[{"type":"summary_text","text":"local"}],"encrypted_content":"opaque"}],"extension":9007199254740993,"usage":{"input_tokens":100,"output_tokens":20}}`, providerStore)
	f.wire = f.body
	if stream {
		f.wire = ": heartbeat\r\nid: 9007199254740993\r\nevent: response.created\r\ndata: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_scoped\",\"status\":\"in_progress\"}}\r\n\nevent: response.completed\ndata: {\"type\":\"response.completed\",\"response\":" + f.body + "}\n\n"
	}
	store := ""
	if requestStore != "" {
		store = `,"store":` + requestStore
	}
	f.request = fmt.Sprintf(`{"model":"gpt-4o","input":"local","max_output_tokens":128,"stream":%t%s}`, stream, store)
	f.getBody.Store(f.body)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			f.posts.Add(1)
			f.postKey.Store(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
			body, _ := io.ReadAll(r.Body)
			if r.URL.Path != "/v1/responses" || string(body) != f.request {
				t.Error("native create request changed")
			}
			kind := "application/json"
			if stream {
				kind = "text/event-stream"
			}
			w.Header().Set("Content-Type", kind)
			_, _ = io.WriteString(w, f.wire)
			return
		}
		f.gets.Add(1)
		f.getKey.Store(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		f.getQuery.Store(r.URL.RawQuery)
		if r.Method != http.MethodGet || r.URL.Path != "/v1/responses/resp_scoped" || r.Header.Get("Cookie") != "" {
			t.Error("retrieval used a different resource or leaked client headers")
		}
		status := int(f.getStatus.Load())
		if status == 0 {
			status = 200
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, f.getBody.Load().(string))
	}))
	t.Cleanup(upstream.Close)
	f.channel = responsesTestChannel(t, upstream.URL, channel.ChannelTypeOpenAI)
	f.owner = newUser(t, "response-read-owner", common.RoleUser, 1_000_000)
	f.token = mustToken(t, f.owner.ID, 1_000_000)
	return f
}

func (f *responseReadFixture) create(t *testing.T) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	responsesTestHandler().ServeHTTP(rec, responsesTestRequest(f.token.Key, f.request))
	if rec.Code != 200 || rec.Body.String() != f.wire || f.posts.Load() != 1 {
		t.Fatalf("native create changed: %d %s", rec.Code, rec.Body)
	}
	assertResponsesBill(t, f.owner, f.token, 100, 20, 1)
	current, err := model.GetChannelByID(f.channel.ID)
	if err != nil {
		t.Fatal(err)
	}
	*f.channel = *current
	return rec
}

func readStoredResponse(key, id, query string) *httptest.ResponseRecorder {
	path := "/v1/responses/" + url.PathEscape(id)
	if query != "" {
		path += "?" + query
	}
	r := httptest.NewRequest(http.MethodGet, path, nil)
	r.SetPathValue("id", id)
	if key != "" {
		r.Header.Set("Authorization", "Bearer "+key)
	}
	r.Header.Set("Cookie", "client-only=synthetic")
	rec := httptest.NewRecorder()
	middleware.TokenReadOnlyAuth(http.HandlerFunc(RetrieveResponse)).ServeHTTP(rec, r)
	return rec
}

func TestStoredResponseRetrievalPinsAccountPreservesWireAndDoesNotRebill(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream%t", stream), func(t *testing.T) {
			f := newResponseReadFixture(t, stream, "true", "true")
			f.channel.Key = "first-account\nsecond-account"
			f.channel.KeyMode = model.KeyModeRoundRobin
			if err := model.UpdateChannel(f.channel); err != nil {
				t.Fatal(err)
			}
			created := f.create(t)
			state := created.Header().Get("X-StarHub-Response-Reference")
			if stream {
				state = created.Result().Trailer.Get("X-StarHub-Response-Reference")
			}
			if state != "recorded" {
				t.Fatalf("reference receipt = %q", state)
			}
			ref, err := model.LookupResponseReference(context.Background(), f.owner.ID, f.token.ID, "resp_scoped", time.Now())
			if err != nil || !ref.HasTools || !ref.HasReasoning || ref.PromptTokens != 100 || ref.CompletionTokens != 20 {
				t.Fatalf("captured terminal binding: %+v %v", ref, err)
			}
			f.channel.Key = "second-account\nfirst-account"
			f.channel.KeyMode = model.KeyModeRandom
			if err := model.UpdateChannel(f.channel); err != nil {
				t.Fatal(err)
			}
			if err := model.DB.Model(&model.User{}).Where("id = ?", f.owner.ID).Update("quota", 0).Error; err != nil {
				t.Fatal(err)
			}
			if err := model.DB.Model(&model.Token{}).Where("id = ?", f.token.ID).Update("remain_quota", 0).Error; err != nil {
				t.Fatal(err)
			}
			beforeUser, _ := model.GetUserByID(f.owner.ID)
			beforeToken, _ := model.GetTokenByID(f.token.ID)
			beforeChannel, _ := model.GetChannelByID(f.channel.ID)
			read := readStoredResponse(f.token.Key, "resp_scoped", "include=message.output_text.logprobs&include%5B%5D=reasoning.encrypted_content&stream=false")
			if read.Code != 200 || read.Body.String() != f.body || read.Header().Get("Cache-Control") != "private, no-store" || f.getKey.Load() != f.postKey.Load() || f.gets.Load() != 1 {
				t.Fatalf("original-account read changed: %d %s", read.Code, read.Body)
			}
			if !strings.Contains(f.getQuery.Load().(string), "reasoning.encrypted_content") {
				t.Fatal("validated retrieve options disappeared")
			}
			afterUser, _ := model.GetUserByID(f.owner.ID)
			afterToken, _ := model.GetTokenByID(f.token.ID)
			afterChannel, _ := model.GetChannelByID(f.channel.ID)
			_, count, _ := model.ListLogs(f.owner.ID, 1, 10, "")
			if afterUser.Quota != 0 || afterUser.UsedQuota != beforeUser.UsedQuota || afterToken.RemainQuota != 0 || afterToken.UsedQuota != beforeToken.UsedQuota || afterChannel.UsedQuota != beforeChannel.UsedQuota || count != 1 {
				t.Fatal("read-only retrieval moved generation quota or produced a second bill")
			}
			denied := httptest.NewRecorder()
			responsesTestHandler().ServeHTTP(denied, responsesTestRequest(f.token.Key, f.request))
			if denied.Code != 403 || f.posts.Load() != 1 {
				t.Fatal("generation authorization also lost its quota requirement")
			}
		})
	}
}

func TestStoredResponseRetrievalRejectsOtherOwnersAndKeys(t *testing.T) {
	f := newResponseReadFixture(t, false, "", "true")
	f.create(t)
	other := newUser(t, "response-read-other", common.RoleRoot, 1_000_000)
	otherToken := mustToken(t, other.ID, 0)
	sameOwner := mustToken(t, f.owner.ID, 0)
	for _, key := range []string{otherToken.Key, sameOwner.Key} {
		read := readStoredResponse(key, "resp_scoped", "")
		if read.Code != 404 || strings.Contains(read.Body.String(), "private-owner-output") {
			t.Fatal("foreign or same-owner different key retrieved a response")
		}
	}
	if read := readStoredResponse("", "resp_scoped", ""); read.Code != 401 || read.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatal("anonymous retrieval was allowed or cacheable")
	}
	for _, id := range []string{"resp_unknown", "../other", "bad?secret=value"} {
		if read := readStoredResponse(f.token.Key, id, ""); read.Code != 404 {
			t.Fatal("unknown or malformed IDs bypassed ownership lookup")
		}
	}
	if f.gets.Load() != 0 {
		t.Fatal("unowned IDs reached the upstream")
	}
}

func TestStoredResponseRetrievalHonorsCurrentScopeAndExactUpstream(t *testing.T) {
	cases := []struct {
		name   string
		status int
		change func(*testing.T, *responseReadFixture)
	}{
		{"channel-disabled", 409, func(t *testing.T, f *responseReadFixture) {
			f.channel.Status = 2
			if err := model.UpdateChannel(f.channel); err != nil {
				t.Fatal(err)
			}
		}},
		{"key-removed", 409, func(t *testing.T, f *responseReadFixture) {
			f.channel.Key = "different-account"
			if err := model.UpdateChannel(f.channel); err != nil {
				t.Fatal(err)
			}
		}},
		{"key-disabled-in-pool", 409, func(t *testing.T, f *responseReadFixture) {
			f.channel.Key += "\nother-account"
			f.channel.MultiKeyStatus = "2,1"
			if err := model.UpdateChannel(f.channel); err != nil {
				t.Fatal(err)
			}
		}},
		{"base-url-changed", 409, func(t *testing.T, f *responseReadFixture) {
			f.channel.BaseURL = "https://another.example.invalid"
			if err := model.UpdateChannel(f.channel); err != nil {
				t.Fatal(err)
			}
		}},
		{"model-revoked", 404, func(t *testing.T, f *responseReadFixture) {
			if err := model.DB.Model(&model.Token{}).Where("id = ?", f.token.ID).Update("model_limits", "other-model").Error; err != nil {
				t.Fatal(err)
			}
		}},
		{"token-disabled", 401, func(t *testing.T, f *responseReadFixture) {
			if err := model.DB.Model(&model.Token{}).Where("id = ?", f.token.ID).Update("status", 2).Error; err != nil {
				t.Fatal(err)
			}
		}},
		{"token-expired", 401, func(t *testing.T, f *responseReadFixture) {
			if err := model.DB.Model(&model.Token{}).Where("id = ?", f.token.ID).Update("expired_time", time.Now().Unix()-1).Error; err != nil {
				t.Fatal(err)
			}
		}},
		{"owner-disabled", 401, func(t *testing.T, f *responseReadFixture) {
			if err := model.DB.Model(&model.User{}).Where("id = ?", f.owner.ID).Update("status", 2).Error; err != nil {
				t.Fatal(err)
			}
		}},
		{"ip-restricted", 403, func(t *testing.T, f *responseReadFixture) {
			if err := model.DB.Model(&model.Token{}).Where("id = ?", f.token.ID).Update("allow_ips", "203.0.113.1").Error; err != nil {
				t.Fatal(err)
			}
		}},
		{"reference-expired", 404, func(t *testing.T, f *responseReadFixture) {
			if err := model.DB.Model(&model.ResponseReference{}).Where("response_id = ?", "resp_scoped").Update("expires_at", time.Now().Unix()).Error; err != nil {
				t.Fatal(err)
			}
		}},
		{"reference-ambiguous", 404, func(t *testing.T, f *responseReadFixture) {
			ref, err := model.LookupResponseReference(context.Background(), f.owner.ID, f.token.ID, "resp_scoped", time.Now())
			if err != nil {
				t.Fatal(err)
			}
			ref.ChannelID++
			if _, err := model.RecordResponseReference(context.Background(), *ref, time.Now()); err != nil {
				t.Fatal(err)
			}
		}},
		{"group-definition-replaced", 404, func(t *testing.T, f *responseReadFixture) {
			if err := model.DB.Where("group_id = ?", "default").Delete(&model.ServiceGroupConfig{}).Error; err != nil {
				t.Fatal(err)
			}
			if err := model.UpsertServiceGroup(&model.ServiceGroupConfig{GroupID: "default", Name: "New definition", Multiplier: "1", Visibility: "common", Enabled: true}); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newResponseReadFixture(t, false, "true", "true")
			f.create(t)
			tc.change(t, f)
			read := readStoredResponse(f.token.Key, "resp_scoped", "")
			if read.Code != tc.status || f.gets.Load() != 0 {
				t.Fatalf("changed scope reached upstream: %d %s calls%d", read.Code, read.Body, f.gets.Load())
			}
		})
	}
}

func TestStoredResponseIndexOptOutAndStorageFailurePreserveNativeBilling(t *testing.T) {
	for _, tc := range []struct {
		name, request, provider string
		fail                    bool
	}{{"request-opt-out", "false", "true", false}, {"provider-opt-out", "true", "false", false}, {"index-failure", "true", "true", true}} {
		t.Run(tc.name, func(t *testing.T) {
			f := newResponseReadFixture(t, false, tc.request, tc.provider)
			if tc.fail {
				if err := model.DB.Exec(`CREATE TRIGGER reject_reference BEFORE INSERT ON response_references BEGIN SELECT RAISE(ABORT, 'synthetic ownership-index failure'); END`).Error; err != nil {
					t.Fatal(err)
				}
			}
			created := f.create(t)
			want := "not_stored"
			if tc.fail {
				want = "unavailable"
			}
			if created.Header().Get("X-StarHub-Response-Reference") != want {
				t.Fatal("failed/opted-out index claimed persistence")
			}
			read := readStoredResponse(f.token.Key, "resp_scoped", "")
			if read.Code != 404 || f.gets.Load() != 0 {
				t.Fatal("missing ownership receipt was replaced by provider probing")
			}
		})
	}
}

func TestStoredResponseReusedIDsAcrossPoolAccountsStayUnavailable(t *testing.T) {
	f := newResponseReadFixture(t, false, "true", "true")
	f.channel.Key, f.channel.KeyMode = "first-account\nsecond-account", model.KeyModeRoundRobin
	if err := model.UpdateChannel(f.channel); err != nil {
		t.Fatal(err)
	}
	f.create(t)
	repeated := httptest.NewRecorder()
	responsesTestHandler().ServeHTTP(repeated, responsesTestRequest(f.token.Key, f.request))
	if repeated.Code != 200 || repeated.Body.String() != f.body || repeated.Header().Get("X-StarHub-Response-Reference") != "unavailable" || f.posts.Load() != 2 {
		t.Fatal("reused provider ID silently advertised another usable account binding")
	}
	read := readStoredResponse(f.token.Key, "resp_scoped", "")
	if read.Code != 404 || f.gets.Load() != 0 {
		t.Fatal("ambiguous provider ID was resolved by selecting a pool account")
	}
	_, count, err := model.ListLogs(f.owner.ID, 1, 10, "")
	if err != nil || count != 2 {
		t.Fatal("two actual generations did not retain their separate bills")
	}
}

func TestStoredResponseRetrievalHonorsFixedAndAutoGroupScopes(t *testing.T) {
	f := newResponseReadFixture(t, false, "true", "true")
	if err := setting.UpdateRatio(setting.OptionKeyGroupRatio, `{"default":1,"vip":1}`); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"default", "vip"} {
		if err := model.UpsertServiceGroup(&model.ServiceGroupConfig{GroupID: id, Name: id, Multiplier: "1", Visibility: "common", Enabled: true}); err != nil {
			t.Fatal(err)
		}
	}
	f.create(t)
	if err := model.DB.Model(&model.Token{}).Where("id = ?", f.token.ID).Update("group", "vip").Error; err != nil {
		t.Fatal(err)
	}
	if read := readStoredResponse(f.token.Key, "resp_scoped", ""); read.Code != 404 || f.gets.Load() != 0 {
		t.Fatal("different fixed group retained the historical grant")
	}
	if err := model.DB.Model(&model.Token{}).Where("id = ?", f.token.ID).Updates(map[string]any{"group": "auto", "auto_groups": "vip,default"}).Error; err != nil {
		t.Fatal(err)
	}
	if read := readStoredResponse(f.token.Key, "resp_scoped", ""); read.Code != 200 || f.gets.Load() != 1 {
		t.Fatalf("eligible original auto group lost read access: %d %s", read.Code, read.Body)
	}
	if err := model.DB.Model(&model.Token{}).Where("id = ?", f.token.ID).Update("auto_groups", "vip").Error; err != nil {
		t.Fatal(err)
	}
	if read := readStoredResponse(f.token.Key, "resp_scoped", ""); read.Code != 404 || f.gets.Load() != 1 {
		t.Fatal("removed auto group still fetched the old response")
	}
}

func TestStoredResponseRetrievalRejectsUnsafeQueriesAndBadProviderObjects(t *testing.T) {
	f := newResponseReadFixture(t, false, "true", "true")
	f.create(t)
	for _, query := range []string{"stream=true", "stream=false&stream=false", "include=bad%2Fpath", "include_obfuscation=1", "starting_after=0", "unknown=1", "include=" + strings.Repeat("x", 8200)} {
		read := readStoredResponse(f.token.Key, "resp_scoped", query)
		if read.Code != 400 || f.gets.Load() != 0 {
			t.Fatal("unsupported query reached upstream")
		}
	}
	for _, body := range []string{`{"id":"resp_other","status":"completed"}`, `{"id":"resp_scoped","status":"in_progress"}`, `not json`} {
		f.getBody.Store(body)
		read := readStoredResponse(f.token.Key, "resp_scoped", "")
		if read.Code != 502 || strings.Contains(read.Body.String(), "resp_other") {
			t.Fatal("invalid or nonterminal provider object was exposed")
		}
	}
	f.getStatus.Store(404)
	if read := readStoredResponse(f.token.Key, "resp_scoped", ""); read.Code != 404 {
		t.Fatal("provider expiration was concealed")
	}
	f.getStatus.Store(500)
	if read := readStoredResponse(f.token.Key, "resp_scoped", ""); read.Code != 502 {
		t.Fatal("provider read failure was hidden")
	}
	_, count, _ := model.ListLogs(f.owner.ID, 1, 10, "")
	if count != 1 || f.posts.Load() != 1 {
		t.Fatal("failed retrieval generated another response or bill")
	}
}

func TestResponseObservationSurvivesCompletedClientCancellationAndReadAuthIsMethodBounded(t *testing.T) {
	f := newResponseReadFixture(t, false, "true", "true")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	flow := &relayFlow{ctx: ctx, req: relayRequest{userID: f.owner.ID, tokenID: f.token.ID, modelName: "gpt-4o", group: "default"}, currentGroup: "default", startTime: time.Now()}
	capture, err := captureResponseReference(flow, f.channel, "supplier-key", &dto.ResponsesRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if state := observeResponseReference(flow, capture, responseAccounting{ID: "resp_cancelled_client", Status: "completed"}, &relay.Usage{PromptTokens: 1}); state != "recorded" {
		t.Fatal("completed response ownership was lost on client disconnect")
	}
	rec := httptest.NewRecorder()
	request := responsesTestRequest(f.token.Key, f.request)
	middleware.TokenReadOnlyAuth(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("read-only auth admitted generation") })).ServeHTTP(rec, request)
	if rec.Code != 405 {
		t.Fatal("read-only auth accepted a non-read method")
	}
}

func TestResponseIDCollisionAcrossOwnersOrDuplicateChannelsNeverFetchesPrivateContent(t *testing.T) {
	for _, duplicate := range []bool{false, true} {
		t.Run(fmt.Sprintf("duplicateChannel%t", duplicate), func(t *testing.T) {
			f := newResponseReadFixture(t, false, "true", "true")
			f.create(t)
			secondOwner := newUser(t, "second-response-owner", common.RoleUser, 1_000_000)
			secondToken := mustToken(t, secondOwner.ID, 1_000_000)
			if duplicate {
				clone := *f.channel
				clone.ID, clone.UsedQuota = 0, 0
				if err := model.InsertChannel(&clone); err != nil {
					t.Fatal(err)
				}
				f.channel.Status = common.ChannelStatusManuallyDisabled
				if err := model.UpdateChannel(f.channel); err != nil {
					t.Fatal(err)
				}
			}
			created := httptest.NewRecorder()
			responsesTestHandler().ServeHTTP(created, responsesTestRequest(secondToken.Key, f.request))
			if created.Code != 200 || created.Body.String() != f.wire || created.Header().Get("X-StarHub-Response-Reference") != "unavailable" {
				t.Fatalf("colliding creation was not preserved with an unavailable index: %d %s", created.Code, created.Body)
			}
			for _, key := range []string{f.token.Key, secondToken.Key} {
				if read := readStoredResponse(key, "resp_scoped", ""); read.Code != 404 {
					t.Fatalf("colliding ownership fetched private upstream content: %d %s", read.Code, read.Body)
				}
			}
			if f.gets.Load() != 0 || f.posts.Load() != 2 {
				t.Fatal("collision refusal performed another upstream operation")
			}
			assertResponsesBill(t, f.owner, f.token, 100, 20, 1)
			assertResponsesBill(t, secondOwner, secondToken, 100, 20, 1)
		})
	}
}
