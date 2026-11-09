package model

import (
	"context"
	"errors"
	"testing"
	"time"

	"gorm.io/gorm"
	"starhub/observability"
)

func TestNFR10TracePreservesFailedSettlementAndRecovery(t *testing.T) {
	newTestDB(t)
	user, token := newBillingUser(t, 1000)
	if err := ReserveQuota(user.ID, token.ID, false, 80); err != nil {
		t.Fatal(err)
	}
	id := observability.NewID() + observability.NewID()
	now := time.Now()
	if err := SaveTrace(context.Background(), observability.Request{ID: id, Status: 200, StartedAt: now.UnixMilli(), EndedAt: now.UnixMilli()}, nil); err != nil {
		t.Fatal(err)
	}
	intent, err := RecordQuotaSettlement(user.ID, token.ID, false, 50, 130, &Log{TraceID: id, Model: "fixture", Status: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := DB.Callback().Create().Before("gorm:create").Register("test:nfr_log_failure", func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Name == "Log" {
			tx.AddError(errors.New("injected write failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = DB.Callback().Create().Remove("test:nfr_log_failure") })
	if _, _, err := ApplyQuotaSettlement(intent.ID); err == nil {
		t.Fatal("expected injected failure")
	}
	view, err := ReadTrace(context.Background(), id)
	if err != nil || view.AccountingStatus != "pending_settlement" || view.Complete || view.LogID != 0 {
		t.Fatalf("false completion: %+v %v", view, err)
	}
	foundFailure := false
	for _, step := range view.Steps {
		if step.Stage == "settlement" && step.Status == "failed" {
			foundFailure = true
		}
	}
	if !foundFailure {
		t.Fatal("failed settlement was omitted")
	}
	var count int64
	DB.Model(&Observation{}).Where("kind = ? AND success = ?", "settlement", false).Count(&count)
	if count != 1 {
		t.Fatal("failure metric missing", count)
	}
	if err := DB.Callback().Create().Remove("test:nfr_log_failure"); err != nil {
		t.Fatal(err)
	}
	if _, won, err := ApplyQuotaSettlement(intent.ID); err != nil || !won {
		t.Fatalf("recovery failed: %v", err)
	}
	if _, won, err := ApplyQuotaSettlement(intent.ID); err != nil || won {
		t.Fatal("duplicate settlement")
	}
	view, err = ReadTrace(context.Background(), id)
	if err != nil || view.LogID == 0 || view.AccountingStatus == "pending_settlement" {
		t.Fatalf("recovery not visible: %+v %v", view, err)
	}
	userAfter, _ := GetUserByID(user.ID)
	if userAfter.Quota != 870 || userAfter.UsedQuota != 130 {
		t.Fatal("telemetry changed accounting")
	}
}
