package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"starhub/model"
	"starhub/relay/channel/ark"
	"starhub/relay/dto"
)

var channelVideoClient = &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

// probeArkVideoAPI checks read-only task access without requiring the model catalog.
func probeArkVideoAPI(ctx context.Context, ch *model.Channel, key string, index int) ProbeResult {
	start := time.Now()
	result := ProbeResult{Scope: "video_api", KeyIdx: index, Code: "upstream_video_failed"}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ark.APIBaseURL(ch.BaseURL)+"/contents/generations/tasks?page_size=1", nil)
	if err != nil {
		return result
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(key))
	req.Header.Set("Accept", "application/json")
	resp, err := channelVideoClient.Do(req)
	result.ElapsedMs = time.Since(start).Milliseconds()
	if err != nil {
		result.Code = "upstream_video_network_error"
		return result
	}
	defer resp.Body.Close()
	result.HTTPStatus = resp.StatusCode
	if resp.StatusCode != http.StatusOK {
		failure := channelUpstreamError(resp, req, key)
		result.Code = strings.Replace(failure.Code, "upstream_models_", "upstream_video_", 1)
		result.Diagnostic = failure.Diagnostic
		return result
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	var envelope struct {
		Items *[]json.RawMessage `json:"items"`
		Total *int               `json:"total"`
	}
	if err != nil || len(body) > 1<<20 || json.Unmarshal(body, &envelope) != nil || envelope.Items == nil || envelope.Total == nil || *envelope.Total < 0 {
		result.Code = "upstream_video_invalid_response"
		return result
	}
	result.OK, result.Code, result.Message = true, "ark_video_api_accessible", "Video task API accessible; generation was not tested"
	return result
}

// SubmitChannelVideoProbe sends one small, explicitly requested admin generation.
func SubmitChannelVideoProbe(ctx context.Context, ch *model.Channel, userID int64, requestID, name, prompt string) (*model.Task, error) {
	if ch.Type != 32 || (!strings.HasPrefix(name, "doubao-seedance-") && ch.ModelCapabilities[name] != "VideoGeneration" && !(ch.ModelCapabilities[name] == "" && strings.HasPrefix(name, "ep-"))) {
		return nil, errors.New("video_probe_model_not_supported")
	}
	if _, err := NormalizeArkModelsBaseURL(ch.BaseURL); err != nil {
		return nil, errors.New("video_probe_invalid_request")
	}
	key, index := ch.SelectKey()
	if key == "" {
		return nil, errors.New("models_key_required")
	}
	raw, _ := json.Marshal(map[string]any{"model": name, "prompt": prompt, "seconds": 4, "size": "480p", "n": 1})
	adaptor := ark.VideoTaskAdaptor{}
	var parsed dto.TaskRequest
	if adaptor.ValidateRequestAndSetAction(raw, &parsed) != nil {
		return nil, errors.New("video_probe_invalid_request")
	}
	body, err := adaptor.ConvertSubmitRequest(&parsed)
	if err != nil {
		return nil, errors.New("video_probe_invalid_request")
	}
	meta, _ := json.Marshal(parsed)
	task := &model.Task{Platform: model.ArkVideoProbePlatform, ChannelID: ch.ID, UserID: userID, KeyIndex: max(index, 0), Model: name, RequestedModel: name, Status: model.TaskStatusSubmitting, Action: dto.TaskActionVideo, RequestMeta: string(meta), QuotaReservationID: requestID}
	task, created, err := model.ReserveVideoProbe(task, ch)
	if err != nil || !created {
		return task, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, adaptor.SubmitURL(ch.BaseURL), bytes.NewReader(body))
	if err != nil {
		return task, errors.New("video_probe_invalid_request")
	}
	_ = adaptor.PrepareRequest(req, key, body)
	req.Header.Set("Content-Type", "application/json")
	resp, err := channelVideoClient.Do(req)
	reason := "video_probe_submission_uncertain"
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			failure := channelUpstreamError(resp, req, key)
			reason = strings.Replace(failure.Code, "upstream_models_", "upstream_video_", 1)
		} else if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			payload, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
			id, parseErr := adaptor.ParseSubmitResponse(resp.StatusCode, payload)
			if parseErr == nil {
				if err := model.CompleteTaskSubmission(task.ID, id); err != nil {
					return task, errors.New("video_probe_submission_uncertain")
				}
				return model.GetTaskByID(task.ID)
			}
		}
	}
	if err := model.UpdateVideoProbe(task.ID, dto.TaskStatusFailed, reason, ""); err != nil {
		return task, err
	}
	return model.GetTaskByID(task.ID)
}

// PollChannelVideoProbe is shared by the admin UI and durable background worker.
func PollChannelVideoProbe(ctx context.Context, task *model.Task) (*model.Task, error) {
	if task.Platform != model.ArkVideoProbePlatform {
		return nil, errors.New("video_probe_not_found")
	}
	if dto.IsTerminalTaskStatus(task.Status) {
		return task, nil
	}
	if err := model.TouchTaskPoll(task.ID); err != nil {
		return task, err
	}
	if task.TaskID == "" {
		if time.Since(time.Unix(task.SubmitTime, 0)) > PluginSubmissionTimeout {
			if err := model.UpdateVideoProbe(task.ID, dto.TaskStatusFailed, "video_probe_submission_uncertain", ""); err != nil {
				return task, err
			}
			return model.GetTaskByID(task.ID)
		}
		return task, nil
	}
	ch, err := model.GetChannelByID(task.ChannelID)
	if err != nil {
		return task, errors.New("video_probe_channel_unavailable")
	}
	keys := ch.Keys()
	if task.KeyIndex < 0 || task.KeyIndex >= len(keys) {
		return task, errors.New("models_key_required")
	}
	adaptor := ark.VideoTaskAdaptor{}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, adaptor.FetchURL(ch.BaseURL, task.TaskID), nil)
	if err != nil {
		return task, err
	}
	_ = adaptor.PrepareRequest(req, keys[task.KeyIndex], nil)
	resp, err := channelVideoClient.Do(req)
	if err != nil {
		return task, errors.New("upstream_video_network_error")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
			if err := model.UpdateVideoProbe(task.ID, dto.TaskStatusFailed, "video_probe_result_expired", ""); err != nil {
				return task, err
			}
			return model.GetTaskByID(task.ID)
		}
		failure := channelUpstreamError(resp, req, keys[task.KeyIndex])
		return task, errors.New(strings.Replace(failure.Code, "upstream_models_", "upstream_video_", 1))
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if err != nil || len(body) > 1<<20 {
		return task, errors.New("upstream_video_invalid_response")
	}
	result, err := adaptor.ParseTaskResult(resp.StatusCode, body)
	if err != nil {
		return task, errors.New("upstream_video_invalid_response")
	}
	for _, output := range result.Outputs {
		if strings.Contains(output, keys[task.KeyIndex]) {
			return task, errors.New("upstream_video_invalid_response")
		}
	}
	encoded, _ := json.Marshal(result)
	if err := model.UpdateVideoProbe(task.ID, result.Status, result.FailReason, string(encoded)); err != nil {
		return task, err
	}
	return model.GetTaskByID(task.ID)
}
