package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"starhub/common"
	"starhub/relay/channel"
)

func TestStatusWriterCapturesFirstStatus(t *testing.T) {
	rec := httptest.NewRecorder()
	sw := newStatusWriter(rec)

	if sw.status != http.StatusOK || sw.wrote {
		t.Fatalf("fresh writer = {status:%d wrote:%v}, want {200 false}", sw.status, sw.wrote)
	}

	sw.WriteHeader(http.StatusAccepted)
	if sw.status != http.StatusAccepted || !sw.wrote {
		t.Fatalf("after WriteHeader(202) = {status:%d wrote:%v}", sw.status, sw.wrote)
	}
	// A second WriteHeader must not overwrite the recorded status (and Go's
	// own recorder ignores it too), so the metric classifier keeps the real
	// first status.
	sw.WriteHeader(http.StatusInternalServerError)
	if sw.status != http.StatusAccepted {
		t.Fatalf("status = %d after a second WriteHeader, want the first one 202", sw.status)
	}
}

func TestStatusWriterWriteImpliesOK(t *testing.T) {
	rec := httptest.NewRecorder()
	sw := newStatusWriter(rec)

	if _, err := sw.Write([]byte("hello")); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if sw.status != http.StatusOK {
		t.Fatalf("status = %d, want 200 after an implicit Write", sw.status)
	}
	if rec.Body.String() != "hello" {
		t.Fatalf("body = %q, want it forwarded", rec.Body.String())
	}
}

func TestStatusWriterFlushDelegates(t *testing.T) {
	rec := httptest.NewRecorder()
	sw := newStatusWriter(rec)
	sw.Flush() // httptest.ResponseRecorder implements http.Flusher.
	if !rec.Flushed {
		t.Fatal("Flush was not delegated to the wrapped writer")
	}
}

func TestUpstreamRequestIDHeaderPriority(t *testing.T) {
	cases := []struct {
		name    string
		headers map[string]string
		want    string
	}{
		{"x-request-id wins", map[string]string{
			"X-Request-Id": "first", "Request-Id": "second", "CF-RAY": "third",
		}, "first"},
		{"request-id beats cf-ray", map[string]string{
			"Request-Id": "second", "CF-RAY": "third",
		}, "second"},
		{"aws header", map[string]string{"X-Amzn-Requestid": "aws-id"}, "aws-id"},
		{"cf-ray last", map[string]string{"CF-RAY": "third"}, "third"},
		{"none present", map[string]string{}, ""},
	}
	for _, tc := range cases {
		resp := &http.Response{Header: http.Header{}}
		for k, v := range tc.headers {
			resp.Header.Set(k, v)
		}
		if got := upstreamRequestID(resp); got != tc.want {
			t.Fatalf("%s: upstreamRequestID = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestRelayRequestFromContext(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx := common.WithValues(r.Context(),
		common.CtxUserID, int64(7),
		common.CtxTokenID, int64(11),
		common.CtxTokenName, "prod-key",
		common.CtxTokenUnlimited, true,
		common.CtxUsingGroup, "vip",
		common.CtxUserGroup, "default",
		common.CtxAutoGroups, []string{"a", "b"},
		common.CtxRequestModel, "gpt-4o",
		common.CtxRequestStream, true,
		common.CtxRequestContentType, "application/json",
	)
	req := relayRequestFromContext(r.WithContext(ctx))

	if req.userID != 7 || req.tokenID != 11 || req.tokenName != "prod-key" {
		t.Fatalf("identity fields = %+v", req)
	}
	if !req.tokenUnlimited {
		t.Fatal("tokenUnlimited was not carried through")
	}
	if req.group != "vip" || req.userGroup != "default" {
		t.Fatalf("groups = %q/%q, want vip/default", req.group, req.userGroup)
	}
	if len(req.autoGroups) != 2 || req.autoGroups[0] != "a" || req.autoGroups[1] != "b" {
		t.Fatalf("autoGroups = %v", req.autoGroups)
	}
	if req.modelName != "gpt-4o" || !req.stream || req.contentType != "application/json" {
		t.Fatalf("request fields = %+v", req)
	}
	if req.requestedModel != "gpt-4o" {
		t.Fatalf("legacy context requested model = %q, want gpt-4o", req.requestedModel)
	}
}

// A missing context value must degrade to the zero value rather than panic:
// relay handlers read these before the middleware chain has necessarily
// populated every key.
func TestRelayRequestFromEmptyContext(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req := relayRequestFromContext(r)

	if req.userID != 0 || req.tokenID != 0 || req.group != "" || req.stream {
		t.Fatalf("zero-value request = %+v", req)
	}
}

func TestBillingGroupPrefersUsingGroupThenAutoGroups(t *testing.T) {
	base := relayRequest{userGroup: "default", group: "vip"}
	if got := base.billingGroup(); got != "vip" {
		t.Fatalf("billingGroup = %q, want the using group vip", got)
	}

	// Under the auto group the first configured fallback is what gets billed.
	auto := relayRequest{userGroup: "default", group: common.AutoGroup, autoGroups: []string{"cheap", "pricey"}}
	if got := auto.billingGroup(); got != "cheap" {
		t.Fatalf("billingGroup = %q, want the first auto group cheap", got)
	}

	// With the auto group but no configured sequence the using group passes
	// through unchanged; the middleware rejects such a token before the
	// relay is reached, so this is a defensive fallback rather than a
	// billing decision.
	empty := relayRequest{userGroup: "default", group: common.AutoGroup}
	if got := empty.billingGroup(); got != common.AutoGroup {
		t.Fatalf("billingGroup = %q, want the using group %q", got, common.AutoGroup)
	}
}

func TestCtxValueHelpersTolerateWrongTypes(t *testing.T) {
	ctx := common.WithValues(context.Background(),
		common.CtxUserID, "not-an-int64",
		common.CtxUsingGroup, 42)

	if got := ctxValueInt64(ctx, common.CtxUserID); got != 0 {
		t.Fatalf("ctxValueInt64 = %d, want 0 for a mistyped value", got)
	}
	if got := ctxValueString(ctx, common.CtxUsingGroup); got != "" {
		t.Fatalf("ctxValueString = %q, want empty for a mistyped value", got)
	}
	if got := ctxValueBool(ctx, common.CtxTokenUnlimited); got {
		t.Fatal("ctxValueBool = true for an absent key")
	}
	if got := ctxValueInt64(context.Background(), common.CtxUserID); got != 0 {
		t.Fatalf("ctxValueInt64 on an empty context = %d, want 0", got)
	}
}

// The model listing is a plain OpenAI-style envelope, not the management API
// success wrapper.
func TestListModelsShape(t *testing.T) {
	newTestDB(t)
	mustChannel(t, "main", "gpt-4o,claude-3", "sk-up", common.ChannelStatusEnabled)

	r := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	r = r.WithContext(common.WithValues(r.Context(), common.CtxUsingGroup, common.DefaultGroup))
	rr := httptest.NewRecorder()
	ListModels(rr, r)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rr.Code, rr.Body.String())
	}
	var got struct {
		Object string `json:"object"`
		Data   []struct {
			ID      string `json:"id"`
			Object  string `json:"object"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}
	decodeJSON(t, rr.Body.Bytes(), &got)
	if got.Object != "list" {
		t.Fatalf("object = %q, want list", got.Object)
	}
	if len(got.Data) != 2 {
		t.Fatalf("data has %d models, want 2", len(got.Data))
	}
	for _, m := range got.Data {
		if m.Object != "model" || m.OwnedBy != "starhub" {
			t.Fatalf("entry shape = %+v", m)
		}
	}
	if got.Data[0].ID != "claude-3" || got.Data[1].ID != "gpt-4o" {
		t.Fatalf("models = %v, want the sorted claude-3, gpt-4o", got.Data)
	}
}

// A group with no channels yields an empty list rather than an error.
func TestListModelsEmptyGroup(t *testing.T) {
	newTestDB(t)

	r := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	r = r.WithContext(common.WithValues(r.Context(), common.CtxUsingGroup, "no-such-group"))
	rr := httptest.NewRecorder()
	ListModels(rr, r)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var got struct {
		Data []any `json:"data"`
	}
	decodeJSON(t, rr.Body.Bytes(), &got)
	if len(got.Data) != 0 {
		t.Fatalf("data = %v, want empty", got.Data)
	}
}

// The image endpoints read the body from the relay context, so a truncated or
// non-JSON body must be a 400 before any upstream call is attempted.
func TestRelayImageGenerationsRejectsInvalidBody(t *testing.T) {
	newTestDB(t)

	r := httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	r = r.WithContext(common.WithValues(r.Context(),
		common.CtxRequestBody, []byte(`{"prompt":`),
		common.CtxRequestModel, "dall-e",
		common.CtxUserID, int64(1),
		common.CtxTokenID, int64(1),
	))
	rr := httptest.NewRecorder()
	RelayImageGenerations(rr, r)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (%s)", rr.Code, rr.Body.String())
	}
}

func TestEstimateAudioTokens(t *testing.T) {
	cases := []struct {
		bytes int64
		want  int64
	}{
		{0, 1},
		{-10, 1},
		{1, 1},
		{640, 1},
		{6400, 10},
	}
	for _, tc := range cases {
		if got := estimateAudioTokens(tc.bytes); got != tc.want {
			t.Fatalf("estimateAudioTokens(%d) = %d, want %d", tc.bytes, got, tc.want)
		}
	}
}

func TestModeLabel(t *testing.T) {
	cases := []struct {
		mode channel.RelayMode
		want string
	}{
		{channel.RelayModeEmbeddings, "embeddings"},
		{channel.RelayModeRerank, "rerank"},
		{channel.RelayModeTTS, "tts"},
		{channel.RelayModeSTT, "stt"},
		{channel.RelayModeSTTTranslation, "stt"},
		{channel.RelayModeImageGeneration, "image"},
		{channel.RelayModeImageEdit, "image"},
		{channel.RelayModeChat, "passthrough"},
	}
	for _, tc := range cases {
		if got := modeLabel(tc.mode); got != tc.want {
			t.Fatalf("modeLabel(%d) = %q, want %q", tc.mode, got, tc.want)
		}
	}
}
