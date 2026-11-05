package model

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"time"

	"gorm.io/gorm"
	"starhub/common"
)

const installationDigest = "staff_installation_digest"

func StaffRootExists() (bool, error) {
	var count int64
	err := DB.Model(&StaffAccount{}).Where("role = ?", common.RoleRoot).Count(&count).Error
	return count > 0, err
}

func NewStaffInstallationToken() (string, error) {
	token := common.GenerateSessionKey()
	sum := sha256.Sum256([]byte(token))
	err := DB.Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&StaffAccount{}).Where("role = ?", common.RoleRoot).Count(&count).Error; err != nil {
			return err
		}
		if count != 0 {
			return errors.New("staff installation is already complete")
		}
		if err := tx.Where("key = ?", installationDigest).Assign(Option{Value: hex.EncodeToString(sum[:])}).FirstOrCreate(&Option{Key: installationDigest}).Error; err != nil {
			return err
		}
		return tx.Where("key = ?", installationDigest+"_expires").Assign(Option{Value: time.Now().Add(15 * time.Minute).Format(time.RFC3339)}).FirstOrCreate(&Option{Key: installationDigest + "_expires"}).Error
	})
	return token, err
}

func InitializeStaffRoot(username, password, installationToken string, requireToken bool) error {
	if len(password) < 12 || len(password) > 72 {
		return errors.New("Root password must be 12 to 72 bytes")
	}
	hash, err := common.HashPassword(password)
	if err != nil {
		return err
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		// Lock the installation record before checking Root count.
		if err := tx.Where("key = ?", installationDigest).FirstOrCreate(&Option{Key: installationDigest}).Error; err != nil {
			return err
		}
		if err := tx.Model(&Option{}).Where("key = ?", installationDigest).UpdateColumn("value", gorm.Expr("value")).Error; err != nil {
			return err
		}
		var count int64
		if err := tx.Model(&StaffAccount{}).Where("role = ?", common.RoleRoot).Count(&count).Error; err != nil {
			return err
		}
		if count != 0 {
			return errors.New("a staff Root already exists")
		}
		if requireToken {
			var digest, expires Option
			if tx.Where("key = ?", installationDigest).First(&digest).Error != nil || tx.Where("key = ?", installationDigest+"_expires").First(&expires).Error != nil {
				return ErrStaffSecurity
			}
			until, err := time.Parse(time.RFC3339, expires.Value)
			sum := sha256.Sum256([]byte(installationToken))
			want := hex.EncodeToString(sum[:])
			if err != nil || time.Now().After(until) || len(installationToken) != 64 || subtle.ConstantTimeCompare([]byte(want), []byte(digest.Value)) != 1 {
				return ErrStaffSecurity
			}
		}
		root := User{Username: username, Password: hash, AccountKind: "staff", Role: common.RoleRoot, Status: common.StatusEnabled, Group: common.DefaultGroup, AffCode: common.GenerateAffCode()}
		if err := createStaffIdentity(tx, &root); err != nil {
			return err
		}
		if err := tx.Model(&Option{}).Where("key = ?", installationDigest).Update("value", "").Error; err != nil {
			return err
		}
		if err := seedDefaultServiceGroups(tx); err != nil {
			return err
		}
		return writeStaffAudit(tx, 0, root.ID, "staff_root_initialized", "Deployment initialization", "")
	})
}

func ResetSoleRootOTP(username string) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&StaffAccount{}).Where("id > 0").UpdateColumn("id", gorm.Expr("id")).Error; err != nil {
			return err
		}
		var roots []StaffAccount
		if err := tx.Where("role = ? AND status = ?", common.RoleRoot, common.StatusEnabled).Find(&roots).Error; err != nil {
			return err
		}
		if len(roots) != 1 || roots[0].Username != username {
			return errors.New("CLI 2FA recovery requires the sole active Root; otherwise use another Root")
		}
		if err := tx.Model(&roots[0]).Updates(map[string]any{"totp_secret": "", "totp_last_step": 0, "otp_failures": 0, "otp_locked_until": 0, "security_version": gorm.Expr("security_version + 1")}).Error; err != nil {
			return err
		}
		return writeStaffAudit(tx, 0, roots[0].ID, "staff_2fa_reset", "Deployment-side sole Root 2FA recovery", "")
	})
}
