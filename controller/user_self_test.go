package controller

import (
	"net/http"
	"testing"

	"starhub/common"
	"starhub/model"
)

func TestUpdateSelfChangePassword(t *testing.T) {
	newTestDB(t)
	u := newUser(t, "alice", common.RoleUser, 1000)

	rr := call(UpdateSelf, testRequest{
		method: http.MethodPut,
		target: "/api/user/self",
		body:   `{"current_password":"secret","new_password":"newpass123"}`,
		userID: u.ID,
		role:   common.RoleUser,
	})
	var got map[string]any
	decodeData(t, rr, &got)
	if got["username"] != "alice" {
		t.Fatalf("username = %v, want alice", got["username"])
	}

	// Verify the new password works.
	user, _ := model.GetUserByID(u.ID)
	if !common.VerifyPassword("newpass123", user.Password) {
		t.Fatal("new password does not verify")
	}
	if common.VerifyPassword("secret", user.Password) {
		t.Fatal("old password should no longer verify")
	}
}

func TestUpdateSelfPasswordWrongCurrent(t *testing.T) {
	newTestDB(t)
	u := newUser(t, "alice", common.RoleUser, 1000)

	rr := call(UpdateSelf, testRequest{
		method: http.MethodPut,
		target: "/api/user/self",
		body:   `{"current_password":"wrong","new_password":"newpass123"}`,
		userID: u.ID,
		role:   common.RoleUser,
	})
	expectError(t, rr, http.StatusForbidden, "current password")
}

func TestUpdateSelfPasswordMissingCurrent(t *testing.T) {
	newTestDB(t)
	u := newUser(t, "alice", common.RoleUser, 1000)

	rr := call(UpdateSelf, testRequest{
		method: http.MethodPut,
		target: "/api/user/self",
		body:   `{"new_password":"newpass123"}`,
		userID: u.ID,
		role:   common.RoleUser,
	})
	expectError(t, rr, http.StatusForbidden, "current password")
}

func TestUpdateSelfPasswordTooShort(t *testing.T) {
	newTestDB(t)
	u := newUser(t, "alice", common.RoleUser, 1000)

	rr := call(UpdateSelf, testRequest{
		method: http.MethodPut,
		target: "/api/user/self",
		body:   `{"current_password":"secret","new_password":"abc"}`,
		userID: u.ID,
		role:   common.RoleUser,
	})
	expectError(t, rr, http.StatusBadRequest, "password must be")
}

func TestUpdateSelfChangeEmail(t *testing.T) {
	newTestDB(t)
	u := newUser(t, "alice", common.RoleUser, 1000)

	rr := call(UpdateSelf, testRequest{
		method: http.MethodPut,
		target: "/api/user/self",
		body:   `{"email":"alice@example.com"}`,
		userID: u.ID,
		role:   common.RoleUser,
	})
	var got map[string]any
	decodeData(t, rr, &got)
	if got["email"] != "alice@example.com" {
		t.Fatalf("email = %v, want alice@example.com", got["email"])
	}
}

func TestUpdateSelfInvalidEmail(t *testing.T) {
	newTestDB(t)
	u := newUser(t, "alice", common.RoleUser, 1000)

	rr := call(UpdateSelf, testRequest{
		method: http.MethodPut,
		target: "/api/user/self",
		body:   `{"email":"not-an-email"}`,
		userID: u.ID,
		role:   common.RoleUser,
	})
	expectError(t, rr, http.StatusBadRequest, "invalid email")
}

func TestUpdateSelfNothingToUpdate(t *testing.T) {
	newTestDB(t)
	u := newUser(t, "alice", common.RoleUser, 1000)

	rr := call(UpdateSelf, testRequest{
		method: http.MethodPut,
		target: "/api/user/self",
		body:   `{}`,
		userID: u.ID,
		role:   common.RoleUser,
	})
	expectError(t, rr, http.StatusBadRequest, "nothing to update")
}

func TestUpdateSelfPasswordAndEmailTogether(t *testing.T) {
	newTestDB(t)
	u := newUser(t, "alice", common.RoleUser, 1000)

	rr := call(UpdateSelf, testRequest{
		method: http.MethodPut,
		target: "/api/user/self",
		body:   `{"current_password":"secret","new_password":"newpass456","email":"bob@example.com"}`,
		userID: u.ID,
		role:   common.RoleUser,
	})
	var got map[string]any
	decodeData(t, rr, &got)
	if got["email"] != "bob@example.com" {
		t.Fatalf("email = %v, want bob@example.com", got["email"])
	}
	user, _ := model.GetUserByID(u.ID)
	if !common.VerifyPassword("newpass456", user.Password) {
		t.Fatal("new password does not verify")
	}
}
