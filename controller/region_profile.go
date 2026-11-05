package controller

import (
	"net/http"
	"strings"

	"starhub/common"
	"starhub/model"
)

type saveRegionProfileRequest struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	DefaultLanguage    string `json:"default_language"`
	SupportedLanguages string `json:"supported_languages"`
	Currency           string `json:"currency"`
	Timezone           string `json:"timezone"`
	CountryCode        string `json:"country_code"`
	AllowedVendors     string `json:"allowed_vendors"`
	AllowedModels      string `json:"allowed_models"`
	AuthPlugins        string `json:"auth_plugins"`
	PaymentPlugins     string `json:"payment_plugins"`
	LegalVersion       string `json:"legal_version"`
	FeatureFlags       string `json:"feature_flags"`
	Enabled            *bool  `json:"enabled"`
	ExpectedRevision   *int64 `json:"expected_revision"`
}

// ListRegionProfilesHandler handles GET /api/region-profile.
// Admin-only: lists all region profiles.
func ListRegionProfilesHandler(w http.ResponseWriter, r *http.Request) {
	profiles, err := model.ListRegionProfiles()
	if err != nil {
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.ApiSuccess(w, profiles)
}

// GetRegionProfileHandler handles GET /api/region-profile/{id}.
func GetRegionProfileHandler(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		common.ApiError(w, http.StatusBadRequest, "region profile id is required")
		return
	}
	profile, err := model.GetRegionProfile(id)
	if err != nil {
		common.ApiError(w, http.StatusNotFound, err.Error())
		return
	}
	common.ApiSuccess(w, profile)
}

// SaveRegionProfileHandler handles PUT /api/region-profile.
// Admin-only: creates or updates a region profile with optimistic concurrency.
func SaveRegionProfileHandler(w http.ResponseWriter, r *http.Request) {
	var req saveRegionProfileRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	profile := &model.RegionProfile{
		ID:                 req.ID,
		Name:               req.Name,
		DefaultLanguage:    req.DefaultLanguage,
		SupportedLanguages: req.SupportedLanguages,
		Currency:           req.Currency,
		Timezone:           req.Timezone,
		CountryCode:        req.CountryCode,
		AllowedVendors:     req.AllowedVendors,
		AllowedModels:      req.AllowedModels,
		AuthPlugins:        req.AuthPlugins,
		PaymentPlugins:     req.PaymentPlugins,
		LegalVersion:       req.LegalVersion,
		FeatureFlags:       req.FeatureFlags,
		Enabled:            enabled,
	}
	var expectedRev int64 = -1
	if req.ExpectedRevision != nil {
		expectedRev = *req.ExpectedRevision
	}
	if err := model.SaveRegionProfile(profile, expectedRev); err != nil {
		if strings.Contains(err.Error(), "modified by another") {
			common.ApiError(w, http.StatusConflict, err.Error())
			return
		}
		common.ApiError(w, http.StatusBadRequest, err.Error())
		return
	}
	common.ApiSuccess(w, profile)
}

// DeleteRegionProfileHandler handles DELETE /api/region-profile/{id}.
// Admin-only: deletes a region profile (refuses last enabled).
func DeleteRegionProfileHandler(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		common.ApiError(w, http.StatusBadRequest, "region profile id is required")
		return
	}
	if err := model.DeleteRegionProfile(id); err != nil {
		common.ApiError(w, http.StatusConflict, err.Error())
		return
	}
	common.ApiSuccess(w, nil)
}
