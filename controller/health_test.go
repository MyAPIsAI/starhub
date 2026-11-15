package controller

import (
	"context"
	"net/http/httptest"
	"testing"

	"starhub/setting"
)

func TestReadinessRequiresDatabaseAndAcceptingTraffic(t *testing.T) {
	newTestDB(t)
	SetDraining(true)
	t.Cleanup(func() { SetDraining(false) })
	if w := call(ReadinessCheck, testRequest{target: "/api/ready"}); w.Code != 503 {
		t.Fatal("draining server ready", w.Code)
	}
	SetDraining(false)
	_ = setting.UpdateResourceProviderMode(setting.OptionKeyResourceProviderMode, "local")
	if w := call(ReadinessCheck, testRequest{target: "/api/ready"}); w.Code != 200 {
		t.Fatal("local server unavailable", w.Code)
	}
	_ = setting.UpdateResourceProviderMode(setting.OptionKeyResourceProviderMode, "commercial")
	if w := call(ReadinessCheck, testRequest{target: "/api/ready"}); w.Code != 503 {
		t.Fatal("cold commercial server ready", w.Code)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w := httptest.NewRecorder()
	HealthCheck(w, httptest.NewRequest("GET", "/api/health", nil).WithContext(ctx))
	if w.Code != 503 {
		t.Fatal("cancelled probe returned healthy", w.Code)
	}
}
