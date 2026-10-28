package model

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
)

var ErrTaskNotFound = errors.New("task not found")

// Task is one asynchronous platform job (video generation, music, ...)
// following the submit-and-poll lifecycle of P3.3. Quota holds the
// pre-charge reserved at submit; the terminal poll settles the difference.
type Task struct {
	ID             int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	TaskID         string `gorm:"size:128;not null;default:'';index" json:"task_id"`
	Platform       string `gorm:"size:32;not null;default:''" json:"platform"`
	Action         string `gorm:"size:32;not null;default:''" json:"action"`
	Status         string `gorm:"size:32;not null;default:'submitted';index" json:"status"`
	Progress       int    `gorm:"not null;default:0" json:"progress"`
	Model          string `gorm:"size:128;not null;default:''" json:"model"`
	RequestedModel string `gorm:"size:128;not null;default:''" json:"requested_model"`
	Protocol       string `gorm:"size:16;not null;default:''" json:"protocol"`
	UserID         int64  `gorm:"index;not null;default:0" json:"user_id"`
	TokenID        int64  `gorm:"not null;default:0" json:"token_id"`
	TokenName      string `gorm:"size:64;not null;default:''" json:"token_name"`
	// TokenUnlimited snapshots the token's unlimited flag at submit so the
	// terminal settlement (which may happen days later) still skips the
	// token balance correctly.
	TokenUnlimited bool   `gorm:"not null;default:false" json:"token_unlimited"`
	ChannelID      int64  `gorm:"not null;default:0" json:"channel_id"`
	Group          string `gorm:"size:64;not null;default:'default'" json:"group"`
	// UserGroup snapshots the user's own group at submit so the terminal
	// settlement (which may happen days later) still applies the group x
	// group matrix multiplier correctly.
	UserGroup    string `gorm:"size:64;not null;default:''" json:"user_group"`
	Quota        int64  `gorm:"not null;default:0" json:"quota"`
	RequestMeta  string `gorm:"type:text;not null;default:''" json:"request_meta"`
	PricingMeta  string `gorm:"type:text;not null;default:''" json:"pricing_meta"`
	ResultMeta   string `gorm:"type:text;not null;default:''" json:"result_meta"`
	FailReason   string `gorm:"size:512;not null;default:''" json:"fail_reason"`
	SubmitTime   int64  `gorm:"autoCreateTime" json:"submit_time"`
	UpdateTime   int64  `gorm:"autoUpdateTime" json:"update_time"`
	CompleteTime int64  `gorm:"not null;default:0" json:"complete_time"`
	// Pending settlement is a durable intent. It survives process interruption
	// and upstream result expiry without committing a terminal task prematurely.
	PendingStatus     string `gorm:"size:32;not null;default:''" json:"-"`
	PendingQuota      int64  `gorm:"not null;default:0" json:"-"`
	PendingFailReason string `gorm:"size:512;not null;default:''" json:"-"`
	PendingResultMeta string `gorm:"type:text;not null;default:''" json:"-"`
}

func InsertTask(t *Task) error {
	return DB.Create(t).Error
}

func GetTaskByID(id int64) (*Task, error) {
	var t Task
	err := DB.First(&t, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrTaskNotFound
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// ListPendingTasks returns up to limit non-terminal tasks (oldest first)
// for the background poller.
func ListPendingTasks(limit int) ([]Task, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	var tasks []Task
	err := DB.Where("status NOT IN ?", []string{"succeeded", "failed", "cancelled"}).
		Order("id ASC").Limit(limit).Find(&tasks).Error
	return tasks, err
}

// UpdateTaskProgress refreshes the mutable state of a still-active task.
func UpdateTaskProgress(id int64, status string, progress int, resultMeta string) error {
	updates := map[string]any{"update_time": time.Now().Unix()}
	if status != "" {
		updates["status"] = status
	}
	if progress >= 0 {
		updates["progress"] = progress
	}
	if resultMeta != "" {
		updates["result_meta"] = resultMeta
	}
	return DB.Model(&Task{}).Where("id = ? AND pending_status = '' AND status NOT IN ?",
		id, []string{"succeeded", "failed", "cancelled"}).Updates(updates).Error
}

// RecordTaskSettlement freezes the first terminal result and its price before
// settlement. Concurrent pollers cannot replace an already-recorded intent.
func RecordTaskSettlement(id int64, status, reason, result string, actual int64, progress ...int) (*Task, error) {
	if actual < 0 || (status != "succeeded" && status != "failed" && status != "cancelled") {
		return nil, errors.New("invalid task settlement intent")
	}
	updates := map[string]any{
		"pending_status": status, "pending_quota": actual,
		"pending_fail_reason": truncateForColumn(reason, 512), "pending_result_meta": result,
		"update_time": time.Now().Unix(),
	}
	if len(progress) > 0 {
		updates["progress"] = min(100, max(0, progress[0]))
	}
	err := DB.Model(&Task{}).Where("id = ? AND pending_status = '' AND status NOT IN ?",
		id, []string{"succeeded", "failed", "cancelled"}).Updates(updates).Error
	if err != nil {
		return nil, err
	}
	return GetTaskByID(id)
}

var errTaskAlreadySettled = errors.New("task already settled")

// SettleTaskQuota commits the terminal state, refund/charge, counters and log
// together. On failure the durable intent stays pending for a later retry.
func SettleTaskQuota(task *Task, entry *Log) (bool, error) {
	if task.PendingQuota < 0 || (task.PendingStatus != "succeeded" && task.PendingStatus != "failed" && task.PendingStatus != "cancelled") {
		return false, errors.New("task has no settlement intent")
	}
	err := settleQuota(task.UserID, task.TokenID, task.TokenUnlimited,
		task.PendingQuota-task.Quota, task.PendingQuota, entry, func(tx *gorm.DB) error {
			res := tx.Model(&Task{}).Where("id = ? AND pending_status = ? AND pending_quota = ? AND status NOT IN ?",
				task.ID, task.PendingStatus, task.PendingQuota, []string{"succeeded", "failed", "cancelled"}).Updates(map[string]any{
				"status": task.PendingStatus, "progress": task.Progress, "fail_reason": task.PendingFailReason,
				"result_meta": task.PendingResultMeta, "complete_time": time.Now().Unix(), "update_time": time.Now().Unix(),
				"pending_status": "", "pending_quota": 0, "pending_fail_reason": "", "pending_result_meta": "",
			})
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				return errTaskAlreadySettled
			}
			return nil
		})
	if errors.Is(err, errTaskAlreadySettled) {
		return false, nil
	}
	return err == nil, err
}

// ClaimTaskFinalize atomically moves an active task to a terminal status.
// This state-only helper does not settle money. Monetary finalization must use
// RecordTaskSettlement and SettleTaskQuota to keep the terminal state atomic.
func ClaimTaskFinalize(id int64, status, failReason, resultMeta string) (bool, error) {
	updates := map[string]any{
		"status":        status,
		"complete_time": time.Now().Unix(),
		"update_time":   time.Now().Unix(),
	}
	if failReason != "" {
		updates["fail_reason"] = truncateForColumn(failReason, 512)
	}
	if resultMeta != "" {
		updates["result_meta"] = resultMeta
	}
	res := DB.Model(&Task{}).
		Where("id = ? AND status NOT IN ?", id, []string{"succeeded", "failed", "cancelled"}).
		Updates(updates)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

func truncateForColumn(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}

// TaskOutputURLs extracts the output URLs of the stored result meta (best
// effort: the view layer only).
func TaskOutputURLs(resultMeta string) []string {
	if !strings.Contains(resultMeta, "outputs") {
		return nil
	}
	var parsed struct {
		Outputs []string `json:"outputs"`
	}
	if err := json.Unmarshal([]byte(resultMeta), &parsed); err != nil {
		return nil
	}
	return parsed.Outputs
}
