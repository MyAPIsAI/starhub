package common

import (
	"encoding/json"
	"net/http"
)

type APIResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Data    any    `json:"data,omitempty"`
}

type OpenAIErrorBody struct {
	Error OpenAIError `json:"error"`
}

type OpenAIError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
}

func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func ApiSuccess(w http.ResponseWriter, data any) {
	WriteJSON(w, http.StatusOK, APIResponse{Success: true, Data: data})
}

func ApiError(w http.ResponseWriter, status int, message string) {
	WriteJSON(w, status, APIResponse{Success: false, Message: message})
}

func RelayError(w http.ResponseWriter, status int, errType, code, message string) {
	WriteJSON(w, status, OpenAIErrorBody{Error: OpenAIError{Message: message, Type: errType, Code: code}})
}
