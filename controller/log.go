package controller

import (
	"encoding/json"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"

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
	resourceFilter := q.Get("resource_id")
	userID := int64(0)
	if !common.CanReadCustomerCalls(requestRole(r)) {
		if resourceFilter != "" {
			common.ApiError(w, http.StatusForbidden, "resource filters require staff access")
			return
		}
		userID = requestUserID(r)
	}
	logs, total, err := model.ListResourceLogs(userID, page, pageSize, modelFilter, resourceFilter)
	if err != nil {
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var items any = logs
	if common.CanReadCustomerCalls(requestRole(r)) {
		items = adminCallRows(logs)
	} else {
		items = customerLogs(logs)
	}
	common.ApiSuccess(w, map[string]any{
		"items":     items,
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
	filters, err := requestLogFilters(r)
	if err != nil {
		common.ApiError(w, 400, err.Error())
		return
	}
	var rows []model.Log
	var total int64
	cutoff := time.Now().AddDate(-1, 0, 0).Unix()
	if filters.Since < cutoff {
		filters.Since = cutoff
	}
	query := model.FilterLogs(model.DB, requestUserID(r), filters)
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
	common.ApiSuccess(w, map[string]any{"items": customerLogs(rows), "total": total, "page": page, "page_size": size})
}

// GetOwnedLog resolves a deep link without relying on the first history page.
func GetOwnedLog(w http.ResponseWriter, r *http.Request) {
	f := model.LogFilters{TraceID: r.PathValue("trace"), Since: time.Now().AddDate(-1, 0, 0).Unix()}
	var row model.Log
	if err := model.FilterLogs(model.DB, requestUserID(r), f).First(&row).Error; err != nil {
		common.ApiError(w, 404, "request not found")
		return
	}
	rows := []model.Log{row}
	if err := model.MarkDeletedLogTokens(rows); err != nil {
		common.ApiError(w, 500, "request metadata unavailable")
		return
	}
	common.ApiSuccess(w, customerLogs(rows)[0])
}
func customerLogs(rows []model.Log) []map[string]any {
	result := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		data, _ := json.Marshal(row)
		var item map[string]any
		decoder := json.NewDecoder(strings.NewReader(string(data)))
		decoder.UseNumber()
		_ = decoder.Decode(&item)
		delete(item, "channel_id")
		delete(item, "used_channels")
		for _, key := range []string{"resource_id", "resource_ids", "snapshot_sequence", "resource_revision", "affinity_used", "routing_recorded"} {
			delete(item, key)
		}
		item["auto_switched"] = strings.Contains(row.UsedChannels, "->")
		item["amount_nano"] = new(big.Int).Mul(big.NewInt(row.Quota), big.NewInt(1_000_000_000/common.QuotaPerUSD)).String()
		if row.TokenDeleted {
			item["token_name"] = "Deleted key"
		}
		result = append(result, item)
	}
	return result
}
