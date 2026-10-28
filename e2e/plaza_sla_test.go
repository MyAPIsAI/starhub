package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"starhub/common"
	"starhub/middleware"
	"starhub/model"
	"starhub/setting"
)

func TestPlazaTargetsFollowSessionGroupAndNeverChangeBilling(t *testing.T) {
	env := setupRelayApp(t)
	if err := model.InsertChannel(&model.Channel{Name: "SLA browse fixture", Type: 1, Key: "synthetic", Models: "gpt-4o", Group: "default,vip", Status: common.ChannelStatusEnabled}); err != nil {
		t.Fatal(err)
	}
	if err := setting.UpdateRatio("group_ratio", `{"default":1,"vip":0.8}`); err != nil {
		t.Fatal(err)
	}
	pointer := func(value int64) *int64 { return &value }
	groups := map[string]*model.ServiceGroupConfig{}
	for _, id := range []string{"default", "vip"} {
		group := &model.ServiceGroupConfig{GroupID: id, Name: id, Multiplier: "1", Enabled: true}
		if err := model.UpsertServiceGroup(group); err != nil {
			t.Fatal(err)
		}
		groups[id] = group
	}
	if _, err := model.UpdateServiceGroupSLA(context.Background(), "default", groups["default"].ID, 0, &model.ServiceGroupSLATarget{SuccessRatePPM: pointer(995000), P95LatencyMS: pointer(800)}); err != nil {
		t.Fatal(err)
	}
	if _, err := model.UpdateServiceGroupSLA(context.Background(), "vip", groups["vip"].ID, 0, &model.ServiceGroupSLATarget{SuccessRatePPM: pointer(0), P95LatencyMS: pointer(1200)}); err != nil {
		t.Fatal(err)
	}
	user := &model.User{Username: "sla-vip", Role: common.RoleUser, Status: common.StatusEnabled, Group: "vip", Quota: 1234, AffCode: common.GenerateAffCode()}
	if err := model.CreateUser(user); err != nil {
		t.Fatal(err)
	}
	session := middleware.Sessions.Create(user.ID, time.Hour)
	t.Cleanup(func() { middleware.Sessions.Remove(session) })
	before, err := model.OptionVersion()
	if err != nil {
		t.Fatal(err)
	}
	read := func(path, session string) struct {
		Group string                 `json:"group"`
		SLA   *model.ServiceGroupSLA `json:"group_sla"`
	} { t.Helper(); res := doRequest(env.base, "GET", path, session, nil); if res.Status != 200 {
		t.Fatalf("browse: %d %s", res.Status, res.Body)
	}; var body struct {
		Data struct {
			Group string                 `json:"group"`
			SLA   *model.ServiceGroupSLA `json:"group_sla"`
		} `json:"data"`
	}; if err := json.Unmarshal([]byte(res.Body), &body); err != nil {
		t.Fatal(err)
	}; return body.Data }
	for _, path := range []string{"/api/plaza/models?group=vip", "/api/plaza/models/gpt-4o?group=vip"} {
		reply := read(path, "")
		if reply.Group != "default" || reply.SLA == nil || reply.SLA.Targets == nil || *reply.SLA.Targets.SuccessRatePPM != 995000 || *reply.SLA.Targets.P95LatencyMS != 800 {
			t.Fatalf("anonymous query overrode session group: %+v", reply)
		}
	}
	for _, path := range []string{"/api/plaza/models?group=default", "/api/plaza/models/gpt-4o?group=default"} {
		reply := read(path, session)
		if reply.Group != "vip" || reply.SLA == nil || reply.SLA.Targets == nil || *reply.SLA.Targets.SuccessRatePPM != 0 || *reply.SLA.Targets.P95LatencyMS != 1200 {
			t.Fatalf("session target was lost or replaced: %+v", reply)
		}
	}
	if _, err := model.UpdateServiceGroupSLA(context.Background(), "vip", groups["vip"].ID, 1, nil); err != nil {
		t.Fatal(err)
	}
	if reply := read("/api/plaza/models/gpt-4o", session); reply.SLA == nil || reply.SLA.Targets != nil || reply.SLA.Revision != 2 {
		t.Fatalf("unpublished target fabricated: %+v", reply)
	}
	groups["default"].Enabled = false
	if err := model.UpsertServiceGroup(groups["default"]); err != nil {
		t.Fatal(err)
	}
	if reply := read("/api/plaza/models/gpt-4o", ""); reply.SLA != nil {
		t.Fatalf("disabled definition advertised target: %+v", reply)
	}
	if after, _ := model.OptionVersion(); after != before {
		t.Fatal("reading targets changed billing settings")
	}
	after, err := model.GetUserByID(user.ID)
	if err != nil || after.Quota != 1234 {
		t.Fatalf("browse changed quota: %+v %v", after, err)
	}
}
