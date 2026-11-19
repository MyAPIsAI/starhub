package model

import (
	"context"
	"slices"
	"time"

	"starhub/setting"
)

// PublicStatusMetric deliberately omits customer, channel and billing fields.
type PublicStatusMetric struct {
	TotalRequests int64    `json:"total_requests"`
	SuccessCount  int64    `json:"success_count"`
	FailedCount   int64    `json:"failed_count"`
	SuccessRate   *float64 `json:"success_rate"`
	AvgDurationMS *int64   `json:"avg_duration_ms"`
}

type PublicStatusDay struct {
	Date string `json:"date"`
	PublicStatusMetric
}

type PublicStatusModel struct {
	ID            string             `json:"id"`
	Vendor        string             `json:"vendor"`
	VendorDisplay string             `json:"vendor_display"`
	Available     bool               `json:"available"`
	State         string             `json:"state"`
	Recent        PublicStatusMetric `json:"recent"`
	History       PublicStatusMetric `json:"history"`
	Days          []PublicStatusDay  `json:"days"`
}

type PublicStatusOverview struct {
	GeneratedAt  int64               `json:"generated_at"`
	WindowStart  int64               `json:"window_start"`
	WindowEnd    int64               `json:"window_end"`
	HistoryStart string              `json:"history_start"`
	Timezone     string              `json:"timezone"`
	State        string              `json:"state"`
	Models       []PublicStatusModel `json:"models"`
}

type publicStatusScope struct {
	ChannelID int64
	Model     string
	Group     string
	Available bool
}

func ReadPublicStatusOverview(ctx context.Context) (*PublicStatusOverview, error) {
	view, scope, err := readPublicStatusCatalog(ctx, time.Now().UTC())
	if err == nil {
		err = populatePublicStatusMetrics(ctx, view, scope)
	}
	return view, err
}

func readPublicStatusCatalog(ctx context.Context, now time.Time) (*PublicStatusOverview, []publicStatusScope, error) {
	now = now.UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	view := &PublicStatusOverview{GeneratedAt: now.Unix(), WindowStart: now.Add(-24 * time.Hour).Unix(), WindowEnd: now.Unix() + 1,
		HistoryStart: today.AddDate(0, 0, -89).Format("2006-01-02"), Timezone: "UTC", State: "unknown", Models: []PublicStatusModel{}}
	var rows []publicStatusScope
	err := DB.WithContext(ctx).Table("abilities AS a").
		Select(`a.channel_id, a.model, a."group", CASE WHEN a.enabled = TRUE AND c.status = 1 AND ` + resourceGatewayAbilityCondition("a") + ` THEN 1 ELSE 0 END AS available`).
		Joins("JOIN channels c ON c.id = a.channel_id").Where(resourceGatewayGrantCondition("a")).
		Order("a.model, a.channel_id").Scan(&rows).Error
	if err != nil {
		return nil, nil, err
	}
	groups := make([]string, 0, len(rows))
	for _, row := range rows {
		groups = append(groups, row.Group)
	}
	access, err := ServiceGroupAccess(ctx, groups, 0)
	if err != nil {
		return nil, nil, err
	}
	metadata, err := ListModelMeta()
	if err != nil {
		return nil, nil, err
	}
	region, err := ActiveRegionProfile()
	if err != nil {
		return nil, nil, err
	}
	indexes := map[string]int{}
	scope := []publicStatusScope{}
	for _, row := range rows {
		meta := metadata[row.Model]
		if !access[row.Group] || !setting.ContainsGroupRatio(row.Group) || !region.ModelVisible(row.Model, meta) {
			continue
		}
		if meta != nil && len(meta.Groups()) > 0 && !slices.Contains(meta.Groups(), row.Group) {
			continue
		}
		scope = append(scope, row)
		index, exists := indexes[row.Model]
		if !exists {
			vendor, display := InferVendor(row.Model)
			if meta != nil {
				if meta.Vendor != "" {
					vendor = meta.Vendor
				}
				if meta.VendorDisplay != "" {
					display = meta.VendorDisplay
				}
			}
			index = len(view.Models)
			indexes[row.Model] = index
			view.Models = append(view.Models, PublicStatusModel{ID: row.Model, Vendor: vendor, VendorDisplay: display, State: "unknown", Days: []PublicStatusDay{}})
		}
		view.Models[index].Available = view.Models[index].Available || row.Available
	}
	for i := range view.Models {
		if !view.Models[i].Available {
			view.Models[i].State = "unavailable"
		}
	}
	return view, scope, nil
}
