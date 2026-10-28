package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"starhub/common"
	"starhub/model"
	"starhub/relay/dto"
	"starhub/setting"
)

func setupAutoRoutingDB(t *testing.T) {
	t.Helper()
	setting.ResetAllToDefaults()
	if err := model.InitDB(filepath.Join(t.TempDir(), "auto-routing.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = model.CloseDB()
		setting.ResetAllToDefaults()
	})
}

func TestDistributeAutoModelGroupCandidates(t *testing.T) {
	setupAutoRoutingDB(t)
	for _, channel := range []model.Channel{
		{Name: "default", Models: "gpt-4o,gpt-4o-mini,myapis-auto", Group: "default", Status: common.ChannelStatusEnabled, Weight: 1},
		{Name: "other", Models: "gemini-1.5-flash", Group: "other", Status: common.ChannelStatusEnabled, Weight: 1},
		{Name: "disabled", Models: "deepseek-chat", Group: "default", Status: common.ChannelStatusManuallyDisabled, Weight: 1},
	} {
		if err := model.InsertChannel(&channel); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name, group, limits, requested, selected string
		status                                   int
	}{
		{"unrestricted current group", "default", "", "myapis-auto", "gpt-4o-mini", http.StatusNoContent},
		{"isolated other group", "other", "", "myapis-auto", "gemini-1.5-flash", http.StatusNoContent},
		{"explicit allowlist stays narrow", "default", "gpt-4o", "myapis-auto", "gpt-4o", http.StatusNoContent},
		{"cross-group allowlist cannot expand", "default", "gemini-1.5-flash", "myapis-auto", "", http.StatusForbidden},
		{"unavailable allowlist cannot expand", "default", "deepseek-chat", "myapis-auto", "", http.StatusForbidden},
		{"nonempty blank allowlist cannot expand", "default", " , ", "myapis-auto", "", http.StatusForbidden},
		{"explicit model stays unchanged", "default", "", "gpt-4o", "gpt-4o", http.StatusNoContent},
		{"explicit model still needs authorization", "default", "gpt-4o", "gpt-4o-mini", "", http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := `{"model":"` + tc.requested + `","messages":[{"role":"user","content":"Hello"}]}`
			r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
			r.Header.Set("Content-Type", "application/json")
			r = r.WithContext(common.WithValues(r.Context(), common.CtxUsingGroup, tc.group, common.CtxTokenModelLimits, tc.limits))
			called := false
			handler := Distribute(dto.InboundOpenAI)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				if got := r.Context().Value(common.CtxRequestModel); got != tc.selected {
					t.Errorf("selected model = %v, want %s", got, tc.selected)
				}
				w.WriteHeader(http.StatusNoContent)
			}))
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, r)
			if rec.Code != tc.status || called != (tc.status == http.StatusNoContent) {
				t.Fatalf("status = %d, handler called = %v, want status %d; body = %s", rec.Code, called, tc.status, rec.Body)
			}
		})
	}
}

func TestDistributeAutoModelListFailure(t *testing.T) {
	setupAutoRoutingDB(t)
	db, err := model.DB.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	for _, inbound := range []dto.InboundFormat{dto.InboundOpenAI, dto.InboundClaude} {
		t.Run(string(inbound), func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"myapis-auto"}`))
			r = r.WithContext(common.WithValues(r.Context(), common.CtxUsingGroup, "default"))
			handler := Distribute(inbound)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				t.Fatal("database errors must stop distribution")
			}))
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, r)
			var response struct {
				Type  string `json:"type"`
				Error struct {
					Message string `json:"message"`
					Code    string `json:"code"`
					Type    string `json:"type"`
				} `json:"error"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if rec.Code != http.StatusInternalServerError || response.Error.Type != "server_error" || response.Error.Message != "failed to list models for the current group" {
				t.Fatalf("unexpected error response: status %d, body %s", rec.Code, rec.Body)
			}
			if inbound == dto.InboundOpenAI && response.Error.Code != "internal_error" {
				t.Fatalf("OpenAI error code = %q, want internal_error", response.Error.Code)
			}
			if inbound == dto.InboundClaude && response.Type != "error" {
				t.Fatalf("Claude envelope type = %q, want error", response.Type)
			}
		})
	}
}
