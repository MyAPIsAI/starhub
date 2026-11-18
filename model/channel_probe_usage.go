package model

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"

	"starhub/common"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// ChannelImageProbe records usage facts without touching customer billing.
// An unfinished attempt stays uncertain after a crash or network failure.
type ChannelImageProbe struct {
	ID             int64  `gorm:"primaryKey;autoIncrement"`
	ChannelID      int64  `gorm:"not null;index"`
	Model          string `gorm:"size:128;not null;default:''"`
	State          string `gorm:"size:16;not null;default:'uncertain'"`
	Images         int64  `gorm:"not null;default:0"`
	PreviewURL     string `gorm:"type:text;not null;default:''" json:"-"`
	ElapsedMs      int64  `gorm:"not null;default:0"`
	ConnectionHash string `gorm:"size:64;not null;default:''" json:"-"`
	CreatedAt      int64  `gorm:"autoCreateTime"`
	UpdatedAt      int64  `gorm:"autoUpdateTime"`
}

type ChannelProbeUsage struct {
	ImageTests        int64  `json:"image_tests"`
	ImageSuccesses    int64  `json:"image_successes"`
	Images            int64  `json:"images"`
	UncertainImages   int64  `json:"uncertain_images"`
	VideoTests        int64  `json:"video_tests"`
	VideoSuccesses    int64  `json:"video_successes"`
	VideoSeconds      int64  `json:"video_seconds"`
	VideoOutputTokens string `json:"video_output_tokens"`
	MissingVideoUsage int64  `json:"missing_video_usage"`
	PendingVideos     int64  `json:"pending_videos"`
}

func imageProbeConnection(ch *Channel) string {
	raw, _ := json.Marshal([]any{ch.Type, ch.BaseURL, ch.Key})
	return common.HashKey("channel-image-probe:" + string(raw))
}

func StartChannelImageProbe(ch *Channel, name string) (*ChannelImageProbe, error) {
	entry := &ChannelImageProbe{ChannelID: ch.ID, Model: name, State: "uncertain", ConnectionHash: imageProbeConnection(ch)}
	return entry, DB.Create(entry).Error
}

func CompleteChannelImageProbe(id int64, state string, images int64, previewURL string, elapsedMs int64) error {
	// Signed preview URLs must never appear in SQL error logs.
	return DB.Session(&gorm.Session{Logger: DB.Logger.LogMode(logger.Silent)}).Model(&ChannelImageProbe{}).Where("id = ? AND state = ?", id, "uncertain").Updates(map[string]any{"state": state, "images": images, "preview_url": previewURL, "elapsed_ms": elapsedMs}).Error
}

// Results belong to the saved connection and configured model, not channel health.
func LatestChannelImageProbe(ch *Channel) (*ChannelImageProbe, error) {
	var entry ChannelImageProbe
	err := DB.Where("channel_id = ? AND connection_hash = ? AND model IN ?", ch.ID, imageProbeConnection(ch), SplitList(ch.Models)).Order("id DESC").First(&entry).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &entry, err
}

// Read persisted tasks directly so historical videos need no monetary backfill.
// The summary includes admin tests only, never ordinary relay usage twice.
func GetChannelProbeUsage(ctx context.Context, channelID int64) (*ChannelProbeUsage, error) {
	result := &ChannelProbeUsage{VideoOutputTokens: "0"}
	rows, err := DB.WithContext(ctx).Model(&ChannelImageProbe{}).Where("channel_id = ?", channelID).Select("state", "images").Rows()
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var state string
		var images int64
		if err := rows.Scan(&state, &images); err != nil {
			rows.Close()
			return nil, err
		}
		result.ImageTests++
		if state == "succeeded" {
			result.ImageSuccesses++
			result.Images += images
		} else if state == "uncertain" {
			result.UncertainImages++
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	rows, err = DB.WithContext(ctx).Model(&Task{}).Where("channel_id = ? AND platform = ?", channelID, ArkVideoProbePlatform).Select("status", "result_meta").Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tokens := new(big.Int)
	for rows.Next() {
		var status, meta string
		if err := rows.Scan(&status, &meta); err != nil {
			return nil, err
		}
		result.VideoTests++
		if status != "succeeded" && status != "failed" && status != "cancelled" {
			result.PendingVideos++
			continue
		}
		var facts struct {
			Seconds int64 `json:"seconds"`
			Usage   *struct {
				CompletionTokens *int64 `json:"completion_tokens"`
			} `json:"usage"`
		}
		valid := json.Unmarshal([]byte(meta), &facts) == nil
		if status == "succeeded" {
			result.VideoSuccesses++
			if valid && facts.Seconds > 0 {
				result.VideoSeconds += facts.Seconds
			}
		}
		if valid && facts.Usage != nil && facts.Usage.CompletionTokens != nil && *facts.Usage.CompletionTokens >= 0 {
			tokens.Add(tokens, big.NewInt(*facts.Usage.CompletionTokens))
		} else if status == "succeeded" {
			result.MissingVideoUsage++
		}
	}
	result.VideoOutputTokens = tokens.String()
	return result, rows.Err()
}
