package model

import "time"

// admissionDayColumn is a SQL expression that resolves to the effective
// day for grouping. Logs written after T30 populate admission_day with
// the UTC midnight of the admission (pre-charge) time; legacy logs with
// admission_day == 0 fall back to created_at (completion time).
const admissionDayColumn = "CASE WHEN admission_day > 0 THEN admission_day ELSE created_at END"

// utcMidnight returns the Unix timestamp of the current UTC midnight.
func utcMidnight() int64 {
	now := time.Now().UTC()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).Unix()
}

type Log struct {
	AmountUnit                 string `gorm:"size:24;not null;default:''" json:"amount_unit,omitempty"`
	QuotaRemainderNano         int64  `gorm:"not null;default:0" json:"-"`
	UnfundedQuotaRemainderNano int64  `gorm:"not null;default:0" json:"-"`
	RoutingFallback            bool   `gorm:"not null;default:false" json:"routing_fallback"`
	ActualVendor               string `gorm:"size:128;not null;default:''" json:"actual_vendor"`
	UsageSource                string `gorm:"size:32;not null;default:''" json:"usage_source,omitempty"`
	ID                         int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID                     int64  `gorm:"index;not null;default:0" json:"user_id"`
	TokenID                    int64  `gorm:"index;not null;default:0" json:"token_id"`
	TokenName                  string `gorm:"size:64;not null;default:''" json:"token_name"`
	ChannelID                  int64  `gorm:"index;not null;default:0" json:"channel_id"`
	Model                      string `gorm:"size:128;not null;default:'';index:idx_logs_quality_window,priority:2;index:idx_logs_quality_model,priority:1" json:"model"`
	RequestedModel             string `gorm:"size:128;not null;default:''" json:"requested_model"`
	Protocol                   string `gorm:"size:16;not null;default:''" json:"protocol"`
	RequestID                  string `gorm:"size:64;not null;default:''" json:"request_id"`
	UpstreamRequestID          string `gorm:"size:64;not null;default:''" json:"upstream_request_id"`
	PromptTokens               int64  `gorm:"not null;default:0" json:"prompt_tokens"`
	CompletionTokens           int64  `gorm:"not null;default:0" json:"completion_tokens"`
	Quota                      int64  `gorm:"not null;default:0;index:idx_logs_quality_window,priority:5" json:"quota"`
	// UnfundedQuota is the priced usage that could not be collected. Quota
	// records the actual charge; their sum is the full priced usage.
	UnfundedQuota int64  `gorm:"not null;default:0" json:"unfunded_quota"`
	UsedChannels  string `gorm:"size:256;not null;default:''" json:"used_channels"`
	// DurationMs is the upstream response time in milliseconds. Zero
	// when not measured (legacy logs, or fast-path responses).
	DurationMs int64 `gorm:"not null;default:0;index:idx_logs_quality_window,priority:3" json:"duration_ms"`
	// Status is 1=success, 2=failed, 3=cancelled, independent of charge. Defaults to
	// 1 so existing log insertion code that omits the field is correct.
	Status int `gorm:"not null;default:1;index:idx_logs_quality_window,priority:4" json:"status"`
	// AdmissionDay is the UTC date (unix midnight) when the request was
	// admitted (pre-charged), used for cross-day settlement attribution.
	// Streaming requests that span midnight are attributed to the admission
	// day, not the completion day. Zero means legacy logs without admission day.
	AdmissionDay       int64  `gorm:"not null;default:0;index" json:"admission_day"`
	AdmittedAt         int64  `gorm:"not null;default:0;index" json:"admitted_at"`
	QuotaPeriodID      int64  `gorm:"not null;default:0;index" json:"quota_period_id"`
	QuotaReservationID string `gorm:"size:64;not null;default:'';index" json:"quota_reservation_id,omitempty"`
	// PriceVersion records the option revision at billing time, enabling
	// offline recomputation. Zero means legacy logs without version tracking.
	PriceVersion int64 `gorm:"not null;default:0" json:"price_version"`
	// ExactFeeMicroUSD is the precise fee in micro-USD (1 USD = 1,000,000)
	// before rounding to quota. Enables audit reconstruction.
	ExactFeeMicroUSD int64  `gorm:"not null;default:0" json:"exact_fee_micro_usd"`
	ExactFeeNanoUSD  string `gorm:"not null;default:''" json:"exact_fee_nano_usd"`
	CreatedAt        int64  `gorm:"index;autoCreateTime;index:idx_logs_quality_window,priority:1;index:idx_logs_quality_model,priority:2" json:"created_at"`
}

func InsertLog(l *Log) error {
	if l.AdmissionDay == 0 {
		if l.CreatedAt > 0 {
			// Derive admission day from the explicit CreatedAt value
			// (used by tests and back-dated entries).
			t := time.Unix(l.CreatedAt, 0).UTC()
			l.AdmissionDay = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC).Unix()
		} else {
			l.AdmissionDay = utcMidnight()
		}
	}
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
	if l.QuotaReservationID != "" {
		var reservation QuotaReservation
		if err := DB.First(&reservation, "id = ? AND user_id = ? AND token_id = ?", l.QuotaReservationID, l.UserID, l.TokenID).Error; err != nil {
			return err
		}
		l.AdmittedAt, l.QuotaPeriodID = reservation.AdmittedAt, reservation.PeriodID
		at := time.Unix(reservation.AdmittedAt, 0).UTC()
		l.AdmissionDay = time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, time.UTC).Unix()
	}
	l.Status = 2 // failed
	l.Quota = 0
	l.UnfundedQuota = 0
	if l.AdmissionDay == 0 {
		if l.CreatedAt > 0 {
			t := time.Unix(l.CreatedAt, 0).UTC()
			l.AdmissionDay = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC).Unix()
		} else {
			l.AdmissionDay = utcMidnight()
		}
	}
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
	q := DB.Model(&Log{}).
		Select("date("+admissionDayColumn+", 'unixepoch') AS date, COUNT(*) AS requests, "+
			"SUM(quota) AS quota, SUM(unfunded_quota) AS unfunded_quota, SUM(prompt_tokens) AS prompt_tokens, "+
			"SUM(completion_tokens) AS completion_tokens, "+
			"SUM(CASE WHEN status = 1 THEN 1 ELSE 0 END) AS success_count, "+
			"SUM(CASE WHEN status IN (2, 3) THEN 1 ELSE 0 END) AS failed_count").
		Where("created_at >= ?", since)
	if userID > 0 {
		q = q.Where("user_id = ?", userID)
	}
	rows := make([]DailyUsage, 0)
	err := q.Group("date(" + admissionDayColumn + ", 'unixepoch')").Order("date ASC").Scan(&rows).Error
	return rows, err
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
	q := DB.Model(&Log{}).
		Select("date("+admissionDayColumn+", 'unixepoch') AS date, model AS model, "+
			"COUNT(*) AS requests, SUM(quota) AS quota, "+
			"SUM(prompt_tokens) AS prompt_tokens, SUM(completion_tokens) AS completion_tokens").
		Where("created_at >= ?", since)
	if userID > 0 {
		q = q.Where("user_id = ?", userID)
	}
	var rows []ModelDailyUsage
	err := q.Group("date(" + admissionDayColumn + ", 'unixepoch'), model").Order("date ASC, model ASC").Scan(&rows).Error
	return rows, err
}

// TokenDailyUsage is a per-day per-token usage summary for the
// operational per-key consumption view.
type TokenDailyUsage struct {
	Date             string `json:"date"`
	TokenID          int64  `json:"token_id"`
	TokenName        string `json:"token_name"`
	Requests         int64  `json:"requests"`
	Quota            int64  `json:"quota"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
}

// SummarizeLogsByToken returns daily usage broken down by token (API key).
// When tokenID > 0 only that token's logs are aggregated; when tokenID == 0
// all tokens for the user are included. The result is ordered by date then
// token id for stable display.
func SummarizeLogsByToken(userID int64, tokenID int64, since int64) ([]TokenDailyUsage, error) {
	q := DB.Model(&Log{}).
		Select("date("+admissionDayColumn+", 'unixepoch') AS date, token_id AS token_id, "+
			"MAX(token_name) AS token_name, "+
			"COUNT(*) AS requests, SUM(quota) AS quota, "+
			"SUM(prompt_tokens) AS prompt_tokens, SUM(completion_tokens) AS completion_tokens").
		Where("created_at >= ?", since)
	if userID > 0 {
		q = q.Where("user_id = ?", userID)
	}
	if tokenID > 0 {
		q = q.Where("token_id = ?", tokenID)
	}
	var rows []TokenDailyUsage
	err := q.Group("date(" + admissionDayColumn + ", 'unixepoch'), token_id").
		Order("date ASC, token_id ASC").Scan(&rows).Error
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
