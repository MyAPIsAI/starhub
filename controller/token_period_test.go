package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
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

func TestTokenNanoBudgetHTTPAndExactBalanceFence(t *testing.T) {
	newTestDB(t)
	u := newUser(t, "nano-budget", common.RoleUser, 100)
	for _, body := range []string{
		`{"name":"nano","remain_quota_nano":"1"}`,
		`{"name":"nano-period","quota_period":"daily","period_quota_limit_nano":"2001"}`,
	} {
		w := call(CreateToken, testRequest{method: "POST", target: "/api/token", userID: u.ID, role: 1, body: body})
		if w.Code != 200 {
			t.Fatalf("exact budget creation: %d %s", w.Code, w.Body)
		}
		var data map[string]any
		decodeData(t, w, &data)
		expected := "1"
		if data["quota_period"] == "daily" {
			expected = "2001"
		}
		if data["remain_quota_nano"] != expected {
			t.Fatalf("fractional budget was rounded: %s", w.Body)
		}
		if expected == "1" {
			id := int64(data["id"].(float64))
			for _, check := range []struct {
				expected string
				status   int
			}{{"0", 409}, {"1", 200}, {"1", 409}} {
				body := fmt.Sprintf(`{"id":%d,"remain_quota_nano":"2","expected_quota_nano":"%s"}`, id, check.expected)
				edited := call(UpdateToken, testRequest{method: "PUT", target: "/api/token", userID: u.ID, role: 1, body: body})
				if edited.Code != check.status {
					t.Fatalf("fractional balance fence: %d %s", edited.Code, edited.Body)
				}
			}
		}
	}
	for _, body := range []string{
		`{"name":"bad","remain_quota_nano":"1","remain_quota":1}`,
		`{"name":"bad","remain_quota_nano":"1.1"}`,
		`{"name":"bad","remain_quota_nano":1}`,
		`{"name":"bad","quota_period":"daily","period_quota_limit_nano":"1","remain_quota_nano":"0"}`,
	} {
		w := call(CreateToken, testRequest{method: "POST", target: "/api/token", userID: u.ID, role: 1, body: body})
		if w.Code != 400 {
			t.Fatalf("invalid monetary contract accepted: %d %s", w.Code, w.Body)
		}
	}
}

func TestOneNanoAccountAndKeyCanPayForExactlyOneHTTPCall(t *testing.T) {
	newTestDB(t)
	user := newUser(t, "last-nano", common.RoleUser, 0)
	if err := model.InsertRedemption(&model.Redemption{Code: "RD-last-nano", Status: 1, QuotaRemainderNano: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := model.RedeemCodeMoney("RD-last-nano", user.ID); err != nil {
		t.Fatal(err)
	}
	token := &model.Token{UserID: user.ID, Name: "last-nano", Status: 1, RemainQuotaRemainderNano: 1}
	if err := model.InsertToken(token); err != nil {
		t.Fatal(err)
	}
	if err := setting.UpdatePricing(setting.OptionKeyModelPrice, `{"last-nano-model":0.000000001}`); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)
	}))
	defer upstream.Close()
	if err := model.InsertChannel(&model.Channel{Name: "last-nano", Models: "last-nano-model", Group: "default", Status: 1, Type: 1, BaseURL: upstream.URL, Key: "fixture"}); err != nil {
		t.Fatal(err)
	}
	handler := middleware.TokenAuth(middleware.Distribute(dto.InboundOpenAI)(http.HandlerFunc(RelayChat)))
	for _, status := range []int{200, 403} {
		r := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"last-nano-model","messages":[{"role":"user","content":"hi"}],"max_tokens":1}`))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Authorization", "Bearer "+token.PlaintextKey())
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("one-nano admission: %d %s", w.Code, w.Body)
		}
	}
	after, _ := model.GetUserByID(user.ID)
	if calls.Load() != 1 || common.QuotaMoney(after.Quota, after.QuotaRemainderNano).Sign() != 0 || common.QuotaMoney(after.UsedQuota, after.UsedQuotaRemainderNano).String() != "1" {
		t.Fatal("last nano could not be spent exactly once")
	}
	if err := model.VerifyMoneyLedger(user.ID); err != nil {
		t.Fatal(err)
	}
}
