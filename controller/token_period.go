package controller

import (
	"net/http"
	"starhub/common"
	"starhub/model"
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
