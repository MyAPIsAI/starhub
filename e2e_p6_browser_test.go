package main

import (
	"context"
	"net"
	"net/http/httptest"
	"os"
	"os/signal"
	"syscall"
	"testing"
	"time"

	"starhub/common"
	"starhub/middleware"
	"starhub/model"
)

// TestP6BrowserServer exposes an isolated real router only for Playwright.
// Its database and upstream are temporary; it never uses the production DB.
func TestP6BrowserServer(t *testing.T) {
	if os.Getenv("STARHUB_P6_E2E") != "1" {
		t.Skip("Playwright fixture only")
	}
	env := setupRelayApp(t)
	middleware.ConfigureRateLimits(100000, 100000)
	t.Cleanup(func() {
		middleware.ConfigureRateLimits(middleware.DefaultIPLimitPerMinute, middleware.DefaultUserLimitPerMinute)
	})
	p6User(t, "operator", common.RoleAdmin)
	p6User(t, "member", common.RoleUser)
	upstream := newMockUpstream(t)
	channelID := createChannelID(t, env, map[string]any{"name": "Local provider", "type": 1, "key": upstreamKey, "models": "gpt-4o-mini", "base_url": upstream.URL})
	createChannelID(t, env, map[string]any{"name": "Failing provider", "type": 1, "key": "invalid-local-key", "models": "gpt-4o-mini", "base_url": upstream.URL})
	for i := 0; i < 24; i++ {
		entry := &model.Log{UserID: env.rootID, TokenID: env.tokenID, TokenName: "p1-key", ChannelID: channelID, Model: "gpt-4o-mini", PromptTokens: 100 + int64(i), CompletionTokens: 50, Quota: 1, UnfundedQuota: 2, UsedChannels: "1", CreatedAt: time.Now().Add(-time.Duration(i/4) * 24 * time.Hour).Unix()}
		if err := model.InsertLog(entry); err != nil {
			t.Fatal(err)
		}
	}
	app := httptest.NewUnstartedServer(NewRouter())
	app.Listener.Close()
	address := os.Getenv("STARHUB_P6_E2E_ADDR")
	if address == "" {
		address = "127.0.0.1:39781"
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	app.Listener = listener
	app.Start()
	t.Cleanup(app.Close)
	t.Logf("Playwright server ready: %s", app.URL)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
}
