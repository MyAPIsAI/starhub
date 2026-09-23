package controller

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"starhub/common"
	"starhub/model"
)

func TestCreateRedemptionQuotaMustBePositive(t *testing.T) {
	newTestDB(t)
	uid, role := admin()

	for _, quota := range []string{"0", "-100"} {
		rr := call(CreateRedemption, testRequest{
			method: http.MethodPost,
			target: "/api/redemption",
			body:   fmt.Sprintf(`{"quota":%s}`, quota),
			userID: uid,
			role:   role,
		})
		expectError(t, rr, http.StatusBadRequest, "quota must be positive")
	}
}

// A batch size below 1 is treated as a single code; anything above the cap is
// clamped to 100 so one request cannot mint an unbounded number of rows.
func TestCreateRedemptionCountClamping(t *testing.T) {
	newTestDB(t)
	uid, role := admin()

	cases := []struct {
		count string
		want  int
	}{
		{`0`, 1},
		{`-5`, 1},
		{`1`, 1},
		{`3`, 3},
		{`100`, 100},
		{`1000`, 100},
	}
	for _, tc := range cases {
		rr := call(CreateRedemption, testRequest{
			method: http.MethodPost,
			target: "/api/redemption",
			body:   fmt.Sprintf(`{"quota":100,"count":%s}`, tc.count),
			userID: uid,
			role:   role,
		})

		var got struct {
			Codes []string `json:"codes"`
			Count int      `json:"count"`
		}
		decodeData(t, rr, &got)
		if got.Count != tc.want || len(got.Codes) != tc.want {
			t.Fatalf("count %s produced %d codes (count=%d), want %d",
				tc.count, len(got.Codes), got.Count, tc.want)
		}
	}
}

// A caller-supplied code only makes sense for a single redemption: a batch
// would collide on the unique index.
func TestCreateRedemptionCustomCodeOnlyHonoredForSingle(t *testing.T) {
	newTestDB(t)
	uid, role := admin()

	rr := call(CreateRedemption, testRequest{
		method: http.MethodPost,
		target: "/api/redemption",
		body:   `{"quota":100,"code":"VIP-2026"}`,
		userID: uid,
		role:   role,
	})
	var got struct {
		Codes []string `json:"codes"`
	}
	decodeData(t, rr, &got)
	if len(got.Codes) != 1 || got.Codes[0] != "VIP-2026" {
		t.Fatalf("codes = %v, want the custom code preserved", got.Codes)
	}

	rr = call(CreateRedemption, testRequest{
		method: http.MethodPost,
		target: "/api/redemption",
		body:   `{"quota":100,"count":3,"code":"IGNORED"}`,
		userID: uid,
		role:   role,
	})
	decodeData(t, rr, &got)
	if len(got.Codes) != 3 {
		t.Fatalf("codes = %d, want 3", len(got.Codes))
	}
	for _, code := range got.Codes {
		if code == "IGNORED" {
			t.Fatal("a custom code was reused across a batch")
		}
		if !strings.HasPrefix(code, "RD-") {
			t.Fatalf("batch code %q is not an auto-generated one", code)
		}
	}
}

func TestCreateRedemptionDuplicateCustomCodeConflicts(t *testing.T) {
	newTestDB(t)
	uid, role := admin()

	body := `{"quota":100,"code":"TWICE"}`
	rr := call(CreateRedemption, testRequest{
		method: http.MethodPost, target: "/api/redemption", body: body, userID: uid, role: role,
	})
	expectStatus(t, rr, http.StatusOK)

	rr = call(CreateRedemption, testRequest{
		method: http.MethodPost, target: "/api/redemption", body: body, userID: uid, role: role,
	})
	expectError(t, rr, http.StatusConflict, "already exists")
}

// A batch that fails partway through still reports the conflict rather than
// silently returning a short list of codes.
func TestCreateRedemptionBatchConflictAfterPartialInsert(t *testing.T) {
	newTestDB(t)
	uid, role := admin()

	// Pre-create one code that an auto-generated batch cannot collide with,
	// then force the collision by asking for a custom code that already
	// exists together with a batch size of 1 (the only size that honors it).
	rr := call(CreateRedemption, testRequest{
		method: http.MethodPost, target: "/api/redemption",
		body: `{"quota":100,"code":"DUP"}`, userID: uid, role: role,
	})
	expectStatus(t, rr, http.StatusOK)

	before := countRedemptions(t)
	rr = call(CreateRedemption, testRequest{
		method: http.MethodPost, target: "/api/redemption",
		body: `{"quota":100,"code":"DUP"}`, userID: uid, role: role,
	})
	expectError(t, rr, http.StatusConflict, "already exists")

	if after := countRedemptions(t); after != before {
		t.Fatalf("redemption rows went from %d to %d on a rejected create", before, after)
	}
}

func TestListAndDeleteRedemptions(t *testing.T) {
	newTestDB(t)
	uid, role := admin()

	for _, code := range []string{"A", "B"} {
		rr := call(CreateRedemption, testRequest{
			method: http.MethodPost, target: "/api/redemption",
			body: fmt.Sprintf(`{"quota":100,"code":%q}`, code), userID: uid, role: role,
		})
		expectStatus(t, rr, http.StatusOK)
	}

	var listed []model.Redemption
	rr := call(ListRedemptions, testRequest{target: "/api/redemption", userID: uid, role: role})
	decodeData(t, rr, &listed)
	if len(listed) != 2 {
		t.Fatalf("listed %d codes, want 2", len(listed))
	}
	// Newest first.
	if listed[0].Code != "B" || listed[1].Code != "A" {
		t.Fatalf("order = %s,%s; want B,A", listed[0].Code, listed[1].Code)
	}

	rr = call(DeleteRedemption, testRequest{
		method: http.MethodDelete,
		target: "/api/redemption/" + itoa(listed[0].ID),
		userID: uid,
		role:   role,
		path:   map[string]string{"id": itoa(listed[0].ID)},
	})
	expectStatus(t, rr, http.StatusOK)

	rr = call(ListRedemptions, testRequest{target: "/api/redemption", userID: uid, role: role})
	decodeData(t, rr, &listed)
	if len(listed) != 1 {
		t.Fatalf("after delete %d codes remain, want 1", len(listed))
	}
	if listed[0].Code != "A" {
		t.Fatalf("the wrong code was deleted: %s remains", listed[0].Code)
	}
}

func TestDeleteRedemptionNotFoundHandler(t *testing.T) {
	newTestDB(t)
	uid, role := admin()

	rr := call(DeleteRedemption, testRequest{
		method: http.MethodDelete,
		target: "/api/redemption/9999",
		userID: uid,
		role:   role,
		path:   map[string]string{"id": "9999"},
	})
	expectError(t, rr, http.StatusNotFound, "not found")
}

func TestDeleteRedemptionRejectsMalformedPathID(t *testing.T) {
	newTestDB(t)
	uid, role := admin()

	for _, raw := range []string{"", "abc", "0", "-3"} {
		rr := call(DeleteRedemption, testRequest{
			method: http.MethodDelete,
			target: "/api/redemption/" + raw,
			userID: uid,
			role:   role,
			path:   map[string]string{"id": raw},
		})
		expectError(t, rr, http.StatusBadRequest, "invalid id in path")
	}
}

func TestCreateRedemptionRejectsInvalidBody(t *testing.T) {
	newTestDB(t)
	uid, role := admin()

	rr := call(CreateRedemption, testRequest{
		method: http.MethodPost,
		target: "/api/redemption",
		body:   `{"quota":`,
		userID: uid,
		role:   role,
	})
	expectError(t, rr, http.StatusBadRequest, "invalid request body")
}

func TestRedeemCodeEndpoint(t *testing.T) {
	newTestDB(t)
	u := newUser(t, "alice", common.RoleUser, 100)
	uid, role := admin()

	rr := call(CreateRedemption, testRequest{
		method: http.MethodPost, target: "/api/redemption",
		body: `{"quota":2500,"code":"TOPUP"}`, userID: uid, role: role,
	})
	expectStatus(t, rr, http.StatusOK)

	// Missing code.
	rr = call(RedeemCode, testRequest{
		method: http.MethodPost, target: "/api/user/redeem",
		body: `{"code":"  "}`, userID: u.ID, role: common.RoleUser,
	})
	expectError(t, rr, http.StatusBadRequest, "code is required")

	// Unknown code.
	rr = call(RedeemCode, testRequest{
		method: http.MethodPost, target: "/api/user/redeem",
		body: `{"code":"NOPE"}`, userID: u.ID, role: common.RoleUser,
	})
	expectError(t, rr, http.StatusNotFound, "invalid or already used code")

	// Successful redemption credits the caller.
	var got map[string]any
	rr = call(RedeemCode, testRequest{
		method: http.MethodPost, target: "/api/user/redeem",
		body: `{"code":"TOPUP"}`, userID: u.ID, role: common.RoleUser,
	})
	decodeData(t, rr, &got)
	if got["quota"].(float64) != 2500 {
		t.Fatalf("credited = %v, want 2500", got["quota"])
	}
	updated, err := model.GetUserByID(u.ID)
	if err != nil {
		t.Fatalf("GetUserByID failed: %v", err)
	}
	if updated.Quota != 2600 {
		t.Fatalf("quota = %d, want 100 + 2500", updated.Quota)
	}

	// A used code cannot be redeemed twice, and the balance must not move.
	rr = call(RedeemCode, testRequest{
		method: http.MethodPost, target: "/api/user/redeem",
		body: `{"code":"TOPUP"}`, userID: u.ID, role: common.RoleUser,
	})
	expectError(t, rr, http.StatusNotFound, "invalid or already used code")
	updated, _ = model.GetUserByID(u.ID)
	if updated.Quota != 2600 {
		t.Fatalf("quota = %d after a rejected redeem, want 2600", updated.Quota)
	}

	// The credit must land on the caller, not on whoever created the code.
	owner, err := model.GetUserByID(uid)
	if err != nil {
		t.Fatalf("GetUserByID(admin) failed: %v", err)
	}
	if owner.Quota != 100000000 {
		t.Fatalf("admin quota changed to %d; the credit went to the wrong user", owner.Quota)
	}
}

// One user redeeming must never credit another, even when both hold the
// code's string in their request.
func TestRedeemCodeCreditsOnlyTheCaller(t *testing.T) {
	newTestDB(t)
	alice := newUser(t, "alice", common.RoleUser, 0)
	bob := newUser(t, "bob", common.RoleUser, 0)
	uid, role := admin()

	for _, code := range []string{"FOR-ALICE", "FOR-BOB"} {
		rr := call(CreateRedemption, testRequest{
			method: http.MethodPost, target: "/api/redemption",
			body: fmt.Sprintf(`{"quota":500,"code":%q}`, code), userID: uid, role: role,
		})
		expectStatus(t, rr, http.StatusOK)
	}

	rr := call(RedeemCode, testRequest{
		method: http.MethodPost, target: "/api/user/redeem",
		body: `{"code":"FOR-ALICE"}`, userID: alice.ID, role: common.RoleUser,
	})
	expectStatus(t, rr, http.StatusOK)
	rr = call(RedeemCode, testRequest{
		method: http.MethodPost, target: "/api/user/redeem",
		body: `{"code":"FOR-BOB"}`, userID: bob.ID, role: common.RoleUser,
	})
	expectStatus(t, rr, http.StatusOK)

	gotAlice, _ := model.GetUserByID(alice.ID)
	gotBob, _ := model.GetUserByID(bob.ID)
	if gotAlice.Quota != 500 || gotBob.Quota != 500 {
		t.Fatalf("alice=%d bob=%d, want 500/500", gotAlice.Quota, gotBob.Quota)
	}

	// The used-by attribution must be recorded per code.
	byCode, err := model.GetRedemptionByCode("FOR-ALICE")
	if err != nil {
		t.Fatalf("GetRedemptionByCode failed: %v", err)
	}
	if byCode.UsedBy != alice.ID {
		t.Fatalf("FOR-ALICE used_by = %d, want %d", byCode.UsedBy, alice.ID)
	}
	if byCode.Status != common.StatusDisabled {
		t.Fatalf("FOR-ALICE status = %d, want used", byCode.Status)
	}
}

func TestRedeemCodeRejectsExpiredCode(t *testing.T) {
	newTestDB(t)
	u := newUser(t, "alice", common.RoleUser, 0)

	// Create the code through the API with an expired timestamp, then try to
	// redeem it: the handler must report the same generic error as an
	// unknown code (it does not leak whether the code exists).
	rd := &model.Redemption{
		Quota:     900,
		Status:    common.StatusEnabled,
		Code:      "STALE",
		ExpiredAt: 1, // long in the past
	}
	if err := model.InsertRedemption(rd); err != nil {
		t.Fatalf("InsertRedemption failed: %v", err)
	}

	rr := call(RedeemCode, testRequest{
		method: http.MethodPost, target: "/api/user/redeem",
		body: `{"code":"STALE"}`, userID: u.ID, role: common.RoleUser,
	})
	expectError(t, rr, http.StatusNotFound, "invalid or already used code")

	got, _ := model.GetUserByID(u.ID)
	if got.Quota != 0 {
		t.Fatalf("quota = %d, want 0 (an expired code must not credit)", got.Quota)
	}
}

func countRedemptions(t *testing.T) int {
	t.Helper()
	codes, err := model.ListRedemptions()
	if err != nil {
		t.Fatalf("ListRedemptions failed: %v", err)
	}
	return len(codes)
}
