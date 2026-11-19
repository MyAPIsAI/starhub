// Package compatible implements the chat subset shared by domestic providers.
package compatible

import (
	"strings"

	"starhub/relay/channel"
	"starhub/relay/channel/openai"
	"starhub/relay/dto"
)

type Adaptor struct {
	openai.Adaptor
	name, root, prefix string
}

// ForType keeps provider identity and endpoint defaults together.
func ForType(channelType int) (*Adaptor, bool) {
	switch channelType {
	case channel.ChannelTypeZhipu:
		return &Adaptor{name: "zhipu", root: "https://open.bigmodel.cn", prefix: "/api/paas/v4"}, true
	case channel.ChannelTypeMoonshot:
		return &Adaptor{name: "moonshot", root: "https://api.moonshot.cn", prefix: "/v1"}, true
	case channel.ChannelTypeDeepSeek:
		return &Adaptor{name: "deepseek", root: "https://api.deepseek.com"}, true
	default:
		return nil, false
	}
}

func (a Adaptor) Name() string { return a.name }

// APIBaseURL accepts the upstream root or the provider's API prefix.
func (a Adaptor) APIBaseURL(raw string) string {
	base := strings.TrimRight(strings.TrimSpace(raw), "/")
	if base == "" {
		base = a.root
	}
	return strings.TrimSuffix(base, a.prefix) + a.prefix
}

func (a Adaptor) ChatURL(base, _, _ string, _ bool) string {
	return a.APIBaseURL(base) + "/chat/completions"
}

// HasModelCatalog reflects verified discovery support, not a static model list.
func (a Adaptor) HasModelCatalog() bool { return a.name != "zhipu" }

func (Adaptor) ModeURL(string, channel.RelayMode) (string, error) {
	return "", channel.ErrUnsupportedMode
}

func (Adaptor) RealtimeURL(string, string) (string, error) {
	return "", channel.ErrUnsupportedMode
}

func (a Adaptor) PreparePassthroughBody(body []byte, stream bool) ([]byte, error) {
	// Zhipu reports final usage by default and uses chunk_include_usage instead.
	if a.name == "zhipu" {
		return body, nil
	}
	return a.Adaptor.PreparePassthroughBody(body, stream)
}

func (a Adaptor) ConvertChatRequest(ir *dto.ChatRequest) ([]byte, error) {
	body, err := dto.BuildOpenAIChatRequest(ir)
	if err != nil {
		return nil, err
	}
	return a.PreparePassthroughBody(body, ir.Stream)
}
