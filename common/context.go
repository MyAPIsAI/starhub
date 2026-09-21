package common

import "context"

type ctxKey string

const (
	CtxUsingGroup      ctxKey = "using_group"
	CtxSelectedChannel ctxKey = "selected_channel"
	CtxRequestModel    ctxKey = "request_model"
	CtxRequestBody     ctxKey = "request_body"
	CtxRequestStream   ctxKey = "request_stream"
	CtxUserID          ctxKey = "user_id"
	CtxRole            ctxKey = "role"
	CtxTokenID         ctxKey = "token_id"
	CtxTokenName       ctxKey = "token_name"
	CtxTokenUnlimited  ctxKey = "token_unlimited"
)

type SelectedChannel struct {
	ID      int64
	Type    int
	Key     string
	BaseURL string
}

func WithValues(ctx context.Context, kv ...any) context.Context {
	for i := 0; i+1 < len(kv); i += 2 {
		ctx = context.WithValue(ctx, kv[i], kv[i+1])
	}
	return ctx
}
