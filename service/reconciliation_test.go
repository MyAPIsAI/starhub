package service

import (
	"context"
	"testing"
	"time"

	"starhub/model"
)

func TestRunDailyReconciliationOnce(t *testing.T) {
	newTestDB(t)

	// Create a user with some used quota.
	user := &model.User{Username: "recon-test", Password: "x", Role: 1, Status: 1, Quota: 1000000, UsedQuota: 500, Group: "default", AffCode: "test"}
	if err := model.CreateUser(user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	// Insert a log entry with yesterday's timestamp and a matching quota amount.
	yesterday := time.Now().UTC().Add(-24 * time.Hour)
	if err := model.InsertLog(&model.Log{UserID: user.ID, Model: "gpt-4o", Quota: 500, Status: 1, CreatedAt: yesterday.Unix()}); err != nil {
		t.Fatalf("InsertLog: %v", err)
	}

	// Run reconciliation.
	RunDailyReconciliationOnce(context.Background())

	// Verify a record was persisted.
	record, err := model.GetLatestReconciliationForUser(user.ID)
	if err != nil {
		t.Fatalf("GetLatestReconciliationForUser: %v", err)
	}
	if record.UserID != user.ID {
		t.Fatalf("record UserID = %d, want %d", record.UserID, user.ID)
	}
	if record.LogQuotaSum != 500 {
		t.Fatalf("LogQuotaSum = %d, want 500", record.LogQuotaSum)
	}
}

func TestRunDailyReconciliationOnceAlertsOnLargeDiff(t *testing.T) {
	newTestDB(t)

	user := &model.User{Username: "recon-alert", Password: "x", Role: 1, Status: 1, Quota: 1000000, UsedQuota: 0, Group: "default", AffCode: "test"}
	if err := model.CreateUser(user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	// Insert a log entry with yesterday's timestamp and a large quota amount.
	yesterday := time.Now().UTC().Add(-24 * time.Hour)
	if err := model.InsertLog(&model.Log{UserID: user.ID, Model: "gpt-4o", Quota: 50000, Status: 1, CreatedAt: yesterday.Unix()}); err != nil {
		t.Fatalf("InsertLog: %v", err)
	}

	RunDailyReconciliationOnce(context.Background())

	record, err := model.GetLatestReconciliationForUser(user.ID)
	if err != nil {
		t.Fatalf("GetLatestReconciliationForUser: %v", err)
	}
	if !record.Alerted {
		t.Fatal("should alert when diff exceeds threshold")
	}
}
