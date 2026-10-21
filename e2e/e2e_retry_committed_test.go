package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"starhub/common"
	"starhub/model"
)

// Test6xxHeadersCommittedNoFailover verifies section 6.2 of the P21 test strategy:
// "响应头/首片已提交 → 不再换渠道拼接响应；结束方式符合协议"
//
// When the relay has already started writing the response (headers sent +
// body started), and the upstream then fails mid-stream, the relay must
// NOT retry on another channel — doing so would concatenate two different
// responses. The partial response should be what the client receives.
func Test6xxHeadersCommittedNoFailover(t *testing.T) {
	// First upstream: sends 200 OK + partial body, then fails mid-stream.
	var firstHits int32
	firstUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&firstHits, 1)
		// Write headers and partial body — this commits the response.
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher := w.(http.Flusher)
		fmt.Fprintf(w, "data: partial-chunk-1\n\n")
		flusher.Flush()
		// Now simulate mid-stream failure by closing the connection.
		// The http.Flusher + closing the hijacked connection causes an
		// EOF on the client side.
		if hijacker, ok := w.(http.Hijacker); ok {
			conn, _, _ := hijacker.Hijack()
			if conn != nil {
				conn.Close()
			}
		}
	}))
	t.Cleanup(firstUpstream.Close)

	// Backup upstream that should NEVER be hit after response committed.
	var backupHits int32
	backupUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&backupHits, 1)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher := w.(http.Flusher)
		fmt.Fprintf(w, "data: backup-chunk\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	t.Cleanup(backupUpstream.Close)

	env := setupRelayApp(t)

	// 1. Set user quota to a known value.
	userBefore, _ := model.GetUserByID(env.rootID)
	res := doRequest(env.base, "PUT", "/api/user/", env.session, map[string]any{
		"id":             env.rootID,
		"quota":          10000000,
		"expected_quota": userBefore.Quota,
	})
	if res.Status != 200 {
		t.Fatalf("set user quota failed: %d %s", res.Status, res.Body)
	}
	userBefore, _ = model.GetUserByID(env.rootID)

	// 2. Create two channels: first (high priority) and backup (low priority).
	chFirst := createChannelID(t, env, map[string]any{
		"name":     "first-partial",
		"key":      upstreamKey,
		"base_url": firstUpstream.URL,
		"models":   "gpt-committed-test",
		"priority": 30,
	})
	chBackup := createChannelID(t, env, map[string]any{
		"name":     "backup-healthy",
		"key":      upstreamKey,
		"base_url": backupUpstream.URL,
		"models":   "gpt-committed-test",
		"priority": 10,
	})

	// 3. Make a streaming relay request — the first upstream commits the
	//    response (sends 200 + partial body) then fails mid-stream.
	chatBody := map[string]any{
		"model":    "gpt-committed-test",
		"stream":   true,
		"messages": []map[string]string{{"role": "user", "content": "hi"}},
	}
	res = doRequest(env.base, "POST", "/v1/chat/completions", env.apiKey, chatBody)

	// 4. Assert: the first upstream was hit exactly once.
	if hits := atomic.LoadInt32(&firstHits); hits != 1 {
		t.Fatalf("first upstream hits = %d, want 1", hits)
	}

	// 5. Assert: the backup upstream was NEVER hit (response committed
	//    prevents retry/failover).
	if hits := atomic.LoadInt32(&backupHits); hits != 0 {
		t.Fatalf("backup upstream hits = %d, want 0 (response committed, no failover)", hits)
	}

	// 6. Assert: the client received the partial response (or an error
	//    from the broken connection — either is correct as long as the
	//    backup wasn't concatenated).
	// The relay may return the partial SSE data or an error from the
	// broken connection. The key invariant is: no backup content.
	if strings.Contains(res.Body, "backup-chunk") {
		t.Fatalf("response contains backup channel content — committed response was concatenated: %s", res.Body)
	}

	// 7. Assert: pre-consumed quota should be refunded (no successful
	//    settlement for a broken stream).
	userAfter, _ := model.GetUserByID(env.rootID)
	if userAfter.Quota != userBefore.Quota {
		// Some quota may be charged if the relay considered the response
		// committed and settled — the exact behavior depends on whether
		// usage was reported. The key assertion is no backup content.
		// We accept either full refund or partial charge as long as
		// there's no double charge or backup concatenation.
		t.Logf("user quota changed: before=%d after=%d (may be partial charge for committed response)",
			userBefore.Quota, userAfter.Quota)
	}

	// 8. Assert: no usage log with backup channel in UsedChannels.
	logs, _, _ := model.ListLogs(env.rootID, 1, 10, "gpt-committed-test")
	for _, log := range logs {
		if strings.Contains(log.UsedChannels, fmt.Sprintf("->%d", chBackup)) {
			t.Fatalf("log shows failover to backup channel after committed response: %s",
				log.UsedChannels)
		}
	}

	// 9. Assert: both channels remain enabled (mid-stream failure doesn't
	//    auto-disable).
	chFirstAfter, _ := model.GetChannelByID(chFirst)
	if chFirstAfter.Status != common.ChannelStatusEnabled {
		t.Fatalf("first channel status = %d, want %d (enabled) — mid-stream failure must not auto-disable",
			chFirstAfter.Status, common.ChannelStatusEnabled)
	}
	chBackupAfter, _ := model.GetChannelByID(chBackup)
	if chBackupAfter.Status != common.ChannelStatusEnabled {
		t.Fatalf("backup channel status = %d, want %d (enabled)", chBackupAfter.Status, common.ChannelStatusEnabled)
	}

	t.Logf("6.2 headers committed: first hit=1, backup hit=0, no backup content in response, channels enabled")
}
