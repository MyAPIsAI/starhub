package middleware

import (
	"encoding/json"
	"errors"
	"net/http"
	"starhub/model"
	"starhub/relay"
)

func responseContinuation(r *http.Request, requested string, body []byte) (*model.ResponseReference, error) {
	if r.URL.Path != "/v1/responses" && r.URL.Path != "/v1/responses/compact" {
		return nil, nil
	}
	var input struct {
		Previous string `json:"previous_response_id"`
	}
	if json.Unmarshal(body, &input) != nil || input.Previous == "" {
		return nil, nil
	}
	ref, _, _, err := relay.OwnedResponseReference(r.Context(), input.Previous)
	if err != nil {
		return nil, err
	}
	if ref.Status != "completed" && ref.Status != "incomplete" {
		return nil, errors.New("previous response is not a successful terminal response")
	}
	if !isAutoModel(requested) && requested != ref.Model {
		return nil, relay.ErrResponseAccess
	}
	return ref, nil
}
