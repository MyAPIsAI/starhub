package controller

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"starhub/common"
	"starhub/metric"
	"starhub/model"
	"starhub/service"
)

func ReceiveRoutingSnapshot(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Snapshot model.RouteSnapshot `json:"route_snapshot"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<20))
	if err := decoder.Decode(&body); err != nil {
		common.ApiError(w, 400, "invalid routing snapshot body")
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		common.ApiError(w, 400, "unexpected trailing snapshot data")
		return
	}
	if err := model.ApplyRoutingSnapshot(body.Snapshot, time.Now()); err != nil {
		common.ApiError(w, 409, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]any{"success": true, "protection_receipt": map[string]any{
		"snapshot_version": body.Snapshot.SnapshotVersion, "checksum": body.Snapshot.Checksum, "applied": true, "shared": true,
	}})
}

func ResourceRoutingPolicy(w http.ResponseWriter, r *http.Request) {
	group := r.PathValue("group")
	if r.Method == http.MethodPut {
		var p model.RoutingGroupPolicy
		if !decodeJSONBody(w, r, &p) {
			return
		}
		p.GroupID = group
		if err := model.SaveRoutingGroupPolicy(p); err != nil {
			common.ApiError(w, 400, err.Error())
			return
		}
	}
	p, err := model.GetRoutingGroupPolicy(group)
	if err != nil {
		common.ApiError(w, 500, "routing policy unavailable")
		return
	}
	common.ApiSuccess(w, p)
}

func ResourceRoutingStatus(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/api/metric/routing/policy-status" {
		common.ApiSuccess(w, map[string]any{"instances": []any{model.RoutingPolicyInstanceStatus(time.Now())}, "error": ""})
		return
	}
	version, age, ready := model.RoutingSnapshotStatus()
	var states []model.LocalRoutingState
	if err := model.DB.Order("resource_key").Find(&states).Error; err != nil {
		common.ApiError(w, 500, "routing state unavailable")
		return
	}
	var events []model.ResourceEventOutbox
	if err := model.DB.Order("created_at DESC").Limit(100).Find(&events).Error; err != nil {
		common.ApiError(w, 500, "event state unavailable")
		return
	}
	var observations []model.ResourceObservationWork
	if err := model.DB.Omit("task", "result").Order("created_at DESC").Limit(100).Find(&observations).Error; err != nil {
		common.ApiError(w, 500, "observation state unavailable")
		return
	}
	var pending int64
	if err := model.DB.Model(&model.ResourceEventOutbox{}).Where("delivered_at = 0").Count(&pending).Error; err != nil {
		common.ApiError(w, 500, "event count unavailable")
		return
	}
	common.ApiSuccess(w, map[string]any{"instance_id": os.Getenv("NODE_NAME"), "snapshot_version": version, "snapshot_age_seconds": max(0, int64(age.Seconds())), "ready": ready, "fallback": ready && age > 3*time.Minute, "local_states": states, "events": events, "pending_events": pending, "observations": observations, "storage": "sqlite", "scope": "single_instance"})
}

func ResourcePlatformMetrics(w http.ResponseWriter, r *http.Request) {
	byModel := strings.Contains(r.URL.Path, "/model/")
	quarter := strings.HasSuffix(r.URL.Path, "/mysql")
	now := time.Now()
	start, end := now.Add(-24*time.Hour).Unix(), now.Unix()
	if !quarter && !strings.HasSuffix(r.URL.Path, "/attempts") {
		_, explicitStart := r.URL.Query()["start_timestamp"]
		_, explicitEnd := r.URL.Query()["end_timestamp"]
		raw, hasMinutes := r.URL.Query()["minutes"]
		if hasMinutes && (explicitStart || explicitEnd || len(raw) != 1) {
			common.ApiError(w, 400, "minutes cannot be combined with an explicit time range")
			return
		}
		if hasMinutes || (!explicitStart && !explicitEnd) {
			minutes := 15
			if hasMinutes {
				var err error
				minutes, err = strconv.Atoi(raw[0])
				if err != nil || minutes < 1 || minutes > 126 {
					common.ApiError(w, 400, "minutes must be between 1 and 126")
					return
				}
			}
			end = now.Unix() / 60 * 60
			start = end - int64(minutes-1)*60
		}
	}
	for key, target := range map[string]*int64{"start_timestamp": &start, "end_timestamp": &end} {
		if raw := r.URL.Query().Get(key); raw != "" {
			v, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || v < 0 {
				common.ApiError(w, 400, "invalid metric time range")
				return
			}
			*target = v
		}
	}
	if end < start {
		common.ApiError(w, 400, "invalid metric time range")
		return
	}
	if strings.HasSuffix(r.URL.Path, "/attempts") {
		page, size := 1, 100
		for key, target := range map[string]*int{"page": &page, "page_size": &size} {
			if raw := r.URL.Query().Get(key); raw != "" {
				v, e := strconv.Atoi(raw)
				if e != nil || v < 1 || v > 1000000 || key == "page_size" && v > 1000 {
					common.ApiError(w, 400, "invalid attempt page")
					return
				}
				*target = v
			}
		}
		rows, err := model.ReadProductionAttempts(max(start/60*60, time.Now().Add(-24*time.Hour).Unix()/60*60), end/60*60, page, size)
		if err != nil {
			common.ApiError(w, 500, "attempt journal unavailable")
			return
		}
		items := []json.RawMessage{}
		for _, row := range rows {
			if !json.Valid([]byte(row.Payload)) {
				common.ApiError(w, 500, "corrupt attempt journal")
				return
			}
			items = append(items, json.RawMessage(row.Payload))
		}
		common.ApiSuccess(w, map[string]any{"schema": "attempt-metadata-v1", "page": page, "page_size": size, "items": items, "retention_hours": 24, "max_retained_attempts": 100000, "sealed_minutes_only": true})
		return
	}
	if byModel && !quarter {
		buckets, err := service.ProductionMetricBuckets(start, end, time.Now())
		if err != nil {
			common.ApiError(w, 500, "production metrics unavailable")
			return
		}
		common.ApiSuccess(w, map[string]any{"buckets": buckets})
		return
	}
	rows := metric.PlatformMetricRows(byModel, quarter, start, end)
	if quarter {
		page, size := 1, 100
		for key, target := range map[string]*int{"page": &page, "page_size": &size} {
			if raw := r.URL.Query().Get(key); raw != "" {
				v, err := strconv.Atoi(raw)
				if err != nil || v < 1 || v > 1000000 {
					common.ApiError(w, 400, "invalid pagination")
					return
				}
				*target = v
			}
		}
		size = min(size, 1000)
		from := min(len(rows), (page-1)*size)
		to := min(len(rows), from+size)
		common.ApiSuccess(w, map[string]any{"page": page, "page_size": size, "total": len(rows), "items": rows[from:to]})
		return
	}
	if !byModel {
		common.ApiSuccess(w, map[string]any{"buckets": rows, "pending_count": 0})
		return
	}
	buckets := []map[string]any{}
	for i := 0; i < len(rows); {
		j := i + 1
		for j < len(rows) && rows[j].Minute == rows[i].Minute {
			j++
		}
		buckets = append(buckets, map[string]any{"key": rows[i].Key, "minute": rows[i].Minute, "models": rows[i:j]})
		i = j
	}
	common.ApiSuccess(w, map[string]any{"buckets": buckets})
}

// ResourcePlatformProbe intentionally avoids ApplyProbeResult: observation
// traffic cannot modify customer billing, affinity, or production metrics.
func ResourcePlatformProbe(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	ch, err := model.GetChannelByID(id)
	if err != nil {
		common.ApiError(w, 404, "channel not found")
		return
	}
	name := r.URL.Query().Get("model")
	if name == "" || !modelInList(name, model.SplitList(ch.Models)) {
		common.ApiError(w, 400, "model does not belong to channel")
		return
	}
	result := service.ProbeChannel(r.Context(), ch, name)
	common.ApiSuccess(w, map[string]any{"success": result.OK, "ok": result.OK, "time": float64(result.ElapsedMs) / 1000, "elapsed_ms": result.ElapsedMs, "status": result.HTTPStatus})
}

func ResourcePlatformCatalog(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/api/channel/models/changes" {
		since, err := strconv.ParseInt(r.URL.Query().Get("since_revision"), 10, 64)
		if err != nil || since < 0 {
			common.ApiError(w, 400, "invalid since_revision")
			return
		}
		catalog, changes, err := model.PlatformCatalogChanges(since)
		if errors.Is(err, model.ErrCatalogRevisionExpired) {
			common.ApiError(w, 409, err.Error())
			return
		}
		if err != nil {
			common.ApiError(w, 500, "catalog read failed")
			return
		}
		common.ApiSuccess(w, map[string]any{"from_revision": since, "revision": catalog.Revision, "items": changes})
		return
	}
	catalog, err := model.ReadPlatformCatalog()
	if err != nil {
		common.ApiError(w, 500, "catalog read failed")
		return
	}
	if r.URL.Path == "/api/channel/catalog/version" {
		common.ApiSuccess(w, map[string]any{"revision": catalog.Revision, "updated_at": catalog.UpdatedAt})
		return
	}
	common.ApiSuccess(w, catalog)
}

func RotateResourceCredential(w http.ResponseWriter, r *http.Request) {
	if requestRole(r) != common.RoleRoot {
		common.ApiError(w, http.StatusForbidden, "root role required")
		return
	}
	var body struct {
		Token    string `json:"token"`
		Disabled bool   `json:"disabled"`
	}
	if !decodeJSONBody(w, r, &body) {
		return
	}
	if err := model.RotateResourceCredential(body.Token, body.Disabled, time.Now()); err != nil {
		common.ApiError(w, 400, err.Error())
		return
	}
	common.ApiSuccess(w, map[string]any{"disabled": body.Disabled, "overlap_seconds": 600})
}
