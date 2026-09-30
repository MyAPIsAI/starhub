package controller

import (
	"net/http"
	"strings"

	"starhub/common"
	"starhub/model"
)

// Model plaza metadata administration.
//
// These endpoints are the only write path in the plaza feature. They touch a
// standalone catalog table and are unreachable from the relay, so a bad row
// can at worst render a wrong label — it can never change a bill.

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
// The body is a single metadata row. Supplying it repeatedly is safe: the row
// is keyed by model name, so this is an idempotent catalog edit rather than a
// create/update pair the caller has to choose between.
func UpsertModelMeta(w http.ResponseWriter, r *http.Request) {
	var meta model.ModelMeta
	if !decodeJSONBody(w, r, &meta) {
		return
	}
	meta.Model = strings.TrimSpace(meta.Model)
	if meta.Model == "" {
		common.ApiError(w, http.StatusBadRequest, "model is required")
		return
	}
	if err := validateModelMeta(&meta); err != nil {
		common.ApiError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := model.UpsertModelMeta(&meta); err != nil {
		common.ApiError(w, http.StatusInternalServerError, err.Error())
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
	if err := model.DeleteModelMeta(name); err != nil {
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.ApiSuccess(w, map[string]any{"model": name})
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
