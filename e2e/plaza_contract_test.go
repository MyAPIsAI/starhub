package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
	"testing"

	"starhub/common"
	"starhub/model"
)

func TestPlazaBrowseContract(t *testing.T) {
	env := setupRelayApp(t)
	name := "publisher/model:latest"
	for _, channelType := range []int{1, 25} {
		if err := model.InsertChannel(&model.Channel{
			Name: fmt.Sprintf("plaza-%d", channelType), Type: channelType,
			Models: name, Group: "default,vip", Status: common.ChannelStatusEnabled,
		}); err != nil {
			t.Fatal(err)
		}
	}
	meta := model.ModelMeta{
		Model: name, Vendor: "openai", VendorDisplay: "OpenAI",
		Description: "description-only-needle", Tags: "text,function_call",
		ContextLength: 1500000, MaxOutput: 4096,
	}
	if err := model.UpsertModelMeta(&meta); err != nil {
		t.Fatal(err)
	}
	type row struct {
		ID              string   `json:"id"`
		Vendor          string   `json:"vendor"`
		VendorDisplay   string   `json:"vendor_display"`
		Protocols       []string `json:"protocols"`
		AvailableGroups []string `json:"available_groups"`
		IsAvailable     bool     `json:"is_available"`
		ChannelCount    int      `json:"channel_count"`
	}
	type list struct {
		Total    int    `json:"total"`
		Page     int    `json:"page"`
		PageSize int    `json:"page_size"`
		Group    string `json:"group"`
		Items    []row  `json:"items"`
	}
	get := func(path string, target any) {
		t.Helper()
		res := doRequest(env.base, "GET", path, "", nil)
		if res.Status != 200 {
			t.Fatalf("%s: %d %s", path, res.Status, res.Body)
		}
		var envelope struct {
			Success bool            `json:"success"`
			Data    json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal([]byte(res.Body), &envelope); err != nil || !envelope.Success {
			t.Fatalf("bad response: %s", res.Body)
		}
		if err := json.Unmarshal(envelope.Data, target); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("vendor identifiers and description search", func(t *testing.T) {
		var result list
		get("/api/plaza/models?vendors=openai&q=description-only-needle", &result)
		if result.Total != 1 || len(result.Items) != 1 {
			t.Fatalf("unexpected matches: %+v", result)
		}
		r := result.Items[0]
		if r.ID != name || r.Vendor != "openai" || r.VendorDisplay != "OpenAI" || !r.IsAvailable {
			t.Fatalf("wrong catalog identity: %+v", r)
		}
		if !reflect.DeepEqual(r.Protocols, []string{"OpenAI", "Anthropic"}) {
			t.Fatalf("wrong gateway protocols: %v", r.Protocols)
		}
		if !reflect.DeepEqual(r.AvailableGroups, []string{"default", "vip"}) || r.ChannelCount != 2 {
			t.Fatalf("wrong coverage: %+v", r)
		}
		get("/api/plaza/models?vendors=OpenAI", &result)
		if result.Total != 0 || len(result.Items) != 0 {
			t.Fatalf("display labels must not match vendor IDs: %+v", result)
		}
	})

	t.Run("detail group and encoded identifiers", func(t *testing.T) {
		var result struct {
			Model row    `json:"model"`
			Group string `json:"group"`
		}
		get("/api/plaza/models/"+url.PathEscape(name), &result)
		if result.Group != "default" || result.Model.ID != name {
			t.Fatalf("wrong detail: %+v", result)
		}
		res := doRequest(env.base, "GET", "/api/plaza/models/gpt-4.1", "", nil)
		if res.Status != 404 {
			t.Fatalf("missing model status: %d", res.Status)
		}
	})

	t.Run("global facets and pagination beyond 200 models", func(t *testing.T) {
		rows := make([]model.ModelMeta, 205)
		for i := range rows {
			rows[i] = model.ModelMeta{Model: fmt.Sprintf("plaza-model-%03d", i), Vendor: "other", VendorDisplay: "Other"}
		}
		if err := model.DB.Create(&rows).Error; err != nil {
			t.Fatal(err)
		}
		var facets struct {
			Total   int `json:"total"`
			Vendors []struct {
				Value string `json:"value"`
				Count int    `json:"count"`
			} `json:"vendors"`
		}
		get("/api/plaza/filters?q=description-only-needle", &facets)
		if facets.Total != 206 || len(facets.Vendors) != 2 {
			t.Fatalf("facets must cover the whole catalog: %+v", facets)
		}
		var result list
		get("/api/plaza/models?page=2&page_size=200&sort=name_asc", &result)
		if result.Total != 206 || result.Page != 2 || result.PageSize != 200 || len(result.Items) != 6 {
			t.Fatalf("wrong second page: %+v", result)
		}
		for _, r := range result.Items {
			if r.ID != name && (r.IsAvailable || len(r.Protocols) != 0) {
				t.Fatalf("metadata-only model advertised as connectable: %+v", r)
			}
		}
	})
}
