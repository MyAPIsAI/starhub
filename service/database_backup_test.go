package service

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBackupRetentionOnlyRemovesManagedSnapshots(t *testing.T) {
	dir := t.TempDir()
	names := []string{"starhub-backup-20261115-000000-00000000-0000-0000-0000-000000000001.db", "starhub-backup-20261115-000001-00000000-0000-0000-0000-000000000002.db", "unrelated.db"}
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("test"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := pruneDatabaseBackups(dir, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, names[0])); !os.IsNotExist(err) {
		t.Fatal("old snapshot retained")
	}
	for _, name := range names[1:] {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatal("retention deleted protected file", err)
		}
	}
}
