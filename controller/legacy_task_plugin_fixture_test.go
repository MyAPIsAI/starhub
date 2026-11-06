package controller

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"slices"
	"strings"

	"starhub/common"
	"starhub/metric"
	"starhub/model"
	"starhub/pkg/taskplugin"
	"starhub/relay"
	"starhub/relay/channel"
	"starhub/relay/dto"
	"starhub/service"
)

// legacyPluginAdmission recreates accepted tasks for recovery tests only.
// Production admission cannot execute a JavaScript task plugin.
func legacyPluginAdmission(w http.ResponseWriter, r *http.Request) {
	req := relayRequestFromContext(r)
	flow := newRelayFlow(r.Context(), w, req, "", "task")
	defer flow.finish()
	for {
		ch, err := flow.pick()
		if err != nil {
			flow.exhausted()
			return
		}
		flow.begin(ch)
		flow.unsupported++
		if submitPluginTask(flow, ch) {
			return
		}
	}
}

// Legacy admission is a test fixture for recovery of pre-retirement tasks.
// submitPluginTask returns false only when the candidate cannot accept new tasks.
// A durable task protects channel credentials before HTTP is sent.
func submitPluginTask(flow *relayFlow, picked *model.Channel) bool {
	ch, err := model.GetChannelByID(picked.ID)
	if err != nil || ch.Type != channel.ChannelTypeTaskPlugin || ch.Status != common.ChannelStatusEnabled || !slices.Contains(model.SplitList(ch.Models), flow.req.modelName) || !slices.Contains(model.ChannelGroups(ch), flow.settleGroup()) {
		return false
	}
	plugin, err := relay.LoadTaskPlugin(flow.ctx, ch.TaskPluginID, true)
	if err != nil {
		flow.lastErr = err
		return false
	}
	if !slices.Contains(plugin.Meta.Models, flow.req.modelName) {
		return false
	}
	taskReq, err := dto.ParseVideoTaskRequest(flow.req.body)
	if err != nil {
		flow.errorOut(http.StatusBadRequest, "invalid_request_error", "invalid_request_body", err.Error())
		return true
	}
	context := map[string]any{"request": taskReq, "model": flow.req.modelName}
	var prepared taskplugin.Prepared
	if err := plugin.Call(flow.ctx, "prepareSubmit", []any{context}, &prepared); err != nil {
		flow.errorOut(400, "invalid_request_error", "plugin_prepare_failed", err.Error())
		return true
	}
	if _, err := prepared.Request.URL(ch.BaseURL, http.MethodPost); err != nil {
		flow.errorOut(502, "server_error", "invalid_plugin_request", err.Error())
		return true
	}
	pd := flow.req.priceFor(flow.settleGroup())
	quota, err := relay.PluginTaskReservation(pd, taskReq, prepared.Usage)
	if err != nil {
		flow.errorOut(400, "invalid_request_error", "invalid_task_pricing", err.Error())
		return true
	}
	snapshot := relay.PluginTaskPricingSnapshot{PriceData: *pd, Usage: prepared.Usage}
	if pd.HasIndependentTaskPricing() {
		snapshot.Version = 2
		snapshot.Estimate = relay.TaskTokenEstimate(taskReq)
	}
	pricing, err := json.Marshal(snapshot)
	if err != nil {
		flow.errorOut(500, "server_error", "invalid_task_pricing", "Unable to save task pricing")
		return true
	}
	request, _ := json.Marshal(taskReq)
	task := &model.Task{SubmitTime: flow.startTime.Unix(), TraceID: flow.traceID, CallType: flow.req.callType, TokenPeriodStartedAt: flow.req.tokenPeriodStartedAt, TaskPluginID: ch.TaskPluginID, Platform: plugin.Meta.Key, Action: dto.TaskActionVideo, Status: model.TaskStatusSubmitting,
		Model: flow.req.modelName, RequestedModel: flow.req.requestedModel, Protocol: flow.req.protocol, UserID: flow.req.userID, TokenID: flow.req.tokenID, TokenName: flow.req.tokenName,
		TokenUnlimited: flow.req.tokenUnlimited, ChannelID: ch.ID, Group: flow.settleGroup(), UserGroup: flow.req.userGroup, Quota: max(1, quota), RequestMeta: string(request), PricingMeta: string(pricing)}
	model.FreezeTaskRouting(flow.ctx, task, flow.affinityPreferred == task.ChannelID)
	if err := model.ReservePluginTask(task, ch); err != nil {
		if errors.Is(err, model.ErrTaskPluginChannelChanged) {
			flow.lastErr = err
			return false
		}
		if errors.Is(err, model.ErrUserQuotaShort) {
			err = relay.ErrInsufficientUserQuota
		}
		if errors.Is(err, model.ErrTokenQuotaShort) {
			err = relay.ErrInsufficientTokenQuota
		}
		writePreConsumeError(flow.w, "", err)
		return true
	}
	// The task row now owns the reservation, including across process restarts.
	flow.preQuota, flow.settled = task.Quota, true
	flow.w.Header().Set("X-StarHub-Task-ID", fmt.Sprint(task.ID))
	completed := false
	defer func() {
		if !completed {
			if _, err := service.FailPluginSubmission(task, "Plugin submission failed; upstream execution may be uncertain"); err != nil {
				log.Printf("plugin task %d submission refund pending: %v", task.ID, err)
			}
		}
	}()
	flow.attempt = flow.span.BeginAttempt(ch.ID)
	outcome, status := metric.AttemptUpstreamError, http.StatusBadGateway
	defer func() { flow.span.EndAttempt(flow.attempt, outcome, status); flow.attempt = nil }()
	key, _ := ch.SelectKey()
	response, err := plugin.Do(flow.ctx, ch.BaseURL, key, prepared.Request, http.MethodPost)
	if err != nil || response.Status < 200 || response.Status >= 300 {
		flow.errorOut(502, "server_error", "plugin_submit_failed", "Upstream submission failed; execution may be uncertain. No automatic retry was made.")
		return true
	}
	var submitted taskplugin.Submitted
	if err := plugin.Call(flow.ctx, "parseSubmit", []any{context, response}, &submitted); err != nil || strings.TrimSpace(submitted.TaskID) == "" || len(submitted.TaskID) > 128 {
		flow.errorOut(502, "server_error", "plugin_submit_uncertain", "Upstream submission could not be parsed. No automatic retry was made.")
		return true
	}
	if err := model.CompletePluginSubmission(task.ID, submitted.TaskID); err != nil {
		flow.errorOut(500, "server_error", "persistence_failed", "Task accepted upstream but its ID could not be saved. No automatic retry was made.")
		return true
	}
	completed = true
	task.TaskID, task.Status = submitted.TaskID, dto.TaskStatusSubmitted
	outcome, status = metric.AttemptSuccess, http.StatusOK
	writeTaskSubmitResponse(flow.w, task)
	return true
}
