package relay

import (
	"fmt"

	"starhub/relay/channel"
	"starhub/relay/channel/aws"
	"starhub/relay/channel/claude"
	"starhub/relay/channel/gemini"
	"starhub/relay/channel/openai"
)

// GetChatAdaptor resolves the chat adaptor of a channel type. Adding a
// provider means adding one case here (P3.1: the registry is the only
// place provider packages are wired together).
func GetChatAdaptor(channelType int) (channel.ChatAdaptor, error) {
	switch channelType {
	case channel.ChannelTypeOpenAI:
		return openai.New(), nil
	case channel.ChannelTypeClaude:
		return claude.New(), nil
	case channel.ChannelTypeGemini:
		return gemini.New(), nil
	case channel.ChannelTypeAWS:
		return aws.New(), nil
	default:
		return nil, fmt.Errorf("unsupported channel type: %d", channelType)
	}
}

// GetTaskAdaptor resolves the async-task adaptor of a channel type (P3.3).
func GetTaskAdaptor(channelType int) (channel.TaskAdaptor, error) {
	switch channelType {
	case channel.ChannelTypeOpenAI:
		return openai.VideoTaskAdaptor{}, nil
	default:
		return nil, fmt.Errorf("channel type %d has no task platform support", channelType)
	}
}

// ChannelTypeName returns the human-readable name of a channel type.
func ChannelTypeName(channelType int) string {
	switch channelType {
	case channel.ChannelTypeOpenAI:
		return "openai"
	case channel.ChannelTypeClaude:
		return "claude"
	case channel.ChannelTypeGemini:
		return "gemini"
	case channel.ChannelTypeAWS:
		return "aws"
	default:
		return fmt.Sprintf("unknown(%d)", channelType)
	}
}

// SupportedChannelTypes lists every channel type the gateway can route to,
// for channel API validation and documentation surfaces.
func SupportedChannelTypes() map[int]string {
	return map[int]string{
		channel.ChannelTypeOpenAI: ChannelTypeName(channel.ChannelTypeOpenAI),
		channel.ChannelTypeClaude: ChannelTypeName(channel.ChannelTypeClaude),
		channel.ChannelTypeGemini: ChannelTypeName(channel.ChannelTypeGemini),
		channel.ChannelTypeAWS:    ChannelTypeName(channel.ChannelTypeAWS),
	}
}
