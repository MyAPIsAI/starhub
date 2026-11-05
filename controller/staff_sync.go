package controller

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"runtime"
	"time"

	"starhub/common"
	"starhub/config"
	"starhub/model"
)

func StaffResourceSyncStatus(w http.ResponseWriter, r *http.Request) {
	catalog, err := model.ReadResourceCatalog(r.Context())
	if err != nil {
		common.ApiError(w, 500, "resource state unavailable")
		return
	}
	sequence := int64(0)
	if catalog.Snapshot != nil {
		sequence = catalog.Snapshot.Sequence
	}
	common.ApiSuccess(w, map[string]any{"configured": config.ResourceSnapshotSource().URL != "", "revision": catalog.Revision, "sequence": sequence, "updated_at": catalog.UpdatedAt})
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

// A forced synchronization fetches a full authoritative snapshot, never resets
// sequence validation, and never changes the published catalog on a failure.
func StaffForceResourceSync(w http.ResponseWriter, r *http.Request) {
	source := config.ResourceSnapshotSource()
	if source.URL == "" {
		common.ApiError(w, 503, "resource snapshot source is not configured")
		return
	}
	catalog, err := model.ReadResourceCatalog(r.Context())
	if err != nil {
		common.ApiError(w, 500, "resource state unavailable")
		return
	}
	address, err := url.Parse(source.URL)
	if err != nil {
		common.ApiError(w, 503, "resource snapshot source is unavailable")
		return
	}
	query := address.Query()
	query.Set("full", "true")
	address.RawQuery = query.Encode()
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address.String(), nil)
	if err != nil {
		common.ApiError(w, 503, "resource synchronization unavailable")
		return
	}
	request.Header.Set("Accept", "application/json")
	if source.Token != "" {
		request.Header.Set("Authorization", "Bearer "+source.Token)
	}
	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(request)
	if err != nil {
		common.ApiError(w, 502, "resource snapshot fetch failed")
		return
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		common.ApiError(w, 502, "resource snapshot source rejected the request")
		return
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(raw) > 1<<20 {
		common.ApiError(w, 400, "resource snapshot exceeds the import limit")
		return
	}
	var snapshot model.ResourceSnapshot
	if json.Unmarshal(raw, &snapshot) != nil {
		common.ApiError(w, 400, "invalid resource snapshot JSON")
		return
	}
	updated, err := model.SaveResourceSnapshot(ctx, catalog.Revision, snapshot)
	if err != nil {
		writeResourceResult(w, nil, err)
		return
	}
	common.ApiSuccess(w, map[string]any{"full": true, "revision": updated.Revision, "sequence": snapshot.Sequence, "instances": len(snapshot.Instances)})
}
