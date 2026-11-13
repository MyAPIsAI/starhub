package common

import (
	"math"
	"testing"
)

func TestNanoAmountsPreserveLegacyBalancesAndSingleNanoDebits(t *testing.T) {
	for _, q := range []int64{math.MaxInt64, math.MinInt64, 0, 1, -1} {
		value := QuotaMoney(q, 0)
		whole, remainder, err := NanoQuotaParts(value)
		if err != nil || whole != q || remainder != 0 {
			t.Fatal("legacy money did not round trip")
		}
	}
	amount, err := ParseNano("1999")
	if err != nil {
		t.Fatal(err)
	}
	q, r, err := NanoQuotaParts(amount)
	if err != nil || q != 0 || r != 1999 {
		t.Fatal("fractional quota was lost")
	}
	if QuotaMoney(math.MaxInt64, 0).String() != "18446744073709551614000" {
		t.Fatal("legacy balance overflowed")
	}
}

func TestUSDParsingIsExactAtNanoPrecision(t *testing.T) {
	for raw, want := range map[string]string{"0.000000001": "1", "-0.000000001": "-1", "123456789.123456789": "123456789123456789"} {
		got, err := ParseUSDNano(raw)
		if err != nil || got.String() != want {
			t.Fatalf("%s -> %v %v", raw, got, err)
		}
	}
	for _, raw := range []string{"NaN", "1e3", "+1", "01", "0.0000000001", " 1"} {
		if _, err := ParseUSDNano(raw); err == nil {
			t.Fatalf("invalid decimal accepted: %s", raw)
		}
	}
}
