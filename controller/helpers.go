package controller

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"starhub/common"
)

func decodeJSONBody(w http.ResponseWriter, r *http.Request, v any) bool {
	defer r.Body.Close()
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(v); err != nil {
		common.ApiError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return false
	}
	return true
}

func pathID(w http.ResponseWriter, r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil || id <= 0 {
		common.ApiError(w, http.StatusBadRequest, "invalid "+name+" in path")
		return 0, false
	}
	return id, true
}

func requestUserID(r *http.Request) int64 {
	v, _ := r.Context().Value(common.CtxUserID).(int64)
	return v
}

func requestRole(r *http.Request) int {
	v, _ := r.Context().Value(common.CtxRole).(int)
	return v
}
