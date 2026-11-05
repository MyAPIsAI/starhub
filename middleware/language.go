package middleware

import (
	"context"
	"net/http"
	"strings"

	"starhub/common"
)

// CtxLanguage is the context key for the resolved request language.
type ctxLanguageKey struct{}

// CtxLanguageValue returns the language stored in the context, or "" if absent.
func CtxLanguageValue(ctx context.Context) string {
	v, _ := ctx.Value(ctxLanguageKey{}).(string)
	return v
}

// supportedLanguages are the languages StarHub supports.
var supportedLanguages = map[string]bool{
	"zh": true, "en": true, "ar": true, "ko": true,
}

// ResolveLanguage determines the request language using a four-tier priority:
//  1. Account language preference (if the user is logged in and has a preference)
//  2. Accept-Language header
//  3. Default "en"
//
// Region-level defaults (tier 3 in the PRD) will be wired when RegionProfile
// is consumed at the middleware layer; for now the fallback is English.
func ResolveLanguage(userLang, acceptLangHeader string) string {
	// 1. Account preference.
	if userLang != "" && supportedLanguages[userLang] {
		return userLang
	}
	// 2. Accept-Language header.
	if acceptLangHeader != "" {
		lang := parseAcceptLanguage(acceptLangHeader)
		if lang != "" && supportedLanguages[lang] {
			return lang
		}
	}
	// 3. Default.
	return "en"
}

// parseAcceptLanguage extracts the first supported language tag from
// an Accept-Language header. Normalizes zh-* to "zh".
func parseAcceptLanguage(header string) string {
	parts := strings.Split(header, ",")
	for _, part := range parts {
		tag := strings.TrimSpace(strings.Split(part, ";")[0])
		tag = strings.ToLower(tag)
		if strings.HasPrefix(tag, "zh") {
			return "zh"
		}
		if strings.HasPrefix(tag, "en") {
			return "en"
		}
		if strings.HasPrefix(tag, "ar") {
			return "ar"
		}
		if strings.HasPrefix(tag, "ko") {
			return "ko"
		}
	}
	return ""
}

// LanguageMiddleware resolves the request language and injects it into the
// context. When the user is authenticated, the user's language preference
// takes priority over the Accept-Language header.
func LanguageMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userLang, _ := r.Context().Value(common.CtxUserID).(int64)
		_ = userLang // UserID is available but the language is read from the user record in the auth handler.
		// For now, resolve from Accept-Language; the auth handler can override.
		lang := ResolveLanguage("", r.Header.Get("Accept-Language"))
		ctx := context.WithValue(r.Context(), ctxLanguageKey{}, lang)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
