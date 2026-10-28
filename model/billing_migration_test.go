package model

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"starhub/setting"
)

func TestLoadLegacyRatioOptionsPersistsEffectivePrices(t *testing.T) {
	newTestDB(t)
	setting.ResetRatiosToDefaults()
	t.Cleanup(setting.ResetRatiosToDefaults)
	for _, key := range []string{setting.OptionKeyModelRatio, setting.OptionKeyCompletionRatio} {
		if err := UpsertOption(key, `{"gpt-4o":7,"claude-sonnet-4-6":0}`); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 2; i++ {
		if err := LoadOptionsToSettings(); err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{setting.OptionKeyModelRatio, setting.OptionKeyCompletionRatio} {
			raw, found, err := GetOption(key)
			if err != nil || !found {
				t.Fatalf("option %s missing: %v", key, err)
			}
			var ratios map[string]float64
			if err := json.Unmarshal([]byte(raw), &ratios); err != nil {
				t.Fatal(err)
			}
			if ratios["gpt-4o"] != 7 || ratios["claude-sonnet-4-6"] != 0 {
				t.Fatalf("operator prices changed: %s", raw)
			}
			if _, ok := ratios["claude-haiku-4-5-20251001"]; !ok {
				t.Fatalf("new model missing from persisted option: %s", raw)
			}
			current, err := setting.SnapshotRatio(key)
			if err != nil || current != raw {
				t.Fatalf("stored and effective prices disagree: %s / %s / %v", raw, current, err)
			}
		}
	}
}

func TestLogMigrationPreservesHistoricalCharges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = CloseDB() })
	entry := &Log{Quota: 123}
	if err := InsertLog(entry); err != nil {
		t.Fatal(err)
	}
	if err := DB.Migrator().DropColumn(&Log{}, "UnfundedQuota"); err != nil {
		t.Fatal(err)
	}
	if err := CloseDB(); err != nil {
		t.Fatal(err)
	}
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	var got Log
	if err := DB.First(&got, entry.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.Quota != 123 || got.UnfundedQuota != 0 {
		t.Fatalf("migration changed historical charge: %+v", got)
	}
}

func TestLogRequestedModelMigrationPreservesHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy-model.db")
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = CloseDB() })
	entry := &Log{Model: "gpt-4o", Quota: 123}
	if err := InsertLog(entry); err != nil {
		t.Fatal(err)
	}
	if err := DB.Migrator().DropColumn(&Log{}, "RequestedModel"); err != nil {
		t.Fatal(err)
	}
	if err := CloseDB(); err != nil {
		t.Fatal(err)
	}
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	var got Log
	if err := DB.First(&got, entry.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.Model != "gpt-4o" || got.RequestedModel != "" || got.Quota != 123 {
		t.Fatalf("migration changed historical attribution or charge: %+v", got)
	}
}

func TestTaskRequestedModelMigrationPreservesHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy-task-model.db")
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = CloseDB() })
	entry := &Task{TaskID: "legacy-video", Model: "sora-2", Quota: 80000, RequestMeta: `{"model":"sora-2","seconds":8,"n":2}`, PricingMeta: `{"estimate":{"units":16}}`}
	if err := InsertTask(entry); err != nil {
		t.Fatal(err)
	}
	if err := DB.Migrator().DropColumn(&Task{}, "RequestedModel"); err != nil {
		t.Fatal(err)
	}
	if err := CloseDB(); err != nil {
		t.Fatal(err)
	}
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	got, err := GetTaskByID(entry.ID)
	if err != nil {
		t.Fatal(err)
	}
	if *got != *entry {
		t.Fatalf("migration changed historical task: got %+v, want %+v", got, entry)
	}
}
