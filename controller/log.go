package controller

import (
	"net/http"
	"strconv"

	"starhub/common"
	"starhub/model"
)

func ListLogs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, _ := strconv.ParseInt(q.Get("p"), 10, 64)
	pageSize, _ := strconv.ParseInt(q.Get("page_size"), 10, 64)
	modelFilter := q.Get("model")
	userID := int64(0)
	if requestRole(r) < common.RoleAdmin {
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
