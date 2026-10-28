package controller

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"starhub/common"
	"starhub/model"
)

func ListResources(w http.ResponseWriter, r *http.Request) {
	catalog, err := model.ReadResourceCatalog(r.Context())
	writeResourceResult(w, catalog, err)
}

func writeResourceResult(w http.ResponseWriter, catalog *model.ResourceCatalog, err error) {
	if err == nil {
		common.ApiSuccess(w, catalog)
		return
	}
	switch {
	case errors.Is(err, model.ErrResourceConflict):
		common.ApiError(w, http.StatusConflict, "resource catalog changed; reload before saving")
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
