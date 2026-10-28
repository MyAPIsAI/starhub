package pricingtime

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func sampleWindow() Window {
	return Window{ID: "offpeak", Label: "Off peak", Days: []int{1}, Start: "23:00", End: "06:00", Multiplier: 0.8}
}
func catalogFor(t *testing.T, schedules map[string]Schedule) *Catalog {
	t.Helper()
	raw, err := json.Marshal(schedules)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := Decode(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}
func instant(t *testing.T, value string) time.Time {
	t.Helper()
	at, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatal(err)
	}
	return at
}

func TestOvernightUsesStartWeekdayAndExclusiveEnd(t *testing.T) {
	catalog := catalogFor(t, map[string]Schedule{"actual": {Timezone: "Asia/Shanghai", Windows: []Window{sampleWindow()}}})
	for _, tc := range []struct {
		at      string
		matched bool
	}{
		{"2026-10-26T14:59:59Z", false},
		{"2026-10-26T15:00:00Z", true},
		{"2026-10-26T18:00:00Z", true},
		{"2026-10-26T21:59:59Z", true},
		{"2026-10-26T22:00:00Z", false},
		{"2026-10-27T15:00:00Z", false},
		{"2026-10-25T18:00:00Z", false},
	} {
		t.Run(tc.at, func(t *testing.T) {
			got := catalog.Match("actual", instant(t, tc.at))
			if (got.WindowID != "") != tc.matched {
				t.Fatalf("match=%+v want matched=%v", got, tc.matched)
			}
			wantMultiplier := 1.0
			if tc.matched {
				wantMultiplier = 0.8
			}
			if got.Multiplier != wantMultiplier {
				t.Fatalf("wrong multiplier: %+v", got)
			}
			if got.Timezone != "Asia/Shanghai" {
				t.Fatal("timezone was lost")
			}
		})
	}
	if got := catalog.Match("unknown", time.Now()); got.Multiplier != 1 || got.WindowID != "" || got.Timezone != "" {
		t.Fatalf("unknown model invented a window: %+v", got)
	}
	var absent *Catalog
	if got := absent.Match("actual", time.Time{}); got.Multiplier != 1 {
		t.Fatal("missing catalog changed the multiplier")
	}
}

func TestWeeklyWrapRejectsOverlapButAcceptsTouchingAndWholeDays(t *testing.T) {
	first := sampleWindow()
	first.Days = []int{6}
	first.End = "02:00"
	second := Window{ID: "sunday", Label: "Sunday", Days: []int{0}, Start: "01:00", End: "03:00", Multiplier: 1.2}
	raw, _ := json.Marshal(map[string]Schedule{"actual": {Timezone: "UTC", Windows: []Window{first, second}}})
	if _, err := Decode(string(raw)); err == nil {
		t.Fatal("Saturday/Sunday overlap accepted")
	}
	second.Start = "02:00"
	catalog := catalogFor(t, map[string]Schedule{"actual": {Timezone: "UTC", Windows: []Window{first, second}}})
	if got := catalog.Match("actual", instant(t, "2026-10-25T01:59:59Z")); got.WindowID != first.ID {
		t.Fatalf("weekly wrap failed: %+v", got)
	}
	if got := catalog.Match("actual", instant(t, "2026-10-25T02:00:00Z")); got.WindowID != second.ID {
		t.Fatalf("adjacent boundary failed: %+v", got)
	}
	first.Days = []int{6, 0}
	first.Start = "00:00"
	first.End = "24:00"
	catalog = catalogFor(t, map[string]Schedule{"actual": {Timezone: "UTC", Windows: []Window{first}}})
	for _, at := range []string{"2026-10-24T00:00:00Z", "2026-10-25T23:59:59Z"} {
		if got := catalog.Match("actual", instant(t, at)); got.WindowID == "" {
			t.Fatal("whole day not matched")
		}
	}
	if got := catalog.Match("actual", instant(t, "2026-10-26T00:00:00Z")); got.WindowID != "" {
		t.Fatal("whole-day end was inclusive")
	}
}

func TestDaylightSavingUsesRealInstantsWithoutInventingLocalMinutes(t *testing.T) {
	window := sampleWindow()
	window.Days = []int{0}
	window.Start = "01:15"
	window.End = "01:45"
	catalog := catalogFor(t, map[string]Schedule{"actual": {Timezone: "America/New_York", Windows: []Window{window}}})
	for _, at := range []string{"2026-11-01T05:30:00Z", "2026-11-01T06:30:00Z"} {
		if got := catalog.Match("actual", instant(t, at)); got.WindowID == "" {
			t.Fatalf("repeated local minute did not match: %s", at)
		}
	}
	window.Start = "02:00"
	window.End = "03:00"
	catalog = catalogFor(t, map[string]Schedule{"actual": {Timezone: "America/New_York", Windows: []Window{window}}})
	for _, at := range []string{"2026-03-08T06:59:59Z", "2026-03-08T07:00:00Z", "2026-03-08T07:30:00Z"} {
		if got := catalog.Match("actual", instant(t, at)); got.WindowID != "" {
			t.Fatalf("nonexistent local minute was invented: %s", at)
		}
	}
}

func TestStrictScheduleValidation(t *testing.T) {
	invalid := []string{
		`null`, `[]`, `{} {}`, `{"m":null}`,
		`{"m":{"timezone":"UTC"}}`,
		`{"m":{"timezone":"UTC","windows":null}}`,
		`{"m":{"timezone":"Local","windows":[]}}`,
		`{"m":{"timezone":"Mars/Test","windows":[]}}`,
		`{"m":{"timezone":" UTC","windows":[]}}`,
		`{" m":{"timezone":"UTC","windows":[]}}`,
		`{"m":{"timezone":"UTC","windows":[],"unknown":true}}`,
		`{"m":{"timezone":"UTC","windows":[]},"m":{"timezone":"UTC","windows":[]}}`,
		`{"m":{"timezone":"UTC","timezone":"Asia/Shanghai","windows":[]}}`,
		strings.Repeat(" ", MaxCatalogBytes+1),
		"{\"\xff\":{}}",
	}
	mutations := []func(*Window){
		func(w *Window) { w.ID = "" }, func(w *Window) { w.ID = "two words" }, func(w *Window) { w.ID = strings.Repeat("i", 65) },
		func(w *Window) { w.Label = " " }, func(w *Window) { w.Label = "bad\nlabel" }, func(w *Window) { w.Label = strings.Repeat("l", 129) },
		func(w *Window) { w.Days = nil }, func(w *Window) { w.Days = []int{1, 1} }, func(w *Window) { w.Days = []int{-1} }, func(w *Window) { w.Days = []int{7} },
		func(w *Window) { w.Multiplier = 0 }, func(w *Window) { w.Multiplier = -1 }, func(w *Window) { w.Multiplier = 100.01 },
		func(w *Window) { w.Start = "24:00" }, func(w *Window) { w.Start = "6:00" }, func(w *Window) { w.Start = "06:60" }, func(w *Window) { w.Start = "23:00:00" }, func(w *Window) { w.End = "24:01" }, func(w *Window) { w.End = w.Start },
	}
	for _, mutate := range mutations {
		window := sampleWindow()
		mutate(&window)
		raw, _ := json.Marshal(map[string]Schedule{"m": {Timezone: "UTC", Windows: []Window{window}}})
		invalid = append(invalid, string(raw))
	}
	base := `{"id":"a","label":"A","days":[1],"start":"01:00","end":"02:00","multiplier":1}`
	for _, window := range []string{strings.Replace(base, `"multiplier":1`, `"multiplier":"1"`, 1), strings.Replace(base, `"days":[1]`, `"days":[1.5]`, 1), strings.Replace(base, `"multiplier":1`, `"tokenPrices":{},"multiplier":1`, 1), strings.Replace(base, `"multiplier":1`, `"multiplier":1,"multiplier":2`, 1)} {
		invalid = append(invalid, `{"m":{"timezone":"UTC","windows":[`+window+`]}}`)
	}
	invalid = append(invalid, `{"m":{"timezone":"UTC","windows":[`+base+`,`+base+`]}}`)
	for i, raw := range invalid {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			if _, err := Decode(raw); err == nil {
				t.Fatal("invalid schedule accepted")
			}
		})
	}
	for _, raw := range []string{`{}`, `{"m":{"timezone":"UTC","windows":[]}}`} {
		if _, err := Decode(raw); err != nil {
			t.Fatalf("valid empty catalog rejected: %v", err)
		}
	}
}

func TestCanonicalCatalogIsBoundedAndImmutableForConcurrentMatching(t *testing.T) {
	window := sampleWindow()
	window.Days = []int{3, 1}
	schedule := Schedule{Timezone: "UTC", Windows: []Window{window}}
	catalog := catalogFor(t, map[string]Schedule{"m": schedule})
	canonical := catalog.JSON()
	again, err := Decode(canonical)
	if err != nil || again.JSON() != canonical {
		t.Fatal("canonical JSON does not round-trip")
	}
	schedule.Windows[0].Days[0] = 6
	schedule.Windows[0].Multiplier = 100
	if got := catalog.Match("m", instant(t, "2026-10-26T23:30:00Z")); got.Multiplier != 0.8 {
		t.Fatal("caller mutated compiled schedule")
	}
	var workers sync.WaitGroup
	for i := 0; i < 100; i++ {
		workers.Go(func() {
			for j := 0; j < 100; j++ {
				got := catalog.Match("m", instant(t, "2026-10-26T23:30:00Z"))
				if got.Multiplier != 0.8 || catalog.JSON() != canonical {
					t.Error("concurrent catalog changed")
				}
			}
		})
	}
	workers.Wait()
	tooMany := map[string]Schedule{}
	for i := 0; i <= MaxModels; i++ {
		tooMany[fmt.Sprint(i)] = Schedule{Timezone: "UTC", Windows: []Window{}}
	}
	raw, _ := json.Marshal(tooMany)
	if _, err := Decode(string(raw)); err == nil {
		t.Fatal("model bound was not applied")
	}
	windows := make([]Window, MaxWindows+1)
	for i := range windows {
		windows[i] = Window{ID: fmt.Sprint(i), Label: "Minute", Days: []int{1}, Start: fmt.Sprintf("00:%02d", i), End: fmt.Sprintf("00:%02d", i+1), Multiplier: 1}
	}
	raw, _ = json.Marshal(map[string]Schedule{"m": {Timezone: "UTC", Windows: windows}})
	if _, err := Decode(string(raw)); err == nil {
		t.Fatal("window bound was not applied")
	}
	for i := range windows {
		windows[i].Label = strings.Repeat("<", 128)
	}
	amplified := map[string]Schedule{}
	for i := 0; i < 100; i++ {
		amplified[fmt.Sprint(i)] = Schedule{Timezone: "UTC", Windows: windows[:MaxWindows]}
	}
	raw, _ = json.Marshal(amplified)
	raw = []byte(strings.ReplaceAll(string(raw), `\u003c`, `<`))
	if len(raw) >= MaxCatalogBytes {
		t.Fatal("amplification fixture is already over the input bound")
	}
	if _, err := Decode(string(raw)); err == nil {
		t.Fatal("normalized JSON amplification exceeded the output bound")
	}
}
