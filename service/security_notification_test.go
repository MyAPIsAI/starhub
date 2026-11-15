package service

import (
	"context"
	"errors"
	"testing"

	"starhub/model"
)

func TestSecurityNotificationsRetainFailuresAndFrozenLanguage(t *testing.T) {
	newTestDB(t)
	p := &mockEmailProvider{err: errors.New("private provider details")}
	SetEmailProvider(p)
	t.Cleanup(func() { SetEmailProvider(nil) })
	row := model.SecurityNotification{ID: "event-1", UserID: 1, Email: "verified@example.test", Language: "zh", Action: "password_changed", Status: "pending"}
	if err := model.DB.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	if err := DeliverSecurityNotificationsOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	model.DB.First(&row, "id = ?", row.ID)
	if row.Status != "pending" || row.Attempts != 1 || row.LastError != "provider_unavailable" || p.lastBody != "您的账户密码已更改。如果并非本人操作，请联系管理员。" {
		t.Fatal("failed notification lost", row)
	}
	p.err = nil
	model.DB.Model(&row).UpdateColumn("next_attempt", 0)
	if err := DeliverSecurityNotificationsOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	model.DB.First(&row, "id = ?", row.ID)
	if row.Status != "accepted" || row.Attempts != 2 {
		t.Fatal("acknowledgment not persisted", row)
	}
	if err := DeliverSecurityNotificationsOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	model.DB.First(&row, "id = ?", row.ID)
	if row.Attempts != 2 {
		t.Fatal("accepted message resent")
	}
}
