package controller

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"

	"errors"
	"gorm.io/gorm"
	"starhub/common"
	"starhub/model"
	"starhub/relay/dto"
	"starhub/service"
)

var videoProbeRequestID = regexp.MustCompile(`^[a-f0-9-]{36}$`)

func SubmitChannelVideoTest(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var input struct {
		Model     string `json:"model"`
		Prompt    string `json:"prompt"`
		RequestID string `json:"request_id"`
		Confirmed bool   `json:"confirm_upstream_charge"`
	}
	if !decodeJSONBody(w, r, &input) {
		return
	}
	if !input.Confirmed || !videoProbeRequestID.MatchString(input.RequestID) || len(input.Prompt) > 1000 || strings.TrimSpace(input.Prompt) == "" {
		common.ApiError(w, 400, "video_probe_confirmation_required")
		return
	}
	ch, err := model.GetChannelByID(id)
	if err != nil {
		common.ApiError(w, 404, "channel not found")
		return
	}
	if !modelInList(input.Model, model.SplitList(ch.Models)) {
		common.ApiError(w, 400, "model does not belong to this channel")
		return
	}
	task, err := service.SubmitChannelVideoProbe(r.Context(), ch, requestUserID(r), input.RequestID, input.Model, input.Prompt)
	if err != nil {
		common.ApiError(w, 400, videoProbeSafeError(err))
		return
	}
	writeVideoProbe(w, task)
}

func GetChannelVideoTest(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var task model.Task
	err := model.DB.Where("channel_id = ? AND platform = ?", id, model.ArkVideoProbePlatform).Order("id DESC").First(&task).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		common.ApiSuccess(w, nil)
		return
	}
	if err != nil {
		common.ApiError(w, 500, "video_probe_unavailable")
		return
	}
	updated, err := service.PollChannelVideoProbe(r.Context(), &task)
	if err != nil {
		common.ApiError(w, 502, videoProbeSafeError(err))
		return
	}
	writeVideoProbe(w, updated)
}

func writeVideoProbe(w http.ResponseWriter, task *model.Task) {
	var result dto.TaskResult
	_ = json.Unmarshal([]byte(task.ResultMeta), &result)
	videoURL := ""
	if task.Status == dto.TaskStatusSucceeded && len(result.Outputs) > 0 {
		videoURL = result.Outputs[0]
	}
	common.ApiSuccess(w, map[string]any{"id": task.ID, "model": task.Model, "status": task.Status, "video_url": videoURL, "message": task.FailReason})
}

func videoProbeSafeError(err error) string {
	switch err.Error() {
	case "video_probe_model_not_supported", "models_key_required", "video_probe_invalid_request", "video_probe_request_conflict", "video_probe_channel_changed", "video_probe_already_running", "video_probe_submission_uncertain", "upstream_video_authentication_failed", "upstream_video_permission_denied", "upstream_video_not_found", "upstream_video_rate_limited", "upstream_video_network_error", "upstream_video_invalid_response":
		return err.Error()
	default:
		return "video_probe_unavailable"
	}
}
