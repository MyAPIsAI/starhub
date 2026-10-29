package controller

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
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
)

func responsesTestHandler() http.Handler {
	return middleware.TokenAuth(middleware.Distribute(dto.InboundOpenAI)(http.HandlerFunc(RelayResponses)))
}

func responsesTestRequest(key, body string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer "+key)
	return r
}

func responsesTestChannel(t *testing.T, url string, kind int) *model.Channel {
	t.Helper()
	ch := &model.Channel{Name: "responses", Type: kind, Key: "supplier-key", BaseURL: url, Models: "gpt-4o", Group: "default", Status: common.ChannelStatusEnabled, Weight: 1}
	if err := model.InsertChannel(ch); err != nil {
		t.Fatal(err)
	}
	return ch
}

func assertResponsesBill(t *testing.T, user *model.User, token *model.Token, prompt, completion int64, status int) {
	t.Helper()
	logs, total, err := model.ListLogs(user.ID, 1, 10, "")
	if err != nil || total != 1 || len(logs) != 1 {
		t.Fatalf("bill count: %d %+v %v", total, logs, err)
	}
	entry := logs[0]
	if entry.PromptTokens != prompt || entry.CompletionTokens != completion || entry.Status != status || entry.Protocol != "OpenAI" {
		t.Fatalf("bill metadata: %+v", entry)
	}
	afterUser, err := model.GetUserByID(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	afterToken, err := model.GetTokenByID(token.ID)
	if err != nil {
		t.Fatal(err)
	}
	if afterUser.Quota != user.Quota-entry.Quota || afterUser.UsedQuota != entry.Quota || afterToken.RemainQuota != token.RemainQuota-entry.Quota || afterToken.UsedQuota != entry.Quota {
		t.Fatalf("reservation or settlement mismatch: log=%+v user=%+v token=%+v", entry, afterUser, afterToken)
	}
}

func TestResponsesNativeJSONPreservesWireAndBillsDetails(t *testing.T) {
	newTestDB(t)
	requestBody := `{"model":"gpt-4o","input":"hello","max_output_tokens":16,"extension":{"sequence":9007199254740993}}`
	responseBody := `{"id":"resp_1","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hello"}]}],"extension":9007199254740993,"usage":{"input_tokens":20,"output_tokens":10,"input_tokens_details":{"cached_tokens":8},"output_tokens_details":{"reasoning_tokens":3}}}`
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		body, _ := io.ReadAll(r.Body)
		if r.URL.Path != "/v1/responses" || r.Header.Get("Authorization") != "Bearer supplier-key" || string(body) != requestBody {
			t.Errorf("native request changed: %s %s %s", r.URL, r.Header.Get("Authorization"), body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, responseBody)
	}))
	defer upstream.Close()
	responsesTestChannel(t, upstream.URL+"/v1/", channel.ChannelTypeOpenAI)
	user := newUser(t, "responses-json", common.RoleUser, 1000000)
	token := mustToken(t, user.ID, 1000000)
	rec := httptest.NewRecorder()
	responsesTestHandler().ServeHTTP(rec, responsesTestRequest(token.Key, requestBody))
	if rec.Code != 200 || rec.Body.String() != responseBody || calls.Load() != 1 {
		t.Fatalf("native response changed: %d %s calls=%d", rec.Code, rec.Body, calls.Load())
	}
	assertResponsesBill(t, user, token, 20, 10, 1)
	logs, _, _ := model.ListLogs(user.ID, 1, 10, "")
	usage, _ := dto.ParseResponsesUsage([]byte(responseBody))
	want := relay.ComputeUsageQuota(relay.NewPriceDataWithUserGroup("gpt-4o", "default", "default"), usage)
	if logs[0].Quota != want {
		t.Fatalf("detail-aware bill = %d, want %d", logs[0].Quota, want)
	}
}

func TestResponsesNativeStreamFlushesAndPreservesFraming(t *testing.T) {
	newTestDB(t)
	first := ": heartbeat\r\nid: 9007199254740993\r\nretry: 123\r\nevent: response.created\r\ndata: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_1\",\"status\":\"in_progress\"}}\r\n\r\n"
	tail := "event: vendor.extension\ndata: {\"type\":\"vendor.extension\",\"n\":9007199254740993}\n\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\nevent: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"status\":\"completed\",\"usage\":{\"input_tokens\":15,\"output_tokens\":7}}}\n\n"
	release := make(chan struct{})
	defer close(release)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), "stream_options") {
			t.Error("Responses request acquired Chat stream_options")
		}
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		_, _ = io.WriteString(w, first)
		w.(http.Flusher).Flush()
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		_, _ = io.WriteString(w, tail)
	}))
	defer upstream.Close()
	responsesTestChannel(t, upstream.URL, channel.ChannelTypeOpenAI)
	user := newUser(t, "responses-stream", common.RoleUser, 1000000)
	token := mustToken(t, user.ID, 1000000)
	done := make(chan struct{})
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		responsesTestHandler().ServeHTTP(w, r)
		close(done)
	}))
	defer gateway.Close()
	request, _ := http.NewRequest(http.MethodPost, gateway.URL+"/v1/responses", strings.NewReader(`{"model":"gpt-4o","input":"hello","stream":true,"max_output_tokens":16}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+token.Key)
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	reader := bufio.NewReader(resp.Body)
	buf := make([]byte, len(first))
	if _, err := io.ReadFull(reader, buf); err != nil || string(buf) != first {
		t.Fatalf("early flush lost: %q %v", buf, err)
	}
	release <- struct{}{}
	rest, err := io.ReadAll(reader)
	if err != nil || string(rest) != tail || resp.StatusCode != 200 || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("stream changed: %q %v", rest, err)
	}
	<-done
	assertResponsesBill(t, user, token, 15, 7, 1)
}

func TestResponsesOutcomesAndMissingUsage(t *testing.T) {
	for _, tc := range []struct {
		name, response string
		stream         bool
		prompt, output int64
		status         int
	}{
		{"zero usage", `{"id":"r","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"not billed"}]}],"usage":{"input_tokens":0,"output_tokens":0}}`, false, 0, 0, 1},
		{"text fallback", `{"id":"r","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"12345678"}]}]}`, false, 2, 2, 1},
		{"failed free", `{"id":"r","status":"failed","output":[]}`, false, 0, 0, 2},
		{"failed measured", `{"id":"r","status":"failed","usage":{"input_tokens":3,"output_tokens":1}}`, false, 3, 1, 2},
		{"cancelled measured", `{"id":"r","status":"cancelled","usage":{"input_tokens":3,"output_tokens":1}}`, false, 3, 1, 3},
		{"incomplete length", `{"id":"r","status":"incomplete","usage":{"input_tokens":3,"output_tokens":1}}`, false, 3, 1, 1},
		{"broken stream partial", "data: {\"type\":\"response.output_text.delta\",\"delta\":\"12345678\"}\n\n", true, 2, 2, 2},
		{"stream failure", "data: {\"type\":\"response.failed\",\"response\":{\"id\":\"r\",\"status\":\"failed\",\"usage\":{\"input_tokens\":3,\"output_tokens\":1}}}\n\n", true, 3, 1, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			newTestDB(t)
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				contentType := "application/json"
				if tc.stream {
					contentType = "text/event-stream"
				}
				w.Header().Set("Content-Type", contentType)
				_, _ = io.WriteString(w, tc.response)
			}))
			defer upstream.Close()
			responsesTestChannel(t, upstream.URL, channel.ChannelTypeOpenAI)
			responsesTestChannel(t, upstream.URL, channel.ChannelTypeOpenAI)
			user := newUser(t, "responses-outcome", common.RoleUser, 1000000)
			token := mustToken(t, user.ID, 1000000)
			rec := httptest.NewRecorder()
			body := fmt.Sprintf(`{"model":"gpt-4o","input":"12345678","max_output_tokens":16,"stream":%t}`, tc.stream)
			responsesTestHandler().ServeHTTP(rec, responsesTestRequest(token.Key, body))
			if rec.Code != 200 || rec.Body.String() != tc.response || calls.Load() != 1 {
				t.Fatalf("outcome changed or retried: %d %s calls=%d", rec.Code, rec.Body, calls.Load())
			}
			assertResponsesBill(t, user, token, tc.prompt, tc.output, tc.status)
		})
	}
}

func TestResponsesRejectUnsupportedBeforeAdmission(t *testing.T) {
	for _, field := range []string{
		`"background":true`, `"previous_response_id":"other-user-id"`, `"conversation":"other-user-id"`,
		`"input":[{"role":"user","content":[{"type":"input_file","file_id":"other-user-file"}]}]`,
		`"tools":[{"type":"web_search"}]`, `"max_output_tokens":100001`,
	} {
		t.Run(field, func(t *testing.T) {
			newTestDB(t)
			responsesTestChannel(t, "http://127.0.0.1:1", channel.ChannelTypeOpenAI)
			user := newUser(t, "responses-unsupported", common.RoleUser, 1000000)
			token := mustToken(t, user.ID, 1000000)
			rec := httptest.NewRecorder()
			responsesTestHandler().ServeHTTP(rec, responsesTestRequest(token.Key, `{"model":"gpt-4o",`+field+`}`))
			want := 400
			if strings.Contains(field, "previous_response_id") || strings.Contains(field, "conversation") {
				want = 404
			}
			if rec.Code != want {
				t.Fatalf("unsupported request admitted: %d %s", rec.Code, rec.Body)
			}
			after, _ := model.GetUserByID(user.ID)
			count, _ := model.CountLogs(user.ID)
			if after.Quota != user.Quota || after.UsedQuota != 0 || count != 0 {
				t.Fatal("rejected request changed quota or created bill")
			}
		})
	}
}

func TestResponsesNoQuotaDoesNotPinOrDispatch(t *testing.T) {
	newTestDB(t)
	responsesTestChannel(t, "http://127.0.0.1:1", channel.ChannelTypeOpenAI)
	if err := model.UpsertModelMeta(&model.ModelMeta{Model: "gpt-4o", Tags: "responses"}); err != nil {
		t.Fatal(err)
	}
	user := newUser(t, "responses-no-quota", common.RoleUser, 0)
	token := mustToken(t, user.ID, 1000000)
	rec := httptest.NewRecorder()
	responsesTestHandler().ServeHTTP(rec, responsesTestRequest(token.Key, `{"model":"myapis-auto","input":"hello","max_output_tokens":16}`))
	var count int64
	if err := model.DB.Model(&model.RoutingChain{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if rec.Code != 403 || count != 0 {
		t.Fatalf("unfunded request admitted: %d %s chains=%d", rec.Code, rec.Body, count)
	}
}

func TestResponsesSmartAdmissionForwardsConcreteModel(t *testing.T) {
	newTestDB(t)
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"model":"gpt-4o"`) || !strings.Contains(string(body), `9007199254740993`) || strings.Contains(string(body), "myapis-auto") {
			t.Errorf("smart request lost concrete model or precision: %s", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"resp_1","status":"completed","usage":{"input_tokens":10,"output_tokens":2}}`)
	}))
	defer upstream.Close()
	responsesTestChannel(t, upstream.URL, channel.ChannelTypeOpenAI)
	if err := model.UpsertModelMeta(&model.ModelMeta{Model: "gpt-4o", Tags: "responses"}); err != nil {
		t.Fatal(err)
	}
	user := newUser(t, "responses-smart", common.RoleUser, 1000000)
	token := mustToken(t, user.ID, 1000000)
	rec := httptest.NewRecorder()
	responsesTestHandler().ServeHTTP(rec, responsesTestRequest(token.Key, `{"model":"myapis-auto","input":"hello","max_output_tokens":16,"extension":9007199254740993}`))
	var chains []model.RoutingChain
	if err := model.DB.Find(&chains).Error; err != nil {
		t.Fatal(err)
	}
	if rec.Code != 200 || calls.Load() != 1 || len(chains) != 1 || chains[0].Model != "gpt-4o" {
		t.Fatalf("admission failed: %d %s calls=%d chains=%+v", rec.Code, rec.Body, calls.Load(), chains)
	}
	assertResponsesBill(t, user, token, 10, 2, 1)
	logs, _, _ := model.ListLogs(user.ID, 1, 10, "")
	if logs[0].RequestedModel != "myapis-auto" || logs[0].Model != "gpt-4o" {
		t.Fatalf("model attribution lost: %+v", logs[0])
	}
}
