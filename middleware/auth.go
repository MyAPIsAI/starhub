package middleware

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"starhub/common"
	"starhub/model"
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
		}
		if key == "" {
			common.RelayError(w, http.StatusUnauthorized,
				"authentication_error", "missing_bearer_key",
				"missing Authorization: Bearer header")
			return
		}
		token, err := model.GetTokenByKey(key)
		if err == model.ErrTokenNotFound {
			common.RelayError(w, http.StatusUnauthorized,
				"authentication_error", "invalid_api_key",
				"invalid API key")
			return
		}
		if err != nil {
			common.RelayError(w, http.StatusInternalServerError,
				"server_error", "token_lookup_failed", err.Error())
			return
		}
		if token.Status != common.StatusEnabled {
			common.RelayError(w, http.StatusUnauthorized,
				"authentication_error", "token_disabled",
				"API key is disabled")
			return
		}
		if token.ExpiredTime > 0 && time.Now().Unix() > token.ExpiredTime {
			common.RelayError(w, http.StatusUnauthorized,
				"authentication_error", "token_expired",
				"API key has expired")
			return
		}
		if !token.UnlimitedQuota && token.RemainQuota <= 0 {
			common.RelayError(w, http.StatusForbidden,
				"insufficient_quota", "token_quota_exhausted",
				"API key quota is exhausted")
			return
		}
		user, err := model.GetUserByID(token.UserID)
		if err != nil || user.Status != common.StatusEnabled {
			common.RelayError(w, http.StatusUnauthorized,
				"authentication_error", "user_unavailable",
				"API key owner is unavailable")
			return
		}
		group := token.Group
		if group == "" {
			group = user.Group
			if group == "" {
				group = common.DefaultGroup
			}
		}
		if !setting.ContainsGroupRatio(group) {
			common.RelayError(w, http.StatusForbidden,
				"invalid_request_error", "group_not_available",
				fmt.Sprintf("group %q has no ratio configured", group))
			return
		}
		ctx := common.WithValues(r.Context(),
			common.CtxUserID, user.ID,
			common.CtxRole, user.Role,
			common.CtxTokenID, token.ID,
			common.CtxTokenName, token.Name,
			common.CtxTokenUnlimited, token.UnlimitedQuota,
			common.CtxUsingGroup, group)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
