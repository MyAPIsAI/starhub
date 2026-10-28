package middleware

import (
	"testing"

	"starhub/common"
	"starhub/model"
	"starhub/relay/channel"
)

func TestSmartEmbeddingsRequireKnownCapabilities(t *testing.T) {
	for _, alias := range []string{"myapis-auto", "veyra-auto"} {
		for _, tc := range []struct {
			name string
			meta *model.ModelMeta
			kind int
			want int
		}{
			{"missing metadata", nil, channel.ChannelTypeOpenAI, 503},
			{"empty tags", &model.ModelMeta{}, channel.ChannelTypeOpenAI, 503},
			{"blank tags", &model.ModelMeta{Tags: ", ,\t"}, channel.ChannelTypeOpenAI, 503},
			{"text capability", &model.ModelMeta{Tags: "text"}, channel.ChannelTypeOpenAI, 503},
			{"embedding capability", &model.ModelMeta{Tags: "embedding"}, channel.ChannelTypeOpenAI, 204},
			{"plural alias", &model.ModelMeta{Tags: "embeddings"}, channel.ChannelTypeOpenAI, 204},
			{"wrong group", &model.ModelMeta{Tags: "embedding", CompatibleGroups: "private"}, channel.ChannelTypeOpenAI, 503},
			{"unsupported adaptor", &model.ModelMeta{Tags: "embedding"}, channel.ChannelTypeClaude, 503},
		} {
			t.Run(alias+"/"+tc.name, func(t *testing.T) {
				setupAutoRoutingDB(t)
				if err := model.InsertChannel(&model.Channel{Name: "review", Type: tc.kind, Models: "review-model", Group: "default", Status: common.ChannelStatusEnabled}); err != nil {
					t.Fatal(err)
				}
				if tc.meta != nil {
					tc.meta.Model = "review-model"
					if err := model.UpsertModelMeta(tc.meta); err != nil {
						t.Fatal(err)
					}
				}
				code, actual := routeSmart(t, `{"model":"`+alias+`","input":"review"}`, "/v1/embeddings", "", "", 1, "")
				if code != tc.want || (code == 204 && actual != "review-model") || (code != 204 && actual != "") {
					t.Fatalf("status=%d model=%q want=%d", code, actual, tc.want)
				}
			})
		}
	}
}

func TestEmbeddingGuardPreservesBasicTextFallback(t *testing.T) {
	setupAutoRoutingDB(t)
	if err := model.InsertChannel(&model.Channel{Name: "text", Type: channel.ChannelTypeOpenAI, Models: "unknown-text", Group: "default", Status: common.ChannelStatusEnabled}); err != nil {
		t.Fatal(err)
	}
	code, actual := routeSmart(t, `{"model":"myapis-auto","messages":[{"role":"user","content":"review"}],"max_tokens":8}`, "/v1/chat/completions", "", "", 1, "")
	if code != 204 || actual != "unknown-text" {
		t.Fatalf("basic text fallback changed: %d %s", code, actual)
	}
}
