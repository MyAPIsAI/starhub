package openai

import (
	"testing"

	"starhub/relay/channel"
	"starhub/relay/dto"
)

func TestTaskStatusValue(t *testing.T) {
	cases := []struct {
		status string
		want   float64
	}{
		{dto.TaskStatusSucceeded, 1},
		{dto.TaskStatusCancelled, 0.5},
		{dto.TaskStatusFailed, 0},
		{"unknown", 0},
		{"", 0},
	}
	for _, c := range cases {
		if got := taskStatusValue(c.status); got != c.want {
			t.Fatalf("taskStatusValue(%q) = %v, want %v", c.status, got, c.want)
		}
	}
}

func TestEstimateBillingExprVarsPreCharge(t *testing.T) {
	// This test verifies the ExprVars contract without depending on global
	// setting state: it constructs the TaskPricing directly and checks the
	// fields that the expression path populates. The end-to-end test in
	// relay/task_billing_test.go covers the full EstimateBilling →
	// TaskPricingQuotaWithPriceData flow with real settings.
	//
	// Here we only verify taskStatusValue and the ExprVars mutation in
	// AdjustBillingOnComplete (which does not need settings).

	adaptor := VideoTaskAdaptor{}
	req := &dto.TaskRequest{Action: "video", Model: "sora-2", Prompt: "cat", Seconds: 12, N: 3}

	// Simulate the ExprVars that EstimateBilling would produce.
	accepted := channel.TaskPricing{
		ExprVars: map[string]float64{
			"seconds":           float64(req.Seconds),
			"n":                 float64(req.N),
			"task_status":       1,
			"prompt_tokens":     101,
			"completion_tokens": 0,
			"total_tokens":      101,
		},
	}

	// AdjustBillingOnComplete for success: task_status stays 1.
	success := adaptor.AdjustBillingOnComplete("sora-2", "default", req,
		&dto.TaskResult{Status: dto.TaskStatusSucceeded}, accepted)
	if success.ExprVars["task_status"] != 1 {
		t.Fatalf("success task_status = %v, want 1", success.ExprVars["task_status"])
	}

	// AdjustBillingOnComplete for cancelled: task_status → 0.5.
	cancelled := adaptor.AdjustBillingOnComplete("sora-2", "default", req,
		&dto.TaskResult{Status: dto.TaskStatusCancelled}, accepted)
	if cancelled.ExprVars["task_status"] != 0.5 {
		t.Fatalf("cancelled task_status = %v, want 0.5", cancelled.ExprVars["task_status"])
	}

	// AdjustBillingOnComplete for failed: task_status → 0.
	failed := adaptor.AdjustBillingOnComplete("sora-2", "default", req,
		&dto.TaskResult{Status: dto.TaskStatusFailed}, accepted)
	if failed.ExprVars["task_status"] != 0 {
		t.Fatalf("failed task_status = %v, want 0", failed.ExprVars["task_status"])
	}

	// seconds and n must be preserved across status adjustment.
	if failed.ExprVars["seconds"] != 12 {
		t.Fatalf("seconds changed on failure: got %v, want 12", failed.ExprVars["seconds"])
	}
	if failed.ExprVars["n"] != 3 {
		t.Fatalf("n changed on failure: got %v, want 3", failed.ExprVars["n"])
	}
}
