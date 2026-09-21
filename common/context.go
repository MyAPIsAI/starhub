package common

import "context"

type ctxKey string

const (
	CtxUsingGroup         ctxKey = "using_group"
	CtxRequestModel       ctxKey = "request_model"
	CtxRequestBody        ctxKey = "request_body"
	CtxRequestStream      ctxKey = "request_stream"
	CtxRequestContentType ctxKey = "request_content_type"
	CtxUserID             ctxKey = "user_id"
	CtxRole               ctxKey = "role"
	CtxTokenID            ctxKey = "token_id"
	CtxTokenName          ctxKey = "token_name"
	CtxTokenUnlimited     ctxKey = "token_unlimited"
)

func WithValues(ctx context.Context, kv ...any) context.Context {
	for i := 0; i+1 < len(kv); i += 2 {
		ctx = context.WithValue(ctx, kv[i], kv[i+1])
	}
	return ctx
}
