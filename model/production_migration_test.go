package model

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestProductionLegacyMigrationPreservesRowsAndExactByteBudgets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	legacy, err := gorm.Open(sqlite.Open(dsn(path)), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := legacy.Exec(`CREATE TABLE production_attempt_records (id TEXT PRIMARY KEY, minute INTEGER, payload TEXT NOT NULL, attempts INTEGER, delivered_at INTEGER, next_attempt_at INTEGER, last_error TEXT)`).Error; err != nil {
		t.Fatal(err)
	}
	payload := `{"model":"测试"}`
	if err := legacy.Exec("INSERT INTO production_attempt_records VALUES (?,?,?,?,?,?,?)", "legacy", 60, payload, 0, 0, 0, "").Error; err != nil {
		t.Fatal(err)
	}
	if err := legacy.Exec("CREATE INDEX idx_production_attempt_records_delivered_at ON production_attempt_records(delivered_at)").Error; err != nil {
		t.Fatal(err)
	}
	sqlDB, err := legacy.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = CloseDB() })
	for i := 0; i < 2; i++ {
		if err := InitDB(path); err != nil {
			t.Fatal(err)
		}
		state, err := ReadProductionStorage()
		if err != nil || state.Rows != 1 || state.Bytes != int64(len(payload)) {
			t.Fatal("migration changed byte accounting", state, err)
		}
		rows, err := ReadProductionAttempts(0, 120, 1, 100)
		if err != nil || len(rows) != 1 || rows[0].Payload != payload || rows[0].BatchID != "" {
			t.Fatal("legacy attempt disappeared", rows, err)
		}
		if DB.Migrator().HasIndex(&ProductionAttemptRecord{}, "idx_production_attempt_records_delivered_at") || !DB.Migrator().HasIndex(&ProductionAttemptRecord{}, "idx_production_pending") {
			t.Fatal("queue index migration incomplete")
		}
		if err := CloseDB(); err != nil {
			t.Fatal(err)
		}
	}
}
