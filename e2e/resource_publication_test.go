package main

import (
	"context"
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

func TestResourcePublicationActualPolicyPreviewAndStaleContext(t *testing.T) {
	env := setupRelayApp(t)
	var upstreamCalls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { upstreamCalls.Add(1); w.WriteHeader(500) }))
	t.Cleanup(upstream.Close)
	channel := &model.Channel{ID: 9007199254740993, Name: "Actual gateway", Type: 1, Key: "private-publication-key", BaseURL: upstream.URL + "/private", Models: "alpha,beta", Group: "default", Status: 1}
	if err := model.InsertChannel(channel); err != nil {
		t.Fatal(err)
	}
	snapshot := model.ResourceSnapshot{ID: "preview-inventory", Sequence: 1, PublishedAt: "2026-10-29T00:00:00Z", Channels: []model.ResourceChannel{{ID: "9007199254740993", Name: "Source identity", Enabled: true}}, Instances: []model.ResourceInstance{{ID: "resource", ChannelID: "9007199254740993", CanonicalModel: "alpha", UpstreamModel: "alpha", Protocols: []string{"OpenAI"}, Capabilities: []string{"text"}, Status: "available"}}}
	if _, err := model.SaveResourceSnapshot(context.Background(), 0, snapshot); err != nil {
		t.Fatal(err)
	}
	modelID := "alpha"
	if _, err := model.SaveResourcePlacement(context.Background(), 1, "resource", model.ResourcePlacement{ModelID: &modelID, GroupIDs: []string{"default"}, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	contextResult := doRequest(env.base, "GET", "/api/resource/publication-context", env.session, nil)
	if contextResult.Status != 200 {
		t.Fatalf("context: %d %s", contextResult.Status, contextResult.Body)
	}
	var contextBody struct {
		Data model.ResourcePublicationContext `json:"data"`
	}
	if err := json.Unmarshal([]byte(contextResult.Body), &contextBody); err != nil {
		t.Fatal(err)
	}
	current := contextBody.Data
	if !current.PreviewOnly || current.AffectsGatewayRouting || len(current.Channels) != 1 || current.Channels[0].ID != "9007199254740993" {
		t.Fatalf("unsafe or lossy context: %+v", current)
	}
	payload := map[string]any{"expected_resource_revision": current.ResourceRevision, "expected_channel_revision": current.ChannelRevision, "expected_option_version": current.OptionVersion, "bindings": []model.ResourceChannelBinding{{SourceChannelID: "9007199254740993", GatewayChannelID: strconv.FormatInt(channel.ID, 10)}}}
	for _, role := range []int{common.RoleUser, common.RoleFinance, 11, 99, 101} {
		user := &model.User{Username: fmt.Sprintf("publication-role-%d", role), Role: role, Status: 1, AffCode: common.GenerateAffCode()}
		if err := model.CreateUser(user); err != nil {
			t.Fatal(err)
		}
		session := middleware.Sessions.Create(user.ID, time.Hour)
		t.Cleanup(func() { middleware.Sessions.Remove(session) })
		for _, request := range []struct {
			method, path string
			body         any
		}{{"GET", "/api/resource/publication-context", nil}, {"POST", "/api/resource/publication-preview", payload}} {
			res := doRequest(env.base, request.method, request.path, session, request.body)
			if res.Status != 403 {
				t.Fatalf("role %d reached %s: %d %s", role, request.path, res.Status, res.Body)
			}
		}
	}
	for _, path := range []string{"/api/resource/publication-context", "/api/resource/publication-preview"} {
		method := "GET"
		if strings.HasSuffix(path, "preview") {
			method = "POST"
		}
		if res := doRequest(env.base, method, path, "", payload); res.Status != 401 {
			t.Fatalf("anonymous context: %d %s", res.Status, res.Body)
		}
	}
	admin := &model.User{Username: "publication-admin", Role: common.RoleAdmin, Status: 1, AffCode: common.GenerateAffCode()}
	if err := model.CreateUser(admin); err != nil {
		t.Fatal(err)
	}
	adminSession := middleware.Sessions.Create(admin.ID, time.Hour)
	t.Cleanup(func() { middleware.Sessions.Remove(adminSession) })
	rootBefore, err := model.GetUserByID(env.rootID)
	if err != nil {
		t.Fatal(err)
	}
	for _, session := range []string{adminSession, env.session} {
		res := doRequest(env.base, "POST", "/api/resource/publication-preview", session, payload)
		if res.Status != 200 {
			t.Fatalf("authorized preview: %d %s", res.Status, res.Body)
		}
		var body struct {
			Data model.ResourcePublicationPreview `json:"data"`
		}
		if err := json.Unmarshal([]byte(res.Body), &body); err != nil {
			t.Fatal(err)
		}
		if !body.Data.Ready || !body.Data.PreviewOnly || body.Data.AffectsGatewayRouting || len(body.Data.ProposedPairs) != 1 || len(body.Data.ExistingPairs) != 2 {
			t.Fatalf("preview result: %+v", body.Data)
		}
		for _, secret := range []string{channel.Key, upstream.URL, `"base_url"`, `"key"`} {
			if strings.Contains(res.Body, secret) || strings.Contains(contextResult.Body, secret) {
				t.Fatalf("secret leaked: %s", secret)
			}
		}
	}
	stored, err := model.GetChannelByID(channel.ID)
	if err != nil || !reflect.DeepEqual(stored, channel) {
		t.Fatalf("preview changed gateway: %+v %v", stored, err)
	}
	rootAfter, err := model.GetUserByID(env.rootID)
	if err != nil || rootBefore.Quota != rootAfter.Quota || upstreamCalls.Load() != 0 {
		t.Fatalf("preview touched money/provider: %v calls=%d", err, upstreamCalls.Load())
	}
	if res := doRequest(env.base, "PUT", "/api/channel", env.session, map[string]any{"id": channel.ID, "name": "Changed independently"}); res.Status != 200 {
		t.Fatalf("independent edit: %d %s", res.Status, res.Body)
	}
	if res := doRequest(env.base, "POST", "/api/resource/publication-preview", env.session, payload); res.Status != 409 {
		t.Fatalf("stale channel context accepted: %d %s", res.Status, res.Body)
	}
	if err := model.UpdateUserFields(admin.ID, map[string]any{"role": common.RoleUser}); err != nil {
		t.Fatal(err)
	}
	if res := doRequest(env.base, "POST", "/api/resource/publication-preview", adminSession, payload); res.Status != 403 {
		t.Fatalf("revoked admin retained preview: %d %s", res.Status, res.Body)
	}
}
