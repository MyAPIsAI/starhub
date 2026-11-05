package middleware

import (
	"net/http"

	"starhub/common"
	"starhub/config"
	"starhub/model"
	"starhub/service/authz"
)

// workspaceCookie never replaces an explicitly selected customer's session.
func workspaceCookie(r *http.Request) (string, bool) {
	if r.Header.Get("X-StarHub-Portal") != "workspace" {
		return "", false
	}
	if _, err := r.Cookie("session"); err == nil {
		return "", false
	}
	cookie, err := r.Cookie("staff_session")
	if err != nil || cookie.Value == "" {
		return "", false
	}
	return cookie.Value, true
}

func authenticatedStaff(r *http.Request, token string) (*model.User, bool) {
	state, ok := Sessions.StaffState(token)
	if !ok || !state.Verified || !config.StaffIPAllowed(common.ClientIP(r)) {
		return nil, false
	}
	user, err := model.GetStaffByID(state.UserID)
	if err != nil || user.AccountKind != "staff" || !common.IsStaffRole(user.Role) || user.Status != common.StatusEnabled || user.SecurityVersion != state.Version || user.MustChangePassword || user.TOTPSecret == "" {
		return nil, false
	}
	return user, true
}

// PlatformWorkspaceOwner also supplies personal group pricing on public catalog pages.
func PlatformWorkspaceOwner(r *http.Request) (*model.User, bool) {
	token, ok := workspaceCookie(r)
	if !ok {
		return nil, false
	}
	user, ok := authenticatedStaff(r, token)
	return user, ok && user.HasPlatformWorkspace()
}

func servePlatformWorkspace(w http.ResponseWriter, r *http.Request, next http.Handler, token string, privateFile bool) {
	user, ok := PlatformWorkspaceOwner(r)
	w.Header().Set("Cache-Control", "private, no-store")
	if !ok {
		if privateFile {
			common.ApiError(w, 404, "export not found")
		} else {
			common.ApiError(w, 401, "workspace sign-in required")
		}
		return
	}
	if !guardSessionOwner(w, r, user.ID) {
		return
	}
	// Customer self-service must never change staff credentials or close the staff identity.
	switch r.Method + " " + r.URL.Path {
	case "PUT /api/user/self", "PUT /api/user/profile-state", "POST /api/user/deactivate":
		common.ApiError(w, 403, "staff identities are managed in staff accounts")
		return
	}
	ctx := common.WithValues(r.Context(), common.CtxUserID, user.ID, common.CtxRole, common.RoleUser, workspaceSessionKey{}, token)
	next.ServeHTTP(w, r.WithContext(ctx))
}

// StaffNetwork applies before password authentication and every staff API call.
func StaffNetwork(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !config.StaffIPAllowed(common.ClientIP(r)) {
			common.ApiError(w, 403, "staff access requires the company network or VPN")
			return
		}
		w.Header().Set("Cache-Control", "private, no-store")
		next.ServeHTTP(w, r)
	})
}

// StaffAuth does not accept customer sessions or unenrolled password sessions.
func StaffAuth(next http.Handler) http.Handler {
	return StaffNetwork(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		state, ok := Sessions.StaffState(StaffSessionToken(r))
		if !ok {
			if _, customer := Sessions.Get(SessionToken(r)); customer {
				common.ApiError(w, 403, "customer sessions cannot access the staff portal")
				return
			}
			common.ApiError(w, 401, "staff login required")
			return
		}
		user, err := model.GetStaffByID(state.UserID)
		if err != nil || user.AccountKind != "staff" || !common.IsStaffRole(user.Role) || user.Status != common.StatusEnabled || state.Version != user.SecurityVersion {
			common.ApiError(w, 401, "staff session expired")
			return
		}
		if !state.Verified || user.MustChangePassword || user.TOTPSecret == "" {
			common.ApiError(w, 403, "staff password change and 2FA setup are required")
			return
		}
		if !guardSessionOwner(w, r, user.ID) {
			return
		}
		ctx := common.WithValues(r.Context(), common.CtxUserID, user.ID, common.CtxRole, user.Role, common.CtxAdminIP, common.ClientIP(r))
		next.ServeHTTP(w, r.WithContext(ctx))
	}))
}

func StaffPermission(next http.Handler) http.Handler {
	return StaffAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		role, _ := r.Context().Value(common.CtxRole).(int)
		if !authz.CheckRequest(role, r) {
			common.ApiError(w, 403, "operation is not permitted for this staff role")
			return
		}
		next.ServeHTTP(w, r)
	}))
}
