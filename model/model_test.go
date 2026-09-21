package model

import (
	"path/filepath"
	"testing"

	"starhub/common"
)

func newTestDB(t *testing.T) {
	t.Helper()
	if err := InitDB(filepath.Join(t.TempDir(), "test.db")); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	t.Cleanup(func() {
		_ = CloseDB()
	})
}

func TestUserCRUDAndQuota(t *testing.T) {
	newTestDB(t)

	hashed, err := common.HashPassword("password-123")
	if err != nil {
		t.Fatalf("hash failed: %v", err)
	}
	user := &User{
		Username: "alice",
		Password: hashed,
		Role:     common.RoleUser,
		Status:   common.StatusEnabled,
		Quota:    1000,
		Group:    common.DefaultGroup,
	}
	if err := CreateUser(user); err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	got, err := GetUserByUsername("alice")
	if err != nil || got.ID != user.ID || got.Quota != 1000 {
		t.Fatalf("GetUserByUsername = %+v, %v", got, err)
	}
	if _, err := GetUserByUsername("bob"); err != ErrUserNotFound {
		t.Fatalf("expected ErrUserNotFound, got %v", err)
	}

	ok, err := DecreaseUserQuota(user.ID, 600)
	if err != nil || !ok {
		t.Fatalf("DecreaseUserQuota failed: %v %v", ok, err)
	}
	ok, _ = DecreaseUserQuota(user.ID, 600)
	if ok {
		t.Fatal("decreasing more than remaining should fail")
	}
	if err := IncreaseUserQuota(user.ID, 100); err != nil {
		t.Fatalf("IncreaseUserQuota failed: %v", err)
	}
	got, _ = GetUserByID(user.ID)
	if got.Quota != 500 {
		t.Fatalf("quota should be 500, got %d", got.Quota)
	}

	if err := UpdateUserFields(user.ID, map[string]any{"group": "vip", "status": common.StatusDisabled}); err != nil {
		t.Fatalf("UpdateUserFields failed: %v", err)
	}
	got, _ = GetUserByID(user.ID)
	if got.Group != "vip" || got.Status != common.StatusDisabled {
		t.Fatalf("update did not apply: %+v", got)
	}
}

func TestTokenInsertAndQuota(t *testing.T) {
	newTestDB(t)

	token := &Token{
		UserID:      1,
		Name:        "test-key",
		Status:      common.StatusEnabled,
		ExpiredTime: -1,
		RemainQuota: 100,
	}
	if err := InsertToken(token); err != nil {
		t.Fatalf("InsertToken failed: %v", err)
	}
	if len(token.Key) != 51 || token.Key[:3] != "sk-" {
		t.Fatalf("unexpected key: %q", token.Key)
	}
	got, err := GetTokenByKey(token.Key)
	if err != nil || got.ID != token.ID {
		t.Fatalf("GetTokenByKey failed: %v", err)
	}

	ok, _ := DecreaseTokenQuota(token.ID, 60)
	if !ok {
		t.Fatal("DecreaseTokenQuota within balance should succeed")
	}
	if ok, _ = DecreaseTokenQuota(token.ID, 60); ok {
		t.Fatal("DecreaseTokenQuota beyond balance should fail")
	}
	if err := IncreaseTokenQuota(token.ID, 10); err != nil {
		t.Fatalf("IncreaseTokenQuota failed: %v", err)
	}
	got, _ = GetTokenByID(token.ID)
	if got.RemainQuota != 50 {
		t.Fatalf("remain quota should be 50, got %d", got.RemainQuota)
	}
}

func TestChannelAbilitiesSync(t *testing.T) {
	newTestDB(t)

	ch := &Channel{
		Name:   "openai-main",
		Type:   common.DefaultChannelType,
		Key:    "sk-upstream",
		Models: "gpt-4o-mini, gpt-4o",
		Group:  "default",
		Status: common.ChannelStatusEnabled,
		Weight: 1,
	}
	if err := InsertChannel(ch); err != nil {
		t.Fatalf("InsertChannel failed: %v", err)
	}
	var count int64
	DB.Model(&Ability{}).Count(&count)
	if count != 2 {
		t.Fatalf("expected 2 ability rows, got %d", count)
	}

	ch.Models = "gpt-4o-mini"
	if err := UpdateChannel(ch); err != nil {
		t.Fatalf("UpdateChannel failed: %v", err)
	}
	DB.Model(&Ability{}).Count(&count)
	if count != 1 {
		t.Fatalf("abilities should be rebuilt to 1 row, got %d", count)
	}

	if err := UpdateChannelStatus(ch.ID, common.ChannelStatusAutoDisabled); err != nil {
		t.Fatalf("UpdateChannelStatus failed: %v", err)
	}
	var enabled int64
	DB.Model(&Ability{}).Where("enabled = ?", true).Count(&enabled)
	if enabled != 0 {
		t.Fatalf("abilities should be disabled with the channel, got %d", enabled)
	}

	if err := DeleteChannel(ch.ID); err != nil {
		t.Fatalf("DeleteChannel failed: %v", err)
	}
	DB.Model(&Ability{}).Count(&count)
	if count != 0 {
		t.Fatalf("abilities should be gone after channel delete, got %d", count)
	}
}

func TestGetSatisfiedChannelPriorityAndWeight(t *testing.T) {
	newTestDB(t)

	low := &Channel{Name: "low", Type: 1, Key: "k1", Models: "m1", Group: "default", Status: 1, Priority: 0, Weight: 1}
	high := &Channel{Name: "high", Type: 1, Key: "k2", Models: "m1", Group: "default", Status: 1, Priority: 10, Weight: 1}
	weighted := &Channel{Name: "weighted", Type: 1, Key: "k3", Models: "m1", Group: "default", Status: 1, Priority: 0, Weight: 100}
	for _, c := range []*Channel{low, high, weighted} {
		if err := InsertChannel(c); err != nil {
			t.Fatalf("InsertChannel(%s) failed: %v", c.Name, err)
		}
	}

	for i := 0; i < 10; i++ {
		got, err := GetSatisfiedChannel("default", "m1")
		if err != nil {
			t.Fatalf("GetSatisfiedChannel failed: %v", err)
		}
		if got.ID != high.ID {
			t.Fatalf("highest priority channel should win, got %s", got.Name)
		}
	}

	if err := UpdateChannelStatus(high.ID, common.ChannelStatusManuallyDisabled); err != nil {
		t.Fatalf("disable high failed: %v", err)
	}
	lowHits := 0
	const pickAttempts = 1500
	for i := 0; i < pickAttempts; i++ {
		got, err := GetSatisfiedChannel("default", "m1")
		if err != nil {
			t.Fatalf("GetSatisfiedChannel failed: %v", err)
		}
		if got.ID == low.ID {
			lowHits++
		}
	}
	if lowHits == 0 || lowHits > 60 {
		t.Fatalf("weighted random seems broken, low-weight channel hit %d/%d", lowHits, pickAttempts)
	}

	if _, err := GetSatisfiedChannel("default", "no-such-model"); err != ErrNoChannel {
		t.Fatalf("expected ErrNoChannel, got %v", err)
	}
	if _, err := GetSatisfiedChannel("no-such-group", "m1"); err != ErrNoChannel {
		t.Fatalf("expected ErrNoChannel for unknown group, got %v", err)
	}
}

func TestListModelsByGroup(t *testing.T) {
	newTestDB(t)

	ch := &Channel{Name: "multi", Type: 1, Key: "k", Models: "m-b, m-a", Group: "default, vip", Status: 1, Weight: 1}
	if err := InsertChannel(ch); err != nil {
		t.Fatalf("InsertChannel failed: %v", err)
	}
	models, err := ListModelsByGroup("default")
	if err != nil {
		t.Fatalf("ListModelsByGroup failed: %v", err)
	}
	if len(models) != 2 || models[0] != "m-a" || models[1] != "m-b" {
		t.Fatalf("unexpected models: %v", models)
	}
	if models, _ = ListModelsByGroup("vip"); len(models) != 2 {
		t.Fatalf("vip group should also see both models: %v", models)
	}
	if models, _ = ListModelsByGroup("other"); len(models) != 0 {
		t.Fatalf("unknown group should see no models: %v", models)
	}
}

func TestLogInsertAndQuery(t *testing.T) {
	newTestDB(t)

	for i := 0; i < 3; i++ {
		if err := InsertLog(&Log{UserID: 7, TokenID: 1, Model: "m1", PromptTokens: 10, CompletionTokens: 5, Quota: 3}); err != nil {
			t.Fatalf("InsertLog failed: %v", err)
		}
	}
	if err := InsertLog(&Log{UserID: 8, TokenID: 2, Model: "m2", PromptTokens: 1, CompletionTokens: 1, Quota: 1}); err != nil {
		t.Fatalf("InsertLog failed: %v", err)
	}

	logs, total, err := ListLogs(7, 1, 10, "")
	if err != nil || total != 3 || len(logs) != 3 {
		t.Fatalf("user 7 logs = %d rows, total %d, err %v", len(logs), total, err)
	}
	logs, total, _ = ListLogs(0, 1, 10, "m2")
	if total != 1 || logs[0].UserID != 8 {
		t.Fatalf("admin all-logs query wrong: %+v", logs)
	}
	logs, total, _ = ListLogs(7, 1, 2, "")
	if total != 3 || len(logs) != 2 {
		t.Fatalf("pagination wrong: %d rows of %d", len(logs), total)
	}
}

func TestOptionUpsertAndSeed(t *testing.T) {
	newTestDB(t)

	if err := SeedOptions(); err != nil {
		t.Fatalf("SeedOptions failed: %v", err)
	}
	value, found, err := GetOption("group_ratio")
	if err != nil || !found {
		t.Fatalf("group_ratio should be seeded: %v", err)
	}
	if value != `{"default":1}` {
		t.Fatalf("unexpected seed value: %s", value)
	}
	if err := UpsertOption("group_ratio", `{"default":1,"vip":2}`); err != nil {
		t.Fatalf("UpsertOption failed: %v", err)
	}
	value, _, _ = GetOption("group_ratio")
	if value != `{"default":1,"vip":2}` {
		t.Fatalf("upsert did not update: %s", value)
	}
}
