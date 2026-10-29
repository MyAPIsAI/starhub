package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"time"

	"starhub/model"
	"starhub/pkg/taskplugin"
	"starhub/relay"
	"starhub/relay/dto"
)

const (
	PluginSubmissionTimeout = 5 * time.Minute
	PluginTaskTimeout       = 24 * time.Hour
	PluginMissingGrace      = 5 * time.Minute
)

// FailPluginSubmission releases a durable reservation exactly once. A completed
// admission cannot be overwritten by a late submission-error cleanup.
func FailPluginSubmission(task *model.Task, reason string) (*model.Task, error) {
	stored, err := model.RecordPluginSubmissionFailure(task.ID, reason)
	if err != nil {
		return task, err
	}
	*task = *stored
	if task.PendingStatus != "" {
		return settlePendingTask(task)
	}
	return task, nil
}

func pollPluginTask(ctx context.Context, task *model.Task, ch *model.Channel) (*model.Task, error) {
	plugin, err := relay.LoadTaskPlugin(ctx, task.TaskPluginID, false)
	if err != nil {
		return task, err
	}
	var request dto.TaskRequest
	if err := json.Unmarshal([]byte(task.RequestMeta), &request); err != nil {
		return task, errors.New("invalid plugin task request snapshot")
	}
	context := map[string]any{"request": request, "model": task.Model, "taskId": task.TaskID}
	var descriptor taskplugin.Request
	if err := plugin.Call(ctx, "buildQuery", []any{context}, &descriptor); err != nil {
		return task, err
	}
	key, _ := ch.SelectKey()
	response, err := plugin.Do(ctx, ch.BaseURL, key, descriptor, http.MethodGet)
	if response.Status == http.StatusNotFound || response.Status == http.StatusGone {
		count, recordErr := model.RecordPluginTaskMissing(task.ID, time.Now())
		if recordErr != nil {
			return task, recordErr
		}
		if count >= 3 && time.Since(time.Unix(task.SubmitTime, 0)) >= PluginMissingGrace {
			return recordAndSettleTask(task, &dto.TaskResult{Status: dto.TaskStatusFailed,
				FailReason: "Upstream task remained missing or expired after the confirmation period"}, "", 0)
		}
		return task, fmt.Errorf("plugin poll HTTP %d; awaiting missing-task confirmation", response.Status)
	}
	if resetErr := model.ResetPluginTaskMissing(task.ID); resetErr != nil {
		return task, resetErr
	}
	if err != nil {
		return task, err
	}
	if response.Status < 200 || response.Status >= 300 {
		return task, fmt.Errorf("plugin poll HTTP %d; task remains pending", response.Status)
	}
	var parsed taskplugin.Result
	if err := plugin.Call(ctx, "parseQuery", []any{context, response}, &parsed); err != nil {
		return task, err
	}
	if !slices.Contains([]string{dto.TaskStatusSubmitted, dto.TaskStatusInProgress, dto.TaskStatusSucceeded, dto.TaskStatusFailed, dto.TaskStatusCancelled}, parsed.Status) || parsed.Progress < 0 || parsed.Progress > 100 || len(parsed.FailReason) > 512 || len(parsed.Outputs) > 10 {
		return task, errors.New("invalid plugin task result; task remains pending")
	}
	if parsed.Usage != nil {
		if err := parsed.Usage.Validate(); err != nil {
			return task, err
		}
	}
	for _, output := range parsed.Outputs {
		u, err := url.Parse(output)
		if err != nil || u == nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil || len(output) > 4096 {
			return task, errors.New("invalid plugin result URL")
		}
	}
	result := &dto.TaskResult{Status: parsed.Status, Progress: parsed.Progress, FailReason: parsed.FailReason, Outputs: parsed.Outputs}
	if parsed.Status == dto.TaskStatusSucceeded {
		if len(parsed.Outputs) == 0 {
			return task, errors.New("plugin reports success without deliverable results; task remains pending")
		}
		result.Progress = 100
	}
	metadata, _ := json.Marshal(result)
	if dto.IsTerminalTaskStatus(result.Status) {
		quota, err := relay.PluginTaskFinalQuota(task, parsed)
		if err != nil {
			return task, err
		}
		return recordAndSettleTask(task, result, string(metadata), quota)
	}
	if err := model.UpdateTaskProgress(task.ID, result.Status, result.Progress, string(metadata)); err != nil {
		return task, err
	}
	task.Status, task.Progress, task.ResultMeta = result.Status, result.Progress, string(metadata)
	return task, nil
}
