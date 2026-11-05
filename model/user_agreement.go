package model

import (
	"time"

	"gorm.io/gorm"
)

// UserAgreement records that a user has agreed to a specific version of the
// legal terms at registration time (and optionally at a later re-consent).
type UserAgreement struct {
	ID               int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID           int64  `gorm:"index;not null" json:"user_id"`
	AgreementVersion string `gorm:"size:32;not null" json:"agreement_version"`
	AgreedAt         int64  `gorm:"not null" json:"agreed_at"`
	CreatedAt        int64  `gorm:"autoCreateTime" json:"created_at"`
}

// RecordAgreementConsent saves the user's consent to the given agreement
// version. If a consent for the same version already exists it is a no-op
// (idempotent).
func RecordAgreementConsent(tx *gorm.DB, userID int64, version string) error {
	if tx == nil {
		tx = DB
	}
	if version == "" {
		return nil
	}
	var existing UserAgreement
	err := tx.Where("user_id = ? AND agreement_version = ?", userID, version).First(&existing).Error
	if err == nil {
		return nil
	}
	if !isRecordNotFound(err) {
		return err
	}
	return tx.Create(&UserAgreement{
		UserID:           userID,
		AgreementVersion: version,
		AgreedAt:         time.Now().Unix(),
	}).Error
}

// GetLatestAgreementConsent returns the most recent agreement consent for
// a user, or nil if none exists.
func GetLatestAgreementConsent(userID int64) (*UserAgreement, error) {
	var agreement UserAgreement
	err := DB.Where("user_id = ?", userID).Order("id DESC").First(&agreement).Error
	if isRecordNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &agreement, nil
}

func isRecordNotFound(err error) bool {
	return err != nil && err.Error() == "record not found"
}
