package main

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"starhub/common"
	"starhub/internal/router"
	"starhub/model"
	"starhub/relay"
)

func TestKeyGatewayProcess(t *testing.T) {
	if os.Getenv("STARHUB_KEY_TEST_GATEWAY") != "1" {
		return
	}
	if err := model.InitDB(os.Getenv("STARHUB_KEY_TEST_DATABASE")); err != nil {
		t.Fatal(err)
	}
	defer model.CloseDB()
	if err := model.LoadOptionsToSettings(); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("KEY_GATEWAY_URL=http://%s\n", listener.Addr())
	if err := http.Serve(listener, router.New()); err != nil {
		t.Fatal(err)
	}
}

func keyGatewayProcess(t *testing.T) string {
	t.Helper()
	var databases []struct {
		Name string
		File string
	}
	if err := model.DB.Raw("PRAGMA database_list").Scan(&databases).Error; err != nil {
		t.Fatal(err)
	}
	var database string
	for _, item := range databases {
		if item.Name == "main" {
			database = item.File
		}
	}
	if database == "" {
		t.Fatal("missing gateway database")
	}
	command := exec.Command(os.Args[0], "-test.run=^TestKeyGatewayProcess$", "-test.timeout=90s")
	command.Env = append(os.Environ(), "STARHUB_KEY_TEST_GATEWAY=1", "STARHUB_KEY_TEST_DATABASE="+database)
	var failure bytes.Buffer
	command.Stderr = &failure
	output, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = command.Process.Kill(); _ = command.Wait() })
	ready := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(output)
		for scanner.Scan() {
			if strings.HasPrefix(scanner.Text(), "KEY_GATEWAY_URL=") {
				ready <- strings.TrimPrefix(scanner.Text(), "KEY_GATEWAY_URL=")
				return
			}
		}
		ready <- ""
	}()
	select {
	case base := <-ready:
		if base == "" {
			t.Fatal("gateway failed to start")
		}
		return base
	case <-time.After(20 * time.Second):
		t.Fatal("gateway startup timed out")
	}
	return ""
}

func TestKeyAC02AC04AC18AC21AcrossGatewayProcesses(t *testing.T) {
	env := setupRelayApp(t)
	setOption(t, env, "group_ratio", `{"default":1,"pro":0.8,"ultra":1}`)
	setOption(t, env, "token_rate_limit", "10000")
	setOption(t, env, "user_rate_limit", "10000")
	started, release := make(chan struct{}, 1), make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Test-Hold") == "1" {
			t.Error("client-only headers must not control upstream")
		}
		var body bytes.Buffer
		_, _ = body.ReadFrom(r.Body)
		if strings.Contains(body.String(), "hold-key-request") {
			started <- struct{}{}
			<-release
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"key-acceptance","choices":[{"message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":100,"completion_tokens":50}}`)
	}))
	defer func() { close(release); upstream.Close() }()
	pro := createChannelID(t, env, map[string]any{"name": "pro resource", "key": upstreamKey, "base_url": upstream.URL, "models": "gpt-4o-mini", "group": "pro"})
	ultra := createChannelID(t, env, map[string]any{"name": "ultra resource", "key": upstreamKey, "base_url": upstream.URL, "models": "gpt-4o-mini", "group": "ultra"})
	if err := model.UpdateTokenFields(env.tokenID, map[string]any{"group": "pro", "unlimited_quota": true}); err != nil {
		t.Fatal(err)
	}
	gateways := []string{keyGatewayProcess(t), keyGatewayProcess(t)}
	body := func(message string) map[string]any {
		return map[string]any{"model": "gpt-4o-mini", "max_tokens": 64, "messages": []map[string]string{{"role": "user", "content": message}}}
	}
	for _, base := range gateways {
		if response := doRequest(base, "POST", "/v1/chat/completions", env.apiKey, body("before")); response.Status != 200 {
			t.Fatalf("initial call: %d %s", response.Status, response.Body)
		}
	}
	pending := make(chan apiResult, 1)
	go func() {
		pending <- doRequest(gateways[0], "POST", "/v1/chat/completions", env.apiKey, body("hold-key-request"))
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("accepted request never reached upstream")
	}
	if response := doRequest(env.base, "PUT", "/api/token", env.session, map[string]any{"id": env.tokenID, "group": "ultra"}); response.Status != 200 {
		t.Fatalf("group change: %s", response.Body)
	}
	for _, base := range gateways {
		if response := doRequest(base, "POST", "/v1/chat/completions", env.apiKey, body("after")); response.Status != 200 {
			t.Fatalf("changed group call: %d %s", response.Status, response.Body)
		}
	}
	if response := doRequest(env.base, "PUT", "/api/token", env.session, map[string]any{"id": env.tokenID, "status": 2}); response.Status != 200 {
		t.Fatal(response.Body)
	}
	stop := time.Now().Add(5*time.Second + 100*time.Millisecond)
	for time.Now().Before(stop) {
		for _, base := range gateways {
			response := doRequest(base, "POST", "/v1/chat/completions", env.apiKey, body("disabled"))
			if response.Status != 403 || !strings.Contains(response.Body, "key_disabled") {
				t.Fatalf("disable not propagated: %d %s", response.Status, response.Body)
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	for _, base := range gateways {
		response := doRequest(base, "POST", "/v1/chat/completions", env.apiKey, body("after-deadline"))
		if response.Status != 403 || !strings.Contains(response.Body, "key_disabled") {
			t.Fatal("five-second disable contract failed")
		}
	}
	release <- struct{}{}
	select {
	case response := <-pending:
		if response.Status != 200 {
			t.Fatalf("accepted request was stopped: %s", response.Body)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("accepted request did not finish")
	}
	logs, _, err := model.ListLogs(env.rootID, 1, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	counts := map[int64]int{}
	for _, entry := range logs {
		counts[entry.ChannelID]++
		group := "pro"
		if entry.ChannelID == ultra {
			group = "ultra"
		}
		want := relay.ComputeQuota("gpt-4o-mini", group, 100, 50)
		if entry.Quota != want {
			t.Fatalf("accepted pricing changed: channel=%d got=%d want=%d", entry.ChannelID, entry.Quota, want)
		}
	}
	if counts[pro] != 3 || counts[ultra] != 2 {
		t.Fatalf("wrong group resources: %v", counts)
	}
	before, _ := model.GetTokenByID(env.tokenID)
	response := doRequest(env.base, "POST", fmt.Sprintf("/api/token/%d/regenerate", env.tokenID), env.session, nil)
	if response.Status != 200 {
		t.Fatal(response.Body)
	}
	fresh, _ := model.GetTokenByID(env.tokenID)
	if fresh.UsedQuota != before.UsedQuota || fresh.Group != before.Group {
		t.Fatal("regeneration changed attributes or usage")
	}
	if err := model.UpdateTokenFields(env.tokenID, map[string]any{"status": 1}); err != nil {
		t.Fatal(err)
	}
	for _, base := range gateways {
		old := doRequest(base, "POST", "/v1/chat/completions", env.apiKey, body("old"))
		if old.Status != 401 || !strings.Contains(old.Body, "invalid_api_key") {
			t.Fatal("old key remained usable")
		}
		if next := doRequest(base, "POST", "/v1/chat/completions", fresh.Key, body("new")); next.Status != 200 {
			t.Fatalf("new key unusable: %s", next.Body)
		}
	}
}

func TestKeyAC10AC11AC12AC19AC20ActualRouter(t *testing.T) {
	env := setupRelayApp(t)
	upstream := newMockUpstream(t)
	createChannelID(t, env, map[string]any{"name": "IP resource", "key": upstreamKey, "base_url": upstream.URL, "models": "gpt-4o-mini", "group": "default"})
	if err := common.ConfigureTrustedProxies([]string{"192.0.2.0/24"}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = common.ConfigureTrustedProxies(nil) })
	app := router.New()
	for _, test := range []struct {
		allow, peer, forwarded string
		expired                int64
		status, want           int
		code                   string
	}{
		{"10.0.0.0/24", "10.0.0.5:1234", "", -1, 1, 200, ""},
		{"10.0.0.0/24", "10.0.1.5:1234", "", -1, 1, 403, "ip_not_allowed"},
		{"203.0.113.0/24", "192.0.2.10:443", "203.0.113.9, 198.51.100.7", -1, 1, 403, "ip_not_allowed"},
		{"203.0.113.0/24", "192.0.2.10:443", "203.0.113.9, 192.0.2.11", -1, 1, 200, ""},
		{"2001:db8::/32", "[2001:db8:0:0::1]:1234", "", -1, 1, 200, ""},
		{"2001:db8::/32", "[2001:db9::1]:1234", "", -1, 1, 403, "ip_not_allowed"},
		{"", "198.51.100.7:1234", "", time.Now().Add(-time.Hour).Unix(), 1, 401, "key_expired"},
		{"", "198.51.100.7:1234", "", -1, 2, 403, "key_disabled"},
		{"", "198.51.100.7:1234", "", -1, 1, 200, ""},
	} {
		if err := model.UpdateTokenFields(env.tokenID, map[string]any{"allow_ips": test.allow, "expired_time": test.expired, "status": test.status}); err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hello"}],"max_tokens":64}`))
		request.RemoteAddr = test.peer
		request.Header.Set("Authorization", "Bearer "+env.apiKey)
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Forwarded-For", test.forwarded)
		response := httptest.NewRecorder()
		app.ServeHTTP(response, request)
		if response.Code != test.want || !strings.Contains(response.Body.String(), test.code) {
			t.Fatalf("allow=%s peer=%s got %d %s", test.allow, test.peer, response.Code, response.Body)
		}
	}
}

func TestKey5aRetiredModelRemainsInTheKeyPolicy(t *testing.T) {
	env := setupRelayApp(t)
	upstream := newMockUpstream(t)
	channel := createChannelID(t, env, map[string]any{"name": "retired model", "key": upstreamKey, "base_url": upstream.URL, "models": "retired-model", "group": "default"})
	if err := model.UpdateTokenFields(env.tokenID, map[string]any{"model_limits": "retired-model"}); err != nil {
		t.Fatal(err)
	}
	body := map[string]any{"model": "retired-model", "max_tokens": 8, "messages": []map[string]string{{"role": "user", "content": "hello"}}}
	if response := doRequest(env.base, "POST", "/v1/chat/completions", env.apiKey, body); response.Status != 200 {
		t.Fatal(response.Body)
	}
	if err := model.DeleteChannel(channel); err != nil {
		t.Fatal(err)
	}
	before, _ := model.GetTokenByID(env.tokenID)
	response := doRequest(env.base, "POST", "/v1/chat/completions", env.apiKey, body)
	if response.Status != 404 || !strings.Contains(response.Body, "model_not_found") {
		t.Fatalf("retired model: %d %s", response.Status, response.Body)
	}
	after, _ := model.GetTokenByID(env.tokenID)
	if after.ModelLimits != "retired-model" || after.UsedQuota != before.UsedQuota || after.RemainQuota != before.RemainQuota {
		t.Fatal("retirement changed the policy or charged a failed admission")
	}
	response = doRequest(env.base, "POST", "/api/token/owned-action", env.session, keyActionFixture(t, env, env.rootID, "update", env.tokenID, map[string]any{"name": "Retained policy"}))
	if response.Status != 200 {
		t.Fatalf("retired policy prevented metadata editing: %s", response.Body)
	}
	after, _ = model.GetTokenByID(env.tokenID)
	if after.ModelLimits != "retired-model" {
		t.Fatal("metadata editing silently removed a retired model")
	}
}
