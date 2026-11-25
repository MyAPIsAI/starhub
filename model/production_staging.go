package model

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// StageProductionAttempts never holds the SQLite writer for an entire minute.
// Existing IDs are checked after insertion so retries cannot replace evidence.
func StageProductionAttempts(attempts []ProductionAttemptRecord) error {
	return stageProductionAttempts(DB, attempts)
}

func StageProductionAttemptsContext(ctx context.Context, attempts []ProductionAttemptRecord) error {
	return stageProductionAttempts(DB.WithContext(ctx), attempts)
}

func stageProductionAttempts(db *gorm.DB, attempts []ProductionAttemptRecord) error {
	for start := 0; start < len(attempts); {
		end, bytes := start, 0
		for end < len(attempts) && end-start < 128 {
			size := len(attempts[end].Payload) + 1024
			if end > start && bytes+size > 1<<20 {
				break
			}
			bytes += size
			end++
		}
		chunk := append([]ProductionAttemptRecord(nil), attempts[start:end]...)
		ids := make([]string, 0, len(chunk))
		expected := map[string]ProductionAttemptRecord{}
		for i := range chunk {
			row := &chunk[i]
			if row.ID == "" || row.BatchID == "" || row.Minute%60 != 0 || len(row.Payload) > 128<<10 {
				return errors.Join(ErrProductionEvidence, errors.New("invalid production stage"))
			}
			row.PayloadBytes = int64(len(row.Payload))
			row.NextAttemptAt = row.Minute + 60
			ids = append(ids, row.ID)
			if previous, ok := expected[row.ID]; ok && (previous.Minute != row.Minute || previous.BatchID != row.BatchID || previous.Payload != row.Payload || previous.RequestID != row.RequestID || previous.Sent != row.Sent) {
				return errors.Join(ErrProductionEvidence, errors.New("conflicting evidence within stage"))
			}
			expected[row.ID] = *row
		}
		if err := productionTransaction(db, func(tx *gorm.DB) error {
			if err := lockProductionStorage(tx); err != nil {
				return err
			}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&chunk).Error; err != nil {
				return err
			}
			var stored []ProductionAttemptRecord
			if err := tx.Where("id IN ?", ids).Find(&stored).Error; err != nil {
				return err
			}
			for _, row := range stored {
				want := expected[row.ID]
				if row.Minute != want.Minute || row.BatchID != want.BatchID || row.Payload != want.Payload || row.RequestID != want.RequestID || row.Sent != want.Sent {
					return errors.Join(ErrProductionEvidence, errors.New("attempt identity reused with different evidence"))
				}
			}
			if len(stored) != len(expected) {
				return errors.New("attempt stage was not stored")
			}
			return nil
		}); err != nil {
			return err
		}
		start = end
	}
	return nil
}
