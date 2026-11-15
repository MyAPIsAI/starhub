package model

import (
	"context"
	"os"
	"starhub/common"
	"sync"
	"testing"
)

func TestNetworkApplicationInitialization(t *testing.T) {
	for _, driver := range []string{"mysql", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			fixture := networkProductionDB(t, driver)
			t.Cleanup(func() {
				if DB != fixture {
					_ = CloseDB()
					DB = fixture
				}
				tables, err := fixture.Migrator().GetTables()
				if err != nil {
					t.Error(err)
					return
				}
				for _, table := range tables {
					if err := fixture.Migrator().DropTable(table); err != nil {
						t.Error(err)
					}
				}
			})
			env := "STARHUB_TEST_MYSQL_DSN"
			if driver == "postgres" {
				env = "STARHUB_TEST_POSTGRES_DSN"
			}
			cfg := DatabaseConfig{Driver: driver, DSN: os.Getenv(env), MaxOpenConns: 8, MaxIdleConns: 2, ConnMaxLifetime: "1m"}
			if err := InitDBWithConfig(cfg); err != nil {
				t.Fatal("full migration", err)
			}
			if err := Seed("p37-local-test"); err != nil {
				t.Fatal("application seed", err)
			}
			if _, err := SaveOption("group_ratio", `{"default":1}`, 1); err != nil {
				t.Fatal("option update", err)
			}
			if _, err := ReadPricingOptions(t.Context()); err != nil {
				t.Fatal("pricing read", err)
			}
			if _, err := ReadTimePricing(t.Context()); err != nil {
				t.Fatal("time pricing read", err)
			}
			channel := &Channel{Type: 1, Name: "network", Key: "test-key", Models: "test-model", Group: "default", Status: 1}
			if err := InsertChannel(channel); err != nil {
				t.Fatal("channel creation", err)
			}
			coverage, err := ListModelCoverage()
			if err != nil || coverage["test-model"] == nil || coverage["test-model"].ChannelCount != 1 {
				t.Fatal("model coverage", coverage, err)
			}
			if selected, err := GetSatisfiedChannel("default", "test-model"); err != nil || selected.ID != channel.ID {
				t.Fatal("channel selection", selected, err)
			}
			user := &User{Username: "accounting", Quota: 1000, AffCode: "p37-accounting"}
			if err := CreateUser(user); err != nil {
				t.Fatal(err)
			}
			var wg sync.WaitGroup
			for range 8 {
				wg.Go(func() {
					for range 10 {
						if ok, err := DecreaseUserQuota(user.ID, 1); err != nil || !ok {
							t.Error("concurrent accounting", ok, err)
							return
						}
					}
				})
			}
			wg.Wait()
			after, err := GetUserByID(user.ID)
			if err != nil || after.Quota != 920 {
				t.Fatal("incorrect accounting balance", after, err)
			}
			if err := VerifyMoneyLedger(user.ID); err != nil {
				t.Fatal("accounting journal", err)
			}
			var source FundingReceipt
			if err := DB.Where("user_id = ?", user.ID).First(&source).Error; err != nil {
				t.Fatal("funding input", err)
			}
			report, _, err := ReconcileBusinessDay(user.ID, source.BusinessDaySnapshot)
			if err != nil || report.State != "matched" || report.FundingDebitsNano != common.QuotaMoney(80, 0).String() {
				t.Fatal("network funding reconciliation", report, err)
			}
			if err := UpdateUserFields(user.ID, map[string]any{"email": "verified@example.test", "email_verified_at": int64(1), "language": "en"}); err != nil {
				t.Fatal(err)
			}
			if err := UpdateUserFields(user.ID, map[string]any{"password": "updated-test-hash"}); err != nil {
				t.Fatal(err)
			}
			var notification SecurityNotification
			if err := DB.Where("user_id = ?", user.ID).First(&notification).Error; err != nil {
				t.Fatal("network security outbox", err)
			}
			if err := CompleteSecurityNotificationContext(context.Background(), notification, true); err != nil {
				t.Fatal("network acceptance", err)
			}
			if err := CloseDB(); err != nil {
				t.Fatal(err)
			}
			if err := InitDBWithConfig(cfg); err != nil {
				t.Fatal("repeated application migration", err)
			}
		})
	}
}
