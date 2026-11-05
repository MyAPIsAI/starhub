package controller

import (
	"net/http"
	"starhub/common"
)

// Customer identities are never promoted into internal staff accounts.
func SetUserFinanceRole(w http.ResponseWriter, r *http.Request) {
	if requestRole(r) != common.RoleRoot {
		common.ApiError(w, 403, "Root is required")
		return
	}
	common.ApiError(w, http.StatusConflict, "create a separate internal Finance account in staff management; customer accounts cannot be promoted")
}
