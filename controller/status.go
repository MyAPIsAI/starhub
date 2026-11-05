package controller

import (
	"context"
	"net/http"
	"strconv"

	"starhub/common"
	"starhub/metric"
	"starhub/model"
)

// StatusHistoryHandler handles GET /api/status/history?months=3 (public).
// Returns monthly availability aggregated from the Log table.
func StatusHistoryHandler(w http.ResponseWriter, r *http.Request) {
	months, _ := strconv.Atoi(r.URL.Query().Get("months"))
	if months <= 0 {
		months = 3
	}
	// Try persisted records first.
	records, err := model.ListAvailabilityHistory(months)
	if err == nil && len(records) > 0 {
		common.ApiSuccess(w, records)
		return
	}
	// Fall back to real-time aggregation from logs.
	records, err = model.AggregateMonthlyAvailability(months)
	if err != nil {
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.ApiSuccess(w, records)
}

// SystemStatusHandler handles GET /api/status/system (AdminAuth).
// Returns the resource catalog version, channel stats and routing state.
func SystemStatusHandler(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()

	// Resource catalog version and sync time.
	catalog, err := model.ReadResourceCatalog(ctx)
	catalogInfo := map[string]any{}
	if err == nil {
		catalogInfo["revision"] = catalog.Revision
		catalogInfo["updated_at"] = catalog.UpdatedAt
		catalogInfo["affects_gateway_routing"] = catalog.AffectsGatewayRouting
		if catalog.Snapshot != nil {
			catalogInfo["snapshot_instances"] = len(catalog.Snapshot.Instances)
			catalogInfo["snapshot_channels"] = len(catalog.Snapshot.Channels)
		}
		catalogInfo["placements"] = len(catalog.Placements)
		catalogInfo["regions"] = len(catalog.Regions)
	}

	// Channel stats from the in-memory metric registry.
	snap := metric.TakeSnapshot()
	channelStats := make([]map[string]any, 0, len(snap.ChannelStats))
	for _, cs := range snap.ChannelStats {
		channelStats = append(channelStats, map[string]any{
			"channel_id":   cs.ChannelID,
			"samples":      cs.Samples,
			"success_rate": cs.SuccessRate,
			"avg_ttft_ms":  cs.AvgTTFTMs,
			"avg_latency":  cs.AvgLatencyMs,
		})
	}

	// Request counters.
	requests := make([]map[string]any, 0, len(snap.Requests))
	for _, req := range snap.Requests {
		requests = append(requests, map[string]any{
			"class": req.Class,
			"count": req.Count,
		})
	}

	// Channel list with status.
	channels, _ := model.ListChannels()
	channelList := make([]map[string]any, 0, len(channels))
	for _, ch := range channels {
		status := "enabled"
		if ch.Status == common.ChannelStatusManuallyDisabled {
			status = "manually_disabled"
		} else if ch.Status == common.ChannelStatusAutoDisabled {
			status = "auto_disabled"
		}
		channelList = append(channelList, map[string]any{
			"id":            ch.ID,
			"name":          ch.Name,
			"status":        status,
			"response_time": ch.ResponseTime,
			"models":        ch.Models,
		})
	}

	common.ApiSuccess(w, map[string]any{
		"resource_catalog": catalogInfo,
		"channel_stats":    channelStats,
		"request_counts":   requests,
		"channels":         channelList,
		"generated_at":     snap.GeneratedAt,
	})
}
