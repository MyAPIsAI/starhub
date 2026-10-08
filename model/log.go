package model

type Log struct {
	ID               int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID           int64  `gorm:"index;not null;default:0" json:"user_id"`
	TokenID          int64  `gorm:"index;not null;default:0" json:"token_id"`
	TokenName        string `gorm:"size:64;not null;default:''" json:"token_name"`
	ChannelID        int64  `gorm:"index;not null;default:0" json:"channel_id"`
	Model            string `gorm:"size:128;not null;default:''" json:"model"`
	PromptTokens     int64  `gorm:"not null;default:0" json:"prompt_tokens"`
	CompletionTokens int64  `gorm:"not null;default:0" json:"completion_tokens"`
	Quota            int64  `gorm:"not null;default:0" json:"quota"`
	// UnfundedQuota is the priced usage that could not be collected. Quota
	// records the actual charge; their sum is the full priced usage.
	UnfundedQuota int64  `gorm:"not null;default:0" json:"unfunded_quota"`
	UsedChannels  string `gorm:"size:256;not null;default:''" json:"used_channels"`
	CreatedAt     int64  `gorm:"index;autoCreateTime" json:"created_at"`
}

func InsertLog(l *Log) error {
	return DB.Create(l).Error
}

type DailyUsage struct {
	Date             string `json:"date"`
	Requests         int64  `json:"requests"`
	Quota            int64  `json:"quota"`
	UnfundedQuota    int64  `json:"unfunded_quota"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
}

func CountLogs(userID int64) (int64, error) {
	q := DB.Model(&Log{})
	if userID > 0 {
		q = q.Where("user_id = ?", userID)
	}
	var total int64
	err := q.Count(&total).Error
	return total, err
}

func SummarizeLogsByDay(userID int64, since int64) ([]DailyUsage, error) {
	q := DB.Model(&Log{}).
		Select("date(created_at, 'unixepoch') AS date, COUNT(*) AS requests, "+
			"SUM(quota) AS quota, SUM(unfunded_quota) AS unfunded_quota, SUM(prompt_tokens) AS prompt_tokens, "+
			"SUM(completion_tokens) AS completion_tokens").
		Where("created_at >= ?", since)
	if userID > 0 {
		q = q.Where("user_id = ?", userID)
	}
	rows := make([]DailyUsage, 0)
	err := q.Group("date(created_at, 'unixepoch')").Order("date ASC").Scan(&rows).Error
	return rows, err
}

func ListLogs(userID int64, page, pageSize int64, model string) ([]Log, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	if pageSize > 100 {
		pageSize = 100
	}
	q := DB.Model(&Log{})
	if userID > 0 {
		q = q.Where("user_id = ?", userID)
	}
	if model != "" {
		q = q.Where("model = ?", model)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var logs []Log
	err := q.Order("id DESC").Limit(int(pageSize)).Offset(int((page - 1) * pageSize)).Find(&logs).Error
	return logs, total, err
}

// ListLogsSince returns all logs since the given unix timestamp for a
// user (or all users when userID == 0), ordered newest first. Used by
// the export feature to generate CSV files without pagination limits.
func ListLogsSince(userID int64, since int64) ([]Log, error) {
	q := DB.Model(&Log{}).Where("created_at >= ?", since)
	if userID > 0 {
		q = q.Where("user_id = ?", userID)
	}
	var logs []Log
	err := q.Order("id DESC").Limit(10000).Find(&logs).Error
	return logs, err
}
