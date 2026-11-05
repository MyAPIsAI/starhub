package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResolveLanguageAccountPreference(t *testing.T) {
	if got := ResolveLanguage("zh", "en"); got != "zh" {
		t.Fatalf("account preference should win: got %s", got)
	}
}

func TestResolveLanguageAcceptHeader(t *testing.T) {
	cases := []struct {
		header string
		want   string
	}{
		{"zh-CN,zh;q=0.9,en;q=0.8", "zh"},
		{"en-US,en;q=0.9", "en"},
		{"ar,en;q=0.8", "ar"},
		{"ko-KR,ko;q=0.9", "ko"},
		{"fr-FR,fr;q=0.9", "en"}, // unsupported → default
		{"", "en"},               // empty → default
	}
	for _, tc := range cases {
		got := ResolveLanguage("", tc.header)
		if got != tc.want {
			t.Errorf("ResolveLanguage(\"\", %q) = %s, want %s", tc.header, got, tc.want)
		}
	}
}

func TestResolveLanguageUnsupportedAccount(t *testing.T) {
	if got := ResolveLanguage("fr", "zh"); got != "zh" {
		t.Fatalf("unsupported account language should fall through to Accept-Language: got %s", got)
	}
}

func TestResolveLanguageDefault(t *testing.T) {
	if got := ResolveLanguage("", ""); got != "en" {
		t.Fatalf("default should be en, got %s", got)
	}
}

func TestLanguageMiddleware(t *testing.T) {
	called := false
	handler := LanguageMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		lang := CtxLanguageValue(r.Context())
		if lang != "zh" {
			t.Fatalf("expected zh in context, got %s", lang)
		}
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/user/self", nil)
	req.Header.Set("Accept-Language", "zh-CN,en;q=0.8")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if !called {
		t.Fatal("handler was not called")
	}
}
