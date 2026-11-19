package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"starhub/common"
	"starhub/model"
)

func TestSetChannelResourceStatusValidation(t *testing.T) {
	newTestDB(t)
	ch := mustChannel(t, "resource toggle", "alpha,beta", "fixture-key", common.ChannelStatusEnabled)
	invoke := func(body string) *httptest.ResponseRecorder {
		return call(SetChannelResourceStatus, testRequest{method: "PUT", target: fmt.Sprintf("/api/channel/%d/resource-status", ch.ID), path: map[string]string{"id": fmt.Sprint(ch.ID)}, body: body})
	}
	for _, body := range []string{`{}`, `{"model":"alpha"}`, `{"model":"alpha","enabled":null}`, `{"model":"alpha","enabled":"false"}`, `{"model":"alpha,beta","enabled":false}`} {
		if response := invoke(body); response.Code != http.StatusBadRequest {
			t.Fatal(response.Body)
		}
	}
	if response := invoke(`{"model":"missing","enabled":true}`); response.Code != http.StatusNotFound {
		t.Fatal(response.Body)
	}
	var updated struct {
		ResourceState model.ChannelResourceState `json:"resource_state"`
	}
	decodeData(t, invoke(`{"model":"alpha","enabled":false}`), &updated)
	if updated.ResourceState.State != "partial" || updated.ResourceState.Enabled != 1 {
		t.Fatal(updated)
	}
}

func TestChannelResponsesIncludeResourceSummary(t *testing.T) {
	newTestDB(t)
	ch := mustChannel(t, "resource summary", "alpha,beta", "fixture-key", common.ChannelStatusEnabled)
	if err := model.DB.Create(&model.ChannelResourceDisable{ChannelID: ch.ID, Model: "beta"}).Error; err != nil {
		t.Fatal(err)
	}
	var detail struct {
		ResourceState model.ChannelResourceState `json:"resource_state"`
	}
	decodeData(t, call(GetChannel, testRequest{target: fmt.Sprintf("/api/channel/%d", ch.ID), path: map[string]string{"id": fmt.Sprint(ch.ID)}}), &detail)
	if detail.ResourceState.State != "partial" || detail.ResourceState.Total != 2 || detail.ResourceState.Enabled != 1 {
		t.Fatalf("summary: %+v", detail)
	}
	var list []struct {
		ResourceState model.ChannelResourceState `json:"resource_state"`
	}
	decodeData(t, call(ListChannels, testRequest{target: "/api/channel"}), &list)
	if len(list) != 1 || list[0].ResourceState.State != "partial" {
		t.Fatal(list)
	}
}
