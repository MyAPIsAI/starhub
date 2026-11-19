package controller

import (
	"net/http"

	"starhub/common"
	"starhub/model"
)

func writeChannelResponses(w http.ResponseWriter, channels []model.Channel, single bool) {
	states, err := model.ReadChannelResourceStates(channels)
	if err != nil {
		common.ApiError(w, http.StatusInternalServerError, "channel resource state is unavailable")
		return
	}
	data := make([]map[string]any, 0, len(channels))
	for i := range channels {
		row := maskedChannel(&channels[i])
		row["resource_state"] = states[channels[i].ID]
		data = append(data, row)
	}
	if single {
		common.ApiSuccess(w, data[0])
	} else {
		common.ApiSuccess(w, data)
	}
}
