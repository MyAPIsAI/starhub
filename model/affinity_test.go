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

	RememberAffinity(key, 42, -1, time.Minute)
	got, ok := AffinityChannel(key)
	if !ok || got != 42 {
		t.Fatalf("AffinityChannel = %d, %v; want 42, true", got, ok)
	}
	if _, ok := AffinityKeyIndex(key); ok {
		t.Fatal("entry with -1 keyIdx should report no key index")
	}
	// A different scope must not see the preference.
	if _, ok := AffinityChannel(AffinityKey{TokenID: 8, Model: "gpt-4o", Group: "default"}); ok {
		t.Fatal("preference leaked to another token")
	}
	if _, ok := AffinityChannel(AffinityKey{TokenID: 7, Model: "gpt-4o", Group: "vip"}); ok {
		t.Fatal("preference leaked to another group")
	}

	// keyIdx is returned when recorded.
	RememberAffinity(key, 42, 1, time.Minute)
	if idx, ok := AffinityKeyIndex(key); !ok || idx != 1 {
		t.Fatalf("AffinityKeyIndex = %d, %v; want 1, true", idx, ok)
	}

	// A tiny TTL expires and the dead entry is pruned on read.
	RememberAffinity(key, 43, -1, time.Millisecond)
	time.Sleep(10 * time.Millisecond)
	if _, ok := AffinityChannel(key); ok {
		t.Fatal("expired preference should miss")
	}
}

func TestAffinityStoreClearSemantics(t *testing.T) {
	ResetAffinity()
	t.Cleanup(ResetAffinity)

	key := AffinityKey{TokenID: 1, Model: "m", Group: "default"}
	RememberAffinity(key, 10, -1, time.Minute)

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
	RememberAffinity(key, 10, -1, time.Minute)
	RememberAffinity(key, 0, -1, time.Minute)
	if _, ok := AffinityChannel(key); ok {
		t.Fatal("zero channel should clear the entry")
	}
	RememberAffinity(key, 10, -1, time.Minute)
	RememberAffinity(key, 10, -1, 0)
	if _, ok := AffinityChannel(key); ok {
		t.Fatal("zero ttl should clear the entry")
	}
}

// Cooldown suppresses a channel for a short window and expires on its own.
func TestAffinityCooldown(t *testing.T) {
	ResetAffinity()
	t.Cleanup(ResetAffinity)

	if ChannelCoolingDown(7) {
		t.Fatal("unknown channel should not be cooling down")
	}
	CoolDownChannel(7, time.Minute)
	if !ChannelCoolingDown(7) {
		t.Fatal("channel should be cooling down")
	}
	if ChannelCoolingDown(8) {
		t.Fatal("cooldown leaked to another channel")
	}
	CoolDownChannel(7, 0)
	if ChannelCoolingDown(7) {
		t.Fatal("zero ttl should clear the cooldown")
	}

	CoolDownChannel(9, time.Millisecond)
	time.Sleep(10 * time.Millisecond)
	if ChannelCoolingDown(9) {
		t.Fatal("expired cooldown should be pruned")
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

// GetSatisfiedChannelHashed picks deterministically per hashKey and stays
// within the highest priority tier.
func TestGetSatisfiedChannelHashed(t *testing.T) {
	newTestDB(t)
	ResetAffinity()
	t.Cleanup(ResetAffinity)

	highA := addChannel(t, "hash-a", "default", "gpt-4o", 10, 5, common.ChannelStatusEnabled)
	highB := addChannel(t, "hash-b", "default", "gpt-4o", 10, 3, common.ChannelStatusEnabled)
	low := addChannel(t, "hash-low", "default", "gpt-4o", 0, 1, common.ChannelStatusEnabled)

	// Deterministic for the same hash key.
	got1, err := GetSatisfiedChannelHashed("default", "gpt-4o", nil, "user-1")
	if err != nil {
		t.Fatalf("hash pick failed: %v", err)
	}
	if got1.ID == low.ID {
		t.Fatal("hash pick must not choose a lower priority channel")
	}
	for i := 0; i < 30; i++ {
		got, err := GetSatisfiedChannelHashed("default", "gpt-4o", nil, "user-1")
		if err != nil || got.ID != got1.ID {
			t.Fatalf("hash pick should be stable: iteration %d got %d, want %d", i, got.ID, got1.ID)
		}
	}

	// Different hash key may pick a different channel.
	got2, err := GetSatisfiedChannelHashed("default", "gpt-4o", nil, "user-2")
	if err != nil {
		t.Fatalf("hash pick failed: %v", err)
	}
	if got2.ID == low.ID {
		t.Fatal("hash pick must not choose a lower priority channel")
	}

	// Excluding the chosen channel must fall back to another in the same tier.
	exclude1 := map[int64]bool{got1.ID: true}
	got3, err := GetSatisfiedChannelHashed("default", "gpt-4o", exclude1, "user-1")
	if err != nil || got3.ID == got1.ID || got3.ID == low.ID {
		t.Fatalf("excluded hash pick = %d, want a different high-tier channel", got3.ID)
	}

	// Excluding the whole top tier degrades to the next.
	excludeAll := map[int64]bool{highA.ID: true, highB.ID: true}
	got4, err := GetSatisfiedChannelHashed("default", "gpt-4o", excludeAll, "user-1")
	if err != nil || got4.ID != low.ID {
		t.Fatalf("exhausted tier: got %d, want low", got4.ID)
	}
}
