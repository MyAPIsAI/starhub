package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"starhub/common"
	"starhub/model"
	"starhub/relay"
	"starhub/relay/dto"
	"starhub/setting"
)

type resourceObservationTask struct {
	ProtocolVersion    int    `json:"protocol_version"`
	LeaseStartedAt     int64  `json:"lease_started_at"`
	ResourceKey        string `json:"resource_key"`
	Epoch              string `json:"epoch"`
	ProbeID            string `json:"probe_id"`
	EndpointGeneration string `json:"endpoint_generation"`
	LeaseUntil         int64  `json:"lease_until"`
	TTFTInputCapTokens int    `json:"ttft_input_cap_tokens"`
	TTFTTimingSchema   string `json:"ttft_timing_schema"`
}

type resourceObservationResult struct {
	resourceObservationTask
	ElapsedMS       int64  `json:"elapsed_ms"`
	InstanceID      string `json:"instance_id"`
	Source          string `json:"source"`
	Outcome         string `json:"outcome"`
	Cause           string `json:"cause"`
	ObservedAt      int64  `json:"observed_at"`
	TTFTMS          int64  `json:"ttft_ms,omitempty"`
	TTFTInputTokens int64  `json:"ttft_input_tokens,omitempty"`
	TTFTSource      string `json:"ttft_source,omitempty"`
}

func resourceSource() string {
	source := os.Getenv("ROUTING_PROTECTION_SOURCE")
	if source == "" {
		source = "talen"
	}
	return source
}

func RunResourceObservations(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	cursor := 0
	for {
		if setting.IsCommercialMode() && os.Getenv("RESOURCE_CATALOG_NOTIFY_URL") != "" && os.Getenv("ROUTING_OBSERVATION_V2_ENABLED") != "0" {
			if err := runResourceObservationOnce(ctx, &cursor); err != nil {
				log.Printf("resource observation failed: %v", err)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func runResourceObservationOnce(ctx context.Context, cursor *int) error {
	var work model.ResourceObservationWork
	err := model.DB.Where("phase IN ?", []string{"claiming", "executing", "result"}).Order("created_at, probe_id").First(&work).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		items := model.RoutingObservationItems()
		if len(items) == 0 {
			return nil
		}
		for scanned := 0; scanned < len(items); scanned++ {
			item := items[*cursor%len(items)]
			*cursor = (*cursor + 1) % len(items)
			if item.Protection == nil || item.Protection.CatalogExcluded || item.Protection.Mode == "manual" {
				continue
			}
			id, _ := strconv.ParseInt(item.ChannelID, 10, 64)
			work = model.ResourceObservationWork{ProbeID: uuid.NewString(), ResourceKey: model.RoutingResourceKey(item), ChannelID: id, ModelName: item.ModelName, EndpointGeneration: item.Protection.EndpointGeneration, Phase: "claiming", CreatedAt: time.Now().Unix()}
			if err := model.DB.Create(&work).Error; err != nil {
				return err
			}
			break
		}
		if work.ProbeID == "" {
			return nil
		}
	} else if err != nil {
		return err
	}
	if work.Phase == "claiming" {
		claim := map[string]any{"source": resourceSource(), "instance_id": os.Getenv("NODE_NAME"), "resource_key": work.ResourceKey, "endpoint_generation": work.EndpointGeneration, "probe_id": work.ProbeID}
		var response struct {
			Task *resourceObservationTask `json:"task"`
		}
		if err := postResource(ctx, "/internal/routing/observations/v2/claim", claim, &response); err != nil {
			return err
		}
		if response.Task == nil {
			return model.DB.Model(&work).Update("phase", "done").Error
		}
		task := *response.Task
		if task.ProtocolVersion != 2 || task.ProbeID != work.ProbeID || task.ResourceKey != work.ResourceKey || task.EndpointGeneration != work.EndpointGeneration || task.LeaseUntil <= task.LeaseStartedAt {
			return errors.New("invalid observation lease")
		}
		raw, _ := json.Marshal(task)
		work.Task = string(raw)
		work.Phase = "executing"
		if err := model.DB.Save(&work).Error; err != nil {
			return err
		}
		result := executeResourceObservation(ctx, work, task)
		raw, _ = json.Marshal(result)
		work.Result = string(raw)
		work.Phase = "result"
		if err := model.DB.Save(&work).Error; err != nil {
			return err
		}
	} else if work.Phase == "executing" {
		// A previous process may have sent this probe. Report uncertainty instead
		// of replaying a potentially completed operation after restart.
		var task resourceObservationTask
		if err := json.Unmarshal([]byte(work.Task), &task); err != nil {
			return err
		}
		result := resourceObservationResult{resourceObservationTask: task, InstanceID: os.Getenv("NODE_NAME"), Source: resourceSource(), Outcome: "unknown", Cause: "worker_restarted"}
		raw, _ := json.Marshal(result)
		work.Result = string(raw)
		work.Phase = "result"
		if err := model.DB.Save(&work).Error; err != nil {
			return err
		}
	}
	if work.Phase == "result" {
		var ack struct {
			Accepted bool   `json:"accepted"`
			ProbeID  string `json:"probe_id"`
		}
		if err := postResource(ctx, "/internal/routing/observations/v2/results", json.RawMessage(work.Result), &ack); err != nil {
			var status resourceHTTPError
			if errors.As(err, &status) && (status == 409 || status == 400) {
				// A fenced or invalid result is terminal; retain it for inspection.
				return model.DB.Model(&work).Updates(map[string]any{"phase": "rejected", "last_error": err.Error()}).Error
			}
			return err
		}
		if !ack.Accepted || ack.ProbeID != work.ProbeID {
			return errors.New("observation result acknowledgement mismatch")
		}
		if err := model.DB.Model(&work).Update("phase", "done").Error; err != nil {
			return err
		}
	}
	return model.DB.Where("phase IN ? AND created_at < ?", []string{"done", "rejected"}, time.Now().Add(-7*24*time.Hour).Unix()).Delete(&model.ResourceObservationWork{}).Error
}

func executeResourceObservation(ctx context.Context, work model.ResourceObservationWork, task resourceObservationTask) (result resourceObservationResult) {
	result = resourceObservationResult{resourceObservationTask: task, InstanceID: os.Getenv("NODE_NAME"), Source: resourceSource(), Outcome: "unknown", Cause: "unavailable"}
	start := time.Now()
	defer func() {
		result.ElapsedMS = min(int64(60000), max(int64(1), time.Since(start).Milliseconds()))
		result.ObservedAt = time.Now().Unix()
	}()
	if task.LeaseUntil <= time.Now().Add(30*time.Second).Unix() {
		result.Cause = "lease_expired"
		return
	}
	ch, err := model.GetChannelByID(work.ChannelID)
	if err != nil || model.ChannelEndpointGeneration(ch) != work.EndpointGeneration {
		return
	}
	adaptor, err := relay.GetChatAdaptor(ch.Type)
	if err != nil {
		result.Cause = "unsupported_protocol"
		return
	}
	probe := &dto.ChatRequest{Model: work.ModelName, Stream: true, Messages: []dto.ChatMessage{{Role: "user", Text: "ping"}}, MaxTokens: 1}
	body, err := adaptor.ConvertChatRequest(probe)
	if err != nil {
		return
	}
	probeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	key, _ := ch.SelectKey()
	req, err := http.NewRequestWithContext(probeCtx, http.MethodPost, adaptor.ChatURL(ch.BaseURL, key, work.ModelName, true), bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if err := adaptor.PrepareRequest(req, key, body); err != nil {
		return
	}
	response, err := probeHTTPClient.Do(req)
	if err != nil {
		result.Outcome = "failure"
		result.Cause = "transport_error"
		return
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		result.Outcome = "failure"
		result.Cause = "upstream_status"
		return
	}
	var first time.Duration
	inputTokens := int64(0)
	complete := false
	if common.AssertJSONContentType(response.Header.Get("Content-Type")) {
		parsed, err := adaptor.ParseChatResponse(response)
		if err != nil {
			return
		}
		if parsed.Text != "" {
			first = time.Since(start)
		}
		if parsed.Usage != nil {
			inputTokens = parsed.Usage.PromptTokens
		}
		complete = true
	} else {
		reader, err := adaptor.StreamChatEvents(response)
		if err != nil {
			return
		}
		defer reader.Close()
		for {
			event, err := reader.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				result.Outcome = "failure"
				result.Cause = "stream_error"
				return
			}
			if event.TextDelta != "" && first == 0 {
				first = time.Since(start)
			}
			if event.Usage != nil {
				inputTokens = event.Usage.PromptTokens
			}
			if event.FinishReason != "" {
				complete = true
			}
		}
	}
	if !complete || first == 0 {
		result.Outcome = "failure"
		result.Cause = "missing_content"
		return
	}
	result.Outcome = "success"
	result.Cause = "completed"
	if inputTokens > 0 && inputTokens <= int64(task.TTFTInputCapTokens) && (task.TTFTTimingSchema == "content_event_v1" || task.TTFTTimingSchema == "content_event_v2") {
		result.TTFTMS = max(int64(1), first.Milliseconds())
		result.TTFTInputTokens = inputTokens
		result.TTFTSource = task.TTFTTimingSchema
	}
	return
}
