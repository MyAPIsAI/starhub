package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"starhub/metric"
	"starhub/model"
)

// Run network suites serially across packages against disposable databases.
func TestNetworkProductionCollectionPublicationDeliveryAndRestart(t *testing.T) {
	for _, driver := range []string{"mysql", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			env := "STARHUB_TEST_MYSQL_DSN"
			if driver == "postgres" {
				env = "STARHUB_TEST_POSTGRES_DSN"
			}
			dsn := os.Getenv(env)
			if dsn == "" {
				t.Skip("disposable database DSN not supplied")
			}
			previous := model.DB
			cfg := model.DatabaseConfig{Driver: driver, DSN: dsn, MaxOpenConns: 8, MaxIdleConns: 2}
			if err := model.InitDBWithConfig(cfg); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if model.DB != nil {
					tables, err := model.DB.Migrator().GetTables()
					if err != nil {
						t.Error(err)
					}
					for _, table := range tables {
						if err := model.DB.Migrator().DropTable(table); err != nil {
							t.Error(err)
						}
					}
				}
				_ = model.CloseDB()
				model.DB = previous
				metric.Reset()
			})
			metric.Reset()
			now := time.Now().Truncate(time.Minute)
			metric.StartProduction(now.Add(-time.Minute))
			request := metric.BeginRequest(metric.RequestMeta{Model: "network-model", Stream: true})
			attempt := request.BeginAttempt(7)
			attempt.SetRouteMetadata(metric.AttemptMetadata{Generation: "generation", ResourceKey: "7:m", Capability: "chat", Group: "default"})
			attempt.MarkSent()
			attempt.RecordUsage(10, 2)
			attempt.RecordFirstToken()
			request.EndAttempt(attempt, metric.AttemptSuccess, 200)
			if err := StageProductionTelemetryContext(t.Context()); err != nil {
				t.Fatal(err)
			}
			rows, err := model.ReadProductionAttempts(now.Unix(), now.Unix(), 1, 10)
			if err != nil || len(rows) != 0 {
				t.Fatal("open minute leaked", rows, err)
			}
			seal := now.Add(time.Minute)
			if err := FlushProductionTelemetryContext(t.Context(), seal); err != nil {
				t.Fatal(err)
			}
			rows, err = model.ReadProductionAttempts(now.Unix(), now.Unix(), 1, 10)
			if err != nil || len(rows) != 1 {
				t.Fatal("published evidence missing", len(rows), err)
			}
			id := rows[0].ID
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var batch struct {
					Attempts []struct {
						ID string `json:"attempt_id"`
					} `json:"attempts"`
				}
				if err := json.NewDecoder(r.Body).Decode(&batch); err != nil || len(batch.Attempts) != 1 || batch.Attempts[0].ID != id {
					t.Error("unstable delivery evidence", err)
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"accepted_ids": []string{id}})
			}))
			defer server.Close()
			t.Setenv("RESOURCE_CATALOG_NOTIFY_URL", server.URL)
			if err := model.DB.Model(&model.ProductionAttemptRecord{}).Where("id=?", id).Update("next_attempt_at", 0).Error; err != nil {
				t.Fatal(err)
			}
			if err := DeliverProductionAttemptsOnce(t.Context()); err != nil {
				t.Fatal(err)
			}
			h := ProductionTelemetryHealth(time.Now())
			if !h.Durable.Available || h.Durable.Backend != driver || h.Durable.Pending != 0 || h.Durable.Storage.Rows != 1 {
				t.Fatal("wrong durable health", h.Durable)
			}
			if err := model.CloseDB(); err != nil {
				t.Fatal(err)
			}
			if err := model.InitDBWithConfig(cfg); err != nil {
				t.Fatal(err)
			}
			metric.Reset()
			buckets, err := ProductionMetricBuckets(now.Unix(), now.Unix(), seal)
			if err != nil || len(buckets) != 1 {
				t.Fatal("restart lost history", err)
			}
			items := buckets[0]["models"].([]map[string]any)
			if len(items) != 2 || items[len(items)-1]["coverage_status"] != "complete" {
				t.Fatal("incorrect history coverage", items)
			}
			quarter, coverage, err := ProductionQuarterMetrics(true, now.Unix(), now.Unix(), seal)
			if err != nil || len(quarter) != 1 || quarter[0].AttemptCount != 1 || len(coverage) != 1 || coverage[0].Complete {
				t.Fatal("incorrect quarter projection", quarter, coverage, err)
			}
		})
	}
}
