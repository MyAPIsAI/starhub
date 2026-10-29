package controller

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"starhub/common"
	"starhub/model"
)

func TestResponsesAmbiguousFieldsNeverReachProviderOrBilling(t *testing.T) {
	newTestDB(t)
	calls := 0
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"resp_owner_a","model":"gpt-4o","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1}}`)
	}))
	defer up.Close()
	responsesTestChannel(t, up.URL, 1)
	a := newUser(t, "response-owner-a", common.RoleUser, 1000000)
	at := mustToken(t, a.ID, 1000000)
	b := newUser(t, "response-owner-b", common.RoleUser, 1000000)
	bt := mustToken(t, b.ID, 1000000)
	if err := model.DB.Model(&model.Token{}).Where("id = ?", bt.ID).Update("model_limits", "gpt-4o").Error; err != nil {
		t.Fatal(err)
	}
	first := httptest.NewRecorder()
	responsesTestHandler().ServeHTTP(first, responsesTestRequest(at.Key, `{"model":"gpt-4o","input":"private"}`))
	if first.Code != 200 || calls != 1 {
		t.Fatalf("setup: %d %s", first.Code, first.Body)
	}
	for _, path := range []string{"/v1/responses", "/v1/responses/compact"} {
		for _, body := range []string{
			`{"model":"unauthorized","Model":"gpt-4o","input":"hello"}`,
			`{"model":"gpt-4o","input":"repeat","previous_response_id":"resp_owner_a","Previous_response_id":""}`,
			`{"model":"gpt-4o","input":"hello","background":true,"Background":false}`,
			`{"model":"gpt-4o","input":"hello","conversation":"conv_private","Conversation":null}`,
			`{"model":"gpt-4o","input":"hello","model":"gpt-4o"}`,
			`{"model":"myapis-auto","input":"hello","previous_response_id":"resp_owner_a","Previous_response_id":""}`,
		} {
			req := responsesTestRequest(bt.Key, body)
			req.URL.Path = path
			rec := httptest.NewRecorder()
			responsesTestHandler().ServeHTTP(rec, req)
			if rec.Code != 400 || calls != 1 {
				t.Fatalf("ambiguous request admitted at %s: %d %s calls=%d", path, rec.Code, rec.Body, calls)
			}
		}
	}
	normal := httptest.NewRecorder()
	responsesTestHandler().ServeHTTP(normal, responsesTestRequest(bt.Key, `{"model":"gpt-4o","input":"repeat","previous_response_id":"resp_owner_a"}`))
	if normal.Code != 404 || calls != 1 {
		t.Fatalf("cross-owner reference admitted: %d %s", normal.Code, normal.Body)
	}
	after, _ := model.GetUserByID(b.ID)
	tokenAfter, _ := model.GetTokenByID(bt.ID)
	count, _ := model.CountLogs(b.ID)
	if after.Quota != b.Quota || tokenAfter.RemainQuota != bt.RemainQuota || count != 0 {
		t.Fatal("rejected requests changed quota or created billing records")
	}
}
