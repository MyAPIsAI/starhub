package model

// ListBillingAuditLogs bounds the read before materializing evidence and keeps
// fractional and zero-token paid records eligible for verification.
func ListBillingAuditLogs(since, until int64, limit int) ([]Log, error) {
	if limit <= 0 || limit > 10000 {
		limit = 1000
	}
	rows := []Log{}
	err := DB.Where("created_at >= ? AND created_at <= ?", since, until).
		Where("prompt_tokens > 0 OR completion_tokens > 0 OR quota <> 0 OR quota_remainder_nano <> 0 OR unfunded_quota <> 0 OR unfunded_quota_remainder_nano <> 0 OR billing_evidence <> ''").
		Order("id DESC").Limit(limit).Find(&rows).Error
	return rows, err
}
