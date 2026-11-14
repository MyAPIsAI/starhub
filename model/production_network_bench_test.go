package model

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
	"starhub/metric"
)

// This fixed local load measures contention, not production relay capacity.
func BenchmarkNetworkProductionWriterContention(b *testing.B) {
	for _, driver := range []string{"mysql", "postgres"} {
		for _, mode := range []string{"baseline", "shared", "idle"} {
			b.Run(driver+"/"+mode, func(b *testing.B) {
				env := "STARHUB_TEST_MYSQL_DSN"
				if driver == "postgres" {
					env = "STARHUB_TEST_POSTGRES_DSN"
				}
				if os.Getenv(env) == "" {
					b.Skip("disposable database DSN not supplied")
				}
				if err := InitDBWithConfig(DatabaseConfig{Driver: driver, DSN: os.Getenv(env), MaxOpenConns: 8, MaxIdleConns: 8}); err != nil {
					b.Fatal(err)
				}
				b.Cleanup(func() {
					tables, err := DB.Migrator().GetTables()
					if err != nil {
						b.Error(err)
					}
					for _, table := range tables {
						if err := DB.Migrator().DropTable(table); err != nil {
							b.Error(err)
						}
					}
					_ = CloseDB()
				})
				user := &User{Username: "benchmark", AffCode: "benchmark", Quota: 1000000000}
				if err := CreateUser(user); err != nil {
					b.Fatal(err)
				}
				for range 8 {
					if ok, err := DecreaseUserQuota(user.ID, 1); err != nil || !ok {
						b.Fatal(ok, err)
					}
				}
				stop, done := make(chan struct{}), make(chan struct{})
				failures := make(chan error, 1)
				var staged int64
				if mode == "shared" {
					go func() {
						defer close(done)
						ticker := time.NewTicker(5 * time.Millisecond)
						defer ticker.Stop()
						for sequence := 0; ; sequence++ {
							select {
							case <-stop:
								return
							case <-ticker.C:
							}
							batch := fmt.Sprintf("load-%d", sequence)
							rows := make([]ProductionAttemptRecord, 128)
							for i := range rows {
								rows[i] = ProductionAttemptRecord{ID: fmt.Sprintf("%s-%d", batch, i), BatchID: batch, Minute: 60, Payload: strings.Repeat("x", 1024)}
							}
							if err := StageProductionAttempts(rows); err != nil {
								failures <- err
								return
							}
							staged += int64(len(rows))
							if err := productionTransaction(DB, func(tx *gorm.DB) error {
								if err := lockProductionStorage(tx); err != nil {
									return err
								}
								return tx.Where("batch_id=?", batch).Delete(&ProductionAttemptRecord{}).Error
							}); err != nil {
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
				for range b.N {
					at := time.Now()
					var err error
					if mode == "idle" {
						_, err = PruneProductionTelemetry(time.Now())
						if err == nil {
							_, err = DueProductionAttempts(b.Context(), time.Now())
						}
					} else {
						var ok bool
						ok, err = DecreaseUserQuota(user.ID, 1)
						if !ok && err == nil {
							err = ErrUserQuotaShort
						}
					}
					latency.Observe(time.Since(at))
					if err != nil {
						close(stop)
						<-done
						b.Fatal(err)
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
				debits := b.N
				if mode == "idle" {
					debits = 0
				}
				if err != nil || after.Quota != user.Quota-int64(debits)-8 {
					b.Fatal("incorrect benchmark balance", after, err)
				}
				if err := VerifyMoneyLedger(user.ID); err != nil {
					b.Fatal(err)
				}
				s := latency.Snapshot()
				b.ReportMetric(float64(s.P95US), "p95_us")
				b.ReportMetric(float64(s.P99US), "p99_us")
				b.ReportMetric(float64(staged)/elapsed.Seconds(), "staged_rows/s")
			})
		}
	}
}
