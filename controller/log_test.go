package controller

import (
	"net/http"
	"testing"

	"starhub/common"
	"starhub/model"
)

func TestListLogsScopedByRole(t *testing.T) {
	newTestDB(t)
	alice := newUser(t, "alice", common.RoleUser, 0)
	bob := newUser(t, "bob", common.RoleUser, 0)

	for i := 0; i < 2; i++ {
		if err := model.InsertLog(&model.Log{UserID: alice.ID, Model: "gpt-4o", Quota: 10}); err != nil {
			t.Fatalf("InsertLog failed: %v", err)
		}
	}
	if err := model.InsertLog(&model.Log{UserID: bob.ID, Model: "gpt-4o", Quota: 10}); err != nil {
		t.Fatalf("InsertLog failed: %v", err)
	}

	type logPage struct {
		Items []model.Log `json:"items"`
		Total int64       `json:"total"`
	}

	var mine logPage
	rr := call(ListLogs, testRequest{target: "/api/log", userID: alice.ID, role: common.RoleUser})
	decodeData(t, rr, &mine)
	if mine.Total != 2 || len(mine.Items) != 2 {
		t.Fatalf("alice sees %d items / total %d, want 2/2", len(mine.Items), mine.Total)
	}
	for _, l := range mine.Items {
		if l.UserID != alice.ID {
			t.Fatalf("leaked log of user %d", l.UserID)
		}
	}

	adminUID, adminRole := admin()
	var all logPage
	rr = call(ListLogs, testRequest{target: "/api/log", userID: adminUID, role: adminRole})
	decodeData(t, rr, &all)
	if all.Total != 3 {
		t.Fatalf("admin total = %d, want all 3", all.Total)
	}
}

// The handler echoes back the p/page_size it received rather than the values
// the model layer clamps them to; that quirk is worth pinning so a future
// refactor does not silently change the response contract.
func TestListLogsPaginationEchoAndFilter(t *testing.T) {
	newTestDB(t)
	u := newUser(t, "alice", common.RoleUser, 0)

	for i := 0; i < 5; i++ {
		modelName := "gpt-4o"
		if i%2 == 0 {
			modelName = "claude-3"
		}
		if err := model.InsertLog(&model.Log{UserID: u.ID, Model: modelName, Quota: 10}); err != nil {
			t.Fatalf("InsertLog failed: %v", err)
		}
	}

	type logPage struct {
		Items    []model.Log `json:"items"`
		Total    int64       `json:"total"`
		Page     int64       `json:"page"`
		PageSize int64       `json:"page_size"`
	}

	var got logPage
	rr := call(ListLogs, testRequest{
		target: "/api/log?p=2&page_size=2",
		userID: u.ID,
		role:   common.RoleUser,
	})
	decodeData(t, rr, &got)
	if got.Page != 2 || got.PageSize != 2 {
		t.Fatalf("echoed page/size = %d/%d, want 2/2", got.Page, got.PageSize)
	}
	if got.Total != 5 {
		t.Fatalf("total = %d, want 5 (unfiltered)", got.Total)
	}
	if len(got.Items) != 2 {
		t.Fatalf("page 2 has %d items, want 2", len(got.Items))
	}

	// The model filter narrows the total as well as the page.
	got = logPage{}
	rr = call(ListLogs, testRequest{
		target: "/api/log?model=claude-3",
		userID: u.ID,
		role:   common.RoleUser,
	})
	decodeData(t, rr, &got)
	if got.Total != 3 {
		t.Fatalf("filtered total = %d, want 3", got.Total)
	}
	for _, l := range got.Items {
		if l.Model != "claude-3" {
			t.Fatalf("filter leaked model %q", l.Model)
		}
	}
}

func TestListAuditLogsDefaultsAndShape(t *testing.T) {
	newTestDB(t)
	uid, role := admin()

	for i := 0; i < 3; i++ {
		if err := model.InsertAuditLog(&model.AuditLog{
			UserID: uid, Username: "root", Method: "PUT", Path: "/api/option",
			Status: http.StatusOK, IP: "127.0.0.1",
		}); err != nil {
			t.Fatalf("InsertAuditLog failed: %v", err)
		}
	}

	type auditPage struct {
		Items []model.AuditLog `json:"items"`
		Total int64            `json:"total"`
	}

	var got auditPage
	rr := call(ListAuditLogs, testRequest{target: "/api/audit", userID: uid, role: role})
	decodeData(t, rr, &got)
	if got.Total != 3 || len(got.Items) != 3 {
		t.Fatalf("items/total = %d/%d, want 3/3", len(got.Items), got.Total)
	}

	// A non-positive p falls back to page 1 with a default page size.
	got = auditPage{}
	rr = call(ListAuditLogs, testRequest{target: "/api/audit?p=0&page_size=0", userID: uid, role: role})
	decodeData(t, rr, &got)
	if len(got.Items) != 3 {
		t.Fatalf("default page returned %d items, want 3", len(got.Items))
	}
}
