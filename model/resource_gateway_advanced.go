package model

import (
	"encoding/json"
	"regexp"
	"slices"
)

const ResourceGatewayAdvancedProfile = "exact_resource_policy_v2"

var resourceAdvancedCapabilities = []string{"text", "code", "vision", "reasoning", "function_call", "responses", "image", "video", "audio", "embedding", "rerank"}

// A protocol declares the provider wire format, not a grant to every endpoint.
func resourceNativeProtocol(channelType int) string {
	switch channelType {
	case 1:
		return "OpenAI"
	case 14:
		return "Anthropic"
	case 25:
		return "Gemini"
	}
	return ""
}

func validResourceProfile(profile string) bool {
	return profile == ResourceGatewayTextProfile || profile == ResourceGatewayAdvancedProfile
}

func resourceAdvancedRoute(route ResourceGatewayRoute) *ResourceGatewayPolicy {
	var caps, protocols []string
	if !resourceText(route.UpstreamModel, 128, true) || !concreteResourceModel(route.UpstreamModel) || json.Unmarshal([]byte(route.Capabilities), &caps) != nil || len(caps) == 0 || !resourceEnums(caps, resourceAdvancedCapabilities...) || json.Unmarshal([]byte(route.Protocols), &protocols) != nil || len(protocols) != 1 || !resourceEnums(protocols, "OpenAI", "Anthropic", "Gemini") {
		return nil
	}
	if protocols[0] == "Gemini" && !regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`).MatchString(route.UpstreamModel) {
		return nil
	}
	if protocols[0] != "OpenAI" {
		if !slices.Contains(caps, "text") && !slices.Contains(caps, "code") {
			return nil
		}
		for _, cap := range caps {
			if !slices.Contains([]string{"text", "code", "vision", "reasoning", "function_call"}, cap) {
				return nil
			}
		}
	} else {
		endpoint := false
		for _, cap := range caps {
			endpoint = endpoint || slices.Contains([]string{"text", "code", "responses", "image", "video", "audio", "embedding", "rerank"}, cap)
		}
		if !endpoint {
			return nil
		}
	}
	return &ResourceGatewayPolicy{contextLength: route.ContextLength, advanced: true, upstream: route.UpstreamModel, protocol: protocols[0], capabilities: caps}
}

func (policy *ResourceGatewayPolicy) Advanced() bool { return policy != nil && policy.advanced }
func (policy *ResourceGatewayPolicy) UpstreamModel() string {
	if policy == nil {
		return ""
	}
	return policy.upstream
}

func (policy *ResourceGatewayPolicy) allowsAdvanced(path string, caps []string) bool {
	has := func(capability string) bool { return slices.Contains(policy.capabilities, capability) }
	for _, cap := range caps {
		if cap != "text" && !(cap == "gemini" && policy.protocol == "Gemini") && !has(cap) {
			return false
		}
		if cap == "audio" && policy.protocol != "OpenAI" {
			return false
		}
		if cap == "reasoning" && ((policy.protocol == "Anthropic" && path != "/v1/messages") || (policy.protocol == "OpenAI" && path == "/v1/messages")) {
			return false
		}
	}
	if policy.protocol == "Gemini" {
		return path == "/v1beta/models" && (has("text") || has("code"))
	}
	if path == "/v1/chat/completions" || path == "/v1/messages" {
		return has("text") || has("code")
	}
	if policy.protocol != "OpenAI" {
		return false
	}
	needed := map[string]string{
		"/v1/responses": "responses", "/v1/responses/compact": "responses",
		"/v1/images/generations": "image", "/v1/images/edits": "image",
		"/v1/video/generations": "video", "/v1/embeddings": "embedding", "/v1/rerank": "rerank",
		"/v1/audio/speech": "audio", "/v1/audio/transcriptions": "audio", "/v1/audio/translations": "audio",
	}
	cap, ok := needed[path]
	return ok && has(cap)
}

// The SQL prefilter must reject malformed advanced rows before candidate limits.
func resourceAdvancedSQL(alias string) string {
	return `(` + alias + `.upstream_model <> '' AND json_valid(` + alias + `.capabilities) AND json_type(` + alias + `.capabilities) = 'array'
	AND json_array_length(` + alias + `.capabilities) > 0 AND NOT EXISTS (SELECT 1 FROM json_each(` + alias + `.capabilities) c WHERE c.type <> 'text' OR c.value NOT IN ('text','code','vision','reasoning','function_call','responses','image','video','audio','embedding','rerank'))
	AND ` + alias + `.protocols IN ('["OpenAI"]','["Anthropic"]','["Gemini"]'))`
}
