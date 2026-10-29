package controller

import (
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"gorm.io/gorm"
	"starhub/common"
	"starhub/middleware"
	"starhub/model"
)

func TestTokenPasswordVerificationIsOwnerBoundReadOnlyAndCookieFree(t *testing.T) {
	newTestDB(t)
	owner := newUser(t, "password-owner", common.RoleUser, 0)
	token := mustToken(t, owner.ID, 0)
	root := newUser(t, "password-root", common.RoleRoot, 0)
	beforeUser, _ := model.GetUserByID(owner.ID)
	beforeToken, _ := model.GetTokenByID(token.ID)
	beforeSessions := middleware.Sessions.ListByUser(owner.ID, "")
	for _, item := range []struct {
		userID   int64
		role     int
		expected int64
		password string
		status   int
	}{
		{owner.ID, common.RoleUser, owner.ID, "secret", 200},
		{owner.ID, common.RoleUser, owner.ID, "wrong-password", 403},
		{root.ID, common.RoleRoot, root.ID, "secret", 404},
		{root.ID, common.RoleRoot, owner.ID, "secret", 409},
	} {
		response := call(VerifyTokenPassword, testRequest{method: http.MethodPost, target: "/api/token/verify-password", userID: item.userID, role: item.role,
			path: map[string]string{"id": strconv.FormatInt(token.ID, 10)}, body: fmt.Sprintf(`{"expected_user_id":%d,"password":%q}`, item.expected, item.password)})
		if response.Code != item.status || len(response.Result().Cookies()) != 0 || !strings.Contains(response.Header().Get("Cache-Control"), "no-store") {
			t.Fatalf("verification status/cookie/cache mismatch: %d %s", response.Code, response.Body)
		}
		if response.Code == 200 {
			var result map[string]any
			decodeData(t, response, &result)
			if len(result) != 3 || result["verified"] != true || result["user_id"] != float64(owner.ID) || result["token_id"] != float64(token.ID) {
				t.Fatal("verification receipt is not bound to the original owner/token")
			}
		}
		if strings.Contains(response.Body.String(), token.Key) || strings.Contains(response.Body.String(), beforeUser.Password) || strings.Contains(response.Body.String(), "secret") {
			t.Fatal("verification exposed credential material")
		}
	}
	afterUser, _ := model.GetUserByID(owner.ID)
	afterToken, _ := model.GetTokenByID(token.ID)
	if *afterUser != *beforeUser || *afterToken != *beforeToken || !reflect.DeepEqual(middleware.Sessions.ListByUser(owner.ID, ""), beforeSessions) {
		t.Fatal("password verification mutated account, token, quota or sessions")
	}
}

func TestTokenPasswordVerificationRejectsMalformedAndUnownedInputs(t *testing.T) {
	newTestDB(t)
	owner := newUser(t, "password-invalid", common.RoleUser, 0)
	token := mustToken(t, owner.ID, 0)
	for _, body := range []string{
		`null`, `{}`, `{"expected_user_id":0,"password":"secret"}`, fmt.Sprintf(`{"expected_user_id":%d,"password":null}`, owner.ID),
		fmt.Sprintf(`{"expected_user_id":%d,"password":%q}`, owner.ID, strings.Repeat("x", 73)),
		fmt.Sprintf(`{"expected_user_id":%d,"password":"secret","username":"root"}`, owner.ID),
		fmt.Sprintf(`{"expected_user_id":%d,"password":"secret"} {}`, owner.ID), strings.Repeat(" ", 4097) + `{}`,
	} {
		response := call(VerifyTokenPassword, testRequest{method: http.MethodPost, target: "/api/token/verify-password", userID: owner.ID, role: 1, path: map[string]string{"id": strconv.FormatInt(token.ID, 10)}, body: body})
		if response.Code != 400 || len(response.Result().Cookies()) != 0 {
			t.Fatalf("unsafe verification input accepted: %d %s", response.Code, response.Body)
		}
	}
	for _, id := range []string{"0", "-1", "bad", "9223372036854775808", "9999999"} {
		response := call(VerifyTokenPassword, testRequest{method: http.MethodPost, target: "/api/token/verify-password", userID: owner.ID, role: 1, path: map[string]string{"id": id}, body: fmt.Sprintf(`{"expected_user_id":%d,"password":"secret"}`, owner.ID)})
		if response.Code != 400 && response.Code != 404 {
			t.Fatalf("invalid/missing token accepted: %d", response.Code)
		}
	}
	if err := model.DB.Model(&model.User{}).Where("id = ?", owner.ID).Update("status", common.StatusDisabled).Error; err != nil {
		t.Fatal(err)
	}
	response := call(VerifyTokenPassword, testRequest{method: http.MethodPost, target: "/api/token/verify-password", userID: owner.ID, role: 1, path: map[string]string{"id": strconv.FormatInt(token.ID, 10)}, body: fmt.Sprintf(`{"expected_user_id":%d,"password":"secret"}`, owner.ID)})
	if response.Code != 403 {
		t.Fatal("disabled account was verified")
	}
}

func TestTokenPasswordVerificationDetectsPasswordOrTokenRotationDuringVerification(t *testing.T) {
	for _, kind := range []string{"password", "token"} {
		t.Run(kind, func(t *testing.T) {
			newTestDB(t)
			owner := newUser(t, "password-race", common.RoleUser, 0)
			token := mustToken(t, owner.ID, 0)
			changed := false
			name := "synthetic-password-verification-change"
			if err := model.DB.Callback().Query().After("gorm:query").Register(name, func(tx *gorm.DB) {
				if changed || tx.Statement.Table != "users" {
					return
				}
				changed = true
				if kind == "token" {
					if err := model.UpdateTokenFields(token.ID, map[string]any{"key": common.GenerateTokenKey()}); err != nil {
						t.Error(err)
					}
				} else {
					hash, err := common.HashPassword("replacement-password")
					if err != nil {
						t.Error(err)
						return
					}
					if err := model.DB.Model(&model.User{}).Where("id = ?", owner.ID).Update("password", hash).Error; err != nil {
						t.Error(err)
					}
				}
			}); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = model.DB.Callback().Query().Remove(name) })
			response := call(VerifyTokenPassword, testRequest{method: http.MethodPost, target: "/api/token/verify-password", userID: owner.ID, role: 1, path: map[string]string{"id": strconv.FormatInt(token.ID, 10)}, body: fmt.Sprintf(`{"expected_user_id":%d,"password":"secret"}`, owner.ID)})
			if !changed || response.Code != 409 || len(response.Result().Cookies()) != 0 {
				t.Fatalf("changed credential produced an unlock: %d %s", response.Code, response.Body)
			}
		})
	}
}
