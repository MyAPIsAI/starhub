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
	yesterday := time.Now().UTC().Add(-48 * time.Hour)
	if err := model.InsertLog(&model.Log{UserID: user.ID, Model: "gpt-4o", Quota: 500, Status: 1, CreatedAt: yesterday.Unix()}); err != nil {
		t.Fatalf("InsertLog: %v", err)
	}

	// Run reconciliation.
	RunDailyReconciliationOnce(context.Background())

	// Verify a record was persisted.
	record, err := reconciliationForDate(user.ID, yesterday.Format("2006-01-02"))
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
	yesterday := time.Now().UTC().Add(-48 * time.Hour)
	if err := model.InsertLog(&model.Log{UserID: user.ID, Model: "gpt-4o", Quota: 50000, Status: 1, CreatedAt: yesterday.Unix()}); err != nil {
		t.Fatalf("InsertLog: %v", err)
	}

	RunDailyReconciliationOnce(context.Background())

	record, err := reconciliationForDate(user.ID, yesterday.Format("2006-01-02"))
	if err != nil {
		t.Fatalf("GetLatestReconciliationForUser: %v", err)
	}
	if !record.Alerted {
		t.Fatal("should alert when diff exceeds threshold")
	}
}

func TestDailyMoneyAuditDetectsOneNanoAndNeverRepairsBalances(t *testing.T) {
	newTestDB(t)
	user := &model.User{Username: "recon-nano", Status: 1, Quota: 1000, AffCode: "recon-nano"}
	if err := model.CreateUser(user); err != nil {
		t.Fatal(err)
	}
	if _, err := model.ReadMoneyAccount(user.ID); err != nil {
		t.Fatal(err)
	}
	if err := model.DB.Model(&model.User{}).Where("id = ?", user.ID).Update("quota_remainder_nano", 1).Error; err != nil {
		t.Fatal(err)
	}
	if err := model.InsertLog(&model.Log{UserID: user.ID, QuotaRemainderNano: 1, Status: 1, CreatedAt: time.Now().Add(-48 * time.Hour).Unix()}); err != nil {
		t.Fatal(err)
	}
	RunDailyReconciliationOnce(context.Background())
	record, err := reconciliationForDate(user.ID, time.Now().Add(-48*time.Hour).UTC().Format("2006-01-02"))
	if err != nil || record.AccountingVersion != 3 || record.LogQuotaNanoSum != "1" || !record.Alerted || record.LedgerVerified || record.LedgerError == "" {
		t.Fatalf("one-nano discrepancy disappeared: %+v %v", record, err)
	}
	after, _ := model.GetUserByID(user.ID)
	if after.QuotaRemainderNano != 1 {
		t.Fatal("audit silently repaired the balance")
	}
}

func reconciliationForDate(userID int64, date string) (*model.ReconciliationRecord, error) {
	var record model.ReconciliationRecord
	err := model.DB.Where("user_id = ? AND date = ?", userID, date).First(&record).Error
	return &record, err
}
