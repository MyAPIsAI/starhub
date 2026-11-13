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
