package model

import (
	"encoding/json"
	"starhub/common"
	"strings"
	"testing"
)

func TestCredentialMigrationAndRotationPreserveTokensAndChannelIdentity(t *testing.T) {
	setKeys := func(keys map[string]string, active string) {
		t.Helper()
		raw, _ := json.Marshal(keys)
		t.Setenv("STARHUB_CREDENTIAL_KEYS", string(raw))
		t.Setenv("STARHUB_CREDENTIAL_ACTIVE_KEY", active)
	}
	oldKey, newKey := strings.Repeat("11", 32), strings.Repeat("22", 32)
	setKeys(map[string]string{"old": oldKey}, "old")
	newTestDB(t)
	channel := &Channel{Name: "migrate-channel", Key: "upstream-secret", Models: "model", Group: "default", Status: 1}
	if err := InsertChannel(channel); err != nil {
		t.Fatal(err)
	}
	user := &User{Username: "migration-owner", Status: 1, Role: 1, AffCode: common.GenerateAffCode()}
	if err := CreateUser(user); err != nil {
		t.Fatal(err)
	}
	token := &Token{UserID: user.ID, Name: "migration-token", Status: 1, UnlimitedQuota: true}
	if err := InsertToken(token); err != nil {
		t.Fatal(err)
	}
	key := token.PlaintextKey()
	if key == "" {
		t.Fatal("missing initial token")
	}
	hash := token.KeyHash
	if err := DB.Exec("UPDATE channels SET key = ? WHERE id = ?", channel.Key, channel.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := DB.Exec("UPDATE tokens SET key = ? WHERE id = ?", key, token.ID).Error; err != nil {
		t.Fatal(err)
	}
	count, err := MigrateCredentialStorage(false, "operator", "Legacy migration")
	if err != nil || count != 2 {
		t.Fatalf("migration: %d %v", count, err)
	}
	if count, err := MigrateCredentialStorage(false, "operator", "Retry"); err != nil || count != 0 {
		t.Fatalf("migration replay: %d %v", count, err)
	}
	setKeys(map[string]string{"old": oldKey, "new": newKey}, "new")
	if count, err := MigrateCredentialStorage(true, "operator", "Rotate wrapping key"); err != nil || count != 2 {
		t.Fatalf("rotation: %d %v", count, err)
	}
	setKeys(map[string]string{"new": newKey}, "new")
	readChannel, err := GetChannelByID(channel.ID)
	if err != nil || readChannel.Key != channel.Key || readChannel.ID != channel.ID {
		t.Fatal("channel identity or credential changed")
	}
	readToken, err := GetTokenByKey(key)
	if err != nil || readToken.ID != token.ID || readToken.KeyHash != hash || readToken.PlaintextKey() != key {
		t.Fatal("customer key identity changed during rotation")
	}
	var audits int64
	if err := DB.Model(&AuditLog{}).Where("path IN ?", []string{"/operator/migrate-credentials", "/operator/rotate-credentials"}).Count(&audits).Error; err != nil || audits != 2 {
		t.Fatalf("audit count: %d %v", audits, err)
	}
}

func TestCredentialMigrationRollsBackEarlierBatchesWhenLaterValueIsCorrupt(t *testing.T) {
	newTestDB(t)
	rows := make([]map[string]any, 501)
	for i := range rows {
		rows[i] = map[string]any{"id": int64(i + 1), "key": "legacy-batch-secret", "name": "migration-batch"}
	}
	rows[500]["key"] = "enc:v2:missing:invalid:invalid"
	if err := DB.Table("channels").CreateInBatches(rows, 100).Error; err != nil {
		t.Fatal(err)
	}
	if count, err := MigrateCredentialStorage(false, "test-operator", "Verify multi-batch atomicity"); err == nil || count != 0 {
		t.Fatal("corrupt second batch did not fail atomically")
	}
	var untouched int64
	if err := DB.Table("channels").Where("id <= ? AND key = ?", 500, "legacy-batch-secret").Count(&untouched).Error; err != nil || untouched != 500 {
		t.Fatal("earlier batch was partially migrated", err)
	}
	var audits int64
	DB.Model(&AuditLog{}).Where("path = ?", "/operator/migrate-credentials").Count(&audits)
	if audits != 0 {
		t.Fatal("failed migration has a success audit")
	}
	if err := DB.Table("channels").Where("id = ?", 501).UpdateColumn("key", "legacy-batch-secret").Error; err != nil {
		t.Fatal(err)
	}
	if count, err := MigrateCredentialStorage(false, "test-operator", "Retry repaired credential migration"); err != nil || count != 501 {
		t.Fatalf("retry changed %d: %v", count, err)
	}
	if count, err := MigrateCredentialStorage(false, "test-operator", "Verify unchanged migration rerun"); err != nil || count != 0 {
		t.Fatal("migration rerun was not idempotent", err)
	}
}

func TestCredentialMigrationAuditFailureRollsBackAllValues(t *testing.T) {
	newTestDB(t)
	channel := &Channel{Name: "atomic-migration", Key: "legacy-secret", Models: "model", Group: "default", Status: 1}
	if err := InsertChannel(channel); err != nil {
		t.Fatal(err)
	}
	if err := DB.Exec("UPDATE channels SET key = ? WHERE id = ?", channel.Key, channel.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := DB.Exec(`CREATE TRIGGER reject_credential_audit BEFORE INSERT ON audit_logs BEGIN SELECT RAISE(ABORT, 'audit failed'); END`).Error; err != nil {
		t.Fatal(err)
	}
	if count, err := MigrateCredentialStorage(false, "operator", "Must be audited"); err == nil || count != 0 {
		t.Fatal("audit failure was ignored")
	}
	var stored string
	if err := DB.Table("channels").Select("key").Where("id = ?", channel.ID).Row().Scan(&stored); err != nil || stored != channel.Key {
		t.Fatal("failed migration did not roll back")
	}
}
