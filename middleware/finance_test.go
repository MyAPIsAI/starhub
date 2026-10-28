package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"starhub/common"
	"starhub/model"
)

func TestFinanceAuthReadsCurrentRoleAndDeniesWrites(t *testing.T) {
	setupAutoRoutingDB(t)
	user := &model.User{Username: "finance-session", Role: common.RoleFinance, Status: common.StatusEnabled, Group: "default"}
	if err := model.CreateUser(user); err != nil {
		t.Fatal(err)
	}
	session := Sessions.Create(user.ID, time.Hour)
	defer Sessions.Remove(session)
	handler := FinanceAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	call := func(method, path, key string) int {
		r := httptest.NewRequest(method, path, nil)
		r.AddCookie(&http.Cookie{Name: "session", Value: key})
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, r)
		return rec.Code
	}
	if status := call("GET", "/api/finance/summary", ""); status != 401 {
		t.Fatalf("unauthenticated finance read: %d", status)
	}
	for _, role := range []int{common.RoleUser, common.RoleFinance, common.RoleAdmin, common.RoleRoot, 99} {
		if err := model.UpdateUserFields(user.ID, map[string]any{"role": role}); err != nil {
			t.Fatal(err)
		}
		want := 403
		if role == common.RoleFinance || role == common.RoleAdmin || role == common.RoleRoot {
			want = 204
		}
		if status := call("GET", "/api/finance/summary", session); status != want {
			t.Fatalf("role %d read=%d want=%d", role, status, want)
		}
		for _, path := range []string{"/api/finance/summary", "/api/finance-lookalike"} {
			if status := call("PUT", path, session); status != 403 {
				t.Fatalf("finance mutation bypass: %d %s %d", role, path, status)
			}
		}
	}
	if err := model.UpdateUserFields(user.ID, map[string]any{"role": common.RoleFinance, "status": common.StatusDisabled}); err != nil {
		t.Fatal(err)
	}
	if status := call("GET", "/api/finance/summary", session); status != 401 {
		t.Fatalf("disabled finance session accepted: %d", status)
	}
	if err := model.UpdateUserFields(user.ID, map[string]any{"status": common.StatusEnabled}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/channel", "/api/finance-lookalike", "/console"} {
		if status := call("GET", path, session); status != 403 {
			t.Fatalf("finance middleware accepted unrelated path %s: %d", path, status)
		}
	}
}
