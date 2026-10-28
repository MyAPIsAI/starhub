package model

import (
	"path/filepath"
	"testing"
)

func TestQualityIndexesMigrateWithoutChangingHistoricalLogs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "quality-migration.db")
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = CloseDB() })
	for _, name := range []string{"idx_logs_quality_window", "idx_logs_quality_model"} {
		if !DB.Migrator().HasIndex(&Log{}, name) {
			t.Fatalf("missing quality index %s", name)
		}
		if err := DB.Migrator().DropIndex(&Log{}, name); err != nil {
			t.Fatal(err)
		}
	}
	entry := &Log{UserID: 1, Model: "legacy", RequestedModel: "veyra-auto", Protocol: "", Quota: 17, UnfundedQuota: 3, DurationMs: 125, Status: 2, CreatedAt: 1700000000}
	if err := InsertLog(entry); err != nil {
		t.Fatal(err)
	}
	before := *entry
	if err := CloseDB(); err != nil {
		t.Fatal(err)
	}
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"idx_logs_quality_window", "idx_logs_quality_model"} {
		if !DB.Migrator().HasIndex(&Log{}, name) {
			t.Fatalf("index %s was not added to existing logs", name)
		}
	}
	var after Log
	if err := DB.First(&after, entry.ID).Error; err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("index migration rewrote historical accounting: before=%+v after=%+v", before, after)
	}
}
