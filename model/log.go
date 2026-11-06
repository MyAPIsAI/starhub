package model

import "gorm.io/gorm"

type Log struct {
	AcceptedAt           int64  `gorm:"not null;default:0" json:"accepted_at"`
	SettledAt            int64  `gorm:"not null;default:0;index" json:"settled_at"`
	ResourceID           string `gorm:"size:128;not null;default:''" json:"resource_id,omitempty"`
	ResourceIDs          string `gorm:"type:text;not null;default:''" json:"resource_ids,omitempty"`
	SnapshotSequence     int64  `gorm:"not null;default:0" json:"snapshot_sequence"`
	ResourceRevision     int64  `gorm:"not null;default:0" json:"resource_revision"`
	AffinityUsed         bool   `gorm:"not null;default:false" json:"affinity_used"`
	RoutingRecorded      bool   `gorm:"not null;default:false" json:"routing_recorded"`
	ServiceGroup         string `gorm:"size:128;not null;default:'';index" json:"service_group"`
	Vendor               string `gorm:"size:128;not null;default:'';index" json:"vendor"`
	CallType             string `gorm:"size:32;not null;default:'';index" json:"call_type"`
	ErrorCode            string `gorm:"size:64;not null;default:''" json:"error_code,omitempty"`
	CacheReadTokens      int64  `gorm:"not null;default:0" json:"cache_read_tokens"`
	CacheWriteTokens     int64  `gorm:"not null;default:0" json:"cache_write_tokens"`
	BillingDetail        string `gorm:"type:text;not null;default:''" json:"billing_detail,omitempty"`
	TraceID              string `gorm:"size:64;index" json:"trace_id,omitempty"`
	RequestSource        string `gorm:"size:32;not null;default:''" json:"request_source,omitempty"`
	TTFTMs               int64  `gorm:"not null;default:0" json:"ttft_ms"`
	TokenPeriodStartedAt int64  `gorm:"not null;default:0" json:"token_period_started_at,omitempty"`
	TokenDeleted         bool   `gorm:"-" json:"token_deleted"`
	UsageSource          string `gorm:"size:32;not null;default:''" json:"usage_source,omitempty"`
	ID                   int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID               int64  `gorm:"index;not null;default:0" json:"user_id"`
	TokenID              int64  `gorm:"index;not null;default:0" json:"token_id"`
	TokenName            string `gorm:"size:64;not null;default:''" json:"token_name"`
	ChannelID            int64  `gorm:"index;not null;default:0" json:"channel_id"`
	Model                string `gorm:"size:128;not null;default:'';index:idx_logs_quality_window,priority:2;index:idx_logs_quality_model,priority:1" json:"model"`
	RequestedModel       string `gorm:"size:128;not null;default:''" json:"requested_model"`
	Protocol             string `gorm:"size:16;not null;default:''" json:"protocol"`
	PromptTokens         int64  `gorm:"not null;default:0" json:"prompt_tokens"`
	CompletionTokens     int64  `gorm:"not null;default:0" json:"completion_tokens"`
	Quota                int64  `gorm:"not null;default:0;index:idx_logs_quality_window,priority:5" json:"quota"`
	// UnfundedQuota is the priced usage that could not be collected. Quota
	// records the actual charge; their sum is the full priced usage.
	UnfundedQuota int64  `gorm:"not null;default:0" json:"unfunded_quota"`
	UsedChannels  string `gorm:"size:256;not null;default:''" json:"used_channels"`
	// DurationMs is the upstream response time in milliseconds. Zero
	// when not measured (legacy logs, or fast-path responses).
	DurationMs int64 `gorm:"not null;default:0;index:idx_logs_quality_window,priority:3" json:"duration_ms"`
	// Status is 1=success, 2=failed, 3=cancelled, independent of charge. Defaults to
	// 1 so existing log insertion code that omits the field is correct.
	Status    int   `gorm:"not null;default:1;index:idx_logs_quality_window,priority:4" json:"status"`
	CreatedAt int64 `gorm:"index;autoCreateTime;index:idx_logs_quality_window,priority:1;index:idx_logs_quality_model,priority:2" json:"created_at"`
}

func InsertLog(l *Log) error {
	return DB.Create(l).Error
}

// InsertFailedLog records a failed request without touching quota. It
// sets Status=2 (failed) and Quota=0. The entry is used for service
// quality and error-rate analytics. Only logged when channelID > 0
// to avoid polluting usage counts with pre-channel failures.
func InsertFailedLog(l *Log) error {
	if l == nil || l.ChannelID == 0 {
		return nil
	}
	if l.Status != 3 {
		l.Status = 2
	}
	l.Quota = 0
	l.UnfundedQuota = 0
	return DB.Create(l).Error
}

type DailyUsage struct {
	Date             string `json:"date"`
	Requests         int64  `json:"requests"`
	Quota            int64  `json:"quota"`
	UnfundedQuota    int64  `json:"unfunded_quota"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
	SuccessCount     int64  `json:"success_count"`
	FailedCount      int64  `json:"failed_count"`
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
	return regionalDailyUsage(userID, since)
}

// ModelDailyUsage is a per-day per-model usage summary.
type ModelDailyUsage struct {
	Date             string `json:"date"`
	Model            string `json:"model"`
	Requests         int64  `json:"requests"`
	Quota            int64  `json:"quota"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
}

// SummarizeLogsByModel returns daily usage broken down by model.
func SummarizeLogsByModel(userID int64, since int64) ([]ModelDailyUsage, error) {
	return regionalModelUsage(userID, since)
}

func ListLogs(userID int64, page, pageSize int64, model string) ([]Log, int64, error) {
	return ListResourceLogs(userID, page, pageSize, model, "")
}

// ListResourceLogs matches complete IDs in recorded routing attribution.
func ListResourceLogs(userID int64, page, pageSize int64, model, resourceID string) ([]Log, int64, error) {
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
	if resourceID != "" {
		q = q.Where(`resource_id = ? OR EXISTS (
			SELECT 1 FROM json_each(CASE WHEN json_valid(logs.resource_ids) THEN logs.resource_ids ELSE '[]' END)
			WHERE type = 'text' AND value = ?
		)`, resourceID, resourceID)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var logs []Log
	err := q.Order("id DESC").Limit(int(pageSize)).Offset(int((page - 1) * pageSize)).Find(&logs).Error
	if err == nil {
		err = MarkDeletedLogTokens(logs)
	}
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
	err := q.Order("id DESC").Find(&logs).Error
	if err == nil {
		err = MarkDeletedLogTokens(logs)
	}
	return logs, err
}

// MarkDeletedLogTokens preserves usage history without restoring deleted key names.
func MarkDeletedLogTokens(logs []Log) error { return markDeletedLogTokens(DB, logs) }

func markDeletedLogTokens(db *gorm.DB, logs []Log) error {
	ids := make([]int64, 0, len(logs))
	for _, entry := range logs {
		if entry.TokenID > 0 {
			ids = append(ids, entry.TokenID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	var existing []int64
	if err := db.Model(&Token{}).Where("id IN ?", ids).Pluck("id", &existing).Error; err != nil {
		return err
	}
	found := map[int64]bool{}
	for _, id := range existing {
		found[id] = true
	}
	for i := range logs {
		logs[i].TokenDeleted = logs[i].TokenID > 0 && !found[logs[i].TokenID]
	}
	return nil
}
