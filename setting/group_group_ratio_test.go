package setting

import (
	"strings"
	"testing"
)

func TestGroupGroupRatioDefault(t *testing.T) {
	// Missing entries default to 1.0 (no extra multiplier).
	if v := GroupGroupRatioValue("vip", "premium"); v != 1.0 {
		t.Fatalf("missing matrix entry should be 1.0, got %v", v)
	}
	if v := GroupGroupRatioValue("", ""); v != 1.0 {
		t.Fatalf("empty groups should be 1.0, got %v", v)
	}
}

func TestGroupGroupRatioUpdateAndSnapshot(t *testing.T) {
	matrix := `{"vip":{"premium":0.8,"default":1},"free":{"premium":1.5}}`
	if err := UpdateGroupGroupRatio(matrix); err != nil {
		t.Fatalf("UpdateGroupGroupRatio failed: %v", err)
	}
	if v := GroupGroupRatioValue("vip", "premium"); v != 0.8 {
		t.Fatalf("vip/premium = %v, want 0.8", v)
	}
	if v := GroupGroupRatioValue("free", "premium"); v != 1.5 {
		t.Fatalf("free/premium = %v, want 1.5", v)
	}
	if v := GroupGroupRatioValue("vip", "unknown"); v != 1.0 {
		t.Fatalf("missing entry should be 1.0, got %v", v)
	}
	snap, err := SnapshotGroupGroupRatio()
	if err != nil {
		t.Fatalf("SnapshotGroupGroupRatio failed: %v", err)
	}
	// encoding/json sorts keys, so the snapshot is stable but may differ
	// from the input order. Check the parsed structure instead.
	if !strings.Contains(snap, `"premium":0.8`) || !strings.Contains(snap, `"default":1`) || !strings.Contains(snap, `"premium":1.5`) {
		t.Fatalf("snapshot missing expected entries: %s", snap)
	}

	// Reset to empty for other tests.
	if err := UpdateGroupGroupRatio(`{}`); err != nil {
		t.Fatalf("reset failed: %v", err)
	}
}

func TestGroupGroupRatioRejectsInvalid(t *testing.T) {
	if err := UpdateGroupGroupRatio(`not-json`); err == nil {
		t.Fatal("invalid JSON should be rejected")
	}
	if err := UpdateGroupGroupRatio(`{"m":{"x":-1}}`); err == nil {
		t.Fatal("negative ratio should be rejected")
	}
	if err := UpdateGroupGroupRatio(`{"":{"x":1}}`); err == nil {
		t.Fatal("empty user group key should be rejected")
	}
	if err := UpdateGroupGroupRatio(`{"m":{"":1}}`); err == nil {
		t.Fatal("empty using group key should be rejected")
	}
}
