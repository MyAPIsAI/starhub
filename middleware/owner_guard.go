package middleware

import (
	"net/http"
	"strconv"

	"starhub/common"
)

const expectedOwnerHeader = "X-StarHub-Expected-User-ID"

// guardSessionOwner compares an optional UI expectation with the authenticated
// session. It never authenticates a request or grants additional privileges.
func guardSessionOwner(w http.ResponseWriter, r *http.Request, owner int64) bool {
	values := r.Header.Values(expectedOwnerHeader)
	if len(values) > 0 {
		expected, err := strconv.ParseInt(values[0], 10, 64)
		if len(values) != 1 || err != nil || expected <= 0 || strconv.FormatInt(expected, 10) != values[0] {
			common.ApiError(w, http.StatusBadRequest, "invalid expected account header")
			return false
		}
		if expected != owner {
			common.ApiError(w, http.StatusConflict, "current account differs from the confirmed account")
			return false
		}
	}
	w.Header().Set("X-StarHub-Owner-Guard", "v1")
	w.Header().Set("X-StarHub-User-ID", strconv.FormatInt(owner, 10))
	return true
}
