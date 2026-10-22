package controller

import (
	"encoding/csv"
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
	logs := []model.Log{
		{CreatedAt: from.Add(-time.Second).Unix(), Model: "before-start"},
		{CreatedAt: from.Unix(), Model: "at-start"},
		{CreatedAt: from.Add(time.Second).Unix(), Model: "after-start"},
	}
	for i := range logs {
		logs[i].UserID = u.ID
		logs[i].TokenName = "key"
		logs[i].PromptTokens = 100
		logs[i].CompletionTokens = 50
		logs[i].Quota = 23
		if err := model.InsertLog(&logs[i]); err != nil {
			t.Fatalf("InsertLog(%d) failed: %v", i, err)
		}
	}

	content, rowCount, err := buildLogExport("requests", u.ID, from)
	if err != nil {
		t.Fatalf("buildLogExport failed: %v", err)
	}
	if rowCount != 2 {
		t.Fatalf("row_count = %d, want 2", rowCount)
	}
	rows, err := csv.NewReader(strings.NewReader(string(content))).ReadAll()
	if err != nil {
		t.Fatalf("invalid CSV: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("CSV rows = %q, want a header and 2 data rows", rows)
	}
	if want := []string{"ID", "Time (UTC)", "User ID", "Token ID", "Token Name", "Model", "Prompt Tokens", "Completion Tokens", "Quota"}; !reflect.DeepEqual(rows[0], want) {
		t.Fatalf("CSV header = %q, want %q", rows[0], want)
	}
	wantByID := map[string][]string{
		itoa(logs[1].ID): {itoa(logs[1].ID), "2026-10-20T00:00:00Z", itoa(u.ID), "0", "key", "at-start", "100", "50", "23"},
		itoa(logs[2].ID): {itoa(logs[2].ID), "2026-10-20T00:00:01Z", itoa(u.ID), "0", "key", "after-start", "100", "50", "23"},
	}
	for _, row := range rows[1:] {
		want, ok := wantByID[row[0]]
		if !ok || !reflect.DeepEqual(row, want) {
			t.Fatalf("CSV row = %q, want %q", row, want)
		}
		delete(wantByID, row[0])
	}
	if count, err := model.CountLogs(u.ID); err != nil || count != 3 {
		t.Fatalf("source log count = %d, err = %v, want 3 unchanged logs", count, err)
	}
}

func TestCreateExportUsage(t *testing.T) {
	newTestDB(t)
	u := newUser(t, "alice", common.RoleUser, 1000)
	day := time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, -2)
	logs := []model.Log{
		{CreatedAt: day.Add(23*time.Hour + 58*time.Minute).Unix(), PromptTokens: 100, CompletionTokens: 50, Quota: 23},
		{CreatedAt: day.Add(23*time.Hour + 59*time.Minute).Unix(), PromptTokens: 20, CompletionTokens: 10, Quota: 7},
		{CreatedAt: day.Add(24*time.Hour + time.Minute).Unix(), PromptTokens: 5, CompletionTokens: 3, Quota: 2},
	}
	for i := range logs {
		logs[i].UserID = u.ID
		logs[i].Model = "gpt-4o"
		if err := model.InsertLog(&logs[i]); err != nil {
			t.Fatalf("InsertLog(%d) failed: %v", i, err)
		}
	}

	rr := call(CreateExport, testRequest{
		method: http.MethodPost,
		target: "/api/export",
		body:   `{"type":"usage","range_days":7}`,
		userID: u.ID,
		role:   common.RoleUser,
	})
	var exp model.ExportRecord
	decodeData(t, rr, &exp)
	if exp.Type != "usage" || exp.Status != "completed" || exp.RowCount != 2 {
		t.Fatalf("export metadata = %+v, want a completed usage export with 2 daily rows", exp)
	}
	stored, err := model.GetExport(exp.ID)
	if err != nil {
		t.Fatalf("GetExport failed: %v", err)
	}
	if stored.RowCount != 2 {
		t.Fatalf("persisted row_count = %d, want 2", stored.RowCount)
	}
	rr = call(DownloadExport, testRequest{
		method: http.MethodGet,
		target: "/api/export/" + itoa(exp.ID) + "/download",
		userID: u.ID,
		role:   common.RoleUser,
		path:   map[string]string{"id": itoa(exp.ID)},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %q)", rr.Code, rr.Body.String())
	}
	if rr.Body.String() != string(stored.Content) {
		t.Fatal("download differs from persisted CSV content")
	}
	rows, err := csv.NewReader(strings.NewReader(rr.Body.String())).ReadAll()
	if err != nil {
		t.Fatalf("invalid CSV: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("CSV rows = %q, want a header and 2 daily rows", rows)
	}
	if want := []string{"Date", "Requests", "Prompt Tokens", "Completion Tokens", "Quota"}; !reflect.DeepEqual(rows[0], want) {
		t.Fatalf("CSV header = %q, want %q", rows[0], want)
	}
	wantByDay := map[string][]string{
		day.Format("2006-01-02"):                  {day.Format("2006-01-02"), "2", "120", "60", "30"},
		day.AddDate(0, 0, 1).Format("2006-01-02"): {day.AddDate(0, 0, 1).Format("2006-01-02"), "1", "5", "3", "2"},
	}
	for _, row := range rows[1:] {
		want, ok := wantByDay[row[0]]
		if !ok || !reflect.DeepEqual(row, want) {
			t.Fatalf("CSV daily row = %q, want %q", row, want)
		}
		delete(wantByDay, row[0])
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
	rows, err := csv.NewReader(strings.NewReader(string(content))).ReadAll()
	if err != nil {
		t.Fatalf("invalid CSV: %v", err)
	}
	if len(rows) != 4 {
		t.Fatalf("CSV rows = %q, want a header and 3 data rows", rows)
	}
	if want := []string{"Ledger ID", "Time (UTC)", "Type", "Amount (nano-USD)", "Balance After (nano-USD)", "Description", "Order ID", "Trace ID"}; !reflect.DeepEqual(rows[0], want) {
		t.Fatalf("CSV header = %q, want %q", rows[0], want)
	}
	wantByID := map[string][]string{
		itoa(entries[0].ID): {itoa(entries[0].ID), "2026-10-20T00:00:00Z", "recharge", "1", "1", "one nano", "ord-tiny", ""},
		itoa(entries[1].ID): {itoa(entries[1].ID), "2026-10-20T00:00:00Z", "recharge", "1000000001", "1000000002", "credit", "ord-credit", ""},
		itoa(entries[2].ID): {itoa(entries[2].ID), "2026-10-20T00:00:00Z", "call_summary", "-2", "1000000000", "debit", "", "trace-debit"},
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
		{"requests", []string{"ID", "Time (UTC)", "User ID", "Token ID", "Token Name", "Model", "Prompt Tokens", "Completion Tokens", "Quota"}},
		{"usage", []string{"Date", "Requests", "Prompt Tokens", "Completion Tokens", "Quota"}},
		{"ledger", []string{"Ledger ID", "Time (UTC)", "Type", "Amount (nano-USD)", "Balance After (nano-USD)", "Description", "Order ID", "Trace ID"}},
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
				target: "/api/export/" + itoa(exp.ID) + "/download",
				userID: u.ID,
				role:   common.RoleUser,
				path:   map[string]string{"id": itoa(exp.ID)},
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
			rows, err := csv.NewReader(strings.NewReader(rr.Body.String())).ReadAll()
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
		target: "/api/export/" + itoa(exportID) + "/download",
		userID: u.ID,
		role:   common.RoleUser,
		path:   map[string]string{"id": itoa(exportID)},
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
	const modelName = "model,\"variant\"\nnext line"
	l := &model.Log{
		UserID:           u.ID,
		TokenID:          42,
		TokenName:        tokenName,
		Model:            modelName,
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
		target: "/api/export/" + itoa(exp.ID) + "/download",
		userID: u.ID,
		role:   common.RoleUser,
		path:   map[string]string{"id": itoa(exp.ID)},
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
	rows, err := csv.NewReader(strings.NewReader(rr.Body.String())).ReadAll()
	if err != nil {
		t.Fatalf("invalid CSV: %v (body %q)", err, rr.Body.String())
	}
	want := [][]string{
		{"ID", "Time (UTC)", "User ID", "Token ID", "Token Name", "Model", "Prompt Tokens", "Completion Tokens", "Quota"},
		{itoa(l.ID), createdAt.Format(time.RFC3339), itoa(u.ID), "42", tokenName, modelName, "100", "50", "23"},
	}
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
		target: "/api/export/" + itoa(exportID) + "/download",
		userID: bob.ID,
		role:   common.RoleUser,
		path:   map[string]string{"id": itoa(exportID)},
	})
	expectError(t, rr, http.StatusForbidden, "another user")
}

func TestDownloadExportNotFound(t *testing.T) {
	newTestDB(t)
	u := newUser(t, "alice", common.RoleUser, 1000)

	rr := call(DownloadExport, testRequest{
		method: http.MethodGet,
		target: "/api/export/9999/download",
		userID: u.ID,
		role:   common.RoleUser,
		path:   map[string]string{"id": "9999"},
	})
	expectError(t, rr, http.StatusNotFound, "export not found")
}

func TestAdminCanListAllExports(t *testing.T) {
	newTestDB(t)
	alice := newUser(t, "alice", common.RoleUser, 1000)
	admin := newUser(t, "admin", common.RoleAdmin, 1000)

	call(CreateExport, testRequest{
		method: http.MethodPost,
		target: "/api/export",
		body:   `{"type":"requests","range_days":7}`,
		userID: alice.ID,
		role:   common.RoleUser,
	})

	rr := call(ListExports, testRequest{
		method: http.MethodGet,
		target: "/api/export",
		userID: admin.ID,
		role:   common.RoleAdmin,
	})
	var exports []map[string]any
	decodeData(t, rr, &exports)
	if len(exports) != 1 {
		t.Fatalf("admin sees %d exports, want 1", len(exports))
	}
}
