package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"starhub/config"
	"starhub/controller"
	"starhub/middleware"
	"starhub/model"
)

// The fixture uses the actual router, SQLite transactions, staff onboarding and
// gateway settlement. The upstream and credentials are synthetic test data.
func TestM7AdministrationAcceptance(t *testing.T) {
	handler, root := staffFixture(t)
	rootSeed := root.enroll("root", "Root-temporary-123")
	if err := model.SeedOptions(); err != nil {
		t.Fatal(err)
	}
	if err := model.LoadOptionsToSettings(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = config.SetRegionTimezone("UTC") })
	roles := map[int]*staffHTTP{100: root}
	for _, role := range []int{10, 5, 20} {
		name := fmt.Sprintf("m7-staff-%d", role)
		root.call("POST", "/api/admin/staff", map[string]any{"username": name, "temporary_password": "M7-temporary-123", "role": role, "reason": "Acceptance fixture"}, 200)
		c := &staffHTTP{t: t, handler: handler, ip: fmt.Sprintf("127.0.0.1:%d", 18000+role)}
		c.enroll(name, "M7-temporary-123")
		roles[role] = c
	}
	ops, finance, tech := roles[10], roles[5], roles[20]
	user := &model.User{Username: "m7-customer", Role: 1, Status: 1, Group: "default", Quota: 100000000, AffCode: "m7-customer", Phone: "13812345678", Email: "synthetic@example.invalid"}
	if err := model.CreateUser(user); err != nil {
		t.Fatal(err)
	}
	if err := model.DB.Create(&model.LedgerEntry{UserID: user.ID, Type: "adjustment", FundingSource: model.FundingSourceStaffAdjustment, Amount: user.Quota * 2000, BalanceAfter: user.Quota * 2000, Description: "Synthetic opening credit"}).Error; err != nil {
		t.Fatal(err)
	}
	key := &model.Token{UserID: user.ID, Name: "Acceptance key", Status: 1, Group: "default", UnlimitedQuota: true, ModelLimits: "m7-model"}
	if err := model.InsertToken(key); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	var requests atomic.Int64
	var block atomic.Bool
	accepted, release := make(chan struct{}, 1), make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if block.Load() {
			accepted <- struct{}{}
			<-release
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"m7-test","object":"chat.completion","model":"m7-model","choices":[{"index":0,"message":{"role":"assistant","content":"Synthetic private response"}}],"usage":{"prompt_tokens":1000,"completion_tokens":100}}`)
	}))
	defer upstream.Close()
	ch := &model.Channel{Name: "Synthetic upstream", Type: 1, Key: "M7-synthetic-upstream-secret", BaseURL: upstream.URL, Models: "m7-model", Group: "default", Status: 1}
	var canonical model.StandardModel
	write := func(listed bool, meta model.ModelMeta, options []model.Option, want int) {
		t.Helper()
		catalog, err := model.ReadModelMetadataCatalog(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		pricing, err := model.ReadPricingOptions(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		meta.Model = "m7-model"
		canonical.Model = "m7-model"
		canonical.Name = "M7 model"
		canonical.Vendor = "OpenAI"
		canonical.Kind = "chat"
		canonical.Protocols = []string{"OpenAI"}
		canonical.Capabilities = []string{"text"}
		canonical.Listed = listed
		ops.call("PUT", "/api/admin/models", model.StandardModelWrite{StandardModel: canonical, Metadata: meta, ExpectedRevision: canonical.Revision, ExpectedCatalogRevision: catalog.Revision, ExpectedPricingVersion: pricing.Version, PricingOptions: options}, want)
		if want == 200 {
			if err := model.DB.First(&canonical, "model = ?", "m7-model").Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	price := func(ratio float64) []model.Option {
		return []model.Option{{Key: "model_ratio", Value: fmt.Sprintf(`{"m7-model":%g}`, ratio)}, {Key: "completion_ratio", Value: `{"m7-model":1}`}}
	}
	call := func() model.Log {
		t.Helper()
		res := doRequest(server.URL, "POST", "/v1/chat/completions", key.Key, map[string]any{"model": "m7-model", "max_tokens": 200, "messages": []map[string]string{{"role": "user", "content": "Synthetic private request"}}})
		if res.Status != 200 {
			t.Fatalf("gateway: %d %s", res.Status, res.Body)
		}
		var row model.Log
		if err := model.DB.Where("user_id = ?", user.ID).Order("id DESC").First(&row).Error; err != nil {
			t.Fatal(err)
		}
		return row
	}
	var original model.Log
	t.Run("ADM-AC-01_roles", func(t *testing.T) {
		ops.call("POST", fmt.Sprintf("/api/admin/customers/%d/adjustments", user.ID), map[string]string{"reason": "Denied adjustment"}, 403)
		ops.call("POST", "/api/admin/resource-sync", nil, 403)
	})
	t.Run("ADM-AC-04_listing_requires_resource", func(t *testing.T) {
		write(false, model.ModelMeta{}, price(1), 200)
		rev := canonical.Revision
		write(true, model.ModelMeta{}, nil, 400)
		if canonical.Revision != rev {
			t.Fatal("failed listing changed model")
		}
	})
	now := time.Now().UTC().Format(time.RFC3339)
	snapshot := model.ResourceSnapshot{ID: "m7-snapshot", Sequence: 41, PublishedAt: now, ReceivedAt: now, Channels: []model.ResourceChannel{{ID: "m7-source", Name: "Synthetic source", Provider: "OpenAI", Enabled: true}}, Instances: []model.ResourceInstance{{ID: "m7-instance", ChannelID: "m7-source", CanonicalModel: "m7-model", UpstreamModel: "m7-model", Protocols: []string{"OpenAI"}, Capabilities: []string{"text"}, Region: "m7-region", Status: "available", ContextLength: 10000}}}
	if _, err := model.SaveResourceSnapshot(context.Background(), 0, snapshot); err != nil {
		t.Fatal(err)
	}
	ops.call("PUT", "/api/service-group", map[string]any{"group_id": "default", "name": "Default", "multiplier": "1", "enabled": true, "visibility": "common"}, 200)
	group, _ := model.GetServiceGroup("default")
	if err := model.InsertChannel(ch); err != nil {
		t.Fatal(err)
	}
	if err := model.DB.Create(&model.ResourceGatewayManagedChannel{ChannelID: ch.ID, SourceChannelID: "m7-source", Active: true, Profile: model.ResourceGatewayTextProfile}).Error; err != nil {
		t.Fatal(err)
	}
	if err := model.DB.Create(&model.ResourceGatewayRoute{GroupDefinitionID: group.ID, ChannelID: ch.ID, Model: "m7-model", Group: "default", Capabilities: `["text"]`, ContextLength: 10000}).Error; err != nil {
		t.Fatal(err)
	}
	name := "m7-model"
	if err := model.DB.Save(&model.ResourcePlacementRecord{ResourceID: "m7-instance", ModelID: &name, Enabled: true, GroupIDs: `["default"]`}).Error; err != nil {
		t.Fatal(err)
	}
	write(true, model.ModelMeta{}, nil, 200)
	original = call()
	t.Run("ADM-AC-03_internal_detail_and_privacy", func(t *testing.T) {
		data := tech.call("GET", fmt.Sprintf("/api/admin/customers/%d/calls/%s", user.ID, original.TraceID), nil, 200)
		raw, _ := json.Marshal(data)
		if data["resource_id"] != "m7-instance" || data["snapshot_sequence"] != float64(41) || bytes.Contains(raw, []byte("Synthetic private")) {
			t.Fatalf("incorrect internal metadata: %s", raw)
		}
		list := ops.call("GET", "/api/admin/customers?search=13812345678", nil, 200)
		raw, _ = json.Marshal(list)
		if bytes.Contains(raw, []byte(user.Phone)) || !bytes.Contains(raw, []byte("m7-customer")) {
			t.Fatal("account search or masking failed")
		}
		data = tech.call("GET", fmt.Sprintf("/api/admin/customers/%d", user.ID), nil, 200)
		if data["concurrency_limit"] != float64(50) {
			t.Fatal("default concurrency limit missing")
		}
	})
	t.Run("ADM-AC-05_price_changes_preserve_history", func(t *testing.T) {
		write(true, model.ModelMeta{}, price(2), 200)
		next := call()
		var old model.Log
		model.DB.First(&old, original.ID)
		if next.Quota != original.Quota*2 || old.Quota != original.Quota {
			t.Fatalf("price or immutable charge: %d %d %d", original.Quota, next.Quota, old.Quota)
		}
		var count int64
		model.DB.Model(&model.AuditLog{}).Where("action = ? AND object_id = ? AND user_id > 0 AND before_summary <> '' AND after_summary <> ''", "model_pricing_changed", "m7-model").Count(&count)
		if count < 2 {
			t.Fatal("missing actor/time/before/after price history")
		}
	})
	t.Run("ADM-AC-21_reference_isolation", func(t *testing.T) {
		v, _ := model.OptionVersion()
		value := 1.25
		meta := model.ModelMeta{OfficialInputPerM: &value}
		write(true, meta, nil, 400)
		meta.OfficialSourceURL = "https://example.invalid/vendor/pricing"
		meta.OfficialVerifiedDate = "2026-11-04"
		meta.OfficialHistorical = true
		meta.OfficialConditions = "Synthetic reference"
		write(true, meta, nil, 200)
		after, _ := model.OptionVersion()
		var old model.Log
		model.DB.First(&old, original.ID)
		if v != after || old.Quota != original.Quota {
			t.Fatal("reference changed billing")
		}
		next := call()
		if next.Quota != original.Quota*2 {
			t.Fatal("reference entered settlement")
		}
	})
	t.Run("ADM-AC-22_group_multiplier_and_inflight", func(t *testing.T) {
		block.Store(true)
		completed := make(chan model.Log, 1)
		go func() { completed <- call() }()
		select {
		case <-accepted:
		case <-time.After(5 * time.Second):
			t.Fatal("upstream did not accept")
		}
		ops.call("PUT", "/api/service-group", map[string]any{"group_id": "default", "name": "Default", "multiplier": "3", "enabled": true, "visibility": "common"}, 200)
		block.Store(false)
		close(release)
		inflight := <-completed
		next := call()
		if inflight.Quota != original.Quota*2 || next.Quota != original.Quota*6 {
			t.Fatalf("group freeze failed: %d %d base %d", inflight.Quota, next.Quota, original.Quota)
		}
		var count int64
		model.DB.Model(&model.AuditLog{}).Where("action = ? AND object_id = ?", "service_group_multiplier_changed", "default").Count(&count)
		if count == 0 {
			t.Fatal("missing group multiplier history")
		}
	})
	t.Run("ADM-AC-20_references_prevent_deletion", func(t *testing.T) {
		catalog, _ := model.ReadModelMetadataCatalog(context.Background())
		ops.call("DELETE", "/api/admin/models/m7-model", map[string]int64{"expected_catalog_revision": catalog.Revision}, 409)
		model.DB.Create(&model.StandardModel{Model: "unused", Name: "Unused", Vendor: "OpenAI", Kind: "chat"})
		catalog, _ = model.ReadModelMetadataCatalog(context.Background())
		ops.call("DELETE", "/api/admin/models/unused", map[string]int64{"expected_catalog_revision": catalog.Revision}, 200)
	})
	t.Run("ADM-AC-06_delisting", func(t *testing.T) {
		visible := doRequest(server.URL, "GET", "/api/plaza/models", "", nil)
		if visible.Status != 200 || !strings.Contains(visible.Body, `"m7-model"`) {
			t.Fatal("listed model was not visible in the plaza")
		}
		write(false, model.ModelMeta{}, nil, 200)
		before := requests.Load()
		res := doRequest(server.URL, "POST", "/v1/chat/completions", key.Key, map[string]any{"model": "m7-model", "messages": []map[string]string{{"role": "user", "content": "Synthetic"}}})
		if res.Status != 404 || !strings.Contains(res.Body, "model_not_found") || requests.Load() != before {
			t.Fatalf("delisting: %d %s", res.Status, res.Body)
		}
		plaza := doRequest(server.URL, "GET", "/api/plaza/models", key.Key, nil)
		if plaza.Status != 200 || strings.Contains(plaza.Body, `"m7-model"`) {
			t.Fatal("delisted model in plaza")
		}
	})
	t.Run("ADM-AC-02_disable_with_reason", func(t *testing.T) {
		ops.call("POST", fmt.Sprintf("/api/admin/customers/%d/control", user.ID), map[string]any{"status": 2, "reason": "Synthetic account restriction", "expected_version": 0}, 200)
		res := doRequest(server.URL, "POST", "/v1/chat/completions", key.Key, map[string]string{"model": "m7-model"})
		if !strings.Contains(res.Body, "account_disabled") {
			t.Fatal(res.Body)
		}
		var entry model.AuditLog
		if err := model.DB.Where("action = ? AND target_id = ?", "customer_disabled", user.ID).Order("id DESC").First(&entry).Error; err != nil || entry.Reason != "Synthetic account restriction" {
			t.Fatal("missing restriction audit")
		}
	})
	profile := model.DefaultDeploymentProfile()
	profile.RegionID = "m7-region"
	profile.AllowedVendors = []string{"OpenAI"}
	profile.Timezone = "Asia/Shanghai"
	t.Run("ADM-AC-17_root_profile_versions", func(t *testing.T) {
		for _, c := range []*staffHTTP{ops, finance, tech} {
			c.call("PUT", "/api/admin/region-profile", map[string]any{"profile": profile, "expected_revision": 0}, 403)
		}
		root.call("PUT", "/api/admin/region-profile", map[string]any{"profile": profile, "expected_revision": 0}, 200)
		profile.Brand = "Version two"
		root.call("PUT", "/api/admin/region-profile", map[string]any{"profile": profile, "expected_revision": 1}, 200)
		root.call("PUT", "/api/admin/region-profile", map[string]any{"profile": profile, "expected_revision": 1}, 409)
		data := root.call("GET", "/api/admin/region-profile", nil, 200)
		if len(data["history"].([]any)) != 2 {
			t.Fatal("profile history lost")
		}
	})
	t.Run("ADM-AC-18_scheduled_public_legal", func(t *testing.T) {
		for _, c := range []*staffHTTP{ops, finance, tech} {
			c.call("GET", "/api/admin/legal", nil, 403)
			c.call("POST", "/api/admin/legal", map[string]string{"kind": "terms"}, 403)
		}

		for i, at := range []string{"2026-01-01T00:00", "2026-06-01T00:00", "2027-01-01T00:00"} {
			root.call("POST", "/api/admin/legal", map[string]any{"kind": "terms", "language": "en", "version": fmt.Sprintf("v%d", i+1), "content": fmt.Sprintf("Synthetic legal version %d", i+1), "effective_local": at}, 200)
		}
		boundary := time.Date(2026, 12, 31, 16, 0, 0, 0, time.UTC)
		for _, test := range []struct {
			at   time.Time
			want string
		}{{boundary.Add(-time.Second), "v2"}, {boundary, "v3"}} {
			r := httptest.NewRequest("GET", "/api/legal/terms?language=en", nil)
			r.SetPathValue("kind", "terms")
			w := httptest.NewRecorder()
			controller.PublishedLegalAt(test.at)(w, r)
			if w.Code != 200 || !strings.Contains(w.Body.String(), `"version":"`+test.want+`"`) {
				t.Fatal(w.Body.String())
			}
		}
		data := root.call("GET", "/api/admin/legal", nil, 200)
		if len(data["items"].([]any)) != 3 {
			t.Fatal("legal history lost")
		}
	})
	t.Run("ADM-AC-19_health_and_reconciliation_permissions", func(t *testing.T) {
		model.DB.Create(&model.OptionInstance{ID: "m7-instance-process", Version: model.LoadedOptionVersion(), LastSeen: time.Now().Unix(), BuildVersion: "acceptance-build", DatabaseHealthy: true})
		data := tech.call("GET", "/api/admin/system", nil, 200)
		raw, _ := json.Marshal(data)
		if !bytes.Contains(raw, []byte("acceptance-build")) || !bytes.Contains(raw, []byte("health_checks")) {
			t.Fatal("system health not measured")
		}
		if err := model.FlushAPICharges(context.Background(), true); err != nil {
			t.Fatal(err)
		}
		if err := model.DB.Create(&model.Log{UserID: user.ID, Model: "m7-model", Quota: 7, Status: 1}).Error; err != nil {
			t.Fatal(err)
		}
		day := time.Now().In(time.FixedZone("regional", 8*3600)).Format("2006-01-02")
		reports, err := model.RunDailyReconciliation(context.Background(), day, user.ID, 1, time.Now())
		if err != nil || len(reports) != 1 || len(reports[0].Differences) != 1 {
			t.Fatalf("missing genuine difference: %+v %v", reports, err)
		}
		finance.call("GET", "/api/admin/reconciliation", nil, 200)
		ops.call("GET", "/api/admin/reconciliation", nil, 403)
	})
	t.Run("ADM-AC-13_audit_excludes_credentials", func(t *testing.T) {
		data := root.call("GET", "/api/audit?page_size=100", nil, 200)
		raw, _ := json.Marshal(data)
		for _, secret := range []string{rootSeed, key.Key, ch.Key, "Root-temporary-123", "Changed-Root-temporary-123", root.cookie.Value, "Synthetic private request", "Synthetic private response"} {
			if secret != "" && bytes.Contains(raw, []byte(secret)) {
				t.Fatal("audit leaked a synthetic credential or content")
			}
		}
	})
}

func TestM7ConcurrencyControlPreservesCustomerSessionAndHidesRouting(t *testing.T) {
	handler, root := staffFixture(t)
	root.enroll("root", "Root-temporary-123")
	user := &model.User{Username: "active-customer", Role: 1, Status: 1, Group: "default", Password: "synthetic-hash", AffCode: "control-customer"}
	if err := model.CreateUser(user); err != nil {
		t.Fatal(err)
	}
	session := middleware.Sessions.CreateConfirmed(user, time.Hour)
	if session == "" {
		t.Fatal("customer session unavailable")
	}
	if err := model.DB.Create(&model.Log{UserID: user.ID, Model: "synthetic-model", ResourceID: "private-resource", ResourceIDs: `["private-resource"]`, SnapshotSequence: 34, ResourceRevision: 8, RoutingRecorded: true, AffinityUsed: true}).Error; err != nil {
		t.Fatal(err)
	}
	path := fmt.Sprintf("/api/admin/customers/%d/control", user.ID)
	root.call("POST", path, map[string]any{"concurrency_limit": 2, "reason": "Limit tuning", "expected_version": 0}, 200)
	root.call("POST", path, map[string]any{"concurrency_limit": 3, "reason": "Stale edit", "expected_version": 0}, 409)
	if _, ok := middleware.Sessions.Get(session); !ok {
		t.Fatal("a limit edit revoked an unrelated customer session")
	}
	for _, path := range []string{"/api/log", "/api/log?scope=owned"} {
		r := httptest.NewRequest("GET", path, nil)
		r.AddCookie(&http.Cookie{Name: "session", Value: session})
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatal(w.Body.String())
		}
		for _, field := range []string{"private-resource", "snapshot_sequence", "resource_revision", "affinity_used", "routing_recorded"} {
			if strings.Contains(w.Body.String(), field) {
				t.Fatalf("customer log API exposed %s", field)
			}
		}
	}
	root.call("POST", fmt.Sprintf("/api/admin/customers/%d/sessions/revoke", user.ID), map[string]string{"reason": "Explicit revocation"}, 200)
	if _, ok := middleware.Sessions.Get(session); ok {
		t.Fatal("explicit revocation did not invalidate session")
	}
}
