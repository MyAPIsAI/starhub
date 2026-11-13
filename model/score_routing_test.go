package model

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"starhub/setting"
)

func scoredFixture(t *testing.T, count int) ([]*Channel, RouteSnapshot) {
	t.Helper()
	newTestDB(t)
	setting.ResetAllToDefaults()
	t.Cleanup(setting.ResetAllToDefaults)
	setting.ResetResourceProviderModeToDefault()
	t.Cleanup(setting.ResetResourceProviderModeToDefault)
	channels := []*Channel{}
	s := testRouteSnapshot(time.Now().UTC().Truncate(time.Second))
	s.Items = nil
	s.ResourceCount = count
	s.EligibleCount = count
	for i := 0; i < count; i++ {
		ch := &Channel{Name: fmt.Sprintf("channel-%d", i), Models: "test-model", Group: "default", Status: 1, Priority: int64(i), Weight: 1, Key: "upstream"}
		if err := InsertChannel(ch); err != nil {
			t.Fatal(err)
		}
		channels = append(channels, ch)
		raw, _ := json.Marshal(map[string]any{"resource_id": fmt.Sprintf("%d:test-model:t10", ch.ID), "channel_id": fmt.Sprint(ch.ID), "model_id": "test-model", "model_name": "test-model", "price_tier": 10, "quality_level": "S1", "state": "hot", "trend": "stable", "score": 90 - i*10, "dim_scores": map[string]int{"success_rate": 99, "cost": 80, "speed": 90, "health": 99, "stability": 98}, "protection": map[string]any{"schema": 1, "endpoint_generation": ChannelEndpointGeneration(ch), "recovery_epoch": 1, "state_version": 1, "mode": "allowed"}, "pricing": map[string]any{"schema_version": 4, "cost_basis": "group_ratio", "groups": []any{map[string]any{"group": "default", "ratio": "0.5", "base_cost": "1", "actual_cost": "0.5", "status": "valid"}}}})
		s.Items = append(s.Items, raw)
	}
	s.Checksum = s.PolicyChecksum()
	if err := ApplyRoutingSnapshot(s, time.Now()); err != nil {
		t.Fatal(err)
	}
	_ = setting.UpdateResourceProviderMode(setting.OptionKeyResourceProviderMode, "commercial")
	return channels, s
}

func TestScoredRoutingTopThreeWeightsAndRetryFourth(t *testing.T) {
	channels, _ := scoredFixture(t, 4)
	counts := map[int64]int{}
	for i := 0; i < 1500; i++ {
		ch, err := GetSatisfiedChannel("default", "test-model")
		if err != nil {
			t.Fatal(err)
		}
		counts[ch.ID]++
	}
	if counts[channels[3].ID] != 0 {
		t.Fatal("fourth place received fresh traffic")
	}
	for i, want := range []float64{.7, .25, .05} {
		got := float64(counts[channels[i].ID]) / 1500
		if got < want-.05 || got > want+.05 {
			t.Fatalf("rank %d proportion %f", i, got)
		}
	}
	excluded := map[int64]bool{channels[0].ID: true, channels[1].ID: true, channels[2].ID: true}
	ch, err := GetSatisfiedChannelExcluding("default", "test-model", excluded)
	if err != nil || ch.ID != channels[3].ID {
		t.Fatalf("fourth-place retry: %+v %v", ch, err)
	}
	ch, err = GetSatisfiedChannelPreferred("default", "test-model", nil, channels[3].ID)
	if err != nil || ch.ID == channels[3].ID {
		t.Fatalf("affinity bypassed top three: %+v %v", ch, err)
	}
}

func TestDegradedResourceRecoversThroughIsolatedProbes(t *testing.T) {
	_, _ = scoredFixture(t, 2)
	ch, err := GetSatisfiedChannel("default", "test-model")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for i := 0; i < 3; i++ {
		if err := RecordRoutingOutcome(ch.RuntimeRoute, true, 200, 20*time.Second, now); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 8; i++ {
		probes, err := ClaimLocalRoutingProbes(now)
		if err != nil {
			t.Fatal(err)
		}
		if len(probes) == 0 {
			break
		}
		if err := CompleteLocalRoutingProbe(probes[0], true, now, time.Second); err != nil {
			t.Fatal(err)
		}
		now = now.Add(31 * time.Second)
	}
	state, err := ReadLocalRoutingState(ch.RuntimeRoute.Item)
	if err != nil || state.Degraded || state.OpenUntil != 0 {
		t.Fatalf("probe recovery: %+v %v", state, err)
	}
	var count int64
	DB.Model(&Log{}).Count(&count)
	if count != 0 {
		t.Fatal("probe created customer billing")
	}
}

func TestScoredRoutingConstraintsSurviveStaleSnapshot(t *testing.T) {
	channels, s := scoredFixture(t, 4)
	if err := SaveRoutingGroupPolicy(RoutingGroupPolicy{GroupID: "default", MaxResourceRatio: "0.4", MinimumQuality: "S2"}); err != nil {
		t.Fatal(err)
	}
	if _, err := GetSatisfiedChannel("default", "test-model"); err != ErrNoChannel {
		t.Fatalf("price cap ignored: %v", err)
	}
	if err := SaveRoutingGroupPolicy(RoutingGroupPolicy{GroupID: "default", MaxResourceRatio: "1", MinimumQuality: "S0"}); err != nil {
		t.Fatal(err)
	}
	if _, err := GetSatisfiedChannel("default", "test-model"); err != ErrNoChannel {
		t.Fatalf("quality floor ignored: %v", err)
	}
	if _, err := GetScoredChannel("default", "test-model", nil, 0, "", false, nil); err != nil {
		t.Fatalf("nontext applied quality filter: %v", err)
	}
	_ = SaveRoutingGroupPolicy(RoutingGroupPolicy{GroupID: "default", MaxResourceRatio: "1", MinimumQuality: "S2"})
	// Move publication time without bypassing the public version-conflict guard.
	if err := DB.Where("id = 1").Delete(&RoutingSnapshotRecord{}).Error; err != nil {
		t.Fatal(err)
	}
	s.PublishedAt = time.Now().Add(-4 * time.Minute)
	s.SnapshotVersion = "stale"
	s.Checksum = s.PolicyChecksum()
	if err := ApplyRoutingSnapshot(s, time.Now()); err != nil {
		t.Fatal(err)
	}
	ch, err := GetSatisfiedChannel("default", "test-model")
	if err != nil || ch.ID != channels[3].ID {
		t.Fatalf("stale priority fallback: %+v %v", ch, err)
	}
	_ = SaveRoutingGroupPolicy(RoutingGroupPolicy{GroupID: "default", MaxResourceRatio: "0.4", MinimumQuality: "S2"})
	if _, err := GetSatisfiedChannel("default", "test-model"); err != ErrNoChannel {
		t.Fatalf("stale snapshot bypassed price cap: %v", err)
	}
}

func TestLocalCircuitHalfOpenAndLateSuccessFence(t *testing.T) {
	_, _ = scoredFixture(t, 1)
	ch, err := GetSatisfiedChannel("default", "test-model")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for i := 0; i < 5; i++ {
		if err := RecordRoutingOutcome(ch.RuntimeRoute, false, 503, 0, now); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := GetSatisfiedChannel("default", "test-model"); err != ErrNoChannel {
		t.Fatal("open circuit admitted customer traffic")
	}
	if err := RecordRoutingOutcome(ch.RuntimeRoute, true, 200, time.Millisecond, now); err != nil {
		t.Fatal(err)
	}
	state, _ := ReadLocalRoutingState(ch.RuntimeRoute.Item)
	if state.OpenUntil == 0 {
		t.Fatal("late success closed circuit")
	}
	probes, err := ClaimLocalRoutingProbes(now.Add(31 * time.Second))
	if err != nil || len(probes) != 1 {
		t.Fatalf("probe lease missing: %v %d", err, len(probes))
	}
	other, _ := ClaimLocalRoutingProbes(now.Add(32 * time.Second))
	if len(other) != 0 {
		t.Fatal("duplicate half-open lease")
	}
	if err := CompleteLocalRoutingProbe(probes[0], false, now.Add(32*time.Second)); err != nil {
		t.Fatal(err)
	}
	state, _ = ReadLocalRoutingState(ch.RuntimeRoute.Item)
	if state.BackoffSeconds != 60 {
		t.Fatalf("backoff: %+v", state)
	}
	probes, _ = ClaimLocalRoutingProbes(now.Add(93 * time.Second))
	if len(probes) != 1 {
		t.Fatal("retry probe missing")
	}
	if err := CompleteLocalRoutingProbe(probes[0], true, now.Add(94*time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := GetSatisfiedChannel("default", "test-model"); err != nil {
		t.Fatalf("successful probe did not recover: %v", err)
	}
}

func TestEWMADemotionAndRecovery(t *testing.T) {
	channels, _ := scoredFixture(t, 4)
	ch, err := GetSatisfiedChannelPreferred("default", "test-model", nil, channels[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := RecordRoutingOutcome(ch.RuntimeRoute, true, 200, 30*time.Second, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	state, _ := ReadLocalRoutingState(ch.RuntimeRoute.Item)
	if !state.Degraded {
		t.Fatal("slow resource was not demoted")
	}
	for i := 0; i < 20; i++ {
		selected, err := GetSatisfiedChannelPreferred("default", "test-model", nil, ch.ID)
		if err != nil || selected.ID == ch.ID {
			t.Fatalf("demoted resource stayed in active pool: %v", err)
		}
	}
	for i := 0; i < 8; i++ {
		if err := RecordRoutingOutcome(ch.RuntimeRoute, true, 200, time.Second, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	state, _ = ReadLocalRoutingState(ch.RuntimeRoute.Item)
	if state.Degraded {
		t.Fatal("healthy resource did not recover")
	}
}
