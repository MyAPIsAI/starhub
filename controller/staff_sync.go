package controller

import (
	"errors"
	"net/http"
	"runtime"
	"slices"
	"starhub/common"
	"starhub/config"
	"starhub/model"
	"strconv"
	"strings"
	"time"
)

func StaffResourceSyncStatus(w http.ResponseWriter, r *http.Request) {
	status, err := model.ReadResourceSyncStatus(r.Context(), time.Now())
	if err != nil {
		common.ApiError(w, 500, "resource state unavailable")
		return
	}
	common.ApiSuccess(w, status)
}

func StaffResourceMonitoring(w http.ResponseWriter, r *http.Request) {
	pages := []int{1, 1}
	for i, key := range []string{"mismatch_page", "incident_page"} {
		if value := r.URL.Query().Get(key); value != "" {
			page, err := strconv.Atoi(value)
			if err != nil || page < 1 || page > 100000 {
				common.ApiError(w, 400, "invalid page")
				return
			}
			pages[i] = page
		}
	}
	result, err := model.ReadResourceMonitorHistory(r.Context(), pages[0], pages[1])
	if err != nil {
		common.ApiError(w, 500, "resource monitoring history unavailable")
		return
	}
	common.ApiSuccess(w, result)
}

func StaffSystemInfo(w http.ResponseWriter, r *http.Request) {
	instances := []model.OptionInstance{}
	if err := model.DB.WithContext(r.Context()).Where("last_seen >= ?", time.Now().Add(-24*time.Hour).Unix()).Order("id").Find(&instances).Error; err != nil {
		common.ApiError(w, 500, "instance health unavailable")
		return
	}
	connection, err := model.DB.DB()
	healthy := err == nil && connection.PingContext(r.Context()) == nil
	if len(instances) == 0 {
		instances = append(instances, model.OptionInstance{ID: "serving-process", Version: model.LoadedOptionVersion(), LastSeen: time.Now().Unix(), BuildVersion: common.Version, DatabaseHealthy: healthy})
	}
	rows := []map[string]any{}
	for _, instance := range instances {
		status := "healthy"
		if !instance.DatabaseHealthy || time.Now().Unix()-instance.LastSeen > 15 {
			status = "unhealthy"
		}
		rows = append(rows, map[string]any{"id": instance.ID, "version": instance.BuildVersion, "configuration_version": instance.Version, "last_seen": instance.LastSeen, "status": status, "health_checks": map[string]bool{"database": instance.DatabaseHealthy, "heartbeat": time.Now().Unix()-instance.LastSeen <= 15}})
	}
	common.ApiSuccess(w, map[string]any{"version": common.Version, "go_version": runtime.Version(), "timezone": config.RegionTimezone(), "resource_source_configured": config.ResourceSnapshotSource().URL != "", "instances": rows})
}

// Forced synchronization shares the scheduled worker's lease and validation.
func StaffForceResourceSync(w http.ResponseWriter, r *http.Request) {
	if config.ResourceSnapshotSource().URL == "" {
		common.ApiError(w, 503, "resource snapshot source is not configured")
		return
	}
	var body struct {
		Confirm bool `json:"confirm"`
	}
	if !decodeResourceBody(w, r, &body) {
		return
	}
	if !body.Confirm {
		common.ApiError(w, 400, "full synchronization confirmation is required")
		return
	}
	actor, _ := r.Context().Value(common.CtxUserID).(int64)
	status, err := model.SyncResourcesOnce(r.Context(), time.Now(), actor)
	if err != nil {
		code := 502
		if errors.Is(err, model.ErrResourceSyncBusy) || strings.Contains(err.Error(), "version_conflict") {
			code = 409
		}
		if strings.Contains(err.Error(), "invalid_snapshot") || strings.Contains(err.Error(), "invalid_payload") {
			code = 400
		}
		common.ApiError(w, code, err.Error())
		return
	}
	common.ApiSuccess(w, status)
}

func StaffResourceInstances(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	page, size := 1, 20
	if value := query.Get("page"); value != "" {
		var err error
		page, err = strconv.Atoi(value)
		if err != nil || page < 1 || page > 100000 {
			common.ApiError(w, 400, "invalid page")
			return
		}
	}
	tier := 0
	if value := query.Get("tier"); value != "" {
		tier, _ = strconv.Atoi(value)
		if tier != 4 && tier != 6 && tier != 8 && tier != 10 {
			common.ApiError(w, 400, "invalid tier")
			return
		}
	}
	now := time.Now()
	rows, err := model.ReadAdminResourceStatus(r.Context(), now)
	if err != nil {
		common.ApiError(w, 500, "resource status unavailable")
		return
	}
	filtered := []model.AdminResourceStatus{}
	vendors, models := []string{}, []string{}
	search := strings.ToLower(strings.TrimSpace(query.Get("q")))
	for _, row := range rows {
		if row.Vendor != "" && !slices.Contains(vendors, row.Vendor) {
			vendors = append(vendors, row.Vendor)
		}
		if !slices.Contains(models, row.CanonicalModel) {
			models = append(models, row.CanonicalModel)
		}
		if search != "" && !strings.Contains(strings.ToLower(row.ID), search) && !strings.Contains(strings.ToLower(row.CanonicalModel), search) {
			continue
		}
		if query.Get("resource_id") != "" && row.ID != query.Get("resource_id") {
			continue
		}
		if query.Get("model") != "" && row.CanonicalModel != query.Get("model") {
			continue
		}
		if query.Get("vendor") != "" && row.Vendor != query.Get("vendor") {
			continue
		}
		if state := query.Get("status"); state != "" {
			matches := row.Status == state || row.State == state
			matches = matches || ((state == "unknown" || state == "abnormal") && len(row.Gateways) == 0)
			for _, gateway := range row.Gateways {
				matches = matches || gateway.State == state || (state == "abnormal" && gateway.State != "normal")
			}
			if !matches {
				continue
			}
		}
		if tier != 0 && !slices.Contains(row.Tiers, tier) {
			continue
		}
		filtered = append(filtered, row)
	}
	slices.Sort(vendors)
	slices.Sort(models)
	start := min((page-1)*size, len(filtered))
	end := min(start+size, len(filtered))
	common.ApiSuccess(w, map[string]any{"items": filtered[start:end], "total": len(filtered), "page": page, "page_size": size, "vendors": vendors, "models": models, "observed_at": now.Unix(), "window_seconds": 300})
}
