package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"starhub/middleware"
	"starhub/model"
	"strings"
	"testing"
	"time"
)

func TestGuardedAccountClosureReceiptRecoveryAndCookieIsolation(t *testing.T) {
	env := setupRelayApp(t)
	owner, session := exportTestUser(t, env.base, "closure-http")
	_, other := exportTestUser(t, env.base, "closure-other")
	second := middleware.Sessions.Create(owner, time.Hour)
	defer middleware.Sessions.Remove(second)
	if err := model.DB.Create(&model.Wallet{UserID: owner, Balance: 9999999}).Error; err != nil {
		t.Fatal(err)
	}
	state, _ := model.ReadAccountDeactivationState(owner)
	body := fmt.Sprintf(`{"schema":"deactivation_action_v1","expected_user_id":%d,"account_revision":"%s","operation_id":"00000000000000000000000000000001","acknowledge_dust":true}`, owner, state.AccountRevision)
	call := func(path, cookie string) (int, []byte) {
		t.Helper()
		req, _ := http.NewRequest("POST", env.base+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if cookie != "" {
			req.AddCookie(&http.Cookie{Name: "session", Value: cookie})
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		raw, _ := io.ReadAll(res.Body)
		if len(res.Cookies()) != 0 || !strings.Contains(res.Header.Get("Cache-Control"), "no-store") {
			t.Fatal("closure changed Cookie or permitted caching")
		}
		return res.StatusCode, raw
	}
	for _, cookie := range []string{other, env.session} {
		if status, _ := call("/api/user/deactivate", cookie); status != 409 {
			t.Fatalf("foreign owner closed account: %d", status)
		}
	}
	status, raw := call("/api/user/deactivate", session)
	if status != 200 {
		t.Fatalf("closure: %d %s", status, raw)
	}
	var receipt struct {
		Data struct {
			Closed bool   `json:"closed"`
			Amount string `json:"written_off_nano"`
		}
	}
	if json.Unmarshal(raw, &receipt) != nil || !receipt.Data.Closed || receipt.Data.Amount != "9999999" {
		t.Fatal("wrong closure receipt")
	}
	for _, token := range []string{session, second} {
		if _, ok := middleware.Sessions.Get(token); ok {
			t.Fatal("owned session survived closure")
		}
	}
	if _, ok := middleware.Sessions.Get(other); !ok {
		t.Fatal("foreign session revoked")
	}
	if status, _ := call("/api/user/deactivate", session); status != 401 {
		t.Fatal("revoked session repeated closure")
	}
	if status, _ := call("/api/user/deactivation-receipt", session); status != 200 {
		t.Fatal("old originating Cookie could not recover committed receipt")
	}
	for _, token := range []string{other, ""} {
		if status, _ := call("/api/user/deactivation-receipt", token); status != 404 {
			t.Fatal("foreign/anonymous receipt read succeeded")
		}
	}
}
