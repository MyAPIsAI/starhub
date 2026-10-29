package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"gorm.io/gorm"
	"starhub/common"
	"starhub/pkg/taskplugin"
	"starhub/relay/channel"
)

var ErrTaskPluginConfiguration = errors.New("invalid task plugin channel configuration")
var ErrTaskPluginChannelBusy = errors.New("unfinished plugin tasks prevent changing channel credentials, type, base URL or deleting the channel")
var ErrTaskPluginChannelChanged = errors.New("plugin channel configuration changed before admission")

const TaskStatusSubmitting = "submitting"

type TaskPlugin struct {
	ID         int64  `gorm:"primaryKey" json:"id"`
	Key        string `gorm:"size:30;not null;uniqueIndex:idx_task_plugin_version" json:"key"`
	Version    string `gorm:"size:64;not null;uniqueIndex:idx_task_plugin_version" json:"version"`
	APIVersion int    `gorm:"not null" json:"api_version"`
	Source     string `gorm:"type:text;not null" json:"-"`
	SourceHash string `gorm:"size:64;not null" json:"source_hash"`
	MetaJSON   string `gorm:"type:text;not null" json:"meta_json"`
	Enabled    bool   `gorm:"not null" json:"enabled"`
	CreatedAt  int64  `gorm:"autoCreateTime" json:"created_at"`
}

// SaveTaskPlugin preserves immutable source for every key/version pair.
func SaveTaskPlugin(source string, meta taskplugin.Meta) (*TaskPlugin, error) {
	hash := sha256.Sum256([]byte(source))
	metadata, err := json.Marshal(meta)
	if err != nil {
		return nil, err
	}
	p := &TaskPlugin{Key: meta.Key, Version: meta.Version, APIVersion: meta.APIVersion, Source: source, SourceHash: hex.EncodeToString(hash[:]), MetaJSON: string(metadata), Enabled: true}
	if err := DB.Where("key = ? AND version = ?", p.Key, p.Version).FirstOrCreate(p).Error; err != nil {
		return nil, err
	}
	if p.Source != source {
		return nil, errors.New("plugin version already exists with different source; upload a new version")
	}
	return p, nil
}

func GetTaskPlugin(id int64) (*TaskPlugin, error) {
	var p TaskPlugin
	if err := DB.First(&p, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

func ListTaskPlugins() ([]TaskPlugin, error) {
	plugins := []TaskPlugin{}
	err := DB.Order("id DESC").Find(&plugins).Error
	return plugins, err
}

func SetTaskPluginEnabled(id int64, enabled bool) error {
	result := DB.Model(&TaskPlugin{}).Where("id = ?", id).Update("enabled", enabled)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func validateTaskPluginChannel(tx *gorm.DB, c *Channel) error {
	if c.Type != channel.ChannelTypeTaskPlugin {
		if c.TaskPluginID != 0 {
			return errors.New("only task plugin channels may bind a plugin")
		}
		return nil
	}
	if c.TaskPluginID <= 0 || len(c.Keys()) != 1 {
		return errors.New("task plugin channel requires a version and exactly one key")
	}
	if _, err := (taskplugin.Request{Method: "GET", Path: "/"}).URL(c.BaseURL, "GET"); err != nil {
		return err
	}
	var p TaskPlugin
	if err := tx.First(&p, "id = ?", c.TaskPluginID).Error; err != nil {
		return errors.New("plugin version not found")
	}
	var meta taskplugin.Meta
	if err := json.Unmarshal([]byte(p.MetaJSON), &meta); err != nil {
		return errors.New("invalid stored plugin metadata")
	}
	for _, name := range SplitList(c.Models) {
		if !slices.Contains(meta.Models, name) {
			return errors.New("channel model is not declared by the plugin: " + name)
		}
	}
	return nil
}

func guardTaskPluginChannel(tx *gorm.DB, previous *Channel, next *Channel) error {
	if previous.Type != channel.ChannelTypeTaskPlugin && (next == nil || next.Type != channel.ChannelTypeTaskPlugin) {
		return nil
	}
	if next != nil && previous.Type == next.Type && strings.TrimSpace(previous.Key) == strings.TrimSpace(next.Key) && previous.BaseURL == next.BaseURL {
		return nil
	}
	var pending int64
	if err := tx.Model(&Task{}).Where("channel_id = ? AND status NOT IN ?", previous.ID, []string{"succeeded", "failed", "cancelled"}).Count(&pending).Error; err != nil {
		return err
	}
	if pending != 0 {
		return ErrTaskPluginChannelBusy
	}
	return nil
}

// ReservePluginTask commits the reservation and a durable submission record
// together. Pending rows protect channel credentials without a process lock.
func ReservePluginTask(task *Task, expected *Channel) error {
	if task == nil || expected == nil || task.TaskPluginID <= 0 || task.Status != TaskStatusSubmitting || task.TaskID != "" || task.Quota < 1 || task.ChannelID != expected.ID {
		return errors.New("invalid plugin task admission")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		// The first balance write also takes SQLite's writer before channel reads.
		if err := reserveQuota(tx, task.UserID, task.TokenID, task.TokenUnlimited, task.Quota); err != nil {
			return err
		}
		var current Channel
		if err := tx.First(&current, "id = ?", expected.ID).Error; err != nil {
			return err
		}
		if current.Type != channel.ChannelTypeTaskPlugin || current.TaskPluginID != task.TaskPluginID || current.Key != expected.Key || current.BaseURL != expected.BaseURL || current.Status != common.ChannelStatusEnabled || !slices.Contains(SplitList(current.Models), task.Model) || !slices.Contains(ChannelGroups(&current), task.Group) {
			return ErrTaskPluginChannelChanged
		}
		var plugin TaskPlugin
		if err := tx.First(&plugin, "id = ? AND enabled = ?", task.TaskPluginID, true).Error; err != nil {
			return ErrTaskPluginChannelChanged
		}
		return tx.Create(task).Error
	})
}

func CompletePluginSubmission(id int64, upstreamID string) error {
	if strings.TrimSpace(upstreamID) == "" || len(upstreamID) > 128 {
		return errors.New("invalid upstream task id")
	}
	result := DB.Model(&Task{}).Where("id = ? AND task_plugin_id > 0 AND status = ? AND pending_status = ''", id, TaskStatusSubmitting).
		Updates(map[string]any{"task_id": upstreamID, "status": "submitted", "update_time": time.Now().Unix()})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("plugin submission is no longer active")
	}
	return nil
}

func RecordPluginSubmissionFailure(id int64, reason string) (*Task, error) {
	err := DB.Model(&Task{}).Where("id = ? AND task_plugin_id > 0 AND status = ? AND pending_status = ''", id, TaskStatusSubmitting).
		Updates(map[string]any{"pending_status": "failed", "pending_quota": 0,
			"pending_fail_reason": truncateForColumn(reason, 512), "update_time": time.Now().Unix()}).Error
	if err != nil {
		return nil, err
	}
	return GetTaskByID(id)
}

// RecordPluginTaskMissing counts spaced, consecutive missing responses. Client
// polling cannot accelerate the grace period or the confirmation intervals.
func RecordPluginTaskMissing(id int64, now time.Time) (int, error) {
	if err := DB.Model(&Task{}).Where("id = ? AND task_plugin_id > 0 AND pending_status = '' AND status NOT IN ? AND plugin_missing_time <= ?",
		id, []string{"succeeded", "failed", "cancelled"}, now.Add(-time.Minute).Unix()).Updates(map[string]any{
		"plugin_missing_count": gorm.Expr("plugin_missing_count + 1"), "plugin_missing_time": now.Unix(),
	}).Error; err != nil {
		return 0, err
	}
	stored, err := GetTaskByID(id)
	if err != nil {
		return 0, err
	}
	return stored.PluginMissingCount, nil
}

func ResetPluginTaskMissing(id int64) error {
	return DB.Model(&Task{}).Where("id = ? AND plugin_missing_count > 0", id).
		UpdateColumns(map[string]any{"plugin_missing_count": 0, "plugin_missing_time": 0}).Error
}
