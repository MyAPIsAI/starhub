package model

import (
	"errors"
	"fmt"
	"gorm.io/gorm"
	"sync"
	"testing"
)

func TestAccountClosureAtomicDustReceiptAndReplay(t *testing.T) {
	for _, balance := range []int64{0, 1, 9999999} {
		t.Run(fmt.Sprint(balance), func(t *testing.T) {
			newTestDB(t)
			u := &User{Username: "close", Status: 1, Quota: 123, AffCode: "close"}
			if err := CreateUser(u); err != nil {
				t.Fatal(err)
			}
			DB.Create(&Wallet{UserID: u.ID, Balance: balance})
			DB.Create(&Token{UserID: u.ID, Key: "synthetic", Status: 1})
			state, _ := ReadAccountDeactivationState(u.ID)
			if balance > 0 {
				if _, err := DeactivateOwnedAccount(u.ID, state.AccountRevision, "unack", "digest", false); !errors.Is(err, ErrDeactivationConflict) {
					t.Fatal("dust was not acknowledged")
				}
			}
			receipt, err := DeactivateOwnedAccount(u.ID, state.AccountRevision, "operation", "digest", true)
			if err != nil || receipt.WrittenOffNano != fmt.Sprint(balance) {
				t.Fatalf("closure: %+v %v", receipt, err)
			}
			user, _ := GetUserByID(u.ID)
			var wallet Wallet
			DB.First(&wallet, "user_id = ?", u.ID)
			var token Token
			DB.First(&token, "user_id = ?", u.ID)
			var entries int64
			DB.Model(&LedgerEntry{}).Where("user_id = ?", u.ID).Count(&entries)
			want := int64(0)
			if balance > 0 {
				want = 1
			}
			if user.Status != 2 || user.Quota != 123 || user.DeactivationVersion != 1 || wallet.Balance != 0 || token.Status != 2 || entries != want {
				t.Fatal("closure did not commit exactly the intended fields")
			}
			if _, err := ReadDeactivationReceipt(u.ID, "operation", "foreign"); err == nil {
				t.Fatal("foreign session recovered receipt")
			}
			if _, err := ReadDeactivationReceipt(u.ID, "operation", "digest"); err != nil {
				t.Fatal(err)
			}
			UpdateUserFields(u.ID, map[string]any{"status": 1})
			fresh, _ := ReadAccountDeactivationState(u.ID)
			if fresh.AccountRevision == state.AccountRevision {
				t.Fatal("reenablement revived old revision")
			}
			if _, err := DeactivateOwnedAccount(u.ID, state.AccountRevision, "another", "digest", true); !errors.Is(err, ErrDeactivationConflict) {
				t.Fatal("stale action replayed")
			}
			if _, err := DeactivateOwnedAccount(u.ID, fresh.AccountRevision, "operation", "digest", true); !errors.Is(err, ErrDeactivationConflict) {
				t.Fatal("used operation replayed")
			}
		})
	}
}
func TestAccountClosureRollbackAndConcurrentAdmission(t *testing.T) {
	newTestDB(t)
	u := &User{Username: "close-race", Status: 1, AffCode: "race"}
	CreateUser(u)
	DB.Create(&Wallet{UserID: u.ID, Balance: 99})
	DB.Create(&Token{UserID: u.ID, Key: "race", Status: 1})
	state, _ := ReadAccountDeactivationState(u.ID)
	DB.Callback().Create().Before("gorm:create").Register("fail-close-receipt", func(tx *gorm.DB) {
		if tx.Statement.Table == "account_deactivation_receipts" {
			tx.AddError(errors.New("injected receipt failure"))
		}
	})
	if _, err := DeactivateOwnedAccount(u.ID, state.AccountRevision, "rollback", "digest", true); err == nil {
		t.Fatal("receipt failure ignored")
	}
	DB.Callback().Create().Remove("fail-close-receipt")
	user, _ := GetUserByID(u.ID)
	var wallet Wallet
	DB.First(&wallet, "user_id = ?", u.ID)
	if user.Status != 1 || wallet.Balance != 99 {
		t.Fatal("failed receipt did not roll back closure")
	}
	var group sync.WaitGroup
	results := make(chan bool, 8)
	for i := 0; i < 8; i++ {
		group.Add(1)
		go func(i int) {
			defer group.Done()
			_, err := DeactivateOwnedAccount(u.ID, state.AccountRevision, fmt.Sprint(i), "digest", true)
			results <- err == nil
		}(i)
	}
	group.Wait()
	close(results)
	wins := 0
	for ok := range results {
		if ok {
			wins++
		}
	}
	if wins != 1 {
		t.Fatalf("closure winners=%d", wins)
	}
	if err := CreateRechargeOrder(&RechargeOrder{UserID: u.ID, Amount: 100}); !errors.Is(err, ErrDeactivationUserUnavailable) {
		t.Fatal("closed owner admitted a new recharge")
	}
}
func TestAccountClosureBlocksLastRootAndPendingOrders(t *testing.T) {
	newTestDB(t)
	root := &User{Username: "last-root", Role: 100, Status: 1, AffCode: "root"}
	CreateUser(root)
	state, _ := ReadAccountDeactivationState(root.ID)
	if _, err := DeactivateOwnedAccount(root.ID, state.AccountRevision, "root", "digest", false); !errors.Is(err, ErrDeactivationConflict) {
		t.Fatal("last Root was closed")
	}
	u := &User{Username: "pending-owner", Status: 1, AffCode: "pending"}
	CreateUser(u)
	state, _ = ReadAccountDeactivationState(u.ID)
	if err := CreateRechargeOrder(&RechargeOrder{UserID: u.ID, Amount: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := DeactivateOwnedAccount(u.ID, state.AccountRevision, "pending", "digest", false); !errors.Is(err, ErrDeactivationConflict) {
		t.Fatal("pending order did not block closure")
	}
}
