package controller

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"starhub/common"
	"starhub/model"
)

func ListResources(w http.ResponseWriter, r *http.Request) {
	catalog, err := model.ReadResourceCatalog(r.Context())
	writeResourceResult(w, catalog, err)
}

func GetResourceBindingState(w http.ResponseWriter, r *http.Request) {
	page, pageSize := 1, 20
	values := r.URL.Query()
	for key, entries := range values {
		if (key != "page" && key != "page_size") || len(entries) != 1 {
			common.ApiError(w, http.StatusBadRequest, "invalid binding pagination")
			return
		}
		value, err := strconv.Atoi(entries[0])
		if err != nil || strconv.Itoa(value) != entries[0] {
			common.ApiError(w, http.StatusBadRequest, "invalid binding pagination")
			return
		}
		if key == "page" {
			page = value
		} else {
			pageSize = value
		}
	}
	state, err := model.ReadResourceBindingState(r.Context(), page, pageSize)
	if err != nil {
		writeResourceResult(w, nil, err)
		return
	}
	common.ApiSuccess(w, state)
}

func ChangeResourceBinding(w http.ResponseWriter, r *http.Request) {
	var request struct {
		ResourceRevision *int64 `json:"expected_resource_revision"`
		ChannelRevision  *int64 `json:"expected_channel_revision"`
		SourceChannelID  string `json:"expected_source_channel_id"`
		PublicationID    string `json:"expected_publication_id"`
		Operation        string `json:"operation"`
		RequestID        string `json:"request_id"`
	}
	if !decodeResourceBody(w, r, &request) {
		return
	}
	if request.ResourceRevision == nil || request.ChannelRevision == nil {
		common.ApiError(w, http.StatusBadRequest, "resource and channel versions are required")
		return
	}
	actorID, _ := r.Context().Value(common.CtxUserID).(int64)
	receipt, err := model.ChangeResourceBinding(r.Context(), r.PathValue("id"), model.ResourceBindingChangeRequest{ResourceRevision: *request.ResourceRevision, ChannelRevision: *request.ChannelRevision, SourceChannelID: request.SourceChannelID, PublicationID: request.PublicationID, Operation: request.Operation, RequestID: request.RequestID}, actorID)
	if err != nil {
		writeResourceResult(w, nil, err)
		return
	}
	common.ApiSuccess(w, receipt)
}

func GetResourceBindingChange(w http.ResponseWriter, r *http.Request) {
	receipt, err := model.ReadResourceBindingChange(r.Context(), r.PathValue("request_id"))
	if errors.Is(err, model.ErrResourceBindingChangeNotFound) {
		common.ApiError(w, http.StatusNotFound, err.Error())
		return
	}
	if err != nil {
		writeResourceResult(w, nil, err)
		return
	}
	common.ApiSuccess(w, receipt)
}

func GetResourcePublicationContext(w http.ResponseWriter, r *http.Request) {
	context, err := model.ReadResourcePublicationContext(r.Context())
	if err != nil {
		writeResourceResult(w, nil, err)
		return
	}
	common.ApiSuccess(w, context)
}

func PreviewResourcePublication(w http.ResponseWriter, r *http.Request) {
	var request struct {
		ResourceRevision *int64                         `json:"expected_resource_revision"`
		ChannelRevision  *int64                         `json:"expected_channel_revision"`
		OptionVersion    *int64                         `json:"expected_option_version"`
		Bindings         []model.ResourceChannelBinding `json:"bindings"`
	}
	if !decodeResourceBody(w, r, &request) {
		return
	}
	if request.ResourceRevision == nil || request.ChannelRevision == nil || request.OptionVersion == nil {
		common.ApiError(w, http.StatusBadRequest, "resource, channel and option versions are required")
		return
	}
	preview, err := model.PreviewResourcePublication(r.Context(), model.ResourcePublicationPrecondition{ResourceRevision: *request.ResourceRevision, ChannelRevision: *request.ChannelRevision, OptionVersion: *request.OptionVersion}, request.Bindings)
	if err != nil {
		writeResourceResult(w, nil, err)
		return
	}
	common.ApiSuccess(w, preview)
}

func PublishResourceGateway(w http.ResponseWriter, r *http.Request) {
	var request struct {
		ResourceRevision *int64                         `json:"expected_resource_revision"`
		ChannelRevision  *int64                         `json:"expected_channel_revision"`
		OptionVersion    *int64                         `json:"expected_option_version"`
		Bindings         []model.ResourceChannelBinding `json:"bindings"`
		ReviewDigest     string                         `json:"review_digest"`
	}
	if !decodeResourceBody(w, r, &request) {
		return
	}
	if request.ResourceRevision == nil || request.ChannelRevision == nil || request.OptionVersion == nil {
		common.ApiError(w, http.StatusBadRequest, "resource, channel and option versions are required")
		return
	}
	actorID, _ := r.Context().Value(common.CtxUserID).(int64)
	ack, err := model.PublishResourceGateway(r.Context(), model.ResourcePublicationPrecondition{ResourceRevision: *request.ResourceRevision, ChannelRevision: *request.ChannelRevision, OptionVersion: *request.OptionVersion}, request.Bindings, request.ReviewDigest, actorID)
	if err != nil {
		writeResourceResult(w, nil, err)
		return
	}
	common.ApiSuccess(w, ack)
}

func GetResourcePublicationReceipt(w http.ResponseWriter, r *http.Request) {
	receipt, err := model.ReadResourcePublicationReceipt(r.Context(), r.PathValue("digest"))
	if errors.Is(err, model.ErrResourcePublicationNotFound) {
		common.ApiError(w, http.StatusNotFound, err.Error())
		return
	}
	if err != nil {
		writeResourceResult(w, nil, err)
		return
	}
	common.ApiSuccess(w, receipt)
}

func writeResourceResult(w http.ResponseWriter, catalog *model.ResourceCatalog, err error) {
	if err == nil {
		common.ApiSuccess(w, catalog)
		return
	}
	switch {
	case errors.Is(err, model.ErrResourceConflict):
		common.ApiError(w, http.StatusConflict, "resource publication or catalog context changed; reload before continuing")
	case errors.Is(err, model.ErrResourceRegionInUse):
		common.ApiError(w, http.StatusConflict, err.Error())
	case errors.Is(err, model.ErrInvalidResource):
		common.ApiError(w, http.StatusBadRequest, err.Error())
	default:
		common.ApiError(w, http.StatusInternalServerError, "resource catalog is unavailable")
	}
}

// Resource imports are strict so credential fields or a second JSON document cannot be persisted accidentally.
func decodeResourceBody(w http.ResponseWriter, r *http.Request, value any) bool {
	defer r.Body.Close()
	reader := http.MaxBytesReader(w, r.Body, (1<<20)+4096)
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	err := decoder.Decode(value)
	if err == nil {
		var extra any
		err = decoder.Decode(&extra)
		if err == io.EOF {
			return true
		}
	}
	var oversized *http.MaxBytesError
	if errors.As(err, &oversized) {
		common.ApiError(w, http.StatusRequestEntityTooLarge, "resource request is too large")
	} else {
		common.ApiError(w, http.StatusBadRequest, "invalid resource request body")
	}
	return false
}

func UpdateResources(w http.ResponseWriter, r *http.Request) {
	var request struct {
		ExpectedRevision *int64                  `json:"expected_revision"`
		Snapshot         *model.ResourceSnapshot `json:"snapshot"`
	}
	if !decodeResourceBody(w, r, &request) {
		return
	}
	if request.ExpectedRevision == nil || request.Snapshot == nil {
		common.ApiError(w, http.StatusBadRequest, "expected_revision and snapshot are required")
		return
	}
	catalog, err := model.SaveResourceSnapshot(r.Context(), *request.ExpectedRevision, *request.Snapshot)
	writeResourceResult(w, catalog, err)
}

func UpdatePlacement(w http.ResponseWriter, r *http.Request) {
	var request struct {
		ExpectedRevision *int64                   `json:"expected_revision"`
		Placement        *model.ResourcePlacement `json:"placement"`
	}
	if !decodeResourceBody(w, r, &request) {
		return
	}
	if request.ExpectedRevision == nil || request.Placement == nil {
		common.ApiError(w, http.StatusBadRequest, "expected_revision and placement are required")
		return
	}
	catalog, err := model.SaveResourcePlacement(r.Context(), *request.ExpectedRevision, r.PathValue("id"), *request.Placement)
	writeResourceResult(w, catalog, err)
}

func UpdateResourceRegion(w http.ResponseWriter, r *http.Request) {
	var request struct {
		ExpectedRevision *int64 `json:"expected_revision"`
		Name             string `json:"name"`
		Description      string `json:"description"`
	}
	if !decodeResourceBody(w, r, &request) {
		return
	}
	if request.ExpectedRevision == nil {
		common.ApiError(w, http.StatusBadRequest, "expected_revision is required")
		return
	}
	catalog, err := model.SaveResourceRegion(r.Context(), *request.ExpectedRevision, model.ResourceRegion{ID: r.PathValue("id"), Name: request.Name, Description: request.Description})
	writeResourceResult(w, catalog, err)
}

func DeleteResourceRegion(w http.ResponseWriter, r *http.Request) {
	var request struct {
		ExpectedRevision *int64 `json:"expected_revision"`
	}
	if !decodeResourceBody(w, r, &request) {
		return
	}
	if request.ExpectedRevision == nil {
		common.ApiError(w, http.StatusBadRequest, "expected_revision is required")
		return
	}
	catalog, err := model.DeleteResourceRegion(r.Context(), *request.ExpectedRevision, r.PathValue("id"))
	writeResourceResult(w, catalog, err)
}
