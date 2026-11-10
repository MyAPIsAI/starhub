package model

import (
	"testing"

	"starhub/common"
)

func TestTokenCreationLimit(t *testing.T) {
	newTestDB(t)
	user := &User{Username: "limit-test", Password: "x", Role: 1, Status: 1, Quota: 0, Group: "default", AffCode: common.GenerateAffCode()}
	if err := CreateUser(user); err != nil {
		t.Fatal(err)
	}

	// Create MaxTokensPerUser tokens.
	for i := 0; i < MaxTokensPerUser; i++ {
		tok := &Token{
			UserID:      user.ID,
			Name:        "key-" + itoa(i),
			Status:      common.StatusEnabled,
			ExpiredTime: -1,
			RemainQuota: 100,
		}
		if err := InsertToken(tok); err != nil {
			t.Fatalf("InsertToken #%d failed: %v", i, err)
		}
	}

	// The next one should fail.
	tok := &Token{
		UserID:      user.ID,
		Name:        "over-limit",
		Status:      common.StatusEnabled,
		ExpiredTime: -1,
		RemainQuota: 100,
	}
	err := InsertToken(tok)
	if err != ErrTokenLimitExceeded {
		t.Fatalf("expected ErrTokenLimitExceeded, got %v", err)
	}

	// Delete one and try again — should succeed.
	tokens, _ := ListTokens(user.ID)
	if err := DeleteToken(tokens[0].ID); err != nil {
		t.Fatal(err)
	}
	tok2 := &Token{
		UserID:      user.ID,
		Name:        "after-delete",
		Status:      common.StatusEnabled,
		ExpiredTime: -1,
		RemainQuota: 100,
	}
	if err := InsertToken(tok2); err != nil {
		t.Fatalf("InsertToken after delete failed: %v", err)
	}
}

func TestTokenCreationLimitDifferentUsers(t *testing.T) {
	newTestDB(t)
	user1 := &User{Username: "user1", Password: "x", Role: 1, Status: 1, Quota: 0, Group: "default", AffCode: common.GenerateAffCode()}
	user2 := &User{Username: "user2", Password: "x", Role: 1, Status: 1, Quota: 0, Group: "default", AffCode: common.GenerateAffCode()}
	CreateUser(user1)
	CreateUser(user2)

	// Fill user1 to the limit.
	for i := 0; i < MaxTokensPerUser; i++ {
		InsertToken(&Token{UserID: user1.ID, Name: "k", Status: 1, ExpiredTime: -1, RemainQuota: 1})
	}

	// User2 should still be able to create keys.
	err := InsertToken(&Token{UserID: user2.ID, Name: "user2-key", Status: 1, ExpiredTime: -1, RemainQuota: 1})
	if err != nil {
		t.Fatalf("user2 should not be affected by user1's limit: %v", err)
	}
}

func itoa(i int) string {
	return string(rune('0' + i%10)) // simple single digit for test names
}
