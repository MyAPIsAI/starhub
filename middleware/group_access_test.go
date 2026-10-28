package middleware

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"starhub/common"
	"starhub/model"
	"starhub/setting"
)

func TestTokenAuthRechecksGroupAccessAndFiltersAutoSequence(t *testing.T) {
	setupAutoRoutingDB(t)
	owner := &model.User{Username: "group-owner", Role: common.RoleRoot, Status: common.StatusEnabled, Group: "private", Quota: 12345}
	if err := model.CreateUser(owner); err != nil {
		t.Fatal(err)
	}
	token := &model.Token{UserID: owner.ID, Status: common.StatusEnabled, UnlimitedQuota: true, ExpiredTime: -1}
	if err := model.InsertToken(token); err != nil {
		t.Fatal(err)
	}
	if err := setting.UpdateRatio("group_ratio", `{"private":0.5,"disabled":0.1,"legacy":0,"common":1}`); err != nil {
		t.Fatal(err)
	}
	private := &model.ServiceGroupConfig{GroupID: "private", Enabled: true, Visibility: "custom", AccountIDs: strconv.FormatInt(owner.ID, 10)}
	for _, group := range []*model.ServiceGroupConfig{private, {GroupID: "disabled", Enabled: false, Visibility: "common"}, {GroupID: "common", Enabled: true, Visibility: "common"}} {
		if err := model.UpsertServiceGroup(group); err != nil {
			t.Fatal(err)
		}
	}
	call := func(path string, want int, wantGroups []string) {
		t.Helper()
		reached := false
		handler := TokenAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			reached = true
			groups, _ := r.Context().Value(common.CtxAutoGroups).([]string)
			if !reflect.DeepEqual(groups, wantGroups) {
				t.Fatalf("authorized sequence %v, want %v", groups, wantGroups)
			}
			w.WriteHeader(http.StatusNoContent)
		}))
		r := httptest.NewRequest("POST", path, strings.NewReader(`{}`))
		r.Header.Set("Authorization", "Bearer "+token.Key)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, r)
		if rec.Code != want || reached != (want == http.StatusNoContent) {
			t.Fatalf("%s: %d %s, reached=%v", path, rec.Code, rec.Body, reached)
		}
		if want == http.StatusInternalServerError && strings.Contains(rec.Body.String(), "service_group_configs") {
			t.Fatal("group storage details leaked")
		}
	}
	for _, path := range []string{"/v1/chat/completions", "/v1/messages", "/v1/responses", "/v1beta/models/gpt-4o:generateContent"} {
		call(path, 204, nil)
	}
	private.AccountIDs = "acct-1,01,+1"
	if err := model.UpsertServiceGroup(private); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/v1/chat/completions", "/v1/messages", "/v1/responses", "/v1beta/models/gpt-4o:generateContent"} {
		call(path, 403, nil)
	}
	if err := model.UpdateTokenFields(token.ID, map[string]any{"group": common.AutoGroup, "auto_groups": "private,disabled,missing,common,legacy"}); err != nil {
		t.Fatal(err)
	}
	call("/v1/chat/completions", 204, []string{"common", "legacy"})
	if err := model.UpdateTokenFields(token.ID, map[string]any{"auto_groups": "private,disabled,missing"}); err != nil {
		t.Fatal(err)
	}
	call("/v1/chat/completions", 403, nil)
	if err := model.UpdateTokenFields(token.ID, map[string]any{"group": "disabled"}); err != nil {
		t.Fatal(err)
	}
	call("/v1/chat/completions", 403, nil)
	if err := model.DB.Migrator().DropTable(&model.ServiceGroupConfig{}); err != nil {
		t.Fatal(err)
	}
	call("/v1/chat/completions", 500, nil)
	after, err := model.GetUserByID(owner.ID)
	if err != nil || after.Quota != owner.Quota || after.UsedQuota != 0 {
		t.Fatal("authorization changed account money")
	}
}
