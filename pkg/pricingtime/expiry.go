package pricingtime

import "time"

// NextChange finds the next actual wall-clock schedule transition, including
// repeated or skipped DST minutes. Unscheduled models have no expiration.
func (c *Catalog) NextChange(model string, at time.Time) int64 {
	if c == nil {
		return 0
	}
	schedule, exists := c.schedules[model]
	if !exists || len(schedule.windows) == 0 {
		return 0
	}
	current := c.Match(model, at)
	boundary := at.Truncate(time.Minute).Add(time.Minute)
	for i := 0; i < 8*24*60; i++ {
		if c.Match(model, boundary) != current {
			return boundary.Unix()
		}
		boundary = boundary.Add(time.Minute)
	}
	return 0
}

func (c *TokenCatalog) NextChange(model string, at time.Time) int64 {
	if c == nil {
		return 0
	}
	return c.matcher.NextChange(model, at)
}

func EarliestChange(a, b int64) int64 {
	if a == 0 || b != 0 && b < a {
		return b
	}
	return a
}
