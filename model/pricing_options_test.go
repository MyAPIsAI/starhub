package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"

	"starhub/setting"
)

func pricingPair(input, completion string) []Option {
	return []Option{{Key: setting.OptionKeyModelRatio, Value: `{"paired":` + input + `,"unrelated":0}`}, {Key: setting.OptionKeyCompletionRatio, Value: `{"paired":` + completion + `,"unrelated":0}`}}
}

func TestPricingOptionBatchIsAtomicGuardedAndRetainsPerKeyHistory(t *testing.T) {
	path := groupConfigurationDB(t)
	ctx := context.Background()
	state, err := ReadPricingOptions(ctx)
	if err != nil || len(state.Options) != 6 || state.Version != 0 {
		t.Fatalf("seeded pricing snapshot: %+v %v", state, err)
	}
	before := groupConfigurationSnapshot(t)
	for _, options := range [][]Option{nil, {{Key: "group_ratio", Value: `{"default":1}`}}, {{Key: "alert_webhook_url", Value: `"http://invalid"`}}, {{Key: "model_ratio", Value: `{}`}, {Key: "model_ratio", Value: `{}`}}, {{Key: "model_ratio", Value: `{"paired":3}`}, {Key: "completion_ratio", Value: `{"paired":-1}`}}, {{Key: "model_ratio", Value: strings.Repeat(" ", maxPricingOptionBytes+1)}}} {
		if _, err := SavePricingOptionsIfVersion(ctx, options, 77, 0); !errors.Is(err, ErrInvalidPricingOptions) {
			t.Fatalf("invalid batch was accepted: %v", err)
		}
		if groupConfigurationSnapshot(t) != before {
			t.Fatal("invalid batch changed metadata, values, history or revision")
		}
	}
	for _, version := range []int64{-1, maxPricingOptionVersion, maxPricingOptionVersion + 1} {
		if _, err := SavePricingOptionsIfVersion(ctx, pricingPair("3", "4"), 77, version); !errors.Is(err, ErrInvalidPricingOptions) {
			t.Fatal("invalid version accepted")
		}
	}
	// Sorted completion_ratio commits first; abort the second key's history insert.
	if err := DB.Exec(`CREATE TRIGGER reject_second_price BEFORE INSERT ON option_changes WHEN NEW.key = 'model_ratio' BEGIN SELECT RAISE(ABORT, 'synthetic second-key failure'); END`).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := SavePricingOptionsIfVersion(ctx, pricingPair("3", "4"), 77, 0); err == nil {
		t.Fatal("second-key failure did not abort the batch")
	}
	if groupConfigurationSnapshot(t) != before {
		t.Fatal("first key or its history escaped a failed batch")
	}
	if err := DB.Exec("DROP TRIGGER reject_second_price").Error; err != nil {
		t.Fatal(err)
	}
	version, err := SavePricingOptionsIfVersion(ctx, pricingPair("3", "4"), 77, 0)
	if err != nil || version != 2 {
		t.Fatalf("batch save: %d %v", version, err)
	}
	changes, err := ListOptionChanges(100)
	if err != nil || len(changes) != 2 || changes[0].ActorID != 77 || changes[1].ActorID != 77 || changes[0].Version != 2 || changes[1].Version != 1 {
		t.Fatal("batch lost per-key versions or actor")
	}
	before = groupConfigurationSnapshot(t)
	if next, err := SavePricingOptionsIfVersion(ctx, pricingPair("3", "4"), 77, version); err != nil || next != version || groupConfigurationSnapshot(t) != before {
		t.Fatal("unchanged batch created a new version or history")
	}
	if _, err := SavePricingOptionsIfVersion(ctx, pricingPair("9", "9"), 77, 0); !errors.Is(err, ErrOptionVersionConflict) || groupConfigurationSnapshot(t) != before {
		t.Fatal("stale batch overwrote prices")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := SavePricingOptionsIfVersion(cancelled, pricingPair("9", "9"), 77, version); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled batch wrote configuration")
	}
	if err := CloseDB(); err != nil {
		t.Fatal(err)
	}
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	after, err := ReadPricingOptions(ctx)
	if err != nil || after.Version != version || groupConfigurationSnapshot(t) != before {
		t.Fatal("batch did not survive restart")
	}
}

func TestPricingOptionBatchConcurrentWritersAndSnapshotCoherence(t *testing.T) {
	groupConfigurationDB(t)
	ctx := context.Background()
	var writers sync.WaitGroup
	results := make(chan error, 8)
	for i := range 8 {
		writers.Add(1)
		go func(value int) {
			defer writers.Done()
			_, err := SavePricingOptionsIfVersion(ctx, pricingPair(fmt.Sprint(value+2), fmt.Sprint(value+2)), int64(value+1), 0)
			results <- err
		}(i)
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
		t.Fatalf("concurrent batch winners=%d", winners)
	}
	writeErrors := make(chan error, 1)
	writers.Add(1)
	go func() {
		defer writers.Done()
		for i := range 30 {
			version, err := OptionVersion()
			if err == nil {
				_, err = SavePricingOptionsIfVersion(ctx, pricingPair(fmt.Sprint(i+10), fmt.Sprint(i+10)), 1, version)
			}
			if err != nil {
				writeErrors <- err
				return
			}
		}
	}()
	for range 100 {
		state, err := ReadPricingOptions(ctx)
		if err != nil {
			t.Error(err)
			break
		}
		values := map[string]float64{}
		for _, option := range state.Options {
			if option.Key == "model_ratio" || option.Key == "completion_ratio" {
				var rates map[string]float64
				if err := json.Unmarshal([]byte(option.Value), &rates); err != nil {
					t.Error(err)
				}
				values[option.Key] = rates["paired"]
				if zero, ok := rates["unrelated"]; !ok || zero != 0 {
					t.Error("explicit unrelated zero was lost")
				}
			}
		}
		if values["model_ratio"] != values["completion_ratio"] {
			t.Errorf("partial pricing pair observed: %+v", state)
			break
		}
	}
	writers.Wait()
	select {
	case err := <-writeErrors:
		t.Fatal(err)
	default:
	}
}

func TestPricingSnapshotRejectsMissingCorruptAndUnsafeState(t *testing.T) {
	groupConfigurationDB(t)
	ctx := context.Background()
	before, err := ReadPricingOptions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if after, err := ReadPricingOptions(ctx); err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("unchanged snapshot differs")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if state, err := ReadPricingOptions(cancelled); !errors.Is(err, context.Canceled) || state != nil {
		t.Fatal("cancelled read returned authoritative data")
	}
	if err := DB.Model(&OptionRevision{}).Where("id = 1").Update("version", maxPricingOptionVersion+1).Error; err != nil {
		t.Fatal(err)
	}
	if state, err := ReadPricingOptions(ctx); err == nil || state != nil {
		t.Fatal("unsafe version accepted")
	}
	if err := DB.Model(&OptionRevision{}).Where("id = 1").Update("version", 0).Error; err != nil {
		t.Fatal(err)
	}
	if err := UpsertOption("model_ratio", `invalid`); err != nil {
		t.Fatal(err)
	}
	if state, err := ReadPricingOptions(ctx); err == nil || state != nil {
		t.Fatal("corrupt state replaced with default prices")
	}
	if err := DB.Where("`key` = ?", "model_ratio").Delete(&Option{}).Error; err != nil {
		t.Fatal(err)
	}
	if state, err := ReadPricingOptions(ctx); err == nil || state != nil {
		t.Fatal("missing state replaced with default prices")
	}
}
