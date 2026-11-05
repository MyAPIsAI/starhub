package middleware

import (
	"net/http"
	"net/http/httptest"
	"starhub/common"
	"starhub/model"
	"testing"
	"time"
)

func verifiedStaffFixture(t *testing.T, user *model.User) string {
	t.Helper()
	if err := model.DB.Model(&model.StaffAccount{}).Where("id = ?", user.ID).Update("must_change_password", false).Error; err != nil {
		t.Fatal(err)
	}
	secret, err := model.NewTOTPSecret()
	if err != nil {
		t.Fatal(err)
	}
	code, _ := model.TOTPCode(secret, time.Now().Unix()/30)
	fresh, err := model.VerifyStaffOTP(user.ID, user.SecurityVersion, code, secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	token := Sessions.CreateStaffConfirmed(fresh, true, time.Hour)
	if token == "" {
		t.Fatal("no staff fixture session")
	}
	return token
}
func TestAdminAuthRequiresVerifiedStaffIdentityAndCurrentRole(t *testing.T) {
	setupAutoRoutingDB(t)
	user := &model.User{Username: "staff-role-boundary", Status: 1, Role: common.RoleRoot}
	if err := model.CreateUser(user); err != nil {
		t.Fatal(err)
	}
	session := verifiedStaffFixture(t, user)
	handler := AdminAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	for _, role := range []int{1, 5, 10, 20, 100, 99, 101} {
		if err := model.DB.Model(&model.StaffAccount{}).Where("id = ?", user.ID).Updates(map[string]any{"role": role}).Error; err != nil {
			t.Fatal(err)
		}
		for _, path := range []string{"/api/metrics", "/api/admin/system", "/api/admin/unmapped"} {
			r := httptest.NewRequest("GET", path, nil)
			r.RemoteAddr = "127.0.0.1:1234"
			r.AddCookie(&http.Cookie{Name: "staff_session", Value: session})
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			want := 403
			if !common.IsStaffRole(role) {
				want = 401
			} else if path == "/api/metrics" && (role == 5 || role == 10 || role == 100) || path == "/api/admin/system" && (role == 20 || role == 100) {
				want = 204
			}
			if w.Code != want {
				t.Fatalf("role %d %s: got %d want %d", role, path, w.Code, want)
			}
		}
	}
}
