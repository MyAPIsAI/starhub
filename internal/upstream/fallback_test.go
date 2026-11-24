package upstream

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
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
	past := time.Now().Unix() - int64(p.FallbackInterval) - 1
	if err := db.Model(&FallbackSlot{}).Where("id = ?", id.key("credential")).Updates(map[string]any{"next_attempt": past, "last_attempt": past}).Error; err != nil {
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
	now := time.Now().Unix()
	s := State{ID: id.key("credential_model"), Identity: id, Scope: "credential_model", Version: 1, OpenUntil: now + 60, RateLimitedUntil: now + 60, Reason: "rate_limited"}
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

func TestRateLimitedSurvivesLaterFailure(t *testing.T) {
	db := fallbackDB(t)
	p := DefaultPolicy()
	id := Identity{ChannelID: 1, Model: "model", Credential: "key", Generation: "endpoint", Capability: "chat"}
	first, err := Capture(db, id, p)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Capture(db, id, p)
	if err != nil {
		t.Fatal(err)
	}
	if err := Record(db, first, HTTP(429, nil, "600", p), Event{Sent: true, Status: 429}, p); err != nil {
		t.Fatal(err)
	}
	if _, err := CaptureFallback(db, id, p); !errors.Is(err, ErrBlocked) {
		t.Fatalf("initial cooldown admit: %v", err)
	}
	// A later, shorter generic failure overwrites Reason/OpenUntil and must not
	// shorten the independent rate-limit deadline.
	if err := Record(db, second, HTTP(404, []byte(`{"error":{"code":"model_not_found"}}`), "", p), Event{Sent: true, Status: 404}, p); err != nil {
		t.Fatal(err)
	}
	var state State
	if err := db.First(&state, "id = ?", id.key("credential_model")).Error; err != nil {
		t.Fatal(err)
	}
	if state.RateLimitedUntil <= time.Now().Unix() {
		t.Fatalf("rate deadline was shortened: %+v", state)
	}
	if _, err := CaptureFallback(db, id, p); !errors.Is(err, ErrBlocked) {
		t.Fatalf("overwritten reason bypassed cooldown: %v", err)
	}
}

func TestFallbackFastPathSkipsWriterForHealthyTraffic(t *testing.T) {
	db := fallbackDB(t)
	p := DefaultPolicy()
	id := Identity{ChannelID: 1, Model: "model", Credential: "key", Generation: "endpoint"}
	attempt, err := CaptureFallback(db, id, p)
	if err != nil {
		t.Fatal(err)
	}
	if len(attempt.Protected) != 0 {
		t.Fatalf("healthy fallback reported protection: %+v", attempt.Protected)
	}
	var count int64
	if err := db.Model(&FallbackSlot{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("healthy fallback still claimed a slot row: %d", count)
	}
}

func TestFallbackIntervalIncreaseExtendsExistingDeadline(t *testing.T) {
	db := fallbackDB(t)
	p := DefaultPolicy()
	id := Identity{ChannelID: 1, Model: "model", Credential: "key"}
	now := time.Now().Unix()
	slot := FallbackSlot{ID: id.key("credential"), NextAttempt: now + 5, LastAttempt: now}
	if err := db.Create(&slot).Error; err != nil {
		t.Fatal(err)
	}
	p.FallbackInterval = 120
	if _, err := CaptureFallback(db, id, p, true); !errors.Is(err, ErrBlocked) {
		t.Fatalf("raised interval did not extend deadline: %v", err)
	}
	p.FallbackInterval = 30
	if err := db.Model(&FallbackSlot{}).Where("id = ?", slot.ID).Updates(map[string]any{"next_attempt": now - 1000, "last_attempt": now - 1000}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := CaptureFallback(db, id, p, true); err != nil {
		t.Fatalf("expired slot rejected: %v", err)
	}
}

// applyReviewCommit mirrors the relay session update: only committed versions
// are followed, and a stale scope stops following for the session lifetime.
func applyReviewCommit(session *Attempt, invalid map[string]bool, committed Committed) {
	for id := range committed.Stale {
		invalid[id] = true
		delete(session.Versions, id)
		delete(session.Failures, id)
	}
	for id, version := range committed.Versions {
		if !invalid[id] {
			session.Versions[id] = version
		}
	}
	for id, failures := range committed.Failures {
		if !invalid[id] {
			session.Failures[id] = failures
		}
	}
}

func TestRealtimeStyleSessionClearsIntermittentFailures(t *testing.T) {
	db := fallbackDB(t)
	p := DefaultPolicy()
	id := Identity{ChannelID: 1, Model: "model", Credential: "key", Generation: "endpoint", Capability: "realtime"}
	captured, err := Capture(db, id, p)
	if err != nil {
		t.Fatal(err)
	}
	session := *captured
	session.Versions = cloneMapUint64(captured.Versions)
	session.Failures = cloneMapInt(captured.Failures)
	invalid := map[string]bool{}
	failure := Verdict{Class: "upstream_failure", Scope: "interface"}
	for i := 0; i < p.Failures+1; i++ {
		for _, verdict := range []Verdict{failure, {Class: "success"}} {
			attempt := session
			attempt.Versions = cloneMapUint64(session.Versions)
			attempt.Failures = cloneMapInt(session.Failures)
			attempt.ID = uuid.NewString()
			committed, err := RecordCommitted(db, &attempt, verdict, Event{Sent: true, Status: 200}, p)
			if err != nil {
				t.Fatal(err)
			}
			applyReviewCommit(&session, invalid, committed)
		}
	}
	var state State
	if err := db.First(&state, "id = ?", id.key("interface")).Error; err != nil {
		t.Fatal(err)
	}
	if state.Failures != 0 || state.OpenUntil != 0 {
		t.Fatalf("intermittent failures accumulated: %+v", state)
	}
}

func TestRealtimeStyleSessionKeepsExternalFailure(t *testing.T) {
	db := fallbackDB(t)
	p := DefaultPolicy()
	id := Identity{ChannelID: 1, Model: "model", Credential: "key", Generation: "endpoint", Capability: "realtime"}
	captured, err := Capture(db, id, p)
	if err != nil {
		t.Fatal(err)
	}
	session := *captured
	session.Versions = cloneMapUint64(captured.Versions)
	session.Failures = cloneMapInt(captured.Failures)
	invalid := map[string]bool{}
	failure := Verdict{Class: "upstream_failure", Scope: "interface"}
	// The session records one failure.
	first := session
	first.Versions = cloneMapUint64(session.Versions)
	first.Failures = cloneMapInt(session.Failures)
	first.ID = uuid.NewString()
	committed, err := RecordCommitted(db, &first, failure, Event{Sent: true, Status: 500}, p)
	if err != nil {
		t.Fatal(err)
	}
	applyReviewCommit(&session, invalid, committed)
	// An external request records another failure on the same scope.
	external, err := Capture(db, id, p)
	if err != nil {
		t.Fatal(err)
	}
	external.ID = uuid.NewString()
	if _, err := RecordCommitted(db, external, failure, Event{Sent: true, Status: 500}, p); err != nil {
		t.Fatal(err)
	}
	// The session's later success must not clear the external failure.
	success := session
	success.Versions = cloneMapUint64(session.Versions)
	success.Failures = cloneMapInt(session.Failures)
	success.ID = uuid.NewString()
	committed, err = RecordCommitted(db, &success, Verdict{Class: "success"}, Event{Sent: true, Status: 200}, p)
	if err != nil {
		t.Fatal(err)
	}
	var state State
	if err := db.First(&state, "id = ?", id.key("interface")).Error; err != nil {
		t.Fatal(err)
	}
	if state.Failures != 2 {
		t.Fatalf("external failure was cleared: %+v committed=%+v", state, committed)
	}
}

func TestRealtimeInitialAbsentRejectsExternalVersion(t *testing.T) {
	db := fallbackDB(t)
	p := DefaultPolicy()
	id := Identity{ChannelID: 1, Model: "model", Credential: "key", Generation: "endpoint", Capability: "realtime"}
	failure := Verdict{Class: "upstream_failure", Scope: "interface"}
	// The session connects while no state row exists: every scope is an explicit
	// version-0 snapshot.
	sessionCapture, err := Capture(db, id, p)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := sessionCapture.Versions[id.key("interface")]; !ok {
		t.Fatal("absent scope was not captured explicitly")
	}
	// An external request creates version 1 before the session records anything.
	external, err := Capture(db, id, p)
	if err != nil {
		t.Fatal(err)
	}
	external.ID = "external"
	if _, err := RecordCommitted(db, external, failure, Event{Sent: true, Status: 500}, p); err != nil {
		t.Fatal(err)
	}
	// The session's own failure must not adopt the external version.
	own := *sessionCapture
	own.Versions = cloneMapUint64(sessionCapture.Versions)
	own.Failures = cloneMapInt(sessionCapture.Failures)
	own.ID = "own"
	committed, err := RecordCommitted(db, &own, failure, Event{Sent: true, Status: 500}, p)
	if err != nil {
		t.Fatal(err)
	}
	if !committed.Stale[id.key("interface")] {
		t.Fatalf("initial-absent session followed an external version: %+v", committed)
	}
	// A later session success cannot clear the external failure.
	success := *sessionCapture
	success.Versions = cloneMapUint64(sessionCapture.Versions)
	success.Failures = cloneMapInt(sessionCapture.Failures)
	success.ID = "ok"
	if _, err := RecordCommitted(db, &success, Verdict{Class: "success"}, Event{Sent: true, Status: 200}, p); err != nil {
		t.Fatal(err)
	}
	var state State
	if err := db.First(&state, "id = ?", id.key("interface")).Error; err != nil {
		t.Fatal(err)
	}
	if state.Failures != 2 {
		t.Fatalf("external failure was cleared: %+v", state)
	}
}

func cloneMapUint64(source map[string]uint64) map[string]uint64 {
	target := make(map[string]uint64, len(source))
	for key, value := range source {
		target[key] = value
	}
	return target
}

func cloneMapInt(source map[string]int) map[string]int {
	target := make(map[string]int, len(source))
	for key, value := range source {
		target[key] = value
	}
	return target
}

func TestRecordCommittedDuplicateEventDoesNotAdvance(t *testing.T) {
	db := fallbackDB(t)
	p := DefaultPolicy()
	id := Identity{ChannelID: 1, Model: "model", Credential: "key", Generation: "endpoint", Capability: "chat"}
	captured, err := Capture(db, id, p)
	if err != nil {
		t.Fatal(err)
	}
	failure := Verdict{Class: "upstream_failure", Scope: "interface"}
	first := *captured
	first.ID = "same"
	committed, err := RecordCommitted(db, &first, failure, Event{Sent: true, Status: 500}, p)
	if err != nil || committed.Versions[id.key("interface")] == 0 {
		t.Fatalf("first record: %+v %v", committed, err)
	}
	second := *captured
	second.ID = "same"
	committed, err = RecordCommitted(db, &second, failure, Event{Sent: true, Status: 500}, p)
	if err != nil {
		t.Fatal(err)
	}
	if len(committed.Versions) != 0 || len(committed.Failures) != 0 || len(committed.Stale) != 0 {
		t.Fatalf("duplicate event advanced the snapshot: %+v", committed)
	}
}

func TestRecordCommittedFollowsOnlyOwnVersion(t *testing.T) {
	db := fallbackDB(t)
	p := DefaultPolicy()
	id := Identity{ChannelID: 1, Model: "model", Credential: "key", Generation: "endpoint", Capability: "chat"}
	captured, err := Capture(db, id, p)
	if err != nil {
		t.Fatal(err)
	}
	failure := Verdict{Class: "upstream_failure", Scope: "interface"}
	first := *captured
	first.ID = "first"
	committed, err := RecordCommitted(db, &first, failure, Event{Sent: true, Status: 500}, p)
	if err != nil {
		t.Fatal(err)
	}
	if committed.Stale[id.key("interface")] || committed.Versions[id.key("interface")] == 0 {
		t.Fatalf("own first failure should be followed: %+v", committed)
	}
	// An external failure bumps the version behind the session snapshot.
	external := *captured
	external.ID = "external"
	if _, err := RecordCommitted(db, &external, failure, Event{Sent: true, Status: 500}, p); err != nil {
		t.Fatal(err)
	}
	session, err := Capture(db, id, p)
	if err != nil {
		t.Fatal(err)
	}
	// Move the session snapshot backwards to simulate the pre-external capture.
	session.Versions[id.key("interface")] = committed.Versions[id.key("interface")]
	session.Failures[id.key("interface")] = 1
	next := *session
	next.ID = "next"
	committed, err = RecordCommitted(db, &next, failure, Event{Sent: true, Status: 500}, p)
	if err != nil {
		t.Fatal(err)
	}
	if !committed.Stale[id.key("interface")] {
		t.Fatalf("external change was absorbed into session: %+v", committed)
	}
}
