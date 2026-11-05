package controller

import (
	"net/http"
	"strings"

	"starhub/common"
	"starhub/middleware"
	"starhub/model"
)

type publishLegalRequest struct {
	Type     string `json:"type"`
	Version  string `json:"version"`
	Language string `json:"language"`
	Content  string `json:"content"`
	RegionID string `json:"region_id"`
}

// GetLegalDocumentHandler handles GET /api/legal/{type}.
// Returns the latest legal document in the requested language and region.
func GetLegalDocumentHandler(w http.ResponseWriter, r *http.Request) {
	docType := r.PathValue("type")
	if docType == "" {
		common.ApiError(w, http.StatusBadRequest, "legal document type is required")
		return
	}
	lang := r.URL.Query().Get("lang")
	if lang == "" {
		lang = middleware.ResolveLanguage("", r.Header.Get("Accept-Language"))
	}
	regionID := r.URL.Query().Get("region")
	doc, err := model.GetLegalDocument(docType, lang, regionID)
	if err != nil {
		common.ApiError(w, http.StatusNotFound, err.Error())
		return
	}
	common.ApiSuccess(w, map[string]any{
		"type":         doc.Type,
		"version":      doc.Version,
		"language":     doc.Language,
		"content":      doc.Content,
		"region_id":    doc.RegionID,
		"published_at": doc.PublishedAt,
	})
}

// PublishLegalDocumentHandler handles PUT /api/legal.
// Admin-only: creates or updates a legal document version.
func PublishLegalDocumentHandler(w http.ResponseWriter, r *http.Request) {
	var req publishLegalRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	req.Type = strings.TrimSpace(req.Type)
	if req.Type != "user_agreement" && req.Type != "privacy_policy" {
		common.ApiError(w, http.StatusBadRequest, "type must be user_agreement or privacy_policy")
		return
	}
	doc := &model.LegalDocument{
		Type:     req.Type,
		Version:  req.Version,
		Language: req.Language,
		Content:  req.Content,
		RegionID: req.RegionID,
	}
	if err := model.PublishLegalDocument(doc); err != nil {
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.ApiSuccess(w, map[string]any{
		"type":      doc.Type,
		"version":   doc.Version,
		"language":  doc.Language,
		"published": true,
	})
}

// ListLegalDocumentsHandler handles GET /api/legal.
// Admin-only: lists all legal documents.
func ListLegalDocumentsHandler(w http.ResponseWriter, r *http.Request) {
	docType := r.URL.Query().Get("type")
	docs, err := model.ListLegalDocuments(docType)
	if err != nil {
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.ApiSuccess(w, docs)
}
