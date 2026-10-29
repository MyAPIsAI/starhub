package controller

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"starhub/common"
	"starhub/middleware"
)

func TestLogoutCurrentGuardsExpectedOwnerAndRevokesOnlyCallingSession(t *testing.T) {
	newTestDB(t)
	owner := newUser(t, "logout-current", 1, 123)
	root := newUser(t, "logout-root", common.RoleRoot, 0)
	current := middleware.Sessions.Create(owner.ID, time.Hour)
	sameOwner := middleware.Sessions.Create(owner.ID, time.Hour)
	foreign := middleware.Sessions.Create(root.ID, time.Hour)
	for _, token := range []string{current, sameOwner, foreign} {
		defer middleware.Sessions.Remove(token)
	}
	request := func(actor int64, token, body string) int {
		t.Helper()
		response := call(LogoutCurrent, testRequest{method: http.MethodPost, target: "/api/user/logout-current", userID: actor, role: common.RoleRoot, body: body, headers: map[string]string{"Authorization": "Bearer " + token}})
		if len(response.Result().Cookies()) != 0 || !strings.Contains(response.Header().Get("Cache-Control"), "no-store") {
			t.Fatal("current logout changed browser cookies or allowed caching")
		}
		return response.Code
	}
	base := fmt.Sprintf(`"schema":"logout_current_v1","expected_user_id":%d`, owner.ID)
	for _, body := range []string{``, `null`, `{}`, `{"schema":"logout_current_v0","expected_user_id":1}`, `{` + base + `,"all_devices":true}`, `{` + base + `} {}`, strings.Repeat(" ", 4097) + `{}`} {
		if status := request(owner.ID, current, body); status != 400 {
			t.Fatalf("invalid logout body accepted: %d", status)
		}
		if _, ok := middleware.Sessions.Get(current); !ok {
			t.Fatal("invalid request revoked calling session")
		}
	}
	if status := request(root.ID, foreign, `{`+base+`}`); status != 409 {
		t.Fatal("Root bypassed expected logout owner")
	}
	response := call(LogoutCurrent, testRequest{method: http.MethodPost, target: "/api/user/logout-current", userID: owner.ID, body: `{` + base + `}`, headers: map[string]string{"Authorization": "Bearer " + current}})
	var receipt struct {
		Schema    string `json:"schema"`
		Owner     int64  `json:"user_id"`
		SignedOut bool   `json:"signed_out"`
	}
	decodeData(t, response, &receipt)
	if receipt.Schema != "logout_receipt_v1" || receipt.Owner != owner.ID || !receipt.SignedOut || len(response.Result().Cookies()) != 0 {
		t.Fatal("logout receipt was not exact or cleared a cookie")
	}
	if _, ok := middleware.Sessions.Get(current); ok {
		t.Fatal("calling session stayed active")
	}
	for _, token := range []string{sameOwner, foreign} {
		if _, ok := middleware.Sessions.Get(token); !ok {
			t.Fatal("current logout revoked another device/account")
		}
	}
}
