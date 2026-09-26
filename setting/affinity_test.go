package setting

import (
	"testing"
	"time"
)

func TestAffinityDefaultsAndDispatch(t *testing.T) {
	ResetAffinityToDefaults()
	t.Cleanup(ResetAffinityToDefaults)

	if RoutingAffinityEnabled() {
		t.Fatal("affinity must be disabled by default")
	}
	if RoutingAffinityTTL() != DefaultRoutingAffinityTTL {
		t.Fatalf("default ttl = %v, want %v", RoutingAffinityTTL(), DefaultRoutingAffinityTTL)
	}
	if !IsAffinityOptionKey(OptionKeyRoutingAffinityEnabled) ||
		!IsAffinityOptionKey(OptionKeyRoutingAffinityTTL) {
		t.Fatal("affinity keys should be recognized")
	}
	if !IsOptionKey(OptionKeyRoutingAffinityEnabled) || !IsOptionKey(OptionKeyRoutingAffinityTTL) {
		t.Fatal("affinity keys should be known options")
	}
}

func TestAffinityEnabledUpdate(t *testing.T) {
	ResetAffinityToDefaults()
	t.Cleanup(ResetAffinityToDefaults)

	for _, value := range []string{`true`, `"true"`, `1`, `on`} {
		if err := UpdateOption(OptionKeyRoutingAffinityEnabled, value); err != nil {
			t.Fatalf("enabling with %q failed: %v", value, err)
		}
		if !RoutingAffinityEnabled() {
			t.Fatalf("affinity not enabled after %q", value)
		}
	}
	for _, value := range []string{`false`, `0`, `off`} {
		if err := UpdateOption(OptionKeyRoutingAffinityEnabled, value); err != nil {
			t.Fatalf("disabling with %q failed: %v", value, err)
		}
		if RoutingAffinityEnabled() {
			t.Fatalf("affinity not disabled after %q", value)
		}
	}
	for _, value := range []string{`maybe`, `2`, `{}`} {
		if err := UpdateOption(OptionKeyRoutingAffinityEnabled, value); err == nil {
			t.Fatalf("invalid bool %q must be rejected", value)
		}
	}
}

func TestAffinityTTLUpdateAndSnapshot(t *testing.T) {
	ResetAffinityToDefaults()
	t.Cleanup(ResetAffinityToDefaults)

	if err := UpdateOption(OptionKeyRoutingAffinityTTL, `120`); err != nil {
		t.Fatalf("valid ttl rejected: %v", err)
	}
	if RoutingAffinityTTL() != 120*time.Second {
		t.Fatalf("ttl = %v, want 120s", RoutingAffinityTTL())
	}
	snapshot, err := SnapshotOption(OptionKeyRoutingAffinityTTL)
	if err != nil || snapshot != "120" {
		t.Fatalf("ttl snapshot = %q, %v; want \"120\"", snapshot, err)
	}

	// A value above the cap is clamped rather than rejected.
	if err := UpdateOption(OptionKeyRoutingAffinityTTL, `1000000`); err != nil {
		t.Fatalf("over-cap ttl rejected: %v", err)
	}
	if RoutingAffinityTTL() != MaxRoutingAffinityTTL {
		t.Fatalf("over-cap ttl = %v, want the cap %v", RoutingAffinityTTL(), MaxRoutingAffinityTTL)
	}

	for _, bad := range []string{`0`, `-1`, `notjson`} {
		if err := UpdateOption(OptionKeyRoutingAffinityTTL, bad); err == nil {
			t.Fatalf("invalid ttl %q must be rejected", bad)
		}
	}
}

func TestAffinityBoolSnapshot(t *testing.T) {
	ResetAffinityToDefaults()
	t.Cleanup(ResetAffinityToDefaults)

	if err := UpdateOption(OptionKeyRoutingAffinityEnabled, `true`); err != nil {
		t.Fatalf("enable failed: %v", err)
	}
	snapshot, err := SnapshotOption(OptionKeyRoutingAffinityEnabled)
	if err != nil || snapshot != "true" {
		t.Fatalf("enabled snapshot = %q, %v; want \"true\"", snapshot, err)
	}
}
