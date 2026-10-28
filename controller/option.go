package controller

import (
	"errors"
	"net/http"
	"strconv"

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
	for i := range opts {
		if opts[i].Key == setting.OptionKeyAlertWebhookURL && opts[i].Value != `""` {
			opts[i].Value = "[redacted]"
		}
	}
	common.ApiSuccess(w, opts)
}

func OptionStatus(w http.ResponseWriter, r *http.Request) {
	version, err := model.OptionVersion()
	if err != nil {
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	loaded := model.LoadedOptionVersion()
	instances, err := model.ActiveOptionInstances()
	if err != nil {
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.ApiSuccess(w, map[string]any{"database_version": version,
		"loaded_version": loaded, "in_sync": version == loaded, "instances": instances})
}

// ReloadOptions applies the stored configuration to this process only.
// Other instance versions remain heartbeat observations, not acknowledgments.
func ReloadOptions(w http.ResponseWriter, r *http.Request) {
	if err := model.LoadOptionsToSettings(); err != nil {
		common.ApiError(w, http.StatusInternalServerError, "failed to reload stored configuration")
		return
	}
	OptionStatus(w, r)
}

func OptionHistory(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	changes, err := model.ListOptionChanges(limit)
	if err != nil {
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.ApiSuccess(w, changes)
}

type optionRollbackRequest struct {
	Version         int64 `json:"version"`
	ExpectedVersion int64 `json:"expected_version"`
}

func RollbackOption(w http.ResponseWriter, r *http.Request) {
	var req optionRollbackRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Version < 1 || req.ExpectedVersion < 1 {
		common.ApiError(w, http.StatusBadRequest, "version and expected_version are required")
		return
	}
	actorID, _ := r.Context().Value(common.CtxUserID).(int64)
	version, err := model.RollbackOption(req.Version, req.ExpectedVersion, actorID)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, model.ErrOptionVersionConflict) {
			status = http.StatusConflict
		}
		common.ApiError(w, status, err.Error())
		return
	}
	if err := model.LoadOptionsToSettings(); err != nil {
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.ApiSuccess(w, map[string]any{"version": version})
}

type optionUpdateRequest struct {
	Key             string `json:"key"`
	Value           string `json:"value"`
	ExpectedVersion *int64 `json:"expected_version"`
}

func UpdateOption(w http.ResponseWriter, r *http.Request) {
	var req optionUpdateRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if !setting.IsOptionKey(req.Key) {
		common.ApiError(w, http.StatusBadRequest, "unknown option key")
		return
	}
	value, err := setting.NormalizeOption(req.Key, req.Value)
	if err != nil {
		common.ApiError(w, http.StatusBadRequest, err.Error())
		return
	}
	actorID, _ := r.Context().Value(common.CtxUserID).(int64)
	if req.ExpectedVersion != nil {
		if _, err := model.SaveOptionIfVersion(req.Key, value, actorID, *req.ExpectedVersion); err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, model.ErrOptionVersionConflict) {
				status = http.StatusConflict
			}
			common.ApiError(w, status, err.Error())
			return
		}
	} else if _, err := model.SaveOption(req.Key, value, actorID); err != nil {
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := model.LoadOptionsToSettings(); err != nil {
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.ApiSuccess(w, nil)
}
