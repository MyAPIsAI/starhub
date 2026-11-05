package controller

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"starhub/common"
	"starhub/model"
)

func TestAuditSearchScopePaginationAndDisplayNames(t *testing.T) {
	newTestDB(t)
	customer := newUser(t, "audit-customer", common.RoleUser, 0)
	token := mustToken(t, customer.ID, 0)
	if err := model.DB.Model(token).Update("name", "audit-key-name").Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now().Unix()
	entries := []model.AuditLog{
		{UserID: 10, Username: "operator", Action: "staff_2fa_verified", ObjectType: "staff", ObjectID: "999", CreatedAt: now - 20},
		{UserID: 10, Username: "operator", Action: "customer_disabled", ObjectType: "customers", ObjectID: itoa(customer.ID), Reason: "literal 50%_discount", CreatedAt: now - 10},
		{UserID: 20, Username: "other-operator", Action: "staff_2fa_verified", ObjectType: "staff", ObjectID: "998", CreatedAt: now - 5},
		{UserID: 0, Username: "deployment-cli", Action: "staff_root_initialized", CreatedAt: now - 1},
		{UserID: 10, Username: "operator", Action: "customer_key_disabled", ObjectType: "keys", ObjectID: itoa(token.ID), CreatedAt: now},
		{UserID: 10, Username: "operator", Action: "staff_2fa_verified", Reason: "expired-record", CreatedAt: time.Now().AddDate(-2, 0, 0).Unix()},
	}
	if err := model.DB.Create(&entries).Error; err != nil {
		t.Fatal(err)
	}
	type result struct {
		Items  []auditLogRow `json:"items"`
		Total  int64         `json:"total"`
		Actors []struct {
			ID int64 `json:"id"`
		} `json:"actors"`
	}
	read := func(query string, role int) result {
		t.Helper()
		response := call(ListAuditLogs, testRequest{target: "/api/audit?" + query, role: role, userID: 10})
		var data result
		decodeData(t, response, &data)
		if strings.Contains(response.Body.String(), token.Key) || strings.Contains(response.Body.String(), customer.Password) || strings.Contains(response.Body.String(), "key_hash") {
			t.Fatal("display-name lookup exposed credentials")
		}
		return data
	}
	translated := "q=" + url.QueryEscape("登录后台") + "&search_action=staff_2fa_verified"
	root := read(translated+"&page_size=1&p=2", common.RoleRoot)
	if root.Total != 2 || len(root.Items) != 1 || root.Items[0].ID != entries[0].ID {
		t.Fatalf("search pagination: %+v", root)
	}
	own := read(translated+"&actor=20", common.RoleFinance)
	if own.Total != 1 || own.Items[0].UserID != 10 || len(own.Actors) != 1 || own.Actors[0].ID != 10 {
		t.Fatalf("search bypassed actor scope: %+v", own)
	}
	combined := read(translated+"&actor=10&action=customer_disabled", common.RoleRoot)
	if combined.Total != 0 {
		t.Fatal("action filter was bypassed")
	}
	if read(translated+"&until="+itoa(now-10), common.RoleRoot).Total != 1 {
		t.Fatal("date filter was bypassed")
	}
	if read("q=expired-record", common.RoleRoot).Total != 0 {
		t.Fatal("retention was bypassed")
	}
	for _, test := range []struct {
		query, name string
		index       int
	}{
		{"q=audit-customer", "audit-customer", 1},
		{"q=audit-key-name", "audit-key-name", 4},
		{"q=" + url.QueryEscape("%_"), "audit-customer", 1},
		{"q=" + url.QueryEscape("50%_discount"), "audit-customer", 1},
	} {
		data := read(test.query, common.RoleRoot)
		if data.Total != 1 || data.Items[0].ID != entries[test.index].ID || data.Items[0].ObjectName != test.name {
			t.Fatalf("literal/name search %s: %+v", test.query, data)
		}
	}
	if read("q="+url.QueryEscape("' OR 1=1 --"), common.RoleRoot).Total != 0 {
		t.Fatal("unsafe query interpolation")
	}
	deployment := read("actor=0", common.RoleRoot)
	if deployment.Total != 1 || deployment.Items[0].UserID != 0 || len(deployment.Actors) != 3 {
		t.Fatal("deployment actor or stable actor choices missing")
	}
	for _, query := range []string{"actor=-1", "from=invalid", "q=" + url.QueryEscape(strings.Repeat("中", 201)), "search_action=" + strings.Repeat("x", 65)} {
		response := call(ListAuditLogs, testRequest{target: "/api/audit?" + query, role: common.RoleRoot, userID: 10})
		expectStatus(t, response, 400)
	}
}
