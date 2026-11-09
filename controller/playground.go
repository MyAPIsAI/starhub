package controller

import (
	"bytes"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"strings"
	"time"

	"gorm.io/gorm"
	"starhub/common"
	"starhub/model"
	"starhub/observability"
	"starhub/relay"
)

func (f *relayFlow) setUsageMetadata(usage *relay.Usage) {
	f.observedUpstreamEnd = time.Now().UnixMilli()
	usage.CallType = f.req.callType
	usage.TraceID = f.traceID
	usage.RequestSource = f.req.requestSource
	usage.TTFTMs = f.ttftMs
	observability.From(f.ctx).Usage(f.ttftMs, usage.RequestStatus == 2, usage.RequestStatus == 3)
	entry := &model.Log{ChannelID: f.lastChannelID, Model: f.req.modelName, ServiceGroup: f.settleGroup()}
	model.FreezeRoutingEvidence(f.ctx, entry, f.affinityPreferred != 0)
	usage.ResourceID, usage.ResourceIDs = entry.ResourceID, entry.ResourceIDs
	usage.SnapshotSequence, usage.ResourceRevision = entry.SnapshotSequence, entry.ResourceRevision
	usage.AffinityUsed, usage.RoutingRecorded = entry.AffinityUsed, entry.RoutingRecorded
}

// GetPlaygroundResult returns only settled metadata owned by the current account.
func GetPlaygroundResult(w http.ResponseWriter, r *http.Request) {
	trace := r.PathValue("trace")
	if len(trace) != 64 || strings.Trim(trace, "0123456789abcdef") != "" {
		common.ApiError(w, 404, "request not found")
		return
	}
	var entry model.Log
	err := model.DB.Where("trace_id = ? AND user_id = ? AND request_source = ?", trace, requestUserID(r), "playground").First(&entry).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		common.ApiError(w, 404, "request not found")
		return
	}
	if err != nil {
		common.ApiError(w, 500, "request metadata unavailable")
		return
	}
	vendor, display := model.InferVendor(entry.Model)
	if meta, _, err := model.GetModelMeta(entry.Model); err == nil && meta != nil {
		if meta.Vendor != "" {
			vendor = meta.Vendor
		}
		if meta.VendorDisplay != "" {
			display = meta.VendorDisplay
		}
	}
	common.ApiSuccess(w, map[string]any{
		"trace_id": entry.TraceID, "key_id": entry.TokenID, "requested_model": entry.RequestedModel,
		"actual_model": entry.Model, "vendor": vendor, "vendor_display": display,
		"fallback": strings.Contains(entry.UsedChannels, "->"), "duration_ms": entry.DurationMs,
		"ttft_ms": entry.TTFTMs, "input_tokens": entry.PromptTokens, "output_tokens": entry.CompletionTokens,
		"cost_nano": new(big.Int).Mul(big.NewInt(entry.Quota), big.NewInt(1_000_000_000/common.QuotaPerUSD)).String(), "status": entry.Status, "source": "playground",
	})
}

// Timing inspects transient response frames and never retains their contents.
type playgroundTimingWriter struct {
	http.ResponseWriter
	flow   *relayFlow
	buffer []byte
}

func (w *playgroundTimingWriter) Write(body []byte) (int, error) {
	if w.flow.ttftMs == 0 && len(w.buffer)+len(body) <= 1<<20 {
		w.buffer = append(w.buffer, body...)
		if strings.HasPrefix(w.Header().Get("Content-Type"), "text/event-stream") {
			for {
				i := bytes.IndexByte(w.buffer, '\n')
				if i < 0 {
					break
				}
				line := bytes.TrimSpace(w.buffer[:i])
				w.buffer = w.buffer[i+1:]
				w.observe(bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:"))))
			}
		} else {
			w.observe(w.buffer)
		}
	}
	return w.ResponseWriter.Write(body)
}

func (w *playgroundTimingWriter) observe(body []byte) {
	var value struct {
		Choices []struct {
			Delta   struct{ Content string }
			Message struct{ Content string }
		}
		Delta      struct{ Text string }
		Content    []struct{ Text string }
		Candidates []struct {
			Content struct{ Parts []struct{ Text string } }
		}
	}
	if json.Unmarshal(body, &value) != nil {
		return
	}
	hasText := value.Delta.Text != ""
	for _, choice := range value.Choices {
		hasText = hasText || choice.Delta.Content != "" || choice.Message.Content != ""
	}
	for _, part := range value.Content {
		hasText = hasText || part.Text != ""
	}
	for _, candidate := range value.Candidates {
		for _, part := range candidate.Content.Parts {
			hasText = hasText || part.Text != ""
		}
	}
	if hasText && w.flow.ttftMs == 0 {
		w.flow.ttftMs = max(1, time.Since(w.flow.startTime).Milliseconds())
		w.buffer = nil
	}
}

func (w *playgroundTimingWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
