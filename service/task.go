package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"starhub/model"
	"starhub/pkg/pricingtime"
	"starhub/relay"
	"starhub/relay/channel"
	"starhub/relay/dto"
)

// taskHTTPClient separates task polling from relay traffic: polls are
// short background calls, not user-facing requests.
var taskHTTPClient = &http.Client{Timeout: 30 * time.Second}

// PollTask fetches the upstream state of one task and applies the P3.3
// lifecycle tail: refresh progress, persist a terminal settlement intent, and
// commit the terminal state and bill together. Pending intents recover without
// polling the upstream again.
// It returns the task after the update.
func PollTask(ctx context.Context, task *model.Task) (*model.Task, error) {
	if dto.IsTerminalTaskStatus(task.Status) {
		return task, nil
	}
	if err := model.TouchTaskPoll(task.ID); err != nil {
		return task, err
	}
	if task.PendingStatus != "" {
		return settlePendingTask(task)
	}
	if task.Platform == "responses" {
		return pollBackgroundResponse(ctx, task)
	}
	if task.TaskPluginID != 0 {
		age := time.Since(time.Unix(task.SubmitTime, 0))
		if task.Status == model.TaskStatusSubmitting {
			if age >= PluginSubmissionTimeout {
				return FailPluginSubmission(task, "Plugin submission did not finish; upstream execution may be uncertain")
			}
			return task, nil
		}
		if age >= PluginTaskTimeout {
			return recordAndSettleTask(task, &dto.TaskResult{Status: dto.TaskStatusFailed,
				FailReason: "Plugin task exceeded the 24-hour completion deadline"}, "", 0)
		}
	}
	if task.Status == model.TaskStatusSubmitting {
		age := time.Since(time.Unix(task.SubmitTime, 0))
		if age >= PluginSubmissionTimeout {
			return FailTaskSubmission(task, "Task submission did not finish; upstream execution may be uncertain")
		}
		return task, nil
	}
	ch, err := model.GetChannelByID(task.ChannelID)
	if err != nil {
		if !errors.Is(err, model.ErrChannelNotFound) {
			return task, fmt.Errorf("load task channel: %w", err)
		}
		// The channel is gone: the task can never complete, refund it.
		return recordAndSettleTask(task, &dto.TaskResult{
			Status:     dto.TaskStatusFailed,
			FailReason: fmt.Sprintf("channel %d no longer exists", task.ChannelID),
		}, "", 0)
	}
	if task.TaskPluginID != 0 {
		return pollPluginTask(ctx, task, ch)
	}
	adaptor, err := relay.GetTaskAdaptor(ch.Type)
	if err != nil {
		return recordAndSettleTask(task, &dto.TaskResult{
			Status:     dto.TaskStatusFailed,
			FailReason: fmt.Sprintf("channel %d lost task platform support", task.ChannelID),
		}, "", 0)
	}

	// Reuse the submission credential so the upstream sees the same account.
	// A multi-key channel that selects a different key at poll time would
	// receive 404 from providers that isolate tasks by account.
	keys := ch.Keys()
	var upstreamKey string
	if task.KeyIndex >= 0 && task.KeyIndex < len(keys) {
		upstreamKey = keys[task.KeyIndex]
	}
	if upstreamKey == "" {
		return recordAndSettleTask(task, &dto.TaskResult{
			Status:     dto.TaskStatusFailed,
			FailReason: fmt.Sprintf("channel %d key index %d no longer exists", task.ChannelID, task.KeyIndex),
		}, "", 0)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		adaptor.FetchURL(ch.BaseURL, task.TaskID), nil)
	if err != nil {
		return task, err
	}
	if err := adaptor.PrepareRequest(req, upstreamKey, nil); err != nil {
		return task, err
	}
	resp, err := taskHTTPClient.Do(req)
	if err != nil {
		// Network issues are transient: leave the task active.
		return task, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= http.StatusBadRequest {
		// Only an explicit missing/expired result ends the task. Rate limits,
		// authentication failures and server errors do not prove execution failed.
		if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
			return recordAndSettleTask(task, &dto.TaskResult{
				Status: dto.TaskStatusFailed,
				FailReason: fmt.Sprintf("upstream poll returned %d: %s",
					resp.StatusCode, string(body)),
			}, "", 0)
		}
		return task, fmt.Errorf("upstream task poll returned HTTP %d; task remains pending", resp.StatusCode)
	}
	result, err := adaptor.ParseTaskResult(resp.StatusCode, body)
	if err != nil {
		return task, err
	}

	resultMeta, _ := json.Marshal(result)
	if dto.IsTerminalTaskStatus(result.Status) {
		actual, err := taskFinalQuota(task, adaptor, result)
		if err != nil {
			return task, err
		}
		resultMeta, _ = json.Marshal(result)
		return recordAndSettleTask(task, result, string(resultMeta), actual)
	}
	if err := model.UpdateTaskProgress(task.ID, result.Status, result.Progress, string(resultMeta)); err != nil {
		log.Printf("update task %d progress failed: %v", task.ID, err)
	} else {
		task.Status = result.Status
		task.Progress = result.Progress
	}
	return task, nil
}

// taskFinalQuota prices the terminal task (AdjustBillingOnComplete) and
// balances the pre-payment. Legacy status discounts stay unchanged. New
// token contracts bill reported facts even on failure or cancellation; missing
// facts use the labelled admission estimate and legacy status discounts.
func taskFinalQuota(task *model.Task, adaptor channel.TaskAdaptor, result *dto.TaskResult) (int64, error) {
	var req dto.TaskRequest
	if err := json.Unmarshal([]byte(task.RequestMeta), &req); err != nil {
		log.Printf("task %d has unreadable request metadata; settlement remains pending: %v", task.ID, err)
		return 0, fmt.Errorf("unreadable task request metadata")
	}
	accepted := adaptor.EstimateBilling(task.Model, task.Group, &req)
	priceData := relay.NewPriceDataWithUserGroup(task.Model, task.Group, task.UserGroup)
	// Tasks without a time-aware snapshot predate scheduled pricing. Do not
	// apply a newly configured time window to their legacy billing contract.
	priceData.TimeAdjustment.Multiplier = 1
	priceData.TimeAdjustment.WindowID = ""
	priceData.TimeAdjustment.Timezone = ""
	// Legacy tasks also predate independent token rates. Never import a newly
	// configured window into a task without its own admission snapshot.
	priceData.TokenRates = pricingtime.TokenRates{}
	priceData.TokenTimeAdjustment = pricingtime.TokenAdjustment{}
	if task.PricingMeta != "" {
		var snapshot relay.TaskBillingSnapshot
		if err := json.Unmarshal([]byte(task.PricingMeta), &snapshot); err != nil {
			log.Printf("task %d has an unreadable pricing snapshot; settlement remains pending: %v", task.ID, err)
			return 0, err
		} else {
			priceData = &snapshot.PriceData
			accepted = snapshot.Estimate
		}
	}
	pricing := adaptor.AdjustBillingOnComplete(task.Model, task.Group, &req, result, accepted)
	if priceData.HasIndependentTaskPricing() {
		if result.UsageError != "" {
			return 0, fmt.Errorf("invalid provider token usage: %s", result.UsageError)
		}
		if result.Usage != nil {
			if err := dto.ValidateTaskUsage(result.Usage); err != nil {
				return 0, err
			}
			pricing.Usage = result.Usage
			result.BillingUsageSource = "upstream"
		} else {
			result.BillingUsageSource = "admission_estimate"
		}
	}
	actual := relay.TaskPricingQuotaWithPriceData(priceData, pricing)
	return actual, nil
}

func recordAndSettleTask(task *model.Task, result *dto.TaskResult, meta string, actual int64) (*model.Task, error) {
	stored, err := model.RecordTaskSettlement(task.ID, result.Status, result.FailReason, meta, actual, result.Progress)
	if err != nil {
		return task, err
	}
	*task = *stored
	return settlePendingTask(task)
}

func settlePendingTask(task *model.Task) (*model.Task, error) {
	if dto.IsTerminalTaskStatus(task.Status) {
		return task, nil
	}
	if _, err := relay.FinalizeTaskQuota(task); err != nil {
		return task, err
	}
	stored, err := model.GetTaskByID(task.ID)
	if err == nil {
		*task = *stored
	}
	return task, err
}

// FailTaskSubmission releases a durable reservation exactly once for a
// native (non-plugin) task whose upstream submission failed. A completed
// admission cannot be overwritten by a late submission-error cleanup.
func FailTaskSubmission(task *model.Task, reason string) (*model.Task, error) {
	stored, err := model.RecordTaskSubmissionFailure(task.ID, reason)
	if err != nil {
		return task, err
	}
	*task = *stored
	if task.PendingStatus != "" {
		return settlePendingTask(task)
	}
	return task, nil
}

// RunTaskPollLoop is the background reaper: clients that never poll their
// tasks still get finished and billed.
func RunTaskPollLoop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			PollPendingTasksOnce(ctx)
		}
	}
}

// PollPendingTasksOnce sweeps every active task once.
func PollPendingTasksOnce(ctx context.Context) {
	tasks, err := model.ListPendingTasks(200)
	if err != nil {
		log.Printf("list pending tasks failed: %v", err)
		return
	}
	for i := range tasks {
		if _, err := PollTask(ctx, &tasks[i]); err != nil {
			log.Printf("poll task %d failed: %v", tasks[i].ID, err)
		}
	}
}
