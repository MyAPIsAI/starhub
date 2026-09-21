package controller

import (
	"net/http"
	"strings"

	"starhub/common"
	"starhub/model"
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
		"id":         c.ID,
		"name":       c.Name,
		"type":       c.Type,
		"key":        common.MaskKey(c.Key),
		"base_url":   c.BaseURL,
		"models":     c.Models,
		"group":      c.Group,
		"status":     c.Status,
		"priority":   c.Priority,
		"weight":     c.Weight,
		"used_quota": c.UsedQuota,
		"created_at": c.CreatedAt,
	}
}
