package controller

import (
	"net/http"

	"starhub/common"
	"starhub/model"
	"starhub/relay/channel"
	"starhub/service"
)

func GetChannelImageTest(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	ch, err := model.GetChannelByID(id)
	if err != nil || ch.Type != channel.ChannelTypeArk {
		common.ApiError(w, 404, "image test channel not found")
		return
	}
	entry, err := model.LatestChannelImageProbe(ch)
	if err != nil {
		common.ApiError(w, 500, "image test result unavailable")
		return
	}
	if entry == nil {
		common.ApiSuccess(w, nil)
		return
	}
	result := channelTestResult{Scope: "image", Model: entry.Model, Status: ch.Status, ElapsedMs: entry.ElapsedMs, OK: entry.State == "succeeded"}
	switch entry.State {
	case "succeeded":
		result.ImageURL = service.ImageProbeURL(entry.PreviewURL)
		result.Message = "Image generated successfully"
	case "failed":
		result.Code, result.Message = "image_probe_upstream_failed", "Image generation failed"
	default:
		result.Code, result.Message = "image_probe_result_uncertain", "Image generation outcome is not confirmed"
	}
	common.ApiSuccess(w, result)
}
