package model

import (
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"starhub/common"
)

func newQuotaUser(t *testing.T, quota int64) *User {
	t.Helper()
	u := &User{
		Username: "redeemer-" + time.Now().Format("150405.000000"),
		Password: "x",
		Role:     common.RoleUser,
		Status:   common.StatusEnabled,
		Quota:    quota,
		Group:    common.DefaultGroup,
		AffCode:  common.GenerateAffCode(),
	}
	if err := CreateUser(u); err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	return u
}

func TestInsertRedemptionGeneratesCode(t *testing.T) {
	newTestDB(t)

	r := &Redemption{Quota: 1000, Status: common.StatusEnabled}
	if err := InsertRedemption(r); err != nil {
		t.Fatalf("InsertRedemption failed: %v", err)
	}
	if !strings.HasPrefix(r.Code, "RD-") {
		t.Fatalf("generated code %q does not start with RD-", r.Code)
	}
	if len(r.Code) != 3+24 {
		t.Fatalf("generated code %q has unexpected length %d", r.Code, len(r.Code))
	}

	// A caller-supplied code must be preserved verbatim.
	custom := &Redemption{Quota: 1, Status: common.StatusEnabled, Code: "MY-CODE"}
	if err := InsertRedemption(custom); err != nil {
		t.Fatalf("InsertRedemption(custom) failed: %v", err)
	}
	got, err := GetRedemptionByCode("MY-CODE")
	if err != nil || got.ID != custom.ID {
		t.Fatalf("GetRedemptionByCode = %+v, %v", got, err)
	}
}

func TestGetRedemptionByCodeErrors(t *testing.T) {
	newTestDB(t)
	if _, err := GetRedemptionByCode(""); err != ErrRedemptionNotFound {
		t.Fatalf("empty code: want ErrRedemptionNotFound, got %v", err)
	}
	if _, err := GetRedemptionByCode("nope"); err != ErrRedemptionNotFound {
		t.Fatalf("unknown code: want ErrRedemptionNotFound, got %v", err)
	}
}

func TestRedeemCodeCreditsOnce(t *testing.T) {
	newTestDB(t)
	user := newQuotaUser(t, 100)
	r := &Redemption{Quota: 5000, Status: common.StatusEnabled, Code: "ONCE"}
	if err := InsertRedemption(r); err != nil {
		t.Fatalf("InsertRedemption failed: %v", err)
	}

	credited, err := RedeemCode("ONCE", user.ID)
	if err != nil {
		t.Fatalf("RedeemCode failed: %v", err)
	}
	if credited != 5000 {
		t.Fatalf("credited = %d, want 5000", credited)
	}
	got, _ := GetUserByID(user.ID)
	if got.Quota != 5100 {
		t.Fatalf("quota = %d, want 5100", got.Quota)
	}
	used, err := GetRedemptionByCode("ONCE")
	if err != nil {
		t.Fatalf("GetRedemptionByCode failed: %v", err)
	}
	if used.Status != common.StatusDisabled {
		t.Fatalf("status = %d, want disabled after redeem", used.Status)
	}
	if used.UsedBy != user.ID || used.UsedAt == 0 {
		t.Fatalf("used_by/used_at not recorded: %+v", used)
	}

	// Second redeem must fail and leave the balance alone.
	if _, err := RedeemCode("ONCE", user.ID); err != ErrRedemptionNotFound {
		t.Fatalf("second redeem: want ErrRedemptionNotFound, got %v", err)
	}
	got, _ = GetUserByID(user.ID)
	if got.Quota != 5100 {
		t.Fatalf("quota changed on failed redeem: %d", got.Quota)
	}
}

func TestRedeemCodeRejectsExpiredAndDisabled(t *testing.T) {
	newTestDB(t)
	user := newQuotaUser(t, 0)

	expired := &Redemption{
		Quota: 100, Status: common.StatusEnabled, Code: "EXPIRED",
		ExpiredAt: time.Now().Add(-time.Hour).Unix(),
	}
	if err := InsertRedemption(expired); err != nil {
		t.Fatalf("InsertRedemption failed: %v", err)
	}
	if _, err := RedeemCode("EXPIRED", user.ID); err != ErrRedemptionNotFound {
		t.Fatalf("expired code: want ErrRedemptionNotFound, got %v", err)
	}

	disabled := &Redemption{Quota: 100, Status: common.StatusDisabled, Code: "DISABLED"}
	if err := InsertRedemption(disabled); err != nil {
		t.Fatalf("InsertRedemption failed: %v", err)
	}
	if _, err := RedeemCode("DISABLED", user.ID); err != ErrRedemptionNotFound {
		t.Fatalf("disabled code: want ErrRedemptionNotFound, got %v", err)
	}

	got, _ := GetUserByID(user.ID)
	if got.Quota != 0 {
		t.Fatalf("quota = %d, want 0 (nothing should have been credited)", got.Quota)
	}
}

// A code with ExpiredAt in the future is still redeemable; 0 means "never
// expires" and must not be treated as already expired.
func TestRedeemCodeAcceptsFutureAndNeverExpiry(t *testing.T) {
	newTestDB(t)
	user := newQuotaUser(t, 0)

	never := &Redemption{Quota: 10, Status: common.StatusEnabled, Code: "NEVER", ExpiredAt: 0}
	future := &Redemption{
		Quota: 20, Status: common.StatusEnabled, Code: "FUTURE",
		ExpiredAt: time.Now().Add(time.Hour).Unix(),
	}
	for _, r := range []*Redemption{never, future} {
		if err := InsertRedemption(r); err != nil {
			t.Fatalf("InsertRedemption failed: %v", err)
		}
	}
	for _, code := range []string{"NEVER", "FUTURE"} {
		if _, err := RedeemCode(code, user.ID); err != nil {
			t.Fatalf("RedeemCode(%s) failed: %v", code, err)
		}
	}
	got, _ := GetUserByID(user.ID)
	if got.Quota != 30 {
		t.Fatalf("quota = %d, want 30", got.Quota)
	}
}

// RedeemCode claims the code inside a transaction with a status guard, so
// exactly one concurrent caller may win. This is the regression test for that
// guard: without it the balance would be credited more than once.
func TestRedeemCodeConcurrentSingleWinner(t *testing.T) {
	newTestDB(t)
	user := newQuotaUser(t, 0)

	const faceValue = 1000
	r := &Redemption{Quota: faceValue, Status: common.StatusEnabled, Code: "RACE"}
	if err := InsertRedemption(r); err != nil {
		t.Fatalf("InsertRedemption failed: %v", err)
	}

	const workers = 8
	var wins atomic.Int64
	var failures atomic.Int64
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if _, err := RedeemCode("RACE", user.ID); err != nil {
				failures.Add(1)
				return
			}
			wins.Add(1)
		}()
	}
	close(start)
	wg.Wait()

	if wins.Load() != 1 {
		t.Fatalf("winners = %d, want exactly 1 (losers = %d)", wins.Load(), failures.Load())
	}
	got, err := GetUserByID(user.ID)
	if err != nil {
		t.Fatalf("GetUserByID failed: %v", err)
	}
	if got.Quota != faceValue {
		t.Fatalf("quota = %d, want %d credited exactly once", got.Quota, faceValue)
	}
}

func TestDeleteRedemptionNotFound(t *testing.T) {
	newTestDB(t)
	if err := DeleteRedemption(9999); err != ErrRedemptionNotFound {
		t.Fatalf("want ErrRedemptionNotFound, got %v", err)
	}
}

func TestListRedemptionsNewestFirst(t *testing.T) {
	newTestDB(t)
	for _, code := range []string{"A", "B", "C"} {
		if err := InsertRedemption(&Redemption{Quota: 1, Status: common.StatusEnabled, Code: code}); err != nil {
			t.Fatalf("InsertRedemption failed: %v", err)
		}
	}
	got, err := ListRedemptions()
	if err != nil {
		t.Fatalf("ListRedemptions failed: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	if got[0].Code != "C" || got[2].Code != "A" {
		t.Fatalf("order = %s,%s,%s; want C,B,A", got[0].Code, got[1].Code, got[2].Code)
	}
}
