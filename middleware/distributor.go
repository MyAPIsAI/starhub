package middleware

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"hash/fnv"
	"io"
	"mime"
	"net/http"
	"strings"

	"starhub/common"
	"starhub/model"
	"starhub/relay/dto"
	"starhub/setting"
)

const maxRequestBodySize = 32 << 20

// affinityPrefixMaxBytes caps how much of the leading prompt is hashed for
// prefix-cache affinity: enough to fingerprint a conversation, small enough
// to stay cheap on large bodies.
const affinityPrefixMaxBytes = 1 << 10

// chatMessage is the subset of an OpenAI/Claude chat message the prefix hash
// needs. Content is kept raw so both string and multimodal array shapes hash
// deterministically.
type chatMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

// Distribute parses and caches the relay request body so the controller
// can replay it on every retry attempt. JSON bodies are probed for the
// model/stream fields; multipart bodies (audio transcriptions, image
// edits) keep their raw bytes + content-type and expose the model from the
// form. inbound selects the error envelope format of the endpoint.
func Distribute(inbound dto.InboundFormat) func(http.Handler) http.Handler {
	return distribute(inbound, "")
}

// DistributeWithDefaultModel is Distribute with a model fallback for
// endpoints whose protocol tolerates a missing model (P3.1: image
// generations defaults to the legacy "dall-e"; image edits extract the
// model from the multipart form and need no fallback).
func DistributeWithDefaultModel(inbound dto.InboundFormat, defaultModel string) func(http.Handler) http.Handler {
	return distribute(inbound, defaultModel)
}

func distribute(inbound dto.InboundFormat, defaultModel string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fail := func(status int, code, message string) {
				dto.WriteRelayError(w, status, inbound, "invalid_request_error", code, message)
			}
			if inbound == "" {
				inbound = dto.InboundOpenAI
			}

			body, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBodySize+1))
			if err != nil {
				fail(http.StatusBadRequest, "read_body_failed",
					"failed to read request body: "+err.Error())
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
			if len(body) > maxRequestBodySize {
				fail(http.StatusRequestEntityTooLarge, "body_too_large",
					"request body exceeds the size limit")
				return
			}

			contentType := r.Header.Get("Content-Type")
			group, _ := r.Context().Value(common.CtxUsingGroup).(string)
			if group == "" {
				dto.WriteRelayError(w, http.StatusInternalServerError, inbound,
					"server_error", "internal_error", "using group is missing from request context")
				return
			}

			var modelName string
			var stream bool
			var prefixHash string
			if isMultipart(contentType) {
				// Metadata-only parse: the cached raw body stays the only
				// in-memory copy of uploaded files (P7.1).
				form, err := common.ParseMultipartFormMeta(contentType, body)
				if err != nil {
					fail(http.StatusBadRequest, "invalid_multipart", err.Error())
					return
				}
				modelName = form.Value("model")
				if raw := form.Value("stream"); raw != "" {
					stream = raw == "true" || raw == "1"
				}
			} else {
				var lite struct {
					Model  string `json:"model"`
					Stream bool   `json:"stream"`
				}
				if err := json.Unmarshal(body, &lite); err != nil {
					fail(http.StatusBadRequest, "invalid_json",
						"request body is not valid JSON: "+err.Error())
					return
				}
				modelName = lite.Model
				stream = lite.Stream
				if setting.RoutingAffinityPrefixCacheEnabled() {
					prefixHash = chatPrefixHashFromBody(body)
				}
			}
			if modelName == "" && defaultModel != "" {
				modelName = defaultModel
			}
			if modelName == "" {
				fail(http.StatusBadRequest, "model_required",
					"the \"model\" field is required")
				return
			}

			requestedModel := modelName
			autoModelSelected := false
			var channelCheck func(*model.Channel, string) (bool, error)
			// Auto-routing: for the canonical entry or its legacy alias, replace
			// it with the lowest-cost available model within the token's
			// authorized scope (group + model whitelist), so the relay can
			// proceed as if the user had chosen that model explicitly.
			if isAutoModel(modelName) {
				requirements, err := readSmartRequirements(r, body, isMultipart(contentType))
				if err != nil {
					fail(http.StatusBadRequest, "invalid_routing_requirements", err.Error())
					return
				}
				best, err := chooseSmartModel(r, requirements)
				if err != nil {
					dto.WriteRelayError(w, http.StatusInternalServerError, inbound, "server_error", "internal_error", "failed to evaluate smart routing candidates")
					return
				}
				if best != "" {
					modelName = best
					autoModelSelected = true
					channelCheck = func(ch *model.Channel, group string) (bool, error) {
						return requirements.compatible(best, group, ch)
					}
				}
			}

			// Token model whitelist: a sub-key can be restricted to a subset of
			// models. The limits string is set by TokenAuth; empty means no limit.
			if limits, _ := r.Context().Value(common.CtxTokenModelLimits).(string); limits != "" {
				if !modelInList(modelName, model.SplitList(limits)) {
					fail(http.StatusForbidden, "model_not_allowed",
						"the model is not in the token's model whitelist")
					return
				}
			}

			// Never use the virtual entry as an upstream fallback model.
			if isAutoModel(modelName) {
				dto.WriteRelayError(w, http.StatusServiceUnavailable, inbound, "server_error", "no_available_channel",
					"no available candidate model for smart routing")
				return
			}

			if autoModelSelected && isMultipart(contentType) {
				body, err = rewriteMultipartModel(contentType, body, modelName)
				if err != nil {
					fail(http.StatusBadRequest, "invalid_multipart", "failed to update the routed multipart model")
					return
				}
				r.Body = io.NopCloser(bytes.NewReader(body))
				r.ContentLength = int64(len(body))
			}
			if autoModelSelected && !isMultipart(contentType) {
				// Keep passthrough bodies and parsed IR aligned with routing.
				// Raw fields preserve extension data and numeric precision.
				var fields map[string]json.RawMessage
				if err := json.Unmarshal(body, &fields); err != nil {
					fail(http.StatusBadRequest, "invalid_json", "failed to parse request body for model routing")
					return
				}
				fields["model"], _ = json.Marshal(modelName)
				body, err = json.Marshal(fields)
				if err != nil {
					dto.WriteRelayError(w, http.StatusInternalServerError, inbound, "server_error", "internal_error",
						"failed to update the routed request model")
					return
				}
				r.Body = io.NopCloser(bytes.NewReader(body))
				r.ContentLength = int64(len(body))
			}

			// Channel selection is left to the relay controller: it retries the
			// request on alternative channels, and every attempt needs a fresh
			// pick. The body stays cached in the context so each attempt can
			// replay it.
			ctx := common.WithValues(r.Context(),
				common.CtxRequestModel, modelName,
				common.CtxRequestedModel, requestedModel,
				common.CtxRequestBody, body,
				common.CtxRequestContentType, contentType,
				common.CtxRequestStream, stream,
				common.CtxRequestPrefixHash, prefixHash)
			if channelCheck != nil {
				ctx = common.WithValues(ctx, common.CtxSmartChannelCheck, channelCheck)
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// isAutoModel recognizes both virtual entries; neither is an upstream model.
func isAutoModel(name string) bool {
	return name == "myapis-auto" || name == "veyra-auto"
}

func isMultipart(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return strings.HasPrefix(strings.ToLower(contentType), "multipart/")
	}
	return strings.HasPrefix(strings.ToLower(mediaType), "multipart/")
}

// modelInList reports whether the model name is in the whitelist. The check
// is exact (no wildcard / prefix matching) to keep the semantics predictable.
func modelInList(model string, list []string) bool {
	for _, m := range list {
		if m == model {
			return true
		}
	}
	return false
}

// chatPrefixHashFromBody derives a stable, non-reversible key from the leading
// system prompt plus the first user message in a chat request body: the part of
// a conversation most likely to be served from an upstream prompt cache. It
// returns "" when no prefix could be extracted so affinity degrades to
// channel-level stickiness. Only called when prefix-cache affinity is enabled,
// so the extra parse never taxes the common path.
func chatPrefixHashFromBody(body []byte) string {
	var lite struct {
		System   json.RawMessage `json:"system"`
		Messages []chatMessage   `json:"messages"`
	}
	if err := json.Unmarshal(body, &lite); err != nil {
		return ""
	}
	h := fnv.New64a()
	hashed := false
	write := func(raw json.RawMessage) {
		if len(raw) == 0 {
			return
		}
		var s string
		if json.Unmarshal(raw, &s) == nil {
			if len(s) > affinityPrefixMaxBytes {
				s = s[:affinityPrefixMaxBytes]
			}
			_, _ = h.Write([]byte(s))
			hashed = true
			return
		}
		var buf bytes.Buffer
		if json.Compact(&buf, raw) == nil {
			b := buf.Bytes()
			if len(b) > affinityPrefixMaxBytes {
				b = b[:affinityPrefixMaxBytes]
			}
			_, _ = h.Write(b)
			hashed = true
		}
	}
	write(lite.System)
	for _, m := range lite.Messages {
		if m.Role == "user" || m.Role == "system" {
			write(m.Content)
			break
		}
	}
	if !hashed {
		return ""
	}
	var out [8]byte
	binary.LittleEndian.PutUint64(out[:], h.Sum64())
	return hex.EncodeToString(out[:])
}
