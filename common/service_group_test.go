package common

import "testing"

func TestServiceGroupMultiplierUsesExactFourDecimalPrecision(t *testing.T) {
	for _, value := range []string{"0", "0.0000", "1", ".5", "+.5", "1e-3", "1e-4", "0.1234", "0.0003", "100", "100.0000", "1.234e1"} {
		if _, err := ParseServiceGroupMultiplier(value); err != nil {
			t.Errorf("valid multiplier %q: %v", value, err)
		}
	}
	for _, value := range []string{"", "-1", "NaN", "Infinity", "1e999", "1e1000000000", "1e-1000000000", "100.0001", "0.12345", "0.00001", "1e-5", "0x10", "1/2"} {
		if _, err := ParseServiceGroupMultiplier(value); err == nil {
			t.Errorf("invalid multiplier %q accepted", value)
		}
	}
}
