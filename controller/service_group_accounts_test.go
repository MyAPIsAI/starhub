package controller

import (
	"testing"

	"starhub/common"
	"starhub/model"
)

func TestGroupAccountRequestsCannotPackMultipleIdentitiesInOneEntry(t *testing.T) {
	newTestDB(t)
	admin := newUser(t, "account-admin", common.RoleAdmin, 1000)
	before, err := model.OptionVersion()
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range []string{`"1,2"`, `"1\n2"`, `"1\r2"`, `"01"`, `"acct-1"`, `"999999999"`} {
		expectStatus(t, call(UpsertServiceGroup, testRequest{method: "PUT", target: "/api/service-group", userID: admin.ID, role: admin.Role, body: `{"group_id":"private","name":"Private","visibility":"custom","account_ids":[` + entry + `]}`}), 400)
	}
	groups, err := model.ListServiceGroups()
	after, versionErr := model.OptionVersion()
	if err != nil || versionErr != nil || len(groups) != 0 || before != after {
		t.Fatal("invalid account request changed configuration")
	}
}
