package controller

import (
	"sync"
	"sync/atomic"
	"testing"

	"starhub/model"
)

// TestTaskStateMachine verifies the task lifecycle state machine
// (section 6.3 of P21): legal migrations, terminal idempotency,
// concurrent finalization, and progress update rejection on terminal tasks.
func TestTaskStateMachine(t *testing.T) {
	newTestDB(t)

	// 1. Insert a task in "submitted" state.
	task := &model.Task{
		TaskID:   "task-sm-001",
		Platform: "openai",
		Action:   "video.generations",
		Status:   "submitted",
		Model:    "sora-1",
		UserID:   1,
		TokenID:  1,
		Quota:    1000,
	}
	if err := model.InsertTask(task); err != nil {
		t.Fatalf("InsertTask failed: %v", err)
	}

	// 2. submitted -> processing (UpdateTaskProgress).
	if err := model.UpdateTaskProgress(task.ID, "processing", 50, ""); err != nil {
		t.Fatalf("UpdateTaskProgress to processing failed: %v", err)
	}
	got, _ := model.GetTaskByID(task.ID)
	if got.Status != "processing" {
		t.Fatalf("status after progress update = %q, want processing", got.Status)
	}
	if got.Progress != 50 {
		t.Fatalf("progress = %d, want 50", got.Progress)
	}

	// 3. processing -> succeeded (ClaimTaskFinalize).
	ok, err := model.ClaimTaskFinalize(task.ID, "succeeded", "", `{"outputs":["url"]}`)
	if err != nil {
		t.Fatalf("ClaimTaskFinalize succeeded failed: %v", err)
	}
	if !ok {
		t.Fatal("ClaimTaskFinalize should claim the task (first caller wins)")
	}
	got, _ = model.GetTaskByID(task.ID)
	if got.Status != "succeeded" {
		t.Fatalf("status after finalize = %q, want succeeded", got.Status)
	}
	if got.CompleteTime == 0 {
		t.Fatal("complete_time should be set after finalize")
	}

	// 4. Terminal task is idempotent: second ClaimTaskFinalize returns false.
	ok2, err := model.ClaimTaskFinalize(task.ID, "failed", "should not apply", "")
	if err != nil {
		t.Fatalf("second ClaimTaskFinalize error: %v", err)
	}
	if ok2 {
		t.Fatal("second ClaimTaskFinalize should return false (terminal, idempotent)")
	}
	got2, _ := model.GetTaskByID(task.ID)
	if got2.Status != "succeeded" {
		t.Fatalf("status after second finalize = %q, want succeeded (unchanged)", got2.Status)
	}
	if got2.FailReason != "" {
		t.Fatalf("fail_reason = %q, want empty (terminal not overwritten)", got2.FailReason)
	}

	// 5. UpdateTaskProgress on terminal task is rejected (no-op).
	if err := model.UpdateTaskProgress(task.ID, "processing", 99, "ignored"); err != nil {
		t.Fatalf("UpdateTaskProgress on terminal should not error: %v", err)
	}
	got3, _ := model.GetTaskByID(task.ID)
	if got3.Status != "succeeded" {
		t.Fatalf("status after progress on terminal = %q, want succeeded (unchanged)", got3.Status)
	}
	if got3.Progress != 50 {
		t.Fatalf("progress after terminal update = %d, want 50 (unchanged)", got3.Progress)
	}

	// 6. Concurrent ClaimTaskFinalize: exactly one wins.
	task2 := &model.Task{
		TaskID:   "task-sm-002",
		Platform: "openai",
		Action:   "video.generations",
		Status:   "submitted",
		Model:    "sora-1",
		UserID:   1,
		TokenID:  1,
		Quota:    2000,
	}
	if err := model.InsertTask(task2); err != nil {
		t.Fatalf("InsertTask 2 failed: %v", err)
	}

	const concurrency = 10
	var wg sync.WaitGroup
	var winners int64
	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		go func() {
			defer wg.Done()
			won, _ := model.ClaimTaskFinalize(task2.ID, "succeeded", "", `{"outputs":["url2"]}`)
			if won {
				atomic.AddInt64(&winners, 1)
			}
		}()
	}
	wg.Wait()

	if winners != 1 {
		t.Fatalf("concurrent ClaimTaskFinalize: %d winners, want exactly 1", winners)
	}
	got4, _ := model.GetTaskByID(task2.ID)
	if got4.Status != "succeeded" {
		t.Fatalf("task2 status after concurrent finalize = %q, want succeeded", got4.Status)
	}

	// 7. ListPendingTasks excludes terminal tasks.
	pending, err := model.ListPendingTasks(100)
	if err != nil {
		t.Fatalf("ListPendingTasks failed: %v", err)
	}
	for _, p := range pending {
		if p.ID == task.ID || p.ID == task2.ID {
			t.Fatalf("terminal task %d should not appear in ListPendingTasks", p.ID)
		}
	}

	// 8. failed terminal state: insert, finalize as failed, verify no progress.
	task3 := &model.Task{
		TaskID: "task-sm-003", Platform: "openai", Action: "video.generations",
		Status: "submitted", Model: "sora-1", UserID: 1, TokenID: 1, Quota: 500,
	}
	model.InsertTask(task3)
	ok3, _ := model.ClaimTaskFinalize(task3.ID, "failed", "upstream error", "")
	if !ok3 {
		t.Fatal("ClaimTaskFinalize failed should claim")
	}
	got5, _ := model.GetTaskByID(task3.ID)
	if got5.Status != "failed" {
		t.Fatalf("task3 status = %q, want failed", got5.Status)
	}
	if got5.FailReason != "upstream error" {
		t.Fatalf("task3 fail_reason = %q, want 'upstream error'", got5.FailReason)
	}
	if got5.CompleteTime == 0 {
		t.Fatal("task3 complete_time should be set")
	}

	t.Logf("6.3 task state machine: submitted->processing->succeeded, terminal idempotent, concurrent 1/10 wins, ListPending excludes terminal, failed state correct")
}
