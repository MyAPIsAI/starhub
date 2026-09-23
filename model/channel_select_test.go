package model

import (
	"strings"
	"testing"

	"starhub/common"
)

func TestSelectKeySingleKey(t *testing.T) {
	ch := &Channel{ID: 1, Key: "sk-only"}
	key, idx := ch.SelectKey()
	if key != "sk-only" || idx != 0 {
		t.Fatalf("SelectKey = %q, %d; want sk-only, 0", key, idx)
	}

	empty := &Channel{ID: 2, Key: "   "}
	if key, idx := empty.SelectKey(); key != "" || idx != -1 {
		t.Fatalf("empty key SelectKey = %q, %d; want \"\", -1", key, idx)
	}
}

// A round-robin channel must hand out every enabled key exactly once per
// cycle. The cursor is in-memory state, so this test resets it first.
func TestSelectKeyRoundRobinCycles(t *testing.T) {
	ResetMultiKeyCounters()
	t.Cleanup(ResetMultiKeyCounters)

	ch := &Channel{
		ID:      42,
		Key:     "k1\nk2\nk3",
		KeyMode: KeyModeRoundRobin,
	}
	const cycles = 4
	counts := map[string]int{}
	for i := 0; i < cycles*3; i++ {
		key, idx := ch.SelectKey()
		if key == "" || idx < 0 {
			t.Fatalf("call %d returned %q, %d", i, key, idx)
		}
		if key != []string{"k1", "k2", "k3"}[idx] {
			t.Fatalf("key %q does not match index %d", key, idx)
		}
		counts[key]++
	}
	for _, k := range []string{"k1", "k2", "k3"} {
		if counts[k] != cycles {
			t.Fatalf("round robin is unfair: %v (want %d each)", counts, cycles)
		}
	}
}

func TestSelectKeySkipsDisabled(t *testing.T) {
	ResetMultiKeyCounters()
	t.Cleanup(ResetMultiKeyCounters)

	ch := &Channel{
		ID:             7,
		Key:            "k1\nk2\nk3",
		KeyMode:        KeyModeRoundRobin,
		MultiKeyStatus: "1,2,1",
	}
	for i := 0; i < 20; i++ {
		key, idx := ch.SelectKey()
		if idx == 1 {
			t.Fatalf("disabled key index 1 was selected (key %q)", key)
		}
		if idx != 0 && idx != 2 {
			t.Fatalf("unexpected index %d", idx)
		}
	}
}

func TestSelectKeyAllDisabled(t *testing.T) {
	ch := &Channel{ID: 9, Key: "k1\nk2", MultiKeyStatus: "2,2"}
	if key, idx := ch.SelectKey(); key != "" || idx != -1 {
		t.Fatalf("all-disabled SelectKey = %q, %d; want \"\", -1", key, idx)
	}
}

func TestKeyStatusesMalformedDefaults(t *testing.T) {
	ch := &Channel{Key: "k1\nk2\nk3", MultiKeyStatus: "2,x"}
	got := ch.KeyStatuses()
	if len(got) != 3 {
		t.Fatalf("KeyStatuses length = %d, want 3", len(got))
	}
	if got[0] != common.StatusDisabled {
		t.Fatalf("entry 0 = %d, want disabled", got[0])
	}
	// "x" is not a number: the entry must fall back to enabled.
	if got[1] != common.StatusEnabled || got[2] != common.StatusEnabled {
		t.Fatalf("malformed/missing entries should default to enabled, got %v", got)
	}
}

// Extra status entries beyond the key count must be ignored rather than
// growing the slice out of alignment with Keys().
func TestKeyStatusesExtraEntriesIgnored(t *testing.T) {
	ch := &Channel{Key: "k1", MultiKeyStatus: "2,2,2,2"}
	got := ch.KeyStatuses()
	if len(got) != 1 {
		t.Fatalf("KeyStatuses length = %d, want 1", len(got))
	}
	if got[0] != common.StatusDisabled {
		t.Fatalf("entry 0 = %d, want disabled", got[0])
	}
}

func TestKeysNormalizesCRLF(t *testing.T) {
	ch := &Channel{Key: "  k1 \r\n\n k2\r\n\r\nk3  \n"}
	got := ch.Keys()
	want := []string{"k1", "k2", "k3"}
	if len(got) != len(want) {
		t.Fatalf("Keys = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Keys = %v, want %v", got, want)
		}
	}
}

func TestIsMultiKey(t *testing.T) {
	if (&Channel{Key: "k1"}).IsMultiKey() {
		t.Fatal("single key should not be multi-key")
	}
	if !(&Channel{Key: "k1\nk2"}).IsMultiKey() {
		t.Fatal("two keys should be multi-key")
	}
}

func TestSplitList(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"a, b ,,c", []string{"a", "b", "c"}},
		{"   ", []string{}},
		{"", []string{}},
		{"single", []string{"single"}},
	}
	for _, tc := range cases {
		got := SplitList(tc.in)
		if len(got) != len(tc.want) {
			t.Fatalf("SplitList(%q) = %v, want %v", tc.in, got, tc.want)
		}
		for i := range tc.want {
			if got[i] != tc.want[i] {
				t.Fatalf("SplitList(%q) = %v, want %v", tc.in, got, tc.want)
			}
		}
	}
}

func TestChannelGroupsFallsBackToDefault(t *testing.T) {
	if got := ChannelGroups(&Channel{Group: " , "}); len(got) != 1 || got[0] != common.DefaultGroup {
		t.Fatalf("ChannelGroups empty = %v, want [%s]", got, common.DefaultGroup)
	}
	got := ChannelGroups(&Channel{Group: "a, b"})
	if strings.Join(got, "|") != "a|b" {
		t.Fatalf("ChannelGroups = %v, want [a b]", got)
	}
}
