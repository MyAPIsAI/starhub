package controller

import (
	"encoding/json"
	"net/http/httptest"
	"reflect"
	"testing"

	"starhub/common"
	"starhub/model"
	"starhub/relay/channel"
)

func TestPlazaProtocols(t *testing.T) {
	tests := []struct {
		name  string
		types []int
		tags  []string
		want  []string
	}{
		{"chat conversion", []int{channel.ChannelTypeGemini}, []string{"text"}, []string{"OpenAI", "Anthropic"}},
		{"vision chat", []int{channel.ChannelTypeClaude}, []string{"text", "vision"}, []string{"OpenAI", "Anthropic"}},
		{"image generation", []int{channel.ChannelTypeOpenAI}, []string{"image"}, []string{"OpenAI"}},
		{"embeddings", []int{channel.ChannelTypeOpenAI}, []string{"embedding"}, []string{"OpenAI"}},
		{"audio", []int{channel.ChannelTypeOpenAI}, []string{"audio"}, []string{"OpenAI"}},
		{"unsupported media channel", []int{channel.ChannelTypeClaude}, []string{"video"}, []string{}},
		{"unknown channel", []int{999}, []string{"text"}, []string{}},
		{"no channel", nil, []string{"text"}, []string{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := plazaProtocols(test.types, test.tags); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("protocols = %v, want %v", got, test.want)
			}
		})
	}
}

func TestPlazaProtocolsFollowGroupAndChannelStatus(t *testing.T) {
	newTestDB(t)
	add := func(name, group, modelName string, channelType int) *model.Channel {
		t.Helper()
		ch := &model.Channel{Name: name, Type: channelType, Models: modelName, Group: group, Status: common.ChannelStatusEnabled, Weight: 1}
		if err := model.InsertChannel(ch); err != nil {
			t.Fatal(err)
		}
		return ch
	}
	first := add("chat-openai", "default", "gpt-4o", channel.ChannelTypeOpenAI)
	second := add("chat-claude", "default", "gpt-4o", channel.ChannelTypeClaude)
	add("media-claude", "default", "scoped-image", channel.ChannelTypeClaude)
	add("media-openai", "vip", "scoped-image", channel.ChannelTypeOpenAI)
	for _, meta := range []*model.ModelMeta{{Model: "gpt-4o", Tags: "text,vision"}, {Model: "scoped-image", Tags: "image"}, {Model: "metadata-only", Tags: "text"}} {
		if err := model.UpsertModelMeta(meta); err != nil {
			t.Fatal(err)
		}
	}
	rowsFor := func(group string) map[string]plazaRow {
		t.Helper()
		rows, err := buildPlazaRows(group, group)
		if err != nil {
			t.Fatal(err)
		}
		out := make(map[string]plazaRow)
		for _, row := range rows {
			out[row.ID] = row
		}
		return out
	}
	rows := rowsFor("default")
	if got := rows["gpt-4o"]; got.ChannelCount != 2 || !reflect.DeepEqual(got.Protocols, []string{"OpenAI", "Anthropic"}) {
		t.Fatalf("mixed chat channel coverage = %#v", got)
	}
	if got := rows["scoped-image"].Protocols; len(got) != 0 {
		t.Fatalf("other group protocols leaked: %v", got)
	}
	if got := rowsFor("vip")["scoped-image"].Protocols; !reflect.DeepEqual(got, []string{"OpenAI"}) {
		t.Fatalf("vip image protocols = %v", got)
	}
	if got := rows["metadata-only"]; got.IsAvailable || len(got.Protocols) != 0 {
		t.Fatalf("metadata-only model is connectable: %#v", got)
	}
	for _, ch := range []*model.Channel{first, second} {
		if err := model.UpdateChannelStatus(ch.ID, common.ChannelStatusManuallyDisabled); err != nil {
			t.Fatal(err)
		}
	}
	if got := rowsFor("default")["gpt-4o"]; got.IsAvailable || got.ChannelCount != 0 || len(got.Protocols) != 0 {
		t.Fatalf("disabled channels still advertised: %#v", got)
	}
	request := httptest.NewRequest("GET", "/api/plaza/models/gpt-4o", nil)
	request.SetPathValue("id", "gpt-4o")
	response := httptest.NewRecorder()
	PlazaGetModel(response, request)
	var envelope struct {
		Data struct {
			Group string   `json:"group"`
			Model plazaRow `json:"model"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if response.Code != 200 || envelope.Data.Group != "default" || envelope.Data.Model.Protocols == nil {
		t.Fatalf("detail group/protocol contract = %s", response.Body.String())
	}
}
