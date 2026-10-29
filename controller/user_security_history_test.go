package controller

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"starhub/common"
	"starhub/model"
)

func TestUserSecurityHistoryIsAtomicOwnedAndDoesNotInventOldEvents(t *testing.T) {
	newTestDB(t)
	owner := newUser(t, "history-owner", 1, 123)
	other := newUser(t, "history-other", 100, 0)
	if events, err := model.ListUserSecurityEvents(owner.ID); err != nil || len(events) != 0 {
		t.Fatal("migration invented historical activity")
	}
	for i := 0; i < 7; i++ {
		snapshot, _ := model.GetUserByID(owner.ID)
		if err := model.UpdateOwnedProfile(snapshot, "email", fmt.Sprintf("history%d@example.test", i)); err != nil {
			t.Fatal(err)
		}
	}
	snapshot, _ := model.GetUserByID(owner.ID)
	if err := model.UpdateOwnedProfile(snapshot, "email", snapshot.Email); err != nil {
		t.Fatal(err)
	}
	var count int64
	model.DB.Model(&model.UserSecurityEvent{}).Where("user_id = ?", owner.ID).Count(&count)
	if count != 7 {
		t.Fatal("same-email save added an invented change")
	}
	response := call(GetUserSecurityState, testRequest{target: fmt.Sprintf("/api/user/security-state?expected_user_id=%d", owner.ID), userID: owner.ID})
	var data struct {
		Schema string                    `json:"schema"`
		Scope  string                    `json:"history_scope"`
		Owner  int64                     `json:"user_id"`
		Events []model.UserSecurityEvent `json:"events"`
	}
	decodeData(t, response, &data)
	if data.Schema != "security_state_v1" || data.Scope != "guarded_profile_changes_v1" || data.Owner != owner.ID || len(data.Events) != 5 {
		t.Fatal("invalid bounded owned history contract")
	}
	for i, event := range data.Events {
		if event.Action != "email_changed" || event.CreatedAt <= 0 || (i > 0 && event.ID >= data.Events[i-1].ID) {
			t.Fatal("wrong security history ordering or metadata")
		}
	}
	if strings.Contains(response.Body.String(), "@example.test") || strings.Contains(response.Body.String(), owner.Password) {
		t.Fatal("history leaked contact or password material")
	}
	for _, path := range []string{fmt.Sprintf("/api/user/security-state?expected_user_id=%d", owner.ID), "/api/user/security-state?expected_user_id=99999"} {
		if rr := call(GetUserSecurityState, testRequest{target: path, userID: other.ID, role: common.RoleRoot}); rr.Code != 409 {
			t.Fatal("Root bypassed history ownership")
		}
	}
	if rr := call(GetUserSecurityState, testRequest{target: "/api/user/security-state?expected_user_id=1&expected_user_id=1", userID: owner.ID}); rr.Code != 400 {
		t.Fatal("ambiguous history owner accepted")
	}
	if events, err := model.ListUserSecurityEvents(other.ID); err != nil || len(events) != 0 {
		t.Fatal("history crossed owner boundaries")
	}
}

func TestProfileWriteRollsBackWhenSecurityHistoryCannotBePersisted(t *testing.T) {
	newTestDB(t)
	owner := newUser(t, "history-rollback", 1, 123)
	if err := model.DB.Migrator().DropTable(&model.UserSecurityEvent{}); err != nil {
		t.Fatal(err)
	}
	if err := model.UpdateOwnedProfile(owner, "email", "must-rollback@example.test"); err == nil {
		t.Fatal("profile changed without durable security event")
	}
	current, _ := model.GetUserByID(owner.ID)
	if *current != *owner {
		t.Fatal("event failure left a partial profile update")
	}
	if err := model.DB.AutoMigrate(&model.UserSecurityEvent{}); err != nil {
		t.Fatal(err)
	}
	if err := model.UpdateOwnedProfile(owner, "email", "saved@example.test"); err != nil {
		t.Fatal(err)
	}
	events, err := model.ListUserSecurityEvents(owner.ID)
	if err != nil || len(events) != 1 || events[0].Action != "email_changed" {
		t.Fatal("successful retry was not recorded exactly once")
	}
	response := call(UpdateProfileState, testRequest{method: http.MethodPut, target: "/api/user/profile-state", userID: owner.ID, body: fmt.Sprintf(`{"expected_user_id":%d,"expected_email":"saved@example.test","current_password":"wrong","new_password":"replacement-password"}`, owner.ID)})
	if response.Code != 403 {
		t.Fatal("wrong password accepted")
	}
	events, _ = model.ListUserSecurityEvents(owner.ID)
	if len(events) != 1 {
		t.Fatal("failed password verification invented an event")
	}
}
