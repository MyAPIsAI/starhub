package controller

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"starhub/common"
	"starhub/model"
)

func TestDeactivationStateIsReadOnlyVersionedAndOwned(t *testing.T) {
	newTestDB(t)
	owner := newUser(t, "deactivation-preview", 1, 123)
	root := newUser(t, "deactivation-preview-root", common.RoleRoot, 0)
	if err := model.DB.Create(&model.Wallet{UserID: owner.ID, Balance: 9007199254740993}).Error; err != nil {
		t.Fatal(err)
	}
	path := fmt.Sprintf("/api/user/deactivation-state?expected_user_id=%d", owner.ID)
	response := call(GetDeactivationState, testRequest{target: path, userID: owner.ID})
	var got map[string]json.RawMessage
	decodeData(t, response, &got)
	if string(got["schema"]) != `"deactivation_state_v1"` || string(got["policy"]) != `"dust_below_cent_v1"` || string(got["guarded_deactivation"]) != "true" || string(got["wallet_balance_nano"]) != `"9007199254740993"` || string(got["dust_limit_nano"]) != `"10000000"` {
		t.Fatal("invalid exact readonly preview contract")
	}
	if strings.Contains(response.Body.String(), owner.Password) || strings.Contains(response.Body.String(), "password") || len(response.Result().Cookies()) != 0 {
		t.Fatal("preview exposed a credential or changed a Cookie")
	}
	if rr := call(GetDeactivationState, testRequest{target: path, userID: root.ID, role: common.RoleRoot}); rr.Code != 409 {
		t.Fatal("Root bypassed expected preview owner")
	}
	for _, query := range []string{"", "?expected_user_id=01", "?expected_user_id=-1", "?expected_user_id=1&expected_user_id=1", "?expected_user_id=1&other=1"} {
		if rr := call(GetDeactivationState, testRequest{target: "/api/user/deactivation-state" + query, userID: owner.ID}); rr.Code != 400 {
			t.Fatal("invalid expected owner query accepted")
		}
	}
	if err := model.DB.Migrator().DropTable(&model.Wallet{}); err != nil {
		t.Fatal(err)
	}
	if rr := call(GetDeactivationState, testRequest{target: path, userID: owner.ID}); rr.Code != 500 || strings.Contains(rr.Body.String(), "\"data\"") {
		t.Fatal("failed wallet read became zero or partial preview")
	}
}
