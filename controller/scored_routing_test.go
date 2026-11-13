package controller

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"starhub/common"
	"starhub/middleware"
	"starhub/model"
	"starhub/relay"
	"starhub/relay/dto"
	"starhub/setting"
)

func publishControllerSnapshot(t *testing.T, channels []*model.Channel) {
	t.Helper()
	previousMode := setting.ResourceProviderModeValue()
	t.Cleanup(func() { _ = setting.UpdateResourceProviderMode(setting.OptionKeyResourceProviderMode, previousMode) })
	s := model.RouteSnapshot{SnapshotVersion: "controller-v1", ScoreAlgorithmVersion: "five-dimension-v1", RatingAlgorithmVersion: "un-in-v1", PublishedAt: time.Now().UTC(), ResourceCount: len(channels), EligibleCount: len(channels)}
	for i, ch := range channels {
		raw, _ := json.Marshal(map[string]any{"resource_id": fmt.Sprintf("%d:%s:t10", ch.ID, ch.Models), "channel_id": fmt.Sprint(ch.ID), "model_id": ch.Models, "model_name": ch.Models, "price_tier": 10, "quality_level": "S1", "state": "hot", "trend": "stable", "score": 90 - i, "dim_scores": map[string]int{"success_rate": 99, "cost": 90, "speed": 90, "health": 99, "stability": 99}, "protection": map[string]any{"schema": 1, "endpoint_generation": model.ChannelEndpointGeneration(ch), "recovery_epoch": 1, "state_version": 1, "mode": "allowed"}, "pricing": map[string]any{"schema_version": 4, "cost_basis": "group_ratio", "groups": []any{map[string]any{"group": "default", "ratio": "1", "base_cost": "1", "actual_cost": "1"}}}})
		s.Items = append(s.Items, raw)
	}
	s.Checksum = s.PolicyChecksum()
	if err := model.ApplyRoutingSnapshot(s, time.Now()); err != nil {
		t.Fatal(err)
	}
	_ = setting.UpdateResourceProviderMode(setting.OptionKeyResourceProviderMode, "commercial")
}

func TestCommercialSmartUnitPriceFallbackAndSettlement(t *testing.T) {
	newTestDB(t)
	_ = setting.UpdateRatio(setting.OptionKeyModelRatio, `{"cheap":1,"fallback":3}`)
	_ = setting.UpdateRatio(setting.OptionKeyCompletionRatio, `{"cheap":2,"fallback":1}`)
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), `"model":"cheap"`) {
			w.WriteHeader(503)
			return
		}
		if !strings.Contains(string(body), `"model":"fallback"`) {
			t.Errorf("wrong fallback body: %s", body)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"model":"fallback","choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":2}}`)
	}))
	defer upstream.Close()
	channels := []*model.Channel{}
	for _, name := range []string{"cheap", "fallback"} {
		ch := &model.Channel{Name: name, Models: name, Group: "default", Status: 1, Type: 1, Key: "test", BaseURL: upstream.URL, Weight: 1}
		if err := model.InsertChannel(ch); err != nil {
			t.Fatal(err)
		}
		channels = append(channels, ch)
		if err := model.UpsertModelMeta(&model.ModelMeta{Model: name, Tags: "text", Vendor: "test-vendor"}); err != nil {
			t.Fatal(err)
		}
	}
	publishControllerSnapshot(t, channels)
	u := newUser(t, "score-user", common.RoleUser, 1000000)
	tk := mustToken(t, u.ID, 1000000)
	body := `{"model":"myapis-auto","messages":[{"role":"user","content":"hello"}],"max_tokens":10000}`
	r := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer "+tk.PlaintextKey())
	w := httptest.NewRecorder()
	middleware.TokenAuth(middleware.Distribute(dto.InboundOpenAI)(http.HandlerFunc(RelayChat))).ServeHTTP(w, r)
	if w.Code != 200 || calls.Load() != 2 || w.Header().Get("X-MyAPIs-Actual-Model") != "fallback" || w.Header().Get("X-MyAPIs-Fallback") != "true" {
		t.Fatalf("fallback failed: %d %s %v calls=%d", w.Code, w.Body, w.Header(), calls.Load())
	}
	logs, total, err := model.ListLogs(u.ID, 1, 10, "")
	if err != nil || total != 1 {
		t.Fatalf("logs: %d %v", total, err)
	}
	if logs[0].Model != "fallback" || logs[0].RequestedModel != "myapis-auto" || !logs[0].RoutingFallback || logs[0].ActualVendor != "test-vendor" {
		t.Fatalf("routing audit: %+v", logs[0])
	}
	after, _ := model.GetUserByID(u.ID)
	if after.Quota != u.Quota-relay.ComputeQuota("fallback", "default", 10, 2) {
		t.Fatalf("fallback reserve was not balanced: %d", after.Quota)
	}
	// Candidate restrictions affect only smart entry and never expand authorization.
	if err := model.UpdateTokenFields(tk.ID, map[string]any{"smart_candidates": "not-authorized"}); err != nil {
		t.Fatal(err)
	}
	r = httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer "+tk.PlaintextKey())
	w = httptest.NewRecorder()
	middleware.TokenAuth(middleware.Distribute(dto.InboundOpenAI)(http.HandlerFunc(RelayChat))).ServeHTTP(w, r)
	if w.Code != 503 || calls.Load() != 2 {
		t.Fatalf("candidate restriction ignored: %d %s", w.Code, w.Body)
	}
}

func TestCommercialRetryBudgetAndUncertainBoundary(t *testing.T) {
	for _, uncertain := range []bool{false, true} {
		t.Run(fmt.Sprint(uncertain), func(t *testing.T) {
			newTestDB(t)
			channels := []*model.Channel{}
			for i := 0; i < 5; i++ {
				ch := &model.Channel{Name: fmt.Sprint(i), Models: "test-model", Group: "default", Status: 1, Type: 1, Key: "test"}
				if err := model.InsertChannel(ch); err != nil {
					t.Fatal(err)
				}
				channels = append(channels, ch)
			}
			publishControllerSnapshot(t, channels)
			flow := newRelayFlow(t.Context(), httptest.NewRecorder(), relayRequest{modelName: "test-model", group: "default"}, dto.InboundOpenAI, "chat")
			defer flow.finish()
			deadline, ok := flow.ctx.Deadline()
			if !ok || time.Until(deadline) > 60*time.Second {
				t.Fatal("missing total retry deadline")
			}
			calls := 0
			flow.runChannels(func(*model.Channel) attemptResult {
				calls++
				if uncertain {
					return attemptResult{outcome: attemptUncertain}
				}
				return attemptResult{outcome: attemptUpstreamError, statusCode: 503}
			})
			want := 3
			if uncertain {
				want = 1
			}
			if calls != want {
				t.Fatalf("sent %d attempts, want %d", calls, want)
			}
		})
	}
}
