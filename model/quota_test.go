package model

import (
	"errors"
	"sync"
	"testing"

	"starhub/common"
)

func newBillingUser(t *testing.T, quota int64) (*User, *Token) {
	t.Helper()
	user := &User{
		Username: "bill-" + common.GenerateTokenKey()[:8],
		Password: "x",
		Role:     common.RoleUser,
		Status:   common.StatusEnabled,
		Quota:    quota,
		Group:    common.DefaultGroup,
	}
	if err := CreateUser(user); err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	token := &Token{
		UserID:      user.ID,
		Name:        "billing",
		Status:      common.StatusEnabled,
		ExpiredTime: -1,
		RemainQuota: quota,
	}
	if err := InsertToken(token); err != nil {
		t.Fatalf("InsertToken failed: %v", err)
	}
	return user, token
}

// The money-moving writes rely on SQLite writer serialization; the DSN must
// switch the database to WAL with a busy timeout so contending billing
// transactions queue instead of failing.
func TestDatabaseConcurrencyPragmas(t *testing.T) {
	newTestDB(t)

	var mode string
	if err := DB.Raw("PRAGMA journal_mode").Scan(&mode).Error; err != nil {
		t.Fatalf("read journal_mode failed: %v", err)
	}
	if mode != "wal" {
		t.Fatalf("journal_mode = %q, want wal (concurrent billing writes rely on it)", mode)
	}
	var timeout int64
	if err := DB.Raw("PRAGMA busy_timeout").Scan(&timeout).Error; err != nil {
		t.Fatalf("read busy_timeout failed: %v", err)
	}
	if timeout < 1000 {
		t.Fatalf("busy_timeout = %d, want >= 1000", timeout)
	}
}

func TestReserveQuotaShortfalls(t *testing.T) {
	newTestDB(t)

	user, token := newBillingUser(t, 100)

	if err := ReserveQuota(user.ID, token.ID, false, 101); !errors.Is(err, ErrUserQuotaShort) {
		t.Fatalf("oversized reserve should hit the user balance, got %v", err)
	}
	if got, _ := GetUserByID(user.ID); got.Quota != 100 {
		t.Fatalf("failed user reserve must not touch the balance, got %d", got.Quota)
	}
	if got, _ := GetTokenByID(token.ID); got.RemainQuota != 100 {
		t.Fatalf("failed user reserve must not touch the token, got %d", got.RemainQuota)
	}

	// user is rich, token is poor: user balance must stay untouched
	if err := UpdateUserFields(user.ID, map[string]any{"quota": 10000}); err != nil {
		t.Fatalf("topup failed: %v", err)
	}
	if err := ReserveQuota(user.ID, token.ID, false, 101); !errors.Is(err, ErrTokenQuotaShort) {
		t.Fatalf("oversized reserve should hit the token balance, got %v", err)
	}
	if got, _ := GetUserByID(user.ID); got.Quota != 10000 {
		t.Fatalf("user decrement must roll back when the token is short, got %d", got.Quota)
	}
	if got, _ := GetTokenByID(token.ID); got.RemainQuota != 100 {
		t.Fatalf("failed token reserve must not touch the token, got %d", got.RemainQuota)
	}

	// unlimited tokens only consult the user balance
	unlimited := &Token{UserID: user.ID, Name: "unlimited", Status: common.StatusEnabled,
		ExpiredTime: -1, UnlimitedQuota: true}
	if err := InsertToken(unlimited); err != nil {
		t.Fatalf("InsertToken unlimited failed: %v", err)
	}
	if err := ReserveQuota(user.ID, unlimited.ID, true, 101); err != nil {
		t.Fatalf("unlimited reserve should pass: %v", err)
	}
	if err := ReserveQuota(999999, unlimited.ID, true, 10); err == nil {
		t.Fatal("unknown user should error")
	}
}

func TestReserveReleaseSettleRoundTrip(t *testing.T) {
	newTestDB(t)

	user, token := newBillingUser(t, 1000)

	// reserve 300
	if err := ReserveQuota(user.ID, token.ID, false, 300); err != nil {
		t.Fatalf("ReserveQuota failed: %v", err)
	}
	gotUser, _ := GetUserByID(user.ID)
	gotToken, _ := GetTokenByID(token.ID)
	if gotUser.Quota != 700 || gotToken.RemainQuota != 700 {
		t.Fatalf("after reserve: user %d token %d, want 700/700", gotUser.Quota, gotToken.RemainQuota)
	}

	// settle actual 200: delta = -100 goes back, used quotas advance, log
	// row written — all in one transaction
	channel := &Channel{Name: "ch", Type: 1, Key: "k", Models: "m",
		Group: common.DefaultGroup, Status: 1, Weight: 1}
	if err := InsertChannel(channel); err != nil {
		t.Fatalf("InsertChannel failed: %v", err)
	}
	entry := &Log{UserID: user.ID, TokenID: token.ID, ChannelID: channel.ID,
		Model: "m", PromptTokens: 10, CompletionTokens: 5, Quota: 200}
	if err := SettleQuota(user.ID, token.ID, false, -100, 200, entry); err != nil {
		t.Fatalf("SettleQuota failed: %v", err)
	}
	gotUser, _ = GetUserByID(user.ID)
	gotToken, _ = GetTokenByID(token.ID)
	if gotUser.Quota != 800 || gotToken.RemainQuota != 800 {
		t.Fatalf("after settle refund: user %d token %d, want 800/800", gotUser.Quota, gotToken.RemainQuota)
	}
	if gotUser.UsedQuota != 200 || gotToken.UsedQuota != 200 {
		t.Fatalf("used quota not advanced: user %d token %d", gotUser.UsedQuota, gotToken.UsedQuota)
	}
	gotChannel, _ := GetChannelByID(channel.ID)
	if gotChannel.UsedQuota != 200 {
		t.Fatalf("channel used quota = %d, want 200", gotChannel.UsedQuota)
	}
	logs, total, _ := ListLogs(user.ID, 1, 10, "m")
	if total != 1 || len(logs) != 1 || logs[0].Quota != 200 {
		t.Fatalf("settle log missing: total=%d %+v", total, logs)
	}

	// a settle that overdraws (actual > pre) deducts the delta from the
	// remaining balance — the balance is sufficient here so the full delta
	// is taken
	if err := SettleQuota(user.ID, token.ID, false, 500, 1300, &Log{
		UserID: user.ID, TokenID: token.ID, ChannelID: 0,
		Model: "m", Quota: 1300}); err != nil {
		t.Fatalf("overdrawing settle failed: %v", err)
	}
	gotUser, _ = GetUserByID(user.ID)
	gotToken, _ = GetTokenByID(token.ID)
	if gotUser.Quota != 300 || gotToken.RemainQuota != 300 {
		t.Fatalf("after overdrawing settle: user %d token %d, want 300/300", gotUser.Quota, gotToken.RemainQuota)
	}

	// full refund of a reservation
	if err := ReleaseQuota(user.ID, token.ID, false, 300); err != nil {
		t.Fatalf("ReleaseQuota failed: %v", err)
	}
	gotUser, _ = GetUserByID(user.ID)
	gotToken, _ = GetTokenByID(token.ID)
	if gotUser.Quota != 600 || gotToken.RemainQuota != 600 {
		t.Fatalf("after release: user %d token %d, want 600/600", gotUser.Quota, gotToken.RemainQuota)
	}
}

// TestSettleQuotaFloorAtZero verifies that a settlement whose delta exceeds
// the available balance clamps at zero instead of driving the account
// negative. With max_tokens-aware pre-charging, a positive delta should be
// rare and small; when it does exceed the balance, the gateway eats the
// overage rather than allowing debt (which would defeat the overdraft
// protection that the pre-charge provides).
func TestSettleQuotaFloorAtZero(t *testing.T) {
	newTestDB(t)

	user, token := newBillingUser(t, 100)

	// reserve 80: user=20, token=20
	if err := ReserveQuota(user.ID, token.ID, false, 80); err != nil {
		t.Fatalf("ReserveQuota failed: %v", err)
	}

	// settle actual=130, pre=80, delta=+50 — but balance is only 20.
	// The deduction must clamp at zero, not drive the balance to -30.
	entry := &Log{UserID: user.ID, TokenID: token.ID, ChannelID: 0,
		Model: "m", Quota: 130}
	if err := SettleQuota(user.ID, token.ID, false, 50, 130, entry); err != nil {
		t.Fatalf("SettleQuota failed: %v", err)
	}
	gotUser, _ := GetUserByID(user.ID)
	gotToken, _ := GetTokenByID(token.ID)
	if gotUser.Quota != 0 {
		t.Fatalf("user quota = %d, want 0 (clamped, not negative)", gotUser.Quota)
	}
	if gotToken.RemainQuota != 0 {
		t.Fatalf("token remain_quota = %d, want 0 (clamped, not negative)", gotToken.RemainQuota)
	}
	// used_quota still advances by the full actual cost
	if gotUser.UsedQuota != 130 || gotToken.UsedQuota != 130 {
		t.Fatalf("used quota: user %d token %d, want 130/130", gotUser.UsedQuota, gotToken.UsedQuota)
	}
}

func TestReserveQuotaConcurrentNoDoubleSpend(t *testing.T) {
	newTestDB(t)

	const workers = 32
	const amount = 25
	user, token := newBillingUser(t, 100) // exactly 4 reservations fit

	var mu sync.Mutex
	var success, userShort, tokenShort int
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := ReserveQuota(user.ID, token.ID, false, amount)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				success++
			case errors.Is(err, ErrUserQuotaShort):
				userShort++
			case errors.Is(err, ErrTokenQuotaShort):
				tokenShort++
			}
		}()
	}
	wg.Wait()

	if success != 4 {
		t.Fatalf("exactly 4 reservations should win, got %d (userShort=%d tokenShort=%d)",
			success, userShort, tokenShort)
	}
	gotUser, _ := GetUserByID(user.ID)
	gotToken, _ := GetTokenByID(token.ID)
	if gotUser.Quota != 0 || gotToken.RemainQuota != 0 {
		t.Fatalf("balance after concurrent reserves: user %d token %d, want 0/0",
			gotUser.Quota, gotToken.RemainQuota)
	}
	if gotUser.Quota < 0 || gotToken.RemainQuota < 0 {
		t.Fatal("balance must never go negative")
	}
}
