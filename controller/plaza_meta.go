package controller

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"starhub/common"
	"starhub/model"
)

// Model plaza metadata administration.
//
// Metadata has its own revision domain. It does not change billing options,
// but published capability/context metadata affects smart-routing candidates.

// ListModelMeta handles GET /api/plaza/meta.
func ListModelMeta(w http.ResponseWriter, r *http.Request) {
	rows, err := model.ListModelMeta()
	if err != nil {
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// A map is the natural shape for the plaza's join, but a list is the
	// natural shape for an admin table; return the list and keep the map
	// internal.
	list := make([]*model.ModelMeta, 0, len(rows))
	for _, m := range rows {
		list = append(list, m)
	}
	common.ApiSuccess(w, list)
}

// UpsertModelMeta handles PUT /api/plaza/meta.
//
// Legacy flat rows remain supported. Guarded clients additionally send
// expected_catalog_revision; unguarded writes still advance that revision.
func UpsertModelMeta(w http.ResponseWriter, r *http.Request) {
	var req struct {
		model.ModelMeta
		ExpectedCatalogRevision json.RawMessage `json:"expected_catalog_revision"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	meta := req.ModelMeta
	meta.Model = strings.TrimSpace(meta.Model)
	if meta.Model == "" {
		common.ApiError(w, http.StatusBadRequest, "model is required")
		return
	}
	if err := validateModelMeta(&meta); err != nil {
		common.ApiError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(req.ExpectedCatalogRevision) != 0 {
		var expected *int64
		if json.Unmarshal(req.ExpectedCatalogRevision, &expected) != nil || expected == nil {
			common.ApiError(w, http.StatusBadRequest, "expected_catalog_revision must be an explicit integer")
			return
		}
		revision, err := model.UpsertModelMetaIfRevision(r.Context(), &meta, *expected)
		if err != nil {
			modelMetadataError(w, err)
			return
		}
		common.ApiSuccess(w, map[string]any{"meta": meta, "revision": revision})
		return
	}
	if err := model.UpsertModelMeta(&meta); err != nil {
		modelMetadataError(w, err)
		return
	}
	common.ApiSuccess(w, meta)
}

// DeleteModelMeta handles DELETE /api/plaza/meta/{model}.
func DeleteModelMeta(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("model"))
	if name == "" {
		common.ApiError(w, http.StatusBadRequest, "model is required")
		return
	}
	if r.Body != nil && r.Body != http.NoBody && r.ContentLength != 0 {
		var req struct {
			ExpectedCatalogRevision *int64 `json:"expected_catalog_revision"`
		}
		if !decodeResourceBody(w, r, &req) {
			return
		}
		if req.ExpectedCatalogRevision == nil {
			common.ApiError(w, http.StatusBadRequest, "expected_catalog_revision is required in a delete body")
			return
		}
		revision, err := model.DeleteModelMetaIfRevision(r.Context(), name, *req.ExpectedCatalogRevision)
		if err != nil {
			modelMetadataError(w, err)
			return
		}
		common.ApiSuccess(w, map[string]any{"model": name, "revision": revision})
		return
	}
	if err := model.DeleteModelMeta(name); err != nil {
		modelMetadataError(w, err)
		return
	}
	common.ApiSuccess(w, map[string]any{"model": name})
}

func GetModelMetadataCatalog(w http.ResponseWriter, r *http.Request) {
	catalog, err := model.ReadModelMetadataCatalog(r.Context())
	if err != nil {
		common.ApiError(w, http.StatusInternalServerError, "model metadata catalog is unavailable")
		return
	}
	common.ApiSuccess(w, map[string]any{"models": catalog.Models, "revision": catalog.Revision, "revision_guards": true})
}

func modelMetadataError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, model.ErrModelMetadataConflict):
		common.ApiError(w, http.StatusConflict, "model metadata catalog changed; reload and review again")
	case errors.Is(err, model.ErrInvalidModelMetadataRevision):
		common.ApiError(w, http.StatusBadRequest, err.Error())
	default:
		common.ApiError(w, http.StatusInternalServerError, "model metadata could not be changed")
	}
}

// validateModelMeta rejects obviously wrong catalog rows at the boundary, so
// the plaza never has to defend against negative prices or a discount that
// would render as a negative savings figure.
func validateModelMeta(m *model.ModelMeta) error {
	if m.ContextLength < 0 || m.MaxOutput < 0 {
		return errPlaza("context_length and max_output must not be negative")
	}
	neg := func(p *float64, name string) error {
		if p != nil && *p < 0 {
			return errPlaza(name + " must not be negative")
		}
		return nil
	}
	for _, c := range []struct {
		p    *float64
		name string
	}{
		{m.OfficialInputPerM, "official_input_per_m"},
		{m.OfficialOutputPerM, "official_output_per_m"},
		{m.OfficialCacheReadPerM, "official_cache_read_per_m"},
		{m.OfficialCacheWritePerM, "official_cache_write_per_m"},
		{m.OfficialPerRequest, "official_per_request"},
		{m.PerRequestUSD, "per_request_usd"},
	} {
		if err := neg(c.p, c.name); err != nil {
			return err
		}
	}
	if m.Score != nil && (*m.Score < 0 || *m.Score > 100) {
		return errPlaza("score must be between 0 and 100")
	}
	return nil
}

type errPlaza string

func (e errPlaza) Error() string { return string(e) }
