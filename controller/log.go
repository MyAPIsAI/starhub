package controller

import (
	"net/http"
	"strconv"

	"starhub/common"
	"starhub/model"
)

func ListLogs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if q.Get("scope") == "owned" {
		listOwnedLogPage(w, r)
		return
	}
	page, _ := strconv.ParseInt(q.Get("p"), 10, 64)
	pageSize, _ := strconv.ParseInt(q.Get("page_size"), 10, 64)
	if page < 1 {
		page = 1
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
	logs, total, err := model.ListLogs(userID, page, pageSize, modelFilter)
	if err != nil {
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.ApiSuccess(w, map[string]any{
		"items":     logs,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// The customer list and export use the same predicates and owner boundary.
func listOwnedLogPage(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("p"))
	if page < 1 {
		page = 1
	}
	if page > 1000000 {
		common.ApiError(w, 400, "invalid page")
		return
	}
	size, _ := strconv.Atoi(q.Get("page_size"))
	if size < 1 {
		size = 20
	}
	if size > 100 {
		size = 100
	}
	spec := exportSpec{UserID: requestUserID(r), createExportRequest: createExportRequest{Status: q.Get("status"), Query: q.Get("query")}}
	if spec.Status != "" && spec.Status != "all" && spec.Status != "success" && spec.Status != "failed" && spec.Status != "cancelled" {
		common.ApiError(w, 400, "invalid status")
		return
	}
	if len(spec.Query) > 256 {
		common.ApiError(w, 400, "search exceeds 256 bytes")
		return
	}
	for name, target := range map[string]*int64{"from": &spec.Since, "until": &spec.Until} {
		if value := q.Get(name); value != "" {
			parsed, err := strconv.ParseInt(value, 10, 64)
			if err != nil || parsed < 0 {
				common.ApiError(w, 400, "invalid date boundary")
				return
			}
			*target = parsed
		}
	}
	if spec.Until > 0 && spec.Until <= spec.Since {
		common.ApiError(w, 400, "invalid date range")
		return
	}
	var rows []model.Log
	var total int64
	query := exportLogsQuery(model.DB, spec)
	if err := query.Count(&total).Error; err != nil {
		common.ApiError(w, 500, "request count unavailable")
		return
	}
	page = min(page, max(1, int((total+int64(size)-1)/int64(size))))
	if err := query.Order("created_at DESC, id DESC").Offset((page - 1) * size).Limit(size).Find(&rows).Error; err != nil {
		common.ApiError(w, 500, "requests unavailable")
		return
	}
	if err := model.MarkDeletedLogTokens(rows); err != nil {
		common.ApiError(w, 500, "request key metadata unavailable")
		return
	}
	common.ApiSuccess(w, map[string]any{"items": rows, "total": total, "page": page, "page_size": size})
}
