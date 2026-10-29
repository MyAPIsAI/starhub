package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"starhub/common"
	"starhub/middleware"
)

func TestGuardedSessionStateChecksOwnerAndKeepsCredentialsPrivate(t *testing.T) {
	newTestDB(t)
	owner := newUser(t, "device-owner", common.RoleUser, 0)
	other := newUser(t, "device-other", common.RoleRoot, 0)
	current := middleware.Sessions.Create(owner.ID, time.Hour)
	second := middleware.Sessions.Create(owner.ID, 2*time.Hour)
	foreign := middleware.Sessions.Create(other.ID, time.Hour)
	defer middleware.Sessions.Remove(current)
	defer middleware.Sessions.Remove(second)
	defer middleware.Sessions.Remove(foreign)
	middleware.Sessions.RemoveAllExcept(owner.ID, current)
	second = middleware.Sessions.Create(owner.ID, 2*time.Hour)
	defer middleware.Sessions.Remove(second)
	response := call(GetSessionState, testRequest{target: fmt.Sprintf("/api/user/session-state?expected_user_id=%d", owner.ID), userID: owner.ID, headers: map[string]string{"Authorization": "Bearer " + current}})
	var result struct {
		Schema   string                   `json:"schema"`
		Guarded  bool                     `json:"guarded_management"`
		UserID   int64                    `json:"user_id"`
		Sessions []middleware.SessionInfo `json:"sessions"`
	}
	decodeData(t, response, &result)
	if result.Schema != "session_state_v1" || !result.Guarded || result.UserID != owner.ID || len(result.Sessions) != 2 {
		t.Fatalf("invalid guarded session contract: %+v", result)
	}
	marked := 0
	for _, session := range result.Sessions {
		if session.IsCurrent {
			marked++
		}
		if session.CreatedAt <= 0 || session.ExpiresAt < session.CreatedAt {
			t.Fatal("invalid public session timing")
		}
	}
	if marked != 1 {
		t.Fatal("current calling session was not marked exactly once")
	}
	for _, secret := range []string{current, second, foreign} {
		if strings.Contains(response.Body.String(), secret) {
			t.Fatal("session state exposed a login credential")
		}
	}
	for _, query := range []string{"", "?expected_user_id=0", "?expected_user_id=01", "?expected_user_id=bad", "?expected_user_id=9223372036854775808", fmt.Sprintf("?expected_user_id=%d&expected_user_id=%d", owner.ID, owner.ID), fmt.Sprintf("?expected_user_id=%d&other=1", owner.ID)} {
		response = call(GetSessionState, testRequest{target: "/api/user/session-state" + query, userID: owner.ID})
		if response.Code != 400 {
			t.Fatalf("invalid query accepted: %s %d", query, response.Code)
		}
	}
	response = call(GetSessionState, testRequest{target: fmt.Sprintf("/api/user/session-state?expected_user_id=%d", owner.ID), userID: other.ID, role: common.RoleRoot})
	if response.Code != 409 {
		t.Fatal("Root/current cookie read another expected owner's devices")
	}
	if uid, ok := middleware.Sessions.Get(current); !ok || uid != owner.ID {
		t.Fatal("guarded reads changed login validity")
	}
}

func TestGuardedSessionTerminationRejectsStaleOwnersBeforeRevocation(t *testing.T) {
	newTestDB(t)
	owner := newUser(t, "device-terminate-owner", common.RoleUser, 0)
	other := newUser(t, "device-terminate-other", common.RoleRoot, 0)
	current := middleware.Sessions.Create(owner.ID, time.Hour)
	middleware.Sessions.RemoveAllExcept(owner.ID, current)
	second := middleware.Sessions.Create(owner.ID, time.Hour)
	foreign := middleware.Sessions.Create(other.ID, time.Hour)
	defer middleware.Sessions.Remove(current)
	defer middleware.Sessions.Remove(second)
	defer middleware.Sessions.Remove(foreign)
	for _, body := range []string{`null`, `{}`, `{"expected_user_id":0}`, `{"expected_user_id":"1"}`, fmt.Sprintf(`{"expected_user_id":%d,"all_users":true}`, owner.ID), fmt.Sprintf(`{"expected_user_id":%d} {}`, owner.ID), strings.Repeat(" ", 4097) + `{}`, fmt.Sprintf(`{"expected_user_id":%d}`, other.ID)} {
		response := call(TerminateOtherSessions, testRequest{method: http.MethodPost, target: "/api/user/sessions/terminate-others", userID: owner.ID, body: body, headers: map[string]string{"Authorization": "Bearer " + current}})
		if response.Code != 400 && response.Code != 409 {
			t.Fatalf("invalid owner/body revoked sessions: %d %s", response.Code, response.Body)
		}
		if _, ok := middleware.Sessions.Get(second); !ok {
			t.Fatal("rejected request revoked another device")
		}
	}
	for index := 0; index < 2; index++ {
		response := call(TerminateOtherSessions, testRequest{method: http.MethodPost, target: "/api/user/sessions/terminate-others", userID: owner.ID, body: fmt.Sprintf(`{"expected_user_id":%d}`, owner.ID), headers: map[string]string{"Authorization": "Bearer " + current}})
		var result map[string]json.RawMessage
		decodeData(t, response, &result)
		if string(result["user_id"]) != fmt.Sprint(owner.ID) || string(result["terminated"]) != fmt.Sprint(1-index) || len(response.Result().Cookies()) != 0 {
			t.Fatal("guarded termination did not confirm exact owner/count without a cookie")
		}
	}
	if _, ok := middleware.Sessions.Get(current); !ok {
		t.Fatal("current device was terminated")
	}
	if _, ok := middleware.Sessions.Get(second); ok {
		t.Fatal("other device remained active")
	}
	if uid, ok := middleware.Sessions.Get(foreign); !ok || uid != other.ID {
		t.Fatal("another owner's device was terminated")
	}
}
