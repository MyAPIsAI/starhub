package model

import (
	"testing"
	"time"

	"starhub/common"
)

func TestCountLogsGlobalVsUser(t *testing.T) {
	newTestDB(t)

	for _, uid := range []int64{1, 1, 2} {
		if err := InsertLog(&Log{UserID: uid, Model: "m", Quota: 1}); err != nil {
			t.Fatalf("InsertLog failed: %v", err)
		}
	}

	all, err := CountLogs(0)
	if err != nil {
		t.Fatalf("CountLogs(0) failed: %v", err)
	}
	if all != 3 {
		t.Fatalf("CountLogs(0) = %d, want 3 (all users)", all)
	}

	mine, err := CountLogs(1)
	if err != nil {
		t.Fatalf("CountLogs(1) failed: %v", err)
	}
	if mine != 2 {
		t.Fatalf("CountLogs(1) = %d, want 2", mine)
	}

	none, err := CountLogs(99)
	if err != nil {
		t.Fatalf("CountLogs(99) failed: %v", err)
	}
	if none != 0 {
		t.Fatalf("CountLogs(99) = %d, want 0", none)
	}
}

func TestListLogsPaginationAndFilter(t *testing.T) {
	newTestDB(t)

	for i := 0; i < 25; i++ {
		model := "gpt-4o"
		if i%5 == 0 {
			model = "claude-3"
		}
		if err := InsertLog(&Log{UserID: 1, Model: model, Quota: 10}); err != nil {
			t.Fatalf("InsertLog failed: %v", err)
		}
	}

	logs, total, err := ListLogs(1, 1, 10, "")
	if err != nil {
		t.Fatalf("ListLogs failed: %v", err)
	}
	if total != 25 {
		t.Fatalf("total = %d, want 25", total)
	}
	if len(logs) != 10 {
		t.Fatalf("page 1 returned %d rows, want 10", len(logs))
	}
	// Newest first: IDs must descend across the page.
	for i := 1; i < len(logs); i++ {
		if logs[i-1].ID <= logs[i].ID {
			t.Fatalf("rows are not ordered by id DESC: %d then %d", logs[i-1].ID, logs[i].ID)
		}
	}

	logs, total, err = ListLogs(1, 3, 10, "")
	if err != nil {
		t.Fatalf("ListLogs page 3 failed: %v", err)
	}
	if total != 25 || len(logs) != 5 {
		t.Fatalf("page 3: total = %d, rows = %d; want 25, 5", total, len(logs))
	}

	// Model filter narrows both the total and the rows.
	filtered, total, err := ListLogs(1, 1, 10, "claude-3")
	if err != nil {
		t.Fatalf("ListLogs(filter) failed: %v", err)
	}
	if total != 5 {
		t.Fatalf("filtered total = %d, want 5", total)
	}
	for _, l := range filtered {
		if l.Model != "claude-3" {
			t.Fatalf("filter leaked model %q", l.Model)
		}
	}
}

// Page and page size are clamped so a hostile query cannot ask for an
// unbounded result set or a negative offset.
func TestListLogsClampsPageAndSize(t *testing.T) {
	newTestDB(t)
	for i := 0; i < 105; i++ {
		if err := InsertLog(&Log{UserID: 1, Model: "m", Quota: 1}); err != nil {
			t.Fatalf("InsertLog failed: %v", err)
		}
	}

	logs, _, err := ListLogs(1, 0, 0, "")
	if err != nil {
		t.Fatalf("ListLogs(0,0) failed: %v", err)
	}
	if len(logs) != 10 {
		t.Fatalf("page 0/size 0 returned %d rows, want 10 (clamped to 1/10)", len(logs))
	}

	logs, _, err = ListLogs(1, -5, -5, "")
	if err != nil {
		t.Fatalf("ListLogs(-5,-5) failed: %v", err)
	}
	if len(logs) != 10 {
		t.Fatalf("negative page/size returned %d rows, want 10", len(logs))
	}

	logs, _, err = ListLogs(1, 1, 1000, "")
	if err != nil {
		t.Fatalf("ListLogs(1,1000) failed: %v", err)
	}
	if len(logs) != 100 {
		t.Fatalf("oversized page size returned %d rows, want the maximum 100", len(logs))
	}
}

// A user-scoped listing must never leak another user's rows.
func TestListLogsScopedToUser(t *testing.T) {
	newTestDB(t)
	if err := InsertLog(&Log{UserID: 1, Model: "m", Quota: 1}); err != nil {
		t.Fatalf("InsertLog failed: %v", err)
	}
	if err := InsertLog(&Log{UserID: 2, Model: "m", Quota: 1}); err != nil {
		t.Fatalf("InsertLog failed: %v", err)
	}

	logs, total, err := ListLogs(1, 1, 10, "")
	if err != nil {
		t.Fatalf("ListLogs failed: %v", err)
	}
	if total != 1 || len(logs) != 1 {
		t.Fatalf("user-scoped listing = %d rows / total %d, want 1/1", len(logs), total)
	}
	if logs[0].UserID != 1 {
		t.Fatalf("leaked log of user %d", logs[0].UserID)
	}
}

func TestSummarizeLogsByDayGroupsByDate(t *testing.T) {
	newTestDB(t)

	day := 24 * time.Hour
	// Anchor to UTC midnight so date(created_at,'unixepoch') buckets are
	// unambiguous regardless of the machine's local timezone.
	today := time.Now().UTC().Truncate(day).Unix()
	yesterday := today - int64(day.Seconds())

	rows := []Log{
		{UserID: 1, Model: "m", Quota: 10, UnfundedQuota: 1, PromptTokens: 100, CompletionTokens: 50, CreatedAt: yesterday},
		{UserID: 1, Model: "m", Quota: 20, UnfundedQuota: 2, PromptTokens: 200, CompletionTokens: 60, CreatedAt: today},
		{UserID: 1, Model: "m", Quota: 30, UnfundedQuota: 3, PromptTokens: 300, CompletionTokens: 70, CreatedAt: today + 60},
	}
	for i := range rows {
		if err := InsertLog(&rows[i]); err != nil {
			t.Fatalf("InsertLog failed: %v", err)
		}
	}

	got, err := SummarizeLogsByDay(1, yesterday)
	if err != nil {
		t.Fatalf("SummarizeLogsByDay failed: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("grouped rows = %d, want 2 days: %+v", len(got), got)
	}
	if got[0].Requests != 1 || got[0].Quota != 10 || got[0].UnfundedQuota != 1 {
		t.Fatalf("yesterday bucket = %+v, want 1 req / quota 10 / unfunded 1", got[0])
	}
	if got[1].Requests != 2 || got[1].Quota != 50 || got[1].UnfundedQuota != 5 {
		t.Fatalf("today bucket = %+v, want 2 req / quota 50 / unfunded 5", got[1])
	}
	if got[1].PromptTokens != 500 || got[1].CompletionTokens != 130 {
		t.Fatalf("today token sums = %d/%d, want 500/130", got[1].PromptTokens, got[1].CompletionTokens)
	}
}

// `since` is an exclusive lower bound: rows older than it are excluded.
func TestSummarizeLogsSinceBoundary(t *testing.T) {
	newTestDB(t)

	day := 24 * time.Hour
	today := time.Now().UTC().Truncate(day).Unix()

	if err := InsertLog(&Log{UserID: 1, Model: "m", Quota: 10, CreatedAt: today - int64(10*day.Seconds())}); err != nil {
		t.Fatalf("InsertLog failed: %v", err)
	}
	if err := InsertLog(&Log{UserID: 1, Model: "m", Quota: 20, CreatedAt: today}); err != nil {
		t.Fatalf("InsertLog failed: %v", err)
	}

	got, err := SummarizeLogsByDay(1, today)
	if err != nil {
		t.Fatalf("SummarizeLogsByDay failed: %v", err)
	}
	if len(got) != 1 || got[0].Quota != 20 {
		t.Fatalf("since-window = %+v, want only today's quota 20", got)
	}
}

func TestListAuditLogsPagination(t *testing.T) {
	newTestDB(t)

	for i := 0; i < 15; i++ {
		entry := &AuditLog{
			UserID:   1,
			Username: "root",
			Method:   "PUT",
			Path:     "/api/option",
			Status:   common.StatusEnabled,
			IP:       "127.0.0.1",
		}
		if err := InsertAuditLog(entry); err != nil {
			t.Fatalf("InsertAuditLog failed: %v", err)
		}
	}

	logs, total, err := ListAuditLogs(1, 10)
	if err != nil {
		t.Fatalf("ListAuditLogs failed: %v", err)
	}
	if total != 15 || len(logs) != 10 {
		t.Fatalf("page 1 = %d rows / total %d, want 10/15", len(logs), total)
	}
	for i := 1; i < len(logs); i++ {
		if logs[i-1].ID <= logs[i].ID {
			t.Fatalf("audit rows not ordered by id DESC: %d then %d", logs[i-1].ID, logs[i].ID)
		}
	}

	// Clamping mirrors ListLogs: page >= 1, size in [1, 100].
	logs, _, err = ListAuditLogs(0, 0)
	if err != nil {
		t.Fatalf("ListAuditLogs(0,0) failed: %v", err)
	}
	if len(logs) != 10 {
		t.Fatalf("page 0/size 0 returned %d rows, want the default 10", len(logs))
	}

	logs, _, err = ListAuditLogs(1, 1000)
	if err != nil {
		t.Fatalf("ListAuditLogs(1,1000) failed: %v", err)
	}
	if len(logs) != 15 {
		t.Fatalf("oversized page size returned %d rows, want all 15", len(logs))
	}
}
