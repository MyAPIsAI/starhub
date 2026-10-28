package middleware

import (
	"starhub/common"
	"starhub/model"
	"starhub/relay/channel"
	"testing"
)

func TestSmartNonChatEndpointsRequireCapabilities(t *testing.T) {
	for path, tags := range smartEndpointTags {
		for _, alias := range []string{"myapis-auto", "veyra-auto"} {
			for _, state := range []string{"missing", "blank", "wrong", "valid"} {
				t.Run(path+"/"+alias+"/"+state, func(t *testing.T) {
					setupAutoRoutingDB(t)
					if err := model.InsertChannel(&model.Channel{Name: "review", Type: channel.ChannelTypeOpenAI, Models: "review-model", Group: "default", Status: common.ChannelStatusEnabled}); err != nil {
						t.Fatal(err)
					}
					if state != "missing" {
						tag := " , "
						if state == "wrong" {
							tag = "text"
						}
						if state == "valid" {
							tag = tags[len(tags)-1]
						}
						if err := model.UpsertModelMeta(&model.ModelMeta{Model: "review-model", Tags: tag}); err != nil {
							t.Fatal(err)
						}
					}
					code, actual := routeSmart(t, `{"model":"`+alias+`","input":"review","max_tokens":8}`, path, "", "", 1, "")
					want := 503
					if state == "valid" {
						want = 204
					}
					if code != want || (want == 204 && actual != "review-model") || (want != 204 && actual != "") {
						t.Fatalf("status=%d model=%q want=%d", code, actual, want)
					}
				})
			}
		}
	}
}

func TestSmartCapabilityAliasesStayWithinTheirEndpoints(t *testing.T) {
	for _, tc := range []struct {
		path, tag, capability string
		want                  int
	}{
		{"/v1/audio/speech", "tts", "audio", 204},
		{"/v1/audio/transcriptions", "stt", "audio", 204},
		{"/v1/embeddings", "embeddings", "embedding", 204},
		{"/v1/images/generations", "image", "audio", 503},
	} {
		t.Run(tc.path, func(t *testing.T) {
			setupAutoRoutingDB(t)
			if err := model.InsertChannel(&model.Channel{Name: "review", Type: channel.ChannelTypeOpenAI, Models: "review-model", Group: "default", Status: common.ChannelStatusEnabled}); err != nil {
				t.Fatal(err)
			}
			if err := model.UpsertModelMeta(&model.ModelMeta{Model: "review-model", Tags: tc.tag}); err != nil {
				t.Fatal(err)
			}
			code, _ := routeSmart(t, `{"model":"myapis-auto","input":"review","max_tokens":8}`, tc.path, "X-MyAPIs-Capability", tc.capability, 1, "")
			if code != tc.want {
				t.Fatalf("capability alias crossed endpoints: %d want=%d", code, tc.want)
			}
		})
	}
}
