package model

import (
	"errors"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"starhub/internal/upstream"
)

func rateLimitRepairDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	conn.SetMaxOpenConns(1)
	previous := DB
	DB = db
	t.Cleanup(func() {
		DB = previous
		_ = conn.Close()
	})
	if err := db.AutoMigrate(&upstream.State{}, &upstream.Event{}, &upstream.Guard{}, &upstream.ProbeSlot{}, &upstream.FallbackSlot{}, &upstreamMigration{}); err != nil {
		t.Fatal(err)
	}
	if err := upstream.InitGuard(db); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestRateLimitRepairRecoveryOrderAndLegacyValues(t *testing.T) {
	rate := func(id string, created, deadline int64) upstream.Event {
		return upstream.Event{ID: id, Kind: "attempt", Class: "rate_limited", Created: created, OpenUntil: deadline}
	}
	reset := func(created int64) upstream.Event {
		return upstream.Event{ID: "reset", Kind: "reset", Class: "manual_reset", Created: created}
	}
	for _, tc := range []struct {
		name                string
		reason              string
		open, rate, updated int64
		events              []upstream.Event
		want                int64
	}{
		{"rate_after_reset", "model_unavailable", 840, 0, -20, []upstream.Event{rate("old", -120, 3600), reset(-90), rate("new", -60, 540)}, 540},
		{"nonzero_rate_after_reset", "model_unavailable", 840, 3600, -20, []upstream.Event{rate("old", -120, 3600), reset(-90), rate("new", -60, 540)}, 540},
		{"current_rate_after_reset", "rate_limited", 540, 3600, -20, []upstream.Event{rate("old", -120, 3600), reset(-90)}, 540},
		{"same_second_reset_then_rate", "rate_limited", 590, 0, -10, []upstream.Event{reset(-10), rate("new", -10, 590)}, 590},
		{"same_second_rate_then_reset", "manual_reset", 0, 500, -10, []upstream.Event{rate("old", -10, 590), reset(-10)}, 0},
		{"same_second_overwritten_reason", "model_unavailable", 840, 0, -10, []upstream.Event{reset(-10), rate("new", -10, 590)}, 590},
		{"expired_wrong_nonzero", "model_unavailable", 840, -30, -60, []upstream.Event{rate("old", -60, 540)}, 540},
		{"active_wrong_nonzero", "model_unavailable", 840, 20, -60, []upstream.Event{rate("old", -60, 540)}, 540},
		{"resurrected_reset", "manual_reset", 0, 500, -30, []upstream.Event{rate("old", -60, 540), reset(-30)}, 0},
		{"resurrected_probe_success", "recovered", 0, 500, -30, []upstream.Event{rate("old", -60, 540), {ID: "probe", Kind: "probe", Class: "success", Created: -30}}, 0},
		{"resurrected_fallback_success", "fallback_recovered", 0, 500, -30, []upstream.Event{rate("old", -60, 540), {ID: "fallback", Kind: "recovery", Class: "fallback_recovered", Created: -30}}, 0},
		{"reset_after_state", "model_unavailable", 840, 500, -60, []upstream.Event{rate("old", -60, 540), reset(-30)}, 0},
		{"skipped_recovery", "model_unavailable", 840, 0, -20, []upstream.Event{rate("old", -60, 540), {ID: "skip", Kind: "recovery", Class: "fallback_recovery_skipped", Created: -30}}, 540},
		{"probe_rate_evidence", "model_unavailable", 840, 0, -20, []upstream.Event{{ID: "probe-rate", Kind: "probe", Class: "failure", Decision: "rate_limited", Created: -60, OpenUntil: 540}}, 540},
		{"existing_without_legacy_evidence", "model_unavailable", 840, 250, -20, nil, 250},
		{"closed_state_not_rebuilt", "manual_reset", 0, 0, -30, []upstream.Event{rate("old", -60, 540)}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := rateLimitRepairDB(t)
			now := time.Now().Unix()
			identity := upstream.Identity{ChannelID: 1, Model: "review", Credential: "key", Generation: "endpoint", Capability: "chat"}
			offset := func(value int64) int64 {
				if value == 0 {
					return 0
				}
				return now + value
			}
			state := upstream.State{ID: identity.StateID("credential_model"), Identity: identity, Scope: "credential_model", Version: 7, Failures: 2, Reason: tc.reason, OpenUntil: offset(tc.open), RateLimitedUntil: offset(tc.rate), Updated: now + tc.updated}
			if err := db.Create(&state).Error; err != nil {
				t.Fatal(err)
			}
			for _, event := range tc.events {
				event.ResourceID = state.ID
				event.Created += now
				event.OpenUntil = offset(event.OpenUntil)
				if err := db.Create(&event).Error; err != nil {
					t.Fatal(err)
				}
			}
			if err := db.Transaction(func(tx *gorm.DB) error { return rebuildRateLimitDeadlines(tx, now) }); err != nil {
				t.Fatal(err)
			}
			if err := db.First(&state, "id = ?", state.ID).Error; err != nil {
				t.Fatal(err)
			}
			if state.RateLimitedUntil != offset(tc.want) {
				t.Fatalf("deadline=%d, want=%d", state.RateLimitedUntil, offset(tc.want))
			}
			_, err := upstream.CaptureFallback(db, identity, upstream.DefaultPolicy())
			if tc.want > 0 && !errors.Is(err, upstream.ErrBlocked) || tc.want == 0 && err != nil {
				t.Fatalf("fallback admission: %v", err)
			}
		})
	}
}

func TestRateLimitV3RepairIsAtomicAndSupersedesV2(t *testing.T) {
	db := rateLimitRepairDB(t)
	now := time.Now().Unix()
	if err := db.Create(&upstreamMigration{ID: "upstream_rate_limit_deadline_v2", Applied: now - 30}).Error; err != nil {
		t.Fatal(err)
	}
	state := upstream.State{ID: "repair", Scope: "credential_model", Version: 7, Reason: "model_unavailable", OpenUntil: now + 840, RateLimitedUntil: now - 30, Updated: now - 60, ProbeID: "old-lease", LeaseUntil: now + 20}
	if err := db.Create(&state).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&upstream.ProbeSlot{ID: state.ProbeID, LeaseUntil: state.LeaseUntil}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&upstream.Event{ID: "rate", ResourceID: state.ID, Kind: "attempt", Class: "rate_limited", Created: now - 60, OpenUntil: now + 540}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TRIGGER reject_repair_marker BEFORE INSERT ON upstream_migrations WHEN NEW.id = 'upstream_rate_limit_deadline_v3' BEGIN SELECT RAISE(ABORT, 'marker failure'); END`).Error; err != nil {
		t.Fatal(err)
	}
	if err := migrateRateLimitedUntil(); err == nil {
		t.Fatal("expected marker failure")
	}
	var got upstream.State
	if err := db.First(&got, "id = ?", state.ID).Error; err != nil {
		t.Fatal(err)
	}
	var leases, markers int64
	if err := db.Model(&upstream.ProbeSlot{}).Count(&leases).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&upstreamMigration{}).Where("id = ?", upstreamRateLimitMigration).Count(&markers).Error; err != nil {
		t.Fatal(err)
	}
	if got.Version != state.Version || got.RateLimitedUntil != state.RateLimitedUntil || got.ProbeID != state.ProbeID || leases != 1 || markers != 0 {
		t.Fatalf("repair escaped rollback: state=%+v leases=%d markers=%d", got, leases, markers)
	}
	if err := db.Exec("DROP TRIGGER reject_repair_marker").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TRIGGER ignore_repair_update BEFORE UPDATE ON upstream_states BEGIN SELECT RAISE(IGNORE); END`).Error; err != nil {
		t.Fatal(err)
	}
	if err := migrateRateLimitedUntil(); err == nil {
		t.Fatal("expected rejected state update to prevent completion")
	}
	if err := db.Model(&upstreamMigration{}).Where("id = ?", upstreamRateLimitMigration).Count(&markers).Error; err != nil || markers != 0 {
		t.Fatalf("incomplete repair was marked complete: markers=%d err=%v", markers, err)
	}
	if err := db.Exec("DROP TRIGGER ignore_repair_update").Error; err != nil {
		t.Fatal(err)
	}
	if err := migrateRateLimitedUntil(); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&got, "id = ?", state.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.RateLimitedUntil != now+540 || got.Version != state.Version+1 || got.ProbeID != "" || got.LeaseUntil != 0 {
		t.Fatalf("v3 did not repair v2 data: %+v", got)
	}
	if err := db.Model(&upstream.ProbeSlot{}).Count(&leases).Error; err != nil || leases != 0 {
		t.Fatalf("stale lease retained: count=%d err=%v", leases, err)
	}
	if err := upstream.Reset(db, state.ID, got.Version, 1); err != nil {
		t.Fatal(err)
	}
	if err := migrateRateLimitedUntil(); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&got, "id = ?", state.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.RateLimitedUntil != 0 {
		t.Fatalf("v3 replayed after reset: %+v", got)
	}
}
