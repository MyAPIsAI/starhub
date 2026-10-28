package model

import (
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"gorm.io/gorm"
)

func TestQuotaSettlementRecoversAcrossRestartExactlyOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settlement.db")
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = CloseDB() })
	user, token := newBillingUser(t, 1000)
	if err := ReserveQuota(user.ID, token.ID, false, 80); err != nil {
		t.Fatal(err)
	}
	entry := &Log{Model: "actual-model", RequestedModel: "myapis-auto", Protocol: "OpenAI", PromptTokens: 20, CompletionTokens: 10, Status: 1, CreatedAt: 1700000000}
	intent, err := RecordQuotaSettlement(user.ID, token.ID, false, 50, 130, entry)
	if err != nil {
		t.Fatal(err)
	}
	entry.Model = "changed-after-recording"
	if err := DB.Callback().Create().Before("gorm:create").Register("test:fail_quota_log", func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Name == "Log" {
			tx.AddError(errors.New("injected log failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ApplyQuotaSettlement(intent.ID); err == nil {
		t.Fatal("expected log failure")
	}
	pending, err := ListPendingQuotaSettlements(200)
	if err != nil || len(pending) != 1 || pending[0].CompletedAt != 0 {
		t.Fatalf("intent lost: %+v %v", pending, err)
	}
	after, _ := GetUserByID(user.ID)
	if after.Quota != 920 || after.UsedQuota != 0 {
		t.Fatalf("partial settlement moved funds: %+v", after)
	}
	if err := CloseDB(); err != nil {
		t.Fatal(err)
	}
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	var claimed atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, won, err := ApplyQuotaSettlement(intent.ID)
			if err != nil {
				t.Error(err)
			}
			if won {
				claimed.Add(1)
			}
		}()
	}
	wg.Wait()
	if claimed.Load() != 1 {
		t.Fatalf("claims=%d want=1", claimed.Load())
	}
	gotUser, _ := GetUserByID(user.ID)
	gotToken, _ := GetTokenByID(token.ID)
	if gotUser.Quota != 870 || gotUser.UsedQuota != 130 || gotToken.RemainQuota != 870 || gotToken.UsedQuota != 130 {
		t.Fatalf("incorrect recovered balances: user=%+v token=%+v", gotUser, gotToken)
	}
	logs, total, err := ListLogs(user.ID, 1, 10, "")
	if err != nil || total != 1 || len(logs) != 1 {
		t.Fatalf("logs=%+v total=%d err=%v", logs, total, err)
	}
	got := logs[0]
	if got.Model != "actual-model" || got.RequestedModel != "myapis-auto" || got.Protocol != "OpenAI" || got.PromptTokens != 20 || got.CompletionTokens != 10 || got.Quota != 130 || got.CreatedAt != 1700000000 {
		t.Fatalf("frozen accounting metadata changed: %+v", got)
	}
	pending, err = ListPendingQuotaSettlements(200)
	if err != nil || len(pending) != 0 {
		t.Fatalf("completed intent still pending: %+v %v", pending, err)
	}
}

func TestQuotaSettlementRefundAfterTokenDeletion(t *testing.T) {
	newTestDB(t)
	user, token := newBillingUser(t, 1000)
	if err := ReserveQuota(user.ID, token.ID, false, 80); err != nil {
		t.Fatal(err)
	}
	intent, err := RecordQuotaSettlement(user.ID, token.ID, false, -50, 30, &Log{Model: "m", Status: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := DeleteToken(token.ID); err != nil {
		t.Fatal(err)
	}
	if _, won, err := ApplyQuotaSettlement(intent.ID); err != nil || !won {
		t.Fatalf("deleted token blocked accepted bill: won=%v err=%v", won, err)
	}
	after, _ := GetUserByID(user.ID)
	if after.Quota != 970 || after.UsedQuota != 30 {
		t.Fatalf("owner refund lost: %+v", after)
	}
}
