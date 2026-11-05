package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"starhub/common"
	"starhub/config"
	"starhub/internal/router"
	"starhub/middleware"
	"starhub/model"
	"starhub/setting"
)

type staffHTTP struct {
	t       *testing.T
	handler http.Handler
	cookie  *http.Cookie
	ip      string
}

func (c *staffHTTP) call(method, path string, body any, want int) map[string]any {
	c.t.Helper()
	data, _ := json.Marshal(body)
	r := httptest.NewRequest(method, path, bytes.NewReader(data))
	r.RemoteAddr = c.ip
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-StarHub-Portal", "staff")
	if c.cookie != nil {
		r.AddCookie(c.cookie)
	}
	w := httptest.NewRecorder()
	c.handler.ServeHTTP(w, r)
	if w.Code != want {
		c.t.Fatalf("%s %s: got %d, want %d: %s", method, path, w.Code, want, w.Body.String())
	}
	for _, cookie := range w.Result().Cookies() {
		if cookie.Name == "staff_session" {
			c.cookie = cookie
		}
	}
	var envelope struct {
		Data map[string]any `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &envelope)
	return envelope.Data
}
func staffFixture(t *testing.T) (http.Handler, *staffHTTP) {
	t.Helper()
	middleware.ResetRateLimits()
	setting.ResetAllToDefaults()
	if err := config.ConfigureStaffNetworks([]string{"127.0.0.1/32", "::1/128"}); err != nil {
		t.Fatal(err)
	}
	if err := model.InitDB(filepath.Join(t.TempDir(), "staff.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = model.CloseDB() })
	if err := model.InitializeStaffRoot("root", "Root-temporary-123", "", false); err != nil {
		t.Fatal(err)
	}
	handler := router.New()
	return handler, &staffHTTP{t: t, handler: handler, ip: "127.0.0.1:18480"}
}
func (c *staffHTTP) enroll(name, temporary string) string {
	c.t.Helper()
	state := c.call("POST", "/api/admin/auth/login", map[string]string{"username": name, "password": temporary}, 200)
	if state["stage"] != "change_password" {
		c.t.Fatal("new staff did not require password change")
	}
	c.call("GET", "/api/user", nil, 403)
	c.call("POST", "/api/admin/auth/enroll", nil, 403)
	state = c.call("POST", "/api/admin/auth/password", map[string]string{"current_password": temporary, "new_password": "Changed-" + temporary}, 200)
	if state["stage"] != "enroll" {
		c.t.Fatal("new staff did not require 2FA enrollment")
	}
	c.call("GET", "/api/user", nil, 403)
	data := c.call("POST", "/api/admin/auth/enroll", nil, 200)
	secret, _ := data["secret"].(string)
	if len(secret) != 32 {
		c.t.Fatal("missing authenticator setup seed")
	}
	c.call("POST", "/api/admin/auth/verify", map[string]string{"code": "invalid"}, 403)
	code, err := model.TOTPCode(secret, time.Now().Unix()/30)
	if err != nil {
		c.t.Fatal(err)
	}
	old := c.cookie
	state = c.call("POST", "/api/admin/auth/verify", map[string]string{"code": code}, 200)
	if state["stage"] != "ready" || old.Value == c.cookie.Value {
		c.t.Fatal("verification did not rotate pending session")
	}
	other := *c
	other.cookie = old
	other.call("GET", "/api/user", nil, 401)
	user, err := model.GetStaffByUsername(name)
	if err != nil || user.TOTPSecret == "" || strings.Contains(user.TOTPSecret, secret) || user.MustChangePassword {
		c.t.Fatal("staff secret or enrollment state not persisted safely")
	}
	c.call("GET", "/api/admin/auth/self", nil, 200)
	return secret
}

func TestADMAC12FirstLoginCannotBypassPasswordAndTwoFactorSetup(t *testing.T) {
	handler, root := staffFixture(t)
	root.enroll("root", "Root-temporary-123")
	root.call("POST", "/api/admin/staff", map[string]any{"username": "new-operator", "temporary_password": "Operator-temporary-123", "role": common.RoleAdmin, "reason": "Staff onboarding"}, 200)
	operator := &staffHTTP{t: t, handler: handler, ip: "127.0.0.1:18481"}
	secret := operator.enroll("new-operator", "Operator-temporary-123")
	operator.call("GET", "/api/user", nil, 200)
	operator.call("POST", "/api/user/login", map[string]string{"username": "new-operator", "password": "Changed-Operator-temporary-123"}, 401)
	pending := &staffHTTP{t: t, handler: handler, ip: "127.0.0.1:18482"}
	state := pending.call("POST", "/api/admin/auth/login", map[string]string{"username": "new-operator", "password": "Changed-Operator-temporary-123"}, 200)
	if state["stage"] != "verify" {
		t.Fatal("bound staff login skipped verification")
	}
	pending.call("GET", "/api/audit", nil, 403)
	bound, _ := model.GetStaffByUsername("new-operator")
	code, _ := model.TOTPCode(secret, bound.TOTPLastStep)
	pending.call("POST", "/api/admin/auth/verify", map[string]string{"code": code}, 403)
	// Replay and invalid-code counters are account-wide, not session-local.
	for i := 0; i < 4; i++ {
		pending.call("POST", "/api/admin/auth/verify", map[string]string{"code": "invalid"}, 403)
	}
	locked, _ := model.GetStaffByUsername("new-operator")
	if locked.OTPLockedUntil <= time.Now().Unix() {
		t.Fatal("OTP failure throttle was bypassed across password sessions")
	}
	pending.call("POST", "/api/admin/auth/login", map[string]string{"username": "new-operator", "password": "Changed-Operator-temporary-123"}, 429)
}

func TestADMAC01FourStaffRolesEnforceMoneySyncAndAuditBoundaries(t *testing.T) {
	handler, root := staffFixture(t)
	root.enroll("root", "Root-temporary-123")
	password, _ := common.HashPassword("Customer-password-123")
	customer := &model.User{Username: "customer", Password: password, Role: common.RoleUser, Status: 1, Group: "default", AffCode: "staff-accept-customer"}
	if err := model.CreateUser(customer); err != nil {
		t.Fatal(err)
	}
	_ = model.InsertAuditLog(&model.AuditLog{UserID: customer.ID, Username: customer.Username, Path: "/private-customer-event"})
	clients := map[int]*staffHTTP{100: root}
	for index, role := range []int{10, 5, 20} {
		name := fmt.Sprintf("staff-%d", role)
		temporary := fmt.Sprintf("Temporary-staff-%d", role)
		root.call("POST", "/api/admin/staff", map[string]any{"username": name, "temporary_password": temporary, "role": role, "reason": "Role acceptance fixture"}, 200)
		client := &staffHTTP{t: t, handler: handler, ip: fmt.Sprintf("127.0.0.1:%d", 18500+index)}
		client.enroll(name, temporary)
		clients[role] = client
	}
	adjustPath := fmt.Sprintf("/api/admin/customers/%d/adjustments", customer.ID)
	adjustment := map[string]any{"amount_usd": "1.000000001", "kind": "gift", "reason": "Service compensation fixture", "operation_id": "staff-adjustment-test-01"}
	for _, role := range []int{10, 5, 20, 100} {
		t.Run(fmt.Sprint(role), func(t *testing.T) {
			client := *clients[role]
			client.t = t
			client.call("GET", "/api/user", nil, 200)
			client.call("GET", "/api/audit", nil, 200)
			if role != 100 {
				client.call("GET", "/api/admin/staff", nil, 403)
				client.call("POST", "/api/admin/staff", map[string]any{}, 403)
			}
			if role == 5 || role == 100 {
				client.call("POST", adjustPath, adjustment, 200)
				client.call("GET", "/api/finance/summary", nil, 200)
			} else {
				client.call("POST", adjustPath, adjustment, 403)
				client.call("GET", "/api/finance/summary", nil, 403)
			}
			if role == 20 || role == 100 {
				client.call("GET", "/api/admin/resource-sync", nil, 200)
				client.call("POST", "/api/admin/resource-sync", nil, 503)
				client.call("GET", "/api/admin/system", nil, 200)
			} else {
				client.call("GET", "/api/admin/resource-sync", nil, 403)
				client.call("POST", "/api/admin/resource-sync", nil, 403)
				client.call("GET", "/api/admin/system", nil, 403)
			}
			if role != 100 {
				r := httptest.NewRequest("GET", "/api/audit", nil)
				r.RemoteAddr = client.ip
				r.AddCookie(client.cookie)
				w := httptest.NewRecorder()
				handler.ServeHTTP(w, r)
				var result struct {
					Data struct {
						Items []model.AuditLog `json:"items"`
					} `json:"data"`
				}
				if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
					t.Fatal(err)
				}
				actor, _ := model.GetStaffByUsername(fmt.Sprintf("staff-%d", role))
				for _, row := range result.Data.Items {
					if row.UserID != actor.ID {
						t.Fatal("staff can read another actor's audit")
					}
				}
			}
		})
	}
	user, _ := model.GetUserByID(customer.ID)
	if model.APIBalanceNano(user).String() != "2000000002" {
		t.Fatalf("exact balance changed outside Finance/Root: %s", model.APIBalanceNano(user))
	}
	finance := clients[5]
	finance.call("POST", adjustPath, adjustment, 200)
	adjustment["amount_usd"] = "-3.000000003"
	adjustment["kind"] = "deduction"
	adjustment["operation_id"] = "staff-adjustment-test-02"
	finance.call("POST", adjustPath, adjustment, 200)
	user, _ = model.GetUserByID(customer.ID)
	if model.APIBalanceNano(user).String() != "-1000000001" {
		t.Fatal("negative exact balance or idempotency failed")
	}
	var rows []model.LedgerEntry
	model.DB.Where("funding_source = ?", model.FundingSourceStaffAdjustment).Find(&rows)
	if len(rows) != 3 {
		t.Fatalf("adjustment ledger count %d", len(rows))
	}
	var audits int64
	model.DB.Model(&model.AuditLog{}).Where("action LIKE ?", "balance_%").Count(&audits)
	if audits != 3 {
		t.Fatalf("adjustment audit count %d", audits)
	}
	target, _ := model.GetStaffByUsername("staff-10")
	root.call("PUT", fmt.Sprintf("/api/admin/staff/%d", target.ID), map[string]any{"expected_version": target.SecurityVersion, "role": 20, "status": 1, "reason": "Change staff duties"}, 200)
	clients[10].call("GET", "/api/user", nil, 401)
	rootUser, _ := model.GetStaffByUsername("root")
	root.call("PUT", fmt.Sprintf("/api/admin/staff/%d", rootUser.ID), map[string]any{"expected_version": rootUser.SecurityVersion, "role": 10, "status": 1, "reason": "Last root protection"}, 409)
}

func TestStaffNetworkAndCustomerCookiesCannotAuthorizeBackOffice(t *testing.T) {
	handler, root := staffFixture(t)
	root.enroll("root", "Root-temporary-123")
	for _, path := range []string{"/api/admin/auth/login", "/api/user", "/api/audit", "/api/admin/auth/self"} {
		r := httptest.NewRequest("GET", path, nil)
		if strings.HasSuffix(path, "login") {
			r.Method = "POST"
		}
		r.RemoteAddr = "198.51.100.9:12345"
		r.Header.Set("X-Forwarded-For", "127.0.0.1")
		r.Header.Set("X-StarHub-Portal", "staff")
		r.AddCookie(root.cookie)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatalf("spoofed staff IP admitted %s: %d", path, w.Code)
		}
	}
	password, _ := common.HashPassword("Customer-password-123")
	customer := &model.User{Username: "customer", Password: password, Role: 1, Status: 1, AffCode: "staff-cookie-customer"}
	if err := model.CreateUser(customer); err != nil {
		t.Fatal(err)
	}
	session := middleware.Sessions.Create(customer.ID, time.Hour)
	r := httptest.NewRequest("GET", "/api/admin/staff", nil)
	r.RemoteAddr = "127.0.0.1:12345"
	r.AddCookie(&http.Cookie{Name: "session", Value: session})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("customer cookie authorized staff endpoint")
	}
	r = httptest.NewRequest("GET", "/api/user/self", nil)
	r.RemoteAddr = "127.0.0.1:12345"
	r.AddCookie(&http.Cookie{Name: "session", Value: session})
	r.AddCookie(root.cookie)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"username":"customer"`) {
		t.Fatal("staff cookie replaced customer identity on customer surface")
	}
	r = httptest.NewRequest("GET", "/api/user/self", nil)
	r.RemoteAddr = "127.0.0.1:12345"
	r.AddCookie(root.cookie)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("staff cookie silently became a customer session")
	}
}

func TestStaffForceSyncReplacesOnlyValidatedFullSnapshots(t *testing.T) {
	_, root := staffFixture(t)
	root.enroll("root", "Root-temporary-123")
	snapshot := model.ResourceSnapshot{ID: "staff-full-sync", Sequence: 1, PublishedAt: "2026-11-04T00:00:00Z", Channels: []model.ResourceChannel{{ID: "source", Name: "Synthetic source", Enabled: true}}, Instances: []model.ResourceInstance{{ID: "instance", ChannelID: "source", UpstreamModel: "alpha", CanonicalModel: "alpha", Protocols: []string{"OpenAI"}, Capabilities: []string{"text"}, ContextLength: 128000, Status: "available", LatencyMS: 10}}}
	status := 200
	full := false
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		full = r.URL.Query().Get("full") == "true" && r.Header.Get("Authorization") == "Bearer synthetic-source-token"
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(snapshot)
	}))
	defer source.Close()
	if err := config.ConfigureSnapshotSource(source.URL, "synthetic-source-token"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = config.ConfigureSnapshotSource("", "") })
	root.call("POST", "/api/admin/resource-sync", nil, 200)
	if !full {
		t.Fatal("forced sync did not request a full authenticated snapshot")
	}
	before, _ := model.ReadResourceCatalog(t.Context())
	root.call("POST", "/api/admin/resource-sync", nil, 409)
	snapshot.Sequence = 2
	snapshot.Instances[0].ChannelID = "missing-source"
	root.call("POST", "/api/admin/resource-sync", nil, 400)
	status = 500
	root.call("POST", "/api/admin/resource-sync", nil, 502)
	after, _ := model.ReadResourceCatalog(t.Context())
	if after.Revision != before.Revision || after.Snapshot.Sequence != 1 {
		t.Fatal("failed sync modified the published snapshot")
	}
}

func TestStaffLostAuthenticatorRecoveryAndOperationalControls(t *testing.T) {
	handler, root := staffFixture(t)
	rootSecret := root.enroll("root", "Root-temporary-123")
	root.call("POST", "/api/admin/staff", map[string]any{"username": "operator", "temporary_password": "Operator-temporary-123", "role": 10, "reason": "Operational controls"}, 200)
	ops := &staffHTTP{t: t, handler: handler, ip: "127.0.0.1:18512"}
	ops.enroll("operator", "Operator-temporary-123")
	for _, endpoint := range []string{"/api/redemption", "/api/task-plugin"} {
		ops.call("POST", endpoint, map[string]any{}, 403)
	}
	ops.call("PUT", "/api/option", map[string]any{"key": "aff_register_bonus", "value": "1000000"}, 403)
	password, _ := common.HashPassword("Customer-password-123")
	customer := &model.User{Username: "customer", Password: password, Role: 1, Status: 1, AffCode: "staff-ops-customer"}
	if err := model.CreateUser(customer); err != nil {
		t.Fatal(err)
	}
	key := &model.Token{UserID: customer.ID, Name: "Customer key", Status: 1, UnlimitedQuota: true}
	if err := model.InsertToken(key); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", fmt.Sprintf("/api/admin/customers/%d/keys", customer.ID), nil)
	r.RemoteAddr = ops.ip
	r.AddCookie(ops.cookie)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 200 || strings.Contains(w.Body.String(), key.Key) || strings.Contains(w.Body.String(), `"key":`) {
		t.Fatal("customer key list exposed a credential")
	}
	ops.call("POST", fmt.Sprintf("/api/admin/customers/%d/keys/%d/disable", customer.ID, key.ID), map[string]string{"reason": "Customer requested revocation"}, 200)
	stored, _ := model.GetTokenByID(key.ID)
	if stored.Status != 2 {
		t.Fatal("Operations could not disable customer Key")
	}
	session := middleware.Sessions.Create(customer.ID, time.Hour)
	ops.call("POST", fmt.Sprintf("/api/admin/customers/%d/disable", customer.ID), map[string]string{"reason": "Customer requested closure"}, 200)
	if _, ok := middleware.Sessions.Get(session); ok {
		t.Fatal("customer disable retained its sessions")
	}
	target, _ := model.GetStaffByUsername("operator")
	body := map[string]any{"expected_version": target.SecurityVersion, "role": 10, "status": 1, "reset_2fa": true, "reason": "Identity verified through the company recovery process", "current_password": "incorrect", "code": "000000"}
	root.call("PUT", fmt.Sprintf("/api/admin/staff/%d", target.ID), body, 403)
	code, _ := model.TOTPCode(rootSecret, time.Now().Unix()/30+1)
	body["current_password"] = "Changed-Root-temporary-123"
	body["code"] = code
	root.call("PUT", fmt.Sprintf("/api/admin/staff/%d", target.ID), body, 200)
	ops.call("GET", "/api/user", nil, 401)
	state := ops.call("POST", "/api/admin/auth/login", map[string]string{"username": "operator", "password": "Changed-Operator-temporary-123"}, 200)
	if state["stage"] != "enroll" {
		t.Fatal("recovered employee skipped 2FA enrollment")
	}
	target, _ = model.GetStaffByUsername("operator")
	if target.TOTPSecret != "" {
		t.Fatal("lost authenticator seed was not reset")
	}
}
