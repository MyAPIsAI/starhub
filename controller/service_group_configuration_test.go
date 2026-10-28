package controller

import (
	"net/http"
	"reflect"
	"testing"

	"starhub/common"
	"starhub/model"
	"starhub/setting"
)

func TestGroupConfigurationHandlerRollsBackStorageFailures(t *testing.T) {
	newTestDB(t)
	admin := newUser(t, "atomic-admin", common.RoleAdmin, 1000)
	request := testRequest{method: "PUT", target: "/api/service-group", userID: admin.ID, role: common.RoleAdmin, body: `{"group_id":"managed","name":"Original","multiplier":"0.8","enabled":true}`}
	expectStatus(t, call(UpsertServiceGroup, request), 200)
	before, err := model.GetServiceGroup("managed")
	if err != nil {
		t.Fatal(err)
	}
	version, err := model.OptionVersion()
	if err != nil {
		t.Fatal(err)
	}
	options, err := model.AllOptions()
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := setting.SnapshotRatio(setting.OptionKeyGroupRatio)
	if err != nil {
		t.Fatal(err)
	}
	if err := model.DB.Exec(`CREATE TRIGGER reject_group_option BEFORE INSERT ON option_changes WHEN NEW.key = 'group_ratio' BEGIN SELECT RAISE(ABORT, 'synthetic failure'); END`).Error; err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{`{"group_id":"fresh","name":"New","multiplier":"0.6"}`, `{"group_id":"managed","name":"Changed","multiplier":"0.6"}`} {
		request.body = body
		expectStatus(t, call(UpsertServiceGroup, request), 500)
	}
	expectStatus(t, call(DeleteServiceGroup, testRequest{method: http.MethodDelete, target: "/api/service-group/managed", path: map[string]string{"id": "managed"}, userID: admin.ID, role: common.RoleAdmin}), 500)
	after, err := model.GetServiceGroup("managed")
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("failed API changed metadata")
	}
	groups, err := model.ListServiceGroups()
	if err != nil || len(groups) != 1 {
		t.Fatal("failed create left a group")
	}
	afterOptions, err := model.AllOptions()
	if err != nil || !reflect.DeepEqual(options, afterOptions) {
		t.Fatal("failed API changed stored options")
	}
	afterVersion, err := model.OptionVersion()
	if err != nil || afterVersion != version {
		t.Fatal("failed API changed version")
	}
	afterRuntime, err := setting.SnapshotRatio(setting.OptionKeyGroupRatio)
	if err != nil || runtime != afterRuntime {
		t.Fatal("failed API changed serving ratios")
	}
}

func TestGroupConfigurationReportsCommittedButUnloadedChanges(t *testing.T) {
	newTestDB(t)
	admin := newUser(t, "reload-admin", common.RoleAdmin, 1000)
	request := testRequest{method: "PUT", target: "/api/service-group", userID: admin.ID, role: common.RoleAdmin, body: `{"group_id":"managed","name":"Original","multiplier":"0.8"}`}
	expectStatus(t, call(UpsertServiceGroup, request), 200)
	if err := model.UpsertOption(setting.OptionKeyCacheRatio, `not-json`); err != nil {
		t.Fatal(err)
	}
	request.body = `{"group_id":"managed","name":"Saved","multiplier":"0.6"}`
	expectError(t, call(UpsertServiceGroup, request), 500, "saved, but serving settings")
	stored, err := model.GetServiceGroup("managed")
	if err != nil || stored.Name != "Saved" || stored.Multiplier != "0.6" {
		t.Fatal("committed metadata not retained")
	}
	value, present := setting.GroupRatioValue("managed")
	if !present || value != 0.8 {
		t.Fatal("failed reload changed serving prices")
	}
	changes, err := model.ListOptionChanges(100)
	if err != nil || changes[0].ActorID != admin.ID {
		t.Fatal("actual option actor lost")
	}
	expectError(t, call(DeleteServiceGroup, testRequest{method: "DELETE", target: "/api/service-group/managed", path: map[string]string{"id": "managed"}, userID: admin.ID, role: common.RoleAdmin}), 500, "deleted, but serving settings")
	if _, err := model.GetServiceGroup("managed"); err != model.ErrServiceGroupNotFound {
		t.Fatal("committed deletion not retained")
	}
	if value, present := setting.GroupRatioValue("managed"); !present || value != 0.8 {
		t.Fatal("failed deletion reload changed serving prices")
	}
}
