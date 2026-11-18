package model

import (
	"errors"
	"slices"
	"time"

	"gorm.io/gorm"
	"starhub/common"
)

const ArkVideoProbePlatform = "ark-video-probe"

// ReserveVideoProbe persists admission before HTTP and serializes duplicate submissions.
func ReserveVideoProbe(task *Task, expected *Channel) (*Task, bool, error) {
	duplicate := errors.New("duplicate video probe")
	var prior Task
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(task).Error; err != nil {
			return err
		}
		err := tx.Where("platform = ? AND channel_id = ? AND user_id = ? AND quota_reservation_id = ? AND id <> ?", ArkVideoProbePlatform, task.ChannelID, task.UserID, task.QuotaReservationID, task.ID).First(&prior).Error
		if err == nil {
			if prior.RequestMeta != task.RequestMeta {
				return errors.New("video_probe_request_conflict")
			}
			return duplicate
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var current Channel
		if err := tx.First(&current, "id = ?", expected.ID).Error; err != nil {
			return err
		}
		_, enabled := current.KeyAt(task.KeyIndex)
		if current.Type != 32 || current.Status != common.ChannelStatusEnabled || current.Key != expected.Key || current.BaseURL != expected.BaseURL || !enabled || !slices.Contains(SplitList(current.Models), task.Model) {
			return errors.New("video_probe_channel_changed")
		}
		var active int64
		if err := tx.Model(&Task{}).Where("platform = ? AND channel_id = ? AND id <> ? AND status NOT IN ?", ArkVideoProbePlatform, current.ID, task.ID, []string{"succeeded", "failed", "cancelled"}).Count(&active).Error; err != nil {
			return err
		}
		if active > 0 {
			return errors.New("video_probe_already_running")
		}
		return nil
	})
	if errors.Is(err, duplicate) {
		return &prior, false, nil
	}
	return task, err == nil, err
}

// UpdateVideoProbe never settles customer quota; only the upstream charges for this admin test.
func UpdateVideoProbe(id int64, status, reason, result string) error {
	values := map[string]any{"status": status, "fail_reason": reason, "result_meta": result, "update_time": time.Now().Unix()}
	if status == "succeeded" || status == "failed" || status == "cancelled" {
		values["complete_time"] = time.Now().Unix()
	}
	return DB.Model(&Task{}).Where("id = ? AND platform = ? AND status NOT IN ?", id, ArkVideoProbePlatform, []string{"succeeded", "failed", "cancelled"}).Updates(values).Error
}
