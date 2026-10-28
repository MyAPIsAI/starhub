package controller

import (
	"errors"
	"net/http"

	"starhub/common"
	"starhub/model"
)

func SetUserFinanceRole(w http.ResponseWriter, r *http.Request) {
	if requestRole(r) != common.RoleRoot {
		common.ApiError(w, http.StatusForbidden, "only Root can change finance access")
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var req struct {
		Enabled      *bool `json:"enabled"`
		ExpectedRole *int  `json:"expected_role"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Enabled == nil || req.ExpectedRole == nil {
		common.ApiError(w, http.StatusBadRequest, "enabled and expected_role are required")
		return
	}
	if err := model.SetUserFinanceRole(id, *req.Enabled, *req.ExpectedRole); err != nil {
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, model.ErrInvalidFinanceRole):
			status = http.StatusBadRequest
		case errors.Is(err, model.ErrUserRoleConflict):
			status = http.StatusConflict
		case errors.Is(err, model.ErrUserNotFound):
			status = http.StatusNotFound
		}
		common.ApiError(w, status, err.Error())
		return
	}
	common.ApiSuccess(w, nil)
}
