package model

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"gorm.io/gorm"

	"starhub/common"
)

var ErrRedemptionNotFound = errors.New("redemption code not found")

// Redemption is a prepaid quota code: an admin creates a batch, a user
// redeems one to top up their quota. The code is single-use.
type Redemption struct {
	ID        int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	Code      string `gorm:"uniqueIndex;size:64;not null" json:"code"`
	Quota     int64  `gorm:"not null;default:0" json:"quota"`
	Status    int    `gorm:"not null;default:1" json:"status"` // 1=unused (Enabled), 2=used (Disabled)
	UsedBy    int64  `gorm:"index;not null;default:0" json:"used_by"`
	UsedAt    int64  `gorm:"not null;default:0" json:"used_at"`
	CreatedAt int64  `gorm:"autoCreateTime" json:"created_at"`
	ExpiredAt int64  `gorm:"not null;default:0" json:"expired_at"` // 0 = never expires
}

// GenerateRedemptionCode generates a random redemption code.
func GenerateRedemptionCode() string {
	raw := make([]byte, 12)
	_, _ = rand.Read(raw)
	return "RD-" + hex.EncodeToString(raw)
}

// InsertRedemption creates a redemption code.
func InsertRedemption(r *Redemption) error {
	if r.Code == "" {
		r.Code = GenerateRedemptionCode()
	}
	return DB.Create(r).Error
}

// GetRedemptionByCode returns a redemption code by its string.
func GetRedemptionByCode(code string) (*Redemption, error) {
	if code == "" {
		return nil, ErrRedemptionNotFound
	}
	var r Redemption
	err := DB.Where("code = ?", code).First(&r).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrRedemptionNotFound
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// ListRedemptions returns all redemption codes (admin view).
func ListRedemptions() ([]Redemption, error) {
	var codes []Redemption
	err := DB.Order("id DESC").Find(&codes).Error
	return codes, err
}

// DeleteRedemption removes a redemption code (admin only, unused codes only
// recommended but not enforced here).
func DeleteRedemption(id int64) error {
	res := DB.Delete(&Redemption{}, "id = ?", id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrRedemptionNotFound
	}
	return nil
}

// RedeemCode atomically claims a redemption code and credits the user's
// quota. It returns ErrRedemptionNotFound when the code does not exist,
// is already used, disabled, or expired.
func RedeemCode(code string, userID int64) (int64, error) {
	var credited int64
	err := DB.Transaction(func(tx *gorm.DB) error {
		var r Redemption
		if err := tx.Where("code = ?", code).First(&r).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrRedemptionNotFound
			}
			return err
		}
		if r.Status != common.StatusEnabled {
			return ErrRedemptionNotFound
		}
		if r.ExpiredAt > 0 && time.Now().Unix() > r.ExpiredAt {
			return ErrRedemptionNotFound
		}
		// Claim the code: set status to used and record the user. The
		// WHERE clause prevents a race between two concurrent redeems.
		res := tx.Model(&Redemption{}).
			Where("id = ? AND status = ?", r.ID, common.StatusEnabled).
			Updates(map[string]any{
				"status":  common.StatusDisabled, // 2 = used
				"used_by": userID,
				"used_at": time.Now().Unix(),
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrRedemptionNotFound
		}
		// Credit the user.
		if err := tx.Model(&User{}).Where("id = ?", userID).
			Updates(map[string]any{"quota": gorm.Expr("quota + ?", r.Quota)}).Error; err != nil {
			return err
		}
		credited = r.Quota
		return nil
	})
	return credited, err
}