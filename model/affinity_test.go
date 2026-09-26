package model

import (
	"testing"
	"time"

	"starhub/common"
)

func TestAffinityStoreRememberAndExpire(t *testing.T) {
	ResetAffinity()
	t.Cleanup(ResetAffinity)

	key := AffinityKey{TokenID: 7, Model: "gpt-4o", Group: "default"}
	if _, ok := AffinityChannel(key); ok {
		t.Fatal("empty store should miss")
	}

	RememberAffinity(key, 42, time.Minute)
	got, ok := AffinityChannel(key)
	if !ok || got != 42 {
		t.Fatalf("AffinityChannel = %d, %v; want 42, true", got, ok)
	}
	// A different scope must not see the preference.
	if _, ok := AffinityChannel(AffinityKey{TokenID: 8, Model: "gpt-4o", Group: "default"}); ok {
		t.Fatal("preference leaked to another token")
	}
	if _, ok := AffinityChannel(AffinityKey{TokenID: 7, Model: "gpt-4o", Group: "vip"}); ok {
		t.Fatal("preference leaked to another group")
	}

	// A tiny TTL expires and the dead entry is pruned on read.
	RememberAffinity(key, 43, time.Millisecond)
	time.Sleep(10 * time.Millisecond)
	if _, ok := AffinityChannel(key); ok {
		t.Fatal("expired preference should miss")
	}
}

func TestAffinityStoreClearSemantics(t *testing.T) {
	ResetAffinity()
	t.Cleanup(ResetAffinity)

	key := AffinityKey{TokenID: 1, Model: "m", Group: "default"}
	RememberAffinity(key, 10, time.Minute)

	// ForgetAffinityIf with the wrong channel must leave the entry alone.
	ForgetAffinityIf(key, 99)
	if got, ok := AffinityChannel(key); !ok || got != 10 {
		t.Fatalf("wrong-channel forget removed the entry: %d, %v", got, ok)
	}
	ForgetAffinityIf(key, 10)
	if _, ok := AffinityChannel(key); ok {
		t.Fatal("matching forget should have cleared the entry")
	}

	// Non-positive channel/ttl clear the entry.
	RememberAffinity(key, 10, time.Minute)
	RememberAffinity(key, 0, time.Minute)
	if _, ok := AffinityChannel(key); ok {
		t.Fatal("zero channel should clear the entry")
	}
	RememberAffinity(key, 10, time.Minute)
	RememberAffinity(key, 10, 0)
	if _, ok := AffinityChannel(key); ok {
		t.Fatal("zero ttl should clear the entry")
	}
}

// GetSatisfiedChannelPreferred must honor the preference inside the highest
// available priority tier and fall back to the weighted draw otherwise.
func TestGetSatisfiedChannelPreferred(t *testing.T) {
	newTestDB(t)
	ResetAffinity()
	t.Cleanup(ResetAffinity)

	highA := addChannel(t, "high-a", "default", "gpt-4o", 10, 1, common.ChannelStatusEnabled)
	highB := addChannel(t, "high-b", "default", "gpt-4o", 10, 1, common.ChannelStatusEnabled)
	low := addChannel(t, "low", "default", "gpt-4o", 0, 1, common.ChannelStatusEnabled)

	// Within the top tier the preference wins deterministically.
	for i := 0; i < 50; i++ {
		got, err := GetSatisfiedChannelPreferred("default", "gpt-4o", nil, highA.ID)
		if err != nil {
			t.Fatalf("preferred pick failed: %v", err)
		}
		if got.ID != highA.ID {
			t.Fatalf("pick %d = %d, want preferred %d", i, got.ID, highA.ID)
		}
	}
	got, err := GetSatisfiedChannelPreferred("default", "gpt-4o", nil, highB.ID)
	if err != nil || got.ID != highB.ID {
		t.Fatalf("preferred high-b = %+v, %v; want high-b", got, err)
	}

	// A preference lower than the top tier must NOT bypass priority.
	for i := 0; i < 50; i++ {
		got, err := GetSatisfiedChannelPreferred("default", "gpt-4o", nil, low.ID)
		if err != nil {
			t.Fatalf("low preference pick failed: %v", err)
		}
		if got.ID == low.ID {
			t.Fatal("a low-priority preference bypassed the higher tier")
		}
	}

	// An excluded preference is ignored: excluding the whole top tier must
	// degrade to the next layer instead of returning the excluded channel.
	got, err = GetSatisfiedChannelPreferred("default", "gpt-4o",
		map[int64]bool{highA.ID: true, highB.ID: true}, highA.ID)
	if err != nil || got.ID != low.ID {
		t.Fatalf("excluded preference: got %+v, %v; want low", got, err)
	}

	// A disabled preference is ignored too.
	if err := UpdateChannelStatus(highA.ID, common.ChannelStatusManuallyDisabled); err != nil {
		t.Fatalf("disable high-a failed: %v", err)
	}
	got, err = GetSatisfiedChannelPreferred("default", "gpt-4o", nil, highA.ID)
	if err != nil || got.ID != highB.ID {
		t.Fatalf("disabled preference: got %+v, %v; want high-b", got, err)
	}

	// Zero preferred is exactly the plain excluding selector.
	got, err = GetSatisfiedChannelPreferred("default", "gpt-4o", nil, 0)
	if err != nil || got.ID != highB.ID {
		t.Fatalf("zero preference: got %+v, %v; want high-b", got, err)
	}
}
