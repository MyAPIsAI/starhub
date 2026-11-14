package model

import (
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// StageProductionAttempts never holds the SQLite writer for an entire minute.
// Existing IDs are checked after insertion so retries cannot replace evidence.
func StageProductionAttempts(attempts []ProductionAttemptRecord) error {
	for start := 0; start < len(attempts); start += 128 {
		chunk := append([]ProductionAttemptRecord(nil), attempts[start:min(start+128, len(attempts))]...)
		ids := make([]string, 0, len(chunk))
		expected := map[string]ProductionAttemptRecord{}
		for i := range chunk {
			row := &chunk[i]
			if row.ID == "" || row.BatchID == "" || row.Minute%60 != 0 || len(row.Payload) > 128<<10 {
				return errors.New("invalid production stage")
			}
			row.PayloadBytes = int64(len(row.Payload))
			ids = append(ids, row.ID)
			expected[row.ID] = *row
		}
		if err := DB.Transaction(func(tx *gorm.DB) error {
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&chunk).Error; err != nil {
				return err
			}
			var stored []ProductionAttemptRecord
			if err := tx.Where("id IN ?", ids).Find(&stored).Error; err != nil {
				return err
			}
			for _, row := range stored {
				want := expected[row.ID]
				if row.Minute != want.Minute || row.BatchID != want.BatchID || row.Payload != want.Payload {
					return errors.New("attempt identity reused with different evidence")
				}
			}
			if len(stored) != len(expected) {
				return errors.New("attempt stage was not stored")
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}
