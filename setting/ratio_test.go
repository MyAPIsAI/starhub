package setting

import "testing"

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
