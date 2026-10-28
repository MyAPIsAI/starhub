package controller

import (
	"fmt"
	"net/http/httptest"
	"testing"

	"starhub/common"
	"starhub/model"
	"starhub/setting"
)

func TestTokenGroupEditsUseOwnerScopeAndAllowRevokedKeyDisable(t *testing.T) {
	newTestDB(t)
	owner := newUser(t, "scoped-owner", common.RoleUser, 1000)
	admin := newUser(t, "scoped-admin", common.RoleRoot, 1000)
	if err := setting.UpdateRatio("group_ratio", `{"default":1,"private":0.5,"disabled":0.3}`); err != nil {
		t.Fatal(err)
	}
	private := &model.ServiceGroupConfig{GroupID: "private", Enabled: true, Visibility: "custom", AccountIDs: itoa(admin.ID)}
	for _, group := range []*model.ServiceGroupConfig{private, {GroupID: "disabled", Enabled: false, Visibility: "common"}} {
		if err := model.UpsertServiceGroup(group); err != nil {
			t.Fatal(err)
		}
	}
	for _, body := range []string{`{"group":"private"}`, `{"group":"disabled"}`, `{"group":"auto","auto_groups":"default,private"}`} {
		expectError(t, call(CreateToken, testRequest{method: "POST", target: "/api/token", body: body, userID: owner.ID, role: owner.Role}), 403, "cannot access")
	}
	if err := model.UpdateUserFields(owner.ID, map[string]any{"group": "private"}); err != nil {
		t.Fatal(err)
	}
	expectError(t, call(CreateToken, testRequest{method: "POST", target: "/api/token", body: `{}`, userID: owner.ID, role: owner.Role}), 403, "cannot access")
	key := mustToken(t, owner.ID, 1000)
	update := func(body string) *httptest.ResponseRecorder {
		return call(UpdateToken, testRequest{method: "PUT", target: "/api/token", body: body, userID: admin.ID, role: admin.Role})
	}
	expectError(t, update(fmt.Sprintf(`{"id":%d,"group":"private"}`, key.ID)), 403, "cannot access")
	stored, err := model.GetTokenByID(key.ID)
	if err != nil || stored.Group != "" {
		t.Fatal("denied group update changed the key")
	}
	private.AccountIDs = itoa(owner.ID)
	if err := model.UpsertServiceGroup(private); err != nil {
		t.Fatal(err)
	}
	expectStatus(t, update(fmt.Sprintf(`{"id":%d,"group":"private"}`, key.ID)), 200)
	private.Enabled = false
	if err := model.UpsertServiceGroup(private); err != nil {
		t.Fatal(err)
	}
	expectStatus(t, update(fmt.Sprintf(`{"id":%d,"status":%d}`, key.ID, common.StatusDisabled)), 200)
	expectError(t, update(fmt.Sprintf(`{"id":%d,"auto_groups":"private"}`, key.ID)), 403, "cannot access")
	stored, err = model.GetTokenByID(key.ID)
	if err != nil || stored.Status != common.StatusDisabled || stored.AutoGroups != "" || stored.RemainQuota != 1000 {
		t.Fatalf("revoked key could not be disabled safely: %+v %v", stored, err)
	}
	if err := model.DB.Migrator().DropTable(&model.ServiceGroupConfig{}); err != nil {
		t.Fatal(err)
	}
	expectError(t, update(fmt.Sprintf(`{"id":%d,"group":"default"}`, key.ID)), 500, "could not be verified")
}
