package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"starhub/model"
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
	if task.PendingStatus != "" {
		return settlePendingTask(task)
	}
	ch, err := model.GetChannelByID(task.ChannelID)
	if err != nil {
		// The channel is gone: the task can never complete, refund it.
		return recordAndSettleTask(task, &dto.TaskResult{
			Status:     dto.TaskStatusFailed,
			FailReason: fmt.Sprintf("channel %d no longer exists", task.ChannelID),
		}, "", 0)
	}
	adaptor, err := relay.GetTaskAdaptor(ch.Type)
	if err != nil {
		return recordAndSettleTask(task, &dto.TaskResult{
			Status:     dto.TaskStatusFailed,
			FailReason: fmt.Sprintf("channel %d lost task platform support", task.ChannelID),
		}, "", 0)
	}

	upstreamKey, _ := ch.SelectKey()
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
		// A 4xx on poll proves the upstream task is dead (unknown id,
		// expired, ...); 5xx is transient.
		if resp.StatusCode < http.StatusInternalServerError {
			return recordAndSettleTask(task, &dto.TaskResult{
				Status: dto.TaskStatusFailed,
				FailReason: fmt.Sprintf("upstream poll returned %d: %s",
					resp.StatusCode, string(body)),
			}, "", 0)
		}
		return task, nil
	}
	result, err := adaptor.ParseTaskResult(resp.StatusCode, body)
	if err != nil {
		return task, err
	}

	resultMeta, _ := json.Marshal(result)
	if dto.IsTerminalTaskStatus(result.Status) {
		return recordAndSettleTask(task, result, string(resultMeta), taskFinalQuota(task, adaptor, result))
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
// balances the pre-payment: failed tasks refund fully, cancellations
// partially, successes keep (or adjust to) the request-based bill.
func taskFinalQuota(task *model.Task, adaptor channel.TaskAdaptor, result *dto.TaskResult) int64 {
	var req dto.TaskRequest
	if err := json.Unmarshal([]byte(task.RequestMeta), &req); err != nil {
		log.Printf("task %d has an unreadable request meta, refunding: %v", task.ID, err)
		return 0
	}
	accepted := adaptor.EstimateBilling(task.Model, task.Group, &req)
	priceData := relay.NewPriceDataWithUserGroup(task.Model, task.Group, task.UserGroup)
	if task.PricingMeta != "" {
		var snapshot relay.TaskBillingSnapshot
		if err := json.Unmarshal([]byte(task.PricingMeta), &snapshot); err != nil {
			log.Printf("task %d has an unreadable pricing snapshot, refunding: %v", task.ID, err)
			return 0
		} else {
			priceData = &snapshot.PriceData
			accepted = snapshot.Estimate
		}
	}
	pricing := adaptor.AdjustBillingOnComplete(task.Model, task.Group, &req, result, accepted)
	actual := relay.TaskPricingQuotaWithPriceData(priceData, pricing)
	return actual
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
