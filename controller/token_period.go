package controller

import (
	"errors"
	"net/http"
	"starhub/common"
	"starhub/model"
	"strconv"
	"time"
)

// TokenQuotaRegions exposes only selectable clock metadata, not deployment,
// authentication or payment settings from the administrative region profile.
func TokenQuotaRegions(w http.ResponseWriter, r *http.Request) {
	var regions []model.RegionProfile
	if err := model.DB.Select("id", "name", "timezone").Where("enabled = ?", true).Order("id").Find(&regions).Error; err != nil {
		common.ApiError(w, 500, "quota regions unavailable")
		return
	}
	options := []map[string]string{{"id": "", "name": "UTC", "timezone": "UTC"}}
	for _, region := range regions {
		options = append(options, map[string]string{"id": region.ID, "name": region.Name, "timezone": region.Timezone})
	}
	common.ApiSuccess(w, options)
}

func OwnedTokenUsage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	periodPage, modelPage := 1, 1
	for key, target := range map[string]*int{"period_page": &periodPage, "model_page": &modelPage} {
		if raw := r.URL.Query().Get(key); raw != "" {
			value, err := strconv.Atoi(raw)
			if err != nil || value < 1 || value > 1000000 {
				common.ApiError(w, 400, "invalid usage page")
				return
			}
			*target = value
		}
	}
	view, err := model.ReadOwnedTokenUsage(requestUserID(r), id, periodPage, modelPage, time.Now())
	if err != nil {
		if errors.Is(err, model.ErrTokenNotFound) {
			common.ApiError(w, 404, "token not found")
		} else {
			common.ApiError(w, 500, "key usage unavailable")
		}
		return
	}
	common.ApiSuccess(w, view)
}
