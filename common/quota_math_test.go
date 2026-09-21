package common

import (
	"bytes"
	"log"
	"testing"
)

func captureSaturationLogs(t *testing.T, fn func()) string {
	t.Helper()
	var buf bytes.Buffer
	orig := log.Writer()
	origFlags := log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(orig)
		log.SetFlags(origFlags)
	}()
	fn()
	return buf.String()
}

func TestQuotaFromFloat64Rounding(t *testing.T) {
	cases := []struct {
		in   float64
		want int64
	}{
		{0, 0},
		{0.4, 0},
		{0.5, 1},
		{1.2, 1},
		{1.5, 2},
		{22.5, 23},
		{123456.789, 123457},
	}
	for _, c := range cases {
		if got := QuotaFromFloat64(c.in); got != c.want {
			t.Errorf("QuotaFromFloat64(%v) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestQuotaFromFloat64Saturation(t *testing.T) {
	cases := []struct {
		in   float64
		want int64
	}{
		{-1, 0},
		{-0.001, 0},
		{float64(MaxQuotaPerRequest) * 1e18, MaxQuotaPerRequest},
	}
	for _, c := range cases {
		logged := captureSaturationLogs(t, func() {
			if got := QuotaFromFloat64(c.in); got != c.want {
				t.Errorf("QuotaFromFloat64(%v) = %d, want %d", c.in, got, c.want)
			}
		})
		if !bytes.Contains([]byte(logged), []byte(QuotaSaturation)) {
			t.Errorf("QuotaFromFloat64(%v) should audit with %s marker, got log: %q", c.in, QuotaSaturation, logged)
		}
	}
	// NaN needs a special case: comparisons with NaN are always false.
	captureSaturationLogs(t, func() {
		if got := QuotaFromFloat64(nan()); got != 0 {
			t.Errorf("QuotaFromFloat64(NaN) = %d, want 0", got)
		}
	})
}

func nan() float64 {
	zero := 0.0
	return zero / zero
}

func TestQuotaFromFloat64QuietOnNormalValues(t *testing.T) {
	logged := captureSaturationLogs(t, func() {
		QuotaFromFloat64(0)
		QuotaFromFloat64(42)
		QuotaFromFloat64(98765.4)
	})
	if logged != "" {
		t.Fatalf("normal conversions must not log, got %q", logged)
	}
}

func TestClampRatio(t *testing.T) {
	if got := ClampRatio(1.0, "group"); got != 1.0 {
		t.Fatalf("ClampRatio(1.0) = %v", got)
	}
	if got := ClampRatio(0.5, "cache"); got != 0.5 {
		t.Fatalf("ClampRatio(0.5) = %v", got)
	}
	logged := captureSaturationLogs(t, func() {
		if got := ClampRatio(-2, "test"); got != 0 {
			t.Fatalf("negative ratio should clamp to 0, got %v", got)
		}
		if got := ClampRatio(1e30, "test"); got != MaxRatioProduct {
			t.Fatalf("huge ratio should clamp to MaxRatioProduct, got %v", got)
		}
		if got := ClampRatio(nan(), "test"); got != 0 {
			t.Fatalf("NaN ratio should clamp to 0, got %v", got)
		}
	})
	if logged == "" {
		t.Fatal("clamped ratios must leave an audit line")
	}
}
