//go:build affinitycheck

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"starhub/model"
	"starhub/setting"
)

const affinityProbeModel = "gpt-4o-mini"

type affinityProbeUpstream struct {
	server *httptest.Server
	fail   atomic.Bool
	hits   atomic.Int64
}

func newAffinityProbeUpstream(t *testing.T, marker string) *affinityProbeUpstream {
	t.Helper()
	upstream := &affinityProbeUpstream{}
	upstream.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstream.hits.Add(1)
		if r.URL.Path != "/v1/chat/completions" ||
			r.Header.Get("Authorization") != "Bearer "+upstreamKey {
			http.Error(w, "unexpected upstream request", http.StatusBadRequest)
			return
		}
		var req struct {
			Model    string            `json:"model"`
			User     string            `json:"user"`
			Metadata map[string]string `json:"metadata"`
			Messages []json.RawMessage `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil ||
			req.Model != affinityProbeModel || req.User != "affinity-probe-user" ||
			req.Metadata["conversation_id"] == "" || len(req.Messages) == 0 {
			http.Error(w, "invalid conversation request", http.StatusBadRequest)
			return
		}
		if upstream.fail.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"id":"affinity-probe","object":"chat.completion","model":%q,"choices":[{"index":0,"message":{"role":"assistant","content":%q}}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`, affinityProbeModel, marker)
	}))
	t.Cleanup(upstream.server.Close)
	return upstream
}

func setupAffinityProbe(t *testing.T) (p1Env, *affinityProbeUpstream, *affinityProbeUpstream) {
	t.Helper()
	a := newAffinityProbeUpstream(t, "A")
	b := newAffinityProbeUpstream(t, "B")
	env := setupRelayApp(t)
	model.ResetAffinity()
	t.Cleanup(model.ResetAffinity)
	t.Cleanup(setting.ResetAffinityToDefaults)
	for _, channel := range []struct {
		name string
		url  string
	}{{"affinity-A", a.server.URL}, {"affinity-B", b.server.URL}} {
		createChannelID(t, env, map[string]any{
			"name": channel.name, "type": 1, "key": upstreamKey,
			"base_url": channel.url, "models": affinityProbeModel,
			"group": "default", "priority": 0, "weight": 1,
		})
	}
	return env, a, b
}

func setAffinityProbeOption(t *testing.T, env p1Env, key, value string) {
	t.Helper()
	res := doRequest(env.base, http.MethodPut, "/api/option/", env.session,
		map[string]string{"key": key, "value": value})
	if res.Status != http.StatusOK {
		t.Fatalf("set %s=%s failed: %d %s", key, value, res.Status, res.Body)
	}
}

type affinityProbeConversation struct {
	id       string
	messages []map[string]string
}

func newAffinityProbeConversation(id string) *affinityProbeConversation {
	return &affinityProbeConversation{
		id:       id,
		messages: []map[string]string{{"role": "user", "content": "Start this conversation."}},
	}
}

func (c *affinityProbeConversation) call(t *testing.T, env p1Env) string {
	t.Helper()
	res := doRequest(env.base, http.MethodPost, "/v1/chat/completions", env.apiKey,
		map[string]any{
			"model": affinityProbeModel, "user": "affinity-probe-user",
			"metadata": map[string]string{"conversation_id": c.id},
			"messages": c.messages,
		})
	if res.Status != http.StatusOK {
		t.Fatalf("conversation %s turn %d returned %d: %s", c.id, len(c.messages)/2+1, res.Status, res.Body)
	}
	var reply struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal([]byte(res.Body), &reply); err != nil ||
		len(reply.Choices) != 1 || (reply.Choices[0].Message.Content != "A" && reply.Choices[0].Message.Content != "B") {
		t.Fatalf("invalid channel marker: %s", res.Body)
	}
	marker := reply.Choices[0].Message.Content
	c.messages = append(c.messages,
		map[string]string{"role": "assistant", "content": marker},
		map[string]string{"role": "user", "content": fmt.Sprintf("Continue turn %d.", len(c.messages)/2+2)})
	return marker
}

func probeAffinitySequence(t *testing.T, env p1Env, c *affinityProbeConversation, count int) []string {
	t.Helper()
	sequence := make([]string, 0, count)
	for i := 0; i < count; i++ {
		sequence = append(sequence, c.call(t, env))
	}
	return sequence
}

func affinityTransitions(sequence []string) int {
	transitions := 0
	for i := 1; i < len(sequence); i++ {
		if sequence[i] != sequence[i-1] {
			transitions++
		}
	}
	return transitions
}

// Run with: go test -tags=affinitycheck ./e2e -run '^TestChannelAffinityDiagnostic' -count=1 -v
func TestChannelAffinityDiagnosticSticky(t *testing.T) {
	env, a, b := setupAffinityProbe(t)
	conversation := newAffinityProbeConversation("conversation-main")
	if setting.RoutingAffinityEnabled() {
		t.Fatal("the default affinity switch should be off")
	}

	baseline := probeAffinitySequence(t, env, conversation, 8)
	if _, ok := model.AffinityChannel(model.AffinityKey{
		TokenID: env.tokenID, Model: affinityProbeModel, Group: "default",
	}); ok {
		t.Fatal("disabled affinity unexpectedly stored a channel preference")
	}
	t.Logf("disabled: requests=%d transitions=%d sequence=%s",
		len(baseline), affinityTransitions(baseline), strings.Join(baseline, ""))

	setAffinityProbeOption(t, env, setting.OptionKeyRoutingAffinityEnabled, "true")
	sticky := probeAffinitySequence(t, env, conversation, 32)
	t.Logf("sticky: requests=%d transitions=%d A=%d B=%d sequence=%s",
		len(sticky), affinityTransitions(sticky), countAffinityMarker(sticky, "A"),
		countAffinityMarker(sticky, "B"), strings.Join(sticky, ""))
	if got := affinityTransitions(sticky); got != 0 {
		t.Errorf("enabled sticky affinity switched a healthy conversation %d times", got)
	}

	otherSessions := make([]string, 0, 8)
	for i := 0; i < 8; i++ {
		otherSessions = append(otherSessions,
			newAffinityProbeConversation(fmt.Sprintf("conversation-other-%d", i)).call(t, env))
	}
	t.Logf("other conversations with same token/model/group: sequence=%s",
		strings.Join(otherSessions, ""))

	if sticky[0] == "A" {
		a.fail.Store(true)
	} else {
		b.fail.Store(true)
	}
	fallback := conversation.call(t, env)
	if fallback == sticky[0] {
		t.Fatal("failed preferred channel did not fail over")
	}
	afterFallback := probeAffinitySequence(t, env, conversation, 6)
	t.Logf("failover: initial=%s fallback=%s subsequent=%s A_hits=%d B_hits=%d",
		sticky[0], fallback, strings.Join(afterFallback, ""), a.hits.Load(), b.hits.Load())
	for i, marker := range afterFallback {
		if marker != fallback {
			t.Errorf("post-failover turn %d used %s, want %s", i+1, marker, fallback)
		}
	}
}

func TestChannelAffinityDiagnosticHash(t *testing.T) {
	env, a, b := setupAffinityProbe(t)
	setAffinityProbeOption(t, env, setting.OptionKeyRoutingAffinityEnabled, "true")
	setAffinityProbeOption(t, env, setting.OptionKeyRoutingAffinityMode, "hash")
	conversation := newAffinityProbeConversation("conversation-hash")

	sequence := make([]string, 0, 16)
	for i := 0; i < 16; i++ {
		model.ResetAffinity()
		sequence = append(sequence, conversation.call(t, env))
	}
	t.Logf("hash across affinity-store resets: requests=%d transitions=%d sequence=%s",
		len(sequence), affinityTransitions(sequence), strings.Join(sequence, ""))
	if got := affinityTransitions(sequence); got != 0 {
		t.Errorf("hash affinity switched a healthy conversation %d times", got)
	}

	primary := a
	if sequence[0] == "B" {
		primary = b
	}
	primary.fail.Store(true)
	hitsBefore := primary.hits.Load()
	fallback := probeAffinitySequence(t, env, conversation, 3)
	t.Logf("hash after primary 5xx: responses=%s failed_primary_attempts=%d",
		strings.Join(fallback, ""), primary.hits.Load()-hitsBefore)
	for i, marker := range fallback {
		if marker == sequence[0] {
			t.Errorf("hash failover turn %d returned the failed primary", i+1)
		}
	}
}

func countAffinityMarker(sequence []string, marker string) int {
	count := 0
	for _, item := range sequence {
		if item == marker {
			count++
		}
	}
	return count
}
