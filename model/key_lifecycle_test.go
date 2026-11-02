package model

import (
	"encoding/base64"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"starhub/common"
)

func TestKeyAC10AC20IPNetworksAndNormalization(t *testing.T) {
	for _, test := range []struct {
		allow, address string
		want           bool
	}{
		{"10.0.0.0/24", "10.0.0.5", true}, {"10.0.0.0/24", "10.0.1.5", false},
		{"2001:db8::/32", "2001:db8:0:0::1", true}, {"2001:db8::/32", "2001:db9::1", false},
		{"2001:db8::1", "2001:db8:0:0:0:0:0:1", true}, {"1.2.3.4", "::ffff:1.2.3.4", true},
		{"::ffff:1.2.3.0/120", "1.2.3.4", true}, {"", "1.2.3.4", true}, {"1.2.3.4", "invalid", false},
	} {
		key := Token{AllowIps: test.allow}
		if got := key.AllowsIP(test.address); got != test.want {
			t.Errorf("%s with %s: %v", test.allow, test.address, got)
		}
	}
	for _, bad := range []string{"not-an-address", "1.2.3.0/99", strings.Repeat("1.2.3.4,", 51)} {
		if _, err := NormalizeTokenIPs(bad); err == nil {
			t.Errorf("accepted invalid allowlist %q", bad)
		}
	}
}

func TestKeyEncryptedMigrationRestartAndHashLookup(t *testing.T) {
	t.Setenv("STARHUB_KEY_ENCRYPTION_KEY", "")
	path := filepath.Join(t.TempDir(), "keys.db")
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = CloseDB() })
	user := &User{Username: "encrypted-owner", Status: 1, AffCode: "encrypted"}
	if err := CreateUser(user); err != nil {
		t.Fatal(err)
	}
	key := &Token{UserID: user.ID, Key: common.GenerateTokenKey(), Name: "Legacy", Status: 1, ExpiredTime: -1, RemainQuota: 123, UsedQuota: 45}
	if err := DB.Create(key).Error; err != nil {
		t.Fatal(err)
	}
	var stored string
	DB.Table("tokens").Where("id = ?", key.ID).Pluck("key", &stored)
	if stored == key.Key || !strings.HasPrefix(stored, tokenCipherPrefix) {
		t.Fatal("key was not encrypted")
	}
	// Recreate a legacy plaintext row to exercise the deployment migration.
	if err := DB.Table("tokens").Where("id = ?", key.ID).Updates(map[string]any{"key": key.Key, "key_hash": nil}).Error; err != nil {
		t.Fatal(err)
	}
	if err := InsertTask(&Task{TokenID: key.ID, UserID: user.ID, Status: "submitted", Quota: 7}); err != nil {
		t.Fatal(err)
	}
	if err := DB.Migrator().DropColumn(&Token{}, "quota_limit"); err != nil {
		t.Fatal(err)
	}
	if err := CloseDB(); err != nil {
		t.Fatal(err)
	}
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	migrated, err := GetTokenByKey(key.Key)
	if err != nil || migrated.ID != key.ID || migrated.UsedQuota != 45 || migrated.RemainQuota != 123 || migrated.QuotaLimit != 175 {
		t.Fatalf("legacy key or usage lost: %+v %v", migrated, err)
	}
	DB.Table("tokens").Where("id = ?", key.ID).Pluck("key", &stored)
	if stored == key.Key || !strings.HasPrefix(stored, tokenCipherPrefix) {
		t.Fatal("legacy plaintext remained in key column")
	}
	if err := UpdateTokenFields(key.ID, map[string]any{"key": common.GenerateTokenKey()}); err != nil {
		t.Fatal(err)
	}
	if _, err := GetTokenByKey(key.Key); err != ErrTokenNotFound {
		t.Fatal("old credential remained usable")
	}
	fresh, err := GetTokenByID(key.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := CloseDB(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("STARHUB_KEY_ENCRYPTION_KEY", base64.StdEncoding.EncodeToString([]byte(strings.Repeat("x", 32))))
	if err := InitDB(path); err == nil {
		t.Fatal("wrong encryption key was accepted")
	}
	t.Setenv("STARHUB_KEY_ENCRYPTION_KEY", "")
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	if restored, err := GetTokenByKey(fresh.Key); err != nil || restored.ID != key.ID || restored.UsedQuota != 45 {
		t.Fatal("failed startup changed credentials or usage")
	}
}

func TestKeyQuotaPeriodsRetainHistoryAndAcceptedReservations(t *testing.T) {
	newTestDB(t)
	user := &User{Username: "period-owner", Status: 1, AffCode: "period", Quota: 10000}
	if err := CreateUser(user); err != nil {
		t.Fatal(err)
	}
	for _, period := range []string{"daily", "monthly"} {
		key := &Token{UserID: user.ID, Key: common.GenerateTokenKey(), Status: 1, QuotaPeriod: period, QuotaLimit: 100, RemainQuota: 10, UsedQuota: 70, PeriodUsedQuota: 70, QuotaTimezone: "Asia/Dubai"}
		if err := DB.Create(key).Error; err != nil {
			t.Fatal(err)
		}
		previous := tokenPeriodStart(period, key.QuotaTimezone, time.Now().AddDate(0, -1, -2))
		if err := UpdateTokenFields(key.ID, map[string]any{"period_started_at": previous}); err != nil {
			t.Fatal(err)
		}
		key.PeriodStartedAt = previous
		if err := RefreshTokenPeriod(key, time.Now()); err != nil {
			t.Fatal(err)
		}
		if key.RemainQuota != 80 || key.PeriodUsedQuota != 0 || key.UsedQuota != 70 {
			t.Fatalf("rollover lost reservations or lifetime usage: %+v", key)
		}
		entry := &Log{UserID: user.ID, TokenID: key.ID, TokenPeriodStartedAt: previous, Status: 1}
		if err := SettleQuota(user.ID, key.ID, false, -5, 15, entry); err != nil {
			t.Fatal(err)
		}
		fresh, _ := GetTokenByID(key.ID)
		var history TokenQuotaPeriod
		if err := DB.First(&history, "token_id = ? AND started_at = ?", key.ID, previous).Error; err != nil {
			t.Fatal(err)
		}
		if fresh.RemainQuota != 100 || fresh.PeriodUsedQuota != 0 || fresh.UsedQuota != 85 || history.UsedQuota != 85 {
			t.Fatalf("old request charged new period: %+v history=%+v", fresh, history)
		}
	}
}

func TestDeletedKeyAcceptedRequestStillSettlesAndHistoryIsMarked(t *testing.T) {
	newTestDB(t)
	user := &User{Username: "deleted-owner", Status: 1, AffCode: "deleted", Quota: 1000}
	if err := CreateUser(user); err != nil {
		t.Fatal(err)
	}
	key := &Token{UserID: user.ID, Key: common.GenerateTokenKey(), Status: 1, RemainQuota: 100}
	if err := DB.Create(key).Error; err != nil {
		t.Fatal(err)
	}
	if err := ReserveQuota(user.ID, key.ID, false, 20); err != nil {
		t.Fatal(err)
	}
	if err := DeleteToken(key.ID); err != nil {
		t.Fatal(err)
	}
	if err := SettleQuota(user.ID, key.ID, false, -5, 15, &Log{TokenName: "Deleted", Status: 1}); err != nil {
		t.Fatal(err)
	}
	logs, _, err := ListLogs(user.ID, 1, 20, "")
	if err != nil || len(logs) != 1 || !logs[0].TokenDeleted || logs[0].Quota != 15 {
		t.Fatalf("deleted key history lost: %+v %v", logs, err)
	}
}

func TestKeyPeriodEditPreservesAcceptedUsageAndPendingQuota(t *testing.T) {
	newTestDB(t)
	user := &User{Username: "edited-period-owner", Status: 1, AffCode: "edited-period", Quota: 10000}
	if err := CreateUser(user); err != nil {
		t.Fatal(err)
	}
	key := &Token{UserID: user.ID, Key: common.GenerateTokenKey(), Status: 1, RemainQuota: 100}
	if err := DB.Create(key).Error; err != nil {
		t.Fatal(err)
	}
	if err := ReserveQuota(user.ID, key.ID, false, 20); err != nil {
		t.Fatal(err)
	}
	expected := int64(80)
	if err := UpdateTokenFieldsWithQuota(key.ID, map[string]any{"quota_period": "daily", "quota_limit": int64(50)}, &expected); err != nil {
		t.Fatal(err)
	}
	fresh, _ := GetTokenByID(key.ID)
	if fresh.RemainQuota != 30 || fresh.QuotaLimit != 50 {
		t.Fatalf("period edit lost reservation: %+v", fresh)
	}
	entry := &Log{TokenPeriodStartedAt: key.PeriodStartedAt, Status: 1}
	if err := SettleQuota(user.ID, key.ID, false, -5, 15, entry); err != nil {
		t.Fatal(err)
	}
	fresh, _ = GetTokenByID(key.ID)
	if fresh.RemainQuota != 50 || fresh.PeriodUsedQuota != 0 || fresh.UsedQuota != 15 {
		t.Fatalf("accepted lifetime request charged the new daily period: %+v", fresh)
	}
	if err := ReserveQuota(user.ID, key.ID, false, 10); err != nil {
		t.Fatal(err)
	}
	expected = 40
	if err := UpdateTokenFieldsWithQuota(key.ID, map[string]any{"quota_limit": int64(9)}, &expected); err == nil {
		t.Fatal("limit erased pending quota")
	}
	fresh, _ = GetTokenByID(key.ID)
	if fresh.QuotaLimit != 50 || fresh.RemainQuota != 40 {
		t.Fatal("rejected limit changed accounting")
	}
}
