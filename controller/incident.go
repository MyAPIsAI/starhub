package controller

import (
	"net/http"

	"starhub/common"
	"starhub/model"
)

type createIncidentRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Severity    string `json:"severity"`
	Component   string `json:"component"`
}

type addIncidentUpdateRequest struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// CreateIncidentHandler handles POST /api/incident (AdminAuth + Audit).
func CreateIncidentHandler(w http.ResponseWriter, r *http.Request) {
	var req createIncidentRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	incident := &model.ServiceIncident{
		Title:       req.Title,
		Description: req.Description,
		Severity:    req.Severity,
		Component:   req.Component,
		CreatedBy:   requestUserID(r),
	}
	if err := model.CreateIncident(incident); err != nil {
		common.ApiError(w, http.StatusBadRequest, err.Error())
		return
	}
	common.ApiSuccess(w, incident)
}

// AddIncidentUpdateHandler handles PUT /api/incident/{id} (AdminAuth + Audit).
func AddIncidentUpdateHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var req addIncidentUpdateRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	update := &model.ServiceIncidentUpdate{
		IncidentID: id,
		Status:     req.Status,
		Message:    req.Message,
		CreatedBy:  requestUserID(r),
	}
	if err := model.AddIncidentUpdate(update); err != nil {
		common.ApiError(w, http.StatusBadRequest, err.Error())
		return
	}
	common.ApiSuccess(w, update)
}

// ListIncidentsHandler handles GET /api/incident (AdminAuth).
func ListIncidentsHandler(w http.ResponseWriter, r *http.Request) {
	incidents, err := model.ListIncidents(20)
	if err != nil {
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.ApiSuccess(w, incidents)
}

// GetIncidentHandler handles GET /api/incident/{id} (AdminAuth).
func GetIncidentHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	incident, updates, err := model.GetIncident(id)
	if err != nil {
		common.ApiError(w, http.StatusNotFound, err.Error())
		return
	}
	common.ApiSuccess(w, map[string]any{
		"incident": incident,
		"updates":  updates,
	})
}

// PublicIncidentsHandler handles GET /api/status/incidents (public, no auth).
func PublicIncidentsHandler(w http.ResponseWriter, r *http.Request) {
	incidents, err := model.ListPublicIncidents()
	if err != nil {
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.ApiSuccess(w, incidents)
}
