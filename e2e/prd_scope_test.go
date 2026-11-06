package main

import (
	"net/http"
	"testing"

	"starhub/model"
)

func TestPRDRetiredFeaturesRejectEveryIdentity(t *testing.T) {
	handler, root := staffFixture(t)
	root.enroll("root", "Root-temporary-123")
	code := &model.Redemption{Code: "RD-retired-fixture", Quota: 500000, Status: 1}
	if err := model.InsertRedemption(code); err != nil {
		t.Fatal(err)
	}
	anonymous := &staffHTTP{t: t, handler: handler, ip: "127.0.0.1:18482"}
	for _, caller := range []*staffHTTP{root, anonymous} {
		for _, path := range []string{"/api/redemption", "/api/redemption/", "/api/redemption/1", "/api/task-plugin", "/api/task-plugin/", "/api/task-plugin/1", "/api/task-plugin/1/test", "/api/user/redeem", "/api/user/redeem/"} {
			for _, method := range []string{"GET", "POST", "PUT", "DELETE"} {
				caller.call(method, path, map[string]any{"code": code.Code, "quota": 500000, "count": 1, "source": "throw new Error('must not execute');"}, http.StatusGone)
			}
		}
	}
	stored, err := model.GetRedemptionByCode(code.Code)
	if err != nil || stored.Status != 1 || stored.UsedBy != 0 {
		t.Fatal("retired route changed historical code")
	}
	var codes, plugins int64
	model.DB.Model(&model.Redemption{}).Count(&codes)
	model.DB.Model(&model.TaskPlugin{}).Count(&plugins)
	if codes != 1 || plugins != 0 {
		t.Fatal("retired routes changed stored features")
	}
}

func TestPRDChannelAPIAllowsNativeAdaptersOnly(t *testing.T) {
	_, root := staffFixture(t)
	root.enroll("root", "Root-temporary-123")
	payload := map[string]any{"name": "native-video", "type": 61, "models": "sora-2", "group": "default", "key": "synthetic-key", "base_url": "https://upstream.invalid"}
	root.call("POST", "/api/channel", payload, 400)
	payload["type"], payload["task_plugin_id"] = 1, 123
	root.call("POST", "/api/channel", payload, 400)
	delete(payload, "task_plugin_id")
	created := root.call("POST", "/api/channel", payload, 200)
	payload["id"], payload["type"] = created["id"], 61
	root.call("PUT", "/api/channel", payload, 400)
	payload["type"], payload["task_plugin_id"] = 1, 123
	root.call("PUT", "/api/channel", payload, 400)
	channels, err := model.ListChannels()
	if err != nil || len(channels) != 1 || channels[0].Type != 1 || channels[0].TaskPluginID != 0 {
		t.Fatal("rejected plugin configuration mutated the native channel")
	}
}
