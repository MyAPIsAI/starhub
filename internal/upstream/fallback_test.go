package upstream

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func fallbackDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { sqlDB.Close() })
	if err := db.AutoMigrate(&State{}, &Event{}, &Guard{}, &ProbeSlot{}, &FallbackSlot{}); err != nil {
		t.Fatal(err)
	}
	if err := InitGuard(db); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestFallbackSuccessResetsOnlyCapturedFailures(t *testing.T) {
	for _, concurrent := range []bool{false, true} {
		t.Run(map[bool]string{false: "unchanged", true: "newer_failure"}[concurrent], func(t *testing.T) {
			db := fallbackDB(t)
			p := DefaultPolicy()
			id := Identity{ChannelID: 1, Model: "model", Credential: "key", Generation: "endpoint", Capability: "chat"}
			s := State{ID: id.key("interface"), Identity: id, Scope: "interface", Failures: 2, Version: 2}
			if err := db.Create(&s).Error; err != nil {
				t.Fatal(err)
			}
			a, err := CaptureFallback(db, id, p)
			if err != nil {
				t.Fatal(err)
			}
			a.RoutingStage = "ordinary_fallback"
			if concurrent {
				if err := db.Model(&s).Updates(map[string]any{"version": 3, "failures": 3}).Error; err != nil {
					t.Fatal(err)
				}
			}
			if err := Record(db, a, Verdict{Class: "success"}, Event{Sent: true, Status: 200}, p); err != nil {
				t.Fatal(err)
			}
			if err := db.First(&s, "id = ?", s.ID).Error; err != nil {
				t.Fatal(err)
			}
			want := 0
			if concurrent {
				want = 3
			}
			if s.Failures != want {
				t.Fatalf("failures=%d, want %d", s.Failures, want)
			}
		})
	}
}

func TestProtectedFallbackAdmissionBudget(t *testing.T) {
	db := fallbackDB(t)
	p := DefaultPolicy()
	id := Identity{ChannelID: 1, Model: "model", Credential: "key", Generation: "endpoint"}
	var admitted atomic.Int32
	var workers sync.WaitGroup
	for range 8 {
		workers.Go(func() {
			_, err := CaptureFallback(db, id, p, true)
			if err == nil {
				admitted.Add(1)
			} else if !errors.Is(err, ErrBlocked) {
				t.Error(err)
			}
		})
	}
	workers.Wait()
	if admitted.Load() != 1 {
		t.Fatalf("admitted %d concurrent calls", admitted.Load())
	}
	other := id
	other.Model = "another-model"
	if _, err := CaptureFallback(db, other, p, true); !errors.Is(err, ErrBlocked) {
		t.Fatalf("shared credential bypass: %v", err)
	}
	if err := db.Model(&FallbackSlot{}).Where("id = ?", id.key("credential")).Update("next_attempt", time.Now().Unix()-1).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := CaptureFallback(db, id, p, true); err != nil {
		t.Fatal(err)
	}
}

func TestFallbackHonorsRateCooldownAndSwitch(t *testing.T) {
	db := fallbackDB(t)
	p := DefaultPolicy()
	id := Identity{ChannelID: 1, Model: "model", Credential: "key"}
	s := State{ID: id.key("credential_model"), Identity: id, Scope: "credential_model", Version: 1, OpenUntil: time.Now().Unix() + 60, Reason: "rate_limited"}
	if err := db.Create(&s).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := CaptureFallback(db, id, p); !errors.Is(err, ErrBlocked) {
		t.Fatalf("ignored cooldown: %v", err)
	}
	p.OrdinaryFallback = false
	if _, err := CaptureFallback(db, Identity{ChannelID: 2}, p); !errors.Is(err, ErrBlocked) {
		t.Fatalf("ignored switch: %v", err)
	}
	if _, err := NormalizePolicy(`{"fallback_interval_seconds":0}`); err == nil {
		t.Fatal("accepted zero interval")
	}
}
