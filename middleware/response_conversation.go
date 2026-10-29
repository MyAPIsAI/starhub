package middleware

import (
	"encoding/json"
	"errors"
	"net/http"
	"starhub/common"
	"starhub/model"
)

func prepareResponseConversation(r *http.Request, body []byte) (*http.Request, []byte, error) {
	if r.URL.Path != "/v1/responses" && r.URL.Path != "/v1/responses/compact" {
		return r, body, nil
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(body, &fields) != nil || len(fields["conversation"]) == 0 || string(fields["conversation"]) == "null" {
		return r, body, nil
	}
	if len(fields["previous_response_id"]) != 0 && string(fields["previous_response_id"]) != "null" {
		return r, body, errors.New("conversation and previous_response_id cannot be combined")
	}
	var id string
	if json.Unmarshal(fields["conversation"], &id) != nil {
		var object struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(fields["conversation"], &object) != nil {
			return r, body, model.ErrConversationUnavailable
		}
		id = object.ID
	}
	owner, _ := r.Context().Value(common.CtxUserID).(int64)
	token, _ := r.Context().Value(common.CtxTokenID).(int64)
	conversation, err := model.GetResponseConversation(id, owner, token)
	if err != nil || conversation.Busy {
		return r, body, model.ErrConversationUnavailable
	}
	if string(fields["store"]) == "false" {
		return r, body, errors.New("gateway conversations require stored Responses")
	}
	if r.URL.Path == "/v1/responses/compact" {
		return r, body, errors.New("compact complete input or an owned previous response instead of a conversation")
	}
	delete(fields, "conversation")
	if conversation.LastResponseID != "" {
		fields["previous_response_id"], _ = json.Marshal(conversation.LastResponseID)
	}
	fields["store"] = json.RawMessage("true")
	body, err = json.Marshal(fields)
	r = r.WithContext(common.WithValues(r.Context(), common.CtxResponseConversation, conversation))
	return r, body, err
}
