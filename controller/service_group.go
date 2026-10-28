package controller

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"

	"starhub/common"
	"starhub/model"
)

type serviceGroupRequest struct {
	GroupID     string   `json:"group_id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Enabled     *bool    `json:"enabled"`
	Visibility  string   `json:"visibility"`
	AccountIDs  []string `json:"account_ids"`
	Multiplier  string   `json:"multiplier"`
}

// ListServiceGroups returns all service group configs.
func ListServiceGroups(w http.ResponseWriter, r *http.Request) {
	groups, err := model.ListServiceGroups()
	if err != nil {
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	data := make([]map[string]any, 0, len(groups))
	for i := range groups {
		data = append(data, serviceGroupJSON(&groups[i]))
	}
	common.ApiSuccess(w, data)
}

// UpsertServiceGroup creates or updates a service group. It also syncs
// the group_ratio option so relay billing picks up the multiplier.
func UpsertServiceGroup(w http.ResponseWriter, r *http.Request) {
	var req serviceGroupRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	req.GroupID = strings.TrimSpace(req.GroupID)
	if !validGroupID(req.GroupID) {
		common.ApiError(w, http.StatusBadRequest, "group_id must be 2-64 chars of [a-z0-9_-], starting with alphanumeric")
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		common.ApiError(w, http.StatusBadRequest, "name is required")
		return
	}
	multiplier := strings.TrimSpace(req.Multiplier)
	if multiplier == "" {
		multiplier = "1"
	}
	mult, err := strconv.ParseFloat(multiplier, 64)
	if err != nil || math.IsNaN(mult) || math.IsInf(mult, 0) || mult <= 0 || mult > 100 {
		common.ApiError(w, http.StatusBadRequest, "multiplier must be a number > 0 and <= 100")
		return
	}
	visibility := strings.TrimSpace(req.Visibility)
	if visibility == "" {
		visibility = "common"
	}
	if visibility != "common" && visibility != "custom" {
		common.ApiError(w, http.StatusBadRequest, "visibility must be common or custom")
		return
	}
	if visibility == "custom" && len(req.AccountIDs) == 0 {
		common.ApiError(w, http.StatusBadRequest, "custom visibility requires at least one account_id")
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	sgc := &model.ServiceGroupConfig{
		GroupID:     req.GroupID,
		Name:        strings.TrimSpace(req.Name),
		Description: strings.TrimSpace(req.Description),
		Enabled:     enabled,
		Visibility:  visibility,
		AccountIDs:  strings.Join(req.AccountIDs, ","),
		Multiplier:  multiplier,
	}
	actorID, _ := r.Context().Value(common.CtxUserID).(int64)
	if err := model.SaveServiceGroupAndRatio(r.Context(), sgc, actorID); err != nil {
		common.ApiError(w, http.StatusInternalServerError, "service group configuration could not be saved")
		return
	}
	if err := model.LoadOptionsToSettings(); err != nil {
		common.ApiError(w, http.StatusInternalServerError, "group saved, but serving settings could not be reloaded")
		return
	}
	common.ApiSuccess(w, serviceGroupJSON(sgc))
}

// DeleteServiceGroup removes a service group and syncs group_ratio.
func DeleteServiceGroup(w http.ResponseWriter, r *http.Request) {
	groupID := r.PathValue("id")
	if groupID == "" {
		common.ApiError(w, http.StatusBadRequest, "group id is required")
		return
	}
	actorID, _ := r.Context().Value(common.CtxUserID).(int64)
	if err := model.DeleteServiceGroupAndRatio(r.Context(), groupID, actorID); err != nil {
		if errors.Is(err, model.ErrServiceGroupNotFound) {
			common.ApiError(w, http.StatusNotFound, "service group not found")
		} else {
			common.ApiError(w, http.StatusInternalServerError, "service group configuration could not be deleted")
		}
		return
	}
	if err := model.LoadOptionsToSettings(); err != nil {
		common.ApiError(w, http.StatusInternalServerError, "group deleted, but serving settings could not be reloaded")
		return
	}
	common.ApiSuccess(w, nil)
}

// --- helpers ---

func validGroupID(id string) bool {
	if len(id) < 2 || len(id) > 64 {
		return false
	}
	for i, c := range id {
		if i == 0 {
			if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')) {
				return false
			}
		} else if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

func serviceGroupJSON(sgc *model.ServiceGroupConfig) map[string]any {
	var accountIDs []string
	if sgc.AccountIDs != "" {
		accountIDs = strings.Split(sgc.AccountIDs, ",")
	} else {
		accountIDs = []string{}
	}
	return map[string]any{
		"id":          sgc.ID,
		"group_id":    sgc.GroupID,
		"name":        sgc.Name,
		"description": sgc.Description,
		"enabled":     sgc.Enabled,
		"visibility":  sgc.Visibility,
		"account_ids": accountIDs,
		"multiplier":  sgc.Multiplier,
		"created_at":  sgc.CreatedAt,
		"updated_at":  sgc.UpdatedAt,
		"sla":         sgc.ServiceLevel(),
	}
}

func UpdateServiceGroupSLA(w http.ResponseWriter, r *http.Request) {
	var request struct {
		ExpectedGroupID  *int64          `json:"expected_group_id"`
		ExpectedRevision *int64          `json:"expected_revision"`
		Targets          json.RawMessage `json:"targets"`
	}
	if !decodeResourceBody(w, r, &request) {
		return
	}
	groupID := r.PathValue("id")
	if !validGroupID(groupID) || request.ExpectedGroupID == nil || request.ExpectedRevision == nil || len(request.Targets) == 0 {
		common.ApiError(w, http.StatusBadRequest, "group identity, expected_revision and explicit targets are required")
		return
	}
	var target *model.ServiceGroupSLATarget
	decoder := json.NewDecoder(bytes.NewReader(request.Targets))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&target); err != nil {
		common.ApiError(w, http.StatusBadRequest, "invalid service-level targets")
		return
	}
	group, err := model.UpdateServiceGroupSLA(r.Context(), groupID, *request.ExpectedGroupID, *request.ExpectedRevision, target)
	if err != nil {
		status := http.StatusInternalServerError
		message := "service-level targets are unavailable"
		switch {
		case errors.Is(err, model.ErrInvalidSLATarget):
			status, message = http.StatusBadRequest, "success_rate_ppm must be 0..1000000 and p95_latency_ms 1..86400000; use null to unpublish"
		case errors.Is(err, model.ErrSLATargetConflict):
			status, message = http.StatusConflict, err.Error()
		case errors.Is(err, model.ErrServiceGroupNotFound):
			status, message = http.StatusNotFound, err.Error()
		}
		common.ApiError(w, status, message)
		return
	}
	common.ApiSuccess(w, serviceGroupJSON(group))
}
