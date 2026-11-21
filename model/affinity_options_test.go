package model

import (
	"errors"
	"starhub/setting"
	"testing"
)

func TestGlobalAffinityBatchRollback(t *testing.T) {
	groupConfigurationDB(t)
	ctx := t.Context()
	before := groupConfigurationSnapshot(t)
	opts := []Option{{Key: setting.OptionKeyRoutingAffinityEnabled, Value: "true"}, {Key: setting.OptionKeyRoutingAffinityTTL, Value: "451"}}
	for _, bad := range []Option{opts[0], {Key: setting.OptionKeyRoutingAffinityRules, Value: "[]"}, {Key: setting.OptionKeyRoutingAffinityCooldown, Value: "601"}} {
		if _, err := SaveAffinityOptionsIfVersion(ctx, []Option{opts[0], bad}, 1, 0); !errors.Is(err, ErrInvalidAffinityBatch) {
			t.Fatal(err)
		}
	}
	if err := DB.Exec(`CREATE TRIGGER reject_affinity_ttl BEFORE INSERT ON option_changes WHEN NEW.key='routing_affinity_ttl_seconds' BEGIN SELECT RAISE(ABORT,'fixture error'); END`).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := SaveAffinityOptionsIfVersion(ctx, opts, 1, 0); err == nil || groupConfigurationSnapshot(t) != before {
		t.Fatal("partial option batch or history survived failure")
	}
	if err := DB.Exec(`DROP TRIGGER reject_affinity_ttl`).Error; err != nil {
		t.Fatal(err)
	}
	if v, err := SaveAffinityOptionsIfVersion(ctx, opts, 1, 0); err != nil || v != 2 {
		t.Fatalf("save: %d %v", v, err)
	}
	if _, err := SaveAffinityOptionsIfVersion(ctx, opts, 1, 0); !errors.Is(err, ErrOptionVersionConflict) {
		t.Fatal("stale version accepted")
	}
}
