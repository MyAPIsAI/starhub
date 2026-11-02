package controller

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"starhub/common"
	"starhub/model"
	"starhub/relay"
	"starhub/relay/dto"
	"starhub/service"
	"time"
)

func beginBackgroundResponse(flow *relayFlow, ch *model.Channel, capture *responseReferenceCapture, request *dto.ResponsesRequest) (*model.Task, error) {
	if capture == nil {
		return nil, errors.New("background ownership snapshot is required")
	}
	r := capture.reference
	snapshot := relay.BackgroundResponseSnapshot{Binding: relay.ResponseTaskBinding{ChannelID: r.ChannelID, GroupDefinitionID: r.GroupDefinitionID, CredentialIdentity: r.CredentialIdentity, AccountIdentity: r.AccountIdentity, UpstreamModel: r.UpstreamModel, Model: r.Model, Group: r.Group, HasImages: r.HasImages, HasTools: r.HasTools, HasReasoning: r.HasReasoning}, PriceData: *flow.req.priceFor(flow.settleGroup()), InputTokens: request.InputTokens, CreatedAt: capture.at.Unix()}
	lease, _ := flow.ctx.Value(common.CtxResponseConversationLease).(*model.ResponseConversationLease)
	if lease != nil {
		snapshot.Binding.ConversationID = lease.Conversation.ID
		snapshot.Binding.ConversationNonce = lease.Nonce
	}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return nil, err
	}
	task := &model.Task{Platform: "responses", Action: "responses", Status: "submitting", Model: r.Model, RequestedModel: flow.req.requestedModel, Protocol: "OpenAI", UserID: r.UserID, TokenID: r.TokenID, TokenName: flow.req.tokenName, TokenUnlimited: flow.req.tokenUnlimited, ChannelID: ch.ID, Group: r.Group, UserGroup: flow.req.userGroup, Quota: flow.preQuota, RequestMeta: string(raw)}
	if err := model.InsertTask(task); err != nil {
		return nil, err
	}
	flow.settled = true
	if lease != nil {
		lease.Handoff = true
	}
	return task, nil
}
func relayBackgroundResponse(flow *relayFlow, response *http.Response, task *model.Task) attemptResult {
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxModeResponseBytes+1))
	var result struct{ ID, Status, Model string }
	if err != nil || len(raw) > maxModeResponseBytes || json.Unmarshal(raw, &result) != nil || result.ID == "" || (result.Model != "" && result.Model != func() string {
		var snapshot relay.BackgroundResponseSnapshot
		_ = json.Unmarshal([]byte(task.RequestMeta), &snapshot)
		if snapshot.Binding.UpstreamModel != "" {
			return snapshot.Binding.UpstreamModel
		}
		return task.Model
	}()) {
		flow.errorOut(502, "server_error", "background_submission_uncertain", "provider response could not be recorded; execution may be uncertain")
		return attemptResult{outcome: attemptUncertain}
	}
	switch result.Status {
	case "queued", "in_progress", "completed", "incomplete", "failed", "cancelled":
	default:
		flow.errorOut(502, "server_error", "background_submission_uncertain", "provider returned an unknown background state")
		return attemptResult{outcome: attemptUncertain}
	}
	if err := model.BindBackgroundResponseTask(task.ID, result.ID); err != nil {
		flow.errorOut(500, "server_error", "background_submission_uncertain", "background response binding could not be persisted")
		return attemptResult{outcome: attemptUncertain}
	}
	task.TaskID = result.ID
	task.Status = "submitted"
	ctx, cancel := context.WithTimeout(context.WithoutCancel(flow.ctx), 5*time.Second)
	defer cancel()
	if _, err := service.EnsureBackgroundResponseReference(ctx, task); err != nil {
		flow.errorOut(502, "server_error", "response_reference_unavailable", "background response ownership could not be confirmed")
		return attemptResult{outcome: attemptUncertain}
	}
	if _, terminal := responseOutcome(result.Status); terminal {
		if _, err := service.ObserveBackgroundResponse(task, raw); err != nil {
			flow.w.Header().Set("X-StarHub-Response-Settlement", "pending")
		}
	}
	flow.w.Header().Set("X-StarHub-Response-Reference", "recorded")
	writePassthroughBody(flow.w, response, raw)
	return attemptResult{outcome: attemptSuccess, statusCode: response.StatusCode}
}
