package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"starhub/common"
	"starhub/model"
	"starhub/relay"
	"starhub/relay/channel"
	"starhub/relay/dto"
	"strings"
)

type smartRequirements struct {
	ctx            context.Context
	body           []byte
	textValidation *managedTextValidation
	path           string
	capabilities   []string
	input, output  int64
	chain          string
	continuation   bool
	multipart      bool
}

var chainIDPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,100}$`)

var smartEndpointTags = map[string][]string{
	"/v1/responses":         {"responses"},
	"/v1/responses/compact": {"responses"},
	"/v1/video/generations": {"video"}, "/v1/embeddings": {"embedding", "embeddings"},
	"/v1/rerank": {"rerank"}, "/v1/audio/speech": {"audio", "tts"},
	"/v1/audio/transcriptions": {"audio", "stt"}, "/v1/audio/translations": {"audio", "stt"},
	"/v1/images/generations": {"image"}, "/v1/images/edits": {"image"},
}

func routingHeader(r *http.Request, name string) string {
	if value := r.Header.Get("X-MyAPIs-" + name); value != "" {
		return value
	}
	return r.Header.Get("X-Veyra-" + name)
}
func readSmartRequirements(r *http.Request, body []byte, multipart bool) (smartRequirements, error) {
	req := smartRequirements{multipart: multipart, ctx: r.Context(), body: body, textValidation: &managedTextValidation{}, path: r.URL.Path, chain: routingHeader(r, "Chain-Id"), input: 1, output: 1}
	if req.chain != "" && !chainIDPattern.MatchString(req.chain) {
		return req, errors.New("invalid routing tool-chain identifier")
	}
	capability := routingHeader(r, "Capability")
	if capability != "" {
		switch capability {
		case "text", "code", "vision", "reasoning", "image", "video", "audio", "embedding", "rerank":
		default:
			return req, errors.New("unknown routing capability")
		}
		req.capabilities = append(req.capabilities, capability)
	}
	if !multipart {
		if isGeminiPath(r.URL.Path) {
			return readGeminiRequirements(r, body, req)
		}
		if r.URL.Path == "/v1/responses" || r.URL.Path == "/v1/responses/compact" {
			return readResponsesRequirements(r, body, req)
		}
		var fields struct {
			Messages []struct {
				Role         string            `json:"role"`
				Content      json.RawMessage   `json:"content"`
				ToolCalls    []json.RawMessage `json:"tool_calls"`
				FunctionCall json.RawMessage   `json:"function_call"`
			} `json:"messages"`
			Tools               []json.RawMessage `json:"tools"`
			Functions           []json.RawMessage `json:"functions"`
			MaxTokens           int64             `json:"max_tokens"`
			MaxCompletionTokens int64             `json:"max_completion_tokens"`
			System              json.RawMessage   `json:"system"`
		}
		if err := json.Unmarshal(body, &fields); err != nil {
			return req, err
		}
		req.input = dto.EstimateTokens(relay.ExtractPromptText(body))
		if len(fields.System) > 0 {
			req.input += dto.EstimateTokens(string(fields.System))
		}
		for _, tool := range fields.Tools {
			req.input += dto.EstimateTokens(string(tool))
		}
		for _, tool := range fields.Functions {
			req.input += dto.EstimateTokens(string(tool))
		}
		req.output = max(fields.MaxTokens, fields.MaxCompletionTokens)
		if req.output <= 0 {
			req.output = common.DefaultCompletionBudgetTokens
		}
		continuation := false
		var firstUser json.RawMessage
		for _, message := range fields.Messages {
			for _, call := range message.ToolCalls {
				req.input += dto.EstimateTokens(string(call))
			}
			if len(message.FunctionCall) > 0 && string(message.FunctionCall) != "null" {
				req.input += dto.EstimateTokens(string(message.FunctionCall))
			}
			continuation = continuation || message.Role == "tool" || message.Role == "function" || len(message.ToolCalls) > 0 || (len(message.FunctionCall) > 0 && string(message.FunctionCall) != "null")
			vision, toolBlocks := smartContentCapabilities(message.Content)
			if toolBlocks {
				req.input += smartToolBlockTokens(message.Content)
			}
			if vision {
				req.capabilities = append(req.capabilities, "vision")
			}
			continuation = continuation || toolBlocks
			if firstUser == nil && message.Role == "user" && !continuation {
				firstUser = message.Content
			}
		}
		if len(fields.Tools) > 0 || len(fields.Functions) > 0 || continuation {
			req.capabilities = append(req.capabilities, "function_call")
			if req.chain == "" {
				var err error
				req.chain, err = inferToolChain(r.URL.Path, firstUser)
				if err != nil {
					return req, err
				}
			}
			req.continuation = continuation
		}
	}
	return req, nil
}
func (req smartRequirements) compatible(name, group string, ch *model.Channel) (bool, error) {
	if allowed, err := req.resourceCompatible(name, group, ch); err != nil || !allowed {
		return false, err
	}
	meta, found, err := model.GetModelMeta(name)
	if err != nil {
		return false, err
	}
	// An adaptor serving an endpoint does not establish a model's capability.
	if tags := smartEndpointTags[req.path]; len(tags) > 0 {
		if !found {
			return false, nil
		}
		matched := false
		for _, tag := range tags {
			matched = matched || modelInList(tag, meta.TagList())
		}
		if !matched {
			return false, nil
		}
	}
	if found {
		if len(meta.Groups()) > 0 && !modelInList(group, meta.Groups()) {
			return false, nil
		}
		if meta.MaxOutput > 0 && req.output > meta.MaxOutput {
			return false, nil
		}
		if meta.ContextLength > 0 && (req.input > meta.ContextLength || req.output > meta.ContextLength-req.input) {
			return false, nil
		}
	}
	for _, capability := range req.capabilities {
		if capability == "text" {
			continue
		}
		accepted := []string{capability}
		if capability == "embedding" {
			accepted = []string{"embedding", "embeddings"}
		}
		if capability == "audio" && strings.HasPrefix(req.path, "/v1/audio/") {
			accepted = smartEndpointTags[req.path]
		}
		matched := false
		if found {
			for _, tag := range accepted {
				matched = matched || modelInList(tag, meta.TagList())
			}
		}
		if !matched {
			return false, nil
		}
	}
	adaptor, err := relay.GetChatAdaptor(ch.Type)
	switch req.path {
	case "/v1beta/models":
		if _, _, pathErr := dto.ParseGeminiPath("/v1beta/models/" + name + ":generateContent"); pathErr != nil {
			return false, nil
		}
		if err == nil && adaptor.NativeInboundFormat() != dto.InboundGemini {
			return false, nil
		}
	case "/v1/responses", "/v1/responses/compact":
		if err == nil {
			_, err = adaptor.ModeURL(ch.BaseURL, channel.RelayModeResponses)
		}
	case "/v1/video/generations":
		_, err = relay.GetTaskAdaptor(ch.Type)
	case "/v1/embeddings", "/v1/rerank", "/v1/audio/speech", "/v1/audio/transcriptions", "/v1/audio/translations", "/v1/images/generations", "/v1/images/edits":
		if err == nil {
			modes := map[string]channel.RelayMode{"/v1/embeddings": channel.RelayModeEmbeddings, "/v1/rerank": channel.RelayModeRerank, "/v1/audio/speech": channel.RelayModeTTS, "/v1/audio/transcriptions": channel.RelayModeSTT, "/v1/audio/translations": channel.RelayModeSTTTranslation, "/v1/images/generations": channel.RelayModeImageGeneration, "/v1/images/edits": channel.RelayModeImageEdit}
			_, err = adaptor.ModeURL(ch.BaseURL, modes[req.path])
		}
	default:
		if found {
			for _, tag := range meta.TagList() {
				switch tag {
				case "image", "video", "embedding", "embeddings", "rerank", "audio", "tts", "stt":
					return false, nil
				}
			}
		}
	}
	return err == nil, nil
}

func (req smartRequirements) available(name, group string) (bool, error) {
	excluded := map[int64]bool{}
	for {
		ch, err := model.GetSatisfiedChannelExcluding(group, name, excluded)
		if errors.Is(err, model.ErrNoChannel) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if ch == nil {
			return false, nil
		}
		ok, err := req.compatible(name, group, ch)
		if err != nil || ok {
			return ok, err
		}
		excluded[ch.ID] = true
	}
}

func chooseSmartModel(r *http.Request, requirements smartRequirements) (string, error) {
	group, _ := r.Context().Value(common.CtxUsingGroup).(string)
	tokenID, _ := r.Context().Value(common.CtxTokenID).(int64)
	var pinned string
	if requirements.chain != "" {
		if tokenID <= 0 {
			return "", errors.New("tool-chain routing requires a token identity")
		}
		var err error
		pinned, err = model.FindRoutingChain(tokenID, group, requirements.chain)
		if err != nil {
			return "", err
		}
		if pinned == "" && requirements.continuation {
			return "", errSmartChainHistory
		}
	}
	groups := []string{group}
	if group == common.AutoGroup {
		groups, _ = r.Context().Value(common.CtxAutoGroups).([]string)
	}
	limits, _ := r.Context().Value(common.CtxTokenModelLimits).(string)
	for _, candidateGroup := range groups {
		candidates := model.SplitList(limits)
		if limits == "" {
			var err error
			candidates, err = model.ListModelsByGroup(candidateGroup)
			if err != nil {
				return "", err
			}
		}
		best, bestQuota := "", int64(1<<62)
		for _, name := range candidates {
			if isAutoModel(name) || (pinned != "" && name != pinned) {
				continue
			}
			ok, err := requirements.available(name, candidateGroup)
			if err != nil {
				return "", err
			}
			if !ok {
				continue
			}
			cost := relay.ComputeQuota(name, candidateGroup, requirements.input, requirements.output)
			if cost < bestQuota {
				best, bestQuota = name, cost
			}
		}
		if best != "" {
			return best, nil
		}
	}
	return "", nil
}
