package model

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"starhub/setting"
)

func independentTimeRules(label string) string {
	return fmt.Sprintf(`{"gpt-4o":{"timezone":"UTC","windows":[{"id":"all","label":%q,"days":[0,1,2,3,4,5,6],"start":"00:00","end":"24:00","prices":{"input_per_m":"1.23","output_per_m":"4.56","cache_read_per_m":"0","cache_write_per_m":"2.5","audio_input_per_m":"8","audio_output_per_m":"16"}}]}}`, label)
}

func TestIndependentTimePricingDurabilityCASRollbackAndModeGuards(t *testing.T) {
	path := groupConfigurationDB(t)
	ctx := context.Background()
	state, err := ReadTokenTimePricing(ctx)
	if err != nil || state.Version != 0 || string(state.Rules) != "{}" {
		t.Fatalf("initial independent price state: %+v %v", state, err)
	}
	before := groupConfigurationSnapshot(t)
	if err := DB.Exec(`CREATE TRIGGER reject_token_price_history BEFORE INSERT ON option_changes WHEN NEW.key = 'model_time_token_prices' BEGIN SELECT RAISE(ABORT, 'synthetic history failure'); END`).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := SaveTokenTimePricingIfVersion(ctx, independentTimeRules("failed"), 77, 0); err == nil || groupConfigurationSnapshot(t) != before {
		t.Fatal("history failure leaked prices or versions")
	}
	if err := DB.Exec("DROP TRIGGER reject_token_price_history").Error; err != nil {
		t.Fatal(err)
	}
	version, err := SaveTokenTimePricingIfVersion(ctx, independentTimeRules("first"), 77, 0)
	if err != nil || version != 1 {
		t.Fatalf("save: %d %v", version, err)
	}
	before = groupConfigurationSnapshot(t)
	if next, err := SaveTokenTimePricingIfVersion(ctx, independentTimeRules("first"), 77, 1); err != nil || next != 1 || groupConfigurationSnapshot(t) != before {
		t.Fatal("canonical no-op changed history")
	}
	if _, err := SaveTokenTimePricingIfVersion(ctx, independentTimeRules("stale"), 77, 0); !errors.Is(err, ErrOptionVersionConflict) {
		t.Fatal("stale token draft replaced current prices")
	}
	for key, raw := range map[string]string{setting.OptionKeyModelPrice: `{"gpt-4o":0.01}`, setting.OptionKeyModelPriceExpr: `{"gpt-4o":"0.01"}`} {
		if _, err := SaveOption(key, raw, 77); !errors.Is(err, setting.ErrTokenPriceMode) || groupConfigurationSnapshot(t) != before {
			t.Fatalf("generic option bypassed mode guard: %v", err)
		}
	}
	if err := CloseDB(); err != nil {
		t.Fatal(err)
	}
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	if state, err := ReadTokenTimePricing(ctx); err != nil || state.Version != 1 || !strings.Contains(string(state.Rules), "first") {
		t.Fatal("restart lost token configuration")
	}
	if _, err := SaveTokenTimePricingIfVersion(ctx, "{}", 77, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveOption(setting.OptionKeyModelPrice, `{"gpt-4o":0.01}`, 77); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveTokenTimePricingIfVersion(ctx, independentTimeRules("conflicting"), 77, 3); !errors.Is(err, ErrInvalidTokenTimePricing) {
		t.Fatal("absolute schedule hid stored flat price")
	}
	if _, err := SaveOption(setting.OptionKeyModelPrice, "{}", 77); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveTokenTimePricingIfVersion(ctx, independentTimeRules("restored"), 77, 4); err != nil {
		t.Fatal(err)
	}
	before = groupConfigurationSnapshot(t)
	if _, err := RollbackOption(4, 5, 77); !errors.Is(err, setting.ErrTokenPriceMode) || groupConfigurationSnapshot(t) != before {
		t.Fatalf("history rollback bypassed coupled mode guard: %v", err)
	}
}

func TestIndependentTimePricingConcurrentWritersShareRevision(t *testing.T) {
	groupConfigurationDB(t)
	var workers sync.WaitGroup
	results := make(chan error, 6)
	for index := 0; index < 6; index++ {
		workers.Go(func() {
			_, err := SaveTokenTimePricingIfVersion(context.Background(), independentTimeRules("race"), 77, 0)
			results <- err
		})
	}
	workers.Wait()
	close(results)
	winners, conflicts := 0, 0
	for err := range results {
		if err == nil {
			winners++
		} else if errors.Is(err, ErrOptionVersionConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if winners != 1 || conflicts != 5 {
		t.Fatalf("writers=%d conflicts=%d", winners, conflicts)
	}
}
