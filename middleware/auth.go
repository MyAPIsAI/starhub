package middleware

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"starhub/common"
	"starhub/config"
	"starhub/model"
	"starhub/observability"
	"starhub/setting"
)

func UserAuth(next http.Handler) http.Handler {
	return userAuth(next, false)
}

// ExportDownloadAuth conceals private files from unauthenticated callers.
func ExportDownloadAuth(next http.Handler) http.Handler {
	return userAuth(next, true)
}

func userAuth(next http.Handler, privateFile bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		staffToken := StaffSessionToken(r)
		if state, ok := Sessions.StaffState(staffToken); ok && (r.Header.Get("X-StarHub-Portal") == "staff" || strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")) == staffToken) {
			if privateFile {
				user, err := model.GetUserByID(state.UserID)
				if err != nil || !state.Verified || user.AccountKind != "staff" || !common.IsStaffRole(user.Role) || user.Status != common.StatusEnabled || user.SecurityVersion != state.Version || user.MustChangePassword || user.TOTPSecret == "" || !config.StaffIPAllowed(common.ClientIP(r)) {
					w.Header().Set("Cache-Control", "private, no-store")
					common.ApiError(w, 404, "export not found")
					return
				}
			}
			StaffPermission(next).ServeHTTP(w, r)
			return
		}
		if token, ok := workspaceCookie(r); ok && r.Header.Get("Authorization") == "" {
			servePlatformWorkspace(w, r, next, token, privateFile)
			return
		}
		if privateFile {
			w.Header().Set("Cache-Control", "private, no-store")
		}
		reject := func(message string) {
			status := http.StatusUnauthorized
			if privateFile {
				status, message = http.StatusNotFound, "export not found"
			}
			common.ApiError(w, status, message)
		}
		uid, ok := Sessions.Get(SessionToken(r))
		if !ok {
			reject("login required")
			return
		}
		user, err := model.GetUserByID(uid)
		if err != nil || user.AccountKind != "customer" || common.IsStaffRole(user.Role) || user.Status != common.StatusEnabled {
			reject("user unavailable")
			return
		}
		if !guardSessionOwner(w, r, user.ID) {
			return
		}
		ctx := common.WithValues(r.Context(),
			common.CtxUserID, user.ID,
			common.CtxRole, user.Role)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func AdminAuth(next http.Handler) http.Handler {
	return StaffPermission(next)
}

func TokenAuth(next http.Handler) http.Handler {
	return tokenAuth(next, false, false)
}

// TokenReadOnlyAuth retains token, owner, IP and group authorization for
// existing-resource reads, without requiring unused generation quota.
func TokenReadOnlyAuth(next http.Handler) http.Handler {
	return tokenAuth(next, true, false)
}

// TokenResponseControlAuth retains all authorization checks without requiring
// generation quota for cancelling or deleting an already owned response.
func TokenResponseControlAuth(next http.Handler) http.Handler {
	return tokenAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		allowed := strings.HasPrefix(r.URL.Path, "/v1/responses/") && ((r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/cancel")) || r.Method == http.MethodDelete)
		if !allowed {
			writeRelayRequestError(w, r, 405, "invalid_request_error", "response_control_only", "this authorization path only manages existing responses")
			return
		}
		next.ServeHTTP(w, r)
	}), true, true)
}

func TokenConversationAuth(next http.Handler) http.Handler {
	return tokenAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if (r.URL.Path != "/v1/conversations" && !strings.HasPrefix(r.URL.Path, "/v1/conversations/")) || (r.Method != "GET" && r.Method != "POST" && r.Method != "DELETE") {
			writeRelayRequestError(w, r, 405, "invalid_request_error", "conversation_metadata_only", "this authorization path only manages conversation metadata")
			return
		}
		next.ServeHTTP(w, r)
	}), true, true)
}

func tokenAuth(next http.Handler, readOnly, control bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if readOnly {
			w.Header().Set("Cache-Control", "private, no-store")
			w.Header().Add("Vary", "Authorization")
			w.Header().Add("Vary", "X-Api-Key")
		}
		if readOnly && !control && r.Method != http.MethodGet && r.Method != http.MethodHead {
			writeRelayRequestError(w, r, http.StatusMethodNotAllowed, "invalid_request_error", "read_only_auth", "this authorization path only supports existing-resource reads")
			return
		}
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
		if !authorizePlayground(w, r, token) {
			return
		}
		if token.Status != common.StatusEnabled {
			writeRelayRequestError(w, r, http.StatusForbidden,
				"authentication_error", "key_disabled",
				"API key is disabled")
			return
		}
		if token.ExpiredTime > 0 && time.Now().Unix() >= token.ExpiredTime {
			writeRelayRequestError(w, r, http.StatusUnauthorized,
				"authentication_error", "key_expired",
				"API key has expired")
			return
		}
		if err := model.RefreshTokenPeriod(token, time.Now()); err != nil {
			writeRelayRequestError(w, r, 500, "server_error", "key_period_unavailable", "key quota period unavailable")
			return
		}
		if !readOnly && !token.UnlimitedQuota && token.RemainQuota <= 0 {
			writeRelayRequestError(w, r, http.StatusForbidden,
				"insufficient_quota", "key_quota_exceeded",
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
		if err != nil {
			writeRelayRequestError(w, r, http.StatusUnauthorized,
				"authentication_error", "user_unavailable",
				"API key owner is unavailable")
			return
		}
		if user.Status != common.StatusEnabled {
			writeRelayRequestError(w, r, http.StatusUnauthorized, "authentication_error", "account_disabled", "account is disabled")
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
		}
		groups := []string{group}
		if group == common.AutoGroup {
			groups = autoGroups
		}
		access, err := model.ServiceGroupAccessDetails(r.Context(), groups, user.ID)
		if err != nil {
			writeRelayRequestError(w, r, http.StatusInternalServerError,
				"server_error", "group_access_unavailable", "group access could not be verified")
			return
		}
		if group != common.AutoGroup && (access[group].Unavailable || !setting.ContainsGroupRatio(group)) {
			writeRelayRequestError(w, r, http.StatusForbidden,
				"invalid_request_error", "tier_capacity_unavailable",
				fmt.Sprintf("service group %q is unavailable", group))
			return
		}
		if group == common.AutoGroup {
			admitted := make([]string, 0, len(autoGroups))
			for _, candidate := range autoGroups {
				if candidate != common.AutoGroup && setting.ContainsGroupRatio(candidate) && access[candidate].Allowed {
					admitted = append(admitted, candidate)
				}
			}
			autoGroups = admitted
		}
		if (group == common.AutoGroup && len(autoGroups) == 0) || (group != common.AutoGroup && !access[group].Allowed) {
			writeRelayRequestError(w, r, http.StatusForbidden,
				"authentication_error", "group_not_allowed", "API key owner cannot access the configured group")
			return
		}
		if trace := observability.From(r.Context()); trace != nil {
			trace.Labels(user.ID, "", group, "")
			end := observability.Begin(r.Context(), "authentication", 0)
			end("success", "")
		}
		ctx := common.WithValues(r.Context(),
			common.CtxUserID, user.ID,
			common.CtxRole, user.Role,
			common.CtxTokenID, token.ID,
			common.CtxTokenName, token.Name,
			common.CtxTokenUnlimited, token.UnlimitedQuota,
			common.CtxTokenPeriodStartedAt, token.PeriodStartedAt,
			common.CtxUsingGroup, group,
			common.CtxUserGroup, user.Group,
			common.CtxAutoGroups, autoGroups,
			common.CtxTokenModelLimits, token.ModelLimits,
			common.CtxTokenSmartAccess, token.AllowsSmartRouting(),
			common.CtxTokenSmartCandidates, token.SmartCandidates)
		if r.Method == http.MethodPost {
			release, err := model.AcquireCustomerRequest(ctx, user.ID)
			if err != nil {
				if err == model.ErrCustomerConcurrency {
					writeRelayRequestError(w, r, 429, "rate_limit_error", "account_concurrency_exceeded", "account concurrent request limit reached")
				} else if err == model.ErrUserNotFound {
					writeRelayRequestError(w, r, 401, "authentication_error", "account_disabled", "account is disabled")
				} else {
					writeRelayRequestError(w, r, 503, "server_error", "account_admission_unavailable", "account concurrency state unavailable")
				}
				return
			}
			defer release()
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
