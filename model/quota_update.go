package model

import (
	"errors"
	"gorm.io/gorm"
	"time"
)

var ErrQuotaConflict = errors.New("quota changed since the record was read")

// updateFieldsWithQuota compares and updates in one SQL statement so a
// stale form cannot overwrite concurrent consumption or redemption.
func updateFieldsWithQuota(entity any, id int64, fields map[string]any, column string, expected *int64) error {
	q := DB.Model(entity).Where("id = ?", id)
	if _, changed := fields[column]; changed {
		if expected == nil {
			return ErrQuotaConflict
		}
		q = q.Where(column+" = ?", *expected)
	}
	res := q.Updates(fields)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrQuotaConflict
	}
	return nil
}

func UpdateUserFieldsWithQuota(id int64, fields map[string]any, expected *int64) error {
	return updateFieldsWithQuota(&User{}, id, fields, "quota", expected)
}

func UpdateTokenFieldsWithQuota(id int64, fields map[string]any, expected *int64) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Token{}).Where("id = ?", id).UpdateColumn("id", gorm.Expr("id")).Error; err != nil {
			return err
		}
		var token Token
		if err := tx.First(&token, id).Error; err != nil {
			return err
		}
		if _, changed := fields["remain_quota"]; changed && (expected == nil || token.RemainQuota != *expected) {
			return ErrQuotaConflict
		}
		if err := configureTokenPeriod(tx, &token, fields, time.Now()); err != nil {
			return err
		}
		fields["revision"] = gorm.Expr("revision + 1")
		return tx.Model(&Token{}).Where("id = ?", id).Updates(fields).Error
	})
}
