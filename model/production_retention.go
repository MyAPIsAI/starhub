package model

import (
	"context"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

type ProductionRetentionGap struct {
	BatchID   string `gorm:"primaryKey;size:64"`
	Minute    int64  `gorm:"index"`
	Count     int64
	Bytes     int64
	CreatedAt int64
}

// Cleanup has its own short transactions and never runs inside publication.
// Hard admission limits remain enforced even if cleanup cannot keep up.
func PruneProductionTelemetry(now time.Time) (int, error) {
	return PruneProductionTelemetryContext(context.Background(), now)
}

func PruneProductionTelemetryContext(ctx context.Context, now time.Time) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, ProductionDatabaseTimeout())
	defer cancel()
	db := DB.WithContext(ctx)
	total := 0
	deadline := time.Now().Add(100 * time.Millisecond)
	for i := 0; i < 16 && time.Now().Before(deadline); i++ {
		n, err := pruneProductionChunkDB(db, now)
		total += n
		if err != nil {
			return total, err
		}
		if n == 0 {
			break
		}
	}
	cutoff := now.Add(-24*time.Hour).Unix() / 60 * 60
	var minutes []int64
	if err := db.Model(&ProductionTelemetryMinute{}).Where("minute < ?", cutoff).Order("minute").Limit(64).Pluck("minute", &minutes).Error; err != nil {
		return total, err
	}
	if len(minutes) > 0 {
		if err := db.Where("minute IN ?", minutes).Delete(&ProductionTelemetryMinute{}).Error; err != nil {
			return total, err
		}
	}
	var gaps []string
	if err := db.Model(&ProductionRetentionGap{}).Where("minute < ?", cutoff).Order("minute").Limit(512).Pluck("batch_id", &gaps).Error; err != nil {
		return total, err
	}
	if len(gaps) > 0 {
		if err := db.Where("batch_id IN ?", gaps).Delete(&ProductionRetentionGap{}).Error; err != nil {
			return total, err
		}
	}
	return total, nil
}

func pruneProductionChunk(now time.Time) (int, error) {
	return pruneProductionChunkDB(DB, now)
}

func pruneProductionChunkDB(db *gorm.DB, now time.Time) (int, error) {
	var state ProductionStorageState
	err := db.First(&state, 1).Error
	if err != nil || state.Rows == 0 {
		return 0, err
	}
	cutoff := now.Add(-24*time.Hour).Unix() / 60 * 60
	var earliest ProductionAttemptRecord
	if err := db.Select("minute").Order("minute").Limit(1).Find(&earliest).Error; err != nil {
		return 0, err
	}
	if earliest.Minute >= cutoff && state.Rows <= max(1, state.MaxRows*9/10) && state.Bytes <= max(1, state.MaxBytes*9/10) {
		return 0, nil
	}
	removed := 0
	err = productionTransaction(db, func(tx *gorm.DB) error {
		removed = 0
		if err := lockProductionStorage(tx); err != nil {
			return err
		}
		if err := tx.First(&state, 1).Error; err != nil {
			return err
		}
		expired := earliest.Minute < cutoff
		columns := "id,minute,batch_id,delivered_at," + productionPayloadLength(tx, "payload") + " AS payload_bytes"
		query := tx.Select(columns).Order("minute,id").Limit(512)
		if expired {
			query = query.Where("minute < ?", cutoff)
		} else {
			query = query.Where("delivered_at > 0")
		}
		var rows []ProductionAttemptRecord
		if err := query.Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 && !expired {
			if err := tx.Select(columns).Order("minute,id").Limit(512).Find(&rows).Error; err != nil {
				return err
			}
		}
		ids := []string{}
		bytes, pending := int64(0), int64(0)
		unpublished := map[string]ProductionRetentionGap{}
		for _, row := range rows {
			if !expired && state.Rows-int64(len(ids)) <= max(1, state.MaxRows*9/10) && state.Bytes-bytes <= max(1, state.MaxBytes*9/10) {
				break
			}
			ids = append(ids, row.ID)
			bytes += row.PayloadBytes
			if row.DeliveredAt == 0 {
				pending++
			}
			if row.BatchID != "" {
				gap := unpublished[row.BatchID]
				gap.BatchID = row.BatchID
				gap.Minute = row.Minute
				gap.Count++
				gap.Bytes += row.PayloadBytes
				gap.CreatedAt = now.Unix()
				unpublished[row.BatchID] = gap
			}
		}
		if len(ids) == 0 {
			return nil
		}
		for id, gap := range unpublished {
			var published int64
			if err := tx.Model(&ProductionTelemetryMinute{}).Where("batch_id = ?", id).Count(&published).Error; err != nil {
				return err
			}
			if published > 0 {
				continue
			}
			if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "batch_id"}}, DoUpdates: clause.Assignments(map[string]any{"count": gorm.Expr("production_retention_gaps.count + ?", gap.Count), "bytes": gorm.Expr("production_retention_gaps.bytes + ?", gap.Bytes)})}).Create(&gap).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("id IN ?", ids).Delete(&ProductionAttemptRecord{}).Error; err != nil {
			return err
		}
		reason := "capacity"
		if expired {
			reason = "age"
		}
		oldest := rows[0].Minute
		if state.OldestDroppedMinute > 0 {
			oldest = min(oldest, state.OldestDroppedMinute)
		}
		if err := tx.Model(&ProductionStorageState{}).Where("id=1").Updates(map[string]any{"dropped": gorm.Expr("dropped + ?", len(ids)), "dropped_bytes": gorm.Expr("dropped_bytes + ?", bytes), "undelivered_dropped": gorm.Expr("undelivered_dropped + ?", pending), "last_drop_at": now.Unix(), "last_drop_reason": reason, "oldest_dropped_minute": oldest}).Error; err != nil {
			return err
		}
		removed = len(ids)
		return nil
	})
	return removed, err
}
