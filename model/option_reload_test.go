package model

import (
	"testing"

	"starhub/setting"
)

func TestOptionRevisionReloadAndRollback(t *testing.T) {
	setting.ResetAllToDefaults()
	defer setting.ResetAllToDefaults()
	newTestDB(t)
	if err := SeedOptions(); err != nil {
		t.Fatal(err)
	}
	if err := LoadOptionsToSettings(); err != nil {
		t.Fatal(err)
	}
	version, err := SaveOption(setting.OptionKeyCacheRatio, "2.5", 42)
	if err != nil || version != 1 {
		t.Fatalf("save version = %d, %v", version, err)
	}
	if got := setting.CacheRatioValue(); got != setting.FallbackCacheRatio {
		t.Fatalf("memory changed before reload: %v", got)
	}
	if changed, err := RefreshOptionsOnce(); err != nil || !changed {
		t.Fatalf("reload = %t, %v", changed, err)
	}
	if got := setting.CacheRatioValue(); got != 2.5 {
		t.Fatalf("cache ratio after reload = %v", got)
	}
	changes, err := ListOptionChanges(10)
	if err != nil || len(changes) != 1 || changes[0].ActorID != 42 {
		t.Fatalf("history = %+v, %v", changes, err)
	}
	if _, err := RollbackOption(1, 0, 42); err == nil {
		t.Fatal("stale rollback should be rejected")
	}
	version, err = RollbackOption(1, 1, 43)
	if err != nil || version != 2 {
		t.Fatalf("rollback version = %d, %v", version, err)
	}
	if _, err := RefreshOptionsOnce(); err != nil {
		t.Fatal(err)
	}
	if got := setting.CacheRatioValue(); got != setting.FallbackCacheRatio {
		t.Fatalf("cache ratio after rollback = %v", got)
	}
}
