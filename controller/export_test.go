package controller

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"starhub/common"
	"starhub/model"
)

func TestCreateExportValidation(t *testing.T) {
	newTestDB(t)
	u := newUser(t, "alice", common.RoleUser, 1000)
	existing := &model.ExportRecord{
		UserID:    u.ID,
		Type:      "usage",
		RangeDays: 7,
		RowCount:  1,
		Content:   []byte("Date,Requests,Prompt Tokens,Completion Tokens,Quota\n2026-10-20,1,100,50,23\n"),
	}
	if err := model.InsertExport(existing); err != nil {
		t.Fatalf("InsertExport failed: %v", err)
	}
	before, err := model.ListExports(0)
	if err != nil {
		t.Fatalf("ListExports before validation failed: %v", err)
	}
	const typeError = "type must be requests, ledger or usage"
	const rangeError = "range_days must be between 1 and 31"

	cases := []struct {
		name    string
		body    string
		message string
	}{
		{"invalid type", `{"type":"invalid","range_days":7}`, typeError},
		{"range too small", `{"type":"requests","range_days":0}`, rangeError},
		{"range too large", `{"type":"requests","range_days":32}`, rangeError},
		{"missing type", `{"range_days":7}`, typeError},
		{"negative range", `{"type":"requests","range_days":-1}`, rangeError},
		{"missing range", `{"type":"requests"}`, rangeError},
		{"malformed JSON", `{"type":"requests",`, "invalid request body:"},
		{"wrong range type", `{"type":"requests","range_days":"7"}`, "invalid request body:"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rr := call(CreateExport, testRequest{
				method: http.MethodPost,
				target: "/api/export",
				body:   tc.body,
				userID: u.ID,
				role:   common.RoleUser,
			})
			expectError(t, rr, http.StatusBadRequest, tc.message)
			after, err := model.ListExports(0)
			if err != nil {
				t.Fatalf("ListExports after validation failed: %v", err)
			}
			if !reflect.DeepEqual(after, before) {
				t.Fatalf("invalid request changed exports: got %+v, want %+v", after, before)
			}
		})
	}
}

func TestCreateExportRequests(t *testing.T) {
	newTestDB(t)
	u := newUser(t, "alice", common.RoleUser, 1000)

	// Insert a log entry so the export has data.
	l := &model.Log{
		UserID:           u.ID,
		Model:            "gpt-4o-mini",
		PromptTokens:     100,
		CompletionTokens: 50,
		Quota:            23,
		TokenName:        "test-key",
	}
	if err := model.InsertLog(l); err != nil {
		t.Fatalf("InsertLog failed: %v", err)
	}

	rr := call(CreateExport, testRequest{
		method: http.MethodPost,
		target: "/api/export",
		body:   `{"type":"requests","range_days":7}`,
		userID: u.ID,
		role:   common.RoleUser,
	})
	var got map[string]any
	decodeData(t, rr, &got)
	if got["type"] != "requests" {
		t.Fatalf("type = %v, want requests", got["type"])
	}
	if got["status"] != "completed" {
		t.Fatalf("status = %v, want completed", got["status"])
	}
	if got["row_count"].(float64) < 1 {
		t.Fatalf("row_count = %v, want >= 1", got["row_count"])
	}
	if got["filename"] == "" {
		t.Fatal("filename should not be empty")
	}
}

func TestBuildLogExportStartTimeBoundary(t *testing.T) {
	newTestDB(t)
	u := newUser(t, "alice", common.RoleUser, 1000)
	from := time.Date(2026, time.October, 20, 0, 0, 0, 0, time.UTC)
	for i, offset := range []time.Duration{-time.Second, 0, time.Second} {
		if err := model.InsertLog(&model.Log{UserID: u.ID, Model: itoa(int64(i)), Quota: 23, CreatedAt: from.Add(offset).Unix()}); err != nil {
			t.Fatal(err)
		}
	}
	content, count, err := buildLogExport("requests", u.ID, from)
	if err != nil || count != 2 {
		t.Fatalf("boundary count=%d err=%v", count, err)
	}
	rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(content), "\ufeff"))).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 || rows[1][5] != "2" || rows[2][5] != "1" || rows[2][1] != "2026-10-20T00:00:00+00:00" || rows[2][12] != "46000" {
		t.Fatalf("wrong boundary rows: %q", rows)
	}
	if count, err := model.CountLogs(u.ID); err != nil || count != 3 {
		t.Fatal("export changed source logs")
	}
}

func TestRequestExportModelAttribution(t *testing.T) {
	newTestDB(t)
	u := newUser(t, "alice", common.RoleUser, 1000)
	other := newUser(t, "bob", common.RoleUser, 1000)
	logs := []model.Log{
		{UserID: u.ID, Model: "gpt-4o", RequestedModel: "myapis-auto", Quota: 23},
		{UserID: u.ID, Model: "gpt-4o-mini", RequestedModel: "gpt-4o-mini", Quota: 7},
		{UserID: u.ID, Model: "legacy-model", Quota: 2},
		{UserID: other.ID, Model: "private-model", RequestedModel: "private-entry", Quota: 99},
	}
	for i := range logs {
		if err := model.InsertLog(&logs[i]); err != nil {
			t.Fatalf("InsertLog(%d) failed: %v", i, err)
		}
	}
	rr := call(CreateExport, testRequest{
		method: http.MethodPost,
		target: "/api/export",
		body:   `{"type":"requests","range_days":7}`,
		userID: u.ID,
		role:   common.RoleUser,
	})
	var exp model.ExportRecord
	decodeData(t, rr, &exp)
	if exp.RowCount != 3 || exp.Status != "completed" {
		t.Fatalf("export metadata = %+v, want 3 completed rows", exp)
	}
	stored, err := model.GetExport(exp.ID)
	if err != nil {
		t.Fatalf("GetExport failed: %v", err)
	}
	rr = call(DownloadExport, testRequest{
		method: http.MethodGet,
		target: "/api/export/files/" + testExportDownloadToken(t, exp.ID) + "/download",
		userID: u.ID,
		role:   common.RoleUser,
		path:   map[string]string{"token": testExportDownloadToken(t, exp.ID)},
	})
	if rr.Code != http.StatusOK || rr.Body.String() != string(stored.Content) {
		t.Fatalf("download status = %d, body differs from persisted export = %t", rr.Code, rr.Body.String() != string(stored.Content))
	}
	rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(rr.Body.String(), "\ufeff"))).ReadAll()
	if err != nil || len(rows) != 4 {
		t.Fatalf("CSV rows = %q, err = %v, want a header and 3 user rows", rows, err)
	}
	want := [][]string{[]string{"Trace ID", "Time", "Key ID", "Key name", "Requested model", "Actual model", "Status", "Input tokens", "Output tokens", "Duration (ms)", "First token (ms)", "Amount (USD)", "Amount (nano-USD)", "Source", "Vendor", "Service group", "Call type", "Automatically switched", "Error code", "Cache read tokens", "Cache write tokens", "Captured billing details"}}
	loc, _ := time.LoadLocation("Asia/Dubai")
	for i := 2; i >= 0; i-- {
		l := logs[i]
		requested := l.RequestedModel
		if requested == "" {
			requested = l.Model
		}
		nano := l.Quota * 2000
		want = append(want, []string{"log_" + itoa(l.ID), time.Unix(l.CreatedAt, 0).In(loc).Format("2006-01-02T15:04:05-07:00"), "0", "", requested, l.Model, "success", "0", "0", "0", "0", fmt.Sprintf("0.%09d", nano), itoa(nano), "api", "", "", "", "false", "", "0", "0", ""})
	}

	if !reflect.DeepEqual(rows, want) {
		t.Fatalf("CSV rows = %q, want %q", rows, want)
	}
	var after []model.Log
	if err := model.DB.Order("id ASC").Find(&after).Error; err != nil {
		t.Fatalf("read source logs failed: %v", err)
	}
	if !reflect.DeepEqual(after, logs) {
		t.Fatalf("export changed source logs: got %+v, want %+v", after, logs)
	}
}

func TestCreateExportUsage(t *testing.T) {
	newTestDB(t)
	u := newUser(t, "alice", common.RoleUser, 1000)
	loc, _ := time.LoadLocation("Asia/Dubai")
	now := time.Now().In(loc)
	day := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, loc).AddDate(0, 0, -2)
	for _, item := range []struct {
		offset time.Duration
		quota  int64
	}{{0, 23}, {time.Hour, 7}, {24 * time.Hour, 2}} {
		if err := model.InsertLog(&model.Log{UserID: u.ID, Model: "gpt-4o", Quota: item.quota, PromptTokens: 10, CompletionTokens: 5, CreatedAt: day.Add(item.offset).Unix()}); err != nil {
			t.Fatal(err)
		}
	}
	rr := call(CreateExport, testRequest{method: http.MethodPost, target: "/api/export", userID: u.ID, role: common.RoleUser, body: `{"type":"usage","range_days":7}`})
	var exp model.ExportRecord
	decodeData(t, rr, &exp)
	if exp.Status != "completed" || exp.RowCount != 2 {
		t.Fatalf("wrong daily aggregation: %+v", exp)
	}
	stored, err := model.GetExport(exp.ID)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(stored.Content), "\ufeff"))).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 || rows[1][0] != day.Format("2006-01-02") || rows[1][3] != "2" || rows[1][6] != "0.000060000" || rows[1][7] != "60000" || rows[2][7] != "4000" {
		t.Fatalf("wrong usage sums: %q", rows)
	}
}

func TestCreateExportLedger(t *testing.T) {
	newTestDB(t)
	u := newUser(t, "alice", common.RoleUser, 1000)

	// Ensure the wallet exists before resolving a recharge.
	model.GetOrCreateWallet(u.ID)

	// Create a recharge order and resolve it to generate a ledger entry.
	o := &model.RechargeOrder{
		UserID:    u.ID,
		Amount:    10_000_000_000, // $10
		Status:    "pending",
		Channel:   "card",
		ExpiresAt: 0,
	}
	model.CreateRechargeOrder(o)
	model.ResolveRechargeOrder(o.ID, "success")

	rr := call(CreateExport, testRequest{
		method: http.MethodPost,
		target: "/api/export",
		body:   `{"type":"ledger","range_days":7}`,
		userID: u.ID,
		role:   common.RoleUser,
	})
	var got map[string]any
	decodeData(t, rr, &got)
	if got["type"] != "ledger" {
		t.Fatalf("type = %v, want ledger", got["type"])
	}
	if got["row_count"].(float64) < 1 {
		t.Fatalf("row_count = %v, want >= 1", got["row_count"])
	}
}

func TestBuildLedgerExportPreservesNanoUSD(t *testing.T) {
	newTestDB(t)
	u := newUser(t, "alice", common.RoleUser, 1000)
	from := time.Date(2026, time.October, 20, 0, 0, 0, 0, time.UTC)
	entries := []model.LedgerEntry{
		{UserID: u.ID, Type: "recharge", Amount: 1, BalanceAfter: 1, Description: "one nano", OrderID: "ord-tiny", CreatedAt: from.Unix()},
		{UserID: u.ID, Type: "recharge", Amount: 1_000_000_001, BalanceAfter: 1_000_000_002, Description: "credit", OrderID: "ord-credit", CreatedAt: from.Unix()},
		{UserID: u.ID, Type: "call_summary", Amount: -2, BalanceAfter: 1_000_000_000, Description: "debit", TraceID: "trace-debit", CreatedAt: from.Unix()},
	}
	if err := model.DB.Create(&entries).Error; err != nil {
		t.Fatalf("create ledger fixtures failed: %v", err)
	}

	content, rowCount, err := buildLedgerExport(u.ID, from)
	if err != nil {
		t.Fatalf("buildLedgerExport failed: %v", err)
	}
	if rowCount != 3 {
		t.Fatalf("row_count = %d, want 3", rowCount)
	}
	rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(content), "\ufeff"))).ReadAll()
	if err != nil {
		t.Fatalf("invalid CSV: %v", err)
	}
	if len(rows) != 4 {
		t.Fatalf("CSV rows = %q, want a header and 3 data rows", rows)
	}
	if want := []string{"Ledger ID", "Time", "Type", "Amount (USD)", "Amount (nano-USD)", "Balance after (USD)", "Balance after (nano-USD)", "Description", "Order ID", "Key ID", "Requests", "Funding Source", "API Quota Eligible"}; !reflect.DeepEqual(rows[0], want) {
		t.Fatalf("CSV header = %q, want %q", rows[0], want)
	}
	wantByID := map[string][]string{
		itoa(entries[0].ID): {itoa(entries[0].ID), "2026-10-20T00:00:00+00:00", "recharge", "0.000000001", "1", "0.000000001", "1", "one nano", "ord-tiny", "0", "0", "legacy_unverified", "false"},
		itoa(entries[1].ID): {itoa(entries[1].ID), "2026-10-20T00:00:00+00:00", "recharge", "1.000000001", "1000000001", "1.000000002", "1000000002", "credit", "ord-credit", "0", "0", "legacy_unverified", "false"},
		itoa(entries[2].ID): {itoa(entries[2].ID), "2026-10-20T00:00:00+00:00", "call_summary", "-0.000000002", "-2", "1.000000000", "1000000000", "debit", "", "0", "0", "legacy_unverified", "false"},
	}

	for _, row := range rows[1:] {
		want, ok := wantByID[row[0]]
		if !ok || !reflect.DeepEqual(row, want) {
			t.Fatalf("CSV ledger row = %q, want %q", row, want)
		}
		delete(wantByID, row[0])
	}
	stored, err := model.ListLedger(u.ID)
	if err != nil {
		t.Fatalf("ListLedger failed: %v", err)
	}
	wantStored := []model.LedgerEntry{entries[2], entries[1], entries[0]}
	if !reflect.DeepEqual(stored, wantStored) {
		t.Fatalf("source ledger changed: got %+v, want %+v", stored, wantStored)
	}
}

func TestEmptyExportContainsOnlyHeader(t *testing.T) {
	cases := []struct {
		typ    string
		header []string
	}{
		{"requests", []string{"Trace ID", "Time", "Key ID", "Key name", "Requested model", "Actual model", "Status", "Input tokens", "Output tokens", "Duration (ms)", "First token (ms)", "Amount (USD)", "Amount (nano-USD)", "Source", "Vendor", "Service group", "Call type", "Automatically switched", "Error code", "Cache read tokens", "Cache write tokens", "Captured billing details"}},
		{"usage", []string{"Date", "Model", "Key ID", "Requests", "Input tokens", "Output tokens", "Amount (USD)", "Amount (nano-USD)"}},
		{"ledger", []string{"Ledger ID", "Time", "Type", "Amount (USD)", "Amount (nano-USD)", "Balance after (USD)", "Balance after (nano-USD)", "Description", "Order ID", "Key ID", "Requests", "Funding Source", "API Quota Eligible"}},
	}
	for _, tc := range cases {
		t.Run(tc.typ, func(t *testing.T) {
			newTestDB(t)
			u := newUser(t, "alice", common.RoleUser, 1000)
			rr := call(CreateExport, testRequest{
				method: http.MethodPost,
				target: "/api/export",
				body:   `{"type":"` + tc.typ + `","range_days":7}`,
				userID: u.ID,
				role:   common.RoleUser,
			})
			var exp model.ExportRecord
			decodeData(t, rr, &exp)
			if exp.ID <= 0 || exp.Type != tc.typ || exp.RowCount != 0 || exp.Status != "completed" {
				t.Fatalf("export metadata = %+v, want a completed empty %s export", exp, tc.typ)
			}
			stored, err := model.GetExport(exp.ID)
			if err != nil {
				t.Fatalf("GetExport failed: %v", err)
			}
			if stored.RowCount != 0 || stored.Status != "completed" || stored.UserID != u.ID {
				t.Fatalf("persisted export metadata = %+v, want a completed empty export owned by %d", stored, u.ID)
			}

			rr = call(DownloadExport, testRequest{
				method: http.MethodGet,
				target: "/api/export/files/" + testExportDownloadToken(t, exp.ID) + "/download",
				userID: u.ID,
				role:   common.RoleUser,
				path:   map[string]string{"token": testExportDownloadToken(t, exp.ID)},
			})
			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 (body %q)", rr.Code, rr.Body.String())
			}
			if got := rr.Header().Get("Content-Type"); got != "text/csv; charset=utf-8" {
				t.Fatalf("Content-Type = %q, want text/csv; charset=utf-8", got)
			}
			if rr.Body.String() != string(stored.Content) {
				t.Fatal("download differs from persisted CSV content")
			}
			rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(rr.Body.String(), "\ufeff"))).ReadAll()
			if err != nil {
				t.Fatalf("invalid CSV: %v", err)
			}
			if want := [][]string{tc.header}; !reflect.DeepEqual(rows, want) {
				t.Fatalf("CSV rows = %q, want only the header %q", rows, want)
			}
		})
	}
}

func TestListExportsUserSeesOwnOnly(t *testing.T) {
	newTestDB(t)
	alice := newUser(t, "alice", common.RoleUser, 1000)
	bob := newUser(t, "bob", common.RoleUser, 1000)

	// Alice creates an export.
	call(CreateExport, testRequest{
		method: http.MethodPost,
		target: "/api/export",
		body:   `{"type":"requests","range_days":7}`,
		userID: alice.ID,
		role:   common.RoleUser,
	})

	// Bob lists exports — should see none.
	rr := call(ListExports, testRequest{
		method: http.MethodGet,
		target: "/api/export",
		userID: bob.ID,
		role:   common.RoleUser,
	})
	var exports []map[string]any
	decodeData(t, rr, &exports)
	if len(exports) != 0 {
		t.Fatalf("bob sees %d exports, want 0", len(exports))
	}

	// Alice lists exports — should see 1.
	rr = call(ListExports, testRequest{
		method: http.MethodGet,
		target: "/api/export",
		userID: alice.ID,
		role:   common.RoleUser,
	})
	decodeData(t, rr, &exports)
	if len(exports) != 1 {
		t.Fatalf("alice sees %d exports, want 1", len(exports))
	}
}

func TestDownloadExport(t *testing.T) {
	newTestDB(t)
	u := newUser(t, "alice", common.RoleUser, 1000)

	model.InsertLog(&model.Log{
		UserID:           u.ID,
		Model:            "gpt-4o-mini",
		PromptTokens:     10,
		CompletionTokens: 5,
		Quota:            2,
		TokenName:        "key1",
	})

	rr := call(CreateExport, testRequest{
		method: http.MethodPost,
		target: "/api/export",
		body:   `{"type":"requests","range_days":7}`,
		userID: u.ID,
		role:   common.RoleUser,
	})
	var exp map[string]any
	decodeData(t, rr, &exp)
	exportID := int64(exp["id"].(float64))

	// Download the CSV.
	rr = call(DownloadExport, testRequest{
		method: http.MethodGet,
		target: "/api/export/files/" + testExportDownloadToken(t, exportID) + "/download",
		userID: u.ID,
		role:   common.RoleUser,
		path:   map[string]string{"token": testExportDownloadToken(t, exportID)},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	ct := rr.Header().Get("Content-Type")
	if ct != "text/csv; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want text/csv", ct)
	}
	body := rr.Body.String()
	if body == "" {
		t.Fatal("download body should not be empty")
	}
}

func TestDownloadExportRequestsCSVEscaping(t *testing.T) {
	newTestDB(t)
	u := newUser(t, "alice", common.RoleUser, 1000)
	createdAt := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	const tokenName = "key,\"quoted\"\nsecond line"
	token := mustToken(t, u.ID, 1000)
	const modelName = "model,\"variant\"\nnext line"
	const requestedModel = "entry,\"alias\"\nnext line"
	l := &model.Log{
		UserID:           u.ID,
		TokenID:          token.ID,
		TokenName:        tokenName,
		Model:            modelName,
		RequestedModel:   requestedModel,
		PromptTokens:     100,
		CompletionTokens: 50,
		Quota:            23,
		CreatedAt:        createdAt.Unix(),
	}
	if err := model.InsertLog(l); err != nil {
		t.Fatalf("InsertLog failed: %v", err)
	}

	rr := call(CreateExport, testRequest{
		method: http.MethodPost,
		target: "/api/export",
		body:   `{"type":"requests","range_days":7}`,
		userID: u.ID,
		role:   common.RoleUser,
	})
	var exp model.ExportRecord
	decodeData(t, rr, &exp)
	if exp.RowCount != 1 || exp.Status != "completed" {
		t.Fatalf("export metadata = %+v, want 1 completed row", exp)
	}
	stored, err := model.GetExport(exp.ID)
	if err != nil {
		t.Fatalf("GetExport failed: %v", err)
	}

	rr = call(DownloadExport, testRequest{
		method: http.MethodGet,
		target: "/api/export/files/" + testExportDownloadToken(t, exp.ID) + "/download",
		userID: u.ID,
		role:   common.RoleUser,
		path:   map[string]string{"token": testExportDownloadToken(t, exp.ID)},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %q)", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("Content-Type"); got != "text/csv; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want text/csv; charset=utf-8", got)
	}
	if rr.Body.String() != string(stored.Content) {
		t.Fatal("download differs from persisted CSV content")
	}
	rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(rr.Body.String(), "\ufeff"))).ReadAll()
	if err != nil {
		t.Fatalf("invalid CSV: %v (body %q)", err, rr.Body.String())
	}
	loc, _ := time.LoadLocation("Asia/Dubai")
	want := [][]string{[]string{"Trace ID", "Time", "Key ID", "Key name", "Requested model", "Actual model", "Status", "Input tokens", "Output tokens", "Duration (ms)", "First token (ms)", "Amount (USD)", "Amount (nano-USD)", "Source", "Vendor", "Service group", "Call type", "Automatically switched", "Error code", "Cache read tokens", "Cache write tokens", "Captured billing details"}, {"log_" + itoa(l.ID), createdAt.In(loc).Format("2006-01-02T15:04:05-07:00"), itoa(token.ID), tokenName, requestedModel, modelName, "success", "100", "50", "0", "0", "0.000046000", "46000", "api", "", "", "", "false", "", "0", "0", ""}}

	if !reflect.DeepEqual(rows, want) {
		t.Fatalf("CSV rows = %q, want %q", rows, want)
	}
}

func TestDownloadExportForbiddenForOtherUser(t *testing.T) {
	newTestDB(t)
	alice := newUser(t, "alice", common.RoleUser, 1000)
	bob := newUser(t, "bob", common.RoleUser, 1000)

	rr := call(CreateExport, testRequest{
		method: http.MethodPost,
		target: "/api/export",
		body:   `{"type":"requests","range_days":7}`,
		userID: alice.ID,
		role:   common.RoleUser,
	})
	var exp map[string]any
	decodeData(t, rr, &exp)
	exportID := int64(exp["id"].(float64))

	rr = call(DownloadExport, testRequest{
		method: http.MethodGet,
		target: "/api/export/files/" + testExportDownloadToken(t, exportID) + "/download",
		userID: bob.ID,
		role:   common.RoleUser,
		path:   map[string]string{"token": testExportDownloadToken(t, exportID)},
	})
	expectError(t, rr, http.StatusNotFound, "export not found")
}

func TestDownloadExportNotFound(t *testing.T) {
	newTestDB(t)
	u := newUser(t, "alice", common.RoleUser, 1000)

	rr := call(DownloadExport, testRequest{
		method: http.MethodGet,
		target: "/api/export/files/" + strings.Repeat("0", 64) + "/download",
		userID: u.ID,
		role:   common.RoleUser,
		path:   map[string]string{"token": strings.Repeat("0", 64)},
	})
	expectError(t, rr, http.StatusNotFound, "export not found")
}

func TestExportHistoryIsOwnerBoundForAdmins(t *testing.T) {
	newTestDB(t)
	alice := newUser(t, "alice", common.RoleUser, 1000)
	admin := newUser(t, "admin", common.RoleAdmin, 1000)

	call(CreateExport, testRequest{
		method: http.MethodPost,
		target: "/api/export?scope=owned",
		body:   `{"type":"requests","range_days":7}`,
		userID: alice.ID,
		role:   common.RoleUser,
	})

	rr := call(ListExports, testRequest{
		method: http.MethodGet,
		target: "/api/export?scope=owned",
		userID: admin.ID,
		role:   common.RoleAdmin,
	})
	var exports []map[string]any
	decodeData(t, rr, &exports)
	if len(exports) != 0 {
		t.Fatalf("admin sees %d foreign exports, want 0", len(exports))
	}
}

func testExportDownloadToken(t *testing.T, id int64) string {
	t.Helper()
	row, err := model.GetExport(id)
	if err != nil {
		t.Fatal(err)
	}
	return row.DownloadToken
}
