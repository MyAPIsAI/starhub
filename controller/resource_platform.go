package controller

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"starhub/common"
	"starhub/model"
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
