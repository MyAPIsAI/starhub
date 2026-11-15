package model

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestVerifiedBackupAndRestoreNeverOverwriteExistingFiles(t *testing.T) {
	newTestDB(t)
	ctx := context.Background()
	snapshot, err := CreateSQLiteBackup(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "restored.db")
	if err := RestoreSQLiteSnapshot(ctx, snapshot, target); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if RestoreSQLiteSnapshot(ctx, snapshot, target) == nil {
		t.Fatal("existing database overwritten")
	}
	after, _ := os.ReadFile(target)
	if string(before) != string(after) {
		t.Fatal("restore failure changed destination")
	}
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := CreateSQLiteBackup(ctx, t.TempDir()); err == nil {
		t.Fatal("cancelled backup published")
	}
}
