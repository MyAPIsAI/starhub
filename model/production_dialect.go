package model

import (
	"errors"

	"gorm.io/gorm"
)

// The existing budget row serializes admission, publication and retention on
// every backend. It also acquires SQLite's writer before any consistent reads.
func lockProductionStorage(tx *gorm.DB) error {
	result := tx.Model(&ProductionStorageState{}).Where("id = 1").UpdateColumn("id", gorm.Expr("id"))
	if result.Error != nil {
		return result.Error
	}
	var state ProductionStorageState
	if err := tx.Select("id").First(&state, 1).Error; err != nil {
		return errors.New("production storage guard is unavailable: " + err.Error())
	}
	return nil
}

// Expressions are assembled only from internal column names, never request data.
func productionPayloadLength(db *gorm.DB, column string) string {
	if db.Dialector.Name() == "sqlite" {
		return "length(CAST(" + column + " AS BLOB))"
	}
	return "octet_length(" + column + ")"
}

func productionQuote(db *gorm.DB, name string) string {
	stmt := &gorm.Statement{DB: db}
	return stmt.Quote(name)
}
