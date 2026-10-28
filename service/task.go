package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
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
// lifecycle tail: refresh progress, and on a terminal status claim the
// finalize (exactly once, even when the poll endpoint and the background
// reaper race) and settle the bill with AdjustBillingOnComplete.
// It returns the task after the update.
func PollTask(ctx context.Context, task *model.Task) (*model.Task, error) {
	ch, err := model.GetChannelByID(task.ChannelID)
	if err != nil {
		// The channel is gone: the task can never complete, refund it.
		claimTerminalTask(task, &dto.TaskResult{
			Status:     dto.TaskStatusFailed,
			FailReason: fmt.Sprintf("channel %d no longer exists", task.ChannelID),
		}, "")
		return task, nil
	}
	adaptor, err := relay.GetTaskAdaptor(ch.Type)
	if err != nil {
		claimTerminalTask(task, &dto.TaskResult{
			Status:     dto.TaskStatusFailed,
			FailReason: fmt.Sprintf("channel %d lost task platform support", task.ChannelID),
		}, "")
		return task, nil
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
			claimTerminalTask(task, &dto.TaskResult{
				Status: dto.TaskStatusFailed,
				FailReason: fmt.Sprintf("upstream poll returned %d: %s",
					resp.StatusCode, string(body)),
			}, "")
		}
		return task, nil
	}
	result, err := adaptor.ParseTaskResult(resp.StatusCode, body)
	if err != nil {
		return task, err
	}

	resultMeta, _ := json.Marshal(result)
	if dto.IsTerminalTaskStatus(result.Status) {
		if claimTerminalTask(task, result, string(resultMeta)) {
			settleTask(ch, task, adaptor, result)
		}
		return task, nil
	}
	if err := model.UpdateTaskProgress(task.ID, result.Status, result.Progress, string(resultMeta)); err != nil {
		log.Printf("update task %d progress failed: %v", task.ID, err)
	} else {
		task.Status = result.Status
		task.Progress = result.Progress
	}
	return task, nil
}

// claimTerminalTask flips the task to a terminal status exactly once and
// mirrors the final state onto the in-memory task for the caller's view.
// Only the caller that wins the claim may settle the task.
func claimTerminalTask(task *model.Task, result *dto.TaskResult, resultMeta string) bool {
	claimed, err := model.ClaimTaskFinalize(task.ID, result.Status, result.FailReason, resultMeta)
	if err != nil {
		log.Printf("finalize task %d failed: %v", task.ID, err)
		return false
	}
	if claimed {
		task.Status = result.Status
		task.Progress = result.Progress
		task.FailReason = result.FailReason
		if resultMeta != "" {
			task.ResultMeta = resultMeta
		}
		task.CompleteTime = time.Now().Unix()
	}
	return claimed
}

// settleTask prices the terminal task (AdjustBillingOnComplete) and
// balances the pre-payment: failed tasks refund fully, cancellations
// partially, successes keep (or adjust to) the request-based bill.
func settleTask(ch *model.Channel, task *model.Task, adaptor channel.TaskAdaptor, result *dto.TaskResult) {
	var req dto.TaskRequest
	if err := json.Unmarshal([]byte(task.RequestMeta), &req); err != nil {
		log.Printf("task %d has an unreadable request meta, refunding: %v", task.ID, err)
		req = dto.TaskRequest{}
	}
	accepted := adaptor.EstimateBilling(task.Model, task.Group, &req)
	priceData := relay.NewPriceDataWithUserGroup(task.Model, task.Group, task.UserGroup)
	if task.PricingMeta != "" {
		var snapshot relay.TaskBillingSnapshot
		if err := json.Unmarshal([]byte(task.PricingMeta), &snapshot); err != nil {
			log.Printf("task %d has an unreadable pricing snapshot, refunding: %v", task.ID, err)
			accepted = channel.TaskPricing{}
		} else {
			priceData = &snapshot.PriceData
			accepted = snapshot.Estimate
		}
	}
	pricing := adaptor.AdjustBillingOnComplete(task.Model, task.Group, &req, result, accepted)
	actual := relay.TaskPricingQuotaWithPriceData(priceData, pricing)
	if err := relay.PostConsumeQuotaAmount(task.UserID, task.TokenID, task.TokenName,
		task.TokenUnlimited, ch.ID, task.Model, actual, task.Quota,
		strconv.FormatInt(ch.ID, 10), 0, task.RequestedModel); err != nil {
		log.Printf("settle task %d quota failed: %v", task.ID, err)
		return
	}
	log.Printf("task %d (%s/%s) settled: pre %d actual %d status %s",
		task.ID, task.Platform, task.Action, task.Quota, actual, result.Status)
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
