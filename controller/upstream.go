package controller

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"starhub/common"
	"starhub/internal/upstream"
	"starhub/middleware"
	"starhub/model"
	"starhub/service"
)

type relayProtection struct {
	attempt      *upstream.Attempt
	verdict      upstream.Verdict
	status       int
	sent, unsafe bool
}

func (f *relayFlow) recordProtectedSkips(ids []int64) {
	if f.protectedSkips == nil {
		f.protectedSkips = map[string]bool{}
	}
	events := []upstream.Event{}
	for _, id := range ids {
		key := fmt.Sprintf("%d:%s", id, f.req.modelName)
		if f.protectedSkips[key] {
			continue
		}
		f.protectedSkips[key] = true
		events = append(events, upstream.Event{ID: uuid.NewString(), RequestID: middleware.CtxRequestIDValue(f.ctx), ChannelID: id, Model: f.req.modelName, Kind: "skip", Class: "resource_protected", Decision: "not_sent", Created: time.Now().Unix(), Started: time.Now().UnixMilli()})
	}
	if len(events) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(f.ctx), 3*time.Second)
	defer cancel()
	if err := model.DB.WithContext(ctx).CreateInBatches(events, 100).Error; err != nil {
		upstream.PersistenceFailed()
		log.Printf("upstream admission event persistence failed: %v", err)
	}
}

func (f *relayFlow) observeUpstream(res attemptResult) (bool, string) {
	p := upstream.CurrentPolicy()
	v := f.protection.verdict
	if v.Class == "" {
		switch {
		case f.sw.responseCompleted || res.outcome == attemptSuccess:
			v.Class = "success"
		case !f.protection.sent:
			v.Class = "local_error"
		case f.sw.writeError != nil || f.ctx.Err() != nil:
			v.Class = "client_disconnected"
		case f.protection.status >= 200 && f.protection.status < 300:
			if observation := middleware.RequestObservation(f.ctx); observation != nil && observation.Stage == "billing" {
				v.Class = "local_error"
			} else {
				v = upstream.Verdict{Class: "protocol_failure", Scope: "interface", Retry: true}
			}
		default:
			v.Class = "unknown"
		}
	}
	observation := middleware.RequestObservation(f.ctx)
	if observation != nil && observation.Entry.FailureStage == "billing" && v.Class != "success" {
		v = upstream.Verdict{Class: "local_error"}
	}
	retry, reason := upstream.RetryReason(p, v, upstream.RetryInput{
		Cancelled: f.ctx.Err() != nil, Committed: f.sw.wrote, Settled: f.settled,
		Strict:   f.affinityRuleUsed && f.affinityMeta.SessionMode == "strict",
		Skip:     f.affinityRuleUsed && f.affinityMeta.SkipRetry && f.affinityPreferred > 0,
		Unsafe:   f.protection.unsafe || res.outcome == attemptUncertain,
		Attempts: f.sent, Elapsed: time.Since(f.startTime),
	})
	if v.Class == "success" {
		retry, reason = false, "completed"
	}
	if f.protection.attempt != nil {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(f.ctx), 3*time.Second)
		defer cancel()
		err := upstream.Record(model.DB.WithContext(ctx), f.protection.attempt, v, upstream.Event{
			RequestID: middleware.CtxRequestIDValue(f.ctx), Status: f.protection.status, Sent: f.protection.sent, Retry: retry, Decision: reason,
		}, p)
		if err != nil {
			log.Printf("upstream protection persistence failed: %v", err)
		}
	}
	f.protection.verdict = v
	if f.protection.sent {
		f.lastUpstream = f.protection
	}
	return retry, reason
}

func (f *relayFlow) upstreamTerminal() bool {
	if f.lastUpstream.status < 400 {
		return false
	}
	status := f.lastUpstream.status
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		status = http.StatusBadGateway
	}
	if delay := f.lastUpstream.verdict.Delay; delay > 0 {
		f.w.Header().Set("Retry-After", strconv.Itoa(delay))
	}
	middleware.SetRequestFailure(f.ctx, "upstream", "upstream_"+f.lastUpstream.verdict.Class, "No eligible upstream completed the request.")
	f.errorOut(status, "server_error", "upstream_"+f.lastUpstream.verdict.Class, "No eligible upstream completed the request.")
	return true
}

func GetUpstreamPolicy(w http.ResponseWriter, r *http.Request) {
	if !requireGatewayRoot(w, r) {
		return
	}
	var option model.Option
	version, err := model.OptionVersion()
	if err == nil {
		err = model.DB.WithContext(r.Context()).First(&option, map[string]any{"key": upstream.OptionKey}).Error
	}
	history := []model.OptionChange{}
	if err == nil {
		err = model.DB.WithContext(r.Context()).Where(map[string]any{"key": upstream.OptionKey}).Order("version DESC").Limit(20).Find(&history).Error
	}
	after, afterErr := model.OptionVersion()
	if err != nil || afterErr != nil || version != after {
		common.ApiError(w, 503, "configuration changed or unavailable; reload")
		return
	}
	common.ApiSuccess(w, map[string]any{"version": version, "value": option.Value, "history": history})
}

func UpdateUpstreamPolicy(w http.ResponseWriter, r *http.Request) {
	if !requireGatewayRoot(w, r) {
		return
	}
	var req struct {
		ExpectedVersion *int64 `json:"expected_version"`
		Value           string `json:"value"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.ExpectedVersion == nil || *req.ExpectedVersion < 0 || *req.ExpectedVersion > 1<<53-2 {
		common.ApiError(w, 400, "expected_version is required")
		return
	}
	value, err := upstream.NormalizePolicy(req.Value)
	if err != nil {
		common.ApiError(w, 400, err.Error())
		return
	}
	actor, _ := r.Context().Value(common.CtxUserID).(int64)
	version, err := model.SaveOptionIfVersion(upstream.OptionKey, value, actor, *req.ExpectedVersion)
	if errors.Is(err, model.ErrOptionVersionConflict) {
		common.ApiError(w, 409, "configuration changed; reload")
		return
	}
	if err != nil {
		common.ApiError(w, 500, "could not save upstream policy")
		return
	}
	common.ApiSuccess(w, map[string]any{"version": version, "applied": model.LoadOptionsToSettings() == nil})
}

func GetUpstreamHealth(w http.ResponseWriter, r *http.Request) {
	current, err := model.CurrentUpstreamStates(r.Context(), false, 500)
	if err != nil {
		common.ApiError(w, 503, "upstream health unavailable")
		return
	}
	common.ApiSuccess(w, map[string]any{"states": current, "enabled": upstream.CurrentPolicy().Enabled, "scope": "shared_database", "limit": 500, "diagnostics": upstream.Diagnostics()})
}

func GetUpstreamEvents(w http.ResponseWriter, r *http.Request) {
	var events []upstream.Event
	q := model.DB.WithContext(r.Context())
	order := "created DESC, started DESC, id DESC"
	if id := r.URL.Query().Get("request_id"); id != "" {
		q = q.Where("request_id = ?", id)
		order = "started ASC, id ASC"
	}
	if id := r.URL.Query().Get("resource_id"); id != "" {
		q = q.Where("resource_id = ?", id)
	}
	if err := q.Order(order).Limit(100).Find(&events).Error; err != nil {
		common.ApiError(w, 503, "upstream events unavailable")
		return
	}
	if events == nil {
		events = []upstream.Event{}
	}
	common.ApiSuccess(w, events)
}

func ProbeUpstream(w http.ResponseWriter, r *http.Request) {
	if !requireGatewayRoot(w, r) {
		return
	}
	var req struct {
		ID string `json:"id"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if err := service.ProbeProtectedResource(r.Context(), req.ID); err != nil {
		common.ApiError(w, 409, "probe unavailable, cooling down, or already running")
		return
	}
	common.ApiSuccess(w, true)
}

func ResetUpstreamProtection(w http.ResponseWriter, r *http.Request) {
	if !requireGatewayRoot(w, r) {
		return
	}
	var req struct {
		ID      string  `json:"id"`
		Version *uint64 `json:"version"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Version == nil || *req.Version > 1<<53-2 {
		common.ApiError(w, 400, "resource version required")
		return
	}
	actor, _ := r.Context().Value(common.CtxUserID).(int64)
	if err := upstream.Reset(model.DB.WithContext(r.Context()), req.ID, *req.Version, actor); err != nil {
		common.ApiError(w, 409, "resource state changed or unavailable; refresh")
		return
	}
	common.ApiSuccess(w, true)
}
