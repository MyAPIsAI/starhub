package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"starhub/setting"
)

func timePricingRules(label string) string {
	return fmt.Sprintf(`{"gpt-4o":{"timezone":"UTC","windows":[{"id":"all","label":%q,"days":[0,1,2,3,4,5,6],"start":"00:00","end":"24:00","multiplier":2}]}}`, label)
}

func TestTimePricingStorageGuardsRollbackHistoryAndRestart(t *testing.T) {
	path := groupConfigurationDB(t)
	ctx := context.Background()
	before := groupConfigurationSnapshot(t)
	state, err := ReadTimePricing(ctx)
	if err != nil || state.Version != 0 || string(state.Rules) != "{}" {
		t.Fatalf("initial time rules: %+v %v", state, err)
	}
	for _, raw := range []string{"", `null`, `{"m":{"timezone":"UTC","windows":null}}`, strings.Repeat(" ", 1<<20+1)} {
		if _, err := SaveTimePricingIfVersion(ctx, raw, 77, 0); !errors.Is(err, ErrInvalidTimePricing) {
			t.Fatal("invalid time rules accepted")
		}
	}
	for _, version := range []int64{-1, maxPricingOptionVersion, maxPricingOptionVersion + 1} {
		if _, err := SaveTimePricingIfVersion(ctx, "{}", 77, version); !errors.Is(err, ErrInvalidTimePricing) {
			t.Fatal("invalid version accepted")
		}
	}
	if groupConfigurationSnapshot(t) != before {
		t.Fatal("invalid drafts changed storage")
	}
	if err := DB.Exec(`CREATE TRIGGER reject_time_history BEFORE INSERT ON option_changes WHEN NEW.key = 'model_time_pricing' BEGIN SELECT RAISE(ABORT, 'synthetic history failure'); END`).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := SaveTimePricingIfVersion(ctx, timePricingRules("v1"), 77, 0); err == nil {
		t.Fatal("history failure did not abort")
	}
	if groupConfigurationSnapshot(t) != before {
		t.Fatal("failed time history leaked option or version")
	}
	if err := DB.Exec("DROP TRIGGER reject_time_history").Error; err != nil {
		t.Fatal(err)
	}
	version, err := SaveTimePricingIfVersion(ctx, timePricingRules("v1"), 77, 0)
	if err != nil || version != 1 {
		t.Fatalf("save: %d %v", version, err)
	}
	changes, err := ListOptionChanges(100)
	if err != nil || len(changes) != 1 || changes[0].ActorID != 77 || changes[0].Key != setting.OptionKeyModelTimePricing {
		t.Fatal("time history lost actor or version")
	}
	before = groupConfigurationSnapshot(t)
	if next, err := SaveTimePricingIfVersion(ctx, timePricingRules("v1"), 77, 1); err != nil || next != 1 || groupConfigurationSnapshot(t) != before {
		t.Fatal("canonical no-op advanced history")
	}
	if _, err := SaveTimePricingIfVersion(ctx, timePricingRules("stale"), 77, 0); !errors.Is(err, ErrOptionVersionConflict) || groupConfigurationSnapshot(t) != before {
		t.Fatal("stale rules overwrote current configuration")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := SaveTimePricingIfVersion(cancelled, timePricingRules("cancelled"), 77, 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled update: %v", err)
	}
	if err := CloseDB(); err != nil {
		t.Fatal(err)
	}
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	if state, err := ReadTimePricing(ctx); err != nil || state.Version != 1 || !strings.Contains(string(state.Rules), "v1") || groupConfigurationSnapshot(t) != before {
		t.Fatal("time rules did not survive restart")
	}
	if _, err := SavePricingOptionsIfVersion(ctx, pricingPair("3", "4"), 77, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveTimePricingIfVersion(ctx, timePricingRules("old pricing"), 77, 1); !errors.Is(err, ErrOptionVersionConflict) {
		t.Fatal("independent price edit did not invalidate time draft")
	}
	if _, err := SaveOption(setting.OptionKeyModelTimePricing, timePricingRules("legacy"), 77); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveTimePricingIfVersion(ctx, timePricingRules("old legacy"), 77, 3); !errors.Is(err, ErrOptionVersionConflict) {
		t.Fatal("legacy option edit did not invalidate time draft")
	}
}

func TestTimePricingConcurrentWritersAndCoherentSnapshots(t *testing.T) {
	groupConfigurationDB(t)
	ctx := context.Background()
	results := make(chan error, 8)
	var writers sync.WaitGroup
	for i := 0; i < 8; i++ {
		writers.Go(func() { _, err := SaveTimePricingIfVersion(ctx, timePricingRules("v1"), 77, 0); results <- err })
	}
	writers.Wait()
	close(results)
	winners := 0
	for err := range results {
		if err == nil {
			winners++
		} else if !errors.Is(err, ErrOptionVersionConflict) {
			t.Fatal(err)
		}
	}
	if winners != 1 {
		t.Fatalf("winners=%d", winners)
	}
	done := make(chan error, 1)
	go func() {
		for i := 2; i <= 30; i++ {
			if _, err := SaveTimePricingIfVersion(ctx, timePricingRules(fmt.Sprintf("v%d", i)), 77, int64(i-1)); err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()
	for i := 0; i < 100; i++ {
		state, err := ReadTimePricing(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var schedules map[string]struct {
			Windows []struct {
				Label string `json:"label"`
			} `json:"windows"`
		}
		if err := json.Unmarshal(state.Rules, &schedules); err != nil {
			t.Fatal(err)
		}
		if schedules["gpt-4o"].Windows[0].Label != fmt.Sprintf("v%d", state.Version) {
			t.Fatal("rules and version came from different snapshots")
		}
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestTimePricingStoredCorruptionNeverFallsBackToDefaults(t *testing.T) {
	groupConfigurationDB(t)
	ctx := context.Background()
	for _, value := range []string{"null", "corrupt"} {
		if err := UpsertOption(setting.OptionKeyModelTimePricing, value); err != nil {
			t.Fatal(err)
		}
		if _, err := ReadTimePricing(ctx); err == nil {
			t.Fatal("corrupt rules were replaced by defaults")
		}
	}
	if err := DB.Where("`key` = ?", setting.OptionKeyModelTimePricing).Delete(&Option{}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := ReadTimePricing(ctx); err == nil {
		t.Fatal("missing rules were replaced by defaults")
	}
	if err := UpsertOption(setting.OptionKeyModelTimePricing, "{}"); err != nil {
		t.Fatal(err)
	}
	if err := DB.Model(&OptionRevision{}).Where("id = 1").Update("version", maxPricingOptionVersion+1).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := ReadTimePricing(ctx); err == nil {
		t.Fatal("unsafe version was exposed")
	}
}
