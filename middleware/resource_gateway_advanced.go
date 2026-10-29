package middleware

import (
	"encoding/json"
	"strings"

	"starhub/model"
	"starhub/relay/dto"
)

func (req smartRequirements) managedAllowed(policy *model.ResourceGatewayPolicy) bool {
	if policy == nil {
		return false
	}
	if !policy.Advanced() {
		return req.managedTextAllowed()
	}
	if req.multipart {
		return req.path == "/v1/images/edits" || req.path == "/v1/audio/transcriptions" || req.path == "/v1/audio/translations"
	}
	if !managedUniqueJSON(req.body) {
		return false
	}
	if req.path == "/v1beta/models" {
		_, err := dto.ParseGeminiRequest(req.body)
		return err == nil
	}
	if req.path == "/v1/responses" || req.path == "/v1/responses/compact" {
		_, err := dto.ParseResponsesRequest(req.body)
		return err == nil
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(req.body, &fields) != nil {
		return false
	}
	if req.path != "/v1/chat/completions" && req.path != "/v1/messages" {
		keys := map[string]string{
			"/v1/embeddings":         "model input encoding_format dimensions user",
			"/v1/rerank":             "model query documents top_n return_documents max_chunks_per_doc",
			"/v1/audio/speech":       "model input voice response_format speed instructions",
			"/v1/images/generations": "model prompt n size quality style response_format user background output_format output_compression moderation",
			"/v1/video/generations":  "model prompt seconds size n extra",
		}[req.path]
		if keys == "" {
			return false
		}
		known := map[string]bool{}
		for _, key := range strings.Fields(keys) {
			known[key] = true
		}
		for name := range fields {
			if !known[name] {
				return false
			}
		}
		if extra, ok := fields["extra"]; ok && string(extra) != "null" && string(extra) != "{}" {
			return false
		}
		if req.path == "/v1/embeddings" {
			var text string
			var texts []string
			var tokens []int64
			var batches [][]int64
			input := fields["input"]
			return json.Unmarshal(input, &text) == nil || json.Unmarshal(input, &texts) == nil || json.Unmarshal(input, &tokens) == nil || json.Unmarshal(input, &batches) == nil
		}
		return true
	}
	allowed := strings.Fields("model messages system stream stream_options temperature top_p top_k max_tokens max_completion_tokens metadata stop stop_sequences n presence_penalty frequency_penalty seed user logprobs top_logprobs logit_bias store service_tier tools functions tool_choice function_call parallel_tool_calls reasoning_effort thinking response_format modalities audio")
	known := map[string]bool{}
	for _, name := range allowed {
		known[name] = true
	}
	for name := range fields {
		if !known[name] {
			return false
		}
	}
	needed := append([]string{}, req.capabilities...)
	if system, ok := fields["system"]; ok && !managedTextContent(system) {
		return false
	}
	for _, name := range []string{"tools", "functions", "tool_choice", "function_call", "parallel_tool_calls"} {
		if _, ok := fields[name]; ok {
			needed = append(needed, "function_call")
		}
	}
	for _, name := range []string{"reasoning_effort", "thinking"} {
		if _, ok := fields[name]; ok {
			needed = append(needed, "reasoning")
		}
	}
	for _, name := range []string{"modalities", "audio"} {
		if _, ok := fields[name]; ok {
			needed = append(needed, "audio")
		}
	}
	var messages []map[string]json.RawMessage
	if json.Unmarshal(fields["messages"], &messages) != nil || len(messages) == 0 {
		return false
	}
	for _, message := range messages {
		for name := range message {
			switch name {
			case "role", "content", "name", "tool_calls", "function_call", "tool_call_id", "audio":
			default:
				return false
			}
		}
		if _, ok := message["audio"]; ok {
			needed = append(needed, "audio")
		}
		var blocks []map[string]json.RawMessage
		var text string
		content := message["content"]
		if len(content) == 0 || string(content) == "null" || json.Unmarshal(content, &text) == nil {
			continue
		}
		if json.Unmarshal(content, &blocks) != nil {
			return false
		}
		for _, block := range blocks {
			var kind string
			if json.Unmarshal(block["type"], &kind) != nil {
				return false
			}
			var names []string
			switch kind {
			case "text":
				names = []string{"type", "text", "cache_control"}
			case "image_url":
				needed = append(needed, "vision")
				names = []string{"type", "image_url"}
			case "image":
				needed = append(needed, "vision")
				names = []string{"type", "source", "cache_control"}
			case "input_audio":
				needed = append(needed, "audio")
				names = []string{"type", "input_audio"}
			case "tool_use":
				needed = append(needed, "function_call")
				names = []string{"type", "id", "name", "input", "cache_control"}
			case "tool_result":
				needed = append(needed, "function_call")
				names = []string{"type", "tool_use_id", "content", "is_error", "cache_control"}
				// Nested attachments are denied until they can be classified too.
				if !managedTextContent(block["content"]) {
					return false
				}
			case "thinking":
				needed = append(needed, "reasoning")
				names = []string{"type", "thinking", "signature"}
			case "redacted_thinking":
				needed = append(needed, "reasoning")
				names = []string{"type", "data"}
			default:
				return false
			}
			for name := range block {
				found := false
				for _, allowed := range names {
					found = found || name == allowed
				}
				if !found {
					return false
				}
			}
		}
	}
	return policy.Allows(req.path, needed, req.input, req.output)
}
