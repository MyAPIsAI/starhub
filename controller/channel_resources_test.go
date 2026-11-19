package controller

import (
	"fmt"
	"testing"

	"starhub/common"
	"starhub/model"
)

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
