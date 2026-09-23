package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"starhub/common"
	"starhub/middleware"
	"starhub/model"
	"starhub/setting"
)

func TestRegisterValidation(t *testing.T) {
	newTestDB(t)

	cases := []struct {
		name string
		body string
		want string
	}{
		{"short username", `{"username":"ab","password":"secret1"}`, "username must be 3-64"},
		{"long username", fmt.Sprintf(`{"username":%q,"password":"secret1"}`, repeat("a", 65)), "username must be 3-64"},
		{"illegal username chars", `{"username":"bad name!","password":"secret1"}`, "username must be 3-64"},
		{"short password", `{"username":"alice","password":"12345"}`, "password must be 6-64"},
		{"long password", fmt.Sprintf(`{"username":"alice","password":%q}`, repeat("p", 65)), "password must be 6-64"},
		{"bad email", `{"username":"alice","password":"secret1","email":"not-an-email"}`, "invalid email"},
		{"email missing domain", `{"username":"alice","password":"secret1","email":"a@"}`, "invalid email"},
	}
	for _, tc := range cases {
		rr := call(Register, testRequest{
			method: http.MethodPost,
			target: "/api/user/register",
			body:   tc.body,
		})
		expectError(t, rr, http.StatusBadRequest, tc.want)
	}

	// Whitespace around the username is trimmed before validation, so this
	// registers successfully.
	rr := call(Register, testRequest{
		method: http.MethodPost,
		target: "/api/user/register",
		body:   `{"username":"  alice  ","password":"secret1"}`,
	})
	var got map[string]any
	decodeData(t, rr, &got)
	if got["username"] != "alice" {
		t.Fatalf("username = %v, want the trimmed alice", got["username"])
	}
	if got["role"].(float64) != float64(common.RoleUser) {
		t.Fatalf("role = %v, want RoleUser (registration must not mint admins)", got["role"])
	}
	if got["quota"].(float64) != float64(common.DefaultRegisterQuota) {
		t.Fatalf("quota = %v, want %d", got["quota"], common.DefaultRegisterQuota)
	}
	if got["aff_code"] == "" {
		t.Fatal("a fresh user should get an affiliate code")
	}
	// The password must never be echoed back.
	if _, present := got["password"]; present {
		t.Fatal("the password field leaked into the response")
	}
}

func TestRegisterDuplicate(t *testing.T) {
	newTestDB(t)
	newUser(t, "alice", common.RoleUser, 0)

	rr := call(Register, testRequest{
		method: http.MethodPost,
		target: "/api/user/register",
		body:   `{"username":"alice","password":"secret1"}`,
	})
	expectError(t, rr, http.StatusConflict, "username already exists")
}

// A valid invite code links the new user to the inviter and credits the
// one-time register bonus; an unusable code must not block registration.
func TestRegisterWithAffCode(t *testing.T) {
	newTestDB(t)
	inviter := newUser(t, "inviter", common.RoleUser, 100)

	// Give the inviter a functioning code and switch the bonus on.
	if err := setting.UpdateOption(setting.OptionKeyAffRegisterBonus, "250"); err != nil {
		t.Fatalf("UpdateOption(bonus) failed: %v", err)
	}
	inviter, err := model.GetUserByID(inviter.ID)
	if err != nil {
		t.Fatalf("GetUserByID failed: %v", err)
	}

	rr := call(Register, testRequest{
		method: http.MethodPost,
		target: "/api/user/register",
		body:   fmt.Sprintf(`{"username":"invitee","password":"secret1","aff_code":%q}`, inviter.AffCode),
	})
	var got map[string]any
	decodeData(t, rr, &got)
	if got["inviter_id"].(float64) != float64(inviter.ID) {
		t.Fatalf("inviter_id = %v, want %d", got["inviter_id"], inviter.ID)
	}

	updated, err := model.GetUserByID(inviter.ID)
	if err != nil {
		t.Fatalf("GetUserByID failed: %v", err)
	}
	if updated.Quota != 350 {
		t.Fatalf("inviter quota = %d, want 100 + 250 bonus", updated.Quota)
	}

	// An unknown code is silently ignored: the registration still succeeds
	// with no inviter attached.
	rr = call(Register, testRequest{
		method: http.MethodPost,
		target: "/api/user/register",
		body:   `{"username":"loner","password":"secret1","aff_code":"no-such-code"}`,
	})
	var loner map[string]any
	decodeData(t, rr, &loner)
	if loner["inviter_id"].(float64) != 0 {
		t.Fatalf("inviter_id = %v, want 0 for an unknown code", loner["inviter_id"])
	}
}

// A disabled inviter must not accrue quota from new registrations.
func TestRegisterIgnoresDisabledInviter(t *testing.T) {
	newTestDB(t)
	inviter := newUser(t, "inviter", common.RoleUser, 100)
	if err := setting.UpdateOption(setting.OptionKeyAffRegisterBonus, "250"); err != nil {
		t.Fatalf("UpdateOption(bonus) failed: %v", err)
	}
	inviter, _ = model.GetUserByID(inviter.ID)
	if err := model.UpdateUserFields(inviter.ID, map[string]any{"status": common.StatusDisabled}); err != nil {
		t.Fatalf("UpdateUserFields failed: %v", err)
	}

	rr := call(Register, testRequest{
		method: http.MethodPost,
		target: "/api/user/register",
		body:   fmt.Sprintf(`{"username":"invitee","password":"secret1","aff_code":%q}`, inviter.AffCode),
	})
	var got map[string]any
	decodeData(t, rr, &got)
	if got["inviter_id"].(float64) != 0 {
		t.Fatalf("a disabled user was recorded as inviter: %v", got["inviter_id"])
	}
	updated, _ := model.GetUserByID(inviter.ID)
	if updated.Quota != 100 {
		t.Fatalf("disabled inviter was credited: quota = %d", updated.Quota)
	}
}

func TestRegisterRejectsInvalidBody(t *testing.T) {
	newTestDB(t)
	rr := call(Register, testRequest{
		method: http.MethodPost,
		target: "/api/user/register",
		body:   `{`,
	})
	expectError(t, rr, http.StatusBadRequest, "invalid request body")
}

func TestLogin(t *testing.T) {
	newTestDB(t)
	u := newUser(t, "alice", common.RoleUser, 500)

	rr := call(Login, testRequest{
		method: http.MethodPost,
		target: "/api/user/login",
		body:   `{"username":"alice","password":"wrong"}`,
	})
	expectError(t, rr, http.StatusUnauthorized, "invalid username or password")

	rr = call(Login, testRequest{
		method: http.MethodPost,
		target: "/api/user/login",
		body:   `{"username":"ghost","password":"secret"}`,
	})
	expectError(t, rr, http.StatusUnauthorized, "invalid username or password")

	var got map[string]any
	rr = call(Login, testRequest{
		method: http.MethodPost,
		target: "/api/user/login",
		body:   `{"username":"alice","password":"secret"}`,
	})
	decodeData(t, rr, &got)
	session, _ := got["session"].(string)
	if session == "" {
		t.Fatal("login did not return a session token")
	}
	// The session must resolve back to the user through the shared store.
	uid, ok := middleware.Sessions.Get(session)
	if !ok || uid != u.ID {
		t.Fatalf("Sessions.Get(%q) = %d, %v; want %d, true", session, uid, ok, u.ID)
	}

	// The cookie is HttpOnly: an XSS must not be able to read it.
	cookies := rr.Result().Cookies()
	var found bool
	for _, c := range cookies {
		if c.Name == "session" {
			found = true
			if !c.HttpOnly {
				t.Fatal("session cookie is not HttpOnly")
			}
			if c.Path != "/" {
				t.Fatalf("session cookie path = %q, want /", c.Path)
			}
		}
	}
	if !found {
		t.Fatal("no session cookie was set on login")
	}
}

func TestLoginRejectsDisabledUser(t *testing.T) {
	newTestDB(t)
	u := newUser(t, "alice", common.RoleUser, 500)
	if err := model.UpdateUserFields(u.ID, map[string]any{"status": common.StatusDisabled}); err != nil {
		t.Fatalf("UpdateUserFields failed: %v", err)
	}

	rr := call(Login, testRequest{
		method: http.MethodPost,
		target: "/api/user/login",
		body:   `{"username":"alice","password":"secret"}`,
	})
	expectError(t, rr, http.StatusForbidden, "user is disabled")
}

func TestLogoutRemovesSession(t *testing.T) {
	newTestDB(t)
	u := newUser(t, "alice", common.RoleUser, 500)
	session := middleware.Sessions.Create(u.ID, sessionTTLForTest)

	rr := call(Logout, testRequest{
		method: http.MethodPost,
		target: "/api/user/logout",
		userID: u.ID,
		role:   common.RoleUser,
		headers: map[string]string{
			"Authorization": "Bearer " + session,
		},
	})
	expectStatus(t, rr, http.StatusOK)

	if _, ok := middleware.Sessions.Get(session); ok {
		t.Fatal("session survived logout")
	}
}

// A second call with the same session must be rejected: the guard is the
// shared session store, not the request context.
func TestUserAuthRejectsAfterLogout(t *testing.T) {
	newTestDB(t)
	u := newUser(t, "alice", common.RoleUser, 500)
	session := middleware.Sessions.Create(u.ID, sessionTTLForTest)

	protected := middleware.UserAuth(http.HandlerFunc(GetSelf))
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/user/self", nil)
	req.Header.Set("Authorization", "Bearer "+session)
	protected.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("authenticated request status = %d, want 200 (%s)", rr.Code, rr.Body.String())
	}

	logout := middleware.UserAuth(http.HandlerFunc(Logout))
	logoutReq := httptest.NewRequest(http.MethodPost, "/api/user/logout", nil)
	logoutReq.Header.Set("Authorization", "Bearer "+session)
	logoutResp := httptest.NewRecorder()
	logout.ServeHTTP(logoutResp, logoutReq)
	if logoutResp.Code != http.StatusOK {
		t.Fatalf("logout status = %d, want 200 (%s)", logoutResp.Code, logoutResp.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/user/self", nil)
	req.Header.Set("Authorization", "Bearer "+session)
	protected.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("post-logout status = %d, want 401 (%s)", rr.Code, rr.Body.String())
	}
}

func TestGetSelfIncludesRequestCount(t *testing.T) {
	newTestDB(t)
	u := newUser(t, "alice", common.RoleUser, 500)

	for i := 0; i < 3; i++ {
		if err := model.InsertLog(&model.Log{UserID: u.ID, Model: "gpt-4o", Quota: 1}); err != nil {
			t.Fatalf("InsertLog failed: %v", err)
		}
	}
	// Another user's logs must not be counted.
	other := newUser(t, "bob", common.RoleUser, 0)
	if err := model.InsertLog(&model.Log{UserID: other.ID, Model: "gpt-4o", Quota: 1}); err != nil {
		t.Fatalf("InsertLog failed: %v", err)
	}

	rr := call(GetSelf, testRequest{
		target: "/api/user/self",
		userID: u.ID,
		role:   common.RoleUser,
	})
	var got map[string]any
	decodeData(t, rr, &got)
	if got["request_count"].(float64) != 3 {
		t.Fatalf("request_count = %v, want 3", got["request_count"])
	}
	if _, present := got["password"]; present {
		t.Fatal("password leaked into GetSelf")
	}
}

func TestGetSelfNotFound(t *testing.T) {
	newTestDB(t)
	rr := call(GetSelf, testRequest{
		target: "/api/user/self",
		userID: 9999,
		role:   common.RoleUser,
	})
	expectError(t, rr, http.StatusNotFound, "user not found")
}

func TestUserUsageDaysClamping(t *testing.T) {
	newTestDB(t)
	u := newUser(t, "alice", common.RoleUser, 0)

	cases := []struct {
		query string
		want  float64
	}{
		{"", 7},
		{"?days=0", 7},
		{"?days=-3", 7},
		{"?days=30", 30},
		{"?days=1000", 90},
		{"?days=abc", 7},
	}
	for _, tc := range cases {
		rr := call(UserUsage, testRequest{
			target: "/api/user/usage" + tc.query,
			userID: u.ID,
			role:   common.RoleUser,
		})
		var got map[string]any
		decodeData(t, rr, &got)
		if got["days"].(float64) != tc.want {
			t.Fatalf("days(%q) = %v, want %v", tc.query, got["days"], tc.want)
		}
	}
}

// Only an admin may pivot the usage report to another user; a regular user's
// user_id parameter is ignored so they cannot read someone else's spend.
func TestUserUsageUserIDRequiresAdmin(t *testing.T) {
	newTestDB(t)
	alice := newUser(t, "alice", common.RoleUser, 0)
	bob := newUser(t, "bob", common.RoleUser, 0)

	if err := model.InsertLog(&model.Log{UserID: bob.ID, Model: "gpt-4o", Quota: 77}); err != nil {
		t.Fatalf("InsertLog failed: %v", err)
	}

	rr := call(UserUsage, testRequest{
		target: "/api/user/usage?user_id=" + itoa(bob.ID),
		userID: alice.ID,
		role:   common.RoleUser,
	})
	var got map[string]any
	decodeData(t, rr, &got)
	items := got["items"].([]any)
	if len(items) != 0 {
		t.Fatalf("a regular user read another user's usage: %v", items)
	}

	// The admin can, and sees bob's spend.
	adminUID, adminRole := admin()
	rr = call(UserUsage, testRequest{
		target: "/api/user/usage?user_id=" + itoa(bob.ID),
		userID: adminUID,
		role:   adminRole,
	})
	decodeData(t, rr, &got)
	items = got["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("admin items = %d, want 1", len(items))
	}
	entry := items[0].(map[string]any)
	if entry["quota"].(float64) != 77 {
		t.Fatalf("quota = %v, want 77", entry["quota"])
	}
}

func TestUpdateUserValidation(t *testing.T) {
	newTestDB(t)
	u := newUser(t, "alice", common.RoleUser, 100)
	id := itoa(u.ID)

	cases := []struct {
		name string
		body string
		want string
	}{
		{"missing id", `{"quota":1}`, "user id is required"},
		{"unknown user", `{"id":9999,"quota":1}`, "user not found"},
		{"negative quota", fmt.Sprintf(`{"id":%s,"quota":-1}`, id), "quota must be >= 0"},
		{"bad status", fmt.Sprintf(`{"id":%s,"status":7}`, id), "invalid status"},
		{"unknown group", fmt.Sprintf(`{"id":%s,"group":"ghost"}`, id), "group has no ratio configured"},
		{"short password", fmt.Sprintf(`{"id":%s,"password":"12345"}`, id), "password must be at least 6"},
		{"nothing to update", fmt.Sprintf(`{"id":%s}`, id), "nothing to update"},
	}
	for _, tc := range cases {
		rr := call(UpdateUser, testRequest{
			method: http.MethodPut,
			target: "/api/user",
			body:   tc.body,
			userID: 1,
			role:   common.RoleAdmin,
		})
		status := http.StatusBadRequest
		if tc.name == "unknown user" {
			status = http.StatusNotFound
		}
		expectError(t, rr, status, tc.want)
	}
}

func TestUpdateUserAppliesFields(t *testing.T) {
	newTestDB(t)
	u := newUser(t, "alice", common.RoleUser, 100)

	rr := call(UpdateUser, testRequest{
		method: http.MethodPut,
		target: "/api/user",
		body: fmt.Sprintf(`{"id":%s,"quota":500,"expected_quota":100,"status":%d,"group":%q,"password":"newpassword"}`,
			itoa(u.ID), common.StatusDisabled, common.DefaultGroup),
		userID: 1,
		role:   common.RoleAdmin,
	})
	expectStatus(t, rr, http.StatusOK)

	got, err := model.GetUserByID(u.ID)
	if err != nil {
		t.Fatalf("GetUserByID failed: %v", err)
	}
	if got.Quota != 500 {
		t.Fatalf("quota = %d, want 500", got.Quota)
	}
	if got.Status != common.StatusDisabled {
		t.Fatalf("status = %d, want disabled", got.Status)
	}
	// The new password must be stored hashed and verifiable.
	if got.Password == "newpassword" {
		t.Fatal("the password was stored in plaintext")
	}
	if !common.VerifyPassword("newpassword", got.Password) {
		t.Fatal("the stored hash does not verify against the new password")
	}
	if common.VerifyPassword("secret", got.Password) {
		t.Fatal("the old password still verifies")
	}
}

func TestListUsersHidesPasswords(t *testing.T) {
	newTestDB(t)
	newUser(t, "alice", common.RoleUser, 100)
	newUser(t, "bob", common.RoleUser, 100)

	rr := call(ListUsers, testRequest{target: "/api/user", userID: 1, role: common.RoleAdmin})
	var got []map[string]any
	decodeData(t, rr, &got)

	// newTestDB seeds the root account, so expect it alongside the two users
	// created here.
	names := map[string]bool{}
	for _, item := range got {
		names[item["username"].(string)] = true
	}
	for _, want := range []string{common.DefaultRootUser, "alice", "bob"} {
		if !names[want] {
			t.Fatalf("user %q missing from the listing: %v", want, names)
		}
	}
	for _, item := range got {
		if _, present := item["password"]; present {
			t.Fatalf("password leaked for user %v", item["username"])
		}
		// publicUser always exposes these keys.
		for _, key := range []string{"id", "username", "role", "status", "quota", "group", "aff_code"} {
			if _, present := item[key]; !present {
				t.Fatalf("publicUser is missing %q: %v", key, item)
			}
		}
	}
}

func TestValidUsernameAndEmailHelpers(t *testing.T) {
	valid := []string{"abc", "a-b_c.d", "User123", repeat("x", 64)}
	for _, name := range valid {
		if !validUsername(name) {
			t.Fatalf("validUsername(%q) = false, want true", name)
		}
	}
	invalid := []string{"", "ab", repeat("x", 65), "has space", "has/slash", "中文"}
	for _, name := range invalid {
		if validUsername(name) {
			t.Fatalf("validUsername(%q) = true, want false", name)
		}
	}

	if !validEmail("") {
		t.Fatal("an empty email should be allowed")
	}
	// validEmail requires 5-64 characters with an '@' somewhere in the middle,
	// so "a@b" is too short to be valid.
	for _, email := range []string{"a@bcd", "alice@example.com"} {
		if !validEmail(email) {
			t.Fatalf("validEmail(%q) = false, want true", email)
		}
	}
	for _, email := range []string{"a@b", "@bcd", "no-at-sign", repeat("x", 64) + "@b"} {
		if validEmail(email) {
			t.Fatalf("validEmail(%q) = true, want false", email)
		}
	}
}
