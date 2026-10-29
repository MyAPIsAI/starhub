package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"sync"

	"starhub/model"
)

type managedTextValidation struct {
	once    sync.Once
	allowed bool
}

func (req smartRequirements) managedTextAllowed() bool {
	if req.textValidation == nil {
		return managedTextBody(req.path, req.body)
	}
	req.textValidation.once.Do(func() { req.textValidation.allowed = managedTextBody(req.path, req.body) })
	return req.textValidation.allowed
}

func (req smartRequirements) resourceCompatible(name, group string, ch *model.Channel) (bool, error) {
	policy, managed, err := model.ReadResourceGatewayPolicy(req.ctx, ch.ID, group, name)
	if err != nil || !managed {
		return !managed && err == nil, err
	}
	allowed := policy.Allows(req.path, req.capabilities, req.input, req.output) && req.managedAllowed(policy)
	if allowed {
		ch.RuntimeModel = policy.UpstreamModel()
	}
	return allowed, nil
}

func resourceGatewayChannelCheck(r *http.Request, body []byte, multipart bool, name string, existing func(*model.Channel, string) (bool, error)) func(*model.Channel, string) (bool, error) {
	var once sync.Once
	var requirements smartRequirements
	var parseErr error
	return func(ch *model.Channel, group string) (bool, error) {
		policy, managed, err := model.ReadResourceGatewayPolicy(r.Context(), ch.ID, group, name)
		if err != nil {
			return false, err
		}
		if managed {
			once.Do(func() { requirements, parseErr = readSmartRequirements(r, body, multipart) })
			if parseErr != nil || !policy.Allows(requirements.path, requirements.capabilities, requirements.input, requirements.output) || !requirements.managedAllowed(policy) {
				return false, nil
			}
		}
		if existing != nil {
			allowed, err := existing(ch, group)
			if err != nil || !allowed {
				return allowed, err
			}
		}
		if managed {
			ch.RuntimeModel = policy.UpstreamModel()
		}
		return true, nil
	}
}

// Managed basic-text profiles reject unknown input shapes instead of forwarding
// attachments, tool payloads or hidden multimodal extensions to a text resource.
func managedTextBody(path string, body []byte) bool {
	if path != "/v1/chat/completions" && path != "/v1/messages" {
		return false
	}
	if !managedUniqueJSON(body) {
		return false
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(body, &fields) != nil || fields == nil {
		return false
	}
	allowed := map[string]bool{}
	keys := []string{"model", "messages", "stream", "temperature", "top_p", "max_tokens", "metadata"}
	if path == "/v1/messages" {
		keys = append(keys, "system", "top_k", "stop_sequences")
	} else {
		keys = append(keys, "stream_options", "max_completion_tokens", "n", "stop", "presence_penalty", "frequency_penalty", "seed", "user", "logprobs", "top_logprobs", "logit_bias", "store", "service_tier")
	}
	for _, key := range keys {
		allowed[key] = true
	}
	for key := range fields {
		if !allowed[key] {
			return false
		}
	}
	if system, exists := fields["system"]; exists && !managedTextContent(system) {
		return false
	}
	var messages []map[string]json.RawMessage
	if json.Unmarshal(fields["messages"], &messages) != nil || len(messages) == 0 {
		return false
	}
	for _, message := range messages {
		for key := range message {
			if key != "role" && key != "content" && (key != "name" || path == "/v1/messages") {
				return false
			}
		}
		var role string
		if json.Unmarshal(message["role"], &role) != nil {
			return false
		}
		if role != "user" && role != "assistant" && (path == "/v1/messages" || (role != "system" && role != "developer")) {
			return false
		}
		if !managedTextContent(message["content"]) {
			return false
		}
	}
	return true
}

func managedTextContent(raw json.RawMessage) bool {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return false
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return true
	}
	var blocks []map[string]json.RawMessage
	if json.Unmarshal(raw, &blocks) != nil || blocks == nil {
		return false
	}
	for _, block := range blocks {
		if len(block) != 2 {
			return false
		}
		var kind string
		if json.Unmarshal(block["type"], &kind) != nil || kind != "text" || bytes.Equal(bytes.TrimSpace(block["text"]), []byte("null")) || json.Unmarshal(block["text"], &text) != nil {
			return false
		}
	}
	return true
}

func managedUniqueJSON(body []byte) bool {
	decoder := json.NewDecoder(bytes.NewReader(body))
	var value func(int) bool
	value = func(depth int) bool {
		if depth > 16 {
			return false
		}
		token, err := decoder.Token()
		if err != nil {
			return false
		}
		delim, container := token.(json.Delim)
		if !container {
			return true
		}
		switch delim {
		case '{':
			seen := map[string]bool{}
			for decoder.More() {
				key, err := decoder.Token()
				name, ok := key.(string)
				if err != nil || !ok || seen[name] {
					return false
				}
				seen[name] = true
				if !value(depth + 1) {
					return false
				}
			}
		case '[':
			for decoder.More() {
				if !value(depth + 1) {
					return false
				}
			}
		default:
			return false
		}
		_, err = decoder.Token()
		return err == nil
	}
	if !value(0) {
		return false
	}
	_, err := decoder.Token()
	return err == io.EOF
}
