package controller

import (
	"net/http"

	"starhub/common"
	"starhub/model"
	"starhub/setting"
)

func ListOptions(w http.ResponseWriter, r *http.Request) {
	opts, err := model.AllOptions()
	if err != nil {
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.ApiSuccess(w, opts)
}

type optionUpdateRequest struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func UpdateOption(w http.ResponseWriter, r *http.Request) {
	var req optionUpdateRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if !setting.IsRatioOptionKey(req.Key) {
		common.ApiError(w, http.StatusBadRequest, "unknown option key")
		return
	}
	if err := setting.UpdateRatio(req.Key, req.Value); err != nil {
		common.ApiError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := model.UpsertOption(req.Key, req.Value); err != nil {
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.ApiSuccess(w, nil)
}
