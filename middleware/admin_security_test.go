package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"starhub/common"
	"starhub/model"
)

func TestStaffGateRequiresPasswordEnrollmentAndSessionProof(t *testing.T) {
	setupAutoRoutingDB(t)
	for _, role := range []int{common.RoleRoot, common.RoleAdmin, common.RoleFinance, common.RoleTech} {
		user := &model.User{Username: "gate-" + common.GenerateSessionKey(), Password: "initial", Status: common.StatusEnabled, Role: role, AffCode: common.GenerateAffCode()}
		if err := model.CreateUser(user); err != nil {
			t.Fatal(err)
		}
		passwordOnly := Sessions.CreateConfirmed(user, time.Hour, "", "")
		check := func(token, path string, want int) {
			t.Helper()
			r := httptest.NewRequest("GET", path, nil)
			r.AddCookie(&http.Cookie{Name: "session", Value: token})
			w := httptest.NewRecorder()
			UserAuth(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })).ServeHTTP(w, r)
			if w.Code != want {
				t.Fatalf("role %d path %s got %d: %s", role, path, w.Code, w.Body.String())
			}
		}
		check(passwordOnly, "/api/wallet/adjustments", 403)
		check(passwordOnly, "/api/user/self", 204)
		if err := model.DB.Create(&model.TwoFA{UserID: user.ID, Secret: "factor", IsEnabled: true}).Error; err != nil {
			t.Fatal(err)
		}
		verified := Sessions.CreateConfirmed(user, time.Hour, "", "", "factor")
		check(verified, "/api/wallet/adjustments", 403)
		if err := model.UpdateUserFields(user.ID, map[string]any{"admin_password_changed_at": 1}); err != nil {
			t.Fatal(err)
		}
		user.AdminPasswordChangedAt = 1
		check(passwordOnly, "/api/wallet/adjustments", 403)
		check(verified, "/api/wallet/adjustments", 204)
		if err := model.UpdateUserFields(user.ID, map[string]any{"password": "changed"}); err != nil {
			t.Fatal(err)
		}
		check(verified, "/api/wallet/adjustments", 403)
		Sessions.Remove(passwordOnly)
		Sessions.Remove(verified)
	}
}

func TestOperatorResetRevokesSecuritySetupSessions(t *testing.T) {
	setupAutoRoutingDB(t)
	user := &model.User{Username: "reset-session", Role: common.RoleAdmin, Status: common.StatusEnabled, AffCode: common.GenerateAffCode()}
	if err := model.CreateUser(user); err != nil {
		t.Fatal(err)
	}
	session := Sessions.CreateConfirmed(user, time.Hour, "", "")
	if !Sessions.CurrentGeneration(session, user) {
		t.Fatal("fresh session rejected")
	}
	if err := model.OperateStaffAccount("reset-2fa", user.Username, "", "operator", "Lost factor"); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "/api/user/self", nil)
	r.AddCookie(&http.Cookie{Name: "session", Value: session})
	w := httptest.NewRecorder()
	UserAuth(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { t.Fatal("revoked session reached setup") })).ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatalf("got %d: %s", w.Code, w.Body.String())
	}
}

func TestStaffTwoFASkipIsBoundToOneSession(t *testing.T) {
	for _, mode := range []string{"development", "production"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("STARHUB_ENV", mode)
			setupAutoRoutingDB(t)
			for _, role := range []int{common.RoleRoot, common.RoleAdmin, common.RoleFinance, common.RoleTech} {
				user := &model.User{Username: "skip-" + common.GenerateSessionKey(), Password: "changed", Role: role, Status: common.StatusEnabled, AdminPasswordChangedAt: 1, AffCode: common.GenerateAffCode()}
				if err := model.CreateUser(user); err != nil {
					t.Fatal(err)
				}
				session := Sessions.CreateConfirmed(user, time.Hour, "", "")
				other := Sessions.CreateConfirmed(user, time.Hour, "", "")
				t.Cleanup(func() { Sessions.Remove(session); Sessions.Remove(other) })
				if !StaffSecurityRequired(session, user) || !Sessions.SkipStaffTwoFA(session, user) {
					t.Fatal("explicit skip was not required and accepted")
				}
				if StaffSecurityRequired(session, user) || !StaffSecurityRequired(other, user) || Sessions.StaffVerified(session, user) {
					t.Fatal("skip leaked to another session or became factor evidence")
				}
				r := httptest.NewRequest("GET", "/api/option/", nil)
				r.AddCookie(&http.Cookie{Name: "session", Value: session})
				w := httptest.NewRecorder()
				UserAuth(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })).ServeHTTP(w, r)
				if w.Code != 204 {
					t.Fatalf("skipped session was rejected: %s", w.Body.String())
				}
				for _, change := range []func(*model.User){
					func(u *model.User) { u.Password = "new-password" },
					func(u *model.User) {
						u.Role = common.RoleAdmin
						if role == common.RoleAdmin {
							u.Role = common.RoleRoot
						}
					},
					func(u *model.User) { u.DeactivationVersion++ },
					func(u *model.User) { u.AdminPasswordChangedAt = 0 },
				} {
					changed := *user
					change(&changed)
					if !StaffSecurityRequired(session, &changed) {
						t.Fatal("skip survived an account security change")
					}
				}
				freshSession := Sessions.CreateConfirmed(user, time.Hour, "", "")
				t.Cleanup(func() { Sessions.Remove(freshSession) })
				if !StaffSecurityRequired(freshSession, user) {
					t.Fatal("a new login inherited the previous session's skip")
				}
				if err := model.DB.Create(&model.TwoFA{UserID: user.ID, Secret: "factor", IsEnabled: true}).Error; err != nil {
					t.Fatal(err)
				}
				if !StaffSecurityRequired(session, user) || Sessions.SkipStaffTwoFA(session, user) {
					t.Fatal("skip bypassed an enabled authenticator")
				}
			}
		})
	}
}

func TestStaffTwoFASkipRequiresChangedPasswordAndActiveStaff(t *testing.T) {
	t.Setenv("STARHUB_ENV", "production")
	setupAutoRoutingDB(t)
	user := &model.User{Username: "skip-preconditions", Password: "initial", Role: common.RoleAdmin, Status: common.StatusEnabled, AffCode: common.GenerateAffCode()}
	if err := model.CreateUser(user); err != nil {
		t.Fatal(err)
	}
	session := Sessions.CreateConfirmed(user, time.Hour, "", "")
	t.Cleanup(func() { Sessions.Remove(session) })
	if CanSkipStaffTwoFA(user) || Sessions.SkipStaffTwoFA(session, user) {
		t.Fatal("initial password requirement was skipped")
	}
	if err := model.UpdateUserFields(user.ID, map[string]any{"admin_password_changed_at": 1}); err != nil {
		t.Fatal(err)
	}
	user.AdminPasswordChangedAt = 1
	if err := model.CreateTwoFASetup(user.ID, "pending-factor"); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"", "development", "production"} {
		t.Setenv("STARHUB_ENV", mode)
		if !CanSkipStaffTwoFA(user) || !Sessions.SkipStaffTwoFA(session, user) {
			t.Fatalf("mode %q rejected a pending enrollment skip", mode)
		}
	}
	t.Setenv("STARHUB_ENV", "invalid")
	if CanSkipStaffTwoFA(user) || Sessions.SkipStaffTwoFA(session, user) || !StaffSecurityRequired(session, user) {
		t.Fatal("invalid environment allowed skip authorization")
	}
	t.Setenv("STARHUB_ENV", "production")
	if Sessions.SkipStaffTwoFA("missing-session", user) {
		t.Fatal("skip accepted without an active session")
	}
	for _, changed := range []model.User{
		{ID: user.ID, Role: common.RoleUser, Status: common.StatusEnabled, AdminPasswordChangedAt: 1},
		{ID: user.ID, Role: common.RoleAdmin, Status: common.StatusDisabled, AdminPasswordChangedAt: 1},
	} {
		if CanSkipStaffTwoFA(&changed) {
			t.Fatal("skip allowed for a disabled or non-staff account")
		}
	}
}

func TestPasswordChangeRefreshesOnlyTheInitiatingSessionProof(t *testing.T) {
	t.Setenv("STARHUB_ENV", "production")
	setupAutoRoutingDB(t)
	user := &model.User{Username: "password-proof", Password: "old-hash", Role: common.RoleAdmin, Status: common.StatusEnabled, AdminPasswordChangedAt: 1, AffCode: common.GenerateAffCode()}
	if err := model.CreateUser(user); err != nil {
		t.Fatal(err)
	}
	current := Sessions.CreateConfirmed(user, time.Hour, "", "")
	other := Sessions.CreateConfirmed(user, time.Hour, "", "")
	t.Cleanup(func() { Sessions.Remove(current); Sessions.Remove(other) })
	if !Sessions.SkipStaffTwoFA(current, user) || !Sessions.SkipStaffTwoFA(other, user) {
		t.Fatal("initial skips failed")
	}
	if err := model.UpdateOwnedProfile(user, "password", "new-hash"); err != nil {
		t.Fatal(err)
	}
	fresh, err := model.GetUserByID(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !StaffSecurityRequired(current, fresh) || !StaffSecurityRequired(other, fresh) {
		t.Fatal("password change did not invalidate skip authorization")
	}
	if Sessions.SkipStaffTwoFA(other, fresh) || Sessions.SkipStaffTwoFA(current, fresh) {
		t.Fatal("an old password proof obtained a new skip")
	}
	if Sessions.RefreshPasswordProof(current, user, "uncommitted-hash") {
		t.Fatal("uncommitted password proof was accepted")
	}
	if !Sessions.RefreshPasswordProof(current, user, "new-hash") {
		t.Fatal("verified password change did not refresh the initiating session")
	}
	if !StaffSecurityRequired(current, fresh) {
		t.Fatal("refresh silently preserved the previous skip")
	}
	if !Sessions.CurrentGeneration(other, fresh) || Sessions.SkipStaffTwoFA(other, fresh) {
		t.Fatal("another active session inherited the new proof")
	}
	if !Sessions.SkipStaffTwoFA(current, fresh) || StaffSecurityRequired(current, fresh) || Sessions.StaffVerified(current, fresh) {
		t.Fatal("fresh password proof did not allow an explicit skip without factor evidence")
	}
	Sessions.Remove(current)
	if Sessions.RefreshPasswordProof(current, user, "new-hash") {
		t.Fatal("removed session was refreshed")
	}
}

func TestPasswordProofRefreshDoesNotRenewFactorEvidence(t *testing.T) {
	t.Setenv("STARHUB_ENV", "production")
	setupAutoRoutingDB(t)
	user := &model.User{Username: "factor-password-proof", Password: "old-hash", Role: common.RoleAdmin, Status: common.StatusEnabled, AdminPasswordChangedAt: 1, AffCode: common.GenerateAffCode()}
	if err := model.CreateUser(user); err != nil {
		t.Fatal(err)
	}
	if err := model.DB.Create(&model.TwoFA{UserID: user.ID, Secret: "verified-factor", IsEnabled: true}).Error; err != nil {
		t.Fatal(err)
	}
	session := Sessions.CreateConfirmed(user, time.Hour, "", "", "verified-factor")
	t.Cleanup(func() { Sessions.Remove(session) })
	if !Sessions.StaffVerified(session, user) {
		t.Fatal("factor session was not verified")
	}
	if err := model.UpdateOwnedProfile(user, "password", "new-hash"); err != nil {
		t.Fatal(err)
	}
	fresh, err := model.GetUserByID(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !Sessions.RefreshPasswordProof(session, user, "new-hash") {
		t.Fatal("password proof refresh failed")
	}
	if Sessions.StaffVerified(session, fresh) || !StaffSecurityRequired(session, fresh) || Sessions.SkipStaffTwoFA(session, fresh) {
		t.Fatal("password proof refresh renewed factor authority")
	}
}
