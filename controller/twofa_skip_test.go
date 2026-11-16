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
	t.Setenv("STARHUB_ENV", "development")
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
	t.Setenv("STARHUB_ENV", "production")
	expectStatus(t, call(Skip2FASetup, request), http.StatusForbidden)
	status = expectStatus(t, call(Get2FAStatus, request), http.StatusOK)
	if json.Unmarshal(status.Data, &factorStatus) != nil || factorStatus.CanSkip {
		t.Fatal("production advertised skip capability")
	}
}
