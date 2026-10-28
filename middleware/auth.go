package middleware

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"starhub/common"
	"starhub/model"
	"starhub/service/authz"
	"starhub/setting"
)

func UserAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid, ok := Sessions.Get(SessionToken(r))
		if !ok {
			common.ApiError(w, http.StatusUnauthorized, "login required")
			return
		}
		user, err := model.GetUserByID(uid)
		if err != nil || user.Status != common.StatusEnabled {
			common.ApiError(w, http.StatusUnauthorized, "user unavailable")
			return
		}
		ctx := common.WithValues(r.Context(),
			common.CtxUserID, user.ID,
			common.CtxRole, user.Role)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func AdminAuth(next http.Handler) http.Handler {
	return UserAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		role, _ := r.Context().Value(common.CtxRole).(int)
		if role < common.RoleAdmin {
			common.ApiError(w, http.StatusForbidden, "admin privilege required")
			return
		}
		if !authz.CheckRequest(role, r) {
			common.ApiError(w, http.StatusForbidden, "operation is not permitted for this role")
			return
		}
		next.ServeHTTP(w, r)
	}))
}

func TokenAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The gateway accepts both header styles: OpenAI SDK clients send
		// "Authorization: Bearer sk-..." while Claude SDK clients send
		// "x-api-key: sk-..." (both route to the same token table).
		authHeader := r.Header.Get("Authorization")
		key := ""
		if strings.HasPrefix(authHeader, "Bearer ") {
			key = strings.TrimSpace(authHeader[len("Bearer "):])
		} else {
			key = strings.TrimSpace(r.Header.Get("x-api-key"))
			if key == "" && isGeminiPath(r.URL.Path) {
				key = strings.TrimSpace(r.Header.Get("x-goog-api-key"))
			}
		}
		if key == "" {
			writeRelayRequestError(w, r, http.StatusUnauthorized,
				"authentication_error", "missing_bearer_key",
				"missing Authorization: Bearer header")
			return
		}
		token, err := model.GetTokenByKey(key)
		if err == model.ErrTokenNotFound {
			writeRelayRequestError(w, r, http.StatusUnauthorized,
				"authentication_error", "invalid_api_key",
				"invalid API key")
			return
		}
		if err != nil {
			writeRelayRequestError(w, r, http.StatusInternalServerError,
				"server_error", "token_lookup_failed", err.Error())
			return
		}
		if token.Status != common.StatusEnabled {
			writeRelayRequestError(w, r, http.StatusUnauthorized,
				"authentication_error", "token_disabled",
				"API key is disabled")
			return
		}
		if token.ExpiredTime > 0 && time.Now().Unix() > token.ExpiredTime {
			writeRelayRequestError(w, r, http.StatusUnauthorized,
				"authentication_error", "token_expired",
				"API key has expired")
			return
		}
		if !token.UnlimitedQuota && token.RemainQuota <= 0 {
			writeRelayRequestError(w, r, http.StatusForbidden,
				"insufficient_quota", "token_quota_exhausted",
				"API key quota is exhausted")
			return
		}
		// IP whitelist: enterprise customers restrict a key to known IPs to
		// prevent leakage.
		if !token.AllowsIP(clientIP(r)) {
			writeRelayRequestError(w, r, http.StatusForbidden,
				"authentication_error", "ip_not_allowed",
				"client IP is not in the token allow-list")
			return
		}
		user, err := model.GetUserByID(token.UserID)
		if err != nil || user.Status != common.StatusEnabled {
			writeRelayRequestError(w, r, http.StatusUnauthorized,
				"authentication_error", "user_unavailable",
				"API key owner is unavailable")
			return
		}
		// Model whitelist is enforced after the body is parsed (the model
		// name lives in the request body); the distributor stashes the
		// token pointer via context so it can run the check once the model
		// is known. For non-relay (management) paths the token never reaches
		// here, so we only need to wire the model whitelist into the relay
		// chain. We store the token's model limits on the context and let
		// the distributor enforce it.
		group := token.Group
		if group == "" {
			group = user.Group
			if group == "" {
				group = common.DefaultGroup
			}
		}
		var autoGroups []string
		if group == common.AutoGroup {
			autoGroups = token.AutoGroupList()
			if len(autoGroups) == 0 {
				writeRelayRequestError(w, r, http.StatusForbidden,
					"invalid_request_error", "auto_group_empty",
					"token uses the auto group but has no auto_groups configured")
				return
			}
		} else if !setting.ContainsGroupRatio(group) {
			writeRelayRequestError(w, r, http.StatusForbidden,
				"invalid_request_error", "group_not_available",
				fmt.Sprintf("group %q has no ratio configured", group))
			return
		}
		groups := []string{group}
		if group == common.AutoGroup {
			groups = autoGroups
		}
		access, err := model.ServiceGroupAccess(r.Context(), groups, user.ID)
		if err != nil {
			writeRelayRequestError(w, r, http.StatusInternalServerError,
				"server_error", "group_access_unavailable", "group access could not be verified")
			return
		}
		if group == common.AutoGroup {
			admitted := make([]string, 0, len(autoGroups))
			for _, candidate := range autoGroups {
				if candidate != common.AutoGroup && setting.ContainsGroupRatio(candidate) && access[candidate] {
					admitted = append(admitted, candidate)
				}
			}
			autoGroups = admitted
		}
		if (group == common.AutoGroup && len(autoGroups) == 0) || (group != common.AutoGroup && !access[group]) {
			writeRelayRequestError(w, r, http.StatusForbidden,
				"authentication_error", "group_not_allowed", "API key owner cannot access the configured group")
			return
		}
		ctx := common.WithValues(r.Context(),
			common.CtxUserID, user.ID,
			common.CtxRole, user.Role,
			common.CtxTokenID, token.ID,
			common.CtxTokenName, token.Name,
			common.CtxTokenUnlimited, token.UnlimitedQuota,
			common.CtxUsingGroup, group,
			common.CtxUserGroup, user.Group,
			common.CtxAutoGroups, autoGroups,
			common.CtxTokenModelLimits, token.ModelLimits)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
