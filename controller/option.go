package controller

import (
	"errors"
	"net/http"
	"strconv"

	"starhub/common"
	"starhub/model"
	"starhub/service"
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
	if req.Key == setting.OptionKeyModelTimeTokenPrices && (req.ExpectedVersion == nil || *req.ExpectedVersion < 0 || *req.ExpectedVersion >= 1<<53-1) {
		common.ApiError(w, http.StatusBadRequest, "a supported nonnegative expected_version is required for independent token prices")
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
			} else if errors.Is(err, setting.ErrTokenPriceMode) {
				status = http.StatusBadRequest
			}
			common.ApiError(w, status, err.Error())
			return
		}
	} else if _, err := model.SaveOption(req.Key, value, actorID); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, setting.ErrTokenPriceMode) {
			status = http.StatusBadRequest
		}
		common.ApiError(w, status, err.Error())
		return
	}
	if err := model.LoadOptionsToSettings(); err != nil {
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.ApiSuccess(w, nil)
}

func UpdateGlobalAffinityOptions(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ExpectedVersion *int64         `json:"expected_version"`
		Options         []model.Option `json:"options"`
	}
	if !decodeResourceBody(w, r, &req) {
		return
	}
	if req.ExpectedVersion == nil {
		common.ApiError(w, 400, "expected_version is required")
		return
	}
	actor, _ := r.Context().Value(common.CtxUserID).(int64)
	version, err := model.SaveAffinityOptionsIfVersion(r.Context(), req.Options, actor, *req.ExpectedVersion)
	if err != nil {
		switch {
		case errors.Is(err, model.ErrInvalidAffinityBatch):
			common.ApiError(w, 400, err.Error())
		case errors.Is(err, model.ErrOptionVersionConflict):
			common.ApiError(w, 409, "configuration changed; reload and review again")
		default:
			common.ApiError(w, 500, "global affinity settings could not be saved")
		}
		return
	}
	if err := model.LoadOptionsToSettings(); err != nil {
		common.ApiError(w, 500, "settings saved but could not be reloaded")
		return
	}
	common.ApiSuccess(w, map[string]int64{"version": version})
}

// GetRoutingAffinityCacheStats reports the live rule-affinity cache: total
// entries, per-rule counts, the rule count and whether the feature is on.
// Admin-only; the cache is process-local so the numbers reflect this replica.
func GetRoutingAffinityCacheStats(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if q.Has("rule") {
		page := 1
		if q.Has("page") {
			var err error
			page, err = strconv.Atoi(q.Get("page"))
			if err != nil || page < 1 {
				common.ApiError(w, http.StatusBadRequest, "invalid detail page")
				return
			}
		}
		common.ApiSuccess(w, service.RuleAffinityDetails(q.Get("rule"), page))
		return
	}
	common.ApiSuccess(w, service.RuleAffinityStats())
}

// ClearRoutingAffinityCache evicts every rule-affinity cache entry. Returns
// the number of entries removed. Admin-only.
func ClearRoutingAffinityCache(w http.ResponseWriter, r *http.Request) {
	removed := service.ClearAllRuleAffinity()
	common.ApiSuccess(w, map[string]any{"removed": removed})
}
