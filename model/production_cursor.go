package model

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
)

var ErrProductionCursor = errors.New("invalid attempt cursor")

type productionCursor struct {
	Minute int64  `json:"minute"`
	ID     string `json:"id"`
}

// ReadProductionAttemptPage uses a stable minute/ID cursor without an offset.
// Retention may remove rows between pages without duplicating remaining rows.
func ReadProductionAttemptPage(start, end int64, cursor string, size int) ([]ProductionAttemptRecord, string, error) {
	return ReadProductionAttemptPageContext(context.Background(), start, end, cursor, size)
}

func ReadProductionAttemptPageContext(ctx context.Context, start, end int64, cursor string, size int) ([]ProductionAttemptRecord, string, error) {
	ctx, cancel := context.WithTimeout(ctx, ProductionDatabaseTimeout())
	defer cancel()
	if size < 1 || size > 1000 {
		return nil, "", ErrProductionCursor
	}
	query := PublishedProductionAttempts(DB.WithContext(ctx)).Where("minute >= ? AND minute <= ?", start, end)
	if cursor != "" {
		var after productionCursor
		raw, err := base64.RawURLEncoding.DecodeString(cursor)
		if len(cursor) > 256 || err != nil || json.Unmarshal(raw, &after) != nil || after.Minute < start || after.Minute > end || after.ID == "" || len(after.ID) > 64 {
			return nil, "", ErrProductionCursor
		}
		query = query.Where("minute >= ? AND (minute > ? OR id > ?)", after.Minute, after.Minute, after.ID)
	}
	rows := []ProductionAttemptRecord{}
	if err := query.Order("minute,id").Limit(size + 1).Find(&rows).Error; err != nil {
		return nil, "", err
	}
	next := ""
	if len(rows) > size {
		rows = rows[:size]
		last := rows[len(rows)-1]
		raw, _ := json.Marshal(productionCursor{last.Minute, last.ID})
		next = base64.RawURLEncoding.EncodeToString(raw)
	}
	return rows, next, nil
}
