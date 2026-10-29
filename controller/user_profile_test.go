package controller

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"starhub/common"
	"starhub/model"
)

func TestProfileStateGuardsOwnerAndReturnsOnlyProfileFields(t *testing.T) {
	newTestDB(t)
	owner := newUser(t, "profile-owner", common.RoleFinance, 123456)
	if err := model.UpdateUserFields(owner.ID, map[string]any{"email": "owner@example.test"}); err != nil {
		t.Fatal(err)
	}
	root := newUser(t, "profile-root", common.RoleRoot, 0)
	path := fmt.Sprintf("/api/user/profile-state?expected_user_id=%d", owner.ID)
	response := call(GetProfileState, testRequest{target: path, userID: owner.ID})
	var got struct {
		Schema string                     `json:"schema"`
		User   map[string]json.RawMessage `json:"user"`
	}
	decodeData(t, response, &got)
	if got.Schema != "profile_state_v1" || len(got.User) != 6 || string(got.User["id"]) != fmt.Sprint(owner.ID) || string(got.User["email"]) != `"owner@example.test"` || string(got.User["role"]) != fmt.Sprint(common.RoleFinance) || string(got.User["created_at"]) != fmt.Sprint(owner.CreatedAt) {
		t.Fatal("profile contract is not the current stored owner metadata")
	}
	for _, field := range []string{"id", "username", "email", "role", "status", "created_at"} {
		if _, ok := got.User[field]; !ok {
			t.Fatalf("missing profile field: %s", field)
		}
	}
	if strings.Contains(response.Body.String(), owner.Password) || strings.Contains(response.Body.String(), "quota") || strings.Contains(response.Body.String(), "password") || len(response.Result().Cookies()) != 0 {
		t.Fatal("profile read exposed unrelated data or wrote a cookie")
	}
	for _, query := range []string{"", "?expected_user_id=0", "?expected_user_id=-1", "?expected_user_id=01", "?expected_user_id=x", "?expected_user_id=9223372036854775808", "?expected_user_id=1&expected_user_id=1", "?expected_user_id=1&other=1", "?expected_user_id=%zz"} {
		if rr := call(GetProfileState, testRequest{target: "/api/user/profile-state" + query, userID: owner.ID}); rr.Code != 400 {
			t.Fatalf("invalid owner query accepted: %s %d", query, rr.Code)
		}
	}
	for _, expected := range []int64{owner.ID, root.ID + 10000} {
		if rr := call(GetProfileState, testRequest{target: fmt.Sprintf("/api/user/profile-state?expected_user_id=%d", expected), userID: root.ID, role: common.RoleRoot}); rr.Code != 409 {
			t.Fatal("Root bypassed personal owner guard")
		}
	}
	updated, err := model.GetUserByID(owner.ID)
	if err != nil || updated.Quota != owner.Quota || updated.Password != owner.Password {
		t.Fatal("profile reads changed private account state")
	}
}
