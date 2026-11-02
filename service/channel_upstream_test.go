package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"starhub/common"
	"starhub/model"
)

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
		{"sk-abc1234567def", "sk-abc1*****7def"},
		{"short", "*****"},
		{"sk-1234567890ab", "sk-1234****90ab"},
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
