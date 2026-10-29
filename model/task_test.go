package model

import (
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func insertTask(t *testing.T, status string) *Task {
	t.Helper()
	tk := &Task{
		TaskID:    "upstream-" + status,
		Platform:  "test-platform",
		Action:    "generate",
		Status:    status,
		Model:     "video-model",
		UserID:    1,
		TokenID:   1,
		ChannelID: 1,
		Group:     "default",
		Quota:     1000,
	}
	if err := InsertTask(tk); err != nil {
		t.Fatalf("InsertTask failed: %v", err)
	}
	return tk
}

func TestGetTaskByIDNotFound(t *testing.T) {
	newTestDB(t)
	if _, err := GetTaskByID(9999); err != ErrTaskNotFound {
		t.Fatalf("want ErrTaskNotFound, got %v", err)
	}
}

func TestListPendingTasksExcludesTerminal(t *testing.T) {
	newTestDB(t)

	insertTask(t, "submitted")
	insertTask(t, "in_progress")
	insertTask(t, "succeeded")
	insertTask(t, "failed")
	insertTask(t, "cancelled")

	pending, err := ListPendingTasks(100)
	if err != nil {
		t.Fatalf("ListPendingTasks failed: %v", err)
	}
	if len(pending) != 2 {
		t.Fatalf("pending = %d, want 2 (submitted + in_progress)", len(pending))
	}
	for _, tk := range pending {
		switch tk.Status {
		case "succeeded", "failed", "cancelled":
			t.Fatalf("terminal task %q returned as pending", tk.Status)
		}
	}
}

func TestPendingTaskPollingDoesNotStarveLaterTasks(t *testing.T) {
	newTestDB(t)
	for range 205 {
		insertTask(t, "submitted")
	}
	first, err := ListPendingTasks(200)
	if err != nil || len(first) != 200 {
		t.Fatalf("first batch: %d %v", len(first), err)
	}
	for _, task := range first {
		if err := TouchTaskPoll(task.ID); err != nil {
			t.Fatal(err)
		}
	}
	next, err := ListPendingTasks(200)
	if err != nil {
		t.Fatal(err)
	}
	for i := range 5 {
		if next[i].ID <= first[len(first)-1].ID {
			t.Fatalf("unpolled task starved: first=%d next=%d", first[len(first)-1].ID, next[i].ID)
		}
	}
}

// A non-positive limit falls back to the documented default of 100 rather
// than degenerating into an unbounded query.
func TestListPendingTasksLimitFallback(t *testing.T) {
	newTestDB(t)
	for i := 0; i < 105; i++ {
		insertTask(t, "submitted")
	}
	for _, limit := range []int{0, -1} {
		got, err := ListPendingTasks(limit)
		if err != nil {
			t.Fatalf("ListPendingTasks(%d) failed: %v", limit, err)
		}
		if len(got) != 100 {
			t.Fatalf("ListPendingTasks(%d) returned %d tasks, want the default limit of 100", limit, len(got))
		}
	}
	got, err := ListPendingTasks(2)
	if err != nil {
		t.Fatalf("ListPendingTasks(2) failed: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListPendingTasks(2) returned %d tasks, want 2", len(got))
	}
}

// The poll endpoint and the background reaper race on the same task; exactly
// one of them may win the finalize claim so the settlement runs once.
func TestClaimTaskFinalizeSingleWinner(t *testing.T) {
	newTestDB(t)
	tk := insertTask(t, "in_progress")

	const workers = 8
	var wins atomic.Int64
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			ok, err := ClaimTaskFinalize(tk.ID, "succeeded", "", `{"outputs":["u"]}`)
			if err != nil {
				t.Errorf("ClaimTaskFinalize failed: %v", err)
				return
			}
			if ok {
				wins.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()

	if wins.Load() != 1 {
		t.Fatalf("claim winners = %d, want exactly 1", wins.Load())
	}
	got, err := GetTaskByID(tk.ID)
	if err != nil {
		t.Fatalf("GetTaskByID failed: %v", err)
	}
	if got.Status != "succeeded" {
		t.Fatalf("status = %q, want succeeded", got.Status)
	}
	if got.CompleteTime == 0 {
		t.Fatal("complete_time was not stamped")
	}

	// A task already in a terminal state can never be claimed again.
	ok, err := ClaimTaskFinalize(tk.ID, "failed", "too late", "")
	if err != nil {
		t.Fatalf("second ClaimTaskFinalize failed: %v", err)
	}
	if ok {
		t.Fatal("a terminal task was claimed a second time")
	}
	got, _ = GetTaskByID(tk.ID)
	if got.Status != "succeeded" {
		t.Fatalf("status was overwritten to %q", got.Status)
	}
}

func TestClaimTaskFinalizeTruncatesFailReason(t *testing.T) {
	newTestDB(t)
	tk := insertTask(t, "in_progress")

	long := strings.Repeat("x", 900)
	if _, err := ClaimTaskFinalize(tk.ID, "failed", long, ""); err != nil {
		t.Fatalf("ClaimTaskFinalize failed: %v", err)
	}
	got, err := GetTaskByID(tk.ID)
	if err != nil {
		t.Fatalf("GetTaskByID failed: %v", err)
	}
	if len(got.FailReason) != 512 {
		t.Fatalf("fail_reason length = %d, want truncated to 512", len(got.FailReason))
	}
}

// UpdateTaskProgress must not resurrect a task that already reached a
// terminal state: the settle has already run for it.
func TestUpdateTaskProgressIgnoresTerminalTasks(t *testing.T) {
	newTestDB(t)
	live := insertTask(t, "submitted")
	done := insertTask(t, "succeeded")

	if err := UpdateTaskProgress(live.ID, "in_progress", 50, `{"note":"half"} `); err != nil {
		t.Fatalf("UpdateTaskProgress failed: %v", err)
	}
	got, _ := GetTaskByID(live.ID)
	if got.Status != "in_progress" || got.Progress != 50 {
		t.Fatalf("live task not updated: %+v", got)
	}

	if err := UpdateTaskProgress(done.ID, "in_progress", 90, ""); err != nil {
		t.Fatalf("UpdateTaskProgress on terminal task failed: %v", err)
	}
	got, _ = GetTaskByID(done.ID)
	if got.Status != "succeeded" {
		t.Fatalf("terminal task was mutated to %q", got.Status)
	}
	if got.Progress != 0 {
		t.Fatalf("terminal task progress mutated to %d", got.Progress)
	}
}

// A negative progress means "leave it alone"; an explicit empty status or
// result meta must likewise not clobber the stored values.
func TestUpdateTaskProgressPartialUpdates(t *testing.T) {
	newTestDB(t)
	tk := insertTask(t, "submitted")

	if err := UpdateTaskProgress(tk.ID, "", 25, ""); err != nil {
		t.Fatalf("UpdateTaskProgress failed: %v", err)
	}
	got, _ := GetTaskByID(tk.ID)
	if got.Status != "submitted" {
		t.Fatalf("empty status should be ignored, got %q", got.Status)
	}
	if got.Progress != 25 {
		t.Fatalf("progress = %d, want 25", got.Progress)
	}

	if err := UpdateTaskProgress(tk.ID, "in_progress", -1, `{"outputs":["a"]}`); err != nil {
		t.Fatalf("UpdateTaskProgress failed: %v", err)
	}
	got, _ = GetTaskByID(tk.ID)
	if got.Progress != 25 {
		t.Fatalf("negative progress should be ignored, got %d", got.Progress)
	}
	if got.Status != "in_progress" {
		t.Fatalf("status = %q, want in_progress", got.Status)
	}
	if got.ResultMeta != `{"outputs":["a"]}` {
		t.Fatalf("result_meta = %q", got.ResultMeta)
	}
}

func TestTaskOutputURLs(t *testing.T) {
	cases := []struct {
		name string
		meta string
		want int
	}{
		{"valid", `{"outputs":["https://a","https://b"]}`, 2},
		{"empty list", `{"outputs":[]}`, 0},
		{"no outputs field", `{"status":"ok"}`, 0},
		{"malformed json", `{"outputs":`, 0},
		{"empty string", ``, 0},
	}
	for _, tc := range cases {
		got := TaskOutputURLs(tc.meta)
		if len(got) != tc.want {
			t.Fatalf("%s: TaskOutputURLs(%q) = %v, want %d entries", tc.name, tc.meta, got, tc.want)
		}
	}

	got := TaskOutputURLs(`{"outputs":["https://a","https://b"]}`)
	if got[0] != "https://a" || got[1] != "https://b" {
		t.Fatalf("URLs = %v, want the stored order preserved", got)
	}
}
