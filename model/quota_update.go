package model

import (
	"errors"
	"gorm.io/gorm"
)

var ErrQuotaConflict = errors.New("quota changed since the record was read")

// updateFieldsWithQuota compares and updates in one SQL statement so a
// stale form cannot overwrite concurrent consumption or redemption.
func updateFieldsWithQuota(db *gorm.DB, entity any, id int64, fields map[string]any, column string, expected *int64) error {
	q := db.Model(entity).Where("id = ?", id)
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
	return updateFieldsWithQuota(DB, &User{}, id, fields, "quota", expected)
}

func UpdateTokenFieldsWithQuota(id int64, fields map[string]any, expected *int64, revision ...int64) error {
	fields["revision"] = gorm.Expr("revision + 1")
	q := DB
	if len(revision) > 0 {
		q = q.Where("revision = ?", revision[0])
	}
	return updateFieldsWithQuota(q, &Token{}, id, fields, "remain_quota", expected)
}
