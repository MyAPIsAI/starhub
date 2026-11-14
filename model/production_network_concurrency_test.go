package model

import (
	"context"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestNetworkProductionLockCancellationAndDeadlockRetry(t *testing.T) {
	for _, driver := range []string{"mysql", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			db := networkProductionDB(t, driver)
			held := db.Begin()
			if err := lockProductionStorage(held); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
			err := StageProductionAttemptsContext(ctx, []ProductionAttemptRecord{{ID: "blocked", BatchID: "batch", Minute: 60, Payload: `{}`}})
			cancel()
			if err == nil {
				t.Error("blocked write ignored cancellation")
			}
			_ = held.Rollback().Error
			state, err := ReadProductionStorage()
			if err != nil || state.Rows != 0 || state.Bytes != 0 {
				t.Fatal("canceled stage consumed capacity", state, err)
			}
			if err := db.AutoMigrate(&RoutingWriteGuard{}); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Migrator().DropTable(&RoutingWriteGuard{}) })
			if err := db.Create(&[]RoutingWriteGuard{{ID: 1}, {ID: 2}}).Error; err != nil {
				t.Fatal(err)
			}
			before := ProductionDatabaseRetries()
			a, b := make(chan struct{}), make(chan struct{})
			var wg sync.WaitGroup
			for i := range 2 {
				wg.Go(func() {
					attempts := 0
					err := productionTransaction(db, func(tx *gorm.DB) error {
						attempts++
						if err := tx.Model(&RoutingWriteGuard{}).Where("id=?", i+1).UpdateColumn("id", gorm.Expr("id")).Error; err != nil {
							return err
						}
						if attempts == 1 {
							peer := a
							if i == 0 {
								close(a)
								peer = b
							} else {
								close(b)
							}
							select {
							case <-peer:
							case <-tx.Statement.Context.Done():
								return tx.Statement.Context.Err()
							}
						}
						if err := tx.Model(&RoutingWriteGuard{}).Where("id=?", 2-i).UpdateColumn("id", gorm.Expr("id")).Error; err != nil {
							return err
						}
						return tx.Model(&ProductionStorageState{}).Where("id=1").UpdateColumn("dropped", gorm.Expr("dropped+1")).Error
					})
					if err != nil {
						t.Error("deadlock did not recover", err)
					}
				})
			}
			wg.Wait()
			state, err = ReadProductionStorage()
			if err != nil || state.Dropped != 2 || ProductionDatabaseRetries() <= before {
				t.Fatal("whole transaction retry was not observed", state, err)
			}
		})
	}
}

func TestNetworkPublicationRacesRetentionWithoutFalseCompleteness(t *testing.T) {
	for _, driver := range []string{"mysql", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			db := networkProductionDB(t, driver)
			now := time.Now().Truncate(time.Minute)
			minute := now.Add(-time.Minute).Unix()
			rows := []ProductionAttemptRecord{{ID: "one", BatchID: "batch", Minute: minute, Payload: `{}`}, {ID: "two", BatchID: "batch", Minute: minute, Payload: `{}`}}
			if err := StageProductionAttempts(rows); err != nil {
				t.Fatal(err)
			}
			if err := db.Model(&ProductionStorageState{}).Where("id=1").Update("max_rows", 1).Error; err != nil {
				t.Fatal(err)
			}
			var wg sync.WaitGroup
			wg.Go(func() {
				if err := publishProductionMinute(ProductionTelemetryMinute{Minute: minute, BatchID: "batch", Complete: true, AttemptCount: 2, Payload: `{}`}, now); err != nil {
					t.Error(err)
				}
			})
			wg.Go(func() {
				if _, err := pruneProductionChunk(now); err != nil {
					t.Error(err)
				}
			})
			wg.Wait()
			var gaps int64
			if err := db.Model(&ProductionRetentionGap{}).Count(&gaps).Error; err != nil {
				t.Fatal(err)
			}
			var published ProductionTelemetryMinute
			if err := db.First(&published, minute).Error; err != nil || published.Complete != (gaps == 0) {
				t.Fatal("false complete publication", published.Complete, gaps, err)
			}
			state, err := ReadProductionStorage()
			if err != nil || state.Rows != 1 || state.Dropped != 1 {
				t.Fatal(state, err)
			}
		})
	}
}
