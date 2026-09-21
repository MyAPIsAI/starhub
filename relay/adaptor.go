package relay

import (
	"fmt"
	"net/http"
)

type Usage struct {
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
}

const ChannelTypeOpenAI = 1

type Adaptor interface {
	Name() string
	ConvertRequestBody(body []byte, stream bool) ([]byte, error)
	RequestURL(baseURL string) string
	SetupRequestHeader(req *http.Request, channelKey string)
	HandleResponse(w http.ResponseWriter, resp *http.Response, stream bool) (*Usage, string, error)
}

func GetAdaptor(channelType int) (Adaptor, error) {
	switch channelType {
	case ChannelTypeOpenAI:
		return openaiAdaptor{}, nil
	default:
		return nil, fmt.Errorf("unsupported channel type: %d", channelType)
	}
}
