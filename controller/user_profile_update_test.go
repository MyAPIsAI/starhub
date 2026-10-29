package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"starhub/common"
	"starhub/middleware"
	"starhub/model"
)

func TestGuardedProfileUpdateKeepsSessionsAndOnlyChangesTheSelectedField(t *testing.T) {
	newTestDB(t)
	owner := newUser(t, "profile-write", common.RoleUser, 123456)
	current := middleware.Sessions.Create(owner.ID, time.Hour)
	other := middleware.Sessions.Create(owner.ID, time.Hour)
	defer middleware.Sessions.Remove(current)
	defer middleware.Sessions.Remove(other)
	write := func(body string) {
		t.Helper()
		response := call(UpdateProfileState, testRequest{method: http.MethodPut, target: "/api/user/profile-state", userID: owner.ID, body: body})
		var receipt map[string]json.RawMessage
		decodeData(t, response, &receipt)
		if len(receipt) != 2 || string(receipt["user_id"]) != fmt.Sprint(owner.ID) || len(response.Result().Cookies()) != 0 {
			t.Fatal("invalid profile receipt")
		}
		for _, secret := range []string{current, other, owner.Password, "new-profile-password"} {
			if strings.Contains(response.Body.String(), secret) {
				t.Fatal("profile write leaked credential material")
			}
		}
	}
	write(fmt.Sprintf(`{"expected_user_id":%d,"expected_email":"","email":"owner@example.test"}`, owner.ID))
	updated, _ := model.GetUserByID(owner.ID)
	if updated.Email != "owner@example.test" || updated.Password != owner.Password || updated.Quota != owner.Quota {
		t.Fatal("email write changed unrelated fields")
	}
	write(fmt.Sprintf(`{"expected_user_id":%d,"expected_email":"owner@example.test","current_password":"secret","new_password":"new-profile-password"}`, owner.ID))
	updated, _ = model.GetUserByID(owner.ID)
	if !common.VerifyPassword("new-profile-password", updated.Password) || common.VerifyPassword("secret", updated.Password) || updated.Email != "owner@example.test" || updated.Quota != owner.Quota {
		t.Fatal("password write changed unrelated fields or failed to rotate")
	}
	for _, session := range []string{current, other} {
		if id, ok := middleware.Sessions.Get(session); !ok || id != owner.ID {
			t.Fatal("profile update changed active devices")
		}
	}
}

func TestGuardedProfileUpdateRejectsMalformedStaleAndWrongPasswordRequests(t *testing.T) {
	newTestDB(t)
	owner := newUser(t, "profile-reject", common.RoleUser, 1)
	root := newUser(t, "profile-reject-root", common.RoleRoot, 2)
	base := fmt.Sprintf(`"expected_user_id":%d,"expected_email":""`, owner.ID)
	for _, body := range []string{`null`, `{}`, `{` + base + `}`, `{` + base + `,"email":"x","new_password":"password123"}`, `{` + base + `,"email":"a@b.test","extra":true}`, `{` + base + `,"email":" spaced@b.test "}`, `{` + base + `,"email":"a@b.test","current_password":"secret"}`, `{` + base + `,"new_password":"short","current_password":"secret"}`, `{` + base + `,"new_password":"password123"}`, `{` + base + `,"email":"a@b.test"} {}`, strings.Repeat(" ", 4097) + `{}`} {
		if response := call(UpdateProfileState, testRequest{method: http.MethodPut, target: "/api/user/profile-state", userID: owner.ID, body: body}); response.Code != 400 {
			t.Fatalf("malformed profile update accepted: %d %s", response.Code, response.Body)
		}
	}
	for _, test := range []struct {
		body         string
		actor        int64
		role, status int
	}{
		{`{` + base + `,"new_password":"password123","current_password":"wrong"}`, owner.ID, 1, 403},
		{fmt.Sprintf(`{"expected_user_id":%d,"expected_email":"stale@example.test","email":"new@example.test"}`, owner.ID), owner.ID, 1, 409},
		{`{` + base + `,"email":"a@b.test"}`, root.ID, common.RoleRoot, 409},
	} {
		response := call(UpdateProfileState, testRequest{method: http.MethodPut, target: "/api/user/profile-state", userID: test.actor, role: test.role, body: test.body})
		if response.Code != test.status {
			t.Fatalf("invalid update result: %d %s", response.Code, response.Body)
		}
	}
	updated, _ := model.GetUserByID(owner.ID)
	if *updated != *owner {
		t.Fatal("rejected profile write changed the account")
	}
}

func TestOwnedProfileCompareAndSwapRejectsConcurrentSecurityChanges(t *testing.T) {
	for _, field := range []string{"email", "password", "role", "group", "status"} {
		t.Run(field, func(t *testing.T) {
			newTestDB(t)
			owner := newUser(t, "profile-cas", 1, 123)
			var value any = "changed"
			if field == "status" {
				value = 2
			}
			if field == "role" {
				value = 5
			}
			if err := model.UpdateUserFields(owner.ID, map[string]any{field: value}); err != nil {
				t.Fatal(err)
			}
			if err := model.UpdateOwnedProfile(owner, "email", "must-not-win@example.test"); err != model.ErrUserProfileConflict {
				t.Fatalf("stale %s snapshot overwrote current state: %v", field, err)
			}
			current, _ := model.GetUserByID(owner.ID)
			if current.Email == "must-not-win@example.test" || current.Quota != 123 {
				t.Fatal("conflicting write changed state")
			}
		})
	}
	t.Run("one concurrent winner", func(t *testing.T) {
		newTestDB(t)
		owner := newUser(t, "profile-concurrent", 1, 123)
		var wait sync.WaitGroup
		results := make(chan error, 8)
		for i := 0; i < 8; i++ {
			wait.Add(1)
			go func(i int) {
				defer wait.Done()
				results <- model.UpdateOwnedProfile(owner, "email", fmt.Sprintf("winner%d@example.test", i))
			}(i)
		}
		wait.Wait()
		close(results)
		wins := 0
		for err := range results {
			if err == nil {
				wins++
			} else if err != model.ErrUserProfileConflict {
				t.Fatal(err)
			}
		}
		if wins != 1 {
			t.Fatalf("concurrent winners: %d", wins)
		}
	})
}
