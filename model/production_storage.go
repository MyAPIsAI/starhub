package model

import (
	"gorm.io/gorm"
)

// SQLite triggers keep budgets exact even for legacy writers and retries.
type ProductionStorageState struct {
	ID                  int    `gorm:"primaryKey;autoIncrement:false" json:"-"`
	Rows                int64  `json:"rows"`
	Bytes               int64  `json:"bytes"`
	MaxRows             int64  `json:"max_rows"`
	MaxBytes            int64  `json:"max_bytes"`
	Dropped             int64  `json:"dropped"`
	DroppedBytes        int64  `json:"dropped_bytes"`
	UndeliveredDropped  int64  `json:"undelivered_dropped"`
	LastDropAt          int64  `json:"last_drop_at"`
	LastDropReason      string `gorm:"size:64" json:"last_drop_reason"`
	OldestDroppedMinute int64  `json:"oldest_dropped_minute"`
}

func initProductionStorage(db *gorm.DB) error {
	if err := db.AutoMigrate(&ProductionStorageState{}, &ProductionRetentionGap{}); err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var n int64
		if err := tx.Model(&ProductionStorageState{}).Where("id = 1").Count(&n).Error; err != nil {
			return err
		}
		if n == 0 {
			var totals struct{ Rows, Bytes int64 }
			if err := tx.Raw("SELECT COUNT(*) AS rows, COALESCE(SUM(length(CAST(payload AS BLOB))),0) AS bytes FROM production_attempt_records").Scan(&totals).Error; err != nil {
				return err
			}
			if err := tx.Create(&ProductionStorageState{ID: 1, Rows: totals.Rows, Bytes: totals.Bytes, MaxRows: 100000, MaxBytes: 128 << 20}).Error; err != nil {
				return err
			}
		}
		for _, sql := range []string{
			`CREATE TRIGGER IF NOT EXISTS production_storage_admission BEFORE INSERT ON production_attempt_records WHEN NOT EXISTS (SELECT 1 FROM production_attempt_records WHERE id = NEW.id) AND EXISTS (SELECT 1 FROM production_storage_states WHERE id = 1 AND (rows >= max_rows OR bytes + length(CAST(NEW.payload AS BLOB)) > max_bytes)) BEGIN SELECT RAISE(ABORT,'production storage capacity exceeded'); END`,
			`CREATE TRIGGER IF NOT EXISTS production_storage_insert AFTER INSERT ON production_attempt_records BEGIN UPDATE production_storage_states SET rows=rows+1, bytes=bytes+length(CAST(NEW.payload AS BLOB)) WHERE id=1; END`,
			`CREATE TRIGGER IF NOT EXISTS production_storage_update_admission BEFORE UPDATE OF payload ON production_attempt_records WHEN EXISTS (SELECT 1 FROM production_storage_states WHERE id=1 AND bytes+length(CAST(NEW.payload AS BLOB))-length(CAST(OLD.payload AS BLOB)) > max_bytes) BEGIN SELECT RAISE(ABORT,'production storage capacity exceeded'); END`,
			`CREATE TRIGGER IF NOT EXISTS production_storage_delete AFTER DELETE ON production_attempt_records BEGIN UPDATE production_storage_states SET rows=rows-1, bytes=bytes-length(CAST(OLD.payload AS BLOB)) WHERE id=1; END`,
			`CREATE TRIGGER IF NOT EXISTS production_storage_update AFTER UPDATE OF payload ON production_attempt_records BEGIN UPDATE production_storage_states SET bytes=bytes+length(CAST(NEW.payload AS BLOB))-length(CAST(OLD.payload AS BLOB)) WHERE id=1; END`,
			`CREATE INDEX IF NOT EXISTS idx_production_pending ON production_attempt_records(next_attempt_at, minute, id) WHERE delivered_at = 0`,
			`CREATE INDEX IF NOT EXISTS idx_production_minute_cursor ON production_attempt_records(minute,id)`,
			`CREATE INDEX IF NOT EXISTS idx_production_publication ON production_attempt_records(batch_id,minute)`,
			`DROP INDEX IF EXISTS idx_production_attempt_records_delivered_at`,
			`DROP INDEX IF EXISTS idx_production_attempt_records_minute`,
			`DROP INDEX IF EXISTS idx_production_attempt_records_batch_id`,
		} {
			if err := tx.Exec(sql).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func ReadProductionStorage() (ProductionStorageState, error) {
	var state ProductionStorageState
	err := DB.First(&state, 1).Error
	return state, err
}
