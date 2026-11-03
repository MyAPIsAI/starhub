package model

import (
	"errors"
	"starhub/common"
	"time"

	"gorm.io/gorm"
)

var ErrExportNotFound = errors.New("export not found")

func initializeExportMetadata(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&ExportRecord{}).Where("expires_at = 0").UpdateColumn("expires_at", gorm.Expr("created_at + ?", 7*86400)).Error; err != nil {
			return err
		}
		var records []ExportRecord
		if err := tx.Select("id").Where("download_token IS NULL OR download_token = ''").Find(&records).Error; err != nil {
			return err
		}
		for _, record := range records {
			if err := tx.Model(&ExportRecord{}).Where("id = ?", record.ID).UpdateColumn("download_token", common.GenerateSessionKey()).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// ExportRecord retains a durable job specification and its generated files.
type ExportRecord struct {
	DownloadToken string `gorm:"uniqueIndex;size:64" json:"-"`
	Specification string `gorm:"type:text" json:"-"`
	Dictionary    []byte `gorm:"type:bytes" json:"-"`
	LeaseUntil    int64  `gorm:"index;not null;default:0" json:"-"`
	Failure       string `gorm:"size:256;not null;default:''" json:"failure,omitempty"`
	ID            int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID        int64  `gorm:"index;not null" json:"user_id"`
	Type          string `gorm:"size:32;not null;default:'requests'" json:"type"` // requests | ledger | usage
	RangeDays     int    `gorm:"not null;default:7" json:"range_days"`
	RowCount      int    `gorm:"not null;default:0" json:"row_count"`
	SizeKB        int    `gorm:"not null;default:0" json:"size_kb"`
	Status        string `gorm:"size:32;not null;default:'completed'" json:"status"` // completed | processing
	Filename      string `gorm:"size:256;not null;default:''" json:"filename"`
	Content       []byte `gorm:"type:bytes" json:"-"`
	CreatedAt     int64  `gorm:"autoCreateTime" json:"created_at"`
	ExpiresAt     int64  `gorm:"not null;default:0" json:"expires_at"` // 0 = 7 days default
}

// ExportRetentionDays is how long export files remain downloadable.
const ExportRetentionDays = 7

// InsertExport creates an export record with its CSV content.
func InsertExport(e *ExportRecord) error {
	if e.DownloadToken == "" {
		e.DownloadToken = common.GenerateSessionKey()
	}
	if e.Filename == "" {
		e.Filename = generateExportFilename(e.Type)
	}
	if e.Status == "" {
		e.Status = "completed"
	}
	if e.ExpiresAt == 0 {
		e.ExpiresAt = time.Now().Unix() + int64(ExportRetentionDays)*86400
	}
	return DB.Create(e).Error
}

// GetExport returns an export by ID.
func GetExport(id int64) (*ExportRecord, error) {
	if id <= 0 {
		return nil, ErrExportNotFound
	}
	var e ExportRecord
	err := DB.First(&e, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrExportNotFound
	}
	if err != nil {
		return nil, err
	}
	if e.ExpiresAt <= time.Now().Unix() {
		return nil, ErrExportNotFound
	}
	return &e, nil
}

// ListExports returns exports for a user (or all for admin), newest first.
func ListExports(userID int64) ([]ExportRecord, error) {
	var exports []ExportRecord
	q := DB.Model(&ExportRecord{})
	if userID > 0 {
		q = q.Where("user_id = ?", userID)
	}
	err := q.Order("id DESC").Find(&exports).Error
	return exports, err
}

// ListExportMetadata avoids loading file blobs while the UI polls job progress.
func ListExportMetadata(userID int64) ([]ExportRecord, error) {
	var exports []ExportRecord
	query := DB.Model(&ExportRecord{}).Omit("content", "dictionary")
	if userID > 0 {
		query = query.Where("user_id = ?", userID)
	}
	err := query.Order("id DESC").Find(&exports).Error
	return exports, err
}

// DeleteExport removes an export record.
func DeleteExport(id int64) error {
	res := DB.Delete(&ExportRecord{}, "id = ?", id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrExportNotFound
	}
	return nil
}

// CleanExpiredExports removes exports past their retention window.
func CleanExpiredExports() error {
	now := time.Now().Unix()
	return DB.Where("expires_at > 0 AND expires_at <= ?", now).Delete(&ExportRecord{}).Error
}

func generateExportFilename(typ string) string {
	return "export_" + typ + "_" + time.Now().UTC().Format("20060102_150405") + ".csv"
}
