package controller

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"time"

	"starhub/common"
	"starhub/model"
	"starhub/relay"
	"starhub/relay/dto"
)

type responseReferenceCapture struct {
	reference model.ResponseReference
	at        time.Time
}

func captureResponseReference(flow *relayFlow, ch *model.Channel, key string, req *dto.ResponsesRequest) (*responseReferenceCapture, error) {
	if req.Store != nil && !*req.Store {
		return nil, nil
	}
	ref := model.ResponseReference{UserID: flow.req.userID, TokenID: flow.req.tokenID, ChannelID: ch.ID,
		CredentialIdentity: model.ResponseCredentialIdentity(ch, key), Model: flow.req.modelName, Group: flow.settleGroup(),
		AccountIdentity: model.ResponseAccountIdentity(ch, key),
		HasImages:       req.HasImages, HasTools: req.HasTools, HasReasoning: req.HasReasoning}
	group, err := model.GetServiceGroup(ref.Group)
	if err != nil && !errors.Is(err, model.ErrServiceGroupNotFound) {
		return nil, err
	}
	if group != nil {
		ref.GroupDefinitionID = group.ID
	}
	return &responseReferenceCapture{reference: ref, at: flow.startTime}, nil
}

// The wire object remains provider-owned. This metadata observation is not a
// local content-storage guarantee; headers/trailers report index availability.
func observeResponseReference(flow *relayFlow, capture *responseReferenceCapture, response responseAccounting, usage *relay.Usage) string {
	if capture == nil || (response.Store != nil && !*response.Store) {
		return "not_stored"
	}
	if response.ID == "" {
		return "unavailable"
	}
	ref := capture.reference
	ref.ResponseID, ref.Status = response.ID, response.Status
	ref.PromptTokens, ref.CompletionTokens = usage.PromptTokens, usage.CompletionTokens
	if len(response.Output) > 0 {
		body, _ := json.Marshal(struct {
			Model string          `json:"model"`
			Input json.RawMessage `json:"input"`
		}{ref.Model, response.Output})
		if parsed, err := dto.ParseResponsesRequest(body); err == nil {
			ref.HasImages = ref.HasImages || parsed.HasImages
			ref.HasTools = ref.HasTools || parsed.HasTools
			ref.HasReasoning = ref.HasReasoning || parsed.HasReasoning
		} else {
			// Unknown output extensions must not erase future context requirements.
			ref.HasImages, ref.HasTools, ref.HasReasoning = true, true, true
		}
	}
	// Completion may already have reached an SSE client that closes its socket.
	// Finish the bounded metadata observation independently of that disconnect.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(flow.ctx), 5*time.Second)
	defer cancel()
	if _, err := model.RecordResponseReference(ctx, ref, capture.at); err != nil {
		log.Printf("Responses ownership index unavailable: %v", err)
		return "unavailable"
	}
	if _, err := model.LookupResponseReference(ctx, ref.UserID, ref.TokenID, ref.ResponseID, time.Now()); err != nil {
		log.Printf("Responses ownership reference cannot be resolved: %v", err)
		return "unavailable"
	}
	if lease, ok := flow.ctx.Value(common.CtxResponseConversationLease).(*model.ResponseConversationLease); ok {
		id := ""
		if response.Status == "completed" || response.Status == "incomplete" {
			id = ref.ResponseID
		}
		if err := model.FinishResponseConversation(lease.Conversation.ID, lease.Nonce, id); err != nil {
			return "unavailable"
		}
		lease.Done = true
	}
	return "recorded"
}
