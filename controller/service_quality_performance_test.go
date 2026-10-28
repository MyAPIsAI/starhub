package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"starhub/common"
	"starhub/model"
)

// Run explicitly with STARHUB_TEST_LARGE_LOGS=1. The local acceptance profile
// covers one million logs, 100 models, 1000 users and 30 days on pure-Go SQLite.
func TestServiceQualityLargeLogPerformance(t *testing.T) {
	if os.Getenv("STARHUB_TEST_LARGE_LOGS") != "1" {
		t.Skip("set STARHUB_TEST_LARGE_LOGS=1 to run the million-log acceptance profile")
	}
	newTestDB(t)
	const count = 1000000
	now := time.Now().Unix()
	start := time.Now()
	if err := model.DB.Exec(`WITH RECURSIVE sequence(n) AS (
		SELECT 1 UNION ALL SELECT n+1 FROM sequence WHERE n < ?
	) INSERT INTO logs (user_id, model, requested_model, protocol, duration_ms, quota, status, created_at)
	SELECT n % 1000 + 1, printf('model-%03d', (n / 1000) % 100), 'myapis-auto', 'OpenAI',
		n % 10000 + 1, 5, CASE WHEN n % 10 = 0 THEN 2 ELSE 1 END,
		? - (n % 720) * 3600 - 1800 FROM sequence`, count, now).Error; err != nil {
		t.Fatal(err)
	}
	t.Logf("profile rows=%d models=100 users=1000 hours=720 seed_ms=%d go=%s os=%s arch=%s cpus=%d", count, time.Since(start).Milliseconds(), runtime.Version(), runtime.GOOS, runtime.GOARCH, runtime.NumCPU())
	model.DB = model.DB.Session(&gorm.Session{Logger: qualityPerformanceLogger{Interface: logger.Discard, t: t}})
	for _, tc := range []struct {
		name   string
		userID int64
		hours  int
		model  string
	}{
		{"global-30d", 0, 720, ""},
		{"global-7d", 0, 168, ""},
		{"global-24h", 0, 24, ""},
		{"model-30d", 0, 720, "model-001"},
		{"user-30d", 1, 720, ""},
		{"user-model-30d", 1, 720, "model-001"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var total, success, quota int64
			durations := make([]int64, 0)
			for n := 1; n <= count; n++ {
				if n%720 >= tc.hours || (tc.userID > 0 && int64(n%1000+1) != tc.userID) || (tc.model != "" && fmt.Sprintf("model-%03d", (n/1000)%100) != tc.model) {
					continue
				}
				total++
				quota += 5
				if n%10 != 0 {
					success++
				}
				durations = append(durations, int64(n%10000+1))
			}
			slices.Sort(durations)
			p95 := int64(0)
			if len(durations) > 0 {
				p95 = durations[(len(durations)*95+99)/100-1]
			}
			for trial := 0; trial < 3; trial++ {
				target := fmt.Sprintf("/api/service-quality?hours=%d&model=%s", tc.hours, tc.model)
				role := common.RoleAdmin
				userID := int64(1)
				if tc.userID > 0 {
					role = common.RoleUser
					userID = tc.userID
				}
				runtime.GC()
				var before, after runtime.MemStats
				runtime.ReadMemStats(&before)
				started := time.Now()
				rec := call(GetServiceQuality, testRequest{target: target, userID: userID, role: role})
				elapsed := time.Since(started)
				runtime.ReadMemStats(&after)
				if rec.Code != 200 {
					t.Fatalf("status=%d body=%s", rec.Code, rec.Body)
				}
				var envelope struct {
					Success bool                        `json:"success"`
					Data    model.ServiceQualitySummary `json:"data"`
				}
				if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
					t.Fatal(err)
				}
				got := envelope.Data
				if !envelope.Success || got.Overall.TotalRequests != total || got.Overall.SuccessCount != success || got.Overall.FailedCount != total-success || got.Overall.TotalQuota != quota || got.Overall.P95DurationMs != p95 {
					t.Fatalf("large-log metrics mismatch: got=%+v total=%d success=%d quota=%d p95=%d", got.Overall, total, success, quota, p95)
				}
				var modelTotal, dayTotal int64
				for _, row := range got.ByModel {
					modelTotal += row.TotalRequests
				}
				for _, row := range got.ByDay {
					dayTotal += row.TotalRequests
				}
				if modelTotal != total || dayTotal != total {
					t.Fatalf("scope totals diverged: model=%d day=%d total=%d", modelTotal, dayTotal, total)
				}
				t.Logf("sample trial=%d elapsed_ms=%d response_bytes=%d allocated_bytes=%d total=%d success=%d p95_ms=%d", trial+1, elapsed.Milliseconds(), rec.Body.Len(), after.TotalAlloc-before.TotalAlloc, total, success, p95)
				if elapsed > 5*time.Second {
					t.Errorf("local query/response budget exceeded: %s > 5s", elapsed)
				}
				if rec.Body.Len() > 128*1024 {
					t.Errorf("aggregate response exceeds 128KiB: %d", rec.Body.Len())
				}
			}
		})
	}
}

type qualityPerformanceLogger struct {
	logger.Interface
	t *testing.T
}

func (l qualityPerformanceLogger) Trace(_ context.Context, begin time.Time, sql func() (string, int64), err error) {
	query, rows := sql()
	operation := "aggregate"
	if strings.Contains(query, "WITH samples") {
		operation = "percentile"
	}
	l.t.Logf("sql operation=%s elapsed_ms=%d rows=%d error=%v", operation, time.Since(begin).Milliseconds(), rows, err)
}
