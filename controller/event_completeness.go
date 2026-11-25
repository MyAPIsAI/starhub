package controller

import (
	"net/http"
	"strconv"
	"time"

	"starhub/common"
	"starhub/model"
)

func EventCompleteness(w http.ResponseWriter, r *http.Request) {
	end := time.Now().UTC().Truncate(time.Minute).Unix()
	start := end - 24*3600
	for key, dest := range map[string]*int64{"start_timestamp": &start, "end_timestamp": &end} {
		if raw := r.URL.Query().Get(key); raw != "" {
			value, err := strconv.ParseInt(raw, 10, 64)
			if err != nil {
				common.ApiError(w, 400, "invalid completeness window")
				return
			}
			*dest = value
		}
	}
	end = min(end, time.Now().UTC().Truncate(time.Minute).Unix())
	if start < time.Now().UTC().Truncate(time.Minute).Unix()-24*3600 || start >= end || start < 1 {
		common.ApiError(w, 400, "completeness window must be within 24 hours")
		return
	}
	data, err := model.ReadEventCompleteness(r.Context(), start, end)
	if err != nil {
		common.ApiError(w, 503, "event completeness unavailable")
		return
	}
	data.Runtime, err = model.ReadRuntimeAuditSummary(r.Context(), end-1800, end)
	if err != nil {
		common.ApiError(w, 503, "runtime audit unavailable")
		return
	}
	common.ApiSuccess(w, data)
}
