package model

import (
	"path/filepath"
	"testing"

	"starhub/setting"
)

// TestOptionPersistenceAcrossDBReopen verifies that options saved to a
// real disk DB survive a close/reopen cycle. This addresses P21 section
// 9 item 6: "重载、重启、迁移使用真实磁盘 DB；区分重新查询与进程重启后恢复".
func TestOptionPersistenceAcrossDBReopen(t *testing.T) {
	// Use a real disk DB in a temp directory.
	dbPath := filepath.Join(t.TempDir(), "reopen.db")

	// Phase 1: open DB, save an option, close.
	if err := InitDB(dbPath); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	setting.ResetAllToDefaults()
	if err := LoadOptionsToSettings(); err != nil {
		t.Fatalf("LoadOptionsToSettings failed: %v", err)
	}

	// Save a known cache_ratio value.
	v1 := "2.5"
	if _, err := SaveOption(setting.OptionKeyCacheRatio, v1, 0); err != nil {
		t.Fatalf("SaveOption failed: %v", err)
	}

	// Reload settings from DB so the in-memory value reflects the save.
	if err := LoadOptionsToSettings(); err != nil {
		t.Fatalf("LoadOptionsToSettings after save failed: %v", err)
	}

	// Verify it's loaded in memory.
	if got := setting.CacheRatioValue(); got != 2.5 {
		t.Fatalf("before reopen: cache_ratio = %v, want 2.5", got)
	}

	// Close the DB (simulates process shutdown).
	if err := CloseDB(); err != nil {
		t.Fatalf("CloseDB failed: %v", err)
	}

	// Phase 2: reopen the same DB file (simulates process restart).
	if err := InitDB(dbPath); err != nil {
		t.Fatalf("InitDB reopen failed: %v", err)
	}
	t.Cleanup(func() { _ = CloseDB() })
	setting.ResetAllToDefaults()

	// Load options from the reopened DB.
	if err := LoadOptionsToSettings(); err != nil {
		t.Fatalf("LoadOptionsToSettings after reopen failed: %v", err)
	}

	// Verify the value survived the reopen.
	if got := setting.CacheRatioValue(); got != 2.5 {
		t.Fatalf("after reopen: cache_ratio = %v, want 2.5 (must survive)", got)
	}

	// Verify the persisted option row matches.
	value, ok, err := GetOption(setting.OptionKeyCacheRatio)
	if err != nil || !ok {
		t.Fatalf("GetOption after reopen: ok=%v err=%v", ok, err)
	}
	if value != v1 {
		t.Fatalf("persisted value = %q, want %q", value, v1)
	}

	// Verify the option version survived.
	version, err := OptionVersion()
	if err != nil {
		t.Fatalf("OptionVersion after reopen: %v", err)
	}
	if version != 1 {
		t.Fatalf("version after reopen = %d, want 1 (must survive)", version)
	}

	t.Logf("9.6 persistence: cache_ratio=2.5 saved -> DB closed -> reopened -> value survived (memory + DB + version)")
}
