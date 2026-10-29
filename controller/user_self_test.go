package controller

import (
	"fmt"
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

func TestDeactivateSelf(t *testing.T) {
	newTestDB(t)
	u := newUser(t, "alice", common.RoleUser, 1000)
	state, _ := model.ReadAccountDeactivationState(u.ID)

	rr := call(DeactivateSelf, testRequest{
		method: http.MethodPost,
		target: "/api/user/deactivate",
		body:   fmt.Sprintf(`{"schema":"deactivation_action_v1","expected_user_id":%d,"account_revision":"%s","operation_id":"%s"}`, u.ID, state.AccountRevision, "00000000000000000000000000000001"),
		userID: u.ID,
		role:   common.RoleUser,
	})
	expectStatus(t, rr, http.StatusOK)

	// Verify the user is now disabled.
	user, _ := model.GetUserByID(u.ID)
	if user.Status != common.StatusDisabled {
		t.Fatalf("status = %d, want %d (disabled)", user.Status, common.StatusDisabled)
	}
}

func TestDeactivateSelfPreservesData(t *testing.T) {
	newTestDB(t)
	u := newUser(t, "alice", common.RoleUser, 5000)

	// Create a token and a wallet so we can verify they survive.
	mustToken(t, u.ID, 1000)
	model.GetOrCreateWallet(u.ID)
	state, _ := model.ReadAccountDeactivationState(u.ID)

	call(DeactivateSelf, testRequest{
		method: http.MethodPost,
		target: "/api/user/deactivate",
		body:   fmt.Sprintf(`{"schema":"deactivation_action_v1","expected_user_id":%d,"account_revision":"%s","operation_id":"%s"}`, u.ID, state.AccountRevision, "00000000000000000000000000000002"),
		userID: u.ID,
		role:   common.RoleUser,
	})

	// Tokens should still exist.
	tokens, _ := model.ListTokens(u.ID)
	if len(tokens) != 1 {
		t.Fatalf("tokens count = %d, want 1 (preserved)", len(tokens))
	}
	// Wallet should still exist.
	wallet, err := model.GetOrCreateWallet(u.ID)
	if err != nil {
		t.Fatalf("wallet error: %v", err)
	}
	if wallet.Balance != 0 {
		t.Fatalf("balance = %d, want 0", wallet.Balance)
	}
}
