package setting

import "testing"

func TestRatioOverridesRetainNewModelDefaults(t *testing.T) {
	ResetRatiosToDefaults()
	t.Cleanup(ResetRatiosToDefaults)
	if err := UpdateRatio(OptionKeyModelRatio, `{"gpt-4o":7,"custom":2,"claude-sonnet-4-6":0}`); err != nil {
		t.Fatal(err)
	}
	if err := UpdateRatio(OptionKeyCompletionRatio, `{"gpt-4o":6}`); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name          string
		input, output float64
	}{
		{"gpt-4o", 7, 6},
		{"gpt-5", 1.25, 8},
		{"claude-sonnet-4-6", 0, 5},
		{"claude-haiku-4-5", 1, 5},
		{"claude-haiku-4-5-20251001", 1, 5},
	} {
		input, foundInput := LookupModelRatio(tc.name)
		output, foundOutput := LookupCompletionRatio(tc.name)
		if !foundInput || !foundOutput || input != tc.input || output != tc.output {
			t.Errorf("%s ratios=%g/%g configured=%t/%t", tc.name, input, output, foundInput, foundOutput)
		}
	}
	if got := ModelRatioValue("custom"); got != 2 {
		t.Fatalf("custom override lost: %g", got)
	}
}

func TestDefaultRatios(t *testing.T) {
	if !ContainsGroupRatio("default") {
		t.Fatal("default group should have a ratio")
	}
	if ContainsGroupRatio("no-such-group") {
		t.Fatal("unknown group should not have a ratio")
	}
	got := ModelRatioValue("gpt-4o-mini")
	if got != 0.15 {
		t.Fatalf("gpt-4o-mini model ratio = %v, want 0.15", got)
	}
	if ModelRatioValue("totally-unknown-model") != FallbackModelRatio {
		t.Fatal("unknown model should fall back to the default ratio")
	}
	if CompletionRatioValue("totally-unknown-model") != FallbackCompletionRatio {
		t.Fatal("unknown model should fall back to the default completion ratio")
	}
}

func TestUpdateAndSnapshot(t *testing.T) {
	err := UpdateRatio(OptionKeyGroupRatio, `{"default":1,"vip":0.5}`)
	if err != nil {
		t.Fatalf("UpdateRatio failed: %v", err)
	}
	v, ok := GroupRatioValue("vip")
	if !ok || v != 0.5 {
		t.Fatalf("vip ratio = %v %v, want 0.5 true", v, ok)
	}
	snapshot, err := SnapshotRatio(OptionKeyGroupRatio)
	if err != nil {
		t.Fatalf("SnapshotRatio failed: %v", err)
	}
	if snapshot != `{"default":1,"vip":0.5}` {
		t.Fatalf("unexpected snapshot: %s", snapshot)
	}
}

func TestUpdateRatioRejectsInvalid(t *testing.T) {
	if err := UpdateRatio(OptionKeyModelRatio, `not-json`); err == nil {
		t.Fatal("invalid JSON should be rejected")
	}
	if err := UpdateRatio(OptionKeyModelRatio, `{"m":-1}`); err == nil {
		t.Fatal("negative ratio should be rejected")
	}
	if err := UpdateRatio(OptionKeyModelRatio, `["list"]`); err == nil {
		t.Fatal("non-object JSON should be rejected")
	}
	if err := UpdateRatio("bogus_key", `{}`); err == nil {
		t.Fatal("unknown option key should be rejected")
	}
}
