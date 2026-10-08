package controller

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"starhub/common"
	"starhub/model"
)

type createExportRequest struct {
	Type      string `json:"type"`
	RangeDays int    `json:"range_days"`
}

func ListExports(w http.ResponseWriter, r *http.Request) {
	userID := int64(0)
	if requestRole(r) < common.RoleAdmin {
		userID = requestUserID(r)
	}
	_ = model.CleanExpiredExports()
	exports, err := model.ListExports(userID)
	if err != nil {
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	data := make([]map[string]any, 0, len(exports))
	for i := range exports {
		data = append(data, exportJSON(&exports[i]))
	}
	common.ApiSuccess(w, data)
}

func CreateExport(w http.ResponseWriter, r *http.Request) {
	var req createExportRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	typ := strings.TrimSpace(req.Type)
	if typ != "requests" && typ != "ledger" && typ != "usage" {
		common.ApiError(w, http.StatusBadRequest, "type must be requests, ledger or usage")
		return
	}
	if req.RangeDays < 1 || req.RangeDays > 31 {
		common.ApiError(w, http.StatusBadRequest, "range_days must be between 1 and 31")
		return
	}
	userID := requestUserID(r)
	now := time.Now().UTC()
	from := now.AddDate(0, 0, -req.RangeDays)
	var content []byte
	var rowCount int
	var err error
	if typ == "ledger" {
		content, rowCount, err = buildLedgerExport(userID, from)
	} else {
		content, rowCount, err = buildLogExport(typ, userID, from)
	}
	if err != nil {
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	sizeKB := (len(content) + 1023) / 1024
	rec := &model.ExportRecord{
		UserID:    userID,
		Type:      typ,
		RangeDays: req.RangeDays,
		RowCount:  rowCount,
		SizeKB:    sizeKB,
		Status:    "completed",
		Content:   content,
	}
	if err := model.InsertExport(rec); err != nil {
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.ApiSuccess(w, exportJSON(rec))
}

func DownloadExport(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	rec, err := model.GetExport(id)
	if err != nil {
		common.ApiError(w, http.StatusNotFound, "export not found")
		return
	}
	if !canManageExport(r, rec) {
		common.ApiError(w, http.StatusForbidden, "cannot download an export owned by another user")
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", rec.Filename))
	w.Header().Set("Content-Length", strconv.Itoa(len(rec.Content)))
	_, _ = w.Write(rec.Content)
}

func canManageExport(r *http.Request, e *model.ExportRecord) bool {
	return requestRole(r) >= common.RoleAdmin || e.UserID == requestUserID(r)
}

func exportJSON(e *model.ExportRecord) map[string]any {
	return map[string]any{
		"id":         e.ID,
		"user_id":    e.UserID,
		"type":       e.Type,
		"range_days": e.RangeDays,
		"row_count":  e.RowCount,
		"size_kb":    e.SizeKB,
		"status":     e.Status,
		"filename":   e.Filename,
		"created_at": e.CreatedAt,
	}
}

func buildLogExport(typ string, userID int64, from time.Time) ([]byte, int, error) {
	logs, err := model.ListLogsSince(userID, from.Unix())
	if err != nil {
		return nil, 0, err
	}
	var buf strings.Builder
	writer := csv.NewWriter(&buf)
	if typ == "usage" {
		writer.Write([]string{"Date", "Requests", "Prompt Tokens", "Completion Tokens", "Quota"})
		dayMap := map[string]*struct {
			date            string
			requests        int64
			promptTokens    int64
			completionToken int64
			quota           int64
		}{}
		var order []string
		for _, l := range logs {
			day := time.Unix(l.CreatedAt, 0).UTC().Format("2006-01-02")
			entry, exists := dayMap[day]
			if !exists {
				entry = &struct {
					date            string
					requests        int64
					promptTokens    int64
					completionToken int64
					quota           int64
				}{date: day}
				dayMap[day] = entry
				order = append(order, day)
			}
			entry.requests++
			entry.promptTokens += l.PromptTokens
			entry.completionToken += l.CompletionTokens
			entry.quota += l.Quota
		}
		for _, day := range order {
			e := dayMap[day]
			writer.Write([]string{
				e.date,
				strconv.FormatInt(e.requests, 10),
				strconv.FormatInt(e.promptTokens, 10),
				strconv.FormatInt(e.completionToken, 10),
				strconv.FormatInt(e.quota, 10),
			})
		}
		writer.Flush()
		return []byte(buf.String()), len(order), nil
	}
	writer.Write([]string{"ID", "Time (UTC)", "User ID", "Token ID", "Token Name", "Model", "Prompt Tokens", "Completion Tokens", "Quota"})
	for _, l := range logs {
		writer.Write([]string{
			strconv.FormatInt(l.ID, 10),
			time.Unix(l.CreatedAt, 0).UTC().Format(time.RFC3339),
			strconv.FormatInt(l.UserID, 10),
			strconv.FormatInt(l.TokenID, 10),
			l.TokenName,
			l.Model,
			strconv.FormatInt(l.PromptTokens, 10),
			strconv.FormatInt(l.CompletionTokens, 10),
			strconv.FormatInt(l.Quota, 10),
		})
	}
	writer.Flush()
	return []byte(buf.String()), len(logs), nil
}

func buildLedgerExport(userID int64, from time.Time) ([]byte, int, error) {
	entries, err := model.ListLedgerSince(userID, from.Unix())
	if err != nil {
		return nil, 0, err
	}
	var buf strings.Builder
	writer := csv.NewWriter(&buf)
	writer.Write([]string{"Ledger ID", "Time (UTC)", "Type", "Amount (nano-USD)", "Balance After (nano-USD)", "Description", "Order ID", "Trace ID"})
	for _, e := range entries {
		writer.Write([]string{
			strconv.FormatInt(e.ID, 10),
			time.Unix(e.CreatedAt, 0).UTC().Format(time.RFC3339),
			e.Type,
			strconv.FormatInt(e.Amount, 10),
			strconv.FormatInt(e.BalanceAfter, 10),
			e.Description,
			e.OrderID,
			e.TraceID,
		})
	}
	writer.Flush()
	return []byte(buf.String()), len(entries), nil
}
