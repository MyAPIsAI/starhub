package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"starhub/common"
	"starhub/middleware"
	"starhub/model"
	"starhub/relay/dto"
	"starhub/setting"
)

func TestPeriodicKeyHTTPAdmissionKeepsLateBillInOriginalPeriod(t *testing.T) {
	newTestDB(t)
	u := newUser(t, "period-relay", common.RoleUser, 10000)
	token := mustToken(t, u.ID, 1000)
	if err := model.UpdateTokenFields(token.ID, map[string]any{"quota_period": "daily", "period_quota_limit": int64(1000)}); err != nil {
		t.Fatal(err)
	}
	token, _ = model.GetTokenByID(token.ID)
	oldID := token.QuotaPeriodID
	setting.UpdateRatio(setting.OptionKeyModelRatio, `{"period-model":1}`)
	setting.UpdateRatio(setting.OptionKeyCompletionRatio, `{"period-model":1}`)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := model.RefreshTokenQuota(token, time.Unix(token.PeriodEnd+1, 0)); err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":2}}`)
	}))
	defer upstream.Close()
	if err := model.InsertChannel(&model.Channel{Name: "period", Models: "period-model", Group: "default", Status: 1, Type: 1, BaseURL: upstream.URL, Key: "fixture"}); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"period-model","messages":[{"role":"user","content":"hello"}],"max_tokens":10}`))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer "+token.PlaintextKey())
	w := httptest.NewRecorder()
	middleware.TokenAuth(middleware.Distribute(dto.InboundOpenAI)(http.HandlerFunc(RelayChat))).ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("relay: %d %s", w.Code, w.Body)
	}
	got, _ := model.GetTokenByID(token.ID)
	var entry model.Log
	model.DB.First(&entry, "token_id = ?", token.ID)
	if entry.Quota <= 0 || entry.QuotaReservationID == "" || entry.AdmittedAt == 0 || entry.QuotaPeriodID != oldID || got.RemainQuota != 1000 || got.PeriodUsedQuota != 0 || got.UsedQuota != entry.Quota {
		t.Fatalf("wrong HTTP admission attribution: %+v %+v", got, entry)
	}
}

func TestCreatePeriodicKeyValidatesQuotaPolicyAndMasksOutput(t *testing.T) {
	newTestDB(t)
	u := newUser(t, "period-create", common.RoleUser, 0)
	for _, body := range []string{
		`{"name":"bad","quota_period":"weekly","period_quota_limit":10}`,
		`{"name":"bad","quota_period":"daily"}`,
		`{"name":"bad","quota_period":"daily","period_quota_limit":-1}`,
		`{"name":"bad","quota_period":"daily","period_quota_limit":10,"unlimited_quota":true}`,
		`{"name":"bad","quota_period":"daily","period_quota_limit":10,"quota_region":"missing"}`,
	} {
		w := call(CreateToken, testRequest{method: "POST", target: "/api/token", userID: u.ID, role: 1, body: body})
		if w.Code != 400 {
			t.Fatalf("invalid period accepted: %d %s", w.Code, w.Body)
		}
	}
	w := call(CreateToken, testRequest{method: "POST", target: "/api/token", userID: u.ID, role: 1, body: `{"name":"monthly","quota_period":"monthly","period_quota_limit":50}`})
	var result map[string]any
	decodeData(t, w, &result)
	if w.Code != 200 || result["quota_period"] != "monthly" || result["period_quota_limit"] != float64(50) || result["quota_timezone"] != "UTC" {
		t.Fatalf("period creation: %s", w.Body)
	}
	w = call(ListOwnedTokens, testRequest{method: "GET", target: "/api/token/owned", userID: u.ID, role: 1})
	if w.Code != 200 || strings.Contains(w.Body.String(), result["key"].(string)) {
		t.Fatal("period list leaked credential")
	}
}

func TestOwnedTokenUsageRejectsForeignKeysAndInvalidPages(t *testing.T) {
	newTestDB(t)
	u := newUser(t, "usage-owner", common.RoleUser, 0)
	other := newUser(t, "usage-root", common.RoleRoot, 0)
	token := mustToken(t, u.ID, 10)
	for _, sample := range []struct {
		uid    int64
		query  string
		status int
	}{{u.ID, "", 200}, {other.ID, "", 404}, {u.ID, "?period_page=0", 400}, {u.ID, "?model_page=1000001", 400}, {u.ID, "?model_page=bad", 400}} {
		w := call(OwnedTokenUsage, testRequest{method: "GET", target: "/api/token/usage" + sample.query, userID: sample.uid, role: common.RoleRoot, path: map[string]string{"id": itoa(token.ID)}})
		if w.Code != sample.status || !strings.Contains(w.Header().Get("Cache-Control"), "no-store") {
			t.Fatalf("usage scope/page: %d %s", w.Code, w.Body)
		}
	}
}
