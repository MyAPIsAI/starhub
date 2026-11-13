package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"starhub/common"
	"starhub/metric"
	"starhub/middleware"
	"starhub/model"
	"starhub/relay/dto"
	"starhub/service"
)

func TestProductionTelemetryCapturesRealRelayMetadataWithoutContent(t *testing.T) {
	newTestDB(t)
	metric.Reset()
	t.Cleanup(metric.Reset)
	now := time.Now().Truncate(time.Minute)
	metric.StartProduction(now.Add(-time.Minute))
	u := newUser(t, "telemetry-relay", common.RoleUser, 10000000)
	token := mustToken(t, u.ID, 1000000)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"content":"private response content"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":2}}`)
	}))
	defer upstream.Close()
	ch := &model.Channel{Name: "telemetry", Models: "telemetry-model", Group: "default", Status: 1, Type: 1, BaseURL: upstream.URL, Key: "private-provider-key"}
	if err := model.InsertChannel(ch); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"telemetry-model","messages":[{"role":"user","content":"private request content"}],"tools":[{"type":"function","function":{"name":"test"}}],"max_tokens":10}`))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer "+token.PlaintextKey())
	w := httptest.NewRecorder()
	middleware.TokenAuth(middleware.Distribute(dto.InboundOpenAI)(http.HandlerFunc(RelayChat))).ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("relay status %d", w.Code)
	}
	if err := service.FlushProductionTelemetry(now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	rows, err := model.ReadProductionAttempts(now.Unix(), now.Unix(), 1, 100)
	if err != nil || len(rows) != 1 {
		t.Fatalf("attempts=%d error=%v", len(rows), err)
	}
	var a metric.ProductionAttempt
	if err := json.Unmarshal([]byte(rows[0].Payload), &a); err != nil {
		t.Fatal(err)
	}
	if a.Generation != model.ChannelEndpointGeneration(ch) || a.Capability != "chat" || a.Outcome != "success" || a.InputTokens != 10 || len(a.RequestedCapabilities) == 0 {
		t.Fatalf("incomplete routing evidence: %+v", a)
	}
	for _, private := range []string{"private request content", "private response content", "private-provider-key", token.PlaintextKey()} {
		if strings.Contains(rows[0].Payload, private) {
			t.Fatal("private content entered telemetry")
		}
	}
}
