package relay

import (
	"testing"

	"starhub/internal/upstream"
)

func TestRealtimeSessionFollowsOnlyCommittedChanges(t *testing.T) {
	session := newRealtimeProtectionSession(&upstream.Attempt{
		ID: "start",
		Identity: upstream.Identity{
			ChannelID: 1, Model: "m", Credential: "k", Generation: "g", Capability: "realtime",
		},
		Versions: map[string]uint64{"iface": 1},
		Failures: map[string]int{"iface": 0},
	})
	session.apply(upstream.Committed{
		Versions: map[string]uint64{"iface": 2},
		Failures: map[string]int{"iface": 1},
	})
	if session.attempt.Versions["iface"] != 2 || session.attempt.Failures["iface"] != 1 {
		t.Fatalf("own change not followed: %+v", session.attempt)
	}
	// A stale scope stops following, so a later success cannot erase it.
	session.apply(upstream.Committed{Stale: map[string]bool{"iface": true}})
	session.apply(upstream.Committed{Versions: map[string]uint64{"iface": 9}, Failures: map[string]int{"iface": 0}})
	if _, ok := session.attempt.Versions["iface"]; ok {
		t.Fatalf("invalid scope resumed: %+v", session.attempt)
	}
	if !session.invalid["iface"] {
		t.Fatal("stale scope was not invalidated")
	}
}

func TestRealtimeSessionSnapshotIsIsolated(t *testing.T) {
	session := newRealtimeProtectionSession(&upstream.Attempt{
		Versions:  map[string]uint64{"iface": 3},
		Failures:  map[string]int{"iface": 2},
		Protected: map[string]uint64{"iface": 3},
	})
	snapshot := session.snapshot()
	snapshot.Versions["iface"] = 99
	snapshot.Failures["iface"] = 99
	if session.attempt.Versions["iface"] != 3 || session.attempt.Failures["iface"] != 2 {
		t.Fatalf("snapshot mutated session: %+v", session.attempt)
	}
}
