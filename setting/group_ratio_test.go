package setting

import "testing"

func TestGroupRatioOptionsEnforceTheSameExactMultiplierContract(t *testing.T) {
	ResetRatiosToDefaults()
	t.Cleanup(ResetRatiosToDefaults)
	const valid = `{"default":1,"free":0,"fraction":0.1234,"scientific":1e-4}`
	if err := UpdateRatio(OptionKeyGroupRatio, valid); err != nil {
		t.Fatal(err)
	}
	before, err := SnapshotRatio(OptionKeyGroupRatio)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{`{"default":100.0001}`, `{"default":0.12345}`, `{"default":0.100000000000000000001}`, `{"default":null}`, `{"default":"0.5"}`, `null`} {
		if _, err := NormalizeOption(OptionKeyGroupRatio, value); err == nil {
			t.Errorf("normalization accepted invalid group ratio %s", value)
		}
		if err := UpdateRatio(OptionKeyGroupRatio, value); err == nil {
			t.Errorf("serving update accepted invalid group ratio %s", value)
		}
		after, _ := SnapshotRatio(OptionKeyGroupRatio)
		if after != before {
			t.Fatal("invalid group ratio changed serving settings")
		}
	}
	if err := UpdateRatio(OptionKeyModelRatio, `{"independent-model":1000.12345}`); err != nil {
		t.Fatalf("group multiplier limits leaked into model token prices: %v", err)
	}
}
