// Package pricingtime validates and matches immutable weekly pricing schedules.
// Callers supply the request instant explicitly; matching never reads a clock.
package pricingtime

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"
	"unicode"
	"unicode/utf8"
)

const (
	Schema          = "weekly_multiplier_v1"
	MaxCatalogBytes = 1 << 20
	MaxModels       = 1024
	MaxWindows      = 32
	weekMinutes     = 7 * 24 * 60
)

type Window struct {
	ID         string  `json:"id"`
	Label      string  `json:"label"`
	Days       []int   `json:"days"`
	Start      string  `json:"start"`
	End        string  `json:"end"`
	Multiplier float64 `json:"multiplier"`
}

type Schedule struct {
	Timezone string   `json:"timezone"`
	Windows  []Window `json:"windows"`
}

type Adjustment struct {
	Multiplier float64 `json:"multiplier"`
	WindowID   string  `json:"window_id"`
	Timezone   string  `json:"timezone"`
}

type compiledWindow struct {
	id         string
	days       [7]bool
	start, end int
	multiplier float64
}

type compiledSchedule struct {
	location *time.Location
	windows  []compiledWindow
}

// Catalog owns its decoded data and can be shared by concurrent readers.
type Catalog struct {
	schedules map[string]compiledSchedule
	canonical string
}

var windowID = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)

func Decode(raw string) (*Catalog, error) {
	if len(raw) > MaxCatalogBytes || !utf8.ValidString(raw) {
		return nil, fmt.Errorf("invalid time-pricing payload size or encoding")
	}
	if err := uniqueJSON(raw); err != nil {
		return nil, err
	}
	var schedules map[string]Schedule
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&schedules); err != nil {
		return nil, err
	}
	if schedules == nil || len(schedules) > MaxModels {
		return nil, fmt.Errorf("time-pricing catalog must contain at most %d model schedules", MaxModels)
	}
	result := &Catalog{schedules: make(map[string]compiledSchedule, len(schedules))}
	for model, schedule := range schedules {
		if model == "" || model != strings.TrimSpace(model) || len(model) > 128 || strings.ContainsFunc(model, unicode.IsControl) {
			return nil, fmt.Errorf("invalid time-pricing model ID")
		}
		compiled, err := compile(schedule)
		if err != nil {
			return nil, fmt.Errorf("time pricing for %q: %w", model, err)
		}
		for i := range schedule.Windows {
			sort.Ints(schedule.Windows[i].Days)
		}
		sort.Slice(schedule.Windows, func(i, j int) bool { return schedule.Windows[i].ID < schedule.Windows[j].ID })
		schedules[model] = schedule
		result.schedules[model] = compiled
	}
	canonical, err := json.Marshal(schedules)
	if err != nil {
		return nil, err
	}
	if len(canonical) > MaxCatalogBytes {
		return nil, fmt.Errorf("normalized time-pricing catalog exceeds size limit")
	}
	result.canonical = string(canonical)
	return result, nil
}

func (c *Catalog) JSON() string { return c.canonical }

// Match uses local wall-clock minutes. Overnight windows belong to the start
// weekday and every end is exclusive. Repeated DST minutes match both instants;
// skipped local minutes have no corresponding instant and are never invented.
func (c *Catalog) Match(model string, at time.Time) Adjustment {
	result := Adjustment{Multiplier: 1}
	if c == nil {
		return result
	}
	schedule, ok := c.schedules[model]
	if !ok {
		return result
	}
	result.Timezone = schedule.location.String()
	local := at.In(schedule.location)
	day := int(local.Weekday())
	minute := local.Hour()*60 + local.Minute()
	for _, window := range schedule.windows {
		matched := window.days[day] && minute >= window.start && minute < window.end
		if window.end < window.start {
			matched = window.days[day] && minute >= window.start || window.days[(day+6)%7] && minute < window.end
		}
		if matched {
			result.Multiplier = window.multiplier
			result.WindowID = window.id
			return result
		}
	}
	return result
}

func compile(schedule Schedule) (compiledSchedule, error) {
	if schedule.Timezone == "" || schedule.Timezone == "Local" || schedule.Timezone != strings.TrimSpace(schedule.Timezone) || len(schedule.Timezone) > 64 {
		return compiledSchedule{}, fmt.Errorf("an explicit IANA timezone or UTC is required")
	}
	location, err := time.LoadLocation(schedule.Timezone)
	if err != nil {
		return compiledSchedule{}, fmt.Errorf("invalid timezone")
	}
	if schedule.Windows == nil || len(schedule.Windows) > MaxWindows {
		return compiledSchedule{}, fmt.Errorf("windows must be an array of at most %d entries", MaxWindows)
	}
	result := compiledSchedule{location: location}
	type interval struct{ start, end int }
	var intervals []interval
	ids := map[string]bool{}
	for _, window := range schedule.Windows {
		if !windowID.MatchString(window.ID) || ids[window.ID] {
			return result, fmt.Errorf("window IDs must be unique ASCII identifiers of 1–64 bytes")
		}
		ids[window.ID] = true
		if strings.TrimSpace(window.Label) == "" || len(window.Label) > 128 || strings.ContainsFunc(window.Label, unicode.IsControl) {
			return result, fmt.Errorf("window label must contain 1–128 bytes without control characters")
		}
		if window.Multiplier <= 0 || window.Multiplier > 100 || math.IsNaN(window.Multiplier) || math.IsInf(window.Multiplier, 0) {
			return result, fmt.Errorf("window multiplier must be greater than zero and at most 100")
		}
		start, err := minute(window.Start, false)
		if err != nil {
			return result, err
		}
		end, err := minute(window.End, true)
		if err != nil {
			return result, err
		}
		if start == end {
			return result, fmt.Errorf("equal start/end is ambiguous; use 00:00–24:00 for a whole day")
		}
		if len(window.Days) == 0 || len(window.Days) > 7 {
			return result, fmt.Errorf("each window requires 1–7 distinct weekdays")
		}
		compiled := compiledWindow{id: window.ID, start: start, end: end, multiplier: window.Multiplier}
		for _, day := range window.Days {
			if day < 0 || day > 6 || compiled.days[day] {
				return result, fmt.Errorf("weekdays must be distinct integers from 0 to 6")
			}
			compiled.days[day] = true
			from, to := day*1440+start, day*1440+end
			if to < from {
				to += 1440
			}
			if to > weekMinutes {
				intervals = append(intervals, interval{from, weekMinutes}, interval{0, to - weekMinutes})
			} else {
				intervals = append(intervals, interval{from, to})
			}
		}
		result.windows = append(result.windows, compiled)
	}
	sort.Slice(intervals, func(i, j int) bool { return intervals[i].start < intervals[j].start })
	for i := 1; i < len(intervals); i++ {
		if intervals[i].start < intervals[i-1].end {
			return result, fmt.Errorf("weekly pricing windows overlap")
		}
	}
	return result, nil
}

func minute(value string, allowDayEnd bool) (int, error) {
	if allowDayEnd && value == "24:00" {
		return 1440, nil
	}
	if len(value) != 5 || value[2] != ':' {
		return 0, fmt.Errorf("time must use HH:MM")
	}
	for _, index := range []int{0, 1, 3, 4} {
		if value[index] < '0' || value[index] > '9' {
			return 0, fmt.Errorf("time must use ASCII HH:MM")
		}
	}
	hour, _ := strconv.Atoi(value[:2])
	minutes, _ := strconv.Atoi(value[3:])
	if hour > 23 || minutes > 59 {
		return 0, fmt.Errorf("time is outside the daily range")
	}
	return hour*60 + minutes, nil
}

// Reject duplicate fields instead of silently accepting the decoder's last
// value. A low depth bound also rejects unrelated deeply nested payloads.
func uniqueJSON(raw string) error {
	decoder := json.NewDecoder(strings.NewReader(raw))
	var value func(int) error
	value = func(depth int) error {
		if depth > 16 {
			return fmt.Errorf("time-pricing JSON is too deeply nested")
		}
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		delimiter, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delimiter {
		case '{':
			seen := map[string]bool{}
			for decoder.More() {
				key, err := decoder.Token()
				if err != nil {
					return err
				}
				name, ok := key.(string)
				if !ok || seen[name] {
					return fmt.Errorf("duplicate or invalid JSON field")
				}
				seen[name] = true
				if err := value(depth + 1); err != nil {
					return err
				}
			}
		case '[':
			for decoder.More() {
				if err := value(depth + 1); err != nil {
					return err
				}
			}
		default:
			return fmt.Errorf("unexpected JSON delimiter")
		}
		_, err = decoder.Token()
		return err
	}
	if err := value(0); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return fmt.Errorf("trailing time-pricing JSON data")
	}
	return nil
}
