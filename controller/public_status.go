package controller

import (
	"errors"
	"net/http"
	"slices"
	"sort"
	"strings"
	"time"

	"starhub/common"
	"starhub/model"
)

type publicListedModel struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Vendor    string   `json:"vendor"`
	Tags      []string `json:"tags"`
	Protocols []string `json:"protocols"`
}
type publicHomeGroup struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Marketing and status pages share the published catalog, never preview seeds.
func publicListedModels(r *http.Request, now time.Time) ([]publicListedModel, error) {
	rows, err := publicCatalogRows(r.Context(), now)
	if err != nil {
		return nil, err
	}
	result := []publicListedModel{}
	for _, row := range rows {
		if row.Kind == "smart" {
			continue
		}
		result = append(result, publicListedModel{ID: row.ID, Name: row.Name, Vendor: row.VendorDisplay, Tags: row.Tags, Protocols: row.Protocols})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Vendor != result[j].Vendor {
			return result[i].Vendor < result[j].Vendor
		}
		return result[i].ID < result[j].ID
	})
	return result, nil
}
func GetPublicHome(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	rows, err := publicListedModels(r, time.Now())
	if err != nil {
		common.ApiError(w, 500, "published catalog is unavailable")
		return
	}
	catalog, err := model.ReadServiceGroupCatalog(r.Context())
	if err != nil {
		common.ApiError(w, 500, "published groups are unavailable")
		return
	}
	groups := []publicHomeGroup{}
	for _, g := range model.PublicCommonGroups(catalog) {
		groups = append(groups, publicHomeGroup{g.GroupID, firstNonEmpty(g.Name, g.GroupID), g.Description})
	}
	common.ApiSuccess(w, map[string]any{"models": rows, "groups": groups})
}
func GetPublicServiceStatus(w http.ResponseWriter, r *http.Request) {
	publicServiceStatusAt(time.Now())(w, r)
}
func publicServiceStatusAt(now time.Time) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		rows, err := publicListedModels(r, now)
		if err != nil {
			common.ApiError(w, 500, "public service status is unavailable")
			return
		}
		vendors := []string{}
		for _, row := range rows {
			vendors = append(vendors, row.Vendor)
		}
		vendors = sortedUnique(vendors)
		type facet struct {
			ID    string `json:"id"`
			Count int    `json:"count"`
		}
		types := []facet{}
		for _, tag := range []string{"text", "embedding", "rerank", "image", "audio", "video"} {
			count := 0
			for _, row := range rows {
				if slices.Contains(row.Tags, tag) {
					count++
				}
			}
			if count > 0 {
				types = append(types, facet{tag, count})
			}
		}
		catalogTotal := len(rows)
		q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
		vendor := r.URL.Query().Get("vendor")
		tag, state := r.URL.Query().Get("type"), r.URL.Query().Get("state")
		if state != "" && state != "abnormal" && state != "unknown" {
			common.ApiError(w, 400, "invalid state filter")
			return
		}
		rows = slices.DeleteFunc(rows, func(row publicListedModel) bool {
			return (tag != "" && !slices.Contains(row.Tags, tag)) || (vendor != "" && vendor != row.Vendor) || (q != "" && !strings.Contains(strings.ToLower(row.ID+" "+row.Name+" "+row.Vendor), q))
		})
		page, size := customerPage(r)
		catalog, err := model.ReadServiceGroupCatalog(r.Context())
		if err != nil {
			common.ApiError(w, 500, "status group configuration is unavailable")
			return
		}
		profile, err := model.CurrentDeploymentProfile(r.Context())
		if err != nil {
			common.ApiError(w, 500, "status timezone is unavailable")
			return
		}
		groups := model.StatusPageGroups(catalog)
		if state != "" {
			names := map[string]bool{}
			for _, row := range rows {
				names[row.ID] = slices.Contains(row.Tags, "text")
			}
			current, err := model.PublicStatusCurrent(r.Context(), names, groups, now)
			if err != nil {
				common.ApiError(w, 500, "public current status is unavailable")
				return
			}
			rows = slices.DeleteFunc(rows, func(row publicListedModel) bool {
				s := current[row.ID].State
				if state == "unknown" {
					return s != "insufficient_data"
				}
				return s != "degraded" && s != "outage"
			})
		}
		total := len(rows)
		start := min((page-1)*size, total)
		rows = rows[start:min(start+size, total)]
		names := map[string]bool{}
		for _, row := range rows {
			names[row.ID] = slices.Contains(row.Tags, "text")
		}
		stats, err := model.PublicStatusHistory(r.Context(), names, groups, profile.Profile.Timezone, now)
		if err != nil {
			common.ApiError(w, 500, "public service history is unavailable")
			return
		}
		type item struct {
			publicListedModel
			model.PublicModelStatus
		}
		items := []item{}
		for _, row := range rows {
			items = append(items, item{row, stats[row.ID]})
		}
		var target *int64
		for _, g := range groups {
			if g.SLAP95MS != nil && (target == nil || *g.SLAP95MS > *target) {
				value := *g.SLAP95MS
				target = &value
			}
		}
		common.ApiSuccess(w, map[string]any{"items": items, "total": total, "page": page, "page_size": size, "vendors": vendors, "types": types, "catalog_total": catalogTotal, "at": now.Unix(), "timezone": profile.Profile.Timezone, "refresh_seconds": 300, "selected_group_count": len(groups), "latency_target_ms": target})
	}
}

func GetPublicStatusHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	at := time.Now()
	rows, err := publicListedModels(r, at)
	if err != nil {
		common.ApiError(w, 500, "public catalog is unavailable")
		return
	}
	id := r.PathValue("id")
	if !slices.ContainsFunc(rows, func(row publicListedModel) bool { return row.ID == id }) {
		common.ApiError(w, 404, "model not found")
		return
	}
	catalog, err := model.ReadServiceGroupCatalog(r.Context())
	if err != nil {
		common.ApiError(w, 500, "status groups are unavailable")
		return
	}
	profile, err := model.CurrentDeploymentProfile(r.Context())
	if err != nil {
		common.ApiError(w, 500, "status timezone is unavailable")
		return
	}
	data, err := model.PublicStatusHours(r.Context(), id, profile.Profile.Timezone, r.URL.Query().Get("date"), model.StatusPageGroups(catalog), at)
	if errors.Is(err, model.ErrInvalidStatusDate) {
		common.ApiError(w, 400, err.Error())
		return
	}
	if err != nil {
		common.ApiError(w, 500, "hourly history is unavailable")
		return
	}
	common.ApiSuccess(w, map[string]any{"model": id, "timezone": profile.Profile.Timezone, "at": at.Unix(), "history": data})
}
