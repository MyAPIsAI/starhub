package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"starhub/common"
	"starhub/model"
	"starhub/relay"
)

type channelPayload struct {
	Name     string `json:"name"`
	Type     *int   `json:"type"`
	Key      string `json:"key"`
	BaseURL  string `json:"base_url"`
	Models   string `json:"models"`
	Group    string `json:"group"`
	Status   *int   `json:"status"`
	Priority *int64 `json:"priority"`
	Weight   *int64 `json:"weight"`
}

func CreateChannel(w http.ResponseWriter, r *http.Request) {
	var req channelPayload
	if !decodeJSONBody(w, r, &req) {
		return
	}
	channelType := common.DefaultChannelType
	if req.Type != nil && *req.Type != 0 {
		channelType = *req.Type
	}
	if channelType != common.DefaultChannelType {
		common.ApiError(w, http.StatusBadRequest, "only channel type 1 (OpenAI-compatible) is supported in P0")
		return
	}
	models := model.SplitList(req.Models)
	if len(models) == 0 {
		common.ApiError(w, http.StatusBadRequest, "models must be a non-empty comma-separated list")
		return
	}
	group := strings.TrimSpace(req.Group)
	if group == "" {
		group = common.DefaultGroup
	}
	if len(model.SplitList(group)) == 0 {
		common.ApiError(w, http.StatusBadRequest, "invalid group")
		return
	}
	baseURL, ok := normalizeBaseURL(w, req.BaseURL)
	if !ok {
		return
	}
	status := common.ChannelStatusEnabled
	if req.Status != nil && *req.Status != 0 {
		status = *req.Status
	}
	if status != common.ChannelStatusEnabled && status != common.ChannelStatusManuallyDisabled {
		common.ApiError(w, http.StatusBadRequest, "invalid status")
		return
	}
	priority := int64(0)
	if req.Priority != nil {
		priority = *req.Priority
	}
	weight := int64(1)
	if req.Weight != nil {
		weight = *req.Weight
	}
	ch := &model.Channel{
		Name:     strings.TrimSpace(req.Name),
		Type:     channelType,
		Key:      strings.TrimSpace(req.Key),
		BaseURL:  baseURL,
		Models:   strings.Join(models, ","),
		Group:    strings.Join(model.SplitList(group), ","),
		Status:   status,
		Priority: priority,
		Weight:   weight,
	}
	if err := model.InsertChannel(ch); err != nil {
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.ApiSuccess(w, maskedChannel(ch))
}

func ListChannels(w http.ResponseWriter, r *http.Request) {
	channels, err := model.ListChannels()
	if err != nil {
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	data := make([]map[string]any, 0, len(channels))
	for i := range channels {
		data = append(data, maskedChannel(&channels[i]))
	}
	common.ApiSuccess(w, data)
}

func GetChannel(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	ch, err := model.GetChannelByID(id)
	if err != nil {
		common.ApiError(w, http.StatusNotFound, "channel not found")
		return
	}
	common.ApiSuccess(w, maskedChannel(ch))
}

type channelUpdateRequest struct {
	ID int64 `json:"id"`
	channelPayload
}

func UpdateChannel(w http.ResponseWriter, r *http.Request) {
	var req channelUpdateRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.ID <= 0 {
		common.ApiError(w, http.StatusBadRequest, "channel id is required")
		return
	}
	ch, err := model.GetChannelByID(req.ID)
	if err != nil {
		common.ApiError(w, http.StatusNotFound, "channel not found")
		return
	}
	if req.Name != "" {
		ch.Name = strings.TrimSpace(req.Name)
	}
	if req.Key != "" {
		ch.Key = strings.TrimSpace(req.Key)
	}
	if req.BaseURL != "" {
		baseURL, ok := normalizeBaseURL(w, req.BaseURL)
		if !ok {
			return
		}
		ch.BaseURL = baseURL
	}
	if req.Models != "" {
		models := model.SplitList(req.Models)
		if len(models) == 0 {
			common.ApiError(w, http.StatusBadRequest, "models must be a non-empty comma-separated list")
			return
		}
		ch.Models = strings.Join(models, ",")
	}
	if req.Group != "" {
		groups := model.SplitList(req.Group)
		if len(groups) == 0 {
			common.ApiError(w, http.StatusBadRequest, "invalid group")
			return
		}
		ch.Group = strings.Join(groups, ",")
	}
	if req.Status != nil && *req.Status != 0 {
		if *req.Status != common.ChannelStatusEnabled && *req.Status != common.ChannelStatusManuallyDisabled {
			common.ApiError(w, http.StatusBadRequest, "invalid status")
			return
		}
		ch.Status = *req.Status
	}
	if req.Priority != nil {
		ch.Priority = *req.Priority
	}
	if req.Weight != nil {
		ch.Weight = *req.Weight
	}
	if err := model.UpdateChannel(ch); err != nil {
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.ApiSuccess(w, maskedChannel(ch))
}

func DeleteChannel(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if err := model.DeleteChannel(id); err != nil {
		if err == model.ErrChannelNotFound {
			common.ApiError(w, http.StatusNotFound, "channel not found")
			return
		}
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.ApiSuccess(w, nil)
}

var channelTestHTTPClient = &http.Client{Timeout: 30 * time.Second}

type channelTestResult struct {
	OK        bool   `json:"ok"`
	Model     string `json:"model"`
	ElapsedMs int64  `json:"elapsed_ms"`
	Message   string `json:"message"`
	Status    int    `json:"status"`
}

func TestChannel(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	ch, err := model.GetChannelByID(id)
	if err != nil {
		common.ApiError(w, http.StatusNotFound, "channel not found")
		return
	}
	adaptor, err := relay.GetAdaptor(ch.Type)
	if err != nil {
		common.ApiError(w, http.StatusBadRequest, err.Error())
		return
	}
	available := model.SplitList(ch.Models)
	if len(available) == 0 {
		common.ApiError(w, http.StatusBadRequest, "channel has no models")
		return
	}
	modelName := r.URL.Query().Get("model")
	if modelName == "" {
		modelName = available[0]
	} else if !modelInList(modelName, available) {
		common.ApiError(w, http.StatusBadRequest, "model does not belong to this channel")
		return
	}
	probe, err := json.Marshal(map[string]any{
		"model":      modelName,
		"messages":   []map[string]string{{"role": "user", "content": "ping"}},
		"max_tokens": 1,
	})
	if err != nil {
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost,
		adaptor.RequestURL(ch.BaseURL), bytes.NewReader(probe))
	if err != nil {
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	adaptor.SetupRequestHeader(req, ch.Key)

	result := channelTestResult{Model: modelName, Status: ch.Status}
	start := time.Now()
	resp, err := channelTestHTTPClient.Do(req)
	result.ElapsedMs = time.Since(start).Milliseconds()
	if err != nil {
		result.Message = "request failed: " + err.Error()
		common.ApiSuccess(w, result)
		return
	}
	defer resp.Body.Close()
	if err := model.UpdateChannelResponseTime(ch.ID, result.ElapsedMs); err != nil {
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if resp.StatusCode >= http.StatusBadRequest {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		result.Message = fmt.Sprintf("upstream returned %d: %s",
			resp.StatusCode, strings.TrimSpace(string(body)))
		if (resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden) &&
			ch.Status == common.ChannelStatusEnabled {
			if err := model.UpdateChannelStatus(ch.ID, common.ChannelStatusAutoDisabled); err == nil {
				result.Status = common.ChannelStatusAutoDisabled
			}
		}
		common.ApiSuccess(w, result)
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	result.OK = true
	result.Message = "ok"
	if ch.Status == common.ChannelStatusAutoDisabled {
		if err := model.UpdateChannelStatus(ch.ID, common.ChannelStatusEnabled); err == nil {
			result.Status = common.ChannelStatusEnabled
		}
	}
	common.ApiSuccess(w, result)
}

func modelInList(name string, models []string) bool {
	for _, m := range models {
		if m == name {
			return true
		}
	}
	return false
}

func normalizeBaseURL(w http.ResponseWriter, raw string) (string, bool) {
	base := strings.TrimRight(strings.TrimSpace(raw), "/")
	if base != "" && !strings.HasPrefix(base, "http://") && !strings.HasPrefix(base, "https://") {
		common.ApiError(w, http.StatusBadRequest, "base_url must start with http:// or https://")
		return "", false
	}
	return base, true
}

func maskedChannel(c *model.Channel) map[string]any {
	return map[string]any{
		"id":            c.ID,
		"name":          c.Name,
		"type":          c.Type,
		"key":           common.MaskKey(c.Key),
		"base_url":      c.BaseURL,
		"models":        c.Models,
		"group":         c.Group,
		"status":        c.Status,
		"priority":      c.Priority,
		"weight":        c.Weight,
		"response_time": c.ResponseTime,
		"used_quota":    c.UsedQuota,
		"created_at":    c.CreatedAt,
	}
}
