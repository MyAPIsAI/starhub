package service

import (
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"time"

	"starhub/common"
	"starhub/model"
	"starhub/relay"
)

// ShadowBillingEntry is the per-record comparison result.
type ShadowBillingEntry struct {
	PriceVersion     int64  `json:"price_version"`
	Kind             string `json:"kind,omitempty"`
	Group            string `json:"group,omitempty"`
	Status           string `json:"status"`
	Reason           string `json:"reason,omitempty"`
	ChargedNano      string `json:"charged_nano"`
	UnfundedNano     string `json:"unfunded_nano"`
	StoredNano       string `json:"stored_nano"`
	RecomputedNano   string `json:"recomputed_nano,omitempty"`
	DiffNano         string `json:"diff_nano,omitempty"`
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
	AmountUnit            string               `json:"amount_unit"`
	Status                string               `json:"status"`
	VerifiedRecords       int                  `json:"verified_records"`
	UnverifiableRecords   int                  `json:"unverifiable_records"`
	TotalDiffNano         string               `json:"total_diff_nano"`
	TotalAbsoluteDiffNano string               `json:"total_absolute_diff_nano"`
	TotalRecords          int                  `json:"total_records"`
	RecordsWithDiff       int                  `json:"records_with_diff"`
	TotalDiff             int64                `json:"total_diff"`
	DiffPercent           float64              `json:"diff_percent"`
	Entries               []ShadowBillingEntry `json:"entries"`
	Since                 int64                `json:"since"`
	Until                 int64                `json:"until"`
}

// RunShadowBillingVerification replays frozen pricing and classified facts.
// Missing historical evidence is explicit, never replaced with current prices.
// No request content is read and no upstream request is issued.
func RunShadowBillingVerification(since int64, limit int) (*ShadowBillingReport, error) {
	if since <= 0 {
		since = time.Now().Add(-24 * time.Hour).Unix()
	}
	if limit <= 0 || limit > 10000 {
		limit = 1000
	}

	until := time.Now().Unix()
	logs, err := model.ListBillingAuditLogs(since, until, limit)
	if err != nil {
		return nil, fmt.Errorf("read logs: %w", err)
	}

	report := &ShadowBillingReport{
		AmountUnit: common.MoneyUnitNano, Status: "matched",
		Since:   since,
		Until:   until,
		Entries: []ShadowBillingEntry{},
	}

	total, absolute := new(big.Int), new(big.Int)
	for _, l := range logs {
		charged := common.QuotaMoney(l.Quota, l.QuotaRemainderNano)
		unfunded := common.QuotaMoney(l.UnfundedQuota, l.UnfundedQuotaRemainderNano)
		stored := new(big.Int).Add(charged, unfunded)
		entry := ShadowBillingEntry{
			LogID:            l.ID,
			Model:            l.Model,
			PromptTokens:     l.PromptTokens,
			CompletionTokens: l.CompletionTokens,
			StoredQuota:      shadowQuotaProjection(stored),
			PriceVersion:     l.PriceVersion, ChargedNano: charged.String(), UnfundedNano: unfunded.String(), StoredNano: stored.String(),
			CreatedAt: l.CreatedAt,
		}
		var facts struct {
			Kind  string                      `json:"kind"`
			Price struct{ UsingGroup string } `json:"price"`
		}
		_ = json.Unmarshal([]byte(l.BillingEvidence), &facts)
		entry.Kind, entry.Group = facts.Kind, facts.Price.UsingGroup
		report.TotalRecords++
		expected, err := relay.RecomputeBilledNano(&l)
		if err != nil {
			entry.Status, entry.Reason = "unverifiable", err.Error()
			report.UnverifiableRecords++
			report.Entries = append(report.Entries, entry)
			continue
		}
		amount, err := common.ParseNano(expected)
		if err != nil {
			return nil, fmt.Errorf("invalid recomputed amount for log %d", l.ID)
		}
		diff := new(big.Int).Sub(amount, stored)
		entry.RecomputedNano, entry.DiffNano = expected, diff.String()
		entry.RecomputedQuota, entry.Diff = shadowQuotaProjection(amount), shadowQuotaProjection(diff)
		report.VerifiedRecords++
		if diff.Sign() != 0 {
			entry.Status = "difference"
			report.RecordsWithDiff++
			total.Add(total, diff)
			absolute.Add(absolute, new(big.Int).Abs(diff))
			report.Entries = append(report.Entries, entry)
		}
	}
	report.TotalDiffNano, report.TotalAbsoluteDiffNano = total.String(), absolute.String()
	report.TotalDiff = shadowQuotaProjection(total)
	if report.VerifiedRecords > 0 {
		report.DiffPercent = float64(report.RecordsWithDiff) / float64(report.VerifiedRecords) * 100
	}
	if report.UnverifiableRecords > 0 {
		report.Status = "incomplete"
	}
	if report.TotalRecords == 0 {
		report.Status = "no_data"
	}
	if report.RecordsWithDiff > 0 {
		report.Status = "differences"
	}
	return report, nil
}

func shadowQuotaProjection(amount *big.Int) int64 {
	whole := new(big.Int).Quo(amount, big.NewInt(common.NanoPerLegacyQuota))
	if whole.IsInt64() {
		return whole.Int64()
	}
	if whole.Sign() < 0 {
		return math.MinInt64
	}
	return math.MaxInt64
}
