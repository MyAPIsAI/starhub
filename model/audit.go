package model

// AuditLog records a sensitive (admin write) operation for later review.
// The audit middleware writes one row per matching request after the
// handler returns, regardless of the response status.
type AuditLog struct {
	ID        int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID    int64  `gorm:"index;not null;default:0" json:"user_id"`
	Username  string `gorm:"size:64;not null;default:''" json:"username"`
	Method    string `gorm:"size:8;not null;default:''" json:"method"`
	Path      string `gorm:"size:256;not null;default:''" json:"path"`
	Status    int    `gorm:"not null;default:0" json:"status"`
	IP        string `gorm:"size:64;not null;default:''" json:"ip"`
	CreatedAt int64  `gorm:"index;autoCreateTime" json:"created_at"`
}

func InsertAuditLog(entry *AuditLog) error {
	return DB.Create(entry).Error
}

func ListAuditLogs(page, pageSize int64) ([]AuditLog, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	if pageSize > 100 {
		pageSize = 100
	}
	q := DB.Model(&AuditLog{})
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var logs []AuditLog
	err := q.Order("id DESC").Limit(int(pageSize)).Offset(int((page - 1) * pageSize)).Find(&logs).Error
	return logs, total, err
}