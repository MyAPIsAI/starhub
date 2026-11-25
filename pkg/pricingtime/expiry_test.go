package pricingtime

import (
	"testing"
	"time"
)

func TestNextPriceChange(t *testing.T) {
	c, err := Decode(`{"m":{"timezone":"UTC","windows":[{"id":"peak","label":"Peak","days":[3],"start":"12:00","end":"13:00","multiplier":2}]}}`)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 11, 25, 11, 59, 30, 0, time.UTC)
	if got := c.NextChange("m", at); got != at.Add(30*time.Second).Unix() {
		t.Fatalf("wrong start: %d", got)
	}
	at = at.Add(time.Minute)
	if got := c.NextChange("m", at); got != time.Date(2026, 11, 25, 13, 0, 0, 0, time.UTC).Unix() {
		t.Fatalf("wrong end: %d", got)
	}
	if c.NextChange("unknown", at) != 0 {
		t.Fatal("unscheduled model expires")
	}
}
