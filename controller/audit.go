package controller

import (
	"net/http"
	"strconv"

	"starhub/common"
	"starhub/model"
)

// ListAuditLogs serves GET /api/audit (admin only).
func ListAuditLogs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, _ := strconv.ParseInt(q.Get("p"), 10, 64)
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.ParseInt(q.Get("page_size"), 10, 64)
	if pageSize < 1 {
		pageSize = 20
	}
	logs, total, err := model.ListAuditLogs(page, pageSize)
	if err != nil {
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.ApiSuccess(w, map[string]any{
		"items": logs,
		"total": total,
	})
}