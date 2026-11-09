package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"starhub/model"
)

func TestNFRAC10TraceFromGatewayThroughLedger(t *testing.T) {
	env := setupRelayApp(t)
	keyPRDConfigure(t, env, "0.001")
	if err := model.UpsertModelMeta(&model.ModelMeta{Model: "prd-model", Vendor: "configured-vendor"}); err != nil {
		t.Fatal(err)
	}
	const private = "NFR_CONTENT_MUST_NOT_BE_RECORDED"
	forwardedTrace := ""
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
	defer bad.Close()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		forwardedTrace = r.Header.Get("X-Request-ID")
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-ID", "provider-request-123")
		_, _ = w.Write([]byte(`{"id":"fixture","choices":[{"message":{"role":"assistant","content":"` + private + `"}}],"usage":{"prompt_tokens":10,"completion_tokens":5}}`))
	}))
	defer upstream.Close()
	for _, ch := range []*model.Channel{{Name: "failure", Type: 1, Status: 1, Key: upstreamKey, BaseURL: bad.URL, Models: "prd-model", Group: "default", Priority: 100}, {Name: "success", Type: 1, Status: 1, Key: upstreamKey, BaseURL: upstream.URL, Models: "prd-model", Group: "default", Priority: 1}} {
		if err := model.InsertChannel(ch); err != nil {
			t.Fatal(err)
		}
	}
	call := func(token string) string {
		t.Helper()
		body, _ := json.Marshal(map[string]any{"model": "prd-model", "messages": []map[string]string{{"role": "user", "content": private}}})
		req, _ := http.NewRequest("POST", env.base+"/v1/chat/completions", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.ReadAll(response.Body)
		response.Body.Close()
		if token == env.apiKey && response.StatusCode != 200 {
			t.Fatal(response.StatusCode)
		}
		return response.Header.Get("X-Request-ID")
	}
	id := call(env.apiKey)
	if forwardedTrace != id || len(id) != 64 {
		t.Fatal("upstream trace identity was not propagated")
	}
	if err := model.FlushAPICharges(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	result := doRequest(env.base, "GET", "/api/admin/traces/"+id, env.session, nil)
	if result.Status != 200 {
		t.Fatalf("trace query: %d %s", result.Status, result.Body)
	}
	var envelope struct {
		Data model.TraceView `json:"data"`
	}
	if err := json.Unmarshal([]byte(result.Body), &envelope); err != nil {
		t.Fatal(err)
	}
	trace := envelope.Data
	if trace.Request.Vendor != "configured-vendor" || trace.Request.ServiceGroup != "default" {
		t.Fatal("configured metric dimensions were lost")
	}
	stages := map[string]int{}
	provider := false
	for _, step := range trace.Steps {
		stages[step.Stage]++
		if step.UpstreamID == "provider-request-123" {
			provider = true
		}
	}
	for _, stage := range []string{"gateway", "authentication", "precharge", "routing", "upstream", "metering", "settlement", "ledger"} {
		if stages[stage] == 0 {
			t.Fatalf("missing %s: %+v", stage, trace)
		}
	}
	if stages["upstream"] != 2 || !provider || trace.Request.ID != id || trace.Request.UserID != env.rootID || trace.AccountingStatus != "posted" || trace.LedgerID == 0 || trace.LogID == 0 || !trace.Complete {
		t.Fatalf("incomplete trace: %+v", trace)
	}
	if strings.Contains(result.Body, private) || strings.Contains(result.Body, env.apiKey) || strings.Contains(result.Body, upstreamKey) {
		t.Fatal("trace exposed private content or credentials")
	}
	if result := doRequest(env.base, "GET", "/api/admin/traces/"+id, env.apiKey, nil); result.Status < 400 {
		t.Fatal("API key read staff trace")
	}
	failedID := call("invalid-key")
	failed, err := model.ReadTrace(context.Background(), failedID)
	if err != nil || failed.Request.Status != 401 || failed.LogID != 0 || failed.AccountingStatus != "not_applicable" {
		t.Fatalf("rejected call missing trace or charged: %+v %v", failed, err)
	}
}

func TestNFR09ObserverPaymentIdempotencyAndProtectedHealth(t *testing.T) {
	env := setupRelayApp(t)
	t.Setenv("STARHUB_OBSERVER_TOKEN", strings.Repeat("o", 40))
	request := func(method, path string, payload any, authorized bool) *http.Response {
		t.Helper()
		raw, _ := json.Marshal(payload)
		req, _ := http.NewRequest(method, env.base+path, bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		if authorized {
			req.Header.Set("Authorization", "Bearer "+strings.Repeat("o", 40))
		}
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, response.Body)
		response.Body.Close()
		return response
	}
	if request("GET", "/internal/health", nil, false).StatusCode != 401 {
		t.Fatal("health endpoint unprotected")
	}
	if request("GET", "/internal/health", nil, true).StatusCode != 200 {
		t.Fatal("healthy observer probe failed")
	}
	payload := map[string]any{"event_id": "callback-event-0001", "source": "fixture", "success": false, "at": time.Now().Unix()}
	for i := 0; i < 2; i++ {
		if response := request("POST", "/internal/observations/payment", payload, true); response.StatusCode != 200 {
			t.Fatal(response.StatusCode)
		}
	}
	var count int64
	model.DB.Model(&model.Observation{}).Where("kind = ?", "payment_callback").Count(&count)
	if count != 1 {
		t.Fatal("callback observation duplicated", count)
	}
	payload["success"] = true
	if request("POST", "/internal/observations/payment", payload, true).StatusCode != 409 {
		t.Fatal("conflicting observation accepted")
	}
	if result := doRequest(env.base, "GET", "/api/admin/observability", env.session, nil); result.Status != 200 {
		t.Fatalf("monitoring query: %d %s", result.Status, result.Body)
	}
}
