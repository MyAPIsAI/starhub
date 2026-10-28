package main

import (
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

func TestResourceBindingChangeActualPolicyAndGatewayRevocation(t *testing.T) {
	env := setupRelayApp(t)
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"local-binding","model":"alpha","choices":[{"message":{"role":"assistant","content":"local"}}],"usage":{"prompt_tokens":10,"completion_tokens":5}}`)
	}))
	t.Cleanup(upstream.Close)
	id := createChannelID(t, env, map[string]any{"name": "Binding lifecycle", "type": 1, "key": "synthetic-binding-key", "base_url": upstream.URL, "models": "alpha", "group": "default", "status": 1})
	snapshot := model.ResourceSnapshot{ID: "binding-lifecycle", Sequence: 1, PublishedAt: "2026-10-29T00:00:00Z", Channels: []model.ResourceChannel{{ID: "binding-source", Name: "Local source", Enabled: true}}, Instances: []model.ResourceInstance{{ID: "binding-resource", ChannelID: "binding-source", UpstreamModel: "alpha", CanonicalModel: "alpha", Protocols: []string{"OpenAI"}, Capabilities: []string{"text"}, Status: "available"}}}
	catalog := publicationAPIData[model.ResourceCatalog](t, doRequest(env.base, "PUT", "/api/resource", env.session, map[string]any{"expected_revision": 0, "snapshot": snapshot}))
	publicationAPIData[model.ResourceCatalog](t, doRequest(env.base, "PUT", "/api/placement/binding-resource", env.session, map[string]any{"expected_revision": catalog.Revision, "placement": map[string]any{"modelId": "alpha", "groupIds": []string{"default"}, "enabled": true}}))
	publish := func(target int64) model.ResourcePublicationAcknowledgement {
		state := publicationAPIData[model.ResourcePublicationContext](t, doRequest(env.base, "GET", "/api/resource/publication-context", env.session, nil))
		payload := map[string]any{"expected_resource_revision": state.ResourceRevision, "expected_channel_revision": state.ChannelRevision, "expected_option_version": state.OptionVersion, "bindings": []model.ResourceChannelBinding{{SourceChannelID: "binding-source", GatewayChannelID: strconv.FormatInt(target, 10)}}}
		plan := publicationAPIData[model.ResourcePublicationPreview](t, doRequest(env.base, "POST", "/api/resource/publication-preview", env.session, payload))
		if !plan.Ready {
			t.Fatalf("plan blocked: %+v", plan.Issues)
		}
		payload["review_digest"] = plan.ReviewDigest
		return publicationAPIData[model.ResourcePublicationAcknowledgement](t, doRequest(env.base, "PUT", "/api/resource/publication", env.session, payload))
	}
	ack := publish(id)
	state := publicationAPIData[model.ResourceBindingState](t, doRequest(env.base, "GET", "/api/resource/publication-state", env.session, nil))
	command := func(operation, requestID string) map[string]any {
		return map[string]any{"expected_resource_revision": state.ResourceRevision, "expected_channel_revision": state.ChannelRevision, "expected_source_channel_id": "binding-source", "expected_publication_id": ack.PublicationID, "operation": operation, "request_id": requestID}
	}
	path := "/api/resource/bindings/" + strconv.FormatInt(id, 10)
	requestID := strings.Repeat("a", 32)
	receiptPath := "/api/resource/binding-changes/" + requestID
	for _, role := range []int{common.RoleUser, common.RoleFinance, 11, 99, 101} {
		user := &model.User{Username: fmt.Sprintf("binding-role-%d", role), Role: role, Status: 1, AffCode: common.GenerateAffCode()}
		if err := model.CreateUser(user); err != nil {
			t.Fatal(err)
		}
		session := middleware.Sessions.Create(user.ID, time.Hour)
		t.Cleanup(func() { middleware.Sessions.Remove(session) })
		if res := doRequest(env.base, "PUT", path, session, command("revoke", requestID)); res.Status != 403 {
			t.Fatalf("role%d reached binding write: %d", role, res.Status)
		}
		if res := doRequest(env.base, "GET", receiptPath, session, nil); res.Status != 403 {
			t.Fatalf("role%d reached binding receipt: %d", role, res.Status)
		}
	}
	if res := doRequest(env.base, "PUT", path, "", command("revoke", requestID)); res.Status != 401 {
		t.Fatal("anonymous binding write allowed")
	}
	if res := doRequest(env.base, "GET", receiptPath, "", nil); res.Status != 401 {
		t.Fatal("anonymous binding receipt allowed")
	}
	if res := doRequest(env.base, "GET", receiptPath, env.session, nil); res.Status != 404 {
		t.Fatal("uncommitted binding operation has receipt")
	}
	admin := &model.User{Username: "binding-admin", Role: common.RoleAdmin, Status: 1, AffCode: common.GenerateAffCode()}
	if err := model.CreateUser(admin); err != nil {
		t.Fatal(err)
	}
	session := middleware.Sessions.Create(admin.ID, time.Hour)
	t.Cleanup(func() { middleware.Sessions.Remove(session) })
	before, _ := model.GetUserByID(env.rootID)
	keyBefore, _ := model.GetTokenByID(env.tokenID)
	channelBefore, _ := model.GetChannelByID(id)
	if err := model.DB.Exec(`CREATE TRIGGER fail_http_binding BEFORE DELETE ON abilities BEGIN SELECT RAISE(ABORT,'private failure'); END`).Error; err != nil {
		t.Fatal(err)
	}
	if res := doRequest(env.base, "PUT", path, session, command("revoke", requestID)); res.Status != 500 || strings.Contains(res.Body, "private failure") || strings.Contains(res.Body, "abilities") {
		t.Fatalf("failed transaction leaked or succeeded: %d %s", res.Status, res.Body)
	}
	if err := model.DB.Exec("DROP TRIGGER fail_http_binding").Error; err != nil {
		t.Fatal(err)
	}
	if current := publicationAPIData[model.ResourceBindingState](t, doRequest(env.base, "GET", "/api/resource/publication-state", env.session, nil)); !reflect.DeepEqual(state, current) {
		t.Fatal("failed HTTP change left partial state")
	}
	revoke := publicationAPIData[model.ResourceBindingChangeReceipt](t, doRequest(env.base, "PUT", path, session, command("revoke", requestID)))
	if revoke.ActorID != strconv.FormatInt(admin.ID, 10) || revoke.Operation != "revoke" || revoke.RetainedSourceChannelID != "binding-source" || revoke.RemovedRouteCount != 1 || !revoke.ProtectiveMarkerRetained || !revoke.Historical || !revoke.Committed {
		t.Fatalf("revoke receipt: %+v", revoke)
	}
	if again := publicationAPIData[model.ResourceBindingChangeReceipt](t, doRequest(env.base, "GET", receiptPath, env.session, nil)); !reflect.DeepEqual(revoke, again) {
		t.Fatal("Root could not verify durable receipt")
	}
	if res := doRequest(env.base, "PUT", path, session, command("revoke", requestID)); res.Status != 409 {
		t.Fatal("operation repeated")
	}
	deny := func() {
		t.Helper()
		count := calls.Load()
		current, _ := model.GetUserByID(env.rootID)
		res := doRequest(env.base, "POST", "/v1/chat/completions", env.apiKey, map[string]any{"model": "alpha", "messages": []map[string]string{{"role": "user", "content": "local"}}, "max_tokens": 10})
		after, _ := model.GetUserByID(env.rootID)
		if res.Status < 400 || calls.Load() != count || current.Quota != after.Quota {
			t.Fatalf("revoked call forwarded or charged: %d", res.Status)
		}
	}
	deny()
	ack = publish(id)
	state = publicationAPIData[model.ResourceBindingState](t, doRequest(env.base, "GET", "/api/resource/publication-state", env.session, nil))
	release := publicationAPIData[model.ResourceBindingChangeReceipt](t, doRequest(env.base, "PUT", path, session, command("release", strings.Repeat("b", 32))))
	if release.RetainedSourceChannelID != "" || release.PublicationID != ack.PublicationID || release.Operation != "release" {
		t.Fatalf("release receipt: %+v", release)
	}
	deny()
	current := publicationAPIData[model.ResourceBindingState](t, doRequest(env.base, "GET", "/api/resource/publication-state", env.session, nil))
	if current.Bindings[0].SourceChannelID != "" || current.Bindings[0].ConfiguredActive || current.Bindings[0].RouteEligiblePairCount != 0 {
		t.Fatal("release removed protective marker")
	}
	after, _ := model.GetUserByID(env.rootID)
	keyAfter, _ := model.GetTokenByID(env.tokenID)
	channelAfter, _ := model.GetChannelByID(id)
	if before.Quota != after.Quota || calls.Load() != 0 || !reflect.DeepEqual(keyBefore, keyAfter) || !reflect.DeepEqual(channelBefore, channelAfter) {
		t.Fatal("binding management changed money, token or credentials")
	}
	if again := publicationAPIData[model.ResourceBindingChangeReceipt](t, doRequest(env.base, "GET", receiptPath, session, nil)); !reflect.DeepEqual(revoke, again) {
		t.Fatal("republish/release rewrote earlier receipt")
	}
	next := createChannelID(t, env, map[string]any{"name": "Explicit replacement", "type": 1, "key": "synthetic-binding-key", "base_url": upstream.URL, "models": "alpha", "group": "default", "status": 1})
	publish(next)
	if res := doRequest(env.base, "POST", "/v1/chat/completions", env.apiKey, map[string]any{"model": "alpha", "messages": []map[string]string{{"role": "user", "content": "local"}}, "max_tokens": 10}); res.Status != 200 || calls.Load() != 1 {
		t.Fatalf("explicit replacement unavailable: %d %s", res.Status, res.Body)
	}
	if err := model.UpdateUserFields(admin.ID, map[string]any{"role": common.RoleUser}); err != nil {
		t.Fatal(err)
	}
	if res := doRequest(env.base, "GET", receiptPath, session, nil); res.Status != 403 {
		t.Fatal("demoted admin kept receipt access")
	}
	if res := doRequest(env.base, "PUT", path, session, command("release", strings.Repeat("c", 32))); res.Status != 403 {
		t.Fatal("demoted admin kept binding write")
	}
	var changes, audits int64
	if err := model.DB.Model(&model.ResourceGatewayBindingChange{}).Count(&changes).Error; err != nil || changes != 2 {
		t.Fatalf("changes=%d %v", changes, err)
	}
	if err := model.DB.Model(&model.AuditLog{}).Where("path = ? AND status = ?", path, 200).Count(&audits).Error; err != nil || audits != 2 {
		t.Fatalf("audits=%d %v", audits, err)
	}
}
