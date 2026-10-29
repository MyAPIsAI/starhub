package model

import (
	"path/filepath"
	"testing"
)

func TestUserSecurityHistorySurvivesRestartWithoutBackfill(t *testing.T) {
	path := filepath.Join(t.TempDir(), "security-history.db")
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = CloseDB() })
	owner := &User{Username: "history-restart", Password: "synthetic-hash", Email: "before@example.test", Status: 1, Role: 1, Group: "default", AffCode: "history-restart-code"}
	if err := CreateUser(owner); err != nil {
		t.Fatal(err)
	}
	if events, err := ListUserSecurityEvents(owner.ID); err != nil || len(events) != 0 {
		t.Fatal("old account received invented history")
	}
	if err := UpdateOwnedProfile(owner, "email", "after@example.test"); err != nil {
		t.Fatal(err)
	}
	if err := CloseDB(); err != nil {
		t.Fatal(err)
	}
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	events, err := ListUserSecurityEvents(owner.ID)
	if err != nil || len(events) != 1 || events[0].Action != "email_changed" || events[0].UserID != owner.ID || events[0].CreatedAt <= 0 {
		t.Fatal("security event did not survive schema reload")
	}
	updated, err := GetUserByID(owner.ID)
	if err != nil || updated.Email != "after@example.test" || updated.Password != owner.Password {
		t.Fatal("profile state changed during restart")
	}
}
