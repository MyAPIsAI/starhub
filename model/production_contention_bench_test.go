package model

import (
	"fmt"
	"path/filepath"
	"starhub/metric"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// This synthetic local benchmark compares a real quota debit transaction with
// and without bounded staging/delete churn. It is not a relay capacity test.
func BenchmarkProductionWriterContention(b *testing.B) {
	for _, mode := range []string{"baseline", "shared_sqlite", "separate_sqlite"} {
		b.Run(mode, func(b *testing.B) {
			if err := InitDB(filepath.Join(b.TempDir(), "accounting.db")); err != nil {
				b.Fatal(err)
			}
			b.Cleanup(func() { _ = CloseDB() })
			user := &User{Username: "benchmark", Quota: 1000000000, Status: 1, Group: "default"}
			if err := CreateUser(user); err != nil {
				b.Fatal(err)
			}
			const warmup = 64
			for i := 0; i < warmup; i++ {
				if ok, err := DecreaseUserQuota(user.ID, 1); err != nil || !ok {
					b.Fatal(ok, err)
				}
			}
			target := DB
			if mode == "separate_sqlite" {
				var err error
				target, err = gorm.Open(sqlite.Open(dsn(filepath.Join(b.TempDir(), "telemetry.db"))), &gorm.Config{Logger: DB.Logger})
				if err != nil {
					b.Fatal(err)
				}
				if err := target.AutoMigrate(&ProductionAttemptRecord{}, &ProductionTelemetryMinute{}); err != nil {
					b.Fatal(err)
				}
				if err := initProductionStorage(target); err != nil {
					b.Fatal(err)
				}
				sqlDB, err := target.DB()
				if err != nil {
					b.Fatal(err)
				}
				b.Cleanup(func() { _ = sqlDB.Close() })
			}
			stop, done := make(chan struct{}), make(chan struct{})
			failures := make(chan error, 1)
			var staged int64
			if mode != "baseline" {
				go func() {
					defer close(done)
					ticker := time.NewTicker(time.Millisecond)
					defer ticker.Stop()
					for seq := 0; ; seq++ {
						select {
						case <-stop:
							return
						case <-ticker.C:
						}
						batch := fmt.Sprintf("benchmark-%d", seq)
						rows := make([]ProductionAttemptRecord, 128)
						for i := range rows {
							rows[i] = ProductionAttemptRecord{ID: fmt.Sprintf("%s-%d", batch, i), BatchID: batch, Minute: 60, Payload: `{"padding":"` + strings.Repeat("x", 1000) + `"}`}
						}
						if err := stageProductionAttempts(target, rows); err != nil {
							failures <- err
							return
						}
						staged += int64(len(rows))
						// Deleting the completed synthetic chunk bounds the fixture;
						// production retention is exercised by separate fault tests.
						if err := target.Where("batch_id = ?", batch).Delete(&ProductionAttemptRecord{}).Error; err != nil {
							failures <- err
							return
						}
					}
				}()
			} else {
				close(done)
			}
			var latency metric.DurationHistogram
			b.ResetTimer()
			started := time.Now()
			for i := 0; i < b.N; i++ {
				at := time.Now()
				ok, err := DecreaseUserQuota(user.ID, 1)
				latency.Observe(time.Since(at))
				if err != nil || !ok {
					close(stop)
					<-done
					b.Fatal(ok, err)
				}
			}
			elapsed := time.Since(started)
			b.StopTimer()
			close(stop)
			<-done
			select {
			case err := <-failures:
				b.Fatal(err)
			default:
			}
			after, err := GetUserByID(user.ID)
			if err != nil || after.Quota != user.Quota-int64(b.N)-warmup {
				b.Fatal("incorrect accounting balance", after, err)
			}
			var entries int64
			if err := DB.Model(&MoneyEntry{}).Where("user_id = ? AND kind = ?", user.ID, "legacy_debit").Count(&entries).Error; err != nil {
				b.Fatal(err)
			}
			if entries != int64(b.N)+warmup {
				b.Fatalf("debit journal count %d, want %d", entries, b.N+warmup)
			}
			s := latency.Snapshot()
			b.ReportMetric(float64(s.P95US), "debit_p95_us")
			b.ReportMetric(float64(s.P99US), "debit_p99_us")
			b.ReportMetric(float64(s.MaxUS), "debit_max_us")
			b.ReportMetric(float64(staged)/elapsed.Seconds(), "staged_rows/s")
		})
	}
}
