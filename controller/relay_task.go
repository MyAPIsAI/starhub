package controller

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	"starhub/model"
	"starhub/relay"
	"starhub/relay/channel"
	"starhub/relay/dto"
	"starhub/service"
)

// ---------------------------------------------------------------------------
// Task API (P3.3): submit-and-poll lifecycle with built-in billing hooks.
// POST /v1/video/generations submits; GET /v1/video/generations/{id} polls.
// ---------------------------------------------------------------------------

// RelayTaskSubmitVideo serves POST /v1/video/generations (Sora-style).
func RelayTaskSubmitVideo(w http.ResponseWriter, r *http.Request) {
	req := relayRequestFromContext(r)
	if req.modelName == "" {
		writeModeError(w, http.StatusInternalServerError, "server_error", "internal_error", "request model is missing")
		return
	}
	flow := newRelayFlow(r.Context(), w, req, "", "task")
	defer flow.finish()

	// 1. Locate a task platform for the model and run
	//    ValidateRequestAndSetAction (billing multipliers are bounded
	//    before any quota moves). Channels without task support are
	//    skipped.
	var platform string
	var selectedAdaptor channel.TaskAdaptor
	var taskReq *dto.TaskRequest
	for {
		ch, err := flow.pick()
		if err != nil {
			break
		}
		flow.begin(ch)
		flow.unsupported++
		if ch.Type == channel.ChannelTypeTaskPlugin {
			if !submitPluginTask(flow, ch) {
				continue
			}
			return
		}
		adaptor, err := relay.GetTaskAdaptor(ch.Type)
		if err != nil {
			continue
		}
		parsed := &dto.TaskRequest{}
		if err := adaptor.ValidateRequestAndSetAction(req.body, parsed); err != nil {
			flow.errorOut(http.StatusBadRequest, "invalid_request_error", "invalid_request_body", err.Error())
			return
		}
		selectedAdaptor = adaptor
		platform = adaptor.Platform()
		taskReq = parsed
		break
	}
	if taskReq == nil {
		flow.exhausted()
		return
	}

	// 2. EstimateBilling → pre-charge. The deferred refund returns the
	//    reservation when no channel accepts the submit.
	billingGroup := flow.settleGroup()
	priceData := req.priceFor(billingGroup)
	pricing := selectedAdaptor.EstimateBillingWithOptions(req.modelName, billingGroup, taskReq, channel.TaskPricingOptions{ModelPriceUSD: priceData.ModelPriceUSD, ModelPriceSet: priceData.HasModelPrice, Expression: priceData.PriceExpr})
	if priceData.HasIndependentTaskPricing() {
		pricing = relay.TaskTokenEstimate(taskReq)
	}
	pre, err := relay.PreConsumeQuotaAmount(req.userID, req.tokenID, req.tokenUnlimited,
		relay.TaskPricingQuotaWithPriceData(priceData, pricing))
	if err != nil {
		writePreConsumeError(w, "", err)
		return
	}
	flow.preQuota = pre
	defer flow.refundUnlessSettled()

	// 3. Submit with the normal failover behavior.
	flow.tried = map[int64]bool{}
	flow.trail = nil
	flow.unsupported = 0
	flow.lastErr = nil
	flow.runChannels(func(ch *model.Channel) attemptResult {
		return taskSubmitAttempt(flow, ch, platform, taskReq, pricing)
	})
}

// taskSubmitAttempt submits the validated request to one channel. The task
// row is persisted in "submitting" status BEFORE the upstream call so a
// persistence failure or process exit cannot orphan an accepted upstream task.
// Side-effecting failures (upstream accepted the request but local processing
// failed, or a transport error after the send) return attemptUncertain to
// prevent unsafe cross-channel retries.
func taskSubmitAttempt(flow *relayFlow, ch *model.Channel, platform string, taskReq *dto.TaskRequest, accepted channel.TaskPricing) attemptResult {
	adaptor, err := relay.GetTaskAdaptor(ch.Type)
	if err != nil || adaptor.Platform() != platform {
		return attemptResult{outcome: attemptUnsupported,
			cause: fmt.Errorf("channel %d does not serve platform %q", ch.ID, platform)}
	}
	priceData := flow.req.priceFor(flow.settleGroup())
	pricingMeta, err := json.Marshal(relay.TaskBillingSnapshot{PriceData: *priceData, Estimate: accepted})
	if err != nil {
		return attemptResult{outcome: attemptUpstreamError, cause: fmt.Errorf("encode task pricing: %v", err)}
	}

	body, err := adaptor.ConvertSubmitRequest(taskReq)
	if err != nil {
		return attemptResult{outcome: attemptUpstreamError,
			cause: fmt.Errorf("channel %d: build submit: %v", ch.ID, err)}
	}
	key, keyIdx := flow.selectKey(ch)

	// Persist the task with its pre-charge BEFORE submitting upstream.
	// Status "submitting" protects the reservation through process restarts
	// and tracks the task if the upstream call is uncertain. The stored
	// key index lets the terminal poll reuse the same upstream account.
	requestMeta, _ := json.Marshal(taskReq)
	task := &model.Task{
		TaskID:         "",
		Platform:       platform,
		Action:         taskReq.Action,
		Status:         model.TaskStatusSubmitting,
		Model:          taskReq.Model,
		RequestedModel: flow.req.requestedModel,
		Protocol:       flow.req.protocol,
		UserID:         flow.req.userID,
		TokenID:        flow.req.tokenID,
		TokenName:      flow.req.tokenName,
		TokenUnlimited: flow.req.tokenUnlimited,
		ChannelID:      ch.ID,
		KeyIndex:       keyIdx,
		Group:          flow.settleGroup(),
		UserGroup:      flow.req.userGroup,
		Quota:          flow.preQuota,
		RequestMeta:    string(requestMeta),
		PricingMeta:    string(pricingMeta),
	}
	if err := model.InsertTask(task); err != nil {
		log.Printf("task persistence failed before upstream submit: %v", err)
		flow.lastErr = fmt.Errorf("task persistence failed: %v", err)
		return attemptResult{outcome: attemptUpstreamError, cause: flow.lastErr}
	}

	// The pre-charge now belongs to the task row; the poll-side settlement
	// balances it (failed tasks refund fully).
	flow.settled = true

	completed := false
	defer func() {
		if !completed {
			reason := "task submission failed; upstream execution may be uncertain"
			if _, ferr := service.FailTaskSubmission(task, reason); ferr != nil {
				log.Printf("task %d submission refund pending: %v", task.ID, ferr)
			}
		}
	}()

	resp, fail := flow.callUpstream(ch, adaptor, upstreamCall{
		method:     http.MethodPost,
		url:        adaptor.SubmitURL(ch.BaseURL),
		body:       body,
		key:        key,
		keyIdx:     keyIdx,
		sideEffect: true,
	})
	if fail != nil {
		return *fail
	}
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()

	taskID, err := adaptor.ParseSubmitResponse(resp.StatusCode, respBody)
	if err != nil {
		log.Printf("task submit: upstream %d accepted but parse submit response failed: %v", ch.ID, err)
		flow.lastErr = fmt.Errorf("channel %d: parse submit response: %v", ch.ID, err)
		dto.WriteUpstreamError(flow.w, http.StatusBadGateway, flow.inbound,
			respBody, resp.Header.Get("Content-Type"))
		return attemptResult{outcome: attemptUncertain, statusCode: resp.StatusCode}
	}

	// Bind the upstream task ID and advance to "submitted".
	if err := model.CompleteTaskSubmission(task.ID, taskID); err != nil {
		log.Printf("task %q accepted by upstream %d but ID persistence failed: %v", taskID, ch.ID, err)
		flow.lastErr = fmt.Errorf("task accepted but ID not persisted: %v", err)
		dto.WriteRelayError(flow.w, http.StatusInternalServerError, flow.inbound,
			"server_error", "persistence_failed",
			"task was accepted by upstream but its ID could not be saved locally")
		return attemptResult{outcome: attemptUncertain, statusCode: http.StatusInternalServerError}
	}

	completed = true
	task.TaskID = taskID
	task.Status = dto.TaskStatusSubmitted
	writeTaskSubmitResponse(flow.w, task)
	return attemptResult{outcome: attemptSuccess, statusCode: http.StatusOK}
}

func writeTaskSubmitResponse(w http.ResponseWriter, task *model.Task) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(dto.TaskSubmitResponse{
		ID:       fmt.Sprintf("%d", task.ID),
		Status:   task.Status,
		Progress: task.Progress,
	})
}

// RelayTaskGetVideo serves GET /v1/video/generations/{id}: non-terminal
// tasks are polled upstream (progress refresh + terminal settlement),
// terminal tasks return the stored result.
func RelayTaskGetVideo(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	userID := requestUserID(r)
	task, err := model.GetTaskByID(id)
	if err != nil || task.UserID != userID {
		writeModeError(w, http.StatusNotFound, "invalid_request_error", "task_not_found",
			"no such task")
		return
	}
	if !dto.IsTerminalTaskStatus(task.Status) {
		if _, err := service.PollTask(r.Context(), task); err != nil {
			log.Printf("poll task %d failed: %v", task.ID, err)
		}
	}
	writeTaskView(w, task)
}

func writeTaskView(w http.ResponseWriter, task *model.Task) {
	view := dto.TaskView{
		ID:           fmt.Sprintf("%d", task.ID),
		Status:       task.Status,
		Progress:     task.Progress,
		FailReason:   task.FailReason,
		Model:        task.Model,
		SubmitTime:   task.SubmitTime,
		CompleteTime: task.CompleteTime,
	}
	var billing dto.TaskResult
	if json.Unmarshal([]byte(task.ResultMeta), &billing) == nil {
		view.BillingUsageSource = billing.BillingUsageSource
		view.Usage = billing.Usage
	}
	if outputs := model.TaskOutputURLs(task.ResultMeta); len(outputs) > 0 {
		view.Outputs = outputs
		if task.Action == dto.TaskActionVideo {
			view.VideoURL = outputs[0]
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(view)
}
