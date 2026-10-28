package common

import "context"

type ctxKey string

const (
	CtxUsingGroup   ctxKey = "using_group"
	CtxRequestModel ctxKey = "request_model"
	// CtxRequestedModel retains the client model before routing rewrites it.
	CtxRequestedModel     ctxKey = "requested_model"
	CtxRequestBody        ctxKey = "request_body"
	CtxRequestStream      ctxKey = "request_stream"
	CtxRequestContentType ctxKey = "request_content_type"
	CtxUserID             ctxKey = "user_id"
	CtxRole               ctxKey = "role"
	CtxTokenID            ctxKey = "token_id"
	CtxTokenName          ctxKey = "token_name"
	CtxTokenUnlimited     ctxKey = "token_unlimited"
	// CtxAutoGroups carries the token's auto-group sequence when the using
	// group is "auto"; the relay flow walks it group by group.
	CtxAutoGroups ctxKey = "auto_groups"
	// CtxUserGroup is the user's own group (for group_group_ratio lookup).
	CtxUserGroup ctxKey = "user_group"
	// CtxTokenModelLimits carries the token's model whitelist (comma-separated)
	// so the distributor can enforce it once the model name is parsed.
	CtxTokenModelLimits ctxKey = "token_model_limits"
	// CtxRequestPrefixHash carries a stable hash of the prompt prefix so
	// prefix-cache affinity can keep a conversation on the same upstream.
	// Empty when prefix affinity is disabled or no prefix could be derived.
	CtxRequestPrefixHash ctxKey = "request_prefix_hash"
	// CtxSmartChannelCheck carries trusted routing constraints into every retry.
	CtxSmartChannelCheck ctxKey = "smart_channel_check"
)

func WithValues(ctx context.Context, kv ...any) context.Context {
	for i := 0; i+1 < len(kv); i += 2 {
		ctx = context.WithValue(ctx, kv[i], kv[i+1])
	}
	return ctx
}
