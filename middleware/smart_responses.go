package middleware

import (
	"net/http"

	"starhub/common"
	"starhub/model"
	"starhub/relay"
	"starhub/relay/dto"
)

func readResponsesRequirements(r *http.Request, body []byte, req smartRequirements) (smartRequirements, error) {
	parsed, err := dto.ParseResponsesRequest(body)
	if err != nil {
		return req, err
	}
	if err := relay.ValidateResponsesRequest(parsed); err != nil {
		return req, err
	}
	req.input = max(1, parsed.InputTokens)
	req.output = common.DefaultCompletionBudgetTokens
	if parsed.MaxOutputTokens != nil {
		req.output = *parsed.MaxOutputTokens
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
	if ref, ok := r.Context().Value(common.CtxResponseReference).(*model.ResponseReference); ok {
		req.input += ref.PromptTokens + ref.CompletionTokens
		if ref.HasImages {
			req.capabilities = append(req.capabilities, "vision")
		}
		if ref.HasTools {
			req.capabilities = append(req.capabilities, "function_call")
		}
		if ref.HasReasoning {
			req.capabilities = append(req.capabilities, "reasoning")
		}
	}
	req.continuation = parsed.Continuation
	// Every complete Responses prefix can acquire a pin, including first
	// turns that later return reasoning or tools without an explicit schema.
	if req.chain == "" && (len(parsed.FirstUser) > 0 || parsed.Continuation || parsed.HasTools) {
		req.chain, err = inferToolChain(r.URL.Path, parsed.FirstUser)
	}
	return req, err
}
