package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"starhub/model"
	"strings"
	"sync/atomic"
	"testing"
)

func TestNativeResponseLifecycleActualRouterRetainsOriginalAccount(t *testing.T) {
	env := setupRelayApp(t)
	var selected string
	var create, cancel, get, deleted atomic.Int64
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "POST" && r.URL.Path == "/v1/responses" {
			selected = r.Header.Get("Authorization")
			create.Add(1)
			fmt.Fprint(w, `{"id":"resp_lifecycle","model":"gpt-4o","status":"queued","store":true}`)
			return
		}
		if r.Header.Get("Authorization") != selected {
			t.Error("lifecycle rotated the original provider account")
		}
		if r.Method == "DELETE" {
			deleted.Add(1)
			fmt.Fprint(w, `{"id":"resp_lifecycle","deleted":true}`)
			return
		}
		if r.Method == "POST" {
			cancel.Add(1)
		} else {
			get.Add(1)
		}
		fmt.Fprint(w, `{"id":"resp_lifecycle","model":"gpt-4o","status":"cancelled","usage":{"input_tokens":10,"output_tokens":5},"output":[]}`)
	}))
	defer up.Close()
	createChannelID(t, env, map[string]any{"name": "lifecycle", "key": "first-account\nsecond-account", "key_mode": "round_robin", "base_url": up.URL, "models": "gpt-4o"})
	call := func(method, path, body string) int {
		t.Helper()
		req, _ := http.NewRequest(method, env.base+path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+env.apiKey)
		req.Header.Set("Content-Type", "application/json")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode >= 500 {
			t.Fatalf("lifecycle: %d %s", res.StatusCode, raw)
		}
		return res.StatusCode
	}
	if got := call("POST", "/v1/responses", `{"model":"gpt-4o","input":"hello","background":true,"store":true}`); got != 200 {
		t.Fatal(got)
	}
	if got := call("POST", "/v1/responses/resp_lifecycle/cancel", ""); got != 200 {
		t.Fatal(got)
	}
	if got := call("GET", "/v1/responses/resp_lifecycle", ""); got != 200 {
		t.Fatal(got)
	}
	if got := call("DELETE", "/v1/responses/resp_lifecycle", ""); got != 200 {
		t.Fatal(got)
	}
	if got := call("GET", "/v1/responses/resp_lifecycle", ""); got != 404 {
		t.Fatal("deleted ID was revived")
	}
	token, _ := model.GetTokenByKey(env.apiKey)
	count, _ := model.CountLogs(token.UserID)
	if count != 1 || create.Load() != 1 || cancel.Load() != 1 || get.Load() != 1 || deleted.Load() != 1 {
		t.Fatal("lifecycle was repeated or billed twice")
	}
}
