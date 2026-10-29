package controller

import (
	"encoding/json"
	"io"
	"net/http"
	"starhub/common"
	"starhub/model"
)

func writeResponseConversation(w http.ResponseWriter, conversation *model.ResponseConversation) {
	var metadata map[string]string
	_ = json.Unmarshal([]byte(conversation.Metadata), &metadata)
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"id": conversation.ID, "object": "conversation", "created_at": conversation.CreatedAt, "metadata": metadata, "blocked": conversation.Busy})
}
func CreateResponseConversation(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Metadata map[string]string `json:"metadata"`
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	d.DisallowUnknownFields()
	if d.Decode(&input) != nil || d.Decode(new(any)) != io.EOF || len(input.Metadata) > 16 {
		writeModeError(w, 400, "invalid_request_error", "invalid_conversation", "create a gateway conversation with metadata only; send messages through Responses input")
		return
	}
	for k, v := range input.Metadata {
		if len(k) > 64 || len(v) > 512 {
			writeModeError(w, 400, "invalid_request_error", "invalid_conversation", "conversation metadata exceeds limits")
			return
		}
	}
	conversation, err := model.CreateResponseConversation(ctxValueInt64(r.Context(), common.CtxUserID), ctxValueInt64(r.Context(), common.CtxTokenID), input.Metadata)
	if err != nil {
		writeModeError(w, 409, "invalid_request_error", "conversation_unavailable", "conversation quota or account state changed")
		return
	}
	writeResponseConversation(w, conversation)
}
func GetResponseConversation(w http.ResponseWriter, r *http.Request) {
	conversation, err := model.GetResponseConversation(r.PathValue("id"), ctxValueInt64(r.Context(), common.CtxUserID), ctxValueInt64(r.Context(), common.CtxTokenID))
	if err != nil {
		writeModeError(w, 404, "invalid_request_error", "conversation_not_found", "conversation unavailable for this API key")
		return
	}
	if r.Method == http.MethodDelete {
		if err := model.DeleteResponseConversation(conversation); err != nil {
			writeModeError(w, 500, "server_error", "conversation_delete_unconfirmed", "conversation deletion is unconfirmed")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": conversation.ID, "object": "conversation.deleted", "deleted": true})
		return
	}
	writeResponseConversation(w, conversation)
}
