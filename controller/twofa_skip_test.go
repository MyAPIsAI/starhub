package controller

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"starhub/common"
	"starhub/middleware"
	"starhub/model"
)

func TestSkipTwoFASetupUpdatesOnlyTheCurrentSession(t *testing.T) {
	t.Setenv("STARHUB_ENV", "production")
	newTestDB(t)
	user := newUser(t, "skip-staff", common.RoleAdmin, 0)
	session := middleware.Sessions.CreateConfirmed(user, time.Hour, "", "")
	t.Cleanup(func() { middleware.Sessions.Remove(session) })
	request := testRequest{method: "POST", target: "/api/user/2fa/skip", userID: user.ID, role: user.Role, headers: map[string]string{"Cookie": "session=" + session}}
	expectStatus(t, call(Skip2FASetup, request), http.StatusForbidden)
	if err := model.UpdateUserFields(user.ID, map[string]any{"admin_password_changed_at": 1}); err != nil {
		t.Fatal(err)
	}
	if err := model.CreateTwoFASetup(user.ID, "pending-secret"); err != nil {
		t.Fatal(err)
	}
	status := expectStatus(t, call(Get2FAStatus, request), http.StatusOK)
	var factorStatus struct {
		Enabled bool `json:"enabled"`
		CanSkip bool `json:"can_skip"`
	}
	if json.Unmarshal(status.Data, &factorStatus) != nil || !factorStatus.CanSkip || factorStatus.Enabled {
		t.Fatal("pending enrollment did not advertise skip capability")
	}
	result := expectStatus(t, call(Skip2FASetup, request), http.StatusOK)
	var profile struct {
		ID       int64 `json:"id"`
		Required bool  `json:"admin_security_required"`
	}
	if json.Unmarshal(result.Data, &profile) != nil || profile.ID != user.ID || profile.Required {
		t.Fatal("skip response did not confirm current session access")
	}
	factor, err := model.GetTwoFA(user.ID)
	if err != nil || factor.IsEnabled {
		t.Fatal("skip enabled the authenticator")
	}
	t.Setenv("STARHUB_ENV", "development")
	expectStatus(t, call(Skip2FASetup, request), http.StatusOK)
	status = expectStatus(t, call(Get2FAStatus, request), http.StatusOK)
	if json.Unmarshal(status.Data, &factorStatus) != nil || !factorStatus.CanSkip {
		t.Fatal("development did not advertise skip capability")
	}
}

func TestSelfServicePasswordChangeRefreshesOnlyItsSessionForSkipping(t *testing.T) {
	t.Setenv("STARHUB_ENV", "production")
	for _, profileUpdate := range []bool{false, true} {
		name := "self"
		if profileUpdate {
			name = "profile-state"
		}
		t.Run(name, func(t *testing.T) {
			newTestDB(t)
			user := newUser(t, "password-change-staff", common.RoleAdmin, 0)
			current := middleware.Sessions.CreateConfirmed(user, time.Hour, "", "")
			other := middleware.Sessions.CreateConfirmed(user, time.Hour, "", "")
			t.Cleanup(func() { middleware.Sessions.Remove(current); middleware.Sessions.Remove(other) })
			request := testRequest{method: "PUT", target: "/api/user/self", userID: user.ID, role: user.Role, headers: map[string]string{"Cookie": "session=" + current}, body: `{"current_password":"secret","new_password":"ChangedPassword93"}`}
			handler := UpdateSelf
			if profileUpdate {
				handler = UpdateProfileState
				request.target = "/api/user/profile-state"
				request.body = `{"expected_user_id":` + itoa(user.ID) + `,"expected_email":"","current_password":"secret","new_password":"ChangedPassword93"}`
			}
			expectStatus(t, call(handler, request), http.StatusOK)
			fresh, err := model.GetUserByID(user.ID)
			if err != nil || !common.VerifyPassword("ChangedPassword93", fresh.Password) {
				t.Fatal("password change did not persist")
			}
			if !middleware.StaffSecurityRequired(current, fresh) {
				t.Fatal("password change granted access without an explicit skip")
			}
			request.method, request.target, request.body = "POST", "/api/user/2fa/skip", ""
			expectStatus(t, call(Skip2FASetup, request), http.StatusOK)
			request.headers["Cookie"] = "session=" + other
			expectStatus(t, call(Skip2FASetup, request), http.StatusConflict)
			if profileUpdate && !middleware.Sessions.CurrentGeneration(other, fresh) {
				t.Fatal("profile update unexpectedly revoked another session")
			}
		})
	}
}
