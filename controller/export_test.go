package controller

import (
	"net/http"
	"testing"

	"starhub/common"
	"starhub/model"
)

func TestCreateExportValidation(t *testing.T) {
	newTestDB(t)
	u := newUser(t, "alice", common.RoleUser, 1000)

	cases := []struct {
		name string
		body string
	}{
		{"invalid type", `{"type":"invalid","range_days":7}`},
		{"range too small", `{"type":"requests","range_days":0}`},
		{"range too large", `{"type":"requests","range_days":32}`},
		{"missing type", `{"range_days":7}`},
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
			expectError(t, rr, http.StatusBadRequest, "")
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

func TestCreateExportUsage(t *testing.T) {
	newTestDB(t)
	u := newUser(t, "alice", common.RoleUser, 1000)

	model.InsertLog(&model.Log{
		UserID:           u.ID,
		Model:            "gpt-4o",
		PromptTokens:     200,
		CompletionTokens: 100,
		Quota:            500,
		TokenName:        "k1",
	})

	rr := call(CreateExport, testRequest{
		method: http.MethodPost,
		target: "/api/export",
		body:   `{"type":"usage","range_days":7}`,
		userID: u.ID,
		role:   common.RoleUser,
	})
	var got map[string]any
	decodeData(t, rr, &got)
	if got["type"] != "usage" {
		t.Fatalf("type = %v, want usage", got["type"])
	}
	if got["row_count"].(float64) < 1 {
		t.Fatalf("row_count = %v, want >= 1", got["row_count"])
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
