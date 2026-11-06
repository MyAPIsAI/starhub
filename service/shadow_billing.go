package service

import (
	"fmt"
	"time"

	"starhub/model"
	"starhub/relay"
)

// ShadowBillingEntry is the per-record comparison result.
type ShadowBillingEntry struct {
	LogID            int64  `json:"log_id"`
	Model            string `json:"model"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
	StoredQuota      int64  `json:"stored_quota"`
	RecomputedQuota  int64  `json:"recomputed_quota"`
	Diff             int64  `json:"diff"`
	CreatedAt        int64  `json:"created_at"`
}

// ShadowBillingReport is the aggregated verification result.
type ShadowBillingReport struct {
	TotalRecords    int                  `json:"total_records"`
	RecordsWithDiff int                  `json:"records_with_diff"`
	TotalDiff       int64                `json:"total_diff"`
	DiffPercent     float64              `json:"diff_percent"`
	Entries         []ShadowBillingEntry `json:"entries"`
	Since           int64                `json:"since"`
	Until           int64                `json:"until"`
}

// RunShadowBillingVerification reads text chat logs from the given time
// range, recomputes the quota using current pricing, and compares with
// the stored Quota. It does NOT save request bodies, does NOT call
// upstream, and only uses metadata already in the Log table.
func RunShadowBillingVerification(since int64, limit int) (*ShadowBillingReport, error) {
	if since <= 0 {
		since = time.Now().Add(-24 * time.Hour).Unix()
	}
	if limit <= 0 || limit > 10000 {
		limit = 1000
	}

	logs, err := model.ListLogsSince(0, since)
	if err != nil {
		return nil, fmt.Errorf("read logs: %w", err)
	}

	report := &ShadowBillingReport{
		Since:   since,
		Until:   time.Now().Unix(),
		Entries: []ShadowBillingEntry{},
	}

	checked := 0
	for _, l := range logs {
		if checked >= limit {
			break
		}
		// Only verify text chat records with token usage.
		if l.PromptTokens+l.CompletionTokens == 0 || l.Quota == 0 {
			continue
		}

		// Recompute using current pricing (best-effort — historical
		// price versions may differ).
		usage := &relay.Usage{
			PromptTokens:     l.PromptTokens,
			CompletionTokens: l.CompletionTokens,
		}
		pd := relay.NewPriceData(l.Model, "default")
		recomputed := relay.ComputeUsageQuota(pd, usage)

		diff := recomputed - l.Quota
		entry := ShadowBillingEntry{
			LogID:            l.ID,
			Model:            l.Model,
			PromptTokens:     l.PromptTokens,
			CompletionTokens: l.CompletionTokens,
			StoredQuota:      l.Quota,
			RecomputedQuota:  recomputed,
			Diff:             diff,
			CreatedAt:        l.CreatedAt,
		}

		report.TotalRecords++
		if diff != 0 {
			report.RecordsWithDiff++
			report.TotalDiff += diff
			report.Entries = append(report.Entries, entry)
		}
		checked++
	}

	if report.TotalRecords > 0 {
		report.DiffPercent = float64(report.RecordsWithDiff) / float64(report.TotalRecords) * 100
	}

	return report, nil
}
