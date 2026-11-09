package controller

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"gorm.io/gorm"
	"starhub/common"
	"starhub/config"
	"starhub/model"
)

func adminFailure(w http.ResponseWriter, err error) {
	status := 500
	switch {
	case errors.Is(err, model.ErrAdminConflict), errors.Is(err, model.ErrOptionVersionConflict), errors.Is(err, model.ErrModelReferenced):
		status = 409
	case errors.Is(err, model.ErrAdminInvalid), errors.Is(err, model.ErrInvalidPricingOptions):
		status = 400
	case errors.Is(err, gorm.ErrRecordNotFound):
		status = 404
	}
	if status == 500 {
		common.ApiError(w, status, "administration operation could not be completed")
		return
	}
	common.ApiError(w, status, err.Error())
}
func GetStandardModels(w http.ResponseWriter, r *http.Request) {
	rows := []model.StandardModel{}
	if err := model.DB.WithContext(r.Context()).Order("sort_weight DESC, model").Find(&rows).Error; err != nil {
		adminFailure(w, err)
		return
	}
	metadata, err := model.ReadModelMetadataCatalog(r.Context())
	if err != nil {
		adminFailure(w, err)
		return
	}
	known := map[string]bool{}
	for _, row := range rows {
		known[row.Model] = true
	}
	names, namesErr := model.AllPlazaModels()
	if namesErr != nil {
		adminFailure(w, namesErr)
		return
	}
	metas := map[string]model.ModelMeta{}
	for _, meta := range metadata.Models {
		metas[meta.Model] = meta
	}
	for _, name := range names {
		if known[name] || name == "myapis-auto" || name == "veyra-auto" {
			continue
		}
		meta := metas[name]
		vendor, _ := model.InferVendor(name)
		if meta.Vendor != "" {
			vendor = meta.Vendor
		}
		rows = append(rows, model.StandardModel{Model: name, Name: name, Vendor: vendor, Kind: "chat", Protocols: []string{"OpenAI"}, Capabilities: meta.TagList(), ContextLength: meta.ContextLength, Description: meta.Description, Listed: true, SortWeight: meta.SortWeight, CreatedAt: meta.CreatedAt})
	}
	for i := range rows {
		if rows[i].Capabilities == nil {
			rows[i].Capabilities = []string{}
		}
	}
	inventory := map[string]model.ModelInventory{}
	for _, row := range rows {
		v, err := model.ModelInventoryFor(r.Context(), row.Model, row.Vendor)
		if err != nil {
			adminFailure(w, err)
			return
		}
		inventory[row.Model] = v
	}
	history := []model.StandardModelHistory{}
	q := model.DB.Order("id DESC").Limit(100)
	if name := r.URL.Query().Get("model"); name != "" {
		q = q.Where("model = ?", name)
	}
	if err := q.Find(&history).Error; err != nil {
		adminFailure(w, err)
		return
	}
	prices, err := model.ReadPricingOptions(r.Context())
	if err != nil {
		adminFailure(w, err)
		return
	}
	advanced := map[string]json.RawMessage{}
	var option model.Option
	if err := model.DB.First(&option, "`key` = ?", "model_time_token_prices").Error; err == nil {
		_ = json.Unmarshal([]byte(option.Value), &advanced)
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		adminFailure(w, err)
		return
	}
	common.ApiSuccess(w, map[string]any{"models": rows, "metadata": metadata.Models, "catalog_revision": metadata.Revision, "inventory": inventory, "history": history, "pricing": prices, "loaded_version": model.LoadedOptionVersion(), "advanced_pricing": advanced})
}
func PutStandardModel(w http.ResponseWriter, r *http.Request) {
	var input model.StandardModelWrite
	if !decodeResourceBody(w, r, &input) {
		return
	}
	if err := validateModelMeta(&input.Metadata); err != nil {
		common.ApiError(w, 400, err.Error())
		return
	}
	row, err := model.SaveStandardModel(r.Context(), input, requestUserID(r), common.ClientIP(r))
	if err != nil {
		adminFailure(w, err)
		return
	}
	if err := model.LoadOptionsToSettings(); err != nil {
		common.ApiError(w, 500, "model saved but pricing reload failed; refresh to verify committed state")
		return
	}
	common.ApiSuccess(w, row)
}
func RemoveStandardModel(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ExpectedCatalogRevision int64 `json:"expected_catalog_revision"`
	}
	if !decodeResourceBody(w, r, &input) {
		return
	}
	if err := model.DeleteStandardModel(r.Context(), r.PathValue("model"), input.ExpectedCatalogRevision, requestUserID(r), common.ClientIP(r)); err != nil {
		adminFailure(w, err)
		return
	}
	common.ApiSuccess(w, nil)
}
func GetDeploymentProfile(w http.ResponseWriter, r *http.Request) {
	current, err := model.CurrentDeploymentProfile(r.Context())
	if err != nil {
		adminFailure(w, err)
		return
	}
	history := []model.DeploymentProfileVersion{}
	if err := model.DB.Order("revision DESC").Find(&history).Error; err != nil {
		adminFailure(w, err)
		return
	}
	common.ApiSuccess(w, map[string]any{"current": current, "history": history})
}
func PutDeploymentProfile(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Profile          model.DeploymentProfile `json:"profile"`
		ExpectedRevision int64                   `json:"expected_revision"`
	}
	if !decodeResourceBody(w, r, &input) {
		return
	}
	row, err := model.SaveDeploymentProfile(r.Context(), input.Profile, input.ExpectedRevision, requestUserID(r), common.ClientIP(r))
	if err != nil {
		adminFailure(w, err)
		return
	}
	_ = config.SetRegionTimezone(row.Profile.Timezone)
	common.ApiSuccess(w, row)
}
func GetLegalVersions(w http.ResponseWriter, r *http.Request) {
	page, size := customerPage(r)
	now := time.Now().Unix()
	query := model.DB.WithContext(r.Context()).Model(&model.LegalDocumentVersion{})
	for _, field := range []string{"kind", "language"} {
		if value := r.URL.Query().Get(field); value != "" {
			query = query.Where(field+" = ?", value)
		}
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		adminFailure(w, err)
		return
	}
	rows := []struct {
		model.LegalDocumentVersion
		Status string `json:"status"`
	}{}
	// Determine current status across the entire history, before pagination.
	err := query.Select(`legal_document_versions.*, CASE WHEN effective_at > ? THEN 'scheduled'
 WHEN EXISTS (SELECT 1 FROM legal_document_versions newer WHERE newer.kind = legal_document_versions.kind
 AND newer.language = legal_document_versions.language AND newer.effective_at <= ?
 AND (newer.effective_at > legal_document_versions.effective_at OR
 (newer.effective_at = legal_document_versions.effective_at AND newer.id > legal_document_versions.id)))
 THEN 'historical' ELSE 'current' END AS status`, now, now).
		Order("kind, language, effective_at DESC, id DESC").Offset((page - 1) * size).Limit(size).Scan(&rows).Error
	if err != nil {
		adminFailure(w, err)
		return
	}
	profile, err := model.CurrentDeploymentProfile(r.Context())
	if err != nil {
		adminFailure(w, err)
		return
	}
	common.ApiSuccess(w, map[string]any{"items": rows, "timezone": profile.Profile.Timezone, "now": now, "page": page, "page_size": size, "total": total})
}

// Only public legal and consent settings are exposed; deployment secrets stay private.
func GetPublicLegalPolicy(w http.ResponseWriter, r *http.Request) {
	profile, err := model.CurrentDeploymentProfile(r.Context())
	if err != nil {
		adminFailure(w, err)
		return
	}
	mode := profile.Profile.CookieConsentMode
	if mode == "" {
		mode = "banner"
	}
	w.Header().Set("Cache-Control", "no-store")
	common.ApiSuccess(w, map[string]any{
		"region_id": profile.Profile.RegionID, "timezone": profile.Profile.Timezone,
		"cookie_consent": profile.Profile.CookieConsent, "cookie_consent_mode": mode,
		"consent_version": "necessary-v1",
	})
}

func PostLegalVersion(w http.ResponseWriter, r *http.Request) {
	var input struct {
		model.LegalDocumentVersion
		EffectiveLocal string `json:"effective_local"`
	}
	if !decodeJSONBody(w, r, &input) {
		return
	}
	row, err := model.SaveLegalDocument(r.Context(), input.LegalDocumentVersion, input.EffectiveLocal, requestUserID(r), common.ClientIP(r))
	if err != nil {
		adminFailure(w, err)
		return
	}
	common.ApiSuccess(w, row)
}
func GetPublishedLegal(w http.ResponseWriter, r *http.Request) { PublishedLegalAt(time.Now())(w, r) }

// PublishedLegalAt allows deterministic boundary verification without changing the server clock.
func PublishedLegalAt(now time.Time) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		lang := r.URL.Query().Get("language")
		if lang == "" {
			lang = "en"
		}
		row, err := model.ActiveLegalDocument(r.Context(), r.PathValue("kind"), lang, now)
		if err != nil {
			adminFailure(w, err)
			return
		}
		var next struct{ At int64 }
		if err := model.DB.WithContext(r.Context()).Model(&model.LegalDocumentVersion{}).
			Select("COALESCE(MIN(effective_at), 0) AS at").Where("kind = ? AND language = ? AND effective_at > ?", row.Kind, row.Language, now.Unix()).Scan(&next).Error; err != nil {
			adminFailure(w, err)
			return
		}
		common.ApiSuccess(w, map[string]any{"kind": row.Kind, "language": row.Language, "version": row.Version,
			"content": row.Content, "effective_at": row.EffectiveAt, "timezone": row.Timezone,
			"now": now.Unix(), "next_effective_at": next.At})
	}
}
func ListPricingHistory(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > 200 {
		limit = 100
	}
	rows := []model.AuditLog{}
	query := model.DB.WithContext(r.Context()).Where("action IN ?", []string{"model_pricing_changed", "service_group_multiplier_changed"})
	if name := r.URL.Query().Get("object"); name != "" {
		query = query.Where("object_id = ?", name)
	}
	if kind := r.URL.Query().Get("kind"); kind != "" {
		query = query.Where("object_type = ?", kind)
	}
	if err := query.Order("id DESC").Limit(limit).Find(&rows).Error; err != nil {
		adminFailure(w, err)
		return
	}
	common.ApiSuccess(w, rows)
}
