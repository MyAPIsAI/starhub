package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"starhub/common"
	"starhub/model"
)

func TestAdminAuthRequiresAnExplicitAdministrativeRole(t *testing.T) {
	setupAutoRoutingDB(t)
	user := &model.User{Username: "admin-role-boundary", Status: common.StatusEnabled, Role: common.RoleUser}
	if err := model.CreateUser(user); err != nil {
		t.Fatal(err)
	}
	session := Sessions.Create(user.ID, time.Hour)
	t.Cleanup(func() { Sessions.Remove(session) })
	handler := AdminAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	for _, role := range []int{common.RoleUser, common.RoleFinance, common.RoleAdmin, common.RoleRoot, 99, 101, 1000} {
		if err := model.UpdateUserFields(user.ID, map[string]any{"role": role}); err != nil {
			t.Fatal(err)
		}
		for _, path := range []string{"/api/metrics", "/api/plaza/meta/configuration", "/api/unmapped-management"} {
			r := httptest.NewRequest("GET", path, nil)
			r.AddCookie(&http.Cookie{Name: "session", Value: session})
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, r)
			want := 403
			if role == common.RoleAdmin || role == common.RoleRoot {
				want = 204
			}
			if rec.Code != want {
				t.Fatalf("role %d path %s got %d want %d", role, path, rec.Code, want)
			}
		}
	}
}
