package main

import (
	"testing"
	"time"

	"starhub/common"
	"starhub/middleware"
	"starhub/model"
)

// fixtureSession prepares established staff identities for tests unrelated to
// enrollment. Login and onboarding tests exercise the real HTTP flow instead.
func fixtureSession(t *testing.T, user *model.User) string {
	t.Helper()
	if !common.IsStaffRole(user.Role) {
		return middleware.Sessions.Create(user.ID, time.Hour)
	}
	if err := model.UpdateUserFields(user.ID, map[string]any{"admin_password_changed_at": 1}); err != nil {
		t.Fatal(err)
	}
	factor, err := model.GetTwoFA(user.ID)
	if err == model.Err2FANotEnabled {
		factor = &model.TwoFA{UserID: user.ID, Secret: "JBSWY3DPEHPK3PXP", IsEnabled: true}
		if err := model.DB.Create(factor).Error; err != nil {
			t.Fatal(err)
		}
	} else if err != nil {
		t.Fatal(err)
	}
	user, err = model.GetUserByID(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	session := middleware.Sessions.CreateConfirmed(user, time.Hour, "", "", factor.Secret)
	if session == "" {
		t.Fatal("staff fixture session rejected")
	}
	t.Cleanup(func() { middleware.Sessions.Remove(session) })
	return session
}
