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
	ID        int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	TaskID    string `gorm:"size:128;not null;default:'';index" json:"task_id"`
	Platform  string `gorm:"size:32;not null;default:''" json:"platform"`
	Action    string `gorm:"size:32;not null;default:''" json:"action"`
	Status    string `gorm:"size:32;not null;default:'submitted';index" json:"status"`
	Progress  int    `gorm:"not null;default:0" json:"progress"`
	Model     string `gorm:"size:128;not null;default:''" json:"model"`
	UserID    int64  `gorm:"index;not null;default:0" json:"user_id"`
	TokenID   int64  `gorm:"not null;default:0" json:"token_id"`
	TokenName string `gorm:"size:64;not null;default:''" json:"token_name"`
	// TokenUnlimited snapshots the token's unlimited flag at submit so the
	// terminal settlement (which may happen days later) still skips the
	// token balance correctly.
	TokenUnlimited bool   `gorm:"not null;default:false" json:"token_unlimited"`
	ChannelID      int64  `gorm:"not null;default:0" json:"channel_id"`
	Group          string `gorm:"size:64;not null;default:'default'" json:"group"`
	Quota          int64  `gorm:"not null;default:0" json:"quota"`
	RequestMeta    string `gorm:"type:text;not null;default:''" json:"request_meta"`
	ResultMeta     string `gorm:"type:text;not null;default:''" json:"result_meta"`
	FailReason     string `gorm:"size:512;not null;default:''" json:"fail_reason"`
	SubmitTime     int64  `gorm:"autoCreateTime" json:"submit_time"`
	UpdateTime     int64  `gorm:"autoUpdateTime" json:"update_time"`
	CompleteTime   int64  `gorm:"not null;default:0" json:"complete_time"`
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
	return DB.Model(&Task{}).Where("id = ? AND status NOT IN ?",
		id, []string{"succeeded", "failed", "cancelled"}).Updates(updates).Error
}

// ClaimTaskFinalize atomically moves an active task to a terminal status.
// Exactly one caller wins the claim (the poll endpoint and the background
// reaper race on the same task), so the terminal settlement runs once.
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
