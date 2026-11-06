package model

import (
	"path/filepath"
	"testing"
)

func TestServiceAnnouncementPersistenceAndAuditRollback(t *testing.T) {
	path := filepath.Join(t.TempDir(), "announcements.db")
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = CloseDB() })
	input := AnnouncementWrite{OperationID: "notice-test", Title: AnnouncementText{"Title ZH", "Title EN", "Title AR"}, Content: AnnouncementText{"Description ZH", "Description EN", "Description AR"}, Vendors: []string{"fixture"}, Status: "investigating"}
	if err := DB.Exec("CREATE TRIGGER reject_notice_audit BEFORE INSERT ON audit_logs WHEN NEW.action = 'announcement_published' BEGIN SELECT RAISE(ABORT, 'synthetic audit failure'); END").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := SaveServiceAnnouncement(t.Context(), 0, input, 1, "127.0.0.1"); err == nil {
		t.Fatal("failed audit did not reject publication")
	}
	public, err := ListServiceAnnouncements(t.Context(), "active", 1, true)
	if err != nil || public.Total != 0 {
		t.Fatal("unaudited publication became visible")
	}
	if err := DB.Exec("DROP TRIGGER reject_notice_audit").Error; err != nil {
		t.Fatal(err)
	}
	row, err := SaveServiceAnnouncement(t.Context(), 0, input, 1, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if err := CloseDB(); err != nil {
		t.Fatal(err)
	}
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	public, err = ListServiceAnnouncements(t.Context(), "active", 1, true)
	if err != nil || public.Total != 1 || public.Items[0].ID != row.ID || public.Items[0].Title != input.Title {
		t.Fatalf("publication lost after restart: %+v %v", public, err)
	}
	input.Content.EN = "Different retry"
	if _, err := SaveServiceAnnouncement(t.Context(), 0, input, 1, "127.0.0.1"); err != ErrAdminConflict {
		t.Fatalf("conflicting creation retry accepted: %v", err)
	}
}
