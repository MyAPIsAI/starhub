package middleware

import (
	"testing"

	"starhub/common"
	"starhub/model"
	"starhub/relay/channel"
)

func TestSmartRerankRequiresKnownCapabilities(t *testing.T) {
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
			{"embedding is not rerank", &model.ModelMeta{Tags: "embedding"}, channel.ChannelTypeOpenAI, 503},
			{"exact capability", &model.ModelMeta{Tags: " rerank "}, channel.ChannelTypeOpenAI, 204},
			{"similar capability", &model.ModelMeta{Tags: "rerank-preview"}, channel.ChannelTypeOpenAI, 503},
			{"wrong group", &model.ModelMeta{Tags: "rerank", CompatibleGroups: "private"}, channel.ChannelTypeOpenAI, 503},
			{"unsupported adaptor", &model.ModelMeta{Tags: "rerank"}, channel.ChannelTypeClaude, 503},
		} {
			t.Run(alias+"/"+tc.name, func(t *testing.T) {
				setupAutoRoutingDB(t)
				if err := model.InsertChannel(&model.Channel{Name: "rerank-review", Type: tc.kind, Models: "review-model", Group: "default", Status: common.ChannelStatusEnabled}); err != nil {
					t.Fatal(err)
				}
				if tc.meta != nil {
					tc.meta.Model = "review-model"
					if err := model.UpsertModelMeta(tc.meta); err != nil {
						t.Fatal(err)
					}
				}
				code, actual := routeSmart(t, `{"model":"`+alias+`","query":"review","documents":["review document"]}`, "/v1/rerank", "", "", 1, "")
				if code != tc.want || (code == 204 && actual != "review-model") || (code != 204 && actual != "") {
					t.Fatalf("status=%d model=%q want=%d", code, actual, tc.want)
				}
			})
		}
	}
}
