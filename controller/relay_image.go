package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"starhub/common"
	"starhub/model"
	"starhub/relay"
	"starhub/relay/channel"
	"starhub/relay/dto"
)

// ---------------------------------------------------------------------------
// Image relay (P3.1): generations (JSON) and edits (multipart), per-image
// billing (size x quality x count), streamed images as SSE.
// ---------------------------------------------------------------------------

// RelayImageGenerations serves POST /v1/images/generations.
func RelayImageGenerations(w http.ResponseWriter, r *http.Request) {
	req := relayRequestFromContext(r)
	imgReq, err := dto.ParseImageRequest(req.body)
	if err != nil {
		writeModeError(w, http.StatusBadRequest, "invalid_request_error", "invalid_request_body", err.Error())
		return
	}
	if imgReq.Stream == nil {
		imgReq.Stream = &req.stream
	}
	relayImage(w, r, req, channel.RelayModeImageGeneration, imgReq, req.body, req.contentType)
}

// RelayImageEdits serves POST /v1/images/edits (multipart). The raw body is
// replayed verbatim on every attempt; the parsed form feeds validation and
// billing only.
func RelayImageEdits(w http.ResponseWriter, r *http.Request) {
	req := relayRequestFromContext(r)
	form, err := common.ParseMultipartFormMeta(req.contentType, req.body)
	if err != nil {
		writeModeError(w, http.StatusBadRequest, "invalid_request_error", "invalid_request_body", err.Error())
		return
	}
	if !form.HasFile("image") {
		writeModeError(w, http.StatusBadRequest, "invalid_request_error", "missing_image",
			"the \"image\" multipart field is required")
		return
	}
	imgReq := &dto.ImageRequest{
		Model:   form.Value("model"),
		Prompt:  form.Value("prompt"),
		Size:    form.Value("size"),
		Quality: form.Value("quality"),
		Extra:   map[string]json.RawMessage{},
	}
	if raw := form.Value("n"); raw != "" {
		n, err := strconv.ParseUint(raw, 10, 32)
		if err != nil {
			writeModeError(w, http.StatusBadRequest, "invalid_request_error", "invalid_request_body",
				fmt.Sprintf("invalid n %q: must be an integer", raw))
			return
		}
		// Same uint shape as the JSON path: a wrapped-around value stays a
		// wrapped number and the [1, MaxImageN] check catches it.
		nVal := uint(n)
		imgReq.N = &nVal
	}
	if raw := form.Value("stream"); raw != "" {
		stream := raw == "true" || raw == "1"
		imgReq.Stream = &stream
	}
	if imgReq.Model == "" {
		writeModeError(w, http.StatusBadRequest, "invalid_request_error", "model_required",
			"the \"model\" form field is required")
		return
	}
	if imgReq.Prompt == "" {
		writeModeError(w, http.StatusBadRequest, "invalid_request_error", "prompt_required",
			"the \"prompt\" form field is required")
		return
	}
	// The multipart n must pass the same bound as the JSON path (both
	// parsing paths share one billing gate, P3.1).
	if err := imgReq.ValidateN(); err != nil {
		writeModeError(w, http.StatusBadRequest, "invalid_request_error", "invalid_request_body", err.Error())
		return
	}
	if err := imgReq.ValidateSize(); err != nil {
		writeModeError(w, http.StatusBadRequest, "invalid_request_error", "invalid_request_body", err.Error())
		return
	}
	// The distributor extracted the model from the form; the image request
	// is authoritative for the rest.
	req.stream = imgReq.Stream != nil && *imgReq.Stream
	relayImage(w, r, req, channel.RelayModeImageEdit, imgReq, req.body, req.contentType)
}

func relayImage(w http.ResponseWriter, r *http.Request, req relayRequest,
	mode channel.RelayMode, imgReq *dto.ImageRequest, outBody []byte, contentType string) {

	stream := imgReq.Stream != nil && *imgReq.Stream
	bill := &relay.ImageBill{
		Model:     imgReq.Model,
		Group:     req.billingGroup(),
		UserGroup: req.userGroup,
		Size:      imgReq.Size,
		Quality:   imgReq.Quality,
		Prompt:    imgReq.Prompt,
		Count:     int(imgReq.Count()),
		Pricing:   req.priceFor(req.billingGroup()),
	}

	pre, err := relay.PreConsumeQuotaAmount(req.userID, req.tokenID, req.tokenUnlimited, bill.PreQuota())
	if err != nil {
		writePreConsumeError(w, "", err)
		return
	}

	flow := newRelayFlow(r.Context(), w, req, "", "image")
	flow.preQuota = pre
	defer flow.refundUnlessSettled()
	defer flow.finish()

	// Image streaming detaches the upstream from the client's request
	// context: the gateway must drain the upstream SSE to EOF even after
	// the client is gone, so the completed-event count (and thus the bill)
	// reflects what the upstream actually delivered. This enforces the two
	// P3.1 billing invariants: a client disconnect must not lower the
	// billed count, and upstream over-delivery must raise it.
	var streamCtx context.Context = flow.ctx
	var streamCancel context.CancelFunc
	if stream {
		streamCtx, streamCancel = context.WithTimeout(context.Background(), 10*time.Minute)
		defer streamCancel()
	}

	flow.runChannels(func(ch *model.Channel) attemptResult {
		adaptor, err := relay.GetChatAdaptor(ch.Type)
		if err != nil {
			return attemptResult{outcome: attemptUpstreamError,
				cause: fmt.Errorf("channel %d: %v", ch.ID, err)}
		}
		url, err := adaptor.ModeURL(ch.BaseURL, mode)
		if err != nil {
			return attemptResult{outcome: attemptUnsupported, cause: err}
		}
		key, keyIdx := flow.selectKey(ch)
		call := upstreamCall{
			method:      http.MethodPost,
			url:         url,
			contentType: contentType,
			body:        outBody,
			key:         key,
			keyIdx:      keyIdx,
			// Image generation and edit are side-effecting: a transport
			// error after the request was sent cannot prove the upstream
			// did not execute it, so such a call is classified as
			// attemptUncertain and is never auto-retried across channels.
			sideEffect: true,
		}
		if stream {
			call.ctx = streamCtx
		}
		resp, fail := flow.callUpstream(ch, adaptor, call)
		if fail != nil {
			return *fail
		}
		if stream {
			return imageStreamAttempt(flow, ch, resp, bill)
		}
		return imageJSONAttempt(flow, ch, resp, bill)
	})
}

// imageJSONAttempt relays a non-stream image response verbatim and settles
// on the delivered count. The body buffer is the only full copy retained;
// billing counts entries without materializing payload strings (P7.1).
func imageJSONAttempt(flow *relayFlow, ch *model.Channel, resp *http.Response, bill *relay.ImageBill) attemptResult {
	body := readImageBody(resp)
	defer releaseImageBody(body)
	resp.Body.Close()
	writePassthroughBody(flow.w, resp, body)

	count := 0
	var usage *dto.ImageUsage
	if urls, b64s, u, err := dto.WalkImageResponse(body, nil); err == nil {
		count = max(urls, b64s)
		usage = u
	}
	bill.Group = flow.settleGroup()
	bill.Pricing = flow.req.priceFor(bill.Group)
	actual := bill.SettleQuota(count, usage)
	settleImageAttempt(flow, ch, actual)
	return attemptResult{outcome: attemptSuccess, statusCode: http.StatusOK}
}

// imageStreamAttempt relays a streamed image response (partial_image
// previews are forwarded free; completed images bill one by one) and
// enforces the two billing invariants of P3.1:
//
//  1. a client disconnect must not lower the billed count — the upstream
//     stream is drained to EOF even after writes fail;
//  2. a completed image arriving after the disconnect must raise it.
//
// Frames are forwarded from the reader's reused contiguous byte buffer.
// Larger frames temporarily use overflow chunks during consolidation;
// forwarding does not add a per-event string or interface conversion.
//
// Upstreams that answer a stream request with plain JSON are wrapped into
// the SSE format (the gateway's protocol promise outranks the upstream's).
func imageStreamAttempt(flow *relayFlow, ch *model.Channel, resp *http.Response, bill *relay.ImageBill) attemptResult {
	isSSE := strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream")
	if !isSSE {
		return imageJSONasSSEAttempt(flow, ch, resp, bill)
	}

	flow.w.Header().Set("Content-Type", "text/event-stream")
	flow.w.Header().Set("Cache-Control", "no-cache")
	flow.w.WriteHeader(http.StatusOK)
	flusher, _ := flow.w.(http.Flusher)

	reader := channel.NewSSEByteReader(resp.Body)
	defer reader.Release()
	defer resp.Body.Close()

	completed := 0
	var streamErr string
	var streamUsage *dto.ImageUsage
	clientGone := false
	writeRaw := func(chunks ...[]byte) {
		if clientGone {
			return
		}
		for _, c := range chunks {
			if len(c) == 0 {
				continue
			}
			if _, werr := flow.w.Write(c); werr != nil {
				clientGone = true
				return
			}
		}
	}
	for {
		line, err := reader.Next()
		if err != nil {
			if err != io.EOF {
				log.Printf("image stream read error: %v", err)
			}
			break
		}
		if len(line.Event) > 0 {
			writeRaw(sseEventPrefix, line.Event, sseNewline)
		}
		if len(line.Data) > 0 {
			if flow.attempt != nil {
				flow.attempt.RecordFirstToken()
			}
			writeRaw(sseDataPrefix, line.Data, sseNewline)
		}
		writeRaw(sseNewline)
		if flusher != nil {
			flusher.Flush()
		}
		if len(line.Data) == 0 {
			continue
		}
		ev, perr := dto.ParseImageStreamEventMeta(line.Event, line.Data)
		if perr != nil {
			continue
		}
		if ev.Usage != nil {
			streamUsage = ev.Usage
		}
		switch {
		case ev.Type == "image_generation.completed":
			completed++
		case ev.Type == "error" && ev.ErrorMessage != "":
			streamErr = ev.ErrorMessage
		}
	}

	if clientGone {
		flow.span.MarkStreamInterrupted()
	}
	// Nothing delivered and the stream errored: nothing settles, the
	// deferred refund returns the whole pre-payment.
	if completed == 0 && streamErr != "" {
		return attemptResult{outcome: attemptSuccess, statusCode: http.StatusOK}
	}
	bill.Group = flow.settleGroup()
	bill.Pricing = flow.req.priceFor(bill.Group)
	actual := bill.SettleQuota(completed, streamUsage)
	settleImageAttempt(flow, ch, actual)
	return attemptResult{outcome: attemptSuccess, statusCode: http.StatusOK}
}

// SSE write segments for imageStreamAttempt (byte-level framing identical
// to the previous fmt.Fprintf output).
var (
	sseEventPrefix = []byte("event: ")
	sseDataPrefix  = []byte("data: ")
	sseNewline     = []byte("\n")
)

// imageJSONasSSEAttempt wraps a JSON image response into SSE completed
// events (one per image) when the client asked for a stream but the
// upstream does not stream. Frames are written straight from the raw JSON
// values, so beyond the body itself at most one entry's payload is alive
// at a time (P7.1).
func imageJSONasSSEAttempt(flow *relayFlow, ch *model.Channel, resp *http.Response, bill *relay.ImageBill) attemptResult {
	body := readImageBody(resp)
	defer releaseImageBody(body)
	resp.Body.Close()

	// Count first: an unparseable or payload-less response passes
	// through unchanged.
	urls, b64s, usage, err := dto.WalkImageResponse(body, nil)
	if err != nil || max(urls, b64s) == 0 {
		// Unparseable / empty: forward the raw JSON unchanged (not SSE) —
		// better a working answer in the wrong shape than data loss.
		writePassthroughBody(flow.w, resp, body)
		bill.Group = flow.settleGroup()
		bill.Pricing = flow.req.priceFor(bill.Group)
		actual := bill.SettleQuota(0, nil)
		settleImageAttempt(flow, ch, actual)
		return attemptResult{outcome: attemptSuccess, statusCode: http.StatusOK}
	}
	count := max(urls, b64s)

	flow.w.Header().Set("Content-Type", "text/event-stream")
	flow.w.Header().Set("Cache-Control", "no-cache")
	flow.w.WriteHeader(http.StatusOK)
	flusher, _ := flow.w.(http.Flusher)
	_, _, _, _ = dto.WalkImageResponse(body, func(e dto.ImageEntryRaw) error {
		_, _ = flow.w.Write(imageCompletedHead)
		_, _ = flow.w.Write(dto.RawStringLiteral(e.B64))
		_, _ = flow.w.Write(imageCompletedMid)
		_, _ = flow.w.Write(dto.RawStringLiteral(e.URL))
		_, _ = flow.w.Write(imageCompletedTail)
		if flusher != nil {
			flusher.Flush()
		}
		return nil
	})
	if usage != nil {
		payload, _ := json.Marshal(map[string]any{"usage": usage})
		fmt.Fprintf(flow.w, "data: %s\n\n", payload)
		if flusher != nil {
			flusher.Flush()
		}
	}
	fmt.Fprint(flow.w, "data: [DONE]\n\n")
	if flusher != nil {
		flusher.Flush()
	}
	bill.Group = flow.settleGroup()
	bill.Pricing = flow.req.priceFor(bill.Group)
	actual := bill.SettleQuota(count, usage)
	settleImageAttempt(flow, ch, actual)
	return attemptResult{outcome: attemptSuccess, statusCode: http.StatusOK}
}

const maxPooledImageBody = 4 << 20

// A small shared cache avoids repeated megabyte allocations across HTTP
// workers while placing a fixed bound on retained image response storage.
var imageBodyPool = make(chan []byte, 4)

func readImageBody(resp *http.Response) []byte {
	var buf []byte
	select {
	case buf = <-imageBodyPool:
	default:
		buf = make([]byte, 0, 64*1024)
	}
	return readBoundedBodyInto(resp, maxModeResponseBytes, buf)
}

func releaseImageBody(body []byte) {
	if cap(body) > maxPooledImageBody {
		return
	}
	body = body[:0]
	select {
	case imageBodyPool <- body:
	default:
	}
}

// Frame segments of the synthesized image_generation.completed events.
var (
	imageCompletedHead = []byte("event: image_generation.completed\ndata: {\"type\":\"image_generation.completed\",\"output_index\":0,\"b64_json\":")
	imageCompletedMid  = []byte(",\"url\":")
	imageCompletedTail = []byte("}\n\n")
)

// settleImageAttempt balances the image bill (unit-based: the amount was
// computed by ImageBill with the count/usage guards).
func settleImageAttempt(flow *relayFlow, ch *model.Channel, actual int64) {
	if err := relay.PostConsumeQuotaAmountWithMetadata(flow.req.userID, flow.req.tokenID, flow.req.tokenName,
		flow.req.tokenUnlimited, ch.ID, flow.req.modelName, actual, flow.preQuota, flow.trailString(), time.Since(flow.startTime).Milliseconds(), 1, flow.req.requestedModel, flow.req.protocol); err != nil {
		log.Printf("post consume image quota failed: %v", err)
	}
	flow.settled = true
}
