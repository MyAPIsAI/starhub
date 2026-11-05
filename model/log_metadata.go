package model

// FreezeLogLabels preserves the labels used by a newly accepted settlement.
// It never reconstructs a historical tariff from current configuration.
func FreezeLogLabels(entry *Log, group, callType string) {
	entry.ServiceGroup, entry.CallType = group, callType
	_, entry.Vendor = InferVendor(entry.Model)
	if meta, found, err := GetModelMeta(entry.Model); err == nil && found {
		if meta.VendorDisplay != "" {
			entry.Vendor = meta.VendorDisplay
		} else if meta.Vendor != "" {
			entry.Vendor = meta.Vendor
		}
	}
}

type BillingPart struct {
	Name         string `json:"name"`
	Quantity     string `json:"quantity"`
	Unit         string `json:"unit"`
	UnitPriceUSD string `json:"unit_price_usd"`
}
type BillingDetail struct {
	Version         int           `json:"version"`
	Mode            string        `json:"mode"`
	PricedAt        string        `json:"priced_at"`
	GroupMultiplier string        `json:"group_multiplier"`
	Formula         string        `json:"formula,omitempty"`
	Parts           []BillingPart `json:"parts"`
}
