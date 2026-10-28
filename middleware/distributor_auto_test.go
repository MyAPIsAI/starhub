package middleware

import (
	"bytes"
	"encoding/json"
	"io"
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

func TestDistributeAutoModelPreservesJSONFields(t *testing.T) {
	setupAutoRoutingDB(t)
	if err := model.InsertChannel(&model.Channel{Name: "json", Models: "gpt-4o", Group: "default", Status: common.ChannelStatusEnabled, Weight: 1}); err != nil {
		t.Fatal(err)
	}
	body := `{
		"model": "myapis-auto", "stream": true, "max_tokens": 64,
		"messages": [{"role":"user","content":"Keep myapis-auto in this text"}],
		"tools": [{"type":"function","function":{"name":"lookup","parameters":{"type":"object"}}}],
		"vendor_extension": {"large":9007199254740993,"precise":1.234567890123456789}
	}`
	var original map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &original); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r = r.WithContext(common.WithValues(r.Context(), common.CtxUsingGroup, "default"))
	called := false
	handler := Distribute(dto.InboundOpenAI)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		cached := r.Context().Value(common.CtxRequestBody).([]byte)
		readBody, err := io.ReadAll(r.Body)
		if err != nil || !bytes.Equal(readBody, cached) || r.ContentLength != int64(len(cached)) {
			t.Fatalf("request reader, cached body, and content length diverged: %v", err)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(cached, &fields); err != nil {
			t.Fatal(err)
		}
		if string(fields["model"]) != `"gpt-4o"` || r.Context().Value(common.CtxRequestModel) != "gpt-4o" {
			t.Fatalf("routed body model and context disagree: %s", cached)
		}
		if len(fields) != len(original) || r.Context().Value(common.CtxRequestStream) != true {
			t.Fatal("routing changed request metadata")
		}
		for key, value := range original {
			if key == "model" {
				continue
			}
			var before, after bytes.Buffer
			if err := json.Compact(&before, value); err != nil {
				t.Fatal(err)
			}
			if err := json.Compact(&after, fields[key]); err != nil {
				t.Fatal(err)
			}
			if before.String() != after.String() {
				t.Errorf("field %s changed: %s -> %s", key, &before, &after)
			}
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, r)
	if !called || rec.Code != http.StatusNoContent {
		t.Fatalf("request did not reach handler: %d %s", rec.Code, rec.Body)
	}
}

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
		{Name: "default", Models: "gpt-4o,gpt-4o-mini,myapis-auto,veyra-auto", Group: "default", Status: common.ChannelStatusEnabled, Weight: 1},
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
		{"legacy entry is not an actual candidate", "default", "veyra-auto,gpt-4o", "myapis-auto", "gpt-4o", http.StatusNoContent},
		{"cross-group allowlist cannot expand", "default", "gemini-1.5-flash", "myapis-auto", "", http.StatusForbidden},
		{"unavailable allowlist cannot expand", "default", "deepseek-chat", "myapis-auto", "", http.StatusForbidden},
		{"nonempty blank allowlist cannot expand", "default", " , ", "myapis-auto", "", http.StatusForbidden},
		{"explicit model stays unchanged", "default", "", "gpt-4o", "gpt-4o", http.StatusNoContent},
		{"explicit model still needs authorization", "default", "gpt-4o", "gpt-4o-mini", "", http.StatusForbidden},
		{"legacy alias current group", "default", "", "veyra-auto", "gpt-4o-mini", http.StatusNoContent},
		{"legacy alias narrow allowlist", "default", "gpt-4o", "veyra-auto", "gpt-4o", http.StatusNoContent},
		{"legacy alias cross-group denial", "default", "gemini-1.5-flash", "veyra-auto", "", http.StatusForbidden},
		{"legacy alias virtual-only allowlist", "default", "veyra-auto", "veyra-auto", "", http.StatusServiceUnavailable},
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
				if got := r.Context().Value(common.CtxRequestedModel); got != tc.requested {
					t.Errorf("original model = %v, want %s", got, tc.requested)
				}
				cached := r.Context().Value(common.CtxRequestBody).([]byte)
				if tc.requested != "myapis-auto" && tc.requested != "veyra-auto" && string(cached) != body {
					t.Errorf("explicit-model request body was rewritten: %s", cached)
				}
				var payload struct {
					Model string `json:"model"`
				}
				if err := json.Unmarshal(cached, &payload); err != nil || payload.Model != tc.selected {
					t.Errorf("cached body model = %q, want %s: %v", payload.Model, tc.selected, err)
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

func TestDistributeAutoModelWithoutCandidates(t *testing.T) {
	setupAutoRoutingDB(t)
	for _, channel := range []model.Channel{
		{Name: "virtual", Models: "myapis-auto,veyra-auto", Group: "virtual", Status: common.ChannelStatusEnabled, Weight: 1},
		{Name: "disabled", Models: "gpt-4o", Group: "disabled", Status: common.ChannelStatusManuallyDisabled, Weight: 1},
		{Name: "other", Models: "gpt-4o", Group: "other", Status: common.ChannelStatusEnabled, Weight: 1},
	} {
		if err := model.InsertChannel(&channel); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct{ name, group, limits string }{
		{"empty group", "empty", ""},
		{"disabled channels", "disabled", ""},
		{"virtual entries only", "virtual", ""},
		{"explicit virtual allowlist", "virtual", "myapis-auto"},
		{"unavailable allowed candidate", "virtual", "myapis-auto,missing"},
	} {
		for _, requested := range []string{"myapis-auto", "veyra-auto"} {
			for _, inbound := range []dto.InboundFormat{dto.InboundOpenAI, dto.InboundClaude} {
				t.Run(tc.name+"/"+requested+"/"+string(inbound), func(t *testing.T) {
					r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"`+requested+`"}`))
					limits := strings.ReplaceAll(tc.limits, "myapis-auto", requested)
					r = r.WithContext(common.WithValues(r.Context(), common.CtxUsingGroup, tc.group, common.CtxTokenModelLimits, limits))
					handler := Distribute(inbound)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
						t.Fatal("an unresolved virtual model must not reach the relay")
					}))
					rec := httptest.NewRecorder()
					handler.ServeHTTP(rec, r)
					var response struct {
						Type  string `json:"type"`
						Error struct {
							Type string `json:"type"`
							Code string `json:"code"`
						} `json:"error"`
					}
					if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
						t.Fatal(err)
					}
					if rec.Code != http.StatusServiceUnavailable || response.Error.Type != "server_error" {
						t.Fatalf("unexpected response: %d %s", rec.Code, rec.Body)
					}
					if inbound == dto.InboundOpenAI && response.Error.Code != "no_available_channel" {
						t.Fatalf("OpenAI error code = %q, want no_available_channel", response.Error.Code)
					}
					if inbound == dto.InboundClaude && response.Type != "error" {
						t.Fatalf("Claude error envelope type = %q, want error", response.Type)
					}
				})
			}
		}
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
