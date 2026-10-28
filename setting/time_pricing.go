package setting

import (
	"sync/atomic"
	"time"

	"starhub/pkg/pricingtime"
)

const OptionKeyModelTimePricing = "model_time_pricing"

var timePricing atomic.Pointer[pricingtime.Catalog]

// TimePricingAt matches the loaded immutable schedule at the supplied instant.
func TimePricingAt(model string, at time.Time) pricingtime.Adjustment {
	return timePricing.Load().Match(model, at)
}

func snapshotTimePricing() string {
	if catalog := timePricing.Load(); catalog != nil {
		return catalog.JSON()
	}
	return "{}"
}
