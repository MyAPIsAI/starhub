package model

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

var ErrExportNotFound = errors.New("export not found")

// ExportRecord is a server-generated CSV export. The CSV content is generated
// synchronously on create and stored alongside the metadata row.
type ExportRecord struct {
	ID        int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID    int64  `gorm:"index;not null" json:"user_id"`
	Type      string `gorm:"size:32;not null;default:'requests'" json:"type"` // requests | ledger | usage
	RangeDays int    `gorm:"not null;default:7" json:"range_days"`
	RowCount  int    `gorm:"not null;default:0" json:"row_count"`
	SizeKB    int    `gorm:"not null;default:0" json:"size_kb"`
	Status    string `gorm:"size:32;not null;default:'completed'" json:"status"` // completed | processing
	Filename  string `gorm:"size:256;not null;default:''" json:"filename"`
	Content   []byte `gorm:"type:bytes" json:"-"`
	CreatedAt int64  `gorm:"autoCreateTime" json:"created_at"`
	ExpiresAt int64  `gorm:"not null;default:0" json:"expires_at"` // 0 = 7 days default
}

// ExportRetentionDays is how long export files remain downloadable.
const ExportRetentionDays = 7

// InsertExport creates an export record with its CSV content.
func InsertExport(e *ExportRecord) error {
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
	return DB.Where("expires_at > 0 AND expires_at < ?", now).Delete(&ExportRecord{}).Error
}

func generateExportFilename(typ string) string {
	return "export_" + typ + "_" + time.Now().UTC().Format("20060102_150405") + ".csv"
}