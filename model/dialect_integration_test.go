package model

import (
	"os"
	"testing"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// These opt-in tests use disposable databases containing only a log fixture.
// They verify SQL projections, not full application migration or concurrency.
func TestSQLDialectStatistics(t *testing.T) {
	for _, driver := range []string{"mysql", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			env := "STARHUB_TEST_MYSQL_DSN"
			if driver == "postgres" {
				env = "STARHUB_TEST_POSTGRES_DSN"
			}
			connection := os.Getenv(env)
			if connection == "" {
				t.Skip("disposable database DSN not supplied")
			}
			var dialector gorm.Dialector = mysql.Open(connection)
			if driver == "postgres" {
				dialector = postgres.Open(connection)
			}
			db, err := gorm.Open(dialector, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			if err != nil {
				t.Fatal(err)
			}
			sqlDB, err := db.DB()
			if err != nil {
				t.Fatal(err)
			}
			sqlDB.SetMaxOpenConns(1)
			t.Cleanup(func() { _ = sqlDB.Close() })
			previous := DB
			DB = db
			t.Cleanup(func() { DB = previous })
			if err := db.Exec("CREATE TABLE logs (model VARCHAR(128), user_id BIGINT, token_id BIGINT, token_name VARCHAR(64), created_at BIGINT, admitted_at BIGINT, admission_day BIGINT, business_date VARCHAR(10), duration_ms BIGINT, status INTEGER, prompt_tokens BIGINT, completion_tokens BIGINT, quota BIGINT, quota_remainder_nano BIGINT, unfunded_quota BIGINT, unfunded_quota_remainder_nano BIGINT)").Error; err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := db.Exec("DROP TABLE logs").Error; err != nil {
					t.Error(err)
				}
			})
			zone := "SET time_zone='+08:00'"
			if driver == "postgres" {
				zone = "SET TIME ZONE 'Asia/Shanghai'"
			}
			if err := db.Exec(zone).Error; err != nil {
				t.Fatal(err)
			}
			var date string
			if err := db.Raw("SELECT " + DateFromUnix("1735774200")).Scan(&date).Error; err != nil || date != "2025-01-01" {
				t.Fatal("session timezone changed UTC date", date, err)
			}
			midnight := time.Now().UTC().Truncate(24 * time.Hour)
			for i := 1; i <= 20; i++ {
				at := midnight.Add(time.Duration(i%2*20+1) * time.Hour).Unix()
				if err := db.Exec("INSERT INTO logs VALUES (?,1,7,'test',?,0,0,'',?,1,2,3,1,0,0,0)", "model", at, i*100).Error; err != nil {
					t.Fatal(err)
				}
			}
			quality, err := SummarizeServiceQuality(1, 24)
			if err != nil {
				t.Fatal(err)
			}
			if quality.Overall.P95DurationMs != 1900 || quality.Overall.TotalRequests != 20 || len(quality.ByDay) != 1 || quality.ByDay[0].P95DurationMs != 1900 || quality.ByDay[0].Date != midnight.Format("2006-01-02") {
				t.Fatalf("incorrect daily counts or nearest-rank percentile: %+v", quality)
			}
			daily, err := SummarizeLogsByDay(1, midnight.Unix())
			if err != nil || len(daily) != 1 || daily[0].Requests != 20 || daily[0].Date != midnight.Format("2006-01-02") {
				t.Fatal("legacy UTC daily summary failed", daily, err)
			}
			var dates []string
			day := DateFromUnix(admissionDayColumn)
			if err := db.Model(&Log{}).Where("user_id = ? AND business_date = ''", 1).Distinct(day).Pluck(day, &dates).Error; err != nil || len(dates) != 1 || dates[0] != midnight.Format("2006-01-02") {
				t.Fatal("legacy reconciliation date projection failed", dates, err)
			}
			models, err := SummarizeLogsByModel(1, midnight.Unix())
			if err != nil || len(models) != 1 || models[0].Quota != 20 {
				t.Fatal("daily model money aggregation failed", models, err)
			}
			tokens, err := SummarizeLogsByToken(1, 7, midnight.Unix())
			if err != nil || len(tokens) != 1 || tokens[0].Quota != 20 {
				t.Fatal("daily token money aggregation failed", tokens, err)
			}
		})
	}
}
