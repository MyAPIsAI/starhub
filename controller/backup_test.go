package controller

import (
	"encoding/json"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"net/http"
	"os"
	"path/filepath"
	"starhub/common"
	"starhub/model"
	"testing"
	"time"
)

func TestManualBackupContainsConsistentMoneySnapshot(t *testing.T) {
	newTestDB(t)
	directory := t.TempDir()
	t.Setenv("STARHUB_BACKUP_DIR", directory)
	user := newUser(t, "backup-owner", common.RoleUser, 1000)
	if _, err := model.ReadMoneyAccount(user.ID); err != nil {
		t.Fatal(err)
	}
	response := call(BackupDatabase, testRequest{method: "POST", target: "/api/backup"})
	envelope := expectStatus(t, response, http.StatusOK)
	var result struct {
		Backup string `json:"backup"`
		Path   string `json:"path"`
		Size   string `json:"size"`
	}
	if err := json.Unmarshal(envelope.Data, &result); err != nil {
		t.Fatal(err)
	}
	if !envelope.Success || result.Path != filepath.Join(directory, result.Backup) || filepath.Base(result.Backup) != result.Backup || result.Size == "0" {
		t.Fatal("invalid backup result")
	}
	copy, err := gorm.Open(sqlite.Open(result.Path), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := copy.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	var integrity string
	if err := copy.Raw("PRAGMA integrity_check").Scan(&integrity).Error; err != nil || integrity != "ok" {
		t.Fatal("invalid SQLite snapshot", err)
	}
	var saved model.User
	if err := copy.First(&saved, user.ID).Error; err != nil || saved.Quota != 1000 {
		t.Fatal("missing account snapshot", err)
	}
	var head model.MoneyAccount
	if err := copy.First(&head, "user_id = ?", user.ID).Error; err != nil || head.AvailableNano != "2000000" || head.Sequence != 1 {
		t.Fatal("money snapshot not consistent", err)
	}
	if _, err := model.AdjustUserQuota(user.ID, 10, user.Quota, 1, "Verify backup isolation"); err != nil {
		t.Fatal(err)
	}
	if err := copy.First(&saved, user.ID).Error; err != nil || saved.Quota != 1000 {
		t.Fatal("backup changed with live account", err)
	}
}

func TestManualBackupFailureDoesNotOverwriteExistingFiles(t *testing.T) {
	newTestDB(t)
	directory := t.TempDir()
	t.Setenv("STARHUB_BACKUP_DIR", directory)
	// Precreate all plausible second-resolution destinations without depending
	// on the handler and test observing the same clock tick.
	marker := []byte("existing snapshot must remain unchanged")
	for offset := -2; offset <= 30; offset++ {
		name := "starhub-backup-" + time.Now().UTC().Add(time.Duration(offset)*time.Second).Format("20060102-150405") + ".db"
		if err := os.WriteFile(filepath.Join(directory, name), marker, 0600); err != nil {
			t.Fatal(err)
		}
	}
	response := call(BackupDatabase, testRequest{method: "POST", target: "/api/backup"})
	envelope := expectStatus(t, response, http.StatusInternalServerError)
	if envelope.Success {
		t.Fatal("existing destination reported backup success")
	}
	files, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		data, err := os.ReadFile(filepath.Join(directory, file.Name()))
		if err != nil || string(data) != string(marker) {
			t.Fatal("existing backup was overwritten", err)
		}
	}
	t.Setenv("STARHUB_BACKUP_DIR", filepath.Join(directory, "missing-directory"))
	response = call(BackupDatabase, testRequest{method: "POST", target: "/api/backup"})
	if response.Code != 500 {
		t.Fatalf("missing destination returned %d", response.Code)
	}
}
