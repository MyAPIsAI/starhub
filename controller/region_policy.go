package controller

import (
	"net/http"
	"starhub/common"
	"starhub/model"
)

func requireRegionFeature(w http.ResponseWriter, feature, auth string) (*model.RegionProfile, bool) {
	profile, err := model.ActiveRegionProfile()
	if err != nil {
		common.ApiError(w, 503, "active deployment policy is unavailable")
		return nil, false
	}
	if !profile.FeatureEnabled(feature) || (auth != "" && !profile.AuthEnabled(auth)) {
		common.ApiError(w, 403, "this feature is disabled in the deployment region")
		return nil, false
	}
	return profile, true
}
