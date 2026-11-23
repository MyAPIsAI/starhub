package controller

import (
	"net/http"
	"strconv"
	"strings"

	"starhub/common"
	"starhub/model"
)

func ListLogs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, _ := strconv.ParseInt(q.Get("p"), 10, 64)
	pageSize, _ := strconv.ParseInt(q.Get("page_size"), 10, 64)
	if page < 1 {
		page = 1
	}
	if page > 1000000000 {
		common.ApiError(w, http.StatusBadRequest, "page is too large")
		return
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	modelFilter := q.Get("model")
	userID := int64(0)
	if !common.IsAdminRole(requestRole(r)) {
		userID = requestUserID(r)
	}
	filters := model.LogFilters{UserID: userID, Model: modelFilter, Query: strings.TrimSpace(q.Get("query"))}
	if len(filters.Query) > 256 {
		common.ApiError(w, http.StatusBadRequest, "search is too long")
		return
	}
	switch q.Get("status") {
	case "", "all":
	case "success":
		filters.Status = 1
	case "failed":
		filters.Status = 2
	case "cancelled":
		filters.Status = 3
	default:
		common.ApiError(w, http.StatusBadRequest, "invalid request status filter")
		return
	}
	for key, target := range map[string]*int64{"from": &filters.From, "until": &filters.Until} {
		if raw := q.Get(key); raw != "" {
			value, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || value < 0 {
				common.ApiError(w, http.StatusBadRequest, "invalid time filter")
				return
			}
			*target = value
		}
	}
	if filters.Until > 0 && filters.From >= filters.Until {
		common.ApiError(w, http.StatusBadRequest, "invalid time range")
		return
	}
	logs, total, summary, err := model.ListFilteredLogs(r.Context(), filters, page, pageSize)
	if err != nil {
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.ApiSuccess(w, map[string]any{
		"filtering_supported": true,
		"items":               logs,
		"total":               total,
		"page":                page,
		"page_size":           pageSize,
		"summary":             summary,
	})
}
