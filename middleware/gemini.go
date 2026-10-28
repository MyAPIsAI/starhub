package middleware

import (
	"net/http"
	"strings"

	"starhub/common"
	"starhub/relay"
	"starhub/relay/dto"
)

func isGeminiPath(path string) bool {
	return strings.HasPrefix(path, "/v1beta/models/")
}

func writeRelayRequestError(w http.ResponseWriter, r *http.Request, status int, errType, code, message string) {
	if isGeminiPath(r.URL.Path) {
		dto.WriteRelayError(w, status, dto.InboundGemini, errType, code, message)
		return
	}
	common.RelayError(w, status, errType, code, message)
}

func readGeminiRequirements(r *http.Request, body []byte, req smartRequirements) (smartRequirements, error) {
	parsed, err := dto.ParseGeminiRequest(body)
	if err != nil {
		return req, err
	}
	if err := relay.ValidateGeminiRequest(parsed); err != nil {
		return req, err
	}
	req.input = max(1, parsed.InputTokens)
	req.output = common.DefaultCompletionBudgetTokens
	if parsed.GenerationConfig.MaxOutputTokens != nil {
		req.output = *parsed.GenerationConfig.MaxOutputTokens
	}
	if parsed.HasImages {
		req.capabilities = append(req.capabilities, "vision")
	}
	if parsed.HasTools {
		req.capabilities = append(req.capabilities, "function_call")
	}
	if parsed.HasReasoning {
		req.capabilities = append(req.capabilities, "reasoning")
	}
	req.path = "/v1beta/models"
	req.capabilities = append(req.capabilities, "gemini")
	req.continuation = parsed.Continuation
	if req.chain == "" {
		req.chain, err = inferToolChain(req.path, parsed.FirstUser)
	}
	return req, err
}
