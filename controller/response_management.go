package controller

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"starhub/model"
	"starhub/relay"
	"starhub/relay/channel"
	"starhub/service"
	"strings"
	"time"
)

func CancelResponse(w http.ResponseWriter, r *http.Request) { manageStoredResponse(w, r, false) }
func DeleteResponse(w http.ResponseWriter, r *http.Request) { manageStoredResponse(w, r, true) }
func manageStoredResponse(w http.ResponseWriter, r *http.Request, deleting bool) {
	w.Header().Set("Cache-Control", "private, no-store")
	if r.URL.RawQuery != "" {
		writeModeError(w, 400, "invalid_request_error", "invalid_request_body", "response management does not accept query fields")
		return
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1025))
	if err != nil || len(raw) > 1024 || (len(bytes.TrimSpace(raw)) > 0 && string(bytes.TrimSpace(raw)) != "{}") {
		writeModeError(w, 400, "invalid_request_error", "invalid_request_body", "response management accepts no body fields")
		return
	}
	ref, ch, key, err := relay.OwnedResponseReference(r.Context(), r.PathValue("id"))
	if err != nil {
		responseNotFound(w)
		return
	}
	if !deleting && !ref.Background {
		writeModeError(w, 400, "invalid_request_error", "response_not_cancellable", "only background responses can be cancelled")
		return
	}
	if deleting && (ref.Status == "queued" || ref.Status == "in_progress") {
		writeModeError(w, 409, "invalid_request_error", "response_active", "cancel the active background response before deleting it")
		return
	}
	adaptor, err := relay.GetChatAdaptor(ch.Type)
	if err != nil {
		responseNotFound(w)
		return
	}
	endpoint, err := adaptor.ModeURL(ch.BaseURL, channel.RelayModeResponses)
	if err != nil {
		responseNotFound(w)
		return
	}
	endpoint = strings.TrimRight(endpoint, "/") + "/" + url.PathEscape(ref.ResponseID)
	method := "DELETE"
	if !deleting {
		endpoint += "/cancel"
		method = "POST"
	}
	request, err := http.NewRequestWithContext(r.Context(), method, endpoint, nil)
	if err != nil || adaptor.PrepareRequest(request, key, nil) != nil {
		writeModeError(w, 502, "server_error", "response_management_failed", "response management request could not be prepared")
		return
	}
	request.Header.Set("Accept", "application/json")
	response, err := upstreamHTTPClient.Do(request)
	if err != nil {
		writeModeError(w, 502, "server_error", "response_management_uncertain", "response management outcome is unconfirmed")
		return
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		writeModeError(w, 502, "server_error", "response_management_uncertain", "provider did not confirm response management")
		return
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxModeResponseBytes+1))
	if err != nil || len(body) > maxModeResponseBytes {
		writeModeError(w, 502, "server_error", "response_management_uncertain", "provider management result is unreadable")
		return
	}
	if deleting {
		var result struct {
			ID      string `json:"id"`
			Deleted bool   `json:"deleted"`
		}
		if json.Unmarshal(body, &result) != nil || result.ID != ref.ResponseID || !result.Deleted {
			writeModeError(w, 502, "server_error", "response_management_uncertain", "provider deletion was not confirmed")
			return
		}
		if err := model.DB.Model(&model.ResponseReference{}).Where("id = ?", ref.ID).Update("deleted_at", time.Now().Unix()).Error; err != nil {
			writeModeError(w, 500, "server_error", "response_management_uncertain", "local response tombstone could not be saved")
			return
		}
	} else {
		var result struct{ ID, Status, Model string }
		if json.Unmarshal(body, &result) != nil || result.ID != ref.ResponseID || (result.Model != "" && result.Model != ref.Model) {
			writeModeError(w, 502, "server_error", "response_management_uncertain", "provider returned a mismatched response")
			return
		}
		task, err := model.GetTaskByID(ref.TaskID)
		if err != nil {
			writeModeError(w, 500, "server_error", "response_settlement_unavailable", "background billing metadata unavailable")
			return
		}
		if _, err := service.ObserveBackgroundResponse(task, body); err != nil {
			w.Header().Set("X-StarHub-Response-Settlement", "pending")
		}
	}
	writePassthroughBody(w, response, body)
}
