package controller

import (
	"net/http"
	"strconv"

	"starhub/common"
	"starhub/model"
	"starhub/service/authz"
)

func GetServiceAnnouncements(w http.ResponseWriter, r *http.Request) {
	if !authz.Allow(requestRole(r), authz.ResourceAnnouncements, authz.ActionRead) {
		common.ApiError(w, 403, "announcement privilege required")
		return
	}
	page := 1
	if raw := r.URL.Query().Get("page"); raw != "" {
		var err error
		page, err = strconv.Atoi(raw)
		if err != nil {
			common.ApiError(w, 400, "invalid announcement page")
			return
		}
	}
	status := r.URL.Query().Get("status")
	if status == "" {
		status = "all"
	}
	result, err := model.ListServiceAnnouncements(r.Context(), status, page, false)
	if err != nil {
		adminFailure(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	common.ApiSuccess(w, result)
}

func PutServiceAnnouncement(w http.ResponseWriter, r *http.Request) {
	if !authz.Allow(requestRole(r), authz.ResourceAnnouncements, authz.ActionWrite) {
		common.ApiError(w, 403, "announcement privilege required")
		return
	}
	var id int64
	if r.Method == http.MethodPut {
		var err error
		id, err = strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id <= 0 {
			common.ApiError(w, 400, "invalid announcement id")
			return
		}
	}
	var input model.AnnouncementWrite
	if !decodeResourceBody(w, r, &input) {
		return
	}
	row, err := model.SaveServiceAnnouncement(r.Context(), id, input, requestUserID(r), common.ClientIP(r))
	if err != nil {
		adminFailure(w, err)
		return
	}
	common.ApiSuccess(w, row)
}

func GetPublishedAnnouncements(w http.ResponseWriter, r *http.Request) {
	result, err := model.ListServiceAnnouncements(r.Context(), "active", 1, true)
	if err != nil {
		adminFailure(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	common.ApiSuccess(w, result)
}
