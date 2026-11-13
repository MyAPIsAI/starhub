package model

import (
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"path/filepath"
	"sync"
	"testing"

	"gorm.io/gorm"
	"starhub/common"
)

func chargeOneNano(tx *gorm.DB, userID int64, operation string) error {
	u, account, err := lockMoneyUser(tx, userID)
	if err != nil {
		return err
	}
	available := common.QuotaMoney(u.Quota, u.QuotaRemainderNano)
	available.Sub(available, big.NewInt(1))
	spent := common.QuotaMoney(u.UsedQuota, u.UsedQuotaRemainderNano)
	spent.Add(spent, big.NewInt(1))
	if err := setMoneyPair(tx, &User{}, u.ID, "quota", available); err != nil {
		return err
	}
	if err := setMoneyPair(tx, &User{}, u.ID, "used_quota", spent); err != nil {
		return err
	}
	_, err = recordMoneyMovement(tx, account, moneyMovement{Available: big.NewInt(-1), Spent: big.NewInt(1), Kind: "consumption", OperationID: operation, Reason: "One-nano accounting fixture"})
	return err
}

func TestMoneyJournalOneNanoChargeRollbackReplayAndReconstruction(t *testing.T) {
	newTestDB(t)
	u, _ := newBillingUser(t, 1000)
	if err := DB.Exec(`CREATE TRIGGER fail_money BEFORE INSERT ON money_entries WHEN NEW.kind = 'consumption' BEGIN SELECT RAISE(ABORT,'injected'); END`).Error; err != nil {
		t.Fatal(err)
	}
	if err := DB.Transaction(func(tx *gorm.DB) error { return chargeOneNano(tx, u.ID, "one") }); err == nil {
		t.Fatal("journal failure did not abort charge")
	}
	got, _ := GetUserByID(u.ID)
	if got.Quota != 1000 || got.QuotaRemainderNano != 0 {
		t.Fatal("failed journal changed balance")
	}
	DB.Exec(`DROP TRIGGER fail_money`)
	if err := DB.Transaction(func(tx *gorm.DB) error { return chargeOneNano(tx, u.ID, "one") }); err != nil {
		t.Fatal(err)
	}
	if err := DB.Transaction(func(tx *gorm.DB) error { return chargeOneNano(tx, u.ID, "one") }); err == nil {
		t.Fatal("duplicate business key charged twice")
	}
	got, _ = GetUserByID(u.ID)
	if common.QuotaMoney(got.Quota, got.QuotaRemainderNano).String() != "1999999" || got.UsedQuotaRemainderNano != 1 {
		t.Fatal("one-nano charge lost precision")
	}
	if err := VerifyMoneyLedger(u.ID); err != nil {
		t.Fatal(err)
	}
	DB.Model(&MoneyEntry{}).Where("user_id = ? AND sequence = 2", u.ID).Update("available_delta_nano", "-2")
	if err := VerifyMoneyLedger(u.ID); err == nil {
		t.Fatal("corrupt journal verified")
	}
}

func TestMoneyJournalConcurrentNanoCharges(t *testing.T) {
	newTestDB(t)
	u, _ := newBillingUser(t, 1000)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := DB.Transaction(func(tx *gorm.DB) error { return chargeOneNano(tx, u.ID, fmt.Sprintf("nano-%d", i)) }); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	if err := VerifyMoneyLedger(u.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := GetUserByID(u.ID)
	if common.QuotaMoney(got.Quota, got.QuotaRemainderNano).String() != "1999984" {
		t.Fatal("concurrent charges lost precision")
	}
}

func TestExactMoneySchemaPreservesLegacyAndInFlightValuesAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "money.db")
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = CloseDB() })
	u := &User{Username: "legacy-money", Quota: math.MaxInt64, UsedQuota: 77}
	if err := CreateUser(u); err != nil {
		t.Fatal(err)
	}
	token := &Token{UserID: u.ID, Name: "legacy", RemainQuota: 123, UsedQuota: 44}
	if err := InsertToken(token); err != nil {
		t.Fatal(err)
	}
	reservation := QuotaReservation{ID: "old-inflight", UserID: u.ID, TokenID: token.ID, Amount: 77}
	if err := DB.Create(&reservation).Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := CloseDB(); err != nil {
			t.Fatal(err)
		}
		if err := InitDB(path); err != nil {
			t.Fatal(err)
		}
		got, err := GetUserByID(u.ID)
		if err != nil || got.Quota != math.MaxInt64 || got.QuotaRemainderNano != 0 {
			t.Fatal("legacy balance was rescaled")
		}
		var data map[string]any
		raw, _ := json.Marshal(got)
		_ = json.Unmarshal(raw, &data)
		if data["quota_nano"] != "18446744073709551614000" {
			t.Fatal("large exact balance lost")
		}
		var old QuotaReservation
		DB.First(&old, "id = ?", reservation.ID)
		if old.Amount != 77 {
			t.Fatal("in-flight reservation changed units")
		}
	}
	if err := DB.Transaction(func(tx *gorm.DB) error {
		_, account, err := lockMoneyUser(tx, u.ID)
		if err != nil {
			return err
		}
		if account.ReservedNano != "154000" {
			return fmt.Errorf("legacy hold not recognized: %s", account.ReservedNano)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := VerifyMoneyLedger(u.ID); err != nil {
		t.Fatal(err)
	}
}
