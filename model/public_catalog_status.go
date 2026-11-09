package model

import (
	"context"
	"math"
	"slices"
	"time"
)

type CatalogStatus struct {
	State         string   `json:"state"`
	Samples       int64    `json:"samples"`
	SuccessRate   *float64 `json:"success_rate"`
	TTFTP95MS     *int64   `json:"ttft_p95_ms"`
	WindowMinutes int      `json:"window_minutes"`
	At            int64    `json:"at"`
}

// Public aggregates contain no customer, key, trace or raw request data.
func PublicCatalogStatus(ctx context.Context, name string, tags []string, at time.Time) (CatalogStatus, error) {
	out := CatalogStatus{State: "insufficient_data", WindowMinutes: 15, At: at.Unix()}
	catalog, err := ReadServiceGroupCatalog(ctx)
	if err != nil {
		return out, err
	}
	groups := StatusPageGroups(catalog)
	ids := []string{}
	var latencyLimit int64
	for _, g := range groups {
		ids = append(ids, g.GroupID)
		if g.SLAP95MS != nil && *g.SLAP95MS > latencyLimit {
			latencyLimit = *g.SLAP95MS
		}
	}
	if len(ids) == 0 {
		return out, nil
	}
	field := "model"
	if name == "veyra-auto" || name == "myapis-auto" {
		field = "requested_model"
	}
	names := []string{name}
	if name == "veyra-auto" || name == "myapis-auto" {
		names = []string{"myapis-auto", "veyra-auto"}
	}
	// Only classified platform/upstream failures enter the public denominator.
	// Unclassified historical failures and customer cancellations are not evidence.

	base := DB.WithContext(ctx).Model(&Log{}).Where(field+" IN ? AND service_group IN ? AND created_at >= ? AND created_at <= ?", names, ids, at.Add(-15*time.Minute).Unix(), at.Unix()).Where("status = 1 OR (status = 2 AND error_code IN ?)", statusFailureCodes)
	var totals struct {
		Total   int64
		Success int64
	}
	if err := base.Select("COUNT(*) AS total, COALESCE(SUM(CASE WHEN status=1 THEN 1 ELSE 0 END),0) AS success").Scan(&totals).Error; err != nil {
		return out, err
	}
	out.Samples = totals.Total
	if totals.Total < 50 {
		return out, nil
	}
	rate := float64(totals.Success) / float64(totals.Total)
	out.SuccessRate = &rate
	out.State = "normal"
	if rate < 0.95 {
		out.State = "outage"
	} else if rate < 0.99 {
		out.State = "degraded"
	}
	if !slices.Contains(tags, "text") {
		return out, nil
	}
	latencies := DB.WithContext(ctx).Model(&Log{}).Where(field+" IN ? AND service_group IN ? AND created_at >= ? AND created_at <= ? AND status = 1 AND prompt_tokens <= 2000 AND ttft_ms > 0 AND call_type IN ?", names, ids, at.Add(-15*time.Minute).Unix(), at.Unix(), []string{"", "chat", "responses"})
	var count int64
	if err := latencies.Count(&count).Error; err != nil {
		return out, err
	}
	if count >= 50 {
		var value struct{ TTFTMs int64 }
		if err := latencies.Select("ttft_ms").Order("ttft_ms").Offset(int(math.Ceil(float64(count)*0.95)) - 1).Limit(1).Scan(&value).Error; err != nil {
			return out, err
		}
		out.TTFTP95MS = &value.TTFTMs
		if latencyLimit > 0 && value.TTFTMs > latencyLimit && out.State == "normal" {
			out.State = "degraded"
		}
	}
	return out, nil
}
