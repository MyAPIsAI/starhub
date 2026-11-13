package model

import (
	"starhub/common"
	"strings"
	"testing"
)

func TestChannelCredentialsAreEncryptedAcrossStructAndMapWrites(t *testing.T) {
	newTestDB(t)
	channel := &Channel{Name: "encrypted", Key: "first-key\nsecond-key", Models: "model", Group: "default", Status: 1}
	if err := InsertChannel(channel); err != nil {
		t.Fatal(err)
	}
	if channel.Key != "first-key\nsecond-key" {
		t.Fatal("write mutated in-memory key")
	}
	check := func(want string) {
		t.Helper()
		var stored string
		if err := DB.Table("channels").Select("key").Where("id = ?", channel.ID).Row().Scan(&stored); err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(stored, common.CredentialEnvelopePrefix) || strings.Contains(stored, want) {
			t.Fatal("upstream key stored as plaintext")
		}
		read, err := GetChannelByID(channel.ID)
		if err != nil || read.Key != want {
			t.Fatalf("credential round trip failed: %v", err)
		}
	}
	check(channel.Key)
	channel.Key = "replacement-key"
	if err := UpdateChannel(channel); err != nil {
		t.Fatal(err)
	}
	check(channel.Key)
	if err := DB.Model(&Channel{}).Where("id = ?", channel.ID).Updates(map[string]any{"key": "mapped-key"}).Error; err != nil {
		t.Fatal(err)
	}
	check("mapped-key")
	if err := DB.Exec("UPDATE channels SET key = ? WHERE id = ?", "legacy-key", channel.ID).Error; err != nil {
		t.Fatal(err)
	}
	read, err := GetChannelByID(channel.ID)
	if err != nil || read.Key != "legacy-key" {
		t.Fatal("legacy plaintext compatibility failed")
	}
	if err := DB.Exec("UPDATE channels SET key = ? WHERE id = ?", "enc:v2:missing:invalid:invalid", channel.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := GetChannelByID(channel.ID); err == nil {
		t.Fatal("corrupted ciphertext treated as a usable key")
	}
}
