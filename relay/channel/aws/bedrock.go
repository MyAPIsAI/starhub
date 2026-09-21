// Package aws implements the AWS Bedrock channel adaptor: the P3.1
// representative of a "signature V4" provider. Chat traffic uses the
// converse API (/model/{modelId}/invoke and invoke-with-response-stream,
// the latter framed with the AWS binary event stream).
package aws

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"starhub/relay/channel"
	"starhub/relay/dto"
)

const (
	defaultBedrockTemplate = "https://bedrock-runtime.%s.amazonaws.com"
	maxResponseBytes       = 32 << 20
)

// ErrBadChannelKey marks an unparsable channel key.
var ErrBadChannelKey = errors.New("invalid aws channel key")

type Adaptor struct{}

// New returns the stateless AWS Bedrock adaptor.
func New() *Adaptor { return &Adaptor{} }

func (Adaptor) Name() string { return "aws" }

// NativeInboundFormat is empty: the gateway exposes no Bedrock-native
// endpoint, so every inbound protocol is served through IR conversion.
func (Adaptor) NativeInboundFormat() dto.InboundFormat { return "" }

func (Adaptor) ChatURL(baseURL, apiKey, model string, stream bool) string {
	base := bedrockBase(baseURL, apiKey)
	suffix := "invoke"
	if stream {
		suffix = "invoke-with-response-stream"
	}
	return base + "/model/" + model + "/" + suffix
}

func (Adaptor) ModeURL(_ string, _ channel.RelayMode) (string, error) {
	return "", channel.ErrUnsupportedMode
}

func (Adaptor) RealtimeURL(_, _ string) (string, error) {
	return "", channel.ErrUnsupportedMode
}

// PrepareRequest signs the fully built request with SigV4. The channel key
// carries the credentials ("accessKeyId|secretAccessKey|region"); an
// explicit region in the base URL wins over the key's region.
func (Adaptor) PrepareRequest(req *http.Request, apiKey string, body []byte) error {
	accessKey, secretKey, region, err := ParseChannelKey(apiKey)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrBadChannelKey, err)
	}
	if region == "" {
		region = regionFromBaseURL(req.URL.Host)
	}
	if region == "" {
		return fmt.Errorf("%w: no region in key or base_url", ErrBadChannelKey)
	}
	s := signer{region: region, service: "bedrock", accessKey: accessKey, secretKey: secretKey}
	return s.sign(req, body, time.Now())
}

// PreparePassthroughBody is never called (no native inbound format).
func (Adaptor) PreparePassthroughBody(body []byte, _ bool) ([]byte, error) {
	return body, nil
}

func (Adaptor) ConvertChatRequest(ir *dto.ChatRequest) ([]byte, error) {
	return dto.BuildBedrockChatRequest(ir)
}

// HandlePassthroughResponse is never called (no native inbound format).
func (Adaptor) HandlePassthroughResponse(w http.ResponseWriter, resp *http.Response, _ bool) (*dto.Usage, string, error) {
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, "", err
	}
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
	}
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
	ir, err := dto.ParseBedrockChatResponse(body)
	if err != nil {
		return nil, "", nil
	}
	return ir.Usage, ir.Text, nil
}

// ParseChatResponse reads a converse response into IR.
func (Adaptor) ParseChatResponse(resp *http.Response) (*dto.ChatResponse, error) {
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, err
	}
	return dto.ParseBedrockChatResponse(body)
}

// StreamChatEvents decodes the AWS event stream of an
// invoke-with-response-stream response into IR events. Each "chunk" event
// payload is one converse-stream JSON record (messageStart,
// contentBlockDelta, messageStop, metadata).
func (Adaptor) StreamChatEvents(resp *http.Response) (channel.ChatStreamReader, error) {
	return &streamReader{body: resp.Body}, nil
}

type streamReader struct {
	body io.Reader
}

func (s *streamReader) Next() (*dto.ChatStreamEvent, error) {
	for {
		msg, err := readEventStreamMessage(s.body)
		if err != nil {
			return nil, err
		}
		if msg.EventType != "chunk" || len(msg.Payload) == 0 {
			continue // ping / exception frames are skipped
		}
		ev, perr := dto.ParseBedrockStreamEvent(msg.Payload)
		if perr != nil {
			continue
		}
		if ev.TextDelta == "" && ev.Usage == nil && ev.FinishReason == "" {
			continue // messageStart and friends carry no IR payload yet
		}
		return ev, nil
	}
}

func (s *streamReader) Close() error {
	if closer, ok := s.body.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

// bedrockBase resolves the endpoint base: an operator-provided base_url
// (https://bedrock-runtime.eu-west-1.amazonaws.com or a proxy) wins; an
// empty base falls back to the default template in the key's region.
func bedrockBase(baseURL, apiKey string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base != "" {
		return base
	}
	region := "us-east-1"
	if _, _, keyRegion, err := ParseChannelKey(apiKey); err == nil && keyRegion != "" {
		region = keyRegion
	}
	return fmt.Sprintf(defaultBedrockTemplate, region)
}

// regionFromBaseURL extracts "eu-west-1" out of
// bedrock-runtime.eu-west-1.amazonaws.com style hosts.
func regionFromBaseURL(host string) string {
	host = strings.ToLower(host)
	if !strings.Contains(host, ".amazonaws.com") {
		return ""
	}
	dot := strings.Index(host, ".")
	if dot <= 0 {
		return ""
	}
	return host[:dot]
}
