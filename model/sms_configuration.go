package model

import (
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"starhub/common"
)

var ErrSMSConfigurationConflict = errors.New("SMS configuration or administrator identity changed")

// SMSConfiguration is separate from public options and their value history.
// Key contains only the encrypted provider secret and is never serialized.
type SMSConfiguration struct {
	ID           int64  `gorm:"primaryKey" json:"-"`
	Version      int64  `gorm:"not null" json:"-"`
	Enabled      bool   `gorm:"not null" json:"-"`
	AccessKeyID  string `gorm:"size:128" json:"-"`
	Key          string `gorm:"type:text" json:"-"`
	SignName     string `gorm:"size:256" json:"-"`
	TemplateCode string `gorm:"size:128" json:"-"`
}

func GetSMSConfiguration() (*SMSConfiguration, error) {
	var value SMSConfiguration
	err := DB.First(&value, "id = ?", 1).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &SMSConfiguration{ID: 1}, nil
	}
	return &value, err
}

// SaveSMSConfiguration atomically checks the password proof and configuration
// version. A failed write preserves every field of the previous configuration.
func SaveSMSConfiguration(value *SMSConfiguration, expectedVersion int64, expectedKey string, owner *User) error {
	if owner == nil || value == nil || expectedVersion < 0 || owner.Status != common.StatusEnabled || (owner.Role != common.RoleAdmin && owner.Role != common.RoleRoot) {
		return ErrSMSConfigurationConflict
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&User{}).Where("id = ?", owner.ID).UpdateColumn("id", gorm.Expr("id")).Error; err != nil {
			return err
		}
		var current User
		if err := tx.First(&current, "id = ?", owner.ID).Error; err != nil {
			return err
		}
		if current.Status != common.StatusEnabled || current.Password != owner.Password || current.Role != owner.Role {
			return ErrSMSConfigurationConflict
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&SMSConfiguration{ID: 1}).Error; err != nil {
			return err
		}
		// A wrapping-key rotation can change ciphertext without changing settings.
		result := tx.Model(&SMSConfiguration{}).Where(map[string]any{"id": 1, "version": expectedVersion, "key": expectedKey}).Updates(map[string]any{
			"version": expectedVersion + 1, "enabled": value.Enabled, "access_key_id": value.AccessKeyID,
			"key": value.Key, "sign_name": value.SignName, "template_code": value.TemplateCode,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrSMSConfigurationConflict
		}
		return nil
	})
}
