package model

// ReconciliationRecord persists the result of one daily reconciliation pass
// for one user. It enables audit reconstruction and difference tracking over
// time. Each run at 02:00 UTC creates one row per active user.
type ReconciliationRecord struct {
	ID             int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID         int64  `gorm:"index;not null" json:"user_id"`
	Date           string `gorm:"size:10;index;not null" json:"date"` // YYYY-MM-DD UTC
	LogQuotaSum    int64  `gorm:"not null;default:0" json:"log_quota_sum"`
	UsedQuotaDelta int64  `gorm:"not null;default:0" json:"used_quota_delta"`
	Diff           int64  `gorm:"not null;default:0" json:"diff"` // abs(LogQuotaSum - UsedQuotaDelta)
	Alerted        bool   `gorm:"not null;default:false" json:"alerted"`
	CreatedAt      int64  `gorm:"autoCreateTime" json:"created_at"`
}

// InsertReconciliationRecord persists one reconciliation result. It is
// best-effort: a failure is logged but does not abort the run.
func InsertReconciliationRecord(r *ReconciliationRecord) error {
	return DB.Create(r).Error
}

// GetLatestReconciliationForUser returns the most recent reconciliation
// record for the given user, or nil if none exists.
func GetLatestReconciliationForUser(userID int64) (*ReconciliationRecord, error) {
	var r ReconciliationRecord
	err := DB.Where("user_id = ?", userID).Order("id DESC").First(&r).Error
	if err != nil {
		return nil, err
	}
	return &r, nil
}
