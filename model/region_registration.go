package model

import (
	"errors"
	"gorm.io/gorm"
)

var ErrRegionRegistrationChanged = errors.New("regional registration policy changed; reload and confirm the current terms")
var ErrRegionLegalUnavailable = errors.New("regional legal documents are not published")

// CreateRegisteredUser commits the account and its legal consent together.
// The profile revision is rechecked after password hashing and before creation.
func CreateRegisteredUser(user *User, version string, accepted *RegionProfile) error {
	if accepted == nil || len(version) > 32 {
		return ErrRegionRegistrationChanged
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("UPDATE users SET id = id WHERE id = -1").Error; err != nil {
			return err
		}
		var current RegionProfile
		err := tx.First(&current, "id = ?", ActiveRegionID()).Error
		if accepted.ID == "" {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrRegionRegistrationChanged
			}
		} else if err != nil || current.ID != accepted.ID || current.Revision != accepted.Revision || !current.Enabled {
			return ErrRegionRegistrationChanged
		}
		if accepted.LegalVersion != "" {
			if version != accepted.LegalVersion {
				return ErrRegionRegistrationChanged
			}
			for _, kind := range []string{"user_agreement", "privacy_policy"} {
				var count int64
				if err := tx.Model(&LegalDocument{}).Where("type = ? AND version = ? AND published_at > 0 AND region_id IN ?", kind, version, []string{accepted.ID, ""}).Count(&count).Error; err != nil {
					return err
				}
				if count == 0 {
					return ErrRegionLegalUnavailable
				}
			}
		}
		if err := tx.Create(user).Error; err != nil {
			return err
		}
		return RecordAgreementConsent(tx, user.ID, version)
	})
}
