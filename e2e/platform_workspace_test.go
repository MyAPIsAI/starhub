package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"starhub/middleware"
	"starhub/model"
)

func TestPlatformWorkspaceUsesVerifiedIdentityAndOwnerPermissions(t *testing.T) {
	handler, root := staffFixture(t)
	root.enroll("root", "Root-temporary-123")
	owner, err := model.GetStaffByUsername("root")
	if err != nil {
		t.Fatal(err)
	}
	customer := &model.User{Username: "personal-customer", Role: 1, Status: 1, Group: "default", AffCode: "personal-customer"}
	if err := model.CreateUser(customer); err != nil {
		t.Fatal(err)
	}
	for _, user := range []*model.User{owner, customer} {
		if err := model.InsertToken(&model.Token{UserID: user.ID, Name: user.Username + "-key", Status: 1, Group: "default", UnlimitedQuota: true}); err != nil {
			t.Fatal(err)
		}
		if err := model.DB.Create(&model.LedgerEntry{UserID: user.ID, Type: "adjustment", Description: user.Username + "-entry", Amount: 100}).Error; err != nil {
			t.Fatal(err)
		}
	}
	request := func(method, path, ip string, body any, want int, cookies ...*http.Cookie) string {
		t.Helper()
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest(method, path, bytes.NewReader(raw))
		r.RemoteAddr = ip
		r.Header.Set("X-StarHub-Portal", "workspace")
		r.Header.Set("Content-Type", "application/json")
		for _, c := range cookies {
			r.AddCookie(c)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s: %d, want %d: %s", method, path, w.Code, want, w.Body.String())
		}
		return w.Body.String()
	}
	self := request("GET", "/api/user/self", root.ip, nil, 200, root.cookie)
	if !strings.Contains(self, `"role":1,`) || !strings.Contains(self, `"platform_admin":true`) {
		t.Fatalf("incorrect workspace principal: %s", self)
	}
	keys := request("GET", "/api/token", root.ip, nil, 200, root.cookie)
	if !strings.Contains(keys, "root-key") || strings.Contains(keys, "personal-customer-key") {
		t.Fatal("workspace did not isolate existing keys")
	}
	ledger := request("GET", "/api/ledger", root.ip, nil, 200, root.cookie)
	if !strings.Contains(ledger, "root-entry") || strings.Contains(ledger, "personal-customer-entry") {
		t.Fatal("workspace ledger included another owner or omitted its own entry")
	}
	pending := middleware.Sessions.CreateStaffConfirmed(owner, false, time.Minute)
	request("GET", "/api/user/self", root.ip, nil, 401, &http.Cookie{Name: "staff_session", Value: pending})
	second := &http.Cookie{Name: "session", Value: middleware.Sessions.CreateConfirmed(customer, time.Hour)}
	keys = request("GET", "/api/token", root.ip, nil, 200, root.cookie, second)
	if strings.Contains(keys, "root-key") || !strings.Contains(keys, "personal-customer-key") {
		t.Fatal("staff cookie shadowed a customer identity")
	}
	request("GET", "/api/user/self", "192.0.2.21:8000", nil, 401, root.cookie)
	request("POST", "/api/user/deactivate", root.ip, nil, 403, root.cookie)
	request("PUT", "/api/user/profile-state", root.ip, nil, 403, root.cookie)
	devices := request("GET", fmt.Sprintf("/api/user/session-state?expected_user_id=%d", owner.ID), root.ip, nil, 200, root.cookie)
	if !strings.Contains(devices, `"is_current":true`) {
		t.Fatal("workspace lost its verified session")
	}
	request("POST", "/api/user/logout-current", root.ip, map[string]any{"schema": "logout_current_v1", "expected_user_id": owner.ID}, 200, root.cookie)
	request("GET", "/api/user/self", root.ip, nil, 401, root.cookie)
	root.call("GET", "/api/admin/auth/self", nil, 401)
}

func TestPlatformWorkspaceAvailableToAllStaffRoles(t *testing.T) {
	handler, root := staffFixture(t)
	root.enroll("root", "Root-temporary-123")
	// Existing installations may retain the former per-account grant column.
	if err := model.DB.Exec("ALTER TABLE users ADD COLUMN platform_admin numeric").Error; err != nil {
		t.Fatal(err)
	}
	for _, role := range []int{5, 10, 20} {
		name := fmt.Sprintf("workspace-staff-%d", role)
		t.Run(name, func(t *testing.T) {
			input := map[string]any{"username": name, "temporary_password": "Workspace-temporary-123", "role": role, "reason": "Workspace acceptance"}
			if role == 10 {
				input["platform_admin"] = false
			}
			created := root.call("POST", "/api/admin/staff", input, 200)
			if created["platform_admin"] != true || created["role"] != float64(role) {
				t.Fatal("unexpected staff account identity")
			}
			if err := model.DB.Exec("UPDATE users SET platform_admin = 0 WHERE username = ?", name).Error; err != nil {
				t.Fatal(err)
			}
			staff := &staffHTTP{t: t, handler: handler, ip: root.ip}
			staff.enroll(name, "Workspace-temporary-123")
			r := httptest.NewRequest("GET", "/api/user/self", nil)
			r.RemoteAddr = staff.ip
			r.Header.Set("X-StarHub-Portal", "workspace")
			r.AddCookie(staff.cookie)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			var response struct {
				Data struct {
					ID            int64
					Role          int
					PlatformAdmin bool `json:"platform_admin"`
				}
			}
			if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &response) != nil || float64(response.Data.ID) != created["id"] || response.Data.Role != 1 || !response.Data.PlatformAdmin {
				t.Fatal("expected an owner-scoped workspace for this staff account")
			}
			staff.call("GET", "/api/admin/staff", nil, 403)
		})
	}
}
