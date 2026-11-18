// Package ark adapts the OpenAI-compatible subset of the Volcengine Ark API.
package ark

import (
	"strings"

	"starhub/relay/channel"
	"starhub/relay/channel/openai"
)

const DefaultBaseURL = "https://ark.cn-beijing.volces.com"

type Adaptor struct{ openai.Adaptor }

func (Adaptor) Name() string { return "ark" }

// APIBaseURL accepts either an upstream root or its /api/v3 prefix.
func APIBaseURL(raw string) string {
	base := strings.TrimRight(strings.TrimSpace(raw), "/")
	if base == "" {
		base = DefaultBaseURL
	}
	return strings.TrimSuffix(base, "/api/v3") + "/api/v3"
}

func (Adaptor) ChatURL(base, _, _ string, _ bool) string {
	return APIBaseURL(base) + "/chat/completions"
}

func (Adaptor) ModeURL(base string, mode channel.RelayMode) (string, error) {
	path := ""
	switch mode {
	case channel.RelayModeResponses:
		path = "/responses"
	case channel.RelayModeEmbeddings:
		path = "/embeddings"
	case channel.RelayModeImageGeneration:
		path = "/images/generations"
	default:
		return "", channel.ErrUnsupportedMode
	}
	return APIBaseURL(base) + path, nil
}

func (Adaptor) RealtimeURL(string, string) (string, error) {
	return "", channel.ErrUnsupportedMode
}
