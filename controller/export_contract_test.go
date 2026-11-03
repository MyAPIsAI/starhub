package controller

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"starhub/common"
	"starhub/model"
)

func TestUI36LocalizedCSVGuardPrecisionAndDictionary(t *testing.T) {
	for _, language := range []string{"zh", "en", "ar"} {
		t.Run(language, func(t *testing.T) {
			newTestDB(t)
			user := newUser(t, "csv-owner", common.RoleUser, 10000)
			key := mustToken(t, user.ID, 1000)
			for _, prefix := range []string{"=", "+", "-", "@", "\t", "\r"} {
				if err := model.InsertLog(&model.Log{UserID: user.ID, TokenID: key.ID, TokenName: prefix + "SUM(A1)", Model: "model", Quota: 1}); err != nil {
					t.Fatal(err)
				}
			}
			spec, err := normalizeExport(createExportRequest{Type: "requests", RangeDays: 7, Language: language}, user.ID, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			data, dictionary, count, err := generateExport(model.DB, spec)
			if err != nil || count != 6 {
				t.Fatalf("export failed: %d %v", count, err)
			}
			if !strings.HasPrefix(string(data), "\ufeff") || !strings.HasPrefix(string(dictionary), "\ufeff") {
				t.Fatal("export files require UTF-8 BOM")
			}
			rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(data), "\ufeff"))).ReadAll()
			if err != nil {
				t.Fatal(err)
			}
			for _, row := range rows[1:] {
				if !strings.HasPrefix(row[3], "'") || row[11] != "0.000002000" || row[12] != "2000" {
					t.Fatalf("unsafe or inexact CSV row: %q", row)
				}
			}
			if language == "zh" && rows[0][0] != "追踪编号" {
				t.Fatal("header language ignored")
			}
			if language == "ar" && rows[0][0] != "معرّف التتبع" {
				t.Fatal("Arabic header ignored")
			}
			if !strings.Contains(string(dictionary), "ISO 8601") || !strings.Contains(string(dictionary), "nine decimal places") {
				t.Fatal("field dictionary omits formats or units")
			}
		})
	}
}

func TestUI36OpaqueDownloadExpiresAndNeverDisclosesForeignFiles(t *testing.T) {
	newTestDB(t)
	owner := newUser(t, "export-owner", common.RoleUser, 1000)
	other := newUser(t, "export-other", common.RoleRoot, 1000)
	row := model.ExportRecord{UserID: owner.ID, Type: "ledger", Content: []byte("private export"), Dictionary: []byte("private dictionary")}
	if err := model.InsertExport(&row); err != nil {
		t.Fatal(err)
	}
	for _, dictionary := range []bool{false, true} {
		path := "/api/export/files/" + row.DownloadToken + "/download"
		if dictionary {
			path = "/api/export/files/" + row.DownloadToken + "/dictionary"
		}
		for _, id := range []int64{owner.ID, other.ID} {
			response := call(DownloadExport, testRequest{method: "GET", target: path, userID: id, role: common.RoleRoot, path: map[string]string{"token": row.DownloadToken}})
			if id == owner.ID && response.Code != 200 {
				t.Fatal("owner cannot download")
			}
			if id != owner.ID && response.Code != 404 {
				t.Fatal("foreign export existence disclosed")
			}
		}
	}
	if err := model.DB.Model(&row).UpdateColumn("expires_at", time.Now().Unix()).Error; err != nil {
		t.Fatal(err)
	}
	response := call(DownloadExport, testRequest{method: "GET", target: "/api/export/files/" + row.DownloadToken + "/download", userID: owner.ID, path: map[string]string{"token": row.DownloadToken}})
	if response.Code != 404 {
		t.Fatal("expired file remained downloadable without visiting the list")
	}
}

func TestUI36LargeExportRetainsAllRowsAndQueuedSnapshot(t *testing.T) {
	newTestDB(t)
	owner := newUser(t, "large-export", common.RoleUser, 1000)
	entries := make([]model.LedgerEntry, 10001)
	for i := range entries {
		entries[i] = model.LedgerEntry{UserID: owner.ID, Type: "adjustment", Amount: 1, BalanceAfter: int64(i + 1), Description: "large export"}
	}
	if err := model.DB.CreateInBatches(&entries, 500).Error; err != nil {
		t.Fatal(err)
	}
	response := call(CreateExport, testRequest{method: http.MethodPost, target: "/api/export", userID: owner.ID, body: `{"type":"ledger","range_days":31,"language":"en"}`})
	var rec model.ExportRecord
	decodeData(t, response, &rec)
	deadline := time.Now().Add(15 * time.Second)
	for {
		fresh, err := model.GetExport(rec.ID)
		if err != nil {
			t.Fatal(err)
		}
		if fresh.Status == "completed" {
			rec = *fresh
			break
		}
		if fresh.Status == "failed" || time.Now().After(deadline) {
			t.Fatalf("large export failed: %+v", fresh)
		}
		time.Sleep(10 * time.Millisecond)
	}
	rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(rec.Content), "\ufeff"))).ReadAll()
	if err != nil || len(rows) != 10002 || rec.RowCount != 10001 {
		t.Fatalf("large export truncated: rows=%d metadata=%d err=%v", len(rows), rec.RowCount, err)
	}
	var spec exportSpec
	if json.Unmarshal([]byte(rec.Specification), &spec) != nil || !spec.Snapshot || spec.MaxLedgerID != entries[len(entries)-1].ID {
		t.Fatal("queued export did not capture its upper row bound")
	}
	if err := model.DB.Create(&model.LedgerEntry{UserID: owner.ID, Type: "adjustment", Amount: 9}).Error; err != nil {
		t.Fatal(err)
	}
	_, _, count, err := generateExport(model.DB, spec)
	if err != nil || count != 10001 {
		t.Fatal("queued snapshot expanded after later ledger changes")
	}
}

func TestUI36LargeRequestAndUsageExportsReadEveryKeysetBatch(t *testing.T) {
	newTestDB(t)
	owner := newUser(t, "large-requests", common.RoleUser, 1000)
	logs := make([]model.Log, 10001)
	for i := range logs {
		logs[i] = model.Log{UserID: owner.ID, Model: "large-model", TokenName: "large-key", Quota: 1, PromptTokens: 2, CompletionTokens: 1}
	}
	if err := model.DB.CreateInBatches(&logs, 500).Error; err != nil {
		t.Fatal(err)
	}
	data, count, err := buildLogExport("requests", owner.ID, time.Now().Add(-time.Hour))
	if err != nil || count != 10001 {
		t.Fatalf("request export truncated keyset pages: count=%d err=%v", count, err)
	}
	rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(data), "\ufeff"))).ReadAll()
	if err != nil || len(rows) != 10002 {
		t.Fatal("request file differs from its row count")
	}
	data, count, err = buildLogExport("usage", owner.ID, time.Now().Add(-time.Hour))
	if err != nil || count != 1 {
		t.Fatalf("usage aggregation failed: %d %v", count, err)
	}
	rows, err = csv.NewReader(strings.NewReader(strings.TrimPrefix(string(data), "\ufeff"))).ReadAll()
	if err != nil || rows[1][3] != "10001" || rows[1][7] != "20002000" {
		t.Fatalf("usage export truncated underlying requests: %q %v", rows, err)
	}
}

func TestUI36FiltersCalendarBoundsAndAbandonedJobRecovery(t *testing.T) {
	newTestDB(t)
	owner := newUser(t, "recover-export", common.RoleUser, 1000)
	now := time.Now()
	for _, input := range []createExportRequest{{Type: "ledger", StartDate: "2026-02-30", EndDate: "2026-03-01"}, {Type: "requests", StartDate: "2026-01-01", EndDate: "2026-02-14"}, {Type: "usage", RangeDays: 32}} {
		if _, err := normalizeExport(input, owner.ID, now); err == nil {
			t.Fatal("invalid date range accepted")
		}
	}
	for _, status := range []int{1, 2} {
		if err := model.InsertLog(&model.Log{UserID: owner.ID, Model: "selected-model", Status: status, Quota: 23}); err != nil {
			t.Fatal(err)
		}
	}
	spec, err := normalizeExport(createExportRequest{Type: "requests", RangeDays: 31, Status: "failed", Query: "selected-model", Language: "en"}, owner.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(spec)
	row := model.ExportRecord{UserID: owner.ID, Type: "requests", RangeDays: 31, Specification: string(encoded), Status: "processing", LeaseUntil: now.Add(-time.Minute).Unix()}
	if err := model.InsertExport(&row); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); RunExportJobs(ctx) }()
	deadline := time.Now().Add(10 * time.Second)
	for {
		fresh, err := model.GetExport(row.ID)
		if err != nil {
			t.Fatal(err)
		}
		if fresh.Status == "completed" {
			if fresh.RowCount != 1 {
				t.Fatal("status/search filters were ignored")
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("abandoned export was not recovered")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	<-done
}
