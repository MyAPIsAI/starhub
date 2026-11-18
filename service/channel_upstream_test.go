package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"starhub/common"
	"starhub/model"
	"starhub/relay/channel"
)

func TestFetchUpstreamTokenUsageArkUnsupported(t *testing.T) {
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer upstream.Close()
	ch := &model.Channel{ID: 32, Type: channel.ChannelTypeArk, BaseURL: upstream.URL, Key: "fixture-key", UsedQuota: 100, UsedQuotaRemainderNano: 1}
	result, err := FetchUpstreamTokenUsage(context.Background(), ch)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 0 || result.Supported || result.Code != "ark_usage_not_supported" {
		t.Fatalf("unexpected Ark usage result: calls=%d result=%+v", calls, result)
	}
	if result.LocalUsedQuotaNano != "200001" || result.UpstreamTotalNano != "" || result.DiffNano != "" || len(result.Keys) != 0 {
		t.Fatalf("local usage must remain available without fabricated upstream amounts: %+v", result)
	}
}

func TestFetchUpstreamTokenUsageUnsupportedUpstream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer upstream.Close()

	ch := &model.Channel{
		ID:      1,
		Name:    "test",
		BaseURL: upstream.URL,
		Key:     "sk-testkey123456",
	}
	result, err := FetchUpstreamTokenUsage(context.Background(), ch)
	if err != nil {
		t.Fatal(err)
	}
	if result.Supported {
		t.Fatal("should be unsupported when upstream returns 404")
	}
	if result.Keys[0].Available {
		t.Fatal("key should not be available")
	}
}

func TestFetchUpstreamTokenUsageSuccess(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/usage/token/" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		data := map[string]any{
			"name":            "upstream-key",
			"total_used":      50000,
			"total_available": 50000,
			"total_granted":   100000,
			"unlimited_quota": false,
			"expires_at":      0,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data":    data,
		})
	}))
	defer upstream.Close()

	ch := &model.Channel{
		ID:        1,
		Name:      "test",
		BaseURL:   upstream.URL,
		Key:       "sk-testkey123456",
		UsedQuota: 48000,
	}
	result, err := FetchUpstreamTokenUsage(context.Background(), ch)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Supported {
		t.Fatal("should be supported")
	}
	if len(result.Keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(result.Keys))
	}
	if result.Keys[0].TotalUsed != 50000 {
		t.Fatalf("expected total_used=50000, got %d", result.Keys[0].TotalUsed)
	}
	if result.UpstreamTotal != 50000 {
		t.Fatalf("expected upstream_total=50000, got %d", result.UpstreamTotal)
	}
	if result.Diff != 2000 {
		t.Fatalf("expected diff=2000, got %d", result.Diff)
	}
	if result.Keys[0].KeyMasked == "sk-testkey123456" {
		t.Fatal("key should be masked")
	}
}

func TestFetchUpstreamTokenUsageMultiKey(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("Authorization")
		used := int64(10000)
		if key == "Bearer sk-secondkey9999" {
			used = 20000
		}
		data := map[string]any{
			"total_used":      used,
			"total_available": 80000,
			"total_granted":   100000,
			"unlimited_quota": false,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data":    data,
		})
	}))
	defer upstream.Close()

	ch := &model.Channel{
		ID:        1,
		Name:      "multi-key",
		BaseURL:   upstream.URL,
		Key:       "sk-firstkey1234\nsk-secondkey9999",
		UsedQuota: 25000,
	}
	result, err := FetchUpstreamTokenUsage(context.Background(), ch)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Supported {
		t.Fatal("should be supported")
	}
	if len(result.Keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(result.Keys))
	}
	if result.UpstreamTotal != 30000 {
		t.Fatalf("expected upstream_total=30000, got %d", result.UpstreamTotal)
	}
	if result.Diff != 5000 {
		t.Fatalf("expected diff=5000, got %d", result.Diff)
	}
}

func TestFetchUpstreamTokenUsageEmptyBaseURL(t *testing.T) {
	ch := &model.Channel{
		ID:      1,
		Name:    "no-base",
		BaseURL: "",
		Key:     "sk-testkey123456",
	}
	result, err := FetchUpstreamTokenUsage(context.Background(), ch)
	if err != nil {
		t.Fatal(err)
	}
	if result.Supported {
		t.Fatal("should be unsupported with empty base URL")
	}
}

func TestFetchUpstreamTokenUsageUnauthorized(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer upstream.Close()

	ch := &model.Channel{
		ID:      1,
		Name:    "bad-key",
		BaseURL: upstream.URL,
		Key:     "sk-invalidkey999",
	}
	result, err := FetchUpstreamTokenUsage(context.Background(), ch)
	if err != nil {
		t.Fatal(err)
	}
	if result.Supported {
		t.Fatal("should be unsupported when all keys fail")
	}
	if result.Keys[0].Available {
		t.Fatal("key should not be available")
	}
	if result.Keys[0].Error == "" {
		t.Fatal("should have error message")
	}
}

func TestMaskKey(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"sk-abc1234567def", "sk-abc1********7def"},
		{"short", "********"},
		{"sk-1234567890ab", "sk-1234********90ab"},
		{strings.Repeat("x", 200), "xxxxxxx********xxxx"},
	}
	for _, tc := range tests {
		got := maskKey(tc.input)
		if got != tc.want {
			t.Errorf("maskKey(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestFetchUpstreamTokenUsageDisabledChannel(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{
				"total_used":      100,
				"total_available": 900,
				"total_granted":   1000,
				"unlimited_quota": false,
			},
		})
	}))
	defer upstream.Close()

	ch := &model.Channel{
		ID:      1,
		Name:    "disabled",
		BaseURL: upstream.URL,
		Key:     "sk-disabledkey12",
		Status:  common.ChannelStatusAutoDisabled,
	}
	result, err := FetchUpstreamTokenUsage(context.Background(), ch)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Supported {
		t.Fatal("usage query should work even for disabled channels")
	}
	if result.Keys[0].TotalUsed != 100 {
		t.Fatalf("expected total_used=100, got %d", result.Keys[0].TotalUsed)
	}
}

func TestFetchUpstreamTokenUsageNoDataWrapper(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"name":"bare","total_used":42,"total_available":58,"total_granted":100,"unlimited_quota":false,"expires_at":0}`)
	}))
	defer upstream.Close()

	ch := &model.Channel{
		ID:        1,
		Name:      "bare",
		BaseURL:   upstream.URL,
		Key:       "sk-barekey123456",
		UsedQuota: 42,
	}
	result, err := FetchUpstreamTokenUsage(context.Background(), ch)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Supported {
		t.Fatal("should be supported")
	}
	if result.Keys[0].TotalUsed != 42 {
		t.Fatalf("expected total_used=42, got %d", result.Keys[0].TotalUsed)
	}
	if result.Diff != 0 {
		t.Fatalf("expected diff=0, got %d", result.Diff)
	}
}

func TestUpstreamUsageResponseValidation(t *testing.T) {
	const valid = `{"total_used":0,"total_available":0,"total_granted":0}`
	tests := []struct {
		name, body, code string
		status           int
	}{
		{"valid zero", `{"success":true,"data":` + valid + `}`, "", 200},
		{"bare zero", valid, "", 200},
		{"business error", `{"success":false,"message":"fixture-secret","data":` + valid + `}`, "usage_query_rejected", 200},
		{"code error", `{"code":false,"data":` + valid + `}`, "usage_query_rejected", 200},
		{"wrong success type", `{"success":"true","data":` + valid + `}`, "usage_invalid_response", 200},
		{"empty object", `{}`, "usage_invalid_response", 200},
		{"null", `null`, "usage_invalid_response", 200},
		{"null data", `{"success":true,"data":null}`, "usage_invalid_response", 200},
		{"html", `<html>fixture-secret</html>`, "usage_invalid_response", 200},
		{"missing used", `{"total_available":0}`, "usage_invalid_response", 200},
		{"missing remaining", `{"total_used":0}`, "usage_invalid_response", 200},
		{"null used", `{"total_used":null,"total_available":0}`, "usage_invalid_response", 200},
		{"string amount", `{"total_used":"1","total_available":0}`, "usage_invalid_response", 200},
		{"fractional amount", `{"total_used":0.5,"total_available":0}`, "usage_invalid_response", 200},
		{"overflow amount", `{"total_used":9223372036854775808,"total_available":0}`, "usage_invalid_response", 200},
		{"negative used", `{"total_used":-1,"total_available":0}`, "usage_invalid_response", 200},
		{"negative granted", `{"total_used":0,"total_available":0,"total_granted":-1}`, "usage_invalid_response", 200},
		{"negative remaining", `{"total_used":1,"total_available":-1}`, "", 200},
		{"conflicting unlimited", `{"total_used":0,"total_available":0,"unlimited":false,"unlimited_quota":true}`, "usage_invalid_response", 200},
		{"oversized", strings.Repeat(" ", (1<<20)+1), "usage_invalid_response", 200},
		{"unsupported", `{}`, "usage_not_supported", 404},
		{"method unsupported", `{}`, "usage_not_supported", 405},
		{"unauthorized", `{}`, "usage_auth_failed", 401},
		{"forbidden", `{}`, "usage_auth_failed", 403},
		{"rate limited", `{}`, "usage_http_error", 429},
		{"server error", `{}`, "usage_http_error", 503},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer upstream.Close()
			usage := fetchOneUpstreamKey(context.Background(), upstream.URL, "fixture-key", 0)
			if usage.Code != tc.code || usage.Available != (tc.code == "") {
				t.Fatalf("unexpected usage: %+v", usage)
			}
			if strings.Contains(usage.Error, "fixture-secret") || strings.Contains(usage.Error, "fixture-key") {
				t.Fatal("upstream details leaked into diagnostic")
			}
		})
	}
}

func TestUpstreamUsageUnlimitedAliases(t *testing.T) {
	for _, flag := range []string{"unlimited", "unlimited_quota"} {
		t.Run(flag, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprintf(w, `{"total_used":1,"total_available":0,%q:true}`, flag)
			}))
			defer upstream.Close()
			usage := fetchOneUpstreamKey(context.Background(), upstream.URL, "fixture-key", 0)
			if !usage.Available || !usage.Unlimited {
				t.Fatalf("unlimited quota was not preserved: %+v", usage)
			}
		})
	}
}

func TestUpstreamUsagePartialAndFailedAggregation(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Header.Get("Authorization") {
		case "Bearer fixture-ok":
			fmt.Fprint(w, `{"total_used":123,"total_available":4}`)
		case "Bearer fixture-unsupported":
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusUnauthorized)
		}
	}))
	defer upstream.Close()
	for _, tc := range []struct{ keys, status, code, nano string }{
		{"fixture-ok\nfixture-invalid", "partial", "usage_incomplete", "246000"},
		{"fixture-invalid", "failed", "usage_query_failed", ""},
		{"fixture-unsupported", "unsupported", "usage_not_supported", ""},
		{"fixture-invalid\nfixture-unsupported", "failed", "usage_query_failed", ""},
	} {
		result, err := FetchUpstreamTokenUsage(context.Background(), &model.Channel{BaseURL: upstream.URL, Key: tc.keys, UsedQuotaRemainderNano: 1})
		if err != nil {
			t.Fatal(err)
		}
		if result.Status != tc.status || result.Code != tc.code || result.UpstreamTotalNano != tc.nano || result.Supported != (tc.status == "partial") {
			t.Fatalf("unexpected aggregate: %+v", result)
		}
		if result.DiffNano != "" || result.Diff != 0 || result.LocalUsedQuotaNano != "1" {
			t.Fatalf("incomplete queries must not produce a difference or lose local usage: %+v", result)
		}
	}
}

func TestUpstreamUsageTimeoutAndTruncatedResponse(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "999")
		fmt.Fprint(w, `{"total_used":0,"total_available":0}`)
	}))
	defer upstream.Close()
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	if got := fetchOneUpstreamKey(ctx, upstream.URL, "fixture-key", 0); got.Code != "usage_timeout" || got.Available {
		t.Fatalf("timeout was misclassified: %+v", got)
	}
	if got := fetchOneUpstreamKey(context.Background(), upstream.URL, "fixture-key", 0); got.Code != "usage_invalid_response" || got.Available {
		t.Fatalf("truncated response was accepted: %+v", got)
	}
}
