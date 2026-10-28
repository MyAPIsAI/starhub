package relay

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"gorm.io/gorm"
	"starhub/model"
	"starhub/setting"
)

func rebateFixture(t *testing.T) (*model.User, *model.User, *model.Token, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "rebate.db")
	if err := model.InitDB(path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = model.CloseDB(); setting.ResetAffiliateToDefaults() })
	if err := setting.UpdateAffiliate(setting.OptionKeyAffRebateRatio, "0.1"); err != nil {
		t.Fatal(err)
	}
	inviter := &model.User{Username: "inviter", AffCode: "inviter"}
	if err := model.CreateUser(inviter); err != nil {
		t.Fatal(err)
	}
	user := &model.User{Username: "invitee", AffCode: "invitee", Quota: 1000, InviterID: inviter.ID}
	if err := model.CreateUser(user); err != nil {
		t.Fatal(err)
	}
	token := &model.Token{UserID: user.ID, RemainQuota: 1000}
	if err := model.InsertToken(token); err != nil {
		t.Fatal(err)
	}
	if err := model.ReserveQuota(user.ID, token.ID, false, 80); err != nil {
		t.Fatal(err)
	}
	return inviter, user, token, path
}

func TestAffiliateRebateDeliveryRecoversWithoutRebilling(t *testing.T) {
	for _, taskMode := range []bool{false, true} {
		name := "synchronous"
		if taskMode {
			name = "task"
		}
		t.Run(name, func(t *testing.T) {
			inviter, user, token, path := rebateFixture(t)
			if err := model.DB.Exec(fmt.Sprintf("CREATE TRIGGER fail_rebate BEFORE UPDATE OF quota ON users WHEN OLD.id = %d AND NEW.quota > OLD.quota BEGIN SELECT RAISE(ABORT, 'injected rebate failure'); END", inviter.ID)).Error; err != nil {
				t.Fatal(err)
			}
			if taskMode {
				task := &model.Task{TaskID: "rebate-task", UserID: user.ID, TokenID: token.ID, Quota: 80, Model: "m", Status: "in_progress", PendingStatus: "succeeded", PendingQuota: 100}
				if err := model.InsertTask(task); err != nil {
					t.Fatal(err)
				}
				if claimed, err := FinalizeTaskQuota(task); err != nil || !claimed {
					t.Fatalf("rebate glitch failed task settlement: %v %v", claimed, err)
				}
			} else {
				if err := PostConsumeQuotaAmount(user.ID, token.ID, "", false, 0, "m", 100, 80, "", 0); err != nil {
					t.Fatalf("rebate glitch failed customer settlement: %v", err)
				}
			}
			pending, err := model.ListPendingAffiliateRebates(200)
			if err != nil || len(pending) != 1 || pending[0].Quota != 10 || pending[0].RecipientID != inviter.ID || pending[0].Ratio != 0.1 {
				t.Fatalf("rebate intent missing: %+v %v", pending, err)
			}
			owner, _ := model.GetUserByID(user.ID)
			recipient, _ := model.GetUserByID(inviter.ID)
			if owner.Quota != 900 || owner.UsedQuota != 100 || recipient.Quota != 0 {
				t.Fatalf("customer bill or failed delivery changed funds: owner=%+v inviter=%+v", owner, recipient)
			}
			if err := model.DB.Exec("DROP TRIGGER fail_rebate").Error; err != nil {
				t.Fatal(err)
			}
			if err := setting.UpdateAffiliate(setting.OptionKeyAffRebateRatio, "1"); err != nil {
				t.Fatal(err)
			}
			if err := model.DB.Model(&model.User{}).Where("id = ?", user.ID).UpdateColumn("inviter_id", 0).Error; err != nil {
				t.Fatal(err)
			}
			if err := model.CloseDB(); err != nil {
				t.Fatal(err)
			}
			if err := model.InitDB(path); err != nil {
				t.Fatal(err)
			}
			var wg sync.WaitGroup
			for i := 0; i < 16; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					if err := model.DeliverAffiliateRebate(pending[0].LogID); err != nil {
						t.Error(err)
					}
				}()
			}
			wg.Wait()
			recipient, _ = model.GetUserByID(inviter.ID)
			owner, _ = model.GetUserByID(user.ID)
			if recipient.Quota != 10 || owner.Quota != 900 || owner.UsedQuota != 100 {
				t.Fatalf("delivery repriced, redirected or rebilled: owner=%+v inviter=%+v", owner, recipient)
			}
			if count, err := model.CountLogs(user.ID); err != nil || count != 1 {
				t.Fatalf("delivery duplicated bills: count=%d err=%v", count, err)
			}
			pending, err = model.ListPendingAffiliateRebates(200)
			if err != nil || len(pending) != 0 {
				t.Fatalf("delivery not acknowledged: %+v %v", pending, err)
			}
		})
	}
}

func TestRebateOutboxFailureRollsBackCustomerSettlement(t *testing.T) {
	_, user, token, _ := rebateFixture(t)
	callback := "test:rebate_outbox_failure"
	if err := model.DB.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Name == "AffiliateRebate" {
			tx.AddError(errors.New("injected outbox failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer model.DB.Callback().Create().Remove(callback)
	if err := PostConsumeQuotaAmount(user.ID, token.ID, "", false, 0, "m", 100, 80, "", 0); err == nil {
		t.Fatal("expected outbox persistence failure")
	}
	owner, _ := model.GetUserByID(user.ID)
	if owner.Quota != 920 || owner.UsedQuota != 0 {
		t.Fatalf("outbox failure partially settled: %+v", owner)
	}
	if count, _ := model.CountLogs(user.ID); count != 0 {
		t.Fatal("log committed without its rebate outbox")
	}
	intents, err := model.ListPendingQuotaSettlements(200)
	if err != nil || len(intents) != 1 {
		t.Fatalf("bill intent lost: %+v %v", intents, err)
	}
	if err := model.DB.Callback().Create().Remove(callback); err != nil {
		t.Fatal(err)
	}
	if err := RetryQuotaSettlement(intents[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := RetryQuotaSettlement(intents[0].ID); err != nil {
		t.Fatal(err)
	}
	owner, _ = model.GetUserByID(user.ID)
	if owner.Quota != 900 || owner.UsedQuota != 100 {
		t.Fatalf("bill recovery mismatch: %+v", owner)
	}
}
