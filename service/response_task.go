package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"starhub/model"
	"starhub/relay"
	"starhub/relay/channel"
	"starhub/relay/dto"
	"strings"
	"time"
)

func responseSnapshot(task *model.Task) (*relay.BackgroundResponseSnapshot, error) {
	var snapshot relay.BackgroundResponseSnapshot
	if err := json.Unmarshal([]byte(task.RequestMeta), &snapshot); err != nil {
		return nil, err
	}
	if snapshot.Binding.ChannelID != task.ChannelID || snapshot.Binding.Model != task.Model || snapshot.Binding.Group != task.Group || snapshot.CreatedAt <= 0 {
		return nil, errors.New("invalid background response snapshot")
	}
	return &snapshot, nil
}
func EnsureBackgroundResponseReference(ctx context.Context, task *model.Task) (*model.ResponseReference, error) {
	snapshot, err := responseSnapshot(task)
	if err != nil {
		return nil, err
	}
	b := snapshot.Binding
	ref := model.ResponseReference{UserID: task.UserID, TokenID: task.TokenID, ChannelID: b.ChannelID, CredentialIdentity: b.CredentialIdentity, AccountIdentity: b.AccountIdentity, UpstreamModel: b.UpstreamModel, Model: b.Model, Group: b.Group, GroupDefinitionID: b.GroupDefinitionID, ResponseID: task.TaskID, Status: "queued", TaskID: task.ID, Background: true, HasImages: b.HasImages, HasTools: b.HasTools, HasReasoning: b.HasReasoning}
	existing, err := model.LookupResponseReference(ctx, task.UserID, task.TokenID, task.TaskID, time.Now())
	if err == nil {
		if existing.TaskID != task.ID {
			return nil, model.ErrResponseReferenceAmbiguous
		}
		return existing, nil
	}
	if !errors.Is(err, model.ErrResponseReferenceNotFound) {
		return nil, err
	}
	if _, err = model.RecordResponseReference(ctx, ref, time.Unix(snapshot.CreatedAt, 0)); err != nil {
		return nil, err
	}
	return model.LookupResponseReference(ctx, task.UserID, task.TokenID, task.TaskID, time.Now())
}
func failResponseTask(task *model.Task, reason string) (*model.Task, error) {
	meta, _ := json.Marshal(model.BackgroundResponseOutcome{ResponseStatus: "failed"})
	return recordAndSettleTask(task, &dto.TaskResult{Status: "failed", FailReason: reason}, string(meta), 0)
}
func pollBackgroundResponse(ctx context.Context, task *model.Task) (*model.Task, error) {
	if task.TaskID == "" {
		if time.Since(time.Unix(task.SubmitTime, 0)) < 10*time.Minute {
			return task, nil
		}
		return failResponseTask(task, "Background submission has no durable provider ID; execution may be uncertain")
	}
	if time.Since(time.Unix(task.SubmitTime, 0)) > 24*time.Hour {
		return failResponseTask(task, "Background response exceeded the polling deadline; provider execution is unconfirmed")
	}
	ref, err := EnsureBackgroundResponseReference(ctx, task)
	if err != nil {
		if errors.Is(err, model.ErrResponseReferenceAmbiguous) {
			return failResponseTask(task, "Provider response ID has conflicting ownership")
		}
		return task, err
	}
	ch, err := model.GetChannelByID(task.ChannelID)
	if err != nil {
		return task, err
	}
	key, ok := model.ResponseCredential(ch, ref.CredentialIdentity)
	if !ok {
		return task, errors.New("original response credential is unavailable")
	}
	adaptor, err := relay.GetChatAdaptor(ch.Type)
	if err != nil {
		return task, err
	}
	endpoint, err := adaptor.ModeURL(ch.BaseURL, channel.RelayModeResponses)
	if err != nil {
		return task, err
	}
	request, err := http.NewRequestWithContext(ctx, "GET", strings.TrimRight(endpoint, "/")+"/"+url.PathEscape(task.TaskID), nil)
	if err != nil {
		return task, err
	}
	if err := adaptor.PrepareRequest(request, key, nil); err != nil {
		return task, err
	}
	request.Header.Set("Accept", "application/json")
	client := *taskHTTPClient
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Do(request)
	if err != nil {
		return task, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return task, fmt.Errorf("background response polling returned %d", response.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, (32<<20)+1))
	if err != nil || len(raw) > 32<<20 {
		return task, errors.New("background response exceeds limit")
	}
	return ObserveBackgroundResponse(task, raw)
}

// ObserveBackgroundResponse freezes the first terminal usage and shares the
// task settlement claim with all GET, cancel and background worker observers.
func ObserveBackgroundResponse(task *model.Task, raw []byte) (*model.Task, error) {
	var response struct {
		ID, Status, Model string
		Output            json.RawMessage
	}
	if json.Unmarshal(raw, &response) != nil || response.ID != task.TaskID || (response.Model != "" && response.Model != func() string {
		var saved relay.BackgroundResponseSnapshot
		_ = json.Unmarshal([]byte(task.RequestMeta), &saved)
		if saved.Binding.UpstreamModel != "" {
			return saved.Binding.UpstreamModel
		}
		return task.Model
	}()) {
		return task, errors.New("provider response identity mismatch")
	}
	switch response.Status {
	case "queued", "in_progress":
		return task, nil
	case "completed", "incomplete", "failed", "cancelled":
	default:
		return task, errors.New("unknown provider response status")
	}
	snapshot, err := responseSnapshot(task)
	if err != nil {
		return task, err
	}
	usage, err := dto.ParseResponsesUsage(raw)
	if err != nil {
		return task, err
	}
	if usage == nil {
		out := int64(0)
		encoded, _ := json.Marshal(map[string]any{"model": task.Model, "input": response.Output})
		if parsed, e := dto.ParseResponsesRequest(encoded); e == nil {
			out = parsed.InputTokens
		}
		usage = &relay.Usage{PromptTokens: snapshot.InputTokens, CompletionTokens: out}
		if response.Status != "completed" && response.Status != "incomplete" && out == 0 {
			usage = &relay.Usage{}
		}
	}
	status := "succeeded"
	if response.Status == "failed" || response.Status == "cancelled" {
		status = response.Status
	}
	outcome := model.BackgroundResponseOutcome{ResponseStatus: response.Status, PromptTokens: usage.PromptTokens, CompletionTokens: usage.CompletionTokens, HasImages: snapshot.Binding.HasImages, HasTools: snapshot.Binding.HasTools, HasReasoning: snapshot.Binding.HasReasoning}
	if len(response.Output) > 0 {
		encoded, _ := json.Marshal(map[string]any{"model": task.Model, "input": response.Output})
		if output, e := dto.ParseResponsesRequest(encoded); e == nil {
			outcome.HasImages = outcome.HasImages || output.HasImages
			outcome.HasTools = outcome.HasTools || output.HasTools
			outcome.HasReasoning = outcome.HasReasoning || output.HasReasoning
		} else {
			outcome.HasImages, outcome.HasTools, outcome.HasReasoning = true, true, true
		}
	}
	meta, _ := json.Marshal(outcome)
	actual := relay.ComputeUsageQuota(&snapshot.PriceData, usage)
	return recordAndSettleTask(task, &dto.TaskResult{Status: status}, string(meta), actual)
}
func FailBackgroundSubmission(task *model.Task, reason string) (*model.Task, error) {
	return failResponseTask(task, reason)
}
