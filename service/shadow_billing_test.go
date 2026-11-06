package service

import (
	"testing"
	"time"

	"starhub/common"
	"starhub/model"
)

func TestRunShadowBillingVerificationWithSyntheticData(t *testing.T) {
	newTestDB(t)
	// Seed: insert logs with known token counts and quota.
	user := &model.User{Username: "shadow-test", Password: "x", Role: 1, Status: 1, Quota: 0, Group: "default", AffCode: common.GenerateAffCode()}
	if err := model.CreateUser(user); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Unix()
	// Insert 3 logs: one matching default pricing, one with diff, one no-tokens (skipped).
	model.InsertLog(&model.Log{UserID: user.ID, Model: "gpt-4o", PromptTokens: 1000, CompletionTokens: 500, Quota: 1500, Status: 1, ChannelID: 1, CreatedAt: now})
	model.InsertLog(&model.Log{UserID: user.ID, Model: "gpt-4o", PromptTokens: 2000, CompletionTokens: 1000, Quota: 100, Status: 1, ChannelID: 1, CreatedAt: now})
	model.InsertLog(&model.Log{UserID: user.ID, Model: "gpt-4o", PromptTokens: 0, CompletionTokens: 0, Quota: 0, Status: 1, ChannelID: 1, CreatedAt: now})

	report, err := RunShadowBillingVerification(now-3600, 100)
	if err != nil {
		t.Fatalf("RunShadowBillingVerification failed: %v", err)
	}
	if report.TotalRecords != 2 {
		t.Fatalf("expected 2 records (skip zero-token), got %d", report.TotalRecords)
	}
	// The second log has a deliberately wrong quota (100 vs recomputed),
	// so at least 1 record should have a diff.
	if report.RecordsWithDiff == 0 {
		t.Fatal("expected at least 1 record with diff")
	}
}

func TestRunShadowBillingVerificationNoData(t *testing.T) {
	newTestDB(t)
	report, err := RunShadowBillingVerification(time.Now().Add(-1*time.Hour).Unix(), 100)
	if err != nil {
		t.Fatal(err)
	}
	if report.TotalRecords != 0 {
		t.Fatalf("expected 0 records with empty DB, got %d", report.TotalRecords)
	}
}

func newTestDB(t *testing.T) {
	t.Helper()
	if err := model.InitDB(t.TempDir() + "/shadow.db"); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	t.Cleanup(func() { _ = model.CloseDB() })
}
