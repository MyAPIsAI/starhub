package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"starhub/common"
	"starhub/middleware"
	"starhub/model"
)

func publicationAPIData[T any](t *testing.T, response apiResult) T {
	t.Helper()
	if response.Status != 200 {
		t.Fatalf("publication API: %d %s", response.Status, response.Body)
	}
	var body struct {
		Success bool `json:"success"`
		Data    T    `json:"data"`
	}
	if err := json.Unmarshal([]byte(response.Body), &body); err != nil || !body.Success {
		t.Fatalf("invalid publication envelope: %v %s", err, response.Body)
	}
	return body.Data
}

func TestResourcePublicationActualPolicyReceiptAndGatewayEffects(t *testing.T) {
	env := setupRelayApp(t)
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Authorization") != "Bearer synthetic-publication-key" {
			t.Error("publication changed provider credentials")
		}
		var input struct {
			Model string `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&input)
		if input.Model != "alpha" && input.Model != "beta" {
			t.Errorf("unexpected upstream model: %s", input.Model)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"id":"local-published","model":%q,"choices":[{"index":0,"message":{"role":"assistant","content":"local"}}],"usage":{"prompt_tokens":10,"completion_tokens":5}}`, input.Model)
	}))
	t.Cleanup(upstream.Close)
	publicationAPIData[any](t, doRequest(env.base, "PUT", "/api/option", env.session, map[string]any{"key": "group_ratio", "value": `{"default":1,"vip":1}`}))
	for _, group := range []string{"default", "vip"} {
		publicationAPIData[any](t, doRequest(env.base, "PUT", "/api/service-group", env.session, map[string]any{"group_id": group, "name": group, "enabled": true, "visibility": "common", "multiplier": "1"}))
	}
	channelID := createChannelID(t, env, map[string]any{"name": "Publication channel", "type": 1, "key": "synthetic-publication-key", "base_url": upstream.URL, "models": "alpha,beta", "group": "default,vip", "status": 1})
	snapshot := model.ResourceSnapshot{ID: "publication", Sequence: 1, PublishedAt: "2026-10-29T00:00:00Z", Channels: []model.ResourceChannel{{ID: "inventory-source", Name: "Source", Enabled: true}}, Instances: []model.ResourceInstance{}}
	for _, name := range []string{"alpha", "beta"} {
		snapshot.Instances = append(snapshot.Instances, model.ResourceInstance{ID: "res-" + name, ChannelID: "inventory-source", CanonicalModel: name, UpstreamModel: name, Protocols: []string{"OpenAI"}, Capabilities: []string{"text"}, ContextLength: 128000, Status: "available"})
	}
	state := publicationAPIData[model.ResourceCatalog](t, doRequest(env.base, "PUT", "/api/resource", env.session, map[string]any{"expected_revision": 0, "snapshot": snapshot}))
	for _, item := range []struct{ model, group string }{{"alpha", "default"}, {"beta", "vip"}} {
		state = publicationAPIData[model.ResourceCatalog](t, doRequest(env.base, "PUT", "/api/placement/res-"+item.model, env.session, map[string]any{"expected_revision": state.Revision, "placement": model.ResourcePlacement{ModelID: &item.model, GroupIDs: []string{item.group}, Enabled: true}}))
	}
	context := publicationAPIData[model.ResourcePublicationContext](t, doRequest(env.base, "GET", "/api/resource/publication-context", env.session, nil))
	if !context.PublishAvailable {
		t.Fatal("server did not declare its guarded publication capability")
	}
	bindings := []model.ResourceChannelBinding{{SourceChannelID: "inventory-source", GatewayChannelID: strconv.FormatInt(channelID, 10)}}
	payload := map[string]any{"expected_resource_revision": context.ResourceRevision, "expected_channel_revision": context.ChannelRevision, "expected_option_version": context.OptionVersion, "bindings": bindings}
	plan := publicationAPIData[model.ResourcePublicationPreview](t, doRequest(env.base, "POST", "/api/resource/publication-preview", env.session, payload))
	if !plan.Ready || len(plan.ProposedRoutes) != 2 || len(plan.ExistingPairs) != 4 {
		t.Fatalf("unexpected actual plan: %+v", plan)
	}
	payload["review_digest"] = plan.ReviewDigest
	receiptPath := "/api/resource/publication-receipt/" + plan.ReviewDigest
	if res := doRequest(env.base, "GET", receiptPath, env.session, nil); res.Status != 404 {
		t.Fatalf("uncommitted review acquired a receipt: %d %s", res.Status, res.Body)
	}
	for _, role := range []int{common.RoleUser, common.RoleFinance, 11, 99, 101} {
		user := &model.User{Username: fmt.Sprintf("publish-role-%d", role), Role: role, Status: common.StatusEnabled, AffCode: common.GenerateAffCode()}
		if err := model.CreateUser(user); err != nil {
			t.Fatal(err)
		}
		session := middleware.Sessions.Create(user.ID, time.Hour)
		t.Cleanup(func() { middleware.Sessions.Remove(session) })
		for _, request := range []struct {
			method, path string
			body         any
		}{{"PUT", "/api/resource/publication", payload}, {"GET", receiptPath, nil}, {"GET", "/api/resource/publication-state", nil}} {
			if res := doRequest(env.base, request.method, request.path, session, request.body); res.Status != 403 {
				t.Fatalf("role%d reached publication: %d %s", role, res.Status, res.Body)
			}
		}
	}
	if res := doRequest(env.base, "PUT", "/api/resource/publication", "", payload); res.Status != 401 {
		t.Fatalf("anonymous publication: %d %s", res.Status, res.Body)
	}
	if res := doRequest(env.base, "GET", receiptPath, "", nil); res.Status != 401 {
		t.Fatalf("anonymous receipt: %d %s", res.Status, res.Body)
	}
	if res := doRequest(env.base, "GET", "/api/resource/publication-state", "", nil); res.Status != 401 {
		t.Fatalf("anonymous binding state: %d %s", res.Status, res.Body)
	}
	admin := &model.User{Username: "publication-admin", Role: common.RoleAdmin, Status: common.StatusEnabled, AffCode: common.GenerateAffCode()}
	if err := model.CreateUser(admin); err != nil {
		t.Fatal(err)
	}
	adminSession := middleware.Sessions.Create(admin.ID, time.Hour)
	t.Cleanup(func() { middleware.Sessions.Remove(adminSession) })
	before, err := model.GetUserByID(env.rootID)
	if err != nil {
		t.Fatal(err)
	}
	keyBefore, err := model.GetTokenByID(env.tokenID)
	if err != nil {
		t.Fatal(err)
	}
	channelBefore, err := model.GetChannelByID(channelID)
	if err != nil {
		t.Fatal(err)
	}
	ack := publicationAPIData[model.ResourcePublicationAcknowledgement](t, doRequest(env.base, "PUT", "/api/resource/publication", adminSession, payload))
	if !ack.Published || ack.Operation != "publish" || ack.RouteCount != 2 || ack.ResourceRevision != context.ResourceRevision+1 || ack.ChannelRevision != context.ChannelRevision+1 {
		t.Fatalf("publication ack: %+v", ack)
	}
	for _, session := range []string{adminSession, env.session} {
		result := doRequest(env.base, "GET", "/api/resource/publication-state?page=1&page_size=10", session, nil)
		bindings := publicationAPIData[model.ResourceBindingState](t, result)
		if !bindings.ReadOnly || bindings.Total != 1 || len(bindings.Bindings) != 1 || bindings.ResourceRevision != ack.ResourceRevision || bindings.ChannelRevision != ack.ChannelRevision || bindings.Bindings[0].PublicationID != ack.PublicationID || !bindings.Bindings[0].ConfiguredActive || bindings.Bindings[0].StoredPairCount != 2 || bindings.Bindings[0].RouteEligiblePairCount != 2 {
			t.Fatalf("current published binding state: %+v", bindings)
		}
		if strings.Contains(result.Body, upstream.URL) || strings.Contains(result.Body, "synthetic-publication-key") {
			t.Fatal("binding state exposed upstream credentials")
		}
	}
	after, err := model.GetUserByID(env.rootID)
	if err != nil || after.Quota != before.Quota || calls.Load() != 0 {
		t.Fatalf("publishing charged or sent upstream: %v calls=%d", err, calls.Load())
	}
	keyAfter, err := model.GetTokenByID(env.tokenID)
	if err != nil || !reflect.DeepEqual(keyBefore, keyAfter) {
		t.Fatal("publishing changed key authorization or quota")
	}
	channelAfter, err := model.GetChannelByID(channelID)
	if err != nil || !reflect.DeepEqual(channelBefore, channelAfter) {
		t.Fatal("publishing changed channel credentials/configuration")
	}
	receiptResult := doRequest(env.base, "GET", receiptPath, adminSession, nil)
	receipt := publicationAPIData[model.ResourcePublicationReceipt](t, receiptResult)
	if !receipt.Historical || !receipt.Committed || receipt.ActorID != strconv.FormatInt(admin.ID, 10) || receipt.PublicationID != ack.PublicationID || !reflect.DeepEqual(receipt.Routes, plan.ProposedRoutes) {
		t.Fatalf("receipt lost actual actor or routes: %+v", receipt)
	}
	for _, route := range receipt.Routes {
		if id, err := strconv.ParseInt(route.GroupDefinitionID, 10, 64); err != nil || id <= 0 {
			t.Fatalf("publication lost actual group definition identity: %+v", route)
		}
	}
	if strings.Contains(receiptResult.Body, upstream.URL) || strings.Contains(receiptResult.Body, "synthetic-publication-key") {
		t.Fatal("receipt exposed upstream credentials or address")
	}
	if res := doRequest(env.base, "PUT", "/api/placement/res-alpha", env.session, map[string]any{"expected_revision": state.Revision, "placement": state.Placements["res-alpha"]}); res.Status != 409 {
		t.Fatalf("pre-publication inventory draft was not invalidated: %d %s", res.Status, res.Body)
	}
	if res := doRequest(env.base, "PUT", "/api/resource/publication", adminSession, payload); res.Status != 409 {
		t.Fatalf("same publication was applied twice: %d %s", res.Status, res.Body)
	}
	chat := func(token, name string) apiResult {
		return doRequest(env.base, "POST", "/v1/chat/completions", token, map[string]any{"model": name, "messages": []map[string]string{{"role": "user", "content": "plain"}}, "max_tokens": 10})
	}
	for _, name := range []string{"alpha", "myapis-auto"} {
		if res := chat(env.apiKey, name); res.Status != 200 {
			t.Fatalf("published %s failed: %d %s", name, res.Status, res.Body)
		}
	}
	if res := doRequest(env.base, "POST", "/v1/messages", env.apiKey, map[string]any{"model": "alpha", "messages": []map[string]string{{"role": "user", "content": "plain"}}, "max_tokens": 10}); res.Status != 200 {
		t.Fatalf("published Anthropic text failed: %d %s", res.Status, res.Body)
	}
	deny := func(token, name string) {
		t.Helper()
		owner, err := model.GetUserByID(env.rootID)
		if err != nil {
			t.Fatal(err)
		}
		count := calls.Load()
		if res := chat(token, name); res.Status < 400 {
			t.Fatalf("unpublished %s succeeded: %d %s", name, res.Status, res.Body)
		}
		current, err := model.GetUserByID(env.rootID)
		if err != nil || current.Quota != owner.Quota || calls.Load() != count {
			t.Fatalf("unpublished %s charged or forwarded: %v", name, err)
		}
	}
	deny(env.apiKey, "beta")
	if res := doRequest(env.base, "GET", "/v1/models", env.apiKey, nil); res.Status != 200 || !strings.Contains(res.Body, `"alpha"`) || strings.Contains(res.Body, `"beta"`) {
		t.Fatalf("default model list broadened published pairs: %d %s", res.Status, res.Body)
	}
	vipToken := publicationAPIData[model.Token](t, doRequest(env.base, "POST", "/api/token", env.session, map[string]any{"name": "Published VIP", "group": "vip", "unlimited_quota": true}))
	if res := chat(vipToken.Key, "beta"); res.Status != 200 {
		t.Fatalf("published VIP beta failed: %d %s", res.Status, res.Body)
	}
	deny(vipToken.Key, "alpha")
	state = publicationAPIData[model.ResourceCatalog](t, doRequest(env.base, "GET", "/api/resource", env.session, nil))
	placement := state.Placements["res-alpha"]
	placement.Enabled = false
	publicationAPIData[model.ResourceCatalog](t, doRequest(env.base, "PUT", "/api/placement/res-alpha", env.session, map[string]any{"expected_revision": state.Revision, "placement": placement}))
	deny(env.apiKey, "alpha")
	deny(vipToken.Key, "beta")
	currentBindings := publicationAPIData[model.ResourceBindingState](t, doRequest(env.base, "GET", "/api/resource/publication-state", env.session, nil))
	if len(currentBindings.Bindings) != 1 || currentBindings.Bindings[0].ConfiguredActive || currentBindings.Bindings[0].StoredPairCount != 0 || currentBindings.Bindings[0].RouteEligiblePairCount != 0 || currentBindings.Bindings[0].SourceChannelID != "inventory-source" || currentBindings.Bindings[0].PublicationID != ack.PublicationID {
		t.Fatalf("revoked ownership state lost: %+v", currentBindings)
	}
	if again := publicationAPIData[model.ResourcePublicationReceipt](t, doRequest(env.base, "GET", receiptPath, env.session, nil)); !reflect.DeepEqual(receipt, again) {
		t.Fatal("revocation rewrote the publication receipt")
	}
	if err := model.UpdateUserFields(admin.ID, map[string]any{"role": common.RoleUser}); err != nil {
		t.Fatal(err)
	}
	if res := doRequest(env.base, "GET", receiptPath, adminSession, nil); res.Status != 403 {
		t.Fatalf("revoked admin retained receipt access: %d %s", res.Status, res.Body)
	}
	if res := doRequest(env.base, "GET", "/api/resource/publication-state", adminSession, nil); res.Status != 403 {
		t.Fatalf("revoked admin retained binding state access: %d %s", res.Status, res.Body)
	}
	if res := doRequest(env.base, "PUT", "/api/resource/publication", adminSession, payload); res.Status != 403 {
		t.Fatalf("revoked admin retained publication access: %d %s", res.Status, res.Body)
	}
	var receipts, audits int64
	if err := model.DB.Model(&model.ResourceGatewayPublication{}).Count(&receipts).Error; err != nil || receipts != 1 {
		t.Fatalf("duplicate receipts: %d %v", receipts, err)
	}
	if err := model.DB.Model(&model.AuditLog{}).Where("path = ? AND status = ?", "/api/resource/publication", 200).Count(&audits).Error; err != nil || audits != 1 {
		t.Fatalf("publication audit missing: %d %v", audits, err)
	}
}
