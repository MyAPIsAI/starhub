package model

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"starhub/internal/upstream"
)

func TestNormalizeUpstreamNullsMaterializesLegacyColumns(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	if err := db.Exec(`CREATE TABLE upstream_states (id text primary key, rate_limited_until integer, open_until integer)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE upstream_fallback_slots (id text primary key, next_attempt integer, last_attempt integer)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO upstream_states (id, rate_limited_until, open_until) VALUES ('legacy-null', NULL, 100), ('kept', 7, 0)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO upstream_fallback_slots (id, next_attempt, last_attempt) VALUES ('slot-null', 5, NULL)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := normalizeUpstreamNulls(db); err != nil {
		t.Fatal(err)
	}
	var states []struct {
		ID    string
		Until int64
	}
	if err := db.Raw(`SELECT id, rate_limited_until AS until FROM upstream_states`).Scan(&states).Error; err != nil {
		t.Fatal(err)
	}
	until := map[string]int64{}
	for _, row := range states {
		until[row.ID] = row.Until
	}
	if until["legacy-null"] != 0 || until["kept"] != 7 {
		t.Fatalf("legacy NULL was not materialized: %+v", until)
	}
	var slot int64
	if err := db.Raw(`SELECT last_attempt FROM upstream_fallback_slots WHERE id = 'slot-null'`).Scan(&slot).Error; err != nil {
		t.Fatal(err)
	}
	if slot != 0 {
		t.Fatalf("legacy slot remained NULL: %d", slot)
	}
}

func TestRebuildRateLimitDeadlinesRestoresLongCooldown(t *testing.T) {
	newTestDB(t)
	now := time.Now().Unix()
	id := "state-rate-long"
	state := upstream.State{ID: id, Scope: "credential_model", Failures: 5, Version: 3, OpenUntil: now + 540, Reason: "model_unavailable", Updated: now - 60}
	if err := DB.Create(&state).Error; err != nil {
		t.Fatal(err)
	}
	if err := DB.Create(&upstream.Event{ID: "evt-rate-long", ResourceID: id, Kind: "attempt", Class: "rate_limited", OpenUntil: now + 600, Created: now - 60}).Error; err != nil {
		t.Fatal(err)
	}
	if err := DB.Transaction(func(tx *gorm.DB) error { return rebuildRateLimitDeadlines(tx, now) }); err != nil {
		t.Fatal(err)
	}
	var got upstream.State
	if err := DB.First(&got, "id = ?", id).Error; err != nil {
		t.Fatal(err)
	}
	if got.RateLimitedUntil != now+600 {
		t.Fatalf("long Retry-After was not restored: %+v", got)
	}
}

func TestRebuildRateLimitDeadlinesRespectsReset(t *testing.T) {
	newTestDB(t)
	now := time.Now().Unix()
	id := "state-rate-reset"
	state := upstream.State{ID: id, Scope: "credential_model", Failures: 5, Version: 3, OpenUntil: now + 540, Reason: "model_unavailable", Updated: now - 60}
	if err := DB.Create(&state).Error; err != nil {
		t.Fatal(err)
	}
	if err := DB.Create(&upstream.Event{ID: "evt-rate-reset", ResourceID: id, Kind: "attempt", Class: "rate_limited", OpenUntil: now + 600, Created: now - 60}).Error; err != nil {
		t.Fatal(err)
	}
	if err := DB.Create(&upstream.Event{ID: "evt-manual-reset", ResourceID: id, Kind: "reset", Class: "manual_reset", Created: now - 30}).Error; err != nil {
		t.Fatal(err)
	}
	if err := DB.Transaction(func(tx *gorm.DB) error { return rebuildRateLimitDeadlines(tx, now) }); err != nil {
		t.Fatal(err)
	}
	var got upstream.State
	if err := DB.First(&got, "id = ?", id).Error; err != nil {
		t.Fatal(err)
	}
	if got.RateLimitedUntil != 0 {
		t.Fatalf("reset did not invalidate old rate evidence: %+v", got)
	}
}

func TestRateLimitMigrationRunsOnce(t *testing.T) {
	newTestDB(t)
	if err := DB.Where("id = ?", upstreamRateLimitMigration).Delete(&upstreamMigration{}).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now().Unix()
	id := "state-rate-once"
	state := upstream.State{ID: id, Scope: "credential_model", Failures: 5, Version: 2, OpenUntil: now + 540, Reason: "model_unavailable", Updated: now - 60}
	if err := DB.Create(&state).Error; err != nil {
		t.Fatal(err)
	}
	if err := DB.Create(&upstream.Event{ID: "evt-rate-once", ResourceID: id, Kind: "attempt", Class: "rate_limited", OpenUntil: now + 600, Created: now - 60}).Error; err != nil {
		t.Fatal(err)
	}
	if err := migrateRateLimitedUntil(); err != nil {
		t.Fatal(err)
	}
	var got upstream.State
	if err := DB.First(&got, "id = ?", id).Error; err != nil {
		t.Fatal(err)
	}
	if got.RateLimitedUntil != now+600 {
		t.Fatalf("first migration did not restore: %+v", got)
	}
	// A later manual clear must not be replayed by another start.
	if err := DB.Model(&upstream.State{}).Where("id = ?", id).Update("rate_limited_until", 0).Error; err != nil {
		t.Fatal(err)
	}
	if err := migrateRateLimitedUntil(); err != nil {
		t.Fatal(err)
	}
	if err := DB.First(&got, "id = ?", id).Error; err != nil {
		t.Fatal(err)
	}
	if got.RateLimitedUntil != 0 {
		t.Fatalf("replayed migration revived a cleared cooldown: %+v", got)
	}
	var markers int64
	if err := DB.Model(&upstreamMigration{}).Where("id = ?", upstreamRateLimitMigration).Count(&markers).Error; err != nil {
		t.Fatal(err)
	}
	if markers != 1 {
		t.Fatalf("migration marker count = %d", markers)
	}
}
