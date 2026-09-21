package model

import (
	"path/filepath"
	"testing"
	"time"

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

func TestGetSatisfiedChannelExcluding(t *testing.T) {
	newTestDB(t)

	high := &Channel{Name: "high", Type: 1, Key: "k1", Models: "m1", Group: "default", Status: 1, Priority: 10, Weight: 1}
	mid := &Channel{Name: "mid", Type: 1, Key: "k2", Models: "m1", Group: "default", Status: 1, Priority: 5, Weight: 1}
	low := &Channel{Name: "low", Type: 1, Key: "k3", Models: "m1", Group: "default", Status: 1, Priority: 0, Weight: 1}
	for _, c := range []*Channel{high, mid, low} {
		if err := InsertChannel(c); err != nil {
			t.Fatalf("InsertChannel(%s) failed: %v", c.Name, err)
		}
	}

	// excluding the top tier degrades to the next priority layer
	got, err := GetSatisfiedChannelExcluding("default", "m1", map[int64]bool{high.ID: true})
	if err != nil || got.ID != mid.ID {
		t.Fatalf("exclude high: got %+v, %v; want mid", got, err)
	}
	// excluding the whole top tier still works via the bottom layer
	got, err = GetSatisfiedChannelExcluding("default", "m1", map[int64]bool{high.ID: true, mid.ID: true})
	if err != nil || got.ID != low.ID {
		t.Fatalf("exclude high+mid: got %+v, %v; want low", got, err)
	}
	// excluding everything leaves nothing
	if _, err := GetSatisfiedChannelExcluding("default", "m1",
		map[int64]bool{high.ID: true, mid.ID: true, low.ID: true}); err != ErrNoChannel {
		t.Fatalf("expected ErrNoChannel after excluding all, got %v", err)
	}
	// a disabled channel is never offered, even when not excluded
	if err := UpdateChannelStatus(mid.ID, common.ChannelStatusAutoDisabled); err != nil {
		t.Fatalf("disable mid failed: %v", err)
	}
	got, err = GetSatisfiedChannelExcluding("default", "m1", map[int64]bool{high.ID: true})
	if err != nil || got.ID != low.ID {
		t.Fatalf("exclude high with mid disabled: got %+v, %v; want low", got, err)
	}
	// nil exclude behaves like the plain selector
	got, err = GetSatisfiedChannelExcluding("default", "m1", nil)
	if err != nil || got.ID != high.ID {
		t.Fatalf("nil exclude should pick the highest tier, got %+v, %v", got, err)
	}
}

func TestMultiKeySelectAndKeyDisable(t *testing.T) {
	newTestDB(t)
	ResetMultiKeyCounters()

	single := &Channel{Name: "single", Type: 1, Key: "k-only", Models: "m1",
		Group: "default", Status: 1, Weight: 1}
	if err := InsertChannel(single); err != nil {
		t.Fatalf("InsertChannel(single) failed: %v", err)
	}
	if single.IsMultiKey() {
		t.Fatal("one key is not a multi-key channel")
	}
	key, idx := single.SelectKey()
	if key != "k-only" || idx != 0 {
		t.Fatalf("single key selection = %q,%d; want k-only,0", key, idx)
	}

	multi := &Channel{Name: "multi", Type: 1, Key: "k-bad\nk-good-1\nk-good-2",
		KeyMode: KeyModeRoundRobin, Models: "m1", Group: "default", Status: 1, Weight: 1}
	if err := InsertChannel(multi); err != nil {
		t.Fatalf("InsertChannel(multi) failed: %v", err)
	}
	if !multi.IsMultiKey() {
		t.Fatal("three keys should be multi-key")
	}
	statuses := multi.KeyStatuses()
	if len(statuses) != 3 || statuses[0] != common.StatusEnabled {
		t.Fatalf("default key statuses wrong: %v", statuses)
	}

	// round robin starts at the first key and walks the list
	for i, want := range []string{"k-bad", "k-good-1", "k-good-2"} {
		key, idx := multi.SelectKey()
		if key != want || idx != i {
			t.Fatalf("round robin pick %d = %q,%d; want %q,%d", i, key, idx, want, i)
		}
	}

	// disabling key 0 removes it from rotation without touching the channel
	allDisabled, err := DisableChannelKey(multi.ID, 0)
	if err != nil || allDisabled {
		t.Fatalf("DisableChannelKey(0) = %v, %v; want false, nil", allDisabled, err)
	}
	got, _ := GetChannelByID(multi.ID)
	if got.Status != common.ChannelStatusEnabled {
		t.Fatalf("channel must stay enabled after one key dies, got %d", got.Status)
	}
	statuses = got.KeyStatuses()
	if len(statuses) != 3 || statuses[0] != common.StatusDisabled ||
		statuses[1] != common.StatusEnabled || statuses[2] != common.StatusEnabled {
		t.Fatalf("per-key statuses after disable: %v", statuses)
	}
	// a fresh rotation cursor skips the disabled key
	ResetMultiKeyCounters()
	for i, want := range []string{"k-good-1", "k-good-2", "k-good-1"} {
		key, _ := got.SelectKey()
		if key != want {
			t.Fatalf("rotation after disable, pick %d = %q; want %q", i, key, want)
		}
	}

	// random mode only ever returns enabled keys
	got, _ = GetChannelByID(multi.ID)
	got.KeyMode = KeyModeRandom
	for i := 0; i < 50; i++ {
		key, _ := got.SelectKey()
		if key != "k-good-1" && key != "k-good-2" {
			t.Fatalf("random mode returned a disabled key: %q", key)
		}
	}

	// when the last key dies the caller is told to disable the channel
	for _, idx := range []int{1, 2} {
		allDisabled, err = DisableChannelKey(multi.ID, idx)
		if err != nil {
			t.Fatalf("DisableChannelKey(%d) failed: %v", idx, err)
		}
	}
	if !allDisabled {
		t.Fatal("disabling the last key should report allDisabled")
	}
	fresh, _ := GetChannelByID(multi.ID)
	key, idx = fresh.SelectKey()
	if key != "" || idx != -1 {
		t.Fatalf("all keys disabled should yield empty key, got %q,%d", key, idx)
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

func TestSummarizeLogsByDay(t *testing.T) {
	newTestDB(t)

	today := time.Now().UTC().Truncate(24 * time.Hour)
	at := func(dayOffset, hour int) int64 {
		return today.AddDate(0, 0, dayOffset).Add(time.Duration(hour) * time.Hour).Unix()
	}
	rows := []Log{
		{UserID: 1, Model: "m", PromptTokens: 10, CompletionTokens: 5, Quota: 3, CreatedAt: at(0, 1)},
		{UserID: 1, Model: "m", PromptTokens: 20, CompletionTokens: 5, Quota: 4, CreatedAt: at(0, 2)},
		{UserID: 1, Model: "m", PromptTokens: 7, CompletionTokens: 1, Quota: 2, CreatedAt: at(-1, 23)},
		{UserID: 2, Model: "m", PromptTokens: 1, CompletionTokens: 1, Quota: 9, CreatedAt: at(0, 3)},
	}
	for i := range rows {
		if err := InsertLog(&rows[i]); err != nil {
			t.Fatalf("InsertLog failed: %v", err)
		}
	}

	if total, err := CountLogs(1); err != nil || total != 3 {
		t.Fatalf("CountLogs(1) = %d, %v; want 3", total, err)
	}
	if total, err := CountLogs(0); err != nil || total != 4 {
		t.Fatalf("CountLogs(0) = %d, %v; want 4", total, err)
	}

	since := today.AddDate(0, 0, -6).Unix()
	usage, err := SummarizeLogsByDay(1, since)
	if err != nil {
		t.Fatalf("SummarizeLogsByDay failed: %v", err)
	}
	if len(usage) != 2 {
		t.Fatalf("user 1 should have 2 daily rows, got %d", len(usage))
	}
	yesterday := today.AddDate(0, 0, -1).Format("2006-01-02")
	if usage[0].Date != yesterday || usage[0].Requests != 1 || usage[0].Quota != 2 ||
		usage[0].PromptTokens != 7 || usage[0].CompletionTokens != 1 {
		t.Fatalf("yesterday aggregate wrong: %+v", usage[0])
	}
	if usage[1].Date != today.Format("2006-01-02") || usage[1].Requests != 2 || usage[1].Quota != 7 ||
		usage[1].PromptTokens != 30 || usage[1].CompletionTokens != 10 {
		t.Fatalf("today aggregate wrong: %+v", usage[1])
	}

	all, err := SummarizeLogsByDay(0, since)
	if err != nil || len(all) != 2 {
		t.Fatalf("all-users aggregate rows = %d, %v; want 2", len(all), err)
	}
	if all[1].Requests != 3 || all[1].Quota != 16 {
		t.Fatalf("all-users today aggregate wrong: %+v", all[1])
	}

	recent, err := SummarizeLogsByDay(0, at(0, 2))
	if err != nil || len(recent) != 1 || recent[0].Requests != 2 {
		t.Fatalf("since-filter aggregate wrong: %+v, %v", recent, err)
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
