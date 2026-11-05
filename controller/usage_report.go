package controller

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"starhub/common"
	"starhub/config"
	"starhub/model"
)

func requestLogFilters(r *http.Request) (model.LogFilters, error) {
	q := r.URL.Query()
	f := model.LogFilters{Model: q.Get("model"), Vendor: q.Get("vendor"), CallType: q.Get("call_type"), Group: q.Get("group"), Status: q.Get("status"), TraceID: q.Get("trace_id"), Query: q.Get("query")}
	for _, v := range []string{f.Model, f.Vendor, f.CallType, f.Group, f.TraceID, f.Query} {
		if len(v) > 256 {
			return f, errors.New("filter exceeds 256 bytes")
		}
	}
	if f.Status != "" && f.Status != "all" && f.Status != "success" && f.Status != "failed" && f.Status != "cancelled" {
		return f, errors.New("invalid status")
	}
	for _, name := range []string{"key_id", "token_id"} {
		if raw := q.Get(name); raw != "" {
			id, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || id <= 0 || f.KeyID != 0 && f.KeyID != id {
				return f, errors.New("invalid key filter")
			}
			f.KeyID = id
		}
	}
	for name, target := range map[string]*int64{"from": &f.Since, "until": &f.Until} {
		if raw := q.Get(name); raw != "" {
			v, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || v < 0 {
				return f, errors.New("invalid date boundary")
			}
			*target = v
		}
	}
	if f.Until > 0 && f.Until <= f.Since {
		return f, errors.New("invalid date range")
	}
	return f, nil
}

func reportDates(r *http.Request, now time.Time, loc *time.Location) (time.Time, time.Time, error) {
	q := r.URL.Query()
	today := now.In(loc)
	end := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, loc)
	start := end.AddDate(0, 0, -6)
	switch q.Get("range") {
	case "today":
		start = end
	case "month":
		start = time.Date(end.Year(), end.Month(), 1, 0, 0, 0, 0, loc)
	case "30d":
		start = end.AddDate(0, 0, -29)
	case "", "7d":
		if raw := q.Get("days"); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil || n < 1 || n > 366 {
				return start, end, errors.New("days must be between 1 and one calendar year")
			}
			start = end.AddDate(0, 0, 1-n)
		}
	case "custom":
		var err error
		start, err = time.ParseInLocation("2006-01-02", q.Get("start_date"), loc)
		if err != nil {
			return start, end, errors.New("invalid start date")
		}
		end, err = time.ParseInLocation("2006-01-02", q.Get("end_date"), loc)
		if err != nil {
			return start, end, errors.New("invalid end date")
		}
	default:
		return start, end, errors.New("invalid report range")
	}
	until := end.AddDate(0, 0, 1)
	if !start.Before(until) || until.After(start.AddDate(1, 0, 0)) {
		return start, until, errors.New("date range exceeds one calendar year")
	}
	return start, until, nil
}

func GetUsageReport(w http.ResponseWriter, r *http.Request) {
	f, err := requestLogFilters(r)
	if err != nil {
		common.ApiError(w, 400, err.Error())
		return
	}
	loc, err := time.LoadLocation(config.RegionTimezone())
	if err != nil {
		common.ApiError(w, 500, "report timezone unavailable")
		return
	}
	start, until, err := reportDates(r, time.Now(), loc)
	if err != nil {
		common.ApiError(w, 400, err.Error())
		return
	}
	f.Since, f.Until = start.Unix(), until.Unix()
	groupBy := r.URL.Query().Get("group_by")
	if groupBy == "" {
		groupBy = "key"
	}
	if !validReportDimension(groupBy) {
		common.ApiError(w, 400, "invalid report dimension")
		return
	}
	granularity := r.URL.Query().Get("granularity")
	if granularity == "" {
		granularity = "day"
	}
	if granularity != "hour" && granularity != "day" {
		common.ApiError(w, 400, "invalid report granularity")
		return
	}
	report, err := model.AggregateUsage(model.DB, requestUserID(r), f, loc, groupBy, granularity)
	if err != nil {
		common.ApiError(w, 500, "usage report unavailable")
		return
	}
	common.ApiSuccess(w, report)
}

func validReportDimension(value string) bool {
	switch value {
	case "key", "model", "vendor", "call_type", "group":
		return true
	}
	return false
}
