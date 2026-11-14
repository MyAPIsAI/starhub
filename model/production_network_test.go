package model

import (
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// These opt-in tests require disposable databases, never production databases.
func networkProductionDB(t *testing.T, driver string) *gorm.DB {
	t.Helper()
	env := "STARHUB_TEST_MYSQL_DSN"
	if driver == "postgres" {
		env = "STARHUB_TEST_POSTGRES_DSN"
	}
	dsn := os.Getenv(env)
	if dsn == "" {
		t.Skip("disposable database DSN not supplied")
	}
	db, err := openConfiguredDatabase(DatabaseConfig{Driver: driver, DSN: dsn})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(8)
	previous := DB
	DB = db
	t.Cleanup(func() { DB = previous; _ = sqlDB.Close() })
	models := []any{&ProductionAttemptRecord{}, &ProductionTelemetryMinute{}, &ProductionRetentionGap{}, &ProductionStorageState{}}
	if err := db.Migrator().DropTable(models...); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Migrator().DropTable(models...); err != nil {
			t.Error(err)
		}
	})
	if err := initProductionStorage(db); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestNetworkProductionStorage(t *testing.T) {
	for _, driver := range []string{"mysql", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			db := networkProductionDB(t, driver)
			if err := initProductionStorage(db); err != nil {
				t.Fatal("repeated migration", err)
			}
			if err := db.Model(&ProductionStorageState{}).Where("id=1").Updates(map[string]any{"max_rows": 1, "max_bytes": 7}).Error; err != nil {
				t.Fatal(err)
			}
			row := ProductionAttemptRecord{ID: "Case", Payload: "中🙂"}
			var duplicateWG sync.WaitGroup
			start := make(chan struct{})
			for range 8 {
				duplicateWG.Go(func() {
					<-start
					copy := row
					if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&copy).Error; err != nil {
						t.Error("concurrent duplicate at capacity", err)
					}
				})
			}
			close(start)
			duplicateWG.Wait()
			if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
				t.Fatal("duplicate at capacity", err)
			}
			if err := db.Create(&ProductionAttemptRecord{ID: "case", Payload: "x"}).Error; err == nil {
				t.Fatal("capacity was not enforced")
			}
			state, err := ReadProductionStorage()
			if err != nil || state.Rows != 1 || state.Bytes != 7 {
				t.Fatal("incorrect UTF-8 accounting", state, err)
			}
			rollback := errors.New("rollback")
			if err := db.Transaction(func(tx *gorm.DB) error {
				if err := tx.Delete(&row).Error; err != nil {
					return err
				}
				return rollback
			}); !errors.Is(err, rollback) {
				t.Fatal(err)
			}
			state, err = ReadProductionStorage()
			if err != nil || state.Rows != 1 || state.Bytes != 7 {
				t.Fatal("rollback changed counters", state, err)
			}
			if err := db.Delete(&row).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Model(&ProductionStorageState{}).Where("id=1").Updates(map[string]any{"max_rows": 5, "max_bytes": 100}).Error; err != nil {
				t.Fatal(err)
			}
			var wg sync.WaitGroup
			for i := 0; i < 20; i++ {
				wg.Go(func() { _ = db.Create(&ProductionAttemptRecord{ID: string(rune('a' + i)), Payload: "x"}).Error })
			}
			wg.Wait()
			state, err = ReadProductionStorage()
			var count int64
			if e := db.Model(&ProductionAttemptRecord{}).Count(&count).Error; e != nil {
				t.Fatal(e)
			}
			if err != nil || state.Rows != 5 || count != 5 || state.Bytes != 5 {
				t.Fatal("concurrent admission exceeded budget or lost counters", state, count, err)
			}
		})
	}
}

func TestNetworkProductionPublication(t *testing.T) {
	for _, driver := range []string{"mysql", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			db := networkProductionDB(t, driver)
			now := time.Now().Truncate(time.Minute)
			minute := now.Add(-time.Minute).Unix()
			rows := []ProductionAttemptRecord{
				{ID: "Case", BatchID: "batch", Minute: minute, Payload: `{}`},
				{ID: "case", BatchID: "batch", Minute: minute, Payload: `{}`},
				{ID: "third", BatchID: "batch", Minute: minute, Payload: `{}`},
			}
			if err := StageProductionAttempts(rows); err != nil {
				t.Fatal(err)
			}
			if err := StageProductionAttempts(rows); err != nil {
				t.Fatal("immutable retry", err)
			}
			visible, err := ReadProductionAttempts(minute, minute, 1, 10)
			if err != nil || len(visible) != 0 {
				t.Fatal("unpublished evidence leaked", visible, err)
			}
			if err := db.Model(&ProductionStorageState{}).Where("id=1").Update("max_rows", 2).Error; err != nil {
				t.Fatal(err)
			}
			if n, err := pruneProductionChunk(now); err != nil || n != 2 {
				t.Fatal("first bounded retention", n, err)
			}
			if err := db.Model(&ProductionStorageState{}).Where("id=1").Update("max_bytes", 0).Error; err != nil {
				t.Fatal(err)
			}
			if n, err := pruneProductionChunk(now); err != nil || n != 1 {
				t.Fatal("repeated gap audit", n, err)
			}
			manifest := ProductionTelemetryMinute{Minute: minute, BatchID: "batch", Complete: true, Payload: `{}`, AttemptCount: 3}
			var wg sync.WaitGroup
			for range 4 {
				wg.Go(func() {
					if err := publishProductionMinute(manifest, now); err != nil {
						t.Error(err)
					}
				})
			}
			wg.Wait()
			published, err := ReadProductionTelemetry(minute, minute)
			if err != nil || len(published) != 1 || published[0].Complete {
				t.Fatal("retention loss published as complete", published, err)
			}
			manifest.BatchID = "other"
			manifest.AttemptCount = 0
			if err := publishProductionMinute(manifest, now); err == nil {
				t.Fatal("conflicting collector overwrote publication")
			}
		})
	}
}
